package store

import (
	"context"
	"fmt"
	"time"
)

// DiscordIssueChange is an issue creation or status transition in a daily report.
type DiscordIssueChange struct {
	Key    string
	Title  string
	Status IssueStatus
	New    bool
}

// DiscordProjectStatus returns current issue counts and the day's most recent issue changes.
func (s *Store) DiscordProjectStatus(ctx context.Context, projectID string, start, end time.Time) (map[IssueStatus]int, []DiscordIssueChange, error) {
	counts := make(map[IssueStatus]int)
	rows, err := s.db.QueryContext(ctx, `SELECT status, COUNT(*) FROM issues WHERE project_id = ? GROUP BY status`, projectID)
	if err != nil {
		return nil, nil, fmt.Errorf("count project issues by status: %w", err)
	}
	for rows.Next() {
		var status IssueStatus
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			rows.Close()
			return nil, nil, fmt.Errorf("read issue status count: %w", err)
		}
		counts[status] = count
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("read issue status counts: %w", err)
	}

	rows, err = s.db.QueryContext(ctx, `SELECT p.key || '-' || i.number, i.title, e.kind,
		COALESCE(json_extract(e.payload, '$.to'), json_extract(e.payload, '$.status'), i.status)
		FROM events e JOIN issues i ON i.id = e.target_id AND i.project_id = e.project_id
		JOIN projects p ON p.id = e.project_id
		WHERE e.project_id = ? AND e.target_type = 'issue' AND e.kind IN ('issue.created', 'issue.status_changed')
		AND e.created_at >= ? AND e.created_at < ? ORDER BY e.sequence DESC LIMIT 11`,
		projectID, start.UTC().Format(timeLayout), end.UTC().Format(timeLayout))
	if err != nil {
		return nil, nil, fmt.Errorf("load today's issue changes: %w", err)
	}
	defer rows.Close()
	var changes []DiscordIssueChange
	for rows.Next() {
		var change DiscordIssueChange
		var kind string
		if err := rows.Scan(&change.Key, &change.Title, &kind, &change.Status); err != nil {
			return nil, nil, fmt.Errorf("read issue change: %w", err)
		}
		change.New = kind == "issue.created"
		changes = append(changes, change)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("read issue changes: %w", err)
	}
	return counts, changes, nil
}
