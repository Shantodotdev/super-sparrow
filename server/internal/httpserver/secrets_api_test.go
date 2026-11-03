package httpserver

import (
	"net/http"
	"slices"
	"strings"
	"testing"
)

// secretsTeam is an apiTeam with project WP, its secrets turned on, and CLI sessions for both people.
type secretsTeam struct {
	*apiTeam
	ownerCLI  string
	memberCLI string
	memberID  string
}

type listingBody struct {
	Secrets []struct {
		Name      string `json:"name"`
		Source    string `json:"source"`
		Version   int    `json:"version"`
		UpdatedBy string `json:"updated_by"`
	} `json:"secrets"`
	Required []string `json:"required"`
	Missing  []string `json:"missing"`
}

func (l listingBody) names() []string {
	names := []string{}
	for _, secret := range l.Secrets {
		names = append(names, secret.Name)
	}
	return names
}

func newSecretsTeam(t *testing.T) *secretsTeam {
	t.Helper()
	a := newAPITeam(t)
	a.project()
	a.call(http.MethodPost, "/api/projects/WP/secrets/enable", a.owner, nil, http.StatusOK, nil)

	team := &secretsTeam{apiTeam: a, ownerCLI: a.cliSignIn("olive@example.com"), memberCLI: a.cliSignIn("max@example.com")}
	for _, person := range team.people() {
		if person.Email == "max@example.com" {
			team.memberID = person.ID
		}
	}
	if team.memberID == "" {
		t.Fatal("member not in the people list")
	}
	return team
}

func (s *secretsTeam) people() []actorView {
	s.t.Helper()
	var body struct {
		Actors []actorView `json:"actors"`
	}
	s.call(http.MethodGet, "/api/people", s.owner, nil, http.StatusOK, &body)
	return body.Actors
}

// patch changes keys of config and returns the listing.
func (s *secretsTeam) patch(config, credential string, headers map[string]string, set map[string]string, del []string, status int) listingBody {
	s.t.Helper()
	response := s.requestWith(http.MethodPatch, "/api/projects/WP/secrets/"+config, credential, headers, map[string]any{"set": set, "delete": del})
	wantStatus(s.t, response, status)

	var listing listingBody
	if status == http.StatusOK {
		decode(s.t, response, &listing)
	}
	return listing
}

func TestSecretsAreOffUntilAnOwnerEnablesThem(t *testing.T) {
	a := newAPITeam(t)
	a.project()

	var before struct {
		Enabled   bool           `json:"enabled"`
		CanManage bool           `json:"can_manage"`
		IsOwner   bool           `json:"is_owner"`
		Configs   map[string]any `json:"configs"`
	}
	a.call(http.MethodGet, "/api/projects/WP/secrets", a.owner, nil, http.StatusOK, &before)
	if before.Enabled || !before.CanManage || !before.IsOwner || before.Configs != nil {
		t.Fatalf("before enabling: %+v", before)
	}

	wantStatus(t, a.request(http.MethodPost, "/api/projects/WP/secrets/enable", a.member, nil), http.StatusForbidden)
	wantStatus(t, a.request(http.MethodPost, "/api/projects/WP/secrets/enable", a.agent, nil), http.StatusForbidden)

	var enabled struct {
		Project projectView `json:"project"`
	}
	a.call(http.MethodPost, "/api/projects/WP/secrets/enable", a.owner, nil, http.StatusOK, &enabled)
	if !enabled.Project.SecretsEnabled {
		t.Fatalf("project %+v", enabled.Project)
	}

	var after struct {
		Enabled bool                   `json:"enabled"`
		Configs map[string]listingBody `json:"configs"`
	}
	a.call(http.MethodGet, "/api/projects/WP/secrets", a.member, nil, http.StatusOK, &after)
	if !after.Enabled || len(after.Configs) != 4 {
		t.Fatalf("after enabling: %+v", after)
	}
	for _, config := range []string{"dev", "dev-defaults", "staging", "production"} {
		if _, ok := after.Configs[config]; !ok {
			t.Errorf("config %s missing", config)
		}
	}

	// A member sees the shape without permissions, and agents see key names too.
	var member struct {
		CanManage bool `json:"can_manage"`
		IsOwner   bool `json:"is_owner"`
	}
	a.call(http.MethodGet, "/api/projects/WP/secrets", a.member, nil, http.StatusOK, &member)
	if member.CanManage || member.IsOwner {
		t.Fatalf("member overview %+v", member)
	}
	wantStatus(t, a.request(http.MethodGet, "/api/projects/WP/secrets", a.agent, nil), http.StatusOK)
}

func TestStagingNeedsPermissionAndABrowser(t *testing.T) {
	s := newSecretsTeam(t)

	listing := s.patch("staging", s.owner, nil, map[string]string{"DB_URL": "postgres://x"}, nil, http.StatusOK)
	if !slices.Equal(listing.names(), []string{"DB_URL"}) {
		t.Fatalf("listing %+v", listing)
	}

	response := s.request(http.MethodGet, "/api/projects/WP/secrets/staging/values/DB_URL", s.owner, nil)
	wantStatus(t, response, http.StatusOK)
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("Cache-Control %q", response.Header().Get("Cache-Control"))
	}
	var value struct {
		Name    string `json:"name"`
		Value   string `json:"value"`
		Version int    `json:"version"`
	}
	decode(t, response, &value)
	if value.Name != "DB_URL" || value.Value != "postgres://x" || value.Version != 1 {
		t.Fatalf("value %+v", value)
	}

	// The owner in the CLI cannot read or download staging, or change it.
	wantStatus(t, s.request(http.MethodGet, "/api/projects/WP/secrets/staging/values/DB_URL", s.ownerCLI, nil), http.StatusForbidden)
	wantStatus(t, s.request(http.MethodGet, "/api/projects/WP/secrets/staging/download?format=json", s.ownerCLI, nil), http.StatusForbidden)
	s.patch("staging", s.ownerCLI, nil, map[string]string{"X": "1"}, nil, http.StatusForbidden)

	// Agents cannot either, and key names stay visible to them.
	wantStatus(t, s.request(http.MethodGet, "/api/projects/WP/secrets/staging/values/DB_URL", s.agent, nil), http.StatusForbidden)

	// A member without the permission is refused.
	wantStatus(t, s.request(http.MethodGet, "/api/projects/WP/secrets/staging/values/DB_URL", s.member, nil), http.StatusForbidden)
	s.patch("staging", s.member, nil, map[string]string{"X": "1"}, nil, http.StatusForbidden)
	wantStatus(t, s.request(http.MethodGet, "/api/projects/WP/secrets/staging/download?format=json", s.member, nil), http.StatusForbidden)
}

func TestOwnerGrantsMembersTheEnvironmentsPermission(t *testing.T) {
	s := newSecretsTeam(t)
	s.patch("staging", s.owner, nil, map[string]string{"DB_URL": "postgres://x"}, nil, http.StatusOK)

	path := "/api/people/" + s.memberID + "/environments"
	wantStatus(t, s.request(http.MethodPatch, path, s.member, map[string]bool{"can_manage_environments": true}), http.StatusForbidden)
	wantStatus(t, s.request(http.MethodPatch, path, s.ownerCLI, map[string]bool{"can_manage_environments": true}), http.StatusForbidden)
	wantField(t, s.request(http.MethodPatch, path, s.owner, map[string]string{}), "can_manage_environments")

	var granted struct {
		Person actorView `json:"person"`
	}
	s.call(http.MethodPatch, path, s.owner, map[string]bool{"can_manage_environments": true}, http.StatusOK, &granted)
	if granted.Person.CanManageEnvironments == nil || !*granted.Person.CanManageEnvironments {
		t.Fatalf("person %+v", granted.Person)
	}

	// The member can now use staging in a browser, but still not in the CLI.
	wantStatus(t, s.request(http.MethodGet, "/api/projects/WP/secrets/staging/values/DB_URL", s.member, nil), http.StatusOK)
	s.patch("staging", s.member, nil, map[string]string{"X": "1"}, nil, http.StatusOK)
	wantStatus(t, s.request(http.MethodGet, "/api/projects/WP/secrets/staging/values/DB_URL", s.memberCLI, nil), http.StatusForbidden)

	// Agents never manage environments, so the list shows the flag for people only.
	agentSeen := false
	for _, person := range s.people() {
		switch person.Kind {
		case "human":
			if person.CanManageEnvironments == nil {
				t.Errorf("human %s lacks can_manage_environments", person.Email)
			}
			if person.Email == "max@example.com" && !*person.CanManageEnvironments {
				t.Errorf("member's permission not listed")
			}
		case "agent":
			agentSeen = true
			if person.CanManageEnvironments != nil {
				t.Errorf("agent %s has can_manage_environments", person.DisplayName)
			}
		}
	}
	if !agentSeen {
		t.Fatal("no agent in the people list to check")
	}

	s.call(http.MethodPatch, path, s.owner, map[string]bool{"can_manage_environments": false}, http.StatusOK, nil)
	wantStatus(t, s.request(http.MethodGet, "/api/projects/WP/secrets/staging/values/DB_URL", s.member, nil), http.StatusForbidden)
}

func TestDevOverridesThroughTheCLI(t *testing.T) {
	s := newSecretsTeam(t)
	s.patch("dev-defaults", s.owner, nil, map[string]string{"A": "default-a", "B": "default-b"}, nil, http.StatusOK)

	// A person sets overrides from the CLI without an agent header.
	listing := s.patch("dev", s.ownerCLI, nil, map[string]string{"A": "mine"}, nil, http.StatusOK)
	sources := map[string]string{}
	for _, secret := range listing.Secrets {
		sources[secret.Name] = secret.Source
	}
	if sources["A"] != "override" || sources["B"] != "default" {
		t.Fatalf("sources %v", sources)
	}

	response := s.request(http.MethodGet, "/api/projects/WP/secrets/dev/values/A", s.ownerCLI, nil)
	wantStatus(t, response, http.StatusOK)
	var value struct {
		Value  string `json:"value"`
		Source string `json:"source"`
	}
	decode(t, response, &value)
	if value.Value != "mine" || value.Source != "override" {
		t.Fatalf("value %+v", value)
	}

	// Another person sees the default, not the override.
	var theirs struct {
		Value string `json:"value"`
	}
	s.call(http.MethodGet, "/api/projects/WP/secrets/dev/values/A", s.member, nil, http.StatusOK, &theirs)
	if theirs.Value != "default-a" {
		t.Fatalf("member saw %q", theirs.Value)
	}

	// Deleting the override falls back to the default.
	s.patch("dev", s.ownerCLI, nil, nil, []string{"A"}, http.StatusOK)
	s.call(http.MethodGet, "/api/projects/WP/secrets/dev/values/A", s.ownerCLI, nil, http.StatusOK, &value)
	if value.Value != "default-a" || value.Source != "default" {
		t.Fatalf("after delete %+v", value)
	}

	// The owner's agent reads dev but never changes it.
	agent := map[string]string{"Sparrow-Agent": "codex"}
	s.patch("dev", s.ownerCLI, agent, map[string]string{"A": "agent"}, nil, http.StatusForbidden)
	s.patch("dev", s.agent, nil, map[string]string{"A": "agent"}, nil, http.StatusForbidden)

	s.patch("dev", s.ownerCLI, nil, map[string]string{"A": "mine-again"}, nil, http.StatusOK)
	response = s.requestWith(http.MethodGet, "/api/projects/WP/secrets/dev/download?format=json", s.ownerCLI, agent, nil)
	wantStatus(t, response, http.StatusOK)
	var downloaded map[string]string
	decode(t, response, &downloaded)
	if downloaded["A"] != "mine-again" || downloaded["B"] != "default-b" {
		t.Fatalf("agent downloaded %v", downloaded)
	}
}

func TestDownloadFormats(t *testing.T) {
	s := newSecretsTeam(t)
	s.patch("dev-defaults", s.owner, nil, map[string]string{"PLAIN": "value", "QUOTED": "it's"}, nil, http.StatusOK)
	path := "/api/projects/WP/secrets/dev/download?format="

	response := s.request(http.MethodGet, path+"json", s.ownerCLI, nil)
	wantStatus(t, response, http.StatusOK)
	var values map[string]string
	decode(t, response, &values)
	if values["PLAIN"] != "value" || values["QUOTED"] != "it's" || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("json %v, Cache-Control %q", values, response.Header().Get("Cache-Control"))
	}

	// A .env line cannot hold a single quote, so the whole download is refused.
	wantField(t, s.request(http.MethodGet, path+"env", s.ownerCLI, nil), "format")

	shell := s.request(http.MethodGet, path+"shell", s.ownerCLI, nil)
	wantStatus(t, shell, http.StatusOK)
	if got, want := shell.Body.String(), "export PLAIN='value'\nexport QUOTED='it'\\''s'\n"; got != want {
		t.Fatalf("shell %q, want %q", got, want)
	}
	if !strings.HasPrefix(shell.Header().Get("Content-Type"), "text/plain") {
		t.Fatalf("content type %q", shell.Header().Get("Content-Type"))
	}

	s.patch("dev-defaults", s.owner, nil, nil, []string{"QUOTED"}, http.StatusOK)
	env := s.request(http.MethodGet, path+"env", s.ownerCLI, nil)
	wantStatus(t, env, http.StatusOK)
	if env.Body.String() != "PLAIN='value'\n" {
		t.Fatalf("env %q", env.Body)
	}

	wantField(t, s.request(http.MethodGet, path+"yaml", s.ownerCLI, nil), "format")
	wantField(t, s.request(http.MethodGet, "/api/projects/WP/secrets/nowhere/download?format=json", s.ownerCLI, nil), "config")
}

func TestSecretHistoryAndRestore(t *testing.T) {
	s := newSecretsTeam(t)
	for _, value := range []string{"one", "two", "three"} {
		s.patch("staging", s.owner, nil, map[string]string{"TOKEN": value}, nil, http.StatusOK)
	}

	var history struct {
		Versions []struct {
			Version int  `json:"version"`
			Deleted bool `json:"deleted"`
		} `json:"versions"`
	}
	s.call(http.MethodGet, "/api/projects/WP/secrets/staging/history/TOKEN", s.owner, nil, http.StatusOK, &history)
	if len(history.Versions) != 3 || history.Versions[0].Version != 3 || history.Versions[2].Version != 1 {
		t.Fatalf("history %+v, want newest first", history.Versions)
	}

	var listing listingBody
	s.call(http.MethodPost, "/api/projects/WP/secrets/staging/history/TOKEN/restore", s.owner, map[string]int{"version": 1}, http.StatusOK, &listing)
	if len(listing.Secrets) != 1 || listing.Secrets[0].Version != 4 {
		t.Fatalf("restored listing %+v", listing)
	}

	var value struct {
		Value string `json:"value"`
	}
	s.call(http.MethodGet, "/api/projects/WP/secrets/staging/values/TOKEN", s.owner, nil, http.StatusOK, &value)
	if value.Value != "one" {
		t.Fatalf("restored value %q", value.Value)
	}

	wantStatus(t, s.request(http.MethodPost, "/api/projects/WP/secrets/staging/history/TOKEN/restore", s.member, map[string]int{"version": 2}), http.StatusForbidden)
	wantStatus(t, s.request(http.MethodGet, "/api/projects/WP/secrets/staging/history/TOKEN", s.ownerCLI, nil), http.StatusForbidden)
}

func TestRequiredKeys(t *testing.T) {
	s := newSecretsTeam(t)
	s.patch("staging", s.owner, nil, map[string]string{"HAVE": "1"}, nil, http.StatusOK)

	var listing listingBody
	s.call(http.MethodPut, "/api/projects/WP/secrets/staging/required", s.owner, map[string][]string{"names": {"HAVE", "WANT"}}, http.StatusOK, &listing)
	if !slices.Equal(listing.Required, []string{"HAVE", "WANT"}) || !slices.Equal(listing.Missing, []string{"WANT"}) {
		t.Fatalf("listing %+v", listing)
	}

	wantStatus(t, s.request(http.MethodPut, "/api/projects/WP/secrets/staging/required", s.member, map[string][]string{"names": {"X"}}), http.StatusForbidden)
	wantStatus(t, s.request(http.MethodPut, "/api/projects/WP/secrets/staging/required", s.ownerCLI, map[string][]string{"names": {"X"}}), http.StatusForbidden)
	wantStatus(t, s.request(http.MethodPut, "/api/projects/WP/secrets/staging/required", s.owner, map[string][]string{"names": {"bad name"}}), http.StatusBadRequest)

	var overview struct {
		Configs map[string]listingBody `json:"configs"`
	}
	s.call(http.MethodGet, "/api/projects/WP/secrets", s.member, nil, http.StatusOK, &overview)
	if !slices.Equal(overview.Configs["staging"].Missing, []string{"WANT"}) {
		t.Fatalf("member sees %+v", overview.Configs["staging"])
	}
}

type accessLogBody struct {
	Entries []struct {
		ID             string   `json:"id"`
		Config         string   `json:"config"`
		Action         string   `json:"action"`
		Names          []string `json:"names"`
		ActorID        *string  `json:"actor_id"`
		ServiceTokenID *string  `json:"service_token_id"`
	} `json:"entries"`
	NextCursor string `json:"next_cursor"`
}

func TestAccessLogPagesAndScopesByViewer(t *testing.T) {
	s := newSecretsTeam(t)
	s.patch("staging", s.owner, nil, map[string]string{"K": "v"}, nil, http.StatusOK)
	s.patch("dev-defaults", s.owner, nil, map[string]string{"D": "v"}, nil, http.StatusOK)

	for range 3 {
		s.call(http.MethodGet, "/api/projects/WP/secrets/staging/values/K", s.owner, nil, http.StatusOK, nil)
	}
	s.call(http.MethodGet, "/api/projects/WP/secrets/staging/download?format=json", s.owner, nil, http.StatusOK, nil)
	s.call(http.MethodGet, "/api/projects/WP/secrets/dev/values/D", s.member, nil, http.StatusOK, nil)

	// The owner pages through staging's log, newest first.
	var first accessLogBody
	s.call(http.MethodGet, "/api/projects/WP/secrets/access-log?config=staging&limit=3", s.owner, nil, http.StatusOK, &first)
	if len(first.Entries) != 3 || first.NextCursor == "" {
		t.Fatalf("first page %+v", first)
	}

	var second accessLogBody
	s.call(http.MethodGet, "/api/projects/WP/secrets/access-log?config=staging&limit=3&cursor="+first.NextCursor, s.owner, nil, http.StatusOK, &second)
	if len(second.Entries) != 1 || second.Entries[0].Action != "reveal" || !slices.Equal(second.Entries[0].Names, []string{"K"}) ||
		second.Entries[0].Config != "staging" {
		t.Fatalf("second page %+v", second)
	}
	if first.Entries[0].Action != "download" || first.Entries[0].ActorID == nil || first.Entries[0].ServiceTokenID != nil {
		t.Fatalf("first entry %+v", first.Entries[0])
	}
	if second.NextCursor != "" {
		t.Fatalf("last page has cursor %q", second.NextCursor)
	}

	wantField(t, s.request(http.MethodGet, "/api/projects/WP/secrets/access-log?config=staging&cursor=abc", s.owner, nil), "cursor")
	wantField(t, s.request(http.MethodGet, "/api/projects/WP/secrets/access-log?config=nope", s.owner, nil), "config")

	// A member cannot see staging's log but sees the log of their own dev.
	wantStatus(t, s.request(http.MethodGet, "/api/projects/WP/secrets/access-log?config=staging", s.member, nil), http.StatusForbidden)
	var mine accessLogBody
	s.call(http.MethodGet, "/api/projects/WP/secrets/access-log?config=dev", s.member, nil, http.StatusOK, &mine)
	if len(mine.Entries) != 1 || !slices.Equal(mine.Entries[0].Names, []string{"D"}) {
		t.Fatalf("member's dev log %+v", mine)
	}

	// The owner's own dev log does not include the member's reads.
	var owners accessLogBody
	s.call(http.MethodGet, "/api/projects/WP/secrets/access-log?config=dev", s.owner, nil, http.StatusOK, &owners)
	if len(owners.Entries) != 0 {
		t.Fatalf("owner's dev log %+v", owners)
	}
}

func TestServiceTokensDownloadOnlyTheirConfig(t *testing.T) {
	s := newSecretsTeam(t)
	s.patch("staging", s.owner, nil, map[string]string{"STAGING_KEY": "s"}, nil, http.StatusOK)
	s.patch("production", s.owner, nil, map[string]string{"PROD_KEY": "p"}, nil, http.StatusOK)
	base := "/api/projects/WP/service-tokens"

	// Only a person who can manage, in a browser, creates tokens.
	wantStatus(t, s.request(http.MethodPost, base, s.member, map[string]string{"config": "staging", "label": "ci"}), http.StatusForbidden)
	wantStatus(t, s.request(http.MethodPost, base, s.ownerCLI, map[string]string{"config": "staging", "label": "ci"}), http.StatusForbidden)
	wantField(t, s.request(http.MethodPost, base, s.owner, map[string]string{"config": "dev", "label": "ci"}), "config")

	var created struct {
		Token  serviceTokenView `json:"token"`
		Secret string           `json:"secret"`
	}
	s.call(http.MethodPost, base, s.owner, map[string]string{"config": "staging", "label": "ci"}, http.StatusCreated, &created)
	if !strings.HasPrefix(created.Secret, "sst_") || created.Token.Config != "staging" || created.Token.Label != "ci" || created.Token.RevokedAt != nil {
		t.Fatalf("created %+v", created)
	}

	var downloaded map[string]string
	s.call(http.MethodGet, "/api/secrets/download?format=json", created.Secret, nil, http.StatusOK, &downloaded)
	if len(downloaded) != 1 || downloaded["STAGING_KEY"] != "s" {
		t.Fatalf("downloaded %v, want only staging", downloaded)
	}
	shell := s.request(http.MethodGet, "/api/secrets/download?format=shell", created.Secret, nil)
	wantStatus(t, shell, http.StatusOK)
	if shell.Body.String() != "export STAGING_KEY='s'\n" {
		t.Fatalf("shell %q", shell.Body)
	}
	wantField(t, s.request(http.MethodGet, "/api/secrets/download?format=bad", created.Secret, nil), "format")

	// The use is logged against the token, and the token shows it was used.
	var log accessLogBody
	s.call(http.MethodGet, "/api/projects/WP/secrets/access-log?config=staging", s.owner, nil, http.StatusOK, &log)
	if len(log.Entries) == 0 || log.Entries[len(log.Entries)-1].ServiceTokenID == nil || *log.Entries[len(log.Entries)-1].ServiceTokenID != created.Token.ID {
		t.Fatalf("log %+v", log)
	}
	var listed struct {
		Tokens []serviceTokenView `json:"tokens"`
	}
	s.call(http.MethodGet, base, s.owner, nil, http.StatusOK, &listed)
	if len(listed.Tokens) != 1 || listed.Tokens[0].ID != created.Token.ID || listed.Tokens[0].LastUsedAt == nil {
		t.Fatalf("listed %+v", listed)
	}
	if strings.Contains(s.request(http.MethodGet, base, s.owner, nil).Body.String(), created.Secret) {
		t.Fatal("the token list repeats the secret")
	}
	wantStatus(t, s.request(http.MethodGet, base, s.member, nil), http.StatusForbidden)

	// Wrong tokens and no token are refused; a token is no session.
	wantStatus(t, s.request(http.MethodGet, "/api/secrets/download?format=json", "sst_"+strings.Repeat("a", 43), nil), http.StatusUnauthorized)
	wantStatus(t, s.request(http.MethodGet, "/api/secrets/download?format=json", "", nil), http.StatusUnauthorized)
	wantStatus(t, s.request(http.MethodGet, "/api/secrets/download?format=json", s.agent, nil), http.StatusUnauthorized)
	wantStatus(t, s.request(http.MethodGet, "/api/me", created.Secret, nil), http.StatusUnauthorized)

	// Renew extends it.
	var renewed struct {
		Token serviceTokenView `json:"token"`
	}
	s.call(http.MethodPost, "/api/service-tokens/"+created.Token.ID+"/renew", s.owner, nil, http.StatusOK, &renewed)
	if renewed.Token.ID != created.Token.ID || renewed.Token.ExpiresAt.Before(created.Token.ExpiresAt) {
		t.Fatalf("renewed %+v from %+v", renewed.Token, created.Token)
	}

	// Revoking ends its access.
	wantStatus(t, s.request(http.MethodDelete, "/api/service-tokens/"+created.Token.ID, s.member, nil), http.StatusForbidden)
	wantStatus(t, s.request(http.MethodDelete, "/api/service-tokens/"+created.Token.ID, s.owner, nil), http.StatusNoContent)
	wantStatus(t, s.request(http.MethodGet, "/api/secrets/download?format=json", created.Secret, nil), http.StatusUnauthorized)

	s.call(http.MethodGet, base, s.owner, nil, http.StatusOK, &listed)
	if len(listed.Tokens) != 1 || listed.Tokens[0].RevokedAt == nil {
		t.Fatalf("listed after revoke %+v", listed)
	}
}

func TestDevSecretEventsAreVisibleOnlyToTheirPerson(t *testing.T) {
	s := newSecretsTeam(t)
	s.patch("dev", s.ownerCLI, nil, map[string]string{"MINE": "1"}, nil, http.StatusOK)
	s.patch("staging", s.owner, nil, map[string]string{"TEAM": "1"}, nil, http.StatusOK)

	kinds := func(credential string) []string {
		var body struct {
			Events []eventView `json:"events"`
		}
		s.call(http.MethodGet, "/api/events?limit=200", credential, nil, http.StatusOK, &body)
		var kinds []string
		for _, event := range body.Events {
			kinds = append(kinds, event.Kind)
		}
		return kinds
	}
	hasDev := func(kinds []string) bool {
		return slices.ContainsFunc(kinds, func(kind string) bool { return strings.HasPrefix(kind, "dev_secret.") })
	}

	if !hasDev(kinds(s.owner)) {
		t.Fatalf("the dev's owner sees %v, want a dev_secret event", kinds(s.owner))
	}
	if other := kinds(s.member); hasDev(other) {
		t.Fatalf("another person sees dev secret events: %v", other)
	}
	if other := kinds(s.agent); hasDev(other) {
		t.Fatalf("another person's agent sees dev secret events: %v", other)
	}
}
