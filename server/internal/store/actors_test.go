package store

import (
	"testing"
)

func TestCreateHumanRequiresAnOwner(t *testing.T) {
	tm := newTeam(t)
	input := NewHuman{DisplayName: "New Person", Email: "new@example.com", Role: RoleMember}

	_, err := tm.store.CreateHuman(tm.ctx, tm.member, input)
	wantErr(t, err, ErrForbidden)

	_, err = tm.store.CreateHuman(tm.ctx, tm.agent, input)
	wantErr(t, err, ErrForbidden)

	person := got(tm.store.CreateHuman(tm.ctx, tm.owner, input)).must(t)
	if person.Kind != ActorHuman || person.Role != RoleMember {
		t.Errorf("created %+v", person)
	}
}

func TestCreateHumanNormalizesAndDeduplicatesEmail(t *testing.T) {
	tm := newTeam(t)

	person := got(tm.store.CreateHuman(tm.ctx, tm.owner, NewHuman{DisplayName: "Ann", Email: "  Ann@Example.COM ", Role: RoleMember})).must(t)
	if person.Email != "ann@example.com" {
		t.Errorf("email = %q, want it lowercased and trimmed", person.Email)
	}

	_, err := tm.store.CreateHuman(tm.ctx, tm.owner, NewHuman{DisplayName: "Ann Again", Email: "ANN@example.com", Role: RoleMember})
	wantErr(t, err, ErrConflict)

	found := got(tm.store.GetActorByEmail(tm.ctx, "ANN@EXAMPLE.com")).must(t)
	if found.ID != person.ID {
		t.Errorf("lookup by email found %s, want %s", found.ID, person.ID)
	}
}

func TestCreateHumanValidatesInput(t *testing.T) {
	tm := newTeam(t)

	cases := map[string]NewHuman{
		"display_name": {DisplayName: " ", Email: "a@example.com", Role: RoleMember},
		"email":        {DisplayName: "A", Email: "Ann <a@example.com>", Role: RoleMember},
		"role":         {DisplayName: "A", Email: "a@example.com", Role: "admin"},
	}
	for field, input := range cases {
		t.Run(field, func(t *testing.T) {
			_, err := tm.store.CreateHuman(tm.ctx, tm.owner, input)
			wantInvalid(t, err, field)
		})
	}
}

func TestCreateAgentBelongsToTheCallingPerson(t *testing.T) {
	tm := newTeam(t)

	agent := got(tm.store.CreateAgent(tm.ctx, tm.owner, NewAgent{DisplayName: "Codex (Olive)", AgentName: "codex"})).must(t)
	if agent.OwnerActorID != tm.owner.ActorID {
		t.Errorf("owner = %s, want the caller %s", agent.OwnerActorID, tm.owner.ActorID)
	}

	_, err := tm.store.CreateAgent(tm.ctx, tm.agent, NewAgent{DisplayName: "Sub-agent", AgentName: "codex"})
	wantErr(t, err, ErrForbidden)

	_, err = tm.store.CreateAgent(tm.ctx, tm.system, NewAgent{DisplayName: "System agent", AgentName: "codex"})
	wantErr(t, err, ErrForbidden)

	_, err = tm.store.CreateAgent(tm.ctx, tm.owner, NewAgent{DisplayName: "Bad", AgentName: "Claude Code"})
	wantInvalid(t, err, "agent_name")
}

func TestDisableActorStopsThePersonAndTheirAgents(t *testing.T) {
	tm := newTeam(t)
	at := "2026-03-01T09:00:00.000Z"
	_, err := tm.store.db.Exec(`
		INSERT INTO sessions (id, actor_id, session_hash, device_label, last_seen_at, expires_at, created_at, updated_at)
		VALUES ('s1', ?, randomblob(32), 'laptop', ?, ?, ?, ?)`, tm.member.ActorID, at, at, at, at)
	if err != nil {
		t.Fatalf("insert session: %v", err)
	}
	_, err = tm.store.db.Exec(`
		INSERT INTO agent_tokens (id, actor_id, label, token_hash, display_hint, expires_at, created_at, updated_at)
		VALUES ('t1', ?, 'laptop', randomblob(32), 'ssp_ab12', ?, ?, ?)`, tm.agent.ActorID, at, at, at)
	if err != nil {
		t.Fatalf("insert token: %v", err)
	}

	if err := tm.store.DisableActor(tm.ctx, tm.owner, tm.member.ActorID); err != nil {
		t.Fatalf("DisableActor: %v", err)
	}

	for _, id := range []string{tm.member.ActorID, tm.agent.ActorID} {
		if actor := got(tm.store.GetActor(tm.ctx, id)).must(t); actor.Active() {
			t.Errorf("%s is still active", actor.DisplayName)
		}
	}

	var liveSessions, liveTokens int
	tm.store.db.QueryRow(`SELECT count(*) FROM sessions WHERE revoked_at IS NULL`).Scan(&liveSessions)
	tm.store.db.QueryRow(`SELECT count(*) FROM agent_tokens WHERE revoked_at IS NULL`).Scan(&liveTokens)
	if liveSessions != 0 || liveTokens != 0 {
		t.Errorf("%d sessions and %d tokens still live", liveSessions, liveTokens)
	}

	project := got(tm.store.CreateProject(tm.ctx, tm.owner, NewProject{Key: "WP", Name: "WP Agent"})).must(t)
	_, err = tm.store.CreateIssue(tm.ctx, tm.agent, NewIssue{ProjectID: project.ID, Title: "Late write", Status: IssueTodo})
	wantErr(t, err, ErrForbidden)
}

func TestTheTeamKeepsAnActiveOwner(t *testing.T) {
	tm := newTeam(t)

	wantErr(t, tm.store.DisableActor(tm.ctx, tm.owner, tm.owner.ActorID), ErrConflict)
	wantErr(t, tm.store.SetRole(tm.ctx, tm.owner, tm.owner.ActorID, RoleMember), ErrConflict)

	if err := tm.store.SetRole(tm.ctx, tm.owner, tm.member.ActorID, RoleOwner); err != nil {
		t.Fatalf("promote member: %v", err)
	}
	if err := tm.store.SetRole(tm.ctx, tm.owner, tm.owner.ActorID, RoleMember); err != nil {
		t.Fatalf("demote the first owner once another exists: %v", err)
	}
}

func TestOnlyOwnersManagePeople(t *testing.T) {
	tm := newTeam(t)

	wantErr(t, tm.store.SetRole(tm.ctx, tm.member, tm.member.ActorID, RoleOwner), ErrForbidden)
	wantErr(t, tm.store.SetRole(tm.ctx, tm.agent, tm.member.ActorID, RoleOwner), ErrForbidden)
	wantErr(t, tm.store.DisableActor(tm.ctx, tm.agent, tm.owner.ActorID), ErrForbidden)
	wantInvalid(t, tm.store.DisableActor(tm.ctx, tm.owner, SystemActorID), "actor_id")
}

func TestUnknownCallerCannotWrite(t *testing.T) {
	tm := newTeam(t)

	_, err := tm.store.CreateProject(tm.ctx, Caller{ActorID: "nobody"}, NewProject{Key: "WP", Name: "WP"})
	wantErr(t, err, ErrForbidden)
}
