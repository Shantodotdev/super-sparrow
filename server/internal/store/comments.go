package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

const commentColumns = `id, issue_id, finding_id, parent_comment_id, body, author_actor_id, edited_at, created_at, updated_at`

// NewComment describes a comment on exactly one issue or finding.
type NewComment struct {
	IssueID   string
	FindingID string
	// ParentCommentID makes the comment a reply. The parent must be on the same issue or finding.
	ParentCommentID string
	Body            string
}

// AddComment adds a comment by the caller. People and agents can comment.
func (s *Store) AddComment(ctx context.Context, by Caller, input NewComment) (Comment, error) {
	if (input.IssueID == "") == (input.FindingID == "") {
		return Comment{}, invalid("target", "must be exactly one of issue_id and finding_id")
	}
	body, err := requiredText("body", input.Body, maxCommentLength)
	if err != nil {
		return Comment{}, err
	}
	input.Body = body

	var comment Comment
	err = s.write(ctx, by, func(w *writeTx) error {
		projectID, issueID, err := commentTarget(w, input.IssueID, input.FindingID)
		if err != nil {
			return err
		}

		comment, err = insertComment(w, projectID, issueID, input)
		return err
	})

	return comment, err
}

// insertComment adds a comment whose body is already validated and whose target exists, and
// records the event. issueID is the issue the target belongs to.
func insertComment(w *writeTx, projectID, issueID string, input NewComment) (Comment, error) {
	if input.ParentCommentID != "" {
		parent, err := getComment(w.ctx, w.tx, input.ParentCommentID)
		if errors.Is(err, ErrNotFound) || (err == nil && (parent.IssueID != input.IssueID || parent.FindingID != input.FindingID)) {
			return Comment{}, invalid("parent_comment_id", "must be a comment on the same issue or finding")
		}
		if err != nil {
			return Comment{}, err
		}
	}

	comment := Comment{
		ID: newID(), IssueID: input.IssueID, FindingID: input.FindingID, ParentCommentID: input.ParentCommentID,
		Body: input.Body, AuthorActorID: w.caller.ID, CreatedAt: w.now, UpdatedAt: w.now,
	}
	_, err := w.exec(`
		INSERT INTO comments (id, issue_id, finding_id, parent_comment_id, body, author_actor_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		comment.ID, nullString(input.IssueID), nullString(input.FindingID), nullString(input.ParentCommentID),
		comment.Body, w.caller.ID, w.timestamp(), w.timestamp())
	if err != nil {
		return Comment{}, fmt.Errorf("add comment: %w", err)
	}

	return comment, w.event(projectID, TargetComment, comment.ID, "comment.added", map[string]any{
		"issue_id": issueID, "finding_id": input.FindingID, "parent_comment_id": input.ParentCommentID,
	})
}

// EditComment replaces a comment's body. Only the comment's author can edit it.
func (s *Store) EditComment(ctx context.Context, by Caller, commentID, body string) (Comment, error) {
	body, err := requiredText("body", body, maxCommentLength)
	if err != nil {
		return Comment{}, err
	}

	var comment Comment
	err = s.write(ctx, by, func(w *writeTx) error {
		current, err := getComment(w.ctx, w.tx, commentID)
		if err != nil {
			return err
		}
		if current.AuthorActorID != w.caller.ID {
			return forbidden("only the author can edit a comment")
		}
		if current.Body == body {
			comment = current
			return nil
		}

		projectID, issueID, err := commentTarget(w, current.IssueID, current.FindingID)
		if err != nil {
			return err
		}

		_, err = w.exec(`UPDATE comments SET body = ?, edited_at = ?, updated_at = ? WHERE id = ?`,
			body, w.timestamp(), w.timestamp(), current.ID)
		if err != nil {
			return fmt.Errorf("edit comment: %w", err)
		}

		comment = current
		comment.Body, comment.EditedAt, comment.UpdatedAt = body, &w.now, w.now
		return w.event(projectID, TargetComment, current.ID, "comment.edited", map[string]any{
			"issue_id": issueID, "finding_id": current.FindingID,
		})
	})

	return comment, err
}

// ListComments returns the comments on an issue or a finding, oldest first. Exactly one of
// issueID and findingID must be given.
func (s *Store) ListComments(ctx context.Context, issueID, findingID string) ([]Comment, error) {
	if (issueID == "") == (findingID == "") {
		return nil, invalid("target", "must be exactly one of issue_id and finding_id")
	}

	column, target := "issue_id", issueID
	if findingID != "" {
		column, target = "finding_id", findingID
	}

	rows, err := s.db.QueryContext(ctx, `SELECT `+commentColumns+` FROM comments WHERE `+column+` = ? ORDER BY created_at, id`, target)
	if err != nil {
		return nil, fmt.Errorf("list comments: %w", err)
	}
	defer rows.Close()

	var comments []Comment
	for rows.Next() {
		comment, err := scanComment(rows)
		if err != nil {
			return nil, err
		}
		comments = append(comments, comment)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list comments: %w", err)
	}
	return comments, nil
}

// commentTarget checks that the commented issue or finding exists and returns its project, which
// the comment's event belongs to, and its issue, which every comment event names so the comment
// appears in the issue's history.
func commentTarget(w *writeTx, issueID, findingID string) (string, string, error) {
	query := `SELECT project_id, id FROM issues WHERE id = ?`
	target, what := issueID, "issue"
	if findingID != "" {
		query = `SELECT i.project_id, i.id FROM findings f JOIN review_rounds r ON r.id = f.review_round_id
			JOIN issues i ON i.id = r.issue_id WHERE f.id = ?`
		target, what = findingID, "finding"
	}

	var projectID, targetIssueID string
	err := w.tx.QueryRowContext(w.ctx, query, target).Scan(&projectID, &targetIssueID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", notFound(what)
	}
	if err != nil {
		return "", "", fmt.Errorf("find comment target: %w", err)
	}
	return projectID, targetIssueID, nil
}

func getComment(ctx context.Context, q querier, id string) (Comment, error) {
	return scanComment(q.QueryRowContext(ctx, `SELECT `+commentColumns+` FROM comments WHERE id = ?`, id))
}

func scanComment(row scanner) (Comment, error) {
	var comment Comment
	var issueID, findingID, parentID, edited sql.NullString
	var created, updated string

	err := row.Scan(&comment.ID, &issueID, &findingID, &parentID, &comment.Body, &comment.AuthorActorID, &edited, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return Comment{}, notFound("comment")
	}
	if err != nil {
		return Comment{}, fmt.Errorf("read comment: %w", err)
	}

	comment.IssueID, comment.FindingID, comment.ParentCommentID = issueID.String, findingID.String, parentID.String
	if comment.EditedAt, err = parseOptionalTime(edited); err != nil {
		return Comment{}, err
	}
	if comment.CreatedAt, comment.UpdatedAt, err = timestamps(created, updated); err != nil {
		return Comment{}, err
	}
	return comment, nil
}
