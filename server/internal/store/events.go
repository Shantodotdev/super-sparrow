package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// EventFilter selects events, oldest first. Pass the last Sequence of one page as
// AfterSequence to get the next, which is also how a live view catches up after reconnecting.
type EventFilter struct {
	ProjectID  string
	TargetType TargetType
	TargetID   string
	// IssueID selects an issue's history: the issue's own events and those of its review rounds,
	// findings, and comments.
	IssueID string
	ActorID string
	// ProjectEventsOnly leaves out events about people, agents, sessions, and tokens, which carry
	// email addresses and device names that agents have no use for.
	ProjectEventsOnly bool
	AfterSequence     int64
	Limit             int
}

// ListEvents returns a page of events matching the filter.
func (s *Store) ListEvents(ctx context.Context, filter EventFilter) ([]Event, error) {
	limit, err := pageLimit(filter.Limit)
	if err != nil {
		return nil, err
	}
	if filter.TargetID != "" && filter.TargetType == "" {
		return nil, invalid("target_type", "is required with target_id")
	}

	where := []string{`sequence > ?`}
	args := []any{filter.AfterSequence}
	for _, condition := range []struct {
		column string
		value  string
	}{
		{"project_id", filter.ProjectID},
		{"target_type", string(filter.TargetType)},
		{"target_id", filter.TargetID},
		{"actor_id", filter.ActorID},
	} {
		if condition.value != "" {
			where = append(where, condition.column+` = ?`)
			args = append(args, condition.value)
		}
	}
	if filter.IssueID != "" {
		where = append(where, `((target_type = 'issue' AND target_id = ?) OR json_extract(payload, '$.issue_id') = ?)`)
		args = append(args, filter.IssueID, filter.IssueID)
	}
	if filter.ProjectEventsOnly {
		where = append(where, `project_id IS NOT NULL`)
	}
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, `
		SELECT sequence, id, project_id, target_type, target_id, kind, actor_id, reported_model, payload, created_at
		FROM events WHERE `+strings.Join(where, " AND ")+` ORDER BY sequence LIMIT ?`, args...)
	if err != nil {
		return nil, fmt.Errorf("list events: %w", err)
	}
	defer rows.Close()

	var events []Event
	for rows.Next() {
		var event Event
		var projectID, model sql.NullString
		var payload, created string

		err := rows.Scan(&event.Sequence, &event.ID, &projectID, &event.TargetType, &event.TargetID, &event.Kind,
			&event.ActorID, &model, &payload, &created)
		if err != nil {
			return nil, fmt.Errorf("read event: %w", err)
		}

		event.ProjectID, event.ReportedModel, event.Payload = projectID.String, model.String, []byte(payload)
		if event.CreatedAt, err = parseTime(created); err != nil {
			return nil, err
		}
		events = append(events, event)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list events: %w", err)
	}
	return events, nil
}

// LatestEventSequence returns the sequence number of the newest event, or 0 when there are none.
// A live view that starts from it receives only changes made after it connected.
func (s *Store) LatestEventSequence(ctx context.Context) (int64, error) {
	var sequence int64
	if err := s.db.QueryRowContext(ctx, `SELECT coalesce(max(sequence), 0) FROM events`).Scan(&sequence); err != nil {
		return 0, fmt.Errorf("find latest event: %w", err)
	}
	return sequence, nil
}
