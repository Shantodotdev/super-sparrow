package store

import (
	"crypto/sha256"
	"testing"
	"time"
)

func hashOf(value string) []byte {
	sum := sha256.Sum256([]byte(value))
	return sum[:]
}

func newSession(secret string) NewSession {
	return NewSession{Hash: hashOf(secret), DeviceLabel: "Firefox on Linux"}
}

func TestSignInCodeIsStoredOnlyForActiveTeammates(t *testing.T) {
	tm := newTeam(t)

	if !got(tm.store.IssueSignInCode(tm.ctx, "Max@Example.com", hashOf("111111"))).must(t) {
		t.Error("no code was stored for a teammate")
	}
	if got(tm.store.IssueSignInCode(tm.ctx, "stranger@example.com", hashOf("111111"))).must(t) {
		t.Error("a code was stored for someone outside the team")
	}

	member := got(tm.store.GetActorByEmail(tm.ctx, "max@example.com")).must(t)
	if err := tm.store.DisableActor(tm.ctx, tm.owner, member.ID); err != nil {
		t.Fatalf("DisableActor: %v", err)
	}
	if got(tm.store.IssueSignInCode(tm.ctx, "max@example.com", hashOf("222222"))).must(t) {
		t.Error("a code was stored for a disabled person")
	}
}

func TestSignInCodeCreatesASessionOnce(t *testing.T) {
	tm := newTeam(t)
	got(tm.store.IssueSignInCode(tm.ctx, "max@example.com", hashOf("123456"))).must(t)

	actor, session, err := tm.store.VerifySignInCode(tm.ctx, "MAX@example.com", hashOf("123456"), newSession("secret-1"))
	if err != nil {
		t.Fatalf("VerifySignInCode: %v", err)
	}
	if actor.Email != "max@example.com" || session.ActorID != actor.ID {
		t.Fatalf("signed in as %+v with %+v", actor, session)
	}
	if session.ExpiresAt.Sub(session.CreatedAt) != SessionLifetime {
		t.Errorf("session lasts %v, want %v", session.ExpiresAt.Sub(session.CreatedAt), SessionLifetime)
	}
	if event := tm.lastEvent(t); event.Kind != "session.created" || event.ActorID != actor.ID {
		t.Errorf("last event = %s by %s, want session.created by the person", event.Kind, event.ActorID)
	}

	_, _, err = tm.store.VerifySignInCode(tm.ctx, "max@example.com", hashOf("123456"), newSession("secret-2"))
	wantErr(t, err, ErrInvalidCode)
}

func TestSignInCodeExpires(t *testing.T) {
	tm := newTeam(t)
	got(tm.store.IssueSignInCode(tm.ctx, "max@example.com", hashOf("123456"))).must(t)

	tm.clock.advance(SignInCodeLifetime)

	_, _, err := tm.store.VerifySignInCode(tm.ctx, "max@example.com", hashOf("123456"), newSession("secret"))
	wantErr(t, err, ErrInvalidCode)
}

func TestFifthWrongAttemptCancelsTheCode(t *testing.T) {
	tm := newTeam(t)
	got(tm.store.IssueSignInCode(tm.ctx, "max@example.com", hashOf("123456"))).must(t)

	for range MaxSignInAttempts {
		_, _, err := tm.store.VerifySignInCode(tm.ctx, "max@example.com", hashOf("000000"), newSession("secret"))
		wantErr(t, err, ErrInvalidCode)
	}

	_, _, err := tm.store.VerifySignInCode(tm.ctx, "max@example.com", hashOf("123456"), newSession("secret"))
	wantErr(t, err, ErrInvalidCode)
}

func TestRecentCodesAllWorkAndTheOldestIsCanceled(t *testing.T) {
	tm := newTeam(t)
	codes := []string{"111111", "222222", "333333", "444444"}
	for _, code := range codes {
		got(tm.store.IssueSignInCode(tm.ctx, "max@example.com", hashOf(code))).must(t)
	}

	_, _, err := tm.store.VerifySignInCode(tm.ctx, "max@example.com", hashOf(codes[0]), newSession("secret"))
	wantErr(t, err, ErrInvalidCode)

	if _, _, err := tm.store.VerifySignInCode(tm.ctx, "max@example.com", hashOf(codes[1]), newSession("secret")); err != nil {
		t.Fatalf("a recent code was refused: %v", err)
	}

	// Signing in cancels the other codes.
	_, _, err = tm.store.VerifySignInCode(tm.ctx, "max@example.com", hashOf(codes[3]), newSession("secret-2"))
	wantErr(t, err, ErrInvalidCode)
}

func TestWrongGuessesCountAgainstEveryCode(t *testing.T) {
	tm := newTeam(t)
	got(tm.store.IssueSignInCode(tm.ctx, "max@example.com", hashOf("111111"))).must(t)
	got(tm.store.IssueSignInCode(tm.ctx, "max@example.com", hashOf("222222"))).must(t)

	for range MaxSignInAttempts {
		_, _, err := tm.store.VerifySignInCode(tm.ctx, "max@example.com", hashOf("000000"), newSession("secret"))
		wantErr(t, err, ErrInvalidCode)
	}

	for _, code := range []string{"111111", "222222"} {
		_, _, err := tm.store.VerifySignInCode(tm.ctx, "max@example.com", hashOf(code), newSession("secret"))
		wantErr(t, err, ErrInvalidCode)
	}
}

func TestCodesAreStoredOnlyAsHashes(t *testing.T) {
	tm := newTeam(t)
	got(tm.store.IssueSignInCode(tm.ctx, "max@example.com", hashOf("123456"))).must(t)

	var stored []byte
	if err := tm.store.db.QueryRow(`SELECT code_hash FROM sign_in_codes`).Scan(&stored); err != nil {
		t.Fatalf("read code: %v", err)
	}
	if string(stored) != string(hashOf("123456")) {
		t.Error("stored code is not the hash that was passed")
	}

	_, err := tm.store.IssueSignInCode(tm.ctx, "max@example.com", []byte("123456"))
	if err == nil {
		t.Error("a code that is not a SHA-256 hash was stored")
	}
}

func TestSessionAuthenticatesAndRenews(t *testing.T) {
	tm := newTeam(t)
	got(tm.store.IssueSignInCode(tm.ctx, "max@example.com", hashOf("123456"))).must(t)
	_, session, err := tm.store.VerifySignInCode(tm.ctx, "max@example.com", hashOf("123456"), newSession("secret"))
	if err != nil {
		t.Fatalf("VerifySignInCode: %v", err)
	}

	checked := got(tm.store.AuthenticateSession(tm.ctx, hashOf("secret"))).must(t)
	if checked.Actor.Email != "max@example.com" || checked.Renewed {
		t.Fatalf("first check = %+v, want the member and no renewal", checked)
	}

	tm.clock.advance(2 * time.Hour)
	renewed := got(tm.store.AuthenticateSession(tm.ctx, hashOf("secret"))).must(t)
	if !renewed.Renewed || !renewed.Session.ExpiresAt.After(session.ExpiresAt) {
		t.Fatalf("session was not renewed: %+v", renewed.Session)
	}

	tm.clock.advance(SessionLifetime)
	_, err = tm.store.AuthenticateSession(tm.ctx, hashOf("secret"))
	wantErr(t, err, ErrUnauthenticated)

	_, err = tm.store.AuthenticateSession(tm.ctx, hashOf("unknown"))
	wantErr(t, err, ErrUnauthenticated)
}

func TestSignOutOneDeviceOrAll(t *testing.T) {
	tm := newTeam(t)
	var sessions []Session
	for _, secret := range []string{"laptop", "desktop", "phone"} {
		got(tm.store.IssueSignInCode(tm.ctx, "max@example.com", hashOf("123456"))).must(t)
		_, session, err := tm.store.VerifySignInCode(tm.ctx, "max@example.com", hashOf("123456"), newSession(secret))
		if err != nil {
			t.Fatalf("VerifySignInCode: %v", err)
		}
		sessions = append(sessions, session)
	}

	wantErr(t, tm.store.RevokeSession(tm.ctx, tm.owner, sessions[0].ID), ErrNotFound)
	wantErr(t, tm.store.RevokeSession(tm.ctx, tm.agent, sessions[0].ID), ErrForbidden)

	if err := tm.store.RevokeSession(tm.ctx, tm.member, sessions[0].ID); err != nil {
		t.Fatalf("RevokeSession: %v", err)
	}
	_, err := tm.store.AuthenticateSession(tm.ctx, hashOf("laptop"))
	wantErr(t, err, ErrUnauthenticated)
	if listed := got(tm.store.ListSessions(tm.ctx, sessions[0].ActorID)).must(t); len(listed) != 2 {
		t.Fatalf("%d sessions listed, want 2", len(listed))
	}

	if count := got(tm.store.RevokeAllSessions(tm.ctx, tm.member)).must(t); count != 2 {
		t.Errorf("revoked %d sessions, want 2", count)
	}
	if listed := got(tm.store.ListSessions(tm.ctx, sessions[0].ActorID)).must(t); len(listed) != 0 {
		t.Errorf("%d sessions left", len(listed))
	}
}

func TestDisablingAPersonEndsTheirSessions(t *testing.T) {
	tm := newTeam(t)
	got(tm.store.IssueSignInCode(tm.ctx, "max@example.com", hashOf("123456"))).must(t)
	actor, _, err := tm.store.VerifySignInCode(tm.ctx, "max@example.com", hashOf("123456"), newSession("secret"))
	if err != nil {
		t.Fatalf("VerifySignInCode: %v", err)
	}

	if err := tm.store.DisableActor(tm.ctx, tm.owner, actor.ID); err != nil {
		t.Fatalf("DisableActor: %v", err)
	}

	_, err = tm.store.AuthenticateSession(tm.ctx, hashOf("secret"))
	wantErr(t, err, ErrUnauthenticated)
}

func TestFirstOwnerOnlyOnAnEmptyTeam(t *testing.T) {
	tm := newTeam(t)

	_, err := tm.store.CreateFirstOwner(tm.ctx, "Second Owner", "second@example.com")
	wantErr(t, err, ErrConflict)
}
