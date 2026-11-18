package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
)

// MoveIssue moves an issue, with its subissues, review rounds, findings, comments, and
// attachments, to another project, where it takes that project's next number and so a new key.
// Labels follow by name, and are created in the target project when it has none of that name;
// the milestone follows when the target has one of the same name, and is cleared otherwise.
// Links to a parent issue and to child issues cross no project, so they are cleared. The move is
// refused when another issue of the target project already uses the issue's branch name. People
// and agents can move issues.
func (s *Store) MoveIssue(ctx context.Context, by Caller, issueID, targetProjectID string) (Issue, error) {
	var moved Issue
	err := s.write(ctx, by, func(w *writeTx) error {
		current, err := getIssue(w.ctx, w.tx, `i.id = ?`, issueID)
		if err != nil {
			return err
		}
		if current.ProjectID == targetProjectID {
			return invalid("project_id", "%s is already in this project", current.Key())
		}
		target, err := getProject(w.ctx, w.tx, `id = ?`, targetProjectID)
		if errors.Is(err, ErrNotFound) {
			return invalid("project_id", "must be an existing project")
		}
		if err != nil {
			return err
		}

		// Branch names are unique within a project, so a clash cannot be moved past.
		if current.BranchName != "" {
			var taken int
			err := w.tx.QueryRowContext(w.ctx, `SELECT count(*) FROM issues WHERE project_id = ? AND branch_name = ?`,
				target.ID, current.BranchName).Scan(&taken)
			if err != nil {
				return fmt.Errorf("check branch name: %w", err)
			}
			if taken > 0 {
				return Refuse(ErrConflict, "another issue in %s already uses branch %s; change the branch name of %s first",
					target.Key, current.BranchName, current.Key())
			}
		}

		// The transaction holds the write lock, so no other writer can take the same number.
		var number int
		err = w.tx.QueryRowContext(w.ctx, `
			UPDATE projects SET next_issue_number = next_issue_number + 1, updated_at = ?
			WHERE id = ? RETURNING next_issue_number - 1`, w.timestamp(), target.ID).Scan(&number)
		if err != nil {
			return fmt.Errorf("take issue number: %w", err)
		}

		labelMap, createdLabels, err := mapLabels(w, current.ID, target.ID)
		if err != nil {
			return err
		}
		milestoneID, droppedMilestone, err := mapMilestone(w, current.MilestoneID, target.ID)
		if err != nil {
			return err
		}

		// Child issues stay behind; the parent link and the children's links to this issue go.
		children, err := w.exec(`UPDATE issues SET parent_issue_id = NULL, updated_at = ? WHERE parent_issue_id = ?`,
			w.timestamp(), current.ID)
		if err != nil {
			return fmt.Errorf("detach child issues: %w", err)
		}
		detached, err := children.RowsAffected()
		if err != nil {
			return fmt.Errorf("count child issues: %w", err)
		}

		_, err = w.exec(`
			UPDATE issues SET project_id = ?, number = ?, milestone_id = ?, parent_issue_id = NULL, updated_at = ?
			WHERE id = ?`, target.ID, number, nullString(milestoneID), w.timestamp(), current.ID)
		if err != nil {
			return fmt.Errorf("move issue: %w", err)
		}
		if _, err := w.exec(`UPDATE attachments SET project_id = ? WHERE issue_id = ?`, target.ID, current.ID); err != nil {
			return fmt.Errorf("move attachments: %w", err)
		}
		if err := remapLabels(w, current.ID, labelMap); err != nil {
			return err
		}

		moved, err = getIssue(w.ctx, w.tx, `i.id = ?`, current.ID)
		if err != nil {
			return err
		}

		payload := map[string]any{
			"from_project_id": current.ProjectID, "from_key": current.Key(), "to_key": moved.Key(),
			"to_project_id": target.ID, "labels_created": createdLabels, "children_detached": detached,
		}
		if current.ParentIssueID != "" {
			payload["parent_cleared"] = current.ParentIssueID
		}
		if droppedMilestone != "" {
			payload["milestone_cleared"] = droppedMilestone
		}
		return w.event(target.ID, TargetIssue, current.ID, "issue.moved", payload)
	})

	return moved, err
}

// mapLabels finds the target project's counterpart, by name, of every label on the issue or its
// subissues, creating those it lacks with the same color. It returns the source-to-target IDs and
// the names of the labels it created.
func mapLabels(w *writeTx, issueID, targetProjectID string) (map[string]string, []string, error) {
	rows, err := w.tx.QueryContext(w.ctx, `
		SELECT l.id, l.name, l.color FROM labels l WHERE l.id IN (
			SELECT label_id FROM issue_labels WHERE issue_id = ?
			UNION
			SELECT sl.label_id FROM subissue_labels sl JOIN subissues s ON s.id = sl.subissue_id WHERE s.issue_id = ?
		) ORDER BY l.name`, issueID, issueID)
	if err != nil {
		return nil, nil, fmt.Errorf("list labels to move: %w", err)
	}

	type source struct{ id, name, color string }
	var sources []source
	for rows.Next() {
		var label source
		if err := rows.Scan(&label.id, &label.name, &label.color); err != nil {
			rows.Close()
			return nil, nil, fmt.Errorf("read label: %w", err)
		}
		sources = append(sources, label)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, nil, fmt.Errorf("list labels to move: %w", err)
	}
	rows.Close()

	mapped, created := map[string]string{}, []string{}
	for _, label := range sources {
		var targetID string
		err := w.tx.QueryRowContext(w.ctx, `SELECT id FROM labels WHERE project_id = ? AND name = ?`,
			targetProjectID, label.name).Scan(&targetID)
		if errors.Is(err, sql.ErrNoRows) {
			targetID = newID()
			_, err = w.exec(`INSERT INTO labels (id, project_id, name, color, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
				targetID, targetProjectID, label.name, label.color, w.timestamp(), w.timestamp())
			if err != nil {
				return nil, nil, fmt.Errorf("create label %s: %w", label.name, err)
			}
			err = w.event(targetProjectID, TargetLabel, targetID, "label.created", map[string]any{"name": label.name, "color": label.color})
			if err != nil {
				return nil, nil, err
			}
			created = append(created, label.name)
		} else if err != nil {
			return nil, nil, fmt.Errorf("find label %s: %w", label.name, err)
		}
		mapped[label.id] = targetID
	}

	sort.Strings(created)
	return mapped, created, nil
}

// mapMilestone returns the target project's milestone with the same name as the issue's, or ""
// with the dropped milestone's name when it has none.
func mapMilestone(w *writeTx, milestoneID, targetProjectID string) (string, string, error) {
	if milestoneID == "" {
		return "", "", nil
	}

	var name string
	if err := w.tx.QueryRowContext(w.ctx, `SELECT name FROM milestones WHERE id = ?`, milestoneID).Scan(&name); err != nil {
		return "", "", fmt.Errorf("read milestone: %w", err)
	}

	var targetID string
	err := w.tx.QueryRowContext(w.ctx, `SELECT id FROM milestones WHERE project_id = ? AND name = ?`, targetProjectID, name).Scan(&targetID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", name, nil
	}
	if err != nil {
		return "", "", fmt.Errorf("find milestone %s: %w", name, err)
	}
	return targetID, "", nil
}

// remapLabels points the issue's and its subissues' label rows at the target project's labels.
func remapLabels(w *writeTx, issueID string, labelMap map[string]string) error {
	for from, to := range labelMap {
		_, err := w.exec(`UPDATE issue_labels SET label_id = ? WHERE issue_id = ? AND label_id = ?`, to, issueID, from)
		if err != nil {
			return fmt.Errorf("move issue label: %w", err)
		}
		_, err = w.exec(`
			UPDATE subissue_labels SET label_id = ?
			WHERE label_id = ? AND subissue_id IN (SELECT id FROM subissues WHERE issue_id = ?)`, to, from, issueID)
		if err != nil {
			return fmt.Errorf("move subissue label: %w", err)
		}
	}
	return nil
}
