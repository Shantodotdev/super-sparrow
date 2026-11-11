package store

import (
	"context"
	"fmt"
	"strings"
)

// ListAssignedIssues returns the issues assigned to any of the given actors across all projects,
// most recently updated first, a page at a time. projectID and statuses narrow the result when
// given. Offset pages reflect current data.
func (s *Store) ListAssignedIssues(ctx context.Context, assigneeIDs []string, projectID string, statuses []IssueStatus, limit, offset int) ([]Issue, error) {
	if len(assigneeIDs) == 0 {
		return nil, invalid("assignee", "is required")
	}
	if limit < 1 || limit > 101 || offset < 0 {
		return nil, invalid("page", "is invalid")
	}

	where := []string{`i.assignee_actor_id IN (` + queryPlaceholders(len(assigneeIDs)) + `)`}
	args := make([]any, 0, len(assigneeIDs)+len(statuses)+3)
	for _, id := range assigneeIDs {
		args = append(args, id)
	}
	if projectID != "" {
		where = append(where, `i.project_id = ?`)
		args = append(args, projectID)
	}
	if len(statuses) > 0 {
		for _, status := range statuses {
			if !status.valid() {
				return nil, invalid("statuses", "contains an unknown status %q", status)
			}
			args = append(args, status)
		}
		where = append(where, `i.status IN (`+queryPlaceholders(len(statuses))+`)`)
	}
	args = append(args, limit, offset)

	rows, err := s.db.QueryContext(ctx,
		`SELECT `+issueColumns+issueFrom+`WHERE `+strings.Join(where, " AND ")+` ORDER BY i.updated_at DESC, i.id LIMIT ? OFFSET ?`, args...)
	if err != nil {
		return nil, fmt.Errorf("list assigned issues: %w", err)
	}
	defer rows.Close()

	issues := []Issue{}
	for rows.Next() {
		issue, err := scanIssue(rows)
		if err != nil {
			return nil, err
		}
		issues = append(issues, issue)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list assigned issues: %w", err)
	}
	return issues, nil
}

// queryPlaceholders returns n comma-separated SQL placeholders.
func queryPlaceholders(n int) string {
	return strings.TrimSuffix(strings.Repeat("?, ", n), ", ")
}
