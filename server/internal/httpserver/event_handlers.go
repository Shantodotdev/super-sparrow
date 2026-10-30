package httpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/shantodotdev/super-sparrow/server/internal/store"
)

const (
	streamBatchSize   = 200
	streamHeartbeat   = 25 * time.Second
	streamRetryMillis = 3000
)

// listEvents returns a page of events, oldest first. It filters by ?project (a key), ?target_type
// with ?target_id, and ?actor, and pages with ?after and ?limit; next_after fetches the next page.
func (s *server) listEvents(w http.ResponseWriter, r *http.Request) error {
	filter, err := s.eventFilter(r)
	if err != nil {
		return err
	}
	filter.TargetType = store.TargetType(r.URL.Query().Get("target_type"))
	filter.TargetID = r.URL.Query().Get("target_id")
	if filter.ActorID = r.URL.Query().Get("actor"); filter.ActorID != "" {
		if _, err := s.store.GetActor(r.Context(), filter.ActorID); err != nil {
			return err
		}
	}
	return s.writeEvents(w, r, filter)
}

// listIssueEvents returns an issue's history: its own events and those of its rounds, findings,
// and comments.
func (s *server) listIssueEvents(w http.ResponseWriter, r *http.Request) error {
	issue, err := s.issueFromPath(r)
	if err != nil {
		return err
	}

	filter := store.EventFilter{IssueID: issue.ID}
	if filter.AfterSequence, filter.Limit, err = eventPage(r); err != nil {
		return err
	}
	return s.writeEvents(w, r, filter)
}

func (s *server) writeEvents(w http.ResponseWriter, r *http.Request, filter store.EventFilter) error {
	events, err := s.store.ListEvents(r.Context(), filter)
	if err != nil {
		return err
	}

	views := make([]eventView, 0, len(events))
	for _, event := range events {
		views = append(views, newEventView(event))
	}
	nextAfter := filter.AfterSequence
	if len(events) > 0 {
		nextAfter = events[len(events)-1].Sequence
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": views, "next_after": nextAfter})
	return nil
}

// eventFilter reads the filters the event list and the stream share: ?project, ?after, and
// ?limit. Agents never see events about people, sessions, and tokens.
func (s *server) eventFilter(r *http.Request) (store.EventFilter, error) {
	filter := store.EventFilter{ProjectEventsOnly: identityFrom(r.Context()).actor.Kind == store.ActorAgent}
	if key := r.URL.Query().Get("project"); key != "" {
		project, err := s.store.GetProjectByKey(r.Context(), key)
		if err != nil {
			return filter, err
		}
		filter.ProjectID = project.ID
	}

	var err error
	filter.AfterSequence, filter.Limit, err = eventPage(r)
	return filter, err
}

func eventPage(r *http.Request) (int64, int, error) {
	after, err := queryInt(r, "after")
	if err != nil {
		return 0, 0, err
	}
	limit, err := queryLimit(r)
	return int64(after), limit, err
}

// streamEvents sends new events as Server-Sent Events, each with its sequence as the event ID.
// A client that reconnects sends Last-Event-ID, or ?after, and receives everything it missed;
// one that sends neither receives only events after it connected. It filters by ?project.
func (s *server) streamEvents(w http.ResponseWriter, r *http.Request) error {
	filter, err := s.eventFilter(r)
	if err != nil {
		return err
	}
	filter.Limit = streamBatchSize

	switch lastID := r.Header.Get("Last-Event-ID"); {
	case lastID != "":
		if filter.AfterSequence, err = strconv.ParseInt(lastID, 10, 64); err != nil || filter.AfterSequence < 0 {
			return &store.ValidationError{Field: "Last-Event-ID", Message: "must be an event sequence number"}
		}
	case r.URL.Query().Get("after") == "":
		if filter.AfterSequence, err = s.store.LatestEventSequence(r.Context()); err != nil {
			return err
		}
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	// Errors after this point cannot change the response, so they end the stream; the client
	// reconnects and catches up from its last event ID.
	controller := http.NewResponseController(w)
	if _, err := fmt.Fprintf(w, "retry: %d\n\n", streamRetryMillis); err != nil {
		return nil
	}
	heartbeat := time.NewTicker(streamHeartbeat)
	defer heartbeat.Stop()

	id := identityFrom(r.Context())
	for {
		// Take the change signal before reading, so a change committed during the read still wakes us.
		changed := s.store.Changed()

		// A person removed or signed out, or a token revoked, while the stream is open must stop
		// receiving changes. The client's reconnect then fails to authenticate.
		if current, err := s.stillCurrent(r.Context(), id); err != nil || !current {
			return nil
		}
		if err := s.sendNewEvents(w, r, &filter); err != nil {
			return nil
		}
		if err := controller.Flush(); err != nil {
			return nil
		}

		select {
		case <-changed:
		case <-heartbeat.C:
			if _, err := fmt.Fprint(w, ": heartbeat\n\n"); err != nil {
				return nil
			}
		case <-r.Context().Done():
			return nil
		case <-s.stop:
			return nil
		}
	}
}

// stillCurrent reports whether the session or token a request was made with still works.
func (s *server) stillCurrent(ctx context.Context, id identity) (bool, error) {
	if id.tokenID != "" {
		return s.store.TokenCurrent(ctx, id.tokenID)
	}
	return s.store.SessionCurrent(ctx, id.sessionID)
}

// sendNewEvents writes every event after filter.AfterSequence and moves it past them.
func (s *server) sendNewEvents(w http.ResponseWriter, r *http.Request, filter *store.EventFilter) error {
	for {
		events, err := s.store.ListEvents(r.Context(), *filter)
		if err != nil {
			s.logger.ErrorContext(r.Context(), "event stream read failed", "error", err)
			return err
		}

		for _, event := range events {
			data, err := json.Marshal(newEventView(event))
			if err != nil {
				return fmt.Errorf("encode event %d: %w", event.Sequence, err)
			}
			if _, err := fmt.Fprintf(w, "id: %d\nevent: change\ndata: %s\n\n", event.Sequence, data); err != nil {
				return err
			}
			filter.AfterSequence = event.Sequence
		}

		if len(events) < filter.Limit {
			return nil
		}
	}
}
