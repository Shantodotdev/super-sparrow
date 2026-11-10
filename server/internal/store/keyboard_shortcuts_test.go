package store

import "testing"

func TestKeyboardShortcutsArePersonalAndAudited(t *testing.T) {
	tm := newTeam(t)
	initial := got(tm.store.GetKeyboardShortcuts(tm.ctx, tm.member)).must(t)
	if initial.Revision != 0 || len(initial.Overrides) != 0 {
		t.Fatalf("initial %+v", initial)
	}
	before := tm.eventCount(t)
	binding := "Ctrl+Alt+K"
	saved := got(tm.store.UpdateKeyboardShortcut(tm.ctx, tm.member, 0, "quickCapture", &binding)).must(t)
	reloaded := got(tm.store.GetKeyboardShortcuts(tm.ctx, tm.member)).must(t)
	owner := got(tm.store.GetKeyboardShortcuts(tm.ctx, tm.owner)).must(t)
	if saved.Revision != 1 || reloaded.Overrides["quickCapture"] != binding || len(owner.Overrides) != 0 {
		t.Fatalf("saved %+v, owner %+v", reloaded, owner)
	}
	event := tm.lastEvent(t)
	if tm.eventCount(t) != before+1 || event.ActorID != tm.member.ActorID || event.TargetID != tm.member.ActorID || event.Kind != "actor.shortcuts_updated" {
		t.Fatalf("event %+v", event)
	}
	disabled := ""
	got(tm.store.UpdateKeyboardShortcut(tm.ctx, tm.member, 1, "quickCapture", &disabled)).must(t)
	reset := got(tm.store.UpdateKeyboardShortcut(tm.ctx, tm.member, 2, "quickCapture", nil)).must(t)
	if _, ok := reset.Overrides["quickCapture"]; ok {
		t.Fatal("reset retained override")
	}
}

func TestKeyboardShortcutsRejectStaleInvalidAndAgentChangesAtomically(t *testing.T) {
	tm := newTeam(t)
	binding := "Ctrl+Alt+K"
	got(tm.store.UpdateKeyboardShortcut(tm.ctx, tm.member, 0, "quickCapture", &binding)).must(t)
	before := tm.eventCount(t)
	_, err := tm.store.UpdateKeyboardShortcut(tm.ctx, tm.member, 0, "quickCapture", nil)
	wantErr(t, err, ErrConflict)
	conflicting := "J"
	_, err = tm.store.UpdateKeyboardShortcut(tm.ctx, tm.member, 1, "newIssue", &conflicting)
	wantInvalid(t, err, "binding")
	_, err = tm.store.UpdateKeyboardShortcut(tm.ctx, tm.agent, 0, "quickCapture", &binding)
	wantErr(t, err, ErrForbidden)
	_, err = tm.store.GetKeyboardShortcuts(tm.ctx, tm.agent)
	wantErr(t, err, ErrForbidden)
	if tm.eventCount(t) != before {
		t.Fatal("rejected change wrote an event")
	}
	loaded := got(tm.store.GetKeyboardShortcuts(tm.ctx, tm.member)).must(t)
	if loaded.Revision != 1 || loaded.Overrides["quickCapture"] != binding {
		t.Fatalf("failed update changed preferences: %+v", loaded)
	}
}
