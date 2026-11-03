package store

import (
	"errors"
	"testing"
)

func TestSubissueWriteRollsBackWithEventFailure(t *testing.T) {
	tm := newTeam(t)
	project := tm.project(t, "WP")
	issue := tm.issue(t, project.ID, "Parent")
	_, err := tm.store.db.Exec(`CREATE TRIGGER refuse_subissue_event BEFORE INSERT ON events
		WHEN NEW.kind = 'issue.subissue_created' BEGIN SELECT RAISE(ABORT, 'test event failure'); END`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tm.store.CreateSubissue(tm.ctx, tm.agent, issue.ID, NewSubissue{Title: "Rolled back", Status: IssueTodo})
	if err == nil {
		t.Fatal("creation succeeded without its event")
	}
	if children := got(tm.store.ListSubissues(tm.ctx, issue.ID)).must(t); len(children) != 0 {
		t.Fatalf("subissue was committed: %+v", children)
	}
	if _, err := tm.store.db.Exec(`DROP TRIGGER refuse_subissue_event`); err != nil {
		t.Fatal(err)
	}
	child := got(tm.store.CreateSubissue(tm.ctx, tm.agent, issue.ID, NewSubissue{Title: "Child", Status: IssueTodo})).must(t)
	if child.Number != 1 {
		t.Fatalf("failed write consumed number: %d", child.Number)
	}
	_, err = tm.store.db.Exec(`CREATE TRIGGER refuse_subissue_update BEFORE INSERT ON events
		WHEN NEW.kind = 'issue.subissue_updated' BEGIN SELECT RAISE(ABORT, 'test event failure'); END`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tm.store.UpdateSubissue(tm.ctx, tm.agent, issue.ID, child.ID, SubissueUpdate{Status: Change(IssueDone)})
	if err == nil {
		t.Fatal("update succeeded without its event")
	}
	if children := got(tm.store.ListSubissues(tm.ctx, issue.ID)).must(t); children[0].Status != IssueTodo {
		t.Fatalf("status was committed: %+v", children)
	}
}

func TestSubissueLabelsStayInTheIssueProject(t *testing.T) {
	tm := newTeam(t)
	project := tm.project(t, "WP")
	other := tm.project(t, "SS")
	issue := tm.issue(t, project.ID, "Parent")
	security := got(tm.store.CreateLabel(tm.ctx, tm.agent, project.ID, "Security")).must(t)
	release := got(tm.store.CreateLabel(tm.ctx, tm.agent, project.ID, "Release")).must(t)
	foreign := got(tm.store.CreateLabel(tm.ctx, tm.agent, other.ID, "Foreign")).must(t)

	child := got(tm.store.CreateSubissue(tm.ctx, tm.agent, issue.ID, NewSubissue{
		Title: "Child", Status: IssueTodo, LabelIDs: []string{security.ID, security.ID},
	})).must(t)
	if len(child.LabelIDs) != 1 || child.LabelIDs[0] != security.ID {
		t.Fatalf("created labels %v", child.LabelIDs)
	}

	_, err := tm.store.UpdateSubissue(tm.ctx, tm.agent, issue.ID, child.ID, SubissueUpdate{LabelIDs: Change([]string{foreign.ID})})
	if err == nil {
		t.Fatal("a label of another project was accepted")
	}
	child = got(tm.store.UpdateSubissue(tm.ctx, tm.agent, issue.ID, child.ID, SubissueUpdate{LabelIDs: Change([]string{release.ID})})).must(t)
	listed := got(tm.store.ListSubissues(tm.ctx, issue.ID)).must(t)
	if len(listed[0].LabelIDs) != 1 || listed[0].LabelIDs[0] != release.ID || tm.lastEvent(t).Kind != "issue.subissue_updated" {
		t.Fatalf("updated labels %v", listed[0].LabelIDs)
	}
}

func TestUpdateLabelRenamesAndRecolors(t *testing.T) {
	tm := newTeam(t)
	project := tm.project(t, "WP")
	label := got(tm.store.CreateLabel(tm.ctx, tm.agent, project.ID, "Bug")).must(t)
	got(tm.store.CreateLabel(tm.ctx, tm.agent, project.ID, "Taken")).must(t)

	updated := got(tm.store.UpdateLabel(tm.ctx, tm.agent, project.ID, label.ID, LabelUpdate{
		Name: Change("Defect"), Color: Change(" #F2A9AE "),
	})).must(t)
	if updated.Name != "Defect" || updated.Color != "#f2a9ae" || tm.lastEvent(t).Kind != "label.updated" {
		t.Fatalf("updated %+v", updated)
	}
	if _, err := tm.store.UpdateLabel(tm.ctx, tm.agent, project.ID, label.ID, LabelUpdate{Name: Change("Taken")}); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate name: %v", err)
	}
	if _, err := tm.store.UpdateLabel(tm.ctx, tm.agent, project.ID, label.ID, LabelUpdate{Color: Change("red")}); err == nil {
		t.Fatal("an invalid color was accepted")
	}
}
