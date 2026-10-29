package store

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/shantodotdev/super-sparrow/server/internal/database"
	"github.com/shantodotdev/super-sparrow/server/migrations"
)

// testClock returns a time that moves forward one second on every call, so records made one
// after another have distinct, ordered timestamps.
type testClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *testClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.now = c.now.Add(time.Second)
	return c.now
}

// team is a test database with an owner, a member, and an agent working for the member.
type team struct {
	store  *Store
	ctx    context.Context
	system Caller
	owner  Caller
	member Caller
	agent  Caller
}

func newTeam(t *testing.T) *team {
	t.Helper()
	ctx := context.Background()

	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	if err := database.Migrate(ctx, db, migrations.Files); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	clock := &testClock{now: time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)}
	tm := &team{store: New(db, clock.Now), ctx: ctx, system: Caller{ActorID: SystemActorID}}

	owner := got(tm.store.CreateHuman(ctx, tm.system, NewHuman{DisplayName: "Olive Owner", Email: "olive@example.com", Role: RoleOwner})).must(t)
	member := got(tm.store.CreateHuman(ctx, tm.system, NewHuman{DisplayName: "Max Member", Email: "max@example.com", Role: RoleMember})).must(t)
	tm.owner, tm.member = Caller{ActorID: owner.ID}, Caller{ActorID: member.ID}

	agent := got(tm.store.CreateAgent(ctx, tm.member, NewAgent{DisplayName: "Claude Code (Max)", AgentName: "claude-code"})).must(t)
	tm.agent = Caller{ActorID: agent.ID, ReportedModel: "claude-test-model"}

	return tm
}

// result holds a value and an error, so got(call()).must(t) can check a two-value call inline.
type result[T any] struct {
	value T
	err   error
}

func got[T any](value T, err error) result[T] { return result[T]{value: value, err: err} }

// must fails the test on an error and otherwise returns the value.
func (r result[T]) must(t *testing.T) T {
	t.Helper()
	if r.err != nil {
		t.Fatalf("unexpected error: %v", r.err)
	}
	return r.value
}

func wantErr(t *testing.T, err, target error) {
	t.Helper()
	if !errors.Is(err, target) {
		t.Fatalf("error = %v, want %v", err, target)
	}
}

func wantInvalid(t *testing.T, err error, field string) {
	t.Helper()
	var validation *ValidationError
	if !errors.As(err, &validation) || validation.Field != field {
		t.Fatalf("error = %v, want a validation error on %s", err, field)
	}
}

func (tm *team) project(t *testing.T, key string) Project {
	t.Helper()
	return got(tm.store.CreateProject(tm.ctx, tm.member, NewProject{Key: key, Name: key + " project"})).must(t)
}

func (tm *team) issue(t *testing.T, projectID, title string) Issue {
	t.Helper()
	return got(tm.store.CreateIssue(tm.ctx, tm.member, NewIssue{ProjectID: projectID, Title: title, Status: IssueTodo})).must(t)
}

func (tm *team) eventCount(t *testing.T) int {
	t.Helper()
	var count int
	if err := tm.store.db.QueryRow(`SELECT count(*) FROM events`).Scan(&count); err != nil {
		t.Fatalf("count events: %v", err)
	}
	return count
}

func (tm *team) lastEvent(t *testing.T) Event {
	t.Helper()
	var last int64
	if err := tm.store.db.QueryRow(`SELECT coalesce(max(sequence), 0) FROM events`).Scan(&last); err != nil {
		t.Fatalf("find last event: %v", err)
	}

	events := got(tm.store.ListEvents(tm.ctx, EventFilter{AfterSequence: last - 1, Limit: 1})).must(t)
	if len(events) != 1 {
		t.Fatal("no events recorded")
	}
	return events[0]
}
