package store

import (
	"testing"
)

func TestCreateProjectNormalizesTheKey(t *testing.T) {
	tm := newTeam(t)

	project := got(tm.store.CreateProject(tm.ctx, tm.member, NewProject{Key: " wp ", Name: "WP Agent"})).must(t)
	if project.Key != "WP" || project.NextIssueNumber != 1 {
		t.Errorf("created %+v", project)
	}

	_, err := tm.store.CreateProject(tm.ctx, tm.member, NewProject{Key: "wp", Name: "Again"})
	wantErr(t, err, ErrConflict)

	for _, key := range []string{"W", "1WP", "WP-1", "TOOLONGKEY1"} {
		_, err := tm.store.CreateProject(tm.ctx, tm.member, NewProject{Key: key, Name: "Bad"})
		wantInvalid(t, err, "key")
	}

	found := got(tm.store.GetProjectByKey(tm.ctx, "wp")).must(t)
	if found.ID != project.ID {
		t.Errorf("lookup by key found %s, want %s", found.ID, project.ID)
	}
}

func TestAgentsCannotCreateOrDeleteProjects(t *testing.T) {
	tm := newTeam(t)

	_, err := tm.store.CreateProject(tm.ctx, tm.agent, NewProject{Key: "AG", Name: "Agent project"})
	wantErr(t, err, ErrForbidden)

	project := tm.project(t, "WP")
	wantErr(t, tm.store.DeleteProject(tm.ctx, tm.agent, project.ID), ErrForbidden)
	wantErr(t, tm.store.DeleteProject(tm.ctx, tm.member, project.ID), ErrForbidden)
}

func TestDeleteProjectRemovesItsWorkAndKeepsHistory(t *testing.T) {
	tm := newTeam(t)
	project := tm.project(t, "WP")
	parent := tm.issue(t, project.ID, "Parent")
	child := got(tm.store.CreateIssue(tm.ctx, tm.member, NewIssue{ProjectID: project.ID, Title: "Child", Status: IssueTodo, ParentIssueID: parent.ID})).must(t)
	round := got(tm.store.StartReviewRound(tm.ctx, tm.agent, NewReviewRound{IssueID: child.ID, ToCommit: "abcdef1"})).must(t)
	finding := got(tm.store.AddFinding(tm.ctx, tm.agent, NewFinding{ReviewRoundID: round.ID, Severity: SeverityNit, Title: "T", Problem: "P", Fix: "F"})).must(t)
	got(tm.store.ChangeFindingStatus(tm.ctx, tm.member, finding.ID, FindingChange{Status: FindingLater})).must(t)
	got(tm.store.AddComment(tm.ctx, tm.member, NewComment{FindingID: finding.ID, Body: "Later is fine"})).must(t)

	if err := tm.store.DeleteProject(tm.ctx, tm.owner, project.ID); err != nil {
		t.Fatalf("DeleteProject: %v", err)
	}

	for _, table := range []string{"issues", "review_rounds", "findings", "comments"} {
		var count int
		tm.store.db.QueryRow(`SELECT count(*) FROM ` + table).Scan(&count)
		if count != 0 {
			t.Errorf("%d rows left in %s", count, table)
		}
	}

	events := got(tm.store.ListEvents(tm.ctx, EventFilter{ProjectID: project.ID, Limit: maximumPageLimit})).must(t)
	if len(events) < 6 || events[len(events)-1].Kind != "project.deleted" {
		t.Errorf("project history has %d events, ending with %q", len(events), events[len(events)-1].Kind)
	}
}

func TestRenameProjectRecordsTheChange(t *testing.T) {
	tm := newTeam(t)
	project := tm.project(t, "WP")

	if err := tm.store.RenameProject(tm.ctx, tm.member, project.ID, "WP Agent"); err != nil {
		t.Fatalf("RenameProject: %v", err)
	}

	event := tm.lastEvent(t)
	if event.Kind != "project.renamed" || string(event.Payload) != `{"from":"WP project","to":"WP Agent"}` {
		t.Errorf("event %s %s", event.Kind, event.Payload)
	}

	before := tm.eventCount(t)
	if err := tm.store.RenameProject(tm.ctx, tm.member, project.ID, "WP Agent"); err != nil {
		t.Fatalf("RenameProject to the same name: %v", err)
	}
	if tm.eventCount(t) != before {
		t.Error("a rename to the same name recorded an event")
	}
}

func TestMilestonesTakeTheNextPosition(t *testing.T) {
	tm := newTeam(t)
	project := tm.project(t, "WP")

	first := got(tm.store.CreateMilestone(tm.ctx, tm.member, NewMilestone{ProjectID: project.ID, Name: "Phase 1", Status: MilestoneActive})).must(t)
	second := got(tm.store.CreateMilestone(tm.ctx, tm.member, NewMilestone{ProjectID: project.ID, Name: "Phase 2", Status: MilestonePlanned})).must(t)
	if first.Position != 0 || second.Position != 1 {
		t.Errorf("positions = %d, %d, want 0, 1", first.Position, second.Position)
	}

	_, err := tm.store.CreateMilestone(tm.ctx, tm.member, NewMilestone{ProjectID: project.ID, Name: "Phase 1", Status: MilestonePlanned})
	wantErr(t, err, ErrConflict)

	_, err = tm.store.CreateMilestone(tm.ctx, tm.agent, NewMilestone{ProjectID: project.ID, Name: "Phase 3", Status: MilestonePlanned})
	wantErr(t, err, ErrForbidden)

	err = tm.store.UpdateMilestone(tm.ctx, tm.member, second.ID, MilestoneUpdate{Name: Change("Phase 1")})
	wantErr(t, err, ErrConflict)

	if err := tm.store.UpdateMilestone(tm.ctx, tm.member, first.ID, MilestoneUpdate{Status: Change(MilestoneDone), Position: Change(5)}); err != nil {
		t.Fatalf("UpdateMilestone: %v", err)
	}
	milestones := got(tm.store.ListMilestones(tm.ctx, project.ID)).must(t)
	if milestones[0].ID != second.ID || milestones[1].Status != MilestoneDone {
		t.Errorf("milestones after update: %+v", milestones)
	}
}

func TestLabelNamesAreUniquePerProject(t *testing.T) {
	tm := newTeam(t)
	first := tm.project(t, "WP")
	second := tm.project(t, "SS")

	got(tm.store.CreateLabel(tm.ctx, tm.agent, first.ID, "Bug")).must(t)
	got(tm.store.CreateLabel(tm.ctx, tm.agent, second.ID, "Bug")).must(t)

	_, err := tm.store.CreateLabel(tm.ctx, tm.agent, first.ID, "Bug")
	wantErr(t, err, ErrConflict)

	_, err = tm.store.CreateLabel(tm.ctx, tm.agent, "missing", "Bug")
	wantErr(t, err, ErrNotFound)
}
