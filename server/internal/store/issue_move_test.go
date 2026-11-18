package store

import (
	"errors"
	"slices"
	"testing"
)

func TestMoveIssueTakesEverythingToTheTargetProject(t *testing.T) {
	tm := newTeam(t)
	from, to := tm.project(t, "AA"), tm.project(t, "BB")
	tm.issue(t, to.ID, "Already in BB")

	bug := got(tm.store.CreateLabelWithColor(tm.ctx, tm.member, from.ID, "Bug", "#ff6b6b")).must(t)
	shared := got(tm.store.CreateLabel(tm.ctx, tm.member, from.ID, "Shared")).must(t)
	targetShared := got(tm.store.CreateLabel(tm.ctx, tm.member, to.ID, "Shared")).must(t)
	sprint := got(tm.store.CreateMilestone(tm.ctx, tm.member, NewMilestone{ProjectID: from.ID, Name: "Sprint 1"})).must(t)
	got(tm.store.CreateMilestone(tm.ctx, tm.member, NewMilestone{ProjectID: to.ID, Name: "Sprint 1"})).must(t)

	issue := got(tm.store.CreateIssue(tm.ctx, tm.member, NewIssue{
		ProjectID: from.ID, Title: "Move me", Status: IssueTodo, MilestoneID: sprint.ID,
		BranchName: "feat/move-me", LabelIDs: []string{bug.ID, shared.ID},
	})).must(t)
	child := got(tm.store.CreateIssue(tm.ctx, tm.member, NewIssue{ProjectID: from.ID, Title: "Child", Status: IssueTodo, ParentIssueID: issue.ID})).must(t)
	subissue := got(tm.store.CreateSubissue(tm.ctx, tm.agent, issue.ID, NewSubissue{Title: "Sub", Status: IssueTodo, LabelIDs: []string{bug.ID}})).must(t)
	attachment := tm.beginAndUpload(t, tm.member, issue.Key(), "shot.png", []byte("png"))
	got(tm.store.CompleteAttachment(tm.ctx, tm.member, attachment.ID)).must(t)

	moved, err := tm.store.MoveIssue(tm.ctx, tm.agent, issue.ID, to.ID)
	if err != nil {
		t.Fatalf("MoveIssue: %v", err)
	}

	if moved.ID != issue.ID || moved.Key() != "BB-2" || moved.ProjectID != to.ID {
		t.Fatalf("moved issue %s in %s", moved.Key(), moved.ProjectKey)
	}
	if moved.BranchName != "feat/move-me" || moved.Status != IssueTodo {
		t.Errorf("branch %q status %s", moved.BranchName, moved.Status)
	}

	// The shared label maps to the target's own; Bug is created there with its color.
	var targetBug Label
	for _, label := range got(tm.store.ListLabels(tm.ctx, to.ID)).must(t) {
		if label.Name == "Bug" {
			targetBug = label
		}
	}
	if targetBug.ID == "" || targetBug.Color != "#ff6b6b" {
		t.Fatalf("Bug in target: %+v", targetBug)
	}
	want := []string{targetBug.ID, targetShared.ID}
	slices.Sort(want)
	if !slices.Equal(moved.LabelIDs, want) {
		t.Errorf("labels = %v, want %v", moved.LabelIDs, want)
	}

	// The milestone maps by name, and the parent link and the child's link are cleared.
	if moved.MilestoneID == "" || moved.MilestoneID == sprint.ID {
		t.Errorf("milestone = %q, want the target's Sprint 1", moved.MilestoneID)
	}
	if got := got(tm.store.GetIssue(tm.ctx, child.ID)).must(t); got.ParentIssueID != "" || got.ProjectID != from.ID {
		t.Errorf("child = %+v, want left behind without a parent", got)
	}

	subissues := got(tm.store.ListSubissues(tm.ctx, issue.ID)).must(t)
	if len(subissues) != 1 || subissues[0].ID != subissue.ID || !slices.Equal(subissues[0].LabelIDs, []string{targetBug.ID}) {
		t.Errorf("subissues = %+v", subissues)
	}
	if attachments := got(tm.store.ListIssueAttachments(tm.ctx, moved.Key())).must(t); len(attachments) != 1 || attachments[0].ProjectKey != "BB" {
		t.Errorf("attachments = %+v", attachments)
	}
	if _, err := tm.store.GetIssueByKey(tm.ctx, "AA-1"); !errors.Is(err, ErrNotFound) {
		t.Errorf("old key error = %v, want not found", err)
	}

	event := tm.lastEvent(t)
	if event.Kind != "issue.moved" || event.ProjectID != to.ID || event.TargetID != issue.ID || event.ActorID != tm.agent.ActorID {
		t.Errorf("event %s in %s by %s", event.Kind, event.ProjectID, event.ActorID)
	}
}

func TestMoveIssueRefusals(t *testing.T) {
	tm := newTeam(t)
	from, to := tm.project(t, "AA"), tm.project(t, "BB")
	issue := got(tm.store.CreateIssue(tm.ctx, tm.member, NewIssue{ProjectID: from.ID, Title: "Branchy", Status: IssueTodo, BranchName: "feat/x"})).must(t)

	_, err := tm.store.MoveIssue(tm.ctx, tm.member, issue.ID, from.ID)
	wantInvalid(t, err, "project_id")
	_, err = tm.store.MoveIssue(tm.ctx, tm.member, issue.ID, "missing")
	wantInvalid(t, err, "project_id")
	if _, err = tm.store.MoveIssue(tm.ctx, tm.member, "missing", to.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing issue error = %v", err)
	}

	got(tm.store.CreateIssue(tm.ctx, tm.member, NewIssue{ProjectID: to.ID, Title: "Holder", Status: IssueTodo, BranchName: "feat/x"})).must(t)
	before := tm.eventCount(t)
	if _, err = tm.store.MoveIssue(tm.ctx, tm.member, issue.ID, to.ID); !errors.Is(err, ErrConflict) {
		t.Errorf("branch clash error = %v, want a conflict", err)
	}
	if stayed := got(tm.store.GetIssue(tm.ctx, issue.ID)).must(t); stayed.ProjectID != from.ID || tm.eventCount(t) != before {
		t.Errorf("a refused move changed %s or wrote events", stayed.Key())
	}

	// Numbers are not reused: moving back takes a fresh one.
	got(tm.store.UpdateIssue(tm.ctx, tm.member, issue.ID, IssueUpdate{BranchName: Change("")})).must(t)
	moved := got(tm.store.MoveIssue(tm.ctx, tm.member, issue.ID, to.ID)).must(t)
	back := got(tm.store.MoveIssue(tm.ctx, tm.member, moved.ID, from.ID)).must(t)
	if moved.Key() != "BB-2" || back.Key() != "AA-2" {
		t.Errorf("keys %s then %s, want BB-2 then AA-2", moved.Key(), back.Key())
	}
}
