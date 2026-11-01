package store

import "testing"

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
