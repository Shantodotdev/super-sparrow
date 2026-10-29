package store

import (
	"encoding/json"
	"testing"
)

func TestEveryChangeRecordsOneEvent(t *testing.T) {
	tm := newTeam(t)
	project := tm.project(t, "WP")
	issue := tm.issue(t, project.ID, "Evented")

	changes := map[string]func() error{
		"set status": func() error { return tm.store.SetIssueStatus(tm.ctx, tm.agent, issue.ID, IssueInProgress) },
		"comment": func() error {
			_, err := tm.store.AddComment(tm.ctx, tm.agent, NewComment{IssueID: issue.ID, Body: "Hi"})
			return err
		},
		"start round": func() error {
			_, err := tm.store.StartReviewRound(tm.ctx, tm.agent, NewReviewRound{IssueID: issue.ID, ToCommit: "abcdef1"})
			return err
		},
	}

	for name, change := range changes {
		before := tm.eventCount(t)
		if err := change(); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if after := tm.eventCount(t); after != before+1 {
			t.Errorf("%s recorded %d events, want 1", name, after-before)
		}

		event := tm.lastEvent(t)
		if event.ActorID != tm.agent.ActorID || event.ReportedModel != "claude-test-model" || event.ProjectID != project.ID {
			t.Errorf("%s event %+v", name, event)
		}
		if !json.Valid(event.Payload) {
			t.Errorf("%s payload is not JSON: %s", name, event.Payload)
		}
	}
}

func TestAFailedChangeRecordsNoEvent(t *testing.T) {
	tm := newTeam(t)
	project := tm.project(t, "WP")
	before := tm.eventCount(t)

	_, err := tm.store.CreateProject(tm.ctx, tm.member, NewProject{Key: "WP", Name: "Duplicate"})
	wantErr(t, err, ErrConflict)
	_, err = tm.store.CreateIssue(tm.ctx, tm.member, NewIssue{ProjectID: project.ID, Title: "T", Status: IssueTodo, ParentIssueID: "missing"})
	wantInvalid(t, err, "parent_issue_id")

	if after := tm.eventCount(t); after != before {
		t.Errorf("failed changes recorded %d events", after-before)
	}
	if fresh := got(tm.store.GetProject(tm.ctx, project.ID)).must(t); fresh.NextIssueNumber != 1 {
		t.Errorf("next issue number = %d, want the failed creation rolled back", fresh.NextIssueNumber)
	}
}

func TestEventsAreAppendOnly(t *testing.T) {
	tm := newTeam(t)
	tm.project(t, "WP")

	if _, err := tm.store.db.Exec(`UPDATE events SET kind = 'tampered'`); err == nil {
		t.Error("an event was updated")
	}
	if _, err := tm.store.db.Exec(`DELETE FROM events`); err == nil {
		t.Error("an event was deleted")
	}
}

func TestListEventsPagesAndFilters(t *testing.T) {
	tm := newTeam(t)
	project := tm.project(t, "WP")
	issue := tm.issue(t, project.ID, "Paged")
	for _, status := range []IssueStatus{IssueInProgress, IssueInReview, IssueDone} {
		if err := tm.store.SetIssueStatus(tm.ctx, tm.member, issue.ID, status); err != nil {
			t.Fatalf("SetIssueStatus: %v", err)
		}
	}

	history := got(tm.store.ListEvents(tm.ctx, EventFilter{TargetType: TargetIssue, TargetID: issue.ID, Limit: 2})).must(t)
	if len(history) != 2 || history[0].Kind != "issue.created" {
		t.Fatalf("first page = %+v", history)
	}

	rest := got(tm.store.ListEvents(tm.ctx, EventFilter{TargetType: TargetIssue, TargetID: issue.ID, AfterSequence: history[1].Sequence})).must(t)
	if len(rest) != 2 || rest[1].Kind != "issue.status_changed" {
		t.Errorf("second page = %+v", rest)
	}

	_, err := tm.store.ListEvents(tm.ctx, EventFilter{TargetID: issue.ID})
	wantInvalid(t, err, "target_type")
}

func TestReportedModelIsBounded(t *testing.T) {
	tm := newTeam(t)
	long := Caller{ActorID: tm.agent.ActorID, ReportedModel: string(make([]byte, maxReportedModel+1))}

	_, err := tm.store.CreateLabel(tm.ctx, long, "missing", "Bug")
	wantInvalid(t, err, "reported_model")
}
