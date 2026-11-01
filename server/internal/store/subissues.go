package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Subissue is a lightweight part of an issue. Reviews and discussions belong to its issue.
type Subissue struct {
	ID                  string
	IssueID             string
	Number              int
	Title               string
	Description         string
	DescriptionRevision int
	Status              IssueStatus
	CreatedByID         string
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

// NewSubissue supplies the content and explicit initial status of a subissue.
type NewSubissue struct {
	Title       string
	Description string
	Status      IssueStatus
}

// SubissueUpdate changes only the supplied fields; description edits require their starting revision.
type SubissueUpdate struct {
	Title               Field[string]
	Description         Field[string]
	DescriptionRevision int
	Status              Field[IssueStatus]
}

const subissueColumns = `id, issue_id, number, title, description, description_revision, status,
	created_by_actor_id, created_at, updated_at`

// CreateSubissue allocates a number under the issue and records the authenticated actor atomically.
func (s *Store) CreateSubissue(ctx context.Context, by Caller, issueID string, input NewSubissue) (Subissue, error) {
	title, err := requiredText("title", input.Title, maxTitleLength)
	if err != nil {
		return Subissue{}, err
	}
	description, err := optionalText("description", input.Description, maxTextLength)
	if err != nil {
		return Subissue{}, err
	}
	if !input.Status.valid() {
		return Subissue{}, invalid("status", "must be backlog, todo, in_progress, in_review, done, or canceled")
	}

	var subissue Subissue
	err = s.write(ctx, by, func(w *writeTx) error {
		issue, err := getIssue(w.ctx, w.tx, `i.id = ?`, issueID)
		if err != nil {
			return err
		}

		var number int
		if err := w.tx.QueryRowContext(w.ctx, `UPDATE issues SET next_subissue_number = next_subissue_number + 1
			WHERE id = ? RETURNING next_subissue_number - 1`, issue.ID).Scan(&number); err != nil {
			return fmt.Errorf("take subissue number: %w", err)
		}
		subissue = Subissue{ID: newID(), IssueID: issue.ID, Number: number, Title: title,
			Description: description, DescriptionRevision: 1, Status: input.Status,
			CreatedByID: w.caller.ID, CreatedAt: w.now, UpdatedAt: w.now}
		_, err = w.exec(`INSERT INTO subissues (`+subissueColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			subissue.ID, issue.ID, number, title, description, 1, input.Status, w.caller.ID, w.timestamp(), w.timestamp())
		if err != nil {
			return fmt.Errorf("create subissue: %w", err)
		}

		return w.event(issue.ProjectID, TargetIssue, issue.ID, "issue.subissue_created", map[string]any{
			"subissue_id": subissue.ID, "number": number, "title": title, "status": input.Status,
		})
	})
	return subissue, err
}

// ListSubissues returns all lightweight subissues of an issue in number order.
func (s *Store) ListSubissues(ctx context.Context, issueID string) ([]Subissue, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+subissueColumns+` FROM subissues WHERE issue_id = ? ORDER BY number`, issueID)
	if err != nil {
		return nil, fmt.Errorf("list subissues: %w", err)
	}
	defer rows.Close()

	subissues := make([]Subissue, 0)
	for rows.Next() {
		subissue, err := scanSubissue(rows)
		if err != nil {
			return nil, err
		}
		subissues = append(subissues, subissue)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list subissues: %w", err)
	}
	return subissues, nil
}

// UpdateSubissue edits a subissue scoped to its issue, refusing stale description edits.
func (s *Store) UpdateSubissue(ctx context.Context, by Caller, issueID, subissueID string, update SubissueUpdate) (Subissue, error) {
	var err error
	if update.Title.Set {
		if update.Title.Value, err = requiredText("title", update.Title.Value, maxTitleLength); err != nil {
			return Subissue{}, err
		}
	}
	if update.Description.Set {
		if update.Description.Value, err = optionalText("description", update.Description.Value, maxTextLength); err != nil {
			return Subissue{}, err
		}
		if update.DescriptionRevision < 1 {
			return Subissue{}, invalid("description_revision", "is required with description")
		}
	}
	if update.Status.Set && !update.Status.Value.valid() {
		return Subissue{}, invalid("status", "must be backlog, todo, in_progress, in_review, done, or canceled")
	}

	var subissue Subissue
	err = s.write(ctx, by, func(w *writeTx) error {
		issue, err := getIssue(w.ctx, w.tx, `i.id = ?`, issueID)
		if err != nil {
			return err
		}
		subissue, err = scanSubissue(w.tx.QueryRowContext(w.ctx, `SELECT `+subissueColumns+` FROM subissues WHERE id = ? AND issue_id = ?`, subissueID, issue.ID))
		if err != nil {
			return err
		}

		changes := map[string]any{}
		applyChange(update.Title, &subissue.Title, "title", changes)
		if update.Status.Set && update.Status.Value != subissue.Status {
			changes["status"] = map[string]any{"from": subissue.Status, "to": update.Status.Value}
			subissue.Status = update.Status.Value
		}
		if update.Description.Set && update.Description.Value != subissue.Description {
			if update.DescriptionRevision != subissue.DescriptionRevision {
				return Refuse(ErrConflict, "subissue description changed; reload it before saving")
			}
			subissue.Description = update.Description.Value
			subissue.DescriptionRevision++
			changes["description"] = map[string]any{"revision": subissue.DescriptionRevision}
		}
		if len(changes) == 0 {
			return nil
		}

		_, err = w.exec(`UPDATE subissues SET title = ?, description = ?, description_revision = ?, status = ?, updated_at = ? WHERE id = ?`,
			subissue.Title, subissue.Description, subissue.DescriptionRevision, subissue.Status, w.timestamp(), subissue.ID)
		if err != nil {
			return fmt.Errorf("update subissue %s: %w", subissue.ID, err)
		}
		subissue.UpdatedAt = w.now
		return w.event(issue.ProjectID, TargetIssue, issue.ID, "issue.subissue_updated", map[string]any{
			"subissue_id": subissue.ID, "number": subissue.Number, "changes": changes,
		})
	})
	return subissue, err
}

// scanSubissue reads a subissue and parses its stored timestamps.
func scanSubissue(row scanner) (Subissue, error) {
	var subissue Subissue
	var created, updated string
	err := row.Scan(&subissue.ID, &subissue.IssueID, &subissue.Number, &subissue.Title, &subissue.Description,
		&subissue.DescriptionRevision, &subissue.Status, &subissue.CreatedByID, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return Subissue{}, ErrNotFound
	}
	if err != nil {
		return Subissue{}, fmt.Errorf("read subissue: %w", err)
	}
	subissue.CreatedAt, subissue.UpdatedAt, err = timestamps(created, updated)
	return subissue, err
}
