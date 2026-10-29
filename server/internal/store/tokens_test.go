package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/shantodotdev/super-sparrow/server/internal/database"
	"github.com/shantodotdev/super-sparrow/server/migrations"
)

func newToken(agentName, secret string) NewAgentToken {
	return NewAgentToken{AgentName: agentName, Label: "Max · " + agentName + " · laptop", Hash: hashOf(secret), DisplayHint: "ssp_…" + secret[len(secret)-4:]}
}

func TestTokensShareOneAgentPerTool(t *testing.T) {
	tm := newTeam(t)

	first := got(tm.store.CreateAgentToken(tm.ctx, tm.member, newToken("codex", "secret-laptop"))).must(t)
	second := got(tm.store.CreateAgentToken(tm.ctx, tm.member, newToken("codex", "secret-desktop"))).must(t)
	if first.AgentActorID != second.AgentActorID {
		t.Fatal("two codex tokens of one person act as different agents")
	}
	if first.ExpiresAt.Sub(first.CreatedAt) != TokenLifetime {
		t.Errorf("token lasts %v, want %v", first.ExpiresAt.Sub(first.CreatedAt), TokenLifetime)
	}

	agent := got(tm.store.GetActor(tm.ctx, first.AgentActorID)).must(t)
	if agent.Kind != ActorAgent || agent.OwnerActorID != tm.member.ActorID || agent.DisplayName != "Codex" {
		t.Errorf("agent = %+v", agent)
	}

	owners := got(tm.store.CreateAgentToken(tm.ctx, tm.owner, newToken("codex", "secret-owner"))).must(t)
	if owners.AgentActorID == first.AgentActorID {
		t.Error("two people share one agent")
	}
}

func TestAgentsCannotCreateTokens(t *testing.T) {
	tm := newTeam(t)

	_, err := tm.store.CreateAgentToken(tm.ctx, tm.agent, newToken("codex", "secret-agent"))
	wantErr(t, err, ErrForbidden)
}

func TestTokenAuthenticatesAsItsAgent(t *testing.T) {
	tm := newTeam(t)
	token := got(tm.store.CreateAgentToken(tm.ctx, tm.member, newToken("claude-code", "secret-1234"))).must(t)

	checked := got(tm.store.AuthenticateToken(tm.ctx, hashOf("secret-1234"))).must(t)
	if checked.Actor.ID != token.AgentActorID || checked.Token.LastUsedAt == nil {
		t.Fatalf("authenticated as %+v, last used %v", checked.Actor, checked.Token.LastUsedAt)
	}

	_, err := tm.store.AuthenticateToken(tm.ctx, hashOf("wrong"))
	wantErr(t, err, ErrUnauthenticated)
}

func TestTokenExpiresAndRenews(t *testing.T) {
	tm := newTeam(t)
	token := got(tm.store.CreateAgentToken(tm.ctx, tm.member, newToken("codex", "secret-1234"))).must(t)

	tm.clock.advance(TokenLifetime)
	_, err := tm.store.AuthenticateToken(tm.ctx, hashOf("secret-1234"))
	wantErr(t, err, ErrUnauthenticated)

	_, err = tm.store.RenewAgentToken(tm.ctx, tm.owner, token.ID)
	wantErr(t, err, ErrNotFound)

	renewed := got(tm.store.RenewAgentToken(tm.ctx, tm.member, token.ID)).must(t)
	if !renewed.ExpiresAt.After(token.ExpiresAt.Add(time.Hour)) {
		t.Errorf("renewed expiry = %v, want later than %v", renewed.ExpiresAt, token.ExpiresAt)
	}
	got(tm.store.AuthenticateToken(tm.ctx, hashOf("secret-1234"))).must(t)
}

func TestRevokingTokens(t *testing.T) {
	tm := newTeam(t)
	mine := got(tm.store.CreateAgentToken(tm.ctx, tm.member, newToken("codex", "secret-mine"))).must(t)
	other := got(tm.store.CreateAgentToken(tm.ctx, tm.member, newToken("codex", "secret-other"))).must(t)
	owners := got(tm.store.CreateAgentToken(tm.ctx, tm.owner, newToken("codex", "secret-owner"))).must(t)

	wantErr(t, tm.store.RevokeAgentToken(tm.ctx, tm.member, owners.ID), ErrNotFound)
	wantErr(t, tm.store.RevokeAgentToken(tm.ctx, tm.agent, mine.ID), ErrForbidden)

	if err := tm.store.RevokeAgentToken(tm.ctx, tm.member, mine.ID); err != nil {
		t.Fatalf("revoke own token: %v", err)
	}
	if err := tm.store.RevokeAgentToken(tm.ctx, tm.owner, other.ID); err != nil {
		t.Fatalf("owner revokes a member's token: %v", err)
	}
	if event := tm.lastEvent(t); event.Kind != "token.revoked" || event.ActorID != tm.owner.ActorID {
		t.Errorf("last event = %s by %s, want token.revoked by the owner", event.Kind, event.ActorID)
	}

	for _, secret := range []string{"secret-mine", "secret-other"} {
		_, err := tm.store.AuthenticateToken(tm.ctx, hashOf(secret))
		wantErr(t, err, ErrUnauthenticated)
	}
	_, err := tm.store.RenewAgentToken(tm.ctx, tm.member, mine.ID)
	wantErr(t, err, ErrInvalidTransition)

	if listed := got(tm.store.ListAgentTokens(tm.ctx, tm.member.ActorID)).must(t); len(listed) != 0 {
		t.Errorf("%d revoked tokens listed", len(listed))
	}
	if listed := got(tm.store.ListAgentTokens(tm.ctx, "")).must(t); len(listed) != 1 || listed[0].ID != owners.ID {
		t.Errorf("all tokens = %+v, want only the owner's", listed)
	}
}

func TestDisablingAPersonRevokesTheirTokens(t *testing.T) {
	tm := newTeam(t)
	got(tm.store.CreateAgentToken(tm.ctx, tm.member, newToken("codex", "secret-1234"))).must(t)

	if err := tm.store.DisableActor(tm.ctx, tm.owner, tm.member.ActorID); err != nil {
		t.Fatalf("DisableActor: %v", err)
	}

	_, err := tm.store.AuthenticateToken(tm.ctx, hashOf("secret-1234"))
	wantErr(t, err, ErrUnauthenticated)
	_, err = tm.store.CreateAgentToken(tm.ctx, tm.member, newToken("codex", "secret-5678"))
	wantErr(t, err, ErrForbidden)
}

func TestAgentDisplayNameIsTheTool(t *testing.T) {
	for agentName, want := range map[string]string{"claude-code": "Claude Code", "codex": "Codex", "aider": "aider"} {
		if name := agentDisplayName(agentName); name != want {
			t.Errorf("agentDisplayName(%q) = %q, want %q", agentName, name, want)
		}
	}
}

func TestCreateFirstOwner(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := database.Migrate(ctx, db, migrations.Files); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	st := New(db, time.Now)

	owner, err := st.CreateFirstOwner(ctx, "Shanto", "Shanto@Example.com")
	if err != nil {
		t.Fatalf("CreateFirstOwner: %v", err)
	}
	if !owner.IsOwner() || owner.Email != "shanto@example.com" {
		t.Errorf("owner = %+v", owner)
	}

	_, err = st.CreateFirstOwner(ctx, "Another", "another@example.com")
	wantErr(t, err, ErrConflict)
}

func TestNamesAndLabelsMustBeOneLine(t *testing.T) {
	tm := newTeam(t)

	_, err := tm.store.CreateHuman(tm.ctx, tm.owner, NewHuman{DisplayName: "Eve\nBcc: everyone", Email: "eve@example.com", Role: RoleMember})
	wantInvalid(t, err, "display_name")

	token := newToken("codex", "secret-1234")
	token.Label = "laptop\r\nsecond line"
	_, err = tm.store.CreateAgentToken(tm.ctx, tm.member, token)
	wantInvalid(t, err, "label")
}
