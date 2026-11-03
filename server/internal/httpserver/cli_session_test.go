package httpserver

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// requestWith sends a request like fixture.request, plus extra headers.
func (f *fixture) requestWith(method, path, credential string, headers map[string]string, body any) *httptest.ResponseRecorder {
	f.t.Helper()

	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			f.t.Fatalf("encode body: %v", err)
		}
		reader = bytes.NewReader(encoded)
	}

	request := httptest.NewRequest(method, path, reader)
	request.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64; rv:140.0) Gecko/20100101 Firefox/140.0")
	switch {
	case strings.HasPrefix(credential, "ssp_"), strings.HasPrefix(credential, "ssc_"), strings.HasPrefix(credential, "sst_"):
		request.Header.Set("Authorization", "Bearer "+credential)
	case credential != "":
		request.AddCookie(&http.Cookie{Name: "__Host-sparrow_session", Value: credential})
	}
	for name, value := range headers {
		request.Header.Set(name, value)
	}

	response := httptest.NewRecorder()
	f.handler.ServeHTTP(response, request)
	return response
}

// cliSignIn signs address in as the CLI and returns the ssc_ token.
func (f *fixture) cliSignIn(address string) string {
	f.t.Helper()

	if response := f.request(http.MethodPost, "/api/auth/code", "", map[string]string{"email": address}); response.Code != http.StatusAccepted {
		f.t.Fatalf("request code: status %d: %s", response.Code, response.Body)
	}
	f.auth.Wait()

	match := codePattern.FindStringSubmatch(f.mailer.last().Text)
	if match == nil {
		f.t.Fatalf("no code emailed to %s", address)
	}

	response := f.request(http.MethodPost, "/api/auth/cli/verify", "", map[string]string{"email": address, "code": match[1], "device_label": "test laptop"})
	wantStatus(f.t, response, http.StatusOK)

	var body struct {
		Token string `json:"token"`
	}
	decode(f.t, response, &body)
	return body.Token
}

// meOf returns the actor GET /api/me reports for the request.
func (f *fixture) meOf(credential string, headers map[string]string) actorView {
	f.t.Helper()
	response := f.requestWith(http.MethodGet, "/api/me", credential, headers, nil)
	wantStatus(f.t, response, http.StatusOK)

	var body struct {
		Actor actorView `json:"actor"`
	}
	decode(f.t, response, &body)
	return body.Actor
}

func TestCLISignInIssuesABearerTokenAndNoCookie(t *testing.T) {
	f := newFixture(t)

	if response := f.request(http.MethodPost, "/api/auth/code", "", map[string]string{"email": "olive@example.com"}); response.Code != http.StatusAccepted {
		t.Fatalf("request code: %d", response.Code)
	}
	f.auth.Wait()
	code := codePattern.FindStringSubmatch(f.mailer.last().Text)[1]

	response := f.request(http.MethodPost, "/api/auth/cli/verify", "", map[string]string{"email": "olive@example.com", "code": code, "device_label": "work laptop"})
	wantStatus(t, response, http.StatusOK)
	if cookies := response.Result().Cookies(); len(cookies) != 0 {
		t.Fatalf("CLI sign-in set cookies: %v", cookies)
	}

	var body struct {
		Token   string      `json:"token"`
		Session sessionView `json:"session"`
		Actor   actorView   `json:"actor"`
	}
	decode(t, response, &body)
	if !strings.HasPrefix(body.Token, "ssc_") || len(body.Token) != len("ssc_")+43 {
		t.Fatalf("token %q is not ssc_ plus 43 characters", body.Token)
	}
	if body.Session.Client != "cli" || body.Session.DeviceLabel != "work laptop" || !body.Session.Current {
		t.Fatalf("session %+v", body.Session)
	}
	if body.Actor.ID != f.owner.ID {
		t.Fatalf("actor %+v, want the owner", body.Actor)
	}

	// The code is single use.
	again := f.request(http.MethodPost, "/api/auth/cli/verify", "", map[string]string{"email": "olive@example.com", "code": code, "device_label": "x"})
	if again.Code == http.StatusOK {
		t.Fatalf("a used code signed in again")
	}

	if me := f.meOf(body.Token, nil); me.ID != f.owner.ID || me.Kind != "human" {
		t.Fatalf("me %+v, want the owner", me)
	}
}

func TestCLITokenAndBrowserCookieAreNotInterchangeable(t *testing.T) {
	f := newFixture(t)
	cookie := f.signIn("olive@example.com")
	cli := f.cliSignIn("olive@example.com")

	// The ssc_ value sent as a cookie is not a browser session.
	wantStatus(t, f.request(http.MethodGet, "/api/me", cli, nil), http.StatusOK)
	asCookie := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	asCookie.AddCookie(&http.Cookie{Name: "__Host-sparrow_session", Value: cli})
	recorder := httptest.NewRecorder()
	f.handler.ServeHTTP(recorder, asCookie)
	wantStatus(t, recorder, http.StatusUnauthorized)

	// A browser cookie value sent as an ssc_ bearer is not a CLI session.
	asBearer := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	asBearer.Header.Set("Authorization", "Bearer ssc_"+cookie)
	recorder = httptest.NewRecorder()
	f.handler.ServeHTTP(recorder, asBearer)
	wantStatus(t, recorder, http.StatusUnauthorized)

	asRawBearer := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	asRawBearer.Header.Set("Authorization", "Bearer "+cookie)
	recorder = httptest.NewRecorder()
	f.handler.ServeHTTP(recorder, asRawBearer)
	wantStatus(t, recorder, http.StatusUnauthorized)
}

func TestSessionsListTheClient(t *testing.T) {
	f := newFixture(t)
	cookie := f.signIn("olive@example.com")
	cli := f.cliSignIn("olive@example.com")

	for _, credential := range []string{cookie, cli} {
		response := f.request(http.MethodGet, "/api/sessions", credential, nil)
		wantStatus(t, response, http.StatusOK)

		var body struct {
			Sessions []sessionView `json:"sessions"`
		}
		decode(t, response, &body)

		clients := map[string]bool{}
		current := ""
		for _, session := range body.Sessions {
			clients[session.Client] = true
			if session.Current {
				current = session.Client
			}
		}
		want := "browser"
		if credential == cli {
			want = "cli"
		}
		if !clients["browser"] || !clients["cli"] || current != want {
			t.Fatalf("sessions %+v, current client %q, want %q", body.Sessions, current, want)
		}
	}
}

func TestCLISignOutRevokesTheSession(t *testing.T) {
	f := newFixture(t)
	cli := f.cliSignIn("olive@example.com")

	wantStatus(t, f.request(http.MethodPost, "/api/auth/sign-out", cli, nil), http.StatusNoContent)
	wantStatus(t, f.request(http.MethodGet, "/api/me", cli, nil), http.StatusUnauthorized)
	wantStatus(t, f.request(http.MethodGet, "/api/sessions", cli, nil), http.StatusUnauthorized)
}

func TestSparrowAgentHeaderActsAsTheOwnersAgent(t *testing.T) {
	f := newFixture(t)
	cli := f.cliSignIn("olive@example.com")
	header := map[string]string{"Sparrow-Agent": "claude-code"}

	first := f.meOf(cli, header)
	if first.Kind != "agent" || first.AgentName != "claude-code" || first.OwnerActorID != f.owner.ID {
		t.Fatalf("agent %+v, want olive's claude-code", first)
	}
	if second := f.meOf(cli, header); second.ID != first.ID {
		t.Fatalf("second call made agent %s, first was %s", second.ID, first.ID)
	}

	codex := f.meOf(cli, map[string]string{"Sparrow-Agent": "codex"})
	if codex.ID == first.ID || codex.AgentName != "codex" {
		t.Fatalf("codex %+v", codex)
	}
	if person := f.meOf(cli, nil); person.ID != f.owner.ID {
		t.Fatalf("without the header the request acted as %+v", person)
	}
}

func TestSparrowAgentHeaderRejectsUnknownAgents(t *testing.T) {
	f := newFixture(t)
	cli := f.cliSignIn("olive@example.com")

	response := f.requestWith(http.MethodGet, "/api/me", cli, map[string]string{"Sparrow-Agent": "aider"}, nil)
	wantField(t, response, "Sparrow-Agent")
}

func TestAgentHeaderMakesHumanOnlyRoutesForbidden(t *testing.T) {
	f := newFixture(t)
	cli := f.cliSignIn("olive@example.com")

	wantStatus(t, f.request(http.MethodGet, "/api/sessions", cli, nil), http.StatusOK)
	wantStatus(t, f.requestWith(http.MethodGet, "/api/sessions", cli, map[string]string{"Sparrow-Agent": "claude-code"}, nil), http.StatusForbidden)
	wantStatus(t, f.requestWith(http.MethodPost, "/api/auth/sign-out", cli, map[string]string{"Sparrow-Agent": "codex"}, nil), http.StatusForbidden)

	// The refused sign-out left the session alive.
	wantStatus(t, f.request(http.MethodGet, "/api/me", cli, nil), http.StatusOK)
}

func TestAgentHeaderIsIgnoredWithAnAgentToken(t *testing.T) {
	f := newFixture(t)
	cookie := f.signIn("olive@example.com")
	secret, _ := f.createToken(cookie)

	plain := f.meOf(secret, nil)
	withHeader := f.meOf(secret, map[string]string{"Sparrow-Agent": "codex"})
	if plain.ID != withHeader.ID || withHeader.AgentName != "claude-code" {
		t.Fatalf("token agent %+v became %+v with the header", plain, withHeader)
	}

	// An unknown name is ignored too, not rejected.
	wantStatus(t, f.requestWith(http.MethodGet, "/api/me", secret, map[string]string{"Sparrow-Agent": "aider"}, nil), http.StatusOK)
}

func TestMCPAcceptsCLISessionsButNotBrowserCookies(t *testing.T) {
	f := newFixture(t)
	cookie := f.signIn("olive@example.com")
	cli := f.cliSignIn("olive@example.com")
	list := map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/list"}

	for _, headers := range []map[string]string{nil, {"Sparrow-Agent": "claude-code"}} {
		response := f.requestWith(http.MethodPost, "/mcp", cli, headers, list)
		wantStatus(t, response, http.StatusOK)

		var body struct {
			Result struct {
				Tools []map[string]any `json:"tools"`
			} `json:"result"`
		}
		decode(t, response, &body)
		if len(body.Result.Tools) == 0 {
			t.Fatalf("tools/list with headers %v returned no tools", headers)
		}
	}

	wantStatus(t, f.request(http.MethodPost, "/mcp", cookie, list), http.StatusForbidden)
	wantStatus(t, f.request(http.MethodPost, "/mcp", "", list), http.StatusUnauthorized)
}

func TestAgentTokensOnlyForKnownAgents(t *testing.T) {
	f := newFixture(t)
	cookie := f.signIn("olive@example.com")

	wantField(t, f.request(http.MethodPost, "/api/tokens", cookie, map[string]string{"agent_name": "aider", "label": "x"}), "agent_name")
	for _, name := range []string{"claude-code", "codex"} {
		wantStatus(t, f.request(http.MethodPost, "/api/tokens", cookie, map[string]string{"agent_name": name, "label": name}), http.StatusCreated)
	}
}

// newFixtureWithCLIDir builds a fixture whose handler offers dir as the CLI release.
func newFixtureWithCLIDir(t *testing.T, dir string) *fixture {
	t.Helper()
	f := newFixture(t)

	publicURL, _ := url.Parse(origin)
	f.handler = New(Options{
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Store: f.store, Auth: f.auth,
		PublicURL: publicURL, Now: time.Now, CLIDir: dir,
	})
	return f
}

func TestCLIFilesAreNotFoundWithoutADirectory(t *testing.T) {
	f := newFixture(t)

	response := f.request(http.MethodGet, "/cli/manifest.json", "", nil)
	wantStatus(t, response, http.StatusNotFound)
	if contentType := response.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "application/json") {
		t.Fatalf("content type %q, want JSON", contentType)
	}
	var body errorBody
	decode(t, response, &body)
	if body.Error == "" {
		t.Fatal("404 carried no error message")
	}
}

func TestCLIFilesAreServedFromTheDirectory(t *testing.T) {
	dir := t.TempDir()
	for name, content := range map[string]string{"manifest.json": `{"version":"v1.0.0"}`, "sparrow-linux-amd64": "binary", "other.txt": "secret"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	f := newFixtureWithCLIDir(t, dir)

	response := f.request(http.MethodGet, "/cli/manifest.json", "", nil)
	wantStatus(t, response, http.StatusOK)
	if response.Body.String() != `{"version":"v1.0.0"}` {
		t.Fatalf("manifest %q", response.Body)
	}
	if response.Header().Get("Cache-Control") != "no-cache" {
		t.Fatalf("Cache-Control %q", response.Header().Get("Cache-Control"))
	}

	binary := f.request(http.MethodGet, "/cli/sparrow-linux-amd64", "", nil)
	wantStatus(t, binary, http.StatusOK)
	if binary.Body.String() != "binary" {
		t.Fatalf("binary %q", binary.Body)
	}

	for _, name := range []string{"..%2Fx", "other.txt", "sparrow-plan9-amd64", "manifest.json.sig", "sparrow-darwin-arm64"} {
		wantStatus(t, f.request(http.MethodGet, "/cli/"+name, "", nil), http.StatusNotFound)
	}
}
