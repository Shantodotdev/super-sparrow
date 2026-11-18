package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
)

// maxIssuesPerMove bounds one move, so a single request holds the write lock only briefly.
const maxIssuesPerMove = 200

// MoveIssue moves one issue to another project, as MoveIssues does.
func (s *Store) MoveIssue(ctx context.Context, by Caller, issueID, targetProjectID string) (Issue, error) {
	moved, err := s.MoveIssues(ctx, by, []string{issueID}, targetProjectID)
	if err != nil {
		return Issue{}, err
	}
	return moved[0], nil
}

// MoveIssues moves issues, with their subissues, review rounds, findings, comments, and
// attachments, to another project in one transaction. Each takes the target's next number in the
// order given, so its key changes, and its old key keeps resolving to it. Labels follow by name,
// and are created in the target project when it has none of that name; the milestone follows when
// the target has one of the same name, and is cleared otherwise. Parent links between the moved
// issues are kept; links to issues that stay behind are cleared, and each child left behind
// records that change. The move is refused when an issue is already in the target project, or
// when its branch name is used in the target project or by another moved issue. People and agents
// can move issues.
func (s *Store) MoveIssues(ctx context.Context, by Caller, issueIDs []string, targetProjectID string) ([]Issue, error) {
	if len(issueIDs) == 0 {
		return nil, invalid("issue_ids", "must name at least one issue")
	}
	if len(issueIDs) > maxIssuesPerMove {
		return nil, invalid("issue_ids", "must have at most %d issues", maxIssuesPerMove)
	}

	var moved []Issue
	err := s.write(ctx, by, func(w *writeTx) error {
		target, err := getProject(w.ctx, w.tx, `id = ?`, targetProjectID)
		if errors.Is(err, ErrNotFound) {
			return invalid("project_id", "must be an existing project")
		}
		if err != nil {
			return err
		}

		// Every issue is checked before any of them changes, so a refusal leaves all in place.
		issues, moving, err := loadIssuesToMove(w, issueIDs, target)
		if err != nil {
			return err
		}

		// Each issue moves without its parent first: the database requires a parent in its child's
		// project, and parents later in the list have not moved yet.
		payloads := make([]map[string]any, len(issues))
		for index, issue := range issues {
			if payloads[index], err = moveIssue(w, issue, target, moving); err != nil {
				return err
			}
		}
		for _, issue := range issues {
			if issue.ParentIssueID == "" || !moving[issue.ParentIssueID] {
				continue
			}
			if _, err := w.exec(`UPDATE issues SET parent_issue_id = ? WHERE id = ?`, issue.ParentIssueID, issue.ID); err != nil {
				return fmt.Errorf("keep parent of %s: %w", issue.Key(), err)
			}
		}

		moved = make([]Issue, 0, len(issues))
		for index, issue := range issues {
			current, err := getIssue(w.ctx, w.tx, `i.id = ?`, issue.ID)
			if err != nil {
				return err
			}
			payloads[index]["to_key"] = current.Key()
			if err := w.event(target.ID, TargetIssue, issue.ID, "issue.moved", payloads[index]); err != nil {
				return err
			}
			moved = append(moved, current)
		}
		return nil
	})

	return moved, err
}

// loadIssuesToMove reads the issues to move, without repeats, and refuses the move when one of
// them is already in the target project or would take a branch name the target already has.
// It returns the issues in order and the set of their IDs.
func loadIssuesToMove(w *writeTx, issueIDs []string, target Project) ([]Issue, map[string]bool, error) {
	issues := make([]Issue, 0, len(issueIDs))
	moving := map[string]bool{}
	branches := map[string]string{}
	for _, id := range issueIDs {
		if moving[id] {
			continue
		}
		issue, err := getIssue(w.ctx, w.tx, `i.id = ?`, id)
		if err != nil {
			return nil, nil, err
		}
		if issue.ProjectID == target.ID {
			return nil, nil, invalid("project_id", "%s is already in this project", issue.Key())
		}

		// Branch names are unique within a project, so a clash cannot be moved past.
		if issue.BranchName != "" {
			if other, ok := branches[issue.BranchName]; ok {
				return nil, nil, Refuse(ErrConflict, "%s and %s both use branch %s; change the branch name of one of them first",
					other, issue.Key(), issue.BranchName)
			}
			branches[issue.BranchName] = issue.Key()

			var taken int
			err := w.tx.QueryRowContext(w.ctx, `SELECT count(*) FROM issues WHERE project_id = ? AND branch_name = ?`,
				target.ID, issue.BranchName).Scan(&taken)
			if err != nil {
				return nil, nil, fmt.Errorf("check branch name: %w", err)
			}
			if taken > 0 {
				return nil, nil, Refuse(ErrConflict, "another issue in %s already uses branch %s; change the branch name of %s first",
					target.Key, issue.BranchName, issue.Key())
			}
		}

		moving[id] = true
		issues = append(issues, issue)
	}
	return issues, moving, nil
}

// moveIssue moves one issue to the target project with no parent, keeps its old key as an alias,
// and detaches its children that are not moving. It returns the payload of its move event.
func moveIssue(w *writeTx, current Issue, target Project, moving map[string]bool) (map[string]any, error) {
	// The transaction holds the write lock, so no other writer can take the same number.
	var number int
	err := w.tx.QueryRowContext(w.ctx, `
		UPDATE projects SET next_issue_number = next_issue_number + 1, updated_at = ?
		WHERE id = ? RETURNING next_issue_number - 1`, w.timestamp(), target.ID).Scan(&number)
	if err != nil {
		return nil, fmt.Errorf("take issue number: %w", err)
	}

	labelMap, createdLabels, err := mapLabels(w, current.ID, target.ID)
	if err != nil {
		return nil, err
	}
	milestoneID, droppedMilestone, err := mapMilestone(w, current.MilestoneID, target.ID)
	if err != nil {
		return nil, err
	}
	detached, err := detachChildren(w, current, moving)
	if err != nil {
		return nil, err
	}

	_, err = w.exec(`
		UPDATE issues SET project_id = ?, number = ?, milestone_id = ?, parent_issue_id = NULL, updated_at = ?
		WHERE id = ?`, target.ID, number, nullString(milestoneID), w.timestamp(), current.ID)
	if err != nil {
		return nil, fmt.Errorf("move issue: %w", err)
	}
	_, err = w.exec(`INSERT INTO issue_key_aliases (project_id, number, issue_id, created_at) VALUES (?, ?, ?, ?)`,
		current.ProjectID, current.Number, current.ID, w.timestamp())
	if err != nil {
		return nil, fmt.Errorf("keep old key %s: %w", current.Key(), err)
	}
	if _, err := w.exec(`UPDATE attachments SET project_id = ? WHERE issue_id = ?`, target.ID, current.ID); err != nil {
		return nil, fmt.Errorf("move attachments: %w", err)
	}
	if err := remapLabels(w, current.ID, labelMap); err != nil {
		return nil, err
	}

	payload := map[string]any{
		"from_project_id": current.ProjectID, "from_key": current.Key(), "to_project_id": target.ID,
		"labels_created": createdLabels, "children_detached": detached,
	}
	if current.ParentIssueID != "" && !moving[current.ParentIssueID] {
		payload["parent_cleared"] = current.ParentIssueID
	}
	if droppedMilestone != "" {
		payload["milestone_cleared"] = droppedMilestone
	}
	return payload, nil
}

// detachChildren clears the parent link of the issue's children that stay behind, recording an
// update for each, and returns their IDs. Children that move along keep their link.
func detachChildren(w *writeTx, parent Issue, moving map[string]bool) ([]string, error) {
	rows, err := w.tx.QueryContext(w.ctx, `SELECT id FROM issues WHERE parent_issue_id = ? ORDER BY number`, parent.ID)
	if err != nil {
		return nil, fmt.Errorf("list child issues: %w", err)
	}
	var children []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, fmt.Errorf("read child issue: %w", err)
		}
		if !moving[id] {
			children = append(children, id)
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("list child issues: %w", err)
	}
	rows.Close()

	for _, id := range children {
		if _, err := w.exec(`UPDATE issues SET parent_issue_id = NULL, updated_at = ? WHERE id = ?`, w.timestamp(), id); err != nil {
			return nil, fmt.Errorf("detach child issue: %w", err)
		}
		change := map[string]any{"parent_issue_id": map[string]any{"from": parent.ID, "to": ""}}
		if err := w.event(parent.ProjectID, TargetIssue, id, "issue.updated", change); err != nil {
			return nil, err
		}
	}
	return children, nil
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
