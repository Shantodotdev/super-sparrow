package store

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"slices"
	"testing"

	"github.com/shantodotdev/super-sparrow/server/internal/secretbox"
)

// secretsTeam is a team with project WP, secrets turned on, and callers for each client.
type secretsTeam struct {
	*team
	project       Project
	ownerWeb      Caller
	ownerCLI      Caller
	memberWeb     Caller
	memberCLI     Caller
	agentToken    Caller
	agentOfMember Actor
}

func newSecretsTeam(t *testing.T) *secretsTeam {
	t.Helper()
	tm := newTeam(t)

	box, err := secretbox.New(bytes.Repeat([]byte{7}, secretbox.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	tm.store.UseSecretBox(box)

	st := &secretsTeam{
		team:       tm,
		ownerWeb:   Caller{ActorID: tm.owner.ActorID, Client: ClientBrowser},
		ownerCLI:   Caller{ActorID: tm.owner.ActorID, Client: ClientCLI},
		memberWeb:  Caller{ActorID: tm.member.ActorID, Client: ClientBrowser},
		memberCLI:  Caller{ActorID: tm.member.ActorID, Client: ClientCLI},
		agentToken: Caller{ActorID: tm.agent.ActorID, Client: ClientAgentToken},
	}
	project := got(tm.store.CreateProject(tm.ctx, tm.owner, NewProject{Key: "WP", Name: "WP Agent"})).must(t)
	st.project = got(tm.store.EnableSecrets(tm.ctx, st.ownerWeb, project.ID)).must(t)
	st.agentOfMember = got(tm.store.GetActor(tm.ctx, tm.agent.ActorID)).must(t)
	return st
}

func (st *secretsTeam) set(t *testing.T, by Caller, config SecretConfig, values map[string]string) SecretListing {
	t.Helper()
	return got(st.store.ChangeSecrets(st.ctx, by, st.project.ID, config, SecretChange{Set: values})).must(t)
}

func (st *secretsTeam) reveal(t *testing.T, by Caller, config SecretConfig, name string) SecretValue {
	t.Helper()
	return got(st.store.RevealSecret(st.ctx, by, st.project.ID, config, name)).must(t)
}

// wantValidation fails unless err reports an invalid input.
func wantValidation(t *testing.T, err error) {
	t.Helper()
	var validation *ValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("error = %v, want a validation error", err)
	}
}

func names(entries []SecretEntry) []string {
	result := []string{}
	for _, entry := range entries {
		result = append(result, entry.Name)
	}
	return result
}

func TestSecretsStayOffUntilAnOwnerTurnsThemOn(t *testing.T) {
	tm := newTeam(t)
	box, err := secretbox.New(bytes.Repeat([]byte{7}, secretbox.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	tm.store.UseSecretBox(box)
	project := got(tm.store.CreateProject(tm.ctx, tm.owner, NewProject{Key: "WP", Name: "WP Agent"})).must(t)
	memberWeb := Caller{ActorID: tm.member.ActorID, Client: ClientBrowser}

	overview := got(tm.store.SecretsOverview(tm.ctx, memberWeb, project.ID)).must(t)
	if overview.Enabled || overview.Configs != nil {
		t.Fatalf("overview before turning on = %+v", overview)
	}
	_, err = tm.store.ChangeSecrets(tm.ctx, memberWeb, project.ID, ConfigDev, SecretChange{Set: map[string]string{"A": "1"}})
	wantErr(t, err, ErrConflict)

	_, err = tm.store.EnableSecrets(tm.ctx, memberWeb, project.ID)
	wantErr(t, err, ErrForbidden)
	_, err = tm.store.EnableSecrets(tm.ctx, tm.agent, project.ID)
	wantErr(t, err, ErrForbidden)

	enabled := got(tm.store.EnableSecrets(tm.ctx, Caller{ActorID: tm.owner.ActorID, Client: ClientBrowser}, project.ID)).must(t)
	if !enabled.SecretsEnabled {
		t.Fatal("project not enabled")
	}
	events := got(tm.store.ListEvents(tm.ctx, EventFilter{ProjectID: project.ID, ViewerActorID: tm.member.ActorID})).must(t)
	if events[len(events)-1].Kind != "secrets.enabled" {
		t.Fatalf("last event %q", events[len(events)-1].Kind)
	}
}

func TestStagingAndProductionNeedThePermissionInTheBrowser(t *testing.T) {
	st := newSecretsTeam(t)
	st.set(t, st.ownerWeb, ConfigStaging, map[string]string{"API_KEY": "staging-key"})

	// Everyone signed in sees names, but only people who can manage read values, and only in a browser.
	overview := got(st.store.SecretsOverview(st.ctx, st.memberWeb, st.project.ID)).must(t)
	if overview.CanManage || !slices.Equal(names(overview.Configs[ConfigStaging].Secrets), []string{"API_KEY"}) {
		t.Fatalf("member overview = %+v", overview)
	}
	for _, by := range []Caller{st.memberWeb, st.memberCLI, st.ownerCLI, st.agentToken} {
		_, err := st.store.RevealSecret(st.ctx, by, st.project.ID, ConfigStaging, "API_KEY")
		wantErr(t, err, ErrForbidden)
		_, err = st.store.DownloadSecrets(st.ctx, by, st.project.ID, ConfigProduction)
		wantErr(t, err, ErrForbidden)
		_, err = st.store.ChangeSecrets(st.ctx, by, st.project.ID, ConfigStaging, SecretChange{Set: map[string]string{"B": "x"}})
		wantErr(t, err, ErrForbidden)
	}
	if value := st.reveal(t, st.ownerWeb, ConfigStaging, "API_KEY"); value.Value != "staging-key" || value.Version != 1 {
		t.Fatalf("owner reveal = %+v", value)
	}

	// An owner gives the member the permission; members cannot give it to themselves.
	_, err := st.store.SetCanManageEnvironments(st.ctx, st.memberWeb, st.member.ActorID, true)
	wantErr(t, err, ErrForbidden)
	person := got(st.store.SetCanManageEnvironments(st.ctx, st.ownerWeb, st.member.ActorID, true)).must(t)
	if !person.CanManageEnvironments {
		t.Fatal("permission not set")
	}
	_, err = st.store.SetCanManageEnvironments(st.ctx, st.ownerWeb, st.agent.ActorID, true)
	wantValidation(t, err)

	st.set(t, st.memberWeb, ConfigProduction, map[string]string{"API_KEY": "production-key"})
	if value := st.reveal(t, st.memberWeb, ConfigProduction, "API_KEY"); value.Value != "production-key" {
		t.Fatalf("member reveal = %+v", value)
	}
	_, err = st.store.RevealSecret(st.ctx, st.memberCLI, st.project.ID, ConfigProduction, "API_KEY")
	wantErr(t, err, ErrForbidden)
	_, err = st.store.RevealSecret(st.ctx, st.agentToken, st.project.ID, ConfigProduction, "API_KEY")
	wantErr(t, err, ErrForbidden)
}

func TestPersonalDevIsDefaultsWithPrivateOverrides(t *testing.T) {
	st := newSecretsTeam(t)
	st.set(t, st.ownerWeb, ConfigDevDefaults, map[string]string{"OPENROUTER_API_KEY": "team-key", "LOG_LEVEL": "info"})

	// A member cannot edit the defaults but can override them for themselves, also from the CLI.
	_, err := st.store.ChangeSecrets(st.ctx, st.memberWeb, st.project.ID, ConfigDevDefaults, SecretChange{Set: map[string]string{"LOG_LEVEL": "debug"}})
	wantErr(t, err, ErrForbidden)
	listing := st.set(t, st.memberCLI, ConfigDev, map[string]string{"LOG_LEVEL": "debug", "MY_ONLY": "mine"})
	if !slices.Equal(names(listing.Secrets), []string{"LOG_LEVEL", "MY_ONLY", "OPENROUTER_API_KEY"}) {
		t.Fatalf("member dev = %v", names(listing.Secrets))
	}
	sources := map[string]SecretSource{}
	for _, entry := range listing.Secrets {
		sources[entry.Name] = entry.Source
	}
	if sources["LOG_LEVEL"] != SourceOverride || sources["OPENROUTER_API_KEY"] != SourceDefault {
		t.Fatalf("sources = %v", sources)
	}

	// The owner's dev is untouched, and nobody sees another person's overrides.
	ownerDev := got(st.store.DownloadSecrets(st.ctx, st.ownerWeb, st.project.ID, ConfigDev)).must(t)
	if ownerDev.Values["LOG_LEVEL"] != "info" || ownerDev.Values["MY_ONLY"] != "" {
		t.Fatalf("owner dev = %v", ownerDev.Values)
	}

	// The member's agent reads the member's dev, defaults included, but cannot change it.
	agentDev := got(st.store.DownloadSecrets(st.ctx, st.agentToken, st.project.ID, ConfigDev)).must(t)
	if agentDev.Values["LOG_LEVEL"] != "debug" || agentDev.Values["OPENROUTER_API_KEY"] != "team-key" || agentDev.Values["MY_ONLY"] != "mine" {
		t.Fatalf("agent dev = %v", agentDev.Values)
	}
	_, err = st.store.ChangeSecrets(st.ctx, st.agentToken, st.project.ID, ConfigDev, SecretChange{Set: map[string]string{"X": "1"}})
	wantErr(t, err, ErrForbidden)

	// Deleting an override goes back to the team default.
	got(st.store.ChangeSecrets(st.ctx, st.memberWeb, st.project.ID, ConfigDev, SecretChange{Delete: []string{"LOG_LEVEL"}})).must(t)
	if value := st.reveal(t, st.memberWeb, ConfigDev, "LOG_LEVEL"); value.Value != "info" || value.Source != SourceDefault {
		t.Fatalf("after reset = %+v", value)
	}

	// Without a client, nobody reads values.
	_, err = st.store.RevealSecret(st.ctx, Caller{ActorID: st.member.ActorID}, st.project.ID, ConfigDev, "LOG_LEVEL")
	wantErr(t, err, ErrForbidden)
}

func TestSecretHistoryKeepsEveryVersion(t *testing.T) {
	st := newSecretsTeam(t)
	st.set(t, st.ownerWeb, ConfigStaging, map[string]string{"TOKEN": "one"})
	st.set(t, st.ownerWeb, ConfigStaging, map[string]string{"TOKEN": "two"})
	got(st.store.ChangeSecrets(st.ctx, st.ownerWeb, st.project.ID, ConfigStaging, SecretChange{Delete: []string{"TOKEN"}})).must(t)

	_, err := st.store.RevealSecret(st.ctx, st.ownerWeb, st.project.ID, ConfigStaging, "TOKEN")
	wantErr(t, err, ErrNotFound)
	_, err = st.store.ChangeSecrets(st.ctx, st.ownerWeb, st.project.ID, ConfigStaging, SecretChange{Delete: []string{"TOKEN"}})
	wantErr(t, err, ErrNotFound)

	versions := got(st.store.SecretHistory(st.ctx, st.ownerWeb, st.project.ID, ConfigStaging, "TOKEN")).must(t)
	if len(versions) != 3 || !versions[0].Deleted || versions[0].Version != 3 || versions[2].Version != 1 {
		t.Fatalf("history = %+v", versions)
	}

	got(st.store.RestoreSecret(st.ctx, st.ownerWeb, st.project.ID, ConfigStaging, "TOKEN", 1)).must(t)
	if value := st.reveal(t, st.ownerWeb, ConfigStaging, "TOKEN"); value.Value != "one" || value.Version != 4 {
		t.Fatalf("restored = %+v", value)
	}
	_, err = st.store.RestoreSecret(st.ctx, st.ownerWeb, st.project.ID, ConfigStaging, "TOKEN", 4)
	wantValidation(t, err)
	_, err = st.store.RestoreSecret(st.ctx, st.memberWeb, st.project.ID, ConfigStaging, "TOKEN", 2)
	wantErr(t, err, ErrForbidden)
	_, err = st.store.SecretHistory(st.ctx, st.memberWeb, st.project.ID, ConfigStaging, "TOKEN")
	wantErr(t, err, ErrForbidden)
}

func TestSecretValuesAreEncryptedAndBoundToTheirRecord(t *testing.T) {
	st := newSecretsTeam(t)
	st.set(t, st.ownerWeb, ConfigStaging, map[string]string{"FIRST": "plain-first-value", "SECOND": "plain-second-value"})

	rows := got(st.store.db.QueryContext(st.ctx, `SELECT ciphertext FROM secret_versions`)).must(t)
	for rows.Next() {
		var ciphertext []byte
		if err := rows.Scan(&ciphertext); err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(ciphertext, []byte("plain-")) {
			t.Fatal("a stored value contains its plaintext")
		}
	}
	rows.Close()

	// Copying one key's ciphertext onto another makes it unreadable rather than revealing it.
	_, err := st.store.db.ExecContext(st.ctx, `
		UPDATE secret_versions SET ciphertext = (
			SELECT v.ciphertext FROM secret_versions v JOIN secrets s ON s.id = v.secret_id WHERE s.name = 'FIRST')
		WHERE secret_id = (SELECT id FROM secrets WHERE name = 'SECOND')`)
	if err == nil {
		t.Fatal("secret versions were changed despite the trigger")
	}
	_, err = st.store.db.ExecContext(st.ctx, `DROP TRIGGER secret_versions_no_update`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.store.db.ExecContext(st.ctx, `
		UPDATE secret_versions SET ciphertext = (
			SELECT v.ciphertext FROM secret_versions v JOIN secrets s ON s.id = v.secret_id WHERE s.name = 'FIRST')
		WHERE secret_id = (SELECT id FROM secrets WHERE name = 'SECOND')`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.store.RevealSecret(st.ctx, st.ownerWeb, st.project.ID, ConfigStaging, "SECOND")
	if err == nil {
		t.Fatal("a moved ciphertext was decrypted")
	}
}

func TestRenamingAStoredSecretMakesItUnreadable(t *testing.T) {
	st := newSecretsTeam(t)
	st.set(t, st.ownerWeb, ConfigDevDefaults, map[string]string{"DEV_KEY": "dev-value"})

	// Moving a dev value into production by editing the row does not reveal it there.
	_, err := st.store.db.ExecContext(st.ctx, `UPDATE secrets SET config = 'production' WHERE name = 'DEV_KEY'`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.store.RevealSecret(st.ctx, st.ownerWeb, st.project.ID, ConfigProduction, "DEV_KEY"); err == nil {
		t.Fatal("a value moved to another config was decrypted")
	}
}

func TestSecretChangesAreValidated(t *testing.T) {
	st := newSecretsTeam(t)
	for _, change := range []SecretChange{
		{},
		{Set: map[string]string{"lower": "x"}},
		{Set: map[string]string{"1ABC": "x"}},
		{Set: map[string]string{"A": string([]byte{0xff})}},
		{Set: map[string]string{"A": string(make([]byte, MaxSecretValue+1))}},
		{Set: map[string]string{"A": "x"}, Delete: []string{"A"}},
		{Delete: []string{"NO-DASH"}},
		{Set: map[string]string{"LD_PRELOAD": "/tmp/x.so"}},
		{Set: map[string]string{"PATH": "/tmp"}},
		{Set: map[string]string{"NODE_OPTIONS": "--require /tmp/x.js"}},
	} {
		_, err := st.store.ChangeSecrets(st.ctx, st.ownerWeb, st.project.ID, ConfigStaging, change)
		wantValidation(t, err)
	}
}

func TestRequiredKeysShowWhatIsMissing(t *testing.T) {
	st := newSecretsTeam(t)
	st.set(t, st.ownerWeb, ConfigDevDefaults, map[string]string{"A": "1"})

	_, err := st.store.SetRequiredKeys(st.ctx, st.memberWeb, st.project.ID, ConfigDev, []string{"A"})
	wantErr(t, err, ErrForbidden)
	_, err = st.store.SetRequiredKeys(st.ctx, st.ownerWeb, st.project.ID, ConfigDevDefaults, []string{"A"})
	wantValidation(t, err)

	listing := got(st.store.SetRequiredKeys(st.ctx, st.ownerWeb, st.project.ID, ConfigDev, []string{"B", "A", "B"})).must(t)
	if !slices.Equal(listing.Required, []string{"A", "B"}) || !slices.Equal(listing.Missing, []string{"B"}) {
		t.Fatalf("listing = %+v", listing)
	}

	st.set(t, st.memberWeb, ConfigDev, map[string]string{"B": "mine"})
	overview := got(st.store.SecretsOverview(st.ctx, st.memberWeb, st.project.ID)).must(t)
	if len(overview.Configs[ConfigDev].Missing) != 0 || !slices.Equal(overview.Configs[ConfigDevDefaults].Missing, []string{"B"}) {
		t.Fatalf("member dev missing %v, defaults missing %v", overview.Configs[ConfigDev].Missing, overview.Configs[ConfigDevDefaults].Missing)
	}
}

func TestServiceTokensReadOneConfig(t *testing.T) {
	st := newSecretsTeam(t)
	st.set(t, st.ownerWeb, ConfigStaging, map[string]string{"DEPLOY_HOST": "staging.example.com"})
	st.set(t, st.ownerWeb, ConfigProduction, map[string]string{"DEPLOY_HOST": "example.com"})
	hash := sha256.Sum256([]byte("sst_test"))

	_, err := st.store.CreateServiceToken(st.ctx, st.memberWeb, st.project.ID, NewServiceToken{Config: ConfigStaging, Label: "CI", Hash: hash[:]})
	wantErr(t, err, ErrForbidden)
	_, err = st.store.CreateServiceToken(st.ctx, st.ownerWeb, st.project.ID, NewServiceToken{Config: ConfigDev, Label: "CI", Hash: hash[:]})
	wantValidation(t, err)
	token := got(st.store.CreateServiceToken(st.ctx, st.ownerWeb, st.project.ID, NewServiceToken{Config: ConfigStaging, Label: "CircleCI", Hash: hash[:], DisplayHint: "sst_…test"})).must(t)

	used, values, err := st.store.DownloadWithServiceToken(st.ctx, hash[:])
	if err != nil || used.ID != token.ID || values.Values["DEPLOY_HOST"] != "staging.example.com" || len(values.Values) != 1 {
		t.Fatalf("download = %+v, %v, %v", used, values, err)
	}
	other := sha256.Sum256([]byte("sst_other"))
	_, _, err = st.store.DownloadWithServiceToken(st.ctx, other[:])
	wantErr(t, err, ErrUnauthenticated)

	// The download is in the owner's access log.
	entries := got(st.store.AccessLog(st.ctx, st.ownerWeb, st.project.ID, ConfigStaging, 0, 0)).must(t)
	if len(entries) != 1 || entries[0].ServiceTokenID != token.ID || entries[0].Action != AccessDownload {
		t.Fatalf("access log = %+v", entries)
	}
	_, err = st.store.AccessLog(st.ctx, st.memberWeb, st.project.ID, ConfigStaging, 0, 0)
	wantErr(t, err, ErrForbidden)

	// Expired and revoked tokens stop working; renewing extends an unrevoked one.
	st.clock.advance(ServiceTokenLifetime)
	_, _, err = st.store.DownloadWithServiceToken(st.ctx, hash[:])
	wantErr(t, err, ErrUnauthenticated)
	renewed := got(st.store.RenewServiceToken(st.ctx, st.ownerWeb, token.ID)).must(t)
	if !renewed.ExpiresAt.After(token.ExpiresAt) {
		t.Fatalf("renewed expiry %v", renewed.ExpiresAt)
	}
	if _, _, err := st.store.DownloadWithServiceToken(st.ctx, hash[:]); err != nil {
		t.Fatalf("download after renewing: %v", err)
	}
	if err := st.store.RevokeServiceToken(st.ctx, st.ownerWeb, token.ID); err != nil {
		t.Fatal(err)
	}
	if err := st.store.RevokeServiceToken(st.ctx, st.ownerWeb, token.ID); err != nil {
		t.Fatal(err)
	}
	_, _, err = st.store.DownloadWithServiceToken(st.ctx, hash[:])
	wantErr(t, err, ErrUnauthenticated)
	_, err = st.store.RenewServiceToken(st.ctx, st.ownerWeb, token.ID)
	wantErr(t, err, ErrConflict)

	tokens := got(st.store.ListServiceTokens(st.ctx, st.ownerWeb, st.project.ID)).must(t)
	if len(tokens) != 1 || tokens[0].RevokedAt == nil {
		t.Fatalf("tokens = %+v", tokens)
	}
}

func TestDevAccessLogAndEventsArePrivate(t *testing.T) {
	st := newSecretsTeam(t)
	st.set(t, st.memberWeb, ConfigDev, map[string]string{"PRIVATE": "x"})
	st.reveal(t, st.memberWeb, ConfigDev, "PRIVATE")

	entries := got(st.store.AccessLog(st.ctx, st.memberWeb, st.project.ID, ConfigDev, 0, 0)).must(t)
	if len(entries) != 1 || entries[0].ActorID != st.member.ActorID {
		t.Fatalf("member's dev log = %+v", entries)
	}
	if entries := got(st.store.AccessLog(st.ctx, st.ownerWeb, st.project.ID, ConfigDev, 0, 0)).must(t); len(entries) != 0 {
		t.Fatalf("owner sees the member's dev log: %+v", entries)
	}

	hasDevEvent := func(viewer string) bool {
		events := got(st.store.ListEvents(st.ctx, EventFilter{ViewerActorID: viewer})).must(t)
		return slices.ContainsFunc(events, func(e Event) bool { return e.Kind == "dev_secret.changed" })
	}
	if !hasDevEvent(st.member.ActorID) || hasDevEvent(st.owner.ActorID) || hasDevEvent("") {
		t.Fatal("a dev secret event is visible to someone other than its person")
	}
}

func TestDisablingAPersonDeletesTheirDevOverrides(t *testing.T) {
	st := newSecretsTeam(t)
	st.set(t, st.memberWeb, ConfigDev, map[string]string{"PRIVATE": "x"})

	if err := st.store.DisableActor(st.ctx, st.owner, st.member.ActorID); err != nil {
		t.Fatal(err)
	}

	var count int
	if err := st.store.db.QueryRowContext(st.ctx, `SELECT count(*) FROM secrets WHERE config = 'dev_personal'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("%d dev overrides remain", count)
	}
}

func TestSecretsNeedTheEncryptionKey(t *testing.T) {
	st := newSecretsTeam(t)
	st.store.UseSecretBox(nil)

	_, err := st.store.ChangeSecrets(st.ctx, st.ownerWeb, st.project.ID, ConfigStaging, SecretChange{Set: map[string]string{"A": "1"}})
	if err == nil {
		t.Fatal("a secret was stored without encryption")
	}
}

func TestUnsettingAKeyThatUsesTheDefaultChangesNothing(t *testing.T) {
	st := newSecretsTeam(t)
	st.set(t, st.ownerWeb, ConfigDevDefaults, map[string]string{"SHARED": "team"})
	st.set(t, st.memberWeb, ConfigDev, map[string]string{"MINE": "x"})

	listing := got(st.store.ChangeSecrets(st.ctx, st.memberCLI, st.project.ID, ConfigDev, SecretChange{Delete: []string{"SHARED", "MINE"}})).must(t)
	if !slices.Equal(names(listing.Secrets), []string{"SHARED"}) {
		t.Fatalf("dev after unset = %v", names(listing.Secrets))
	}
	_, err := st.store.ChangeSecrets(st.ctx, st.memberCLI, st.project.ID, ConfigDev, SecretChange{Delete: []string{"NEVER_SET"}})
	wantErr(t, err, ErrNotFound)
}

func TestCITokensStopWhenTheirCreatorLosesAccess(t *testing.T) {
	st := newSecretsTeam(t)
	got(st.store.SetCanManageEnvironments(st.ctx, st.ownerWeb, st.member.ActorID, true)).must(t)
	first, second := sha256.Sum256([]byte("sst_first")), sha256.Sum256([]byte("sst_second"))
	got(st.store.CreateServiceToken(st.ctx, st.memberWeb, st.project.ID, NewServiceToken{Config: ConfigStaging, Label: "One", Hash: first[:]})).must(t)
	got(st.store.CreateServiceToken(st.ctx, st.ownerWeb, st.project.ID, NewServiceToken{Config: ConfigStaging, Label: "Two", Hash: second[:]})).must(t)

	got(st.store.SetCanManageEnvironments(st.ctx, st.ownerWeb, st.member.ActorID, false)).must(t)
	_, _, err := st.store.DownloadWithServiceToken(st.ctx, first[:])
	wantErr(t, err, ErrUnauthenticated)
	if _, _, err := st.store.DownloadWithServiceToken(st.ctx, second[:]); err != nil {
		t.Fatalf("the owner's token stopped working: %v", err)
	}

	other := got(st.store.CreateHuman(st.ctx, st.owner, NewHuman{DisplayName: "Second Owner", Email: "second@example.com", Role: RoleOwner})).must(t)
	if err := st.store.DisableActor(st.ctx, Caller{ActorID: other.ID}, st.owner.ActorID); err != nil {
		t.Fatal(err)
	}
	_, _, err = st.store.DownloadWithServiceToken(st.ctx, second[:])
	wantErr(t, err, ErrUnauthenticated)
}

func TestAccessLogIsNewestFirst(t *testing.T) {
	st := newSecretsTeam(t)
	st.set(t, st.memberWeb, ConfigDev, map[string]string{"A": "1", "B": "2"})
	st.reveal(t, st.memberWeb, ConfigDev, "A")
	st.reveal(t, st.memberWeb, ConfigDev, "B")

	page := got(st.store.AccessLog(st.ctx, st.memberWeb, st.project.ID, ConfigDev, 0, 1)).must(t)
	if len(page) != 1 || page[0].Names[0] != "B" {
		t.Fatalf("first page = %+v", page)
	}
	next := got(st.store.AccessLog(st.ctx, st.memberWeb, st.project.ID, ConfigDev, page[0].Sequence, 1)).must(t)
	if len(next) != 1 || next[0].Names[0] != "A" {
		t.Fatalf("second page = %+v", next)
	}
}
