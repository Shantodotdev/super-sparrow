// Package store reads and writes Super Sparrow's data in SQLite.
//
// Every change runs in one transaction that also loads the caller fresh, checks that they are
// still active, and appends the change's event, so a change and its event are saved together or
// not at all.
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

const timeLayout = "2006-01-02T15:04:05.000Z"

// Store is the data access layer. It is safe for concurrent use.
type Store struct {
	db  *sql.DB
	now func() time.Time

	changeMu sync.Mutex
	changed  chan struct{}
}

// New returns a Store over db, which must already be migrated. now supplies the current time;
// pass time.Now outside tests.
func New(db *sql.DB, now func() time.Time) *Store {
	return &Store{db: db, now: now, changed: make(chan struct{})}
}

// Changed returns a channel that is closed after the next committed change. Live views take it
// before reading new events, so a change committed in between still wakes them.
func (s *Store) Changed() <-chan struct{} {
	s.changeMu.Lock()
	defer s.changeMu.Unlock()
	return s.changed
}

// notifyChanged wakes everyone waiting on Changed and starts a new wait.
func (s *Store) notifyChanged() {
	s.changeMu.Lock()
	defer s.changeMu.Unlock()
	close(s.changed)
	s.changed = make(chan struct{})
}

// querier is the part of *sql.DB and *sql.Tx that reads use, so one read helper serves both.
type querier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// writeTx is the transaction a change runs in, with the caller it runs as.
type writeTx struct {
	tx     *sql.Tx
	ctx    context.Context
	caller Actor
	model  string
	now    time.Time
}

// write runs fn in a transaction as the caller and commits when fn succeeds.
func (s *Store) write(ctx context.Context, by Caller, fn func(w *writeTx) error) error {
	model, err := optionalText("reported_model", by.ReportedModel, maxReportedModel)
	if err != nil {
		return err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Load the caller inside the transaction, so a person disabled a moment ago cannot write.
	caller, err := getActor(ctx, tx, by.ActorID)
	if errors.Is(err, ErrNotFound) {
		return forbidden("unknown caller")
	}
	if err != nil {
		return err
	}
	if !caller.Active() {
		return forbidden("caller is disabled")
	}

	w := &writeTx{tx: tx, ctx: ctx, caller: caller, model: model, now: s.now().UTC().Truncate(time.Millisecond)}
	if err := fn(w); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	s.notifyChanged()
	return nil
}

// transact runs fn in a transaction that no caller owns, such as signing in, and commits when fn
// succeeds. It passes fn the current time.
func (s *Store) transact(ctx context.Context, fn func(tx *sql.Tx, now time.Time) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	if err := fn(tx, s.now().UTC().Truncate(time.Millisecond)); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	s.notifyChanged()
	return nil
}

func (w *writeTx) exec(query string, args ...any) (sql.Result, error) {
	return w.tx.ExecContext(w.ctx, query, args...)
}

func (w *writeTx) timestamp() string { return w.now.Format(timeLayout) }

// event appends one event for the change being made. payload is encoded as JSON.
func (w *writeTx) event(projectID string, target TargetType, targetID, kind string, payload any) error {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode %s event: %w", kind, err)
	}

	_, err = w.exec(`
		INSERT INTO events (id, project_id, target_type, target_id, kind, actor_id, reported_model, payload, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		newID(), nullString(projectID), target, targetID, kind, w.caller.ID,
		nullString(w.model), string(encoded), w.timestamp())
	if err != nil {
		return fmt.Errorf("append %s event: %w", kind, err)
	}

	return nil
}

// requireHuman allows only people to make the change.
func (w *writeTx) requireHuman(action string) error {
	if w.caller.Kind != ActorHuman {
		return forbidden("only a person can " + action)
	}
	return nil
}

// requireOwner allows only owners and the system to make the change.
func (w *writeTx) requireOwner(action string) error {
	if w.caller.Kind == ActorSystem || w.caller.IsOwner() {
		return nil
	}
	return forbidden("only an owner can " + action)
}

func newID() string { return uuid.Must(uuid.NewV7()).String() }

func nullString(value string) sql.NullString {
	return sql.NullString{String: value, Valid: value != ""}
}

func nullInt(value int) sql.NullInt64 {
	return sql.NullInt64{Int64: int64(value), Valid: value != 0}
}

func parseTime(value string) (time.Time, error) {
	parsed, err := time.Parse(timeLayout, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse stored time %q: %w", value, err)
	}
	return parsed, nil
}

func parseOptionalTime(value sql.NullString) (*time.Time, error) {
	if !value.Valid {
		return nil, nil
	}

	parsed, err := parseTime(value.String)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

// timestamps parses a record's created_at and updated_at.
func timestamps(created, updated string) (time.Time, time.Time, error) {
	createdAt, err := parseTime(created)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}

	updatedAt, err := parseTime(updated)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}

	return createdAt, updatedAt, nil
}
