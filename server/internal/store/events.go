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
	ProjectID     string
	TargetType    TargetType
	TargetID      string
	ActorID       string
	AfterSequence int64
	Limit         int
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
