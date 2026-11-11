package store

import "testing"

func newCLILogin(tm *team, t *testing.T, name string) {
	t.Helper()
	got(tm.store.CreateCLILogin(tm.ctx, NewCLILogin{
		DeviceCodeHash: hashOf("device-" + name), UserCodeHash: hashOf("user-" + name),
		DeviceLabel: "laptop · CLI", RequestedFrom: "192.0.2.7",
	})).must(t)
}

func TestCLILoginExpiresUnapproved(t *testing.T) {
	tm := newTeam(t)
	newCLILogin(tm, t, "a")

	tm.clock.advance(CLILoginLifetime)

	_, err := tm.store.GetCLILogin(tm.ctx, hashOf("user-a"))
	wantErr(t, err, ErrNotFound)
	_, err = tm.store.DecideCLILogin(tm.ctx, Caller{ActorID: tm.member.ActorID, Client: ClientBrowser}, hashOf("user-a"), true)
	wantErr(t, err, ErrNotFound)
	status, _, _, err := tm.store.ClaimCLILogin(tm.ctx, hashOf("device-a"), hashOf("session-a"))
	if err != nil || status != CLILoginExpired {
		t.Fatalf("claim after expiry: %q, %v", status, err)
	}
}

func TestCLILoginApprovedByADisabledPersonIsDenied(t *testing.T) {
	tm := newTeam(t)
	newCLILogin(tm, t, "a")
	got(tm.store.DecideCLILogin(tm.ctx, Caller{ActorID: tm.member.ActorID, Client: ClientBrowser}, hashOf("user-a"), true)).must(t)

	if err := tm.store.DisableActor(tm.ctx, tm.owner, tm.member.ActorID); err != nil {
		t.Fatal(err)
	}

	status, _, _, err := tm.store.ClaimCLILogin(tm.ctx, hashOf("device-a"), hashOf("session-a"))
	if err != nil || status != CLILoginDenied {
		t.Fatalf("claim: %q, %v", status, err)
	}
}

func TestCLILoginClaimCreatesOneCLISessionWithAnEvent(t *testing.T) {
	tm := newTeam(t)
	newCLILogin(tm, t, "a")
	got(tm.store.DecideCLILogin(tm.ctx, Caller{ActorID: tm.member.ActorID, Client: ClientBrowser}, hashOf("user-a"), true)).must(t)

	status, actor, session, err := tm.store.ClaimCLILogin(tm.ctx, hashOf("device-a"), hashOf("session-a"))
	if err != nil || status != CLILoginApproved || actor.ID != tm.member.ActorID || session.Client != ClientCLI || session.DeviceLabel != "laptop · CLI" {
		t.Fatalf("claim: %q %+v %+v %v", status, actor, session, err)
	}
	checked := got(tm.store.AuthenticateSession(tm.ctx, hashOf("session-a"))).must(t)
	if checked.Actor.ID != tm.member.ActorID {
		t.Fatalf("session belongs to %s", checked.Actor.ID)
	}

	events := got(tm.store.ListEvents(tm.ctx, EventFilter{})).must(t)
	last := events[len(events)-1]
	if last.Kind != "session.created" || last.ActorID != tm.member.ActorID {
		t.Fatalf("last event %+v", last)
	}

	again, _, _, err := tm.store.ClaimCLILogin(tm.ctx, hashOf("device-a"), hashOf("session-b"))
	if err != nil || again != CLILoginExpired {
		t.Fatalf("second claim: %q, %v", again, err)
	}
}

func TestOnlyAPersonInTheAppDecidesCLILogins(t *testing.T) {
	tm := newTeam(t)
	newCLILogin(tm, t, "a")

	for _, caller := range []Caller{
		{ActorID: tm.member.ActorID, Client: ClientCLI},
		{ActorID: tm.agent.ActorID, Client: ClientBrowser},
		{ActorID: tm.agent.ActorID, Client: ClientAgentToken},
	} {
		_, err := tm.store.DecideCLILogin(tm.ctx, caller, hashOf("user-a"), true)
		wantErr(t, err, ErrForbidden)
	}
}
