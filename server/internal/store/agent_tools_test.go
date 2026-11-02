package store

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestAgentToolRollbackAndActorScopedReceipts(t *testing.T) {
	tm := newTeam(t)
	project := got(tm.store.CreateProject(tm.ctx, tm.member, NewProject{Key: "WP", Name: "Work"})).must(t)
	fn := func(scoped *Store) (any, error) {
		issue, err := scoped.CreateIssue(tm.ctx, tm.agent, NewIssue{ProjectID: project.ID, Title: "Atomic", Status: IssueTodo})
		if err != nil {
			return nil, err
		}
		return map[string]any{"key": issue.Key()}, nil
	}
	sentinel := errors.New("fail after write")
	_, err := tm.store.RunAgentTool(tm.ctx, tm.agent, "retry", "hash", func(scoped *Store) (any, error) {
		if _, err := fn(scoped); err != nil {
			return nil, err
		}
		return nil, sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("rollback returned %v", err)
	}
	if issues := got(tm.store.ListIssues(tm.ctx, IssueFilter{ProjectID: project.ID, Limit: 100})).must(t); len(issues) != 0 {
		t.Fatalf("rolled back tool left %d issues", len(issues))
	}
	first := got(tm.store.RunAgentTool(tm.ctx, tm.agent, "retry", "hash", fn)).must(t)
	// A fresh Store reads the durable receipt rather than executing the callback again.
	restarted := &Store{db: tm.store.db, now: tm.clock.Now, changed: make(chan struct{})}
	second := got(restarted.RunAgentTool(tm.ctx, tm.agent, "retry", "hash", func(*Store) (any, error) { t.Fatal("receipt callback ran again"); return nil, nil })).must(t)
	if string(first) != string(second) {
		t.Fatalf("receipt changed: %s %s", first, second)
	}
	var output map[string]any
	if err := json.Unmarshal(first, &output); err != nil {
		t.Fatal(err)
	}
	if output["key"] != "WP-1" {
		t.Fatalf("rollback consumed number: %s", first)
	}
	other := got(tm.store.CreateAgent(tm.ctx, tm.member, NewAgent{AgentName: "codex", DisplayName: "Codex"})).must(t)
	by := Caller{ActorID: other.ID}
	got(tm.store.RunAgentTool(tm.ctx, by, "retry", "different-hash", func(scoped *Store) (any, error) {
		issue, err := scoped.CreateIssue(tm.ctx, by, NewIssue{ProjectID: project.ID, Title: "Other agent", Status: IssueTodo})
		return map[string]any{"key": issue.Key()}, err
	})).must(t)
	if issues := got(tm.store.ListIssues(tm.ctx, IssueFilter{ProjectID: project.ID, Limit: 100})).must(t); len(issues) != 2 {
		t.Fatalf("actor-scoped retry produced %d issues", len(issues))
	}
	events := got(tm.store.ListEvents(tm.ctx, EventFilter{ActorID: tm.agent.ActorID, ProjectEventsOnly: true, Limit: 100})).must(t)
	if len(events) != 1 || events[0].Kind != "issue.created" {
		t.Fatalf("rollback or replay created extra events: %+v", events)
	}
	if _, err := tm.store.RunAgentTool(tm.ctx, tm.member, "human", "hash", fn); !errors.Is(err, ErrForbidden) {
		t.Fatalf("human tool write %v", err)
	}
}
