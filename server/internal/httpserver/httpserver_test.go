package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/shantodotdev/super-sparrow/server/internal/auth"
	"github.com/shantodotdev/super-sparrow/server/internal/database"
	"github.com/shantodotdev/super-sparrow/server/internal/email"
	"github.com/shantodotdev/super-sparrow/server/internal/store"
	"github.com/shantodotdev/super-sparrow/server/migrations"
)

const origin = "https://sparrow.example.com"

// fakeMailer records sent messages.
type fakeMailer struct {
	mu   sync.Mutex
	sent []email.Message
}

func (m *fakeMailer) Send(_ context.Context, message email.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent = append(m.sent, message)
	return nil
}

func (m *fakeMailer) last() email.Message {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.sent) == 0 {
		return email.Message{}
	}
	return m.sent[len(m.sent)-1]
}

func (m *fakeMailer) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.sent)
}

type fixture struct {
	t       *testing.T
	handler http.Handler
	store   *store.Store
	auth    *auth.Service
	mailer  *fakeMailer
	owner   store.Actor
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	ctx := context.Background()

	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := database.Migrate(ctx, db, migrations.Files); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	st := store.New(db, time.Now)
	owner, err := st.CreateFirstOwner(ctx, "Olive Owner", "olive@example.com")
	if err != nil {
		t.Fatalf("CreateFirstOwner: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	publicURL, _ := url.Parse(origin)
	mailer := &fakeMailer{}
	service := auth.New(st, mailer, publicURL, logger, time.Now)

	handler := New(Options{Logger: logger, DB: db, Store: st, Auth: service, PublicURL: publicURL, Now: time.Now})
	return &fixture{t: t, handler: handler, store: st, auth: service, mailer: mailer, owner: owner}
}

// request sends a request with an optional JSON body and credential. A credential starting with
// ssp_ is sent as a bearer token, anything else as the session cookie.
func (f *fixture) request(method, path, credential string, body any) *httptest.ResponseRecorder {
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
	case strings.HasPrefix(credential, auth.TokenPrefix):
		request.Header.Set("Authorization", "Bearer "+credential)
	case credential != "":
		request.AddCookie(&http.Cookie{Name: "__Host-sparrow_session", Value: credential})
	}

	response := httptest.NewRecorder()
	f.handler.ServeHTTP(response, request)
	return response
}

var codePattern = regexp.MustCompile(`code is (\d{6})`)

// signIn signs address in through the API and returns the session cookie's value.
func (f *fixture) signIn(address string) string {
	f.t.Helper()

	if response := f.request(http.MethodPost, "/api/auth/code", "", map[string]string{"email": address}); response.Code != http.StatusAccepted {
		f.t.Fatalf("request code: status %d: %s", response.Code, response.Body)
	}
	f.auth.Wait()

	match := codePattern.FindStringSubmatch(f.mailer.last().Text)
	if match == nil {
		f.t.Fatalf("no code emailed to %s", address)
	}

	response := f.request(http.MethodPost, "/api/auth/verify", "", map[string]string{"email": address, "code": match[1]})
	if response.Code != http.StatusOK {
		f.t.Fatalf("verify: status %d: %s", response.Code, response.Body)
	}
	for _, cookie := range response.Result().Cookies() {
		if cookie.Name == "__Host-sparrow_session" {
			return cookie.Value
		}
	}
	f.t.Fatal("verify set no session cookie")
	return ""
}

// createToken creates a token through the API with session and returns its secret and ID.
func (f *fixture) createToken(session string) (string, string) {
	f.t.Helper()
	response := f.request(http.MethodPost, "/api/tokens", session, map[string]string{"agent_name": "claude-code", "label": "laptop"})
	if response.Code != http.StatusCreated {
		f.t.Fatalf("create token: status %d: %s", response.Code, response.Body)
	}

	var body struct {
		Secret string    `json:"secret"`
		Token  tokenView `json:"token"`
	}
	decode(f.t, response, &body)
	return body.Secret, body.Token.ID
}

func decode(t *testing.T, response *httptest.ResponseRecorder, dst any) {
	t.Helper()
	if err := json.NewDecoder(response.Body).Decode(dst); err != nil {
		t.Fatalf("decode response: %v", err)
	}
}

func wantStatus(t *testing.T, response *httptest.ResponseRecorder, status int) {
	t.Helper()
	if response.Code != status {
		t.Fatalf("status = %d, want %d: %s", response.Code, status, response.Body)
	}
}

func wantField(t *testing.T, response *httptest.ResponseRecorder, field string) {
	t.Helper()
	wantStatus(t, response, http.StatusBadRequest)
	var body errorBody
	decode(t, response, &body)
	if body.Field != field {
		t.Fatalf("error field = %q, want %q (%s)", body.Field, field, body.Error)
	}
}

func TestHealthReportsOK(t *testing.T) {
	f := newFixture(t)

	response := f.request(http.MethodGet, "/api/health", "", nil)

	wantStatus(t, response, http.StatusOK)
	var body map[string]string
	decode(t, response, &body)
	if body["status"] != "ok" {
		t.Errorf("status field = %q, want ok", body["status"])
	}
}

func TestSignInAndOut(t *testing.T) {
	f := newFixture(t)

	stranger := f.request(http.MethodPost, "/api/auth/code", "", map[string]string{"email": "stranger@example.com"})
	teammate := f.request(http.MethodPost, "/api/auth/code", "", map[string]string{"email": "olive@example.com"})
	wantStatus(t, stranger, http.StatusAccepted)
	if stranger.Body.String() != teammate.Body.String() {
		t.Errorf("answers differ: %q and %q", stranger.Body, teammate.Body)
	}
	f.auth.Wait()
	if f.mailer.count() != 1 || f.mailer.last().To != "olive@example.com" {
		t.Fatalf("emails sent = %d, last to %q", f.mailer.count(), f.mailer.last().To)
	}

	session := f.signIn("olive@example.com")
	me := f.request(http.MethodGet, "/api/me", session, nil)
	wantStatus(t, me, http.StatusOK)
	var body struct {
		Actor actorView `json:"actor"`
	}
	decode(t, me, &body)
	if body.Actor.ID != f.owner.ID {
		t.Fatalf("me = %+v", body.Actor)
	}

	signOut := f.request(http.MethodPost, "/api/auth/sign-out", session, nil)
	wantStatus(t, signOut, http.StatusNoContent)
	if cookies := signOut.Result().Cookies(); len(cookies) != 1 || cookies[0].MaxAge >= 0 {
		t.Errorf("sign-out did not clear the cookie: %+v", cookies)
	}
	wantStatus(t, f.request(http.MethodGet, "/api/me", session, nil), http.StatusUnauthorized)
}

func TestSessionCookieIsLockedDown(t *testing.T) {
	f := newFixture(t)
	f.request(http.MethodPost, "/api/auth/code", "", map[string]string{"email": "olive@example.com"})
	f.auth.Wait()
	code := codePattern.FindStringSubmatch(f.mailer.last().Text)[1]

	response := f.request(http.MethodPost, "/api/auth/verify", "", map[string]string{"email": "olive@example.com", "code": code})
	wantStatus(t, response, http.StatusOK)

	cookie := response.Result().Cookies()[0]
	if !cookie.Secure || !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode || cookie.Path != "/" {
		t.Errorf("cookie = %+v", cookie)
	}
	if lifetime := time.Duration(cookie.MaxAge) * time.Second; lifetime < store.SessionLifetime-time.Minute {
		t.Errorf("cookie lasts %v, want about %v", lifetime, store.SessionLifetime)
	}
	if strings.Contains(response.Body.String(), cookie.Value) {
		t.Error("response body contains the session ID")
	}
}

func TestWrongCodeAndBadBodiesNameTheField(t *testing.T) {
	f := newFixture(t)
	f.request(http.MethodPost, "/api/auth/code", "", map[string]string{"email": "olive@example.com"})
	f.auth.Wait()
	code := codePattern.FindStringSubmatch(f.mailer.last().Text)[1]
	wrong := "000000"
	if code == wrong {
		wrong = "111111"
	}

	wantField(t, f.request(http.MethodPost, "/api/auth/verify", "", map[string]string{"email": "olive@example.com", "code": wrong}), "code")
	wantField(t, f.request(http.MethodPost, "/api/auth/verify", "", map[string]string{"email": "olive@example.com", "code": "12"}), "code")
	wantField(t, f.request(http.MethodPost, "/api/auth/code", "", map[string]string{"email": "not an email"}), "email")
	wantField(t, f.request(http.MethodPost, "/api/auth/code", "", map[string]any{"email": "olive@example.com", "admin": true}), "admin")
	wantField(t, f.request(http.MethodPost, "/api/auth/code", "", map[string]any{"email": 5}), "email")
	wantField(t, f.request(http.MethodPost, "/api/auth/code", "", nil), "body")
}

func TestCodeRequestsAreRateLimited(t *testing.T) {
	f := newFixture(t)

	var response *httptest.ResponseRecorder
	for range 6 {
		response = f.request(http.MethodPost, "/api/auth/code", "", map[string]string{"email": "olive@example.com"})
	}
	f.auth.Wait()

	wantStatus(t, response, http.StatusTooManyRequests)
	if response.Header().Get("Retry-After") == "" {
		t.Error("no Retry-After header")
	}
}

func TestChangesFromAnotherOriginAreRefused(t *testing.T) {
	f := newFixture(t)

	request := httptest.NewRequest(http.MethodPost, "/api/auth/code", strings.NewReader(`{"email":"olive@example.com"}`))
	request.Header.Set("Origin", "https://evil.example.com")
	response := httptest.NewRecorder()
	f.handler.ServeHTTP(response, request)

	wantStatus(t, response, http.StatusForbidden)
}

func TestSignedOutRequestsAreRefused(t *testing.T) {
	f := newFixture(t)

	wantStatus(t, f.request(http.MethodGet, "/api/me", "", nil), http.StatusUnauthorized)
	wantStatus(t, f.request(http.MethodGet, "/api/me", "not-a-session", nil), http.StatusUnauthorized)
	wantStatus(t, f.request(http.MethodGet, "/api/me", "ssp_not-a-token", nil), http.StatusUnauthorized)
}

func TestSessionsListAndSignOutEverywhere(t *testing.T) {
	f := newFixture(t)
	laptop := f.signIn("olive@example.com")
	desktop := f.signIn("olive@example.com")

	response := f.request(http.MethodGet, "/api/sessions", laptop, nil)
	wantStatus(t, response, http.StatusOK)
	var body struct {
		Sessions []sessionView `json:"sessions"`
	}
	decode(t, response, &body)
	current := 0
	for _, session := range body.Sessions {
		if session.Current {
			current++
		}
		if session.DeviceLabel != "Firefox on Linux" {
			t.Errorf("device label = %q", session.DeviceLabel)
		}
	}
	if len(body.Sessions) != 2 || current != 1 {
		t.Fatalf("sessions = %+v, want 2 with one current", body.Sessions)
	}

	wantStatus(t, f.request(http.MethodDelete, "/api/sessions", laptop, nil), http.StatusOK)
	wantStatus(t, f.request(http.MethodGet, "/api/me", desktop, nil), http.StatusUnauthorized)
}

func TestAgentTokensActAsTheirAgent(t *testing.T) {
	f := newFixture(t)
	session := f.signIn("olive@example.com")
	secret, tokenID := f.createToken(session)

	me := f.request(http.MethodGet, "/api/me", secret, nil)
	wantStatus(t, me, http.StatusOK)
	var body struct {
		Actor actorView `json:"actor"`
	}
	decode(t, me, &body)
	if body.Actor.Kind != "agent" || body.Actor.OwnerActorID != f.owner.ID || body.Actor.DisplayName != "Claude Code (Olive Owner)" {
		t.Fatalf("token acts as %+v", body.Actor)
	}

	list := f.request(http.MethodGet, "/api/tokens", session, nil)
	wantStatus(t, list, http.StatusOK)
	if strings.Contains(list.Body.String(), strings.TrimPrefix(secret, auth.TokenPrefix)) {
		t.Error("token list contains the secret")
	}

	wantStatus(t, f.request(http.MethodPost, "/api/tokens/"+tokenID+"/renew", session, nil), http.StatusOK)
	wantStatus(t, f.request(http.MethodDelete, "/api/tokens/"+tokenID, session, nil), http.StatusNoContent)
	wantStatus(t, f.request(http.MethodGet, "/api/me", secret, nil), http.StatusUnauthorized)
}

func TestAgentsAreRefusedPeopleActions(t *testing.T) {
	f := newFixture(t)
	session := f.signIn("olive@example.com")
	secret, tokenID := f.createToken(session)

	refused := []struct {
		method, path string
		body         any
	}{
		{http.MethodPost, "/api/tokens", map[string]string{"agent_name": "codex", "label": "sneaky"}},
		{http.MethodPost, "/api/tokens/" + tokenID + "/renew", nil},
		{http.MethodDelete, "/api/tokens/" + tokenID, nil},
		{http.MethodGet, "/api/tokens", nil},
		{http.MethodPost, "/api/people", map[string]string{"display_name": "Eve", "email": "eve@example.com", "role": "owner"}},
		{http.MethodPost, "/api/people/" + f.owner.ID + "/disable", nil},
		{http.MethodGet, "/api/sessions", nil},
		{http.MethodPost, "/api/auth/sign-out", nil},
	}
	for _, call := range refused {
		t.Run(call.method+" "+call.path, func(t *testing.T) {
			wantStatus(t, f.request(call.method, call.path, secret, call.body), http.StatusForbidden)
		})
	}

	people := f.request(http.MethodGet, "/api/people", secret, nil)
	wantStatus(t, people, http.StatusOK)
	if strings.Contains(people.Body.String(), "olive@example.com") {
		t.Error("agents can read teammates' email addresses")
	}
}

func TestOnlyOwnersInviteAndDisable(t *testing.T) {
	f := newFixture(t)
	owner := f.signIn("olive@example.com")

	invite := f.request(http.MethodPost, "/api/people", owner, map[string]string{"display_name": "Max Member", "email": "max@example.com", "role": "member"})
	wantStatus(t, invite, http.StatusCreated)
	var body struct {
		Actor          actorView `json:"actor"`
		InvitationSent bool      `json:"invitation_sent"`
	}
	decode(t, invite, &body)
	if !body.InvitationSent || f.mailer.last().To != "max@example.com" {
		t.Fatalf("invitation = %+v, last email to %q", body, f.mailer.last().To)
	}

	member := f.signIn("max@example.com")
	memberToken, _ := f.createToken(member)
	wantStatus(t, f.request(http.MethodPost, "/api/people", member,
		map[string]string{"display_name": "Eve", "email": "eve@example.com", "role": "member"}), http.StatusForbidden)
	wantStatus(t, f.request(http.MethodGet, "/api/tokens?all=true", member, nil), http.StatusForbidden)
	wantStatus(t, f.request(http.MethodGet, "/api/tokens?all=true", owner, nil), http.StatusOK)

	wantStatus(t, f.request(http.MethodPost, "/api/people/"+body.Actor.ID+"/disable", owner, nil), http.StatusNoContent)
	wantStatus(t, f.request(http.MethodGet, "/api/me", member, nil), http.StatusUnauthorized)
	wantStatus(t, f.request(http.MethodGet, "/api/me", memberToken, nil), http.StatusUnauthorized)
}

func TestTokenRequestsAreRateLimited(t *testing.T) {
	f := newFixture(t)
	secret, _ := f.createToken(f.signIn("olive@example.com"))

	for i := range tokenRequestsPerMinute {
		if response := f.request(http.MethodGet, "/api/me", secret, nil); response.Code != http.StatusOK {
			t.Fatalf("request %d: status %d", i+1, response.Code)
		}
	}

	response := f.request(http.MethodGet, "/api/me", secret, nil)
	wantStatus(t, response, http.StatusTooManyRequests)
	if response.Header().Get("Retry-After") == "" {
		t.Error("no Retry-After header")
	}
}

func TestClientIP(t *testing.T) {
	trusted := []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}
	cases := []struct {
		name, remote, forwarded, want string
	}{
		{"direct", "203.0.113.7:5000", "", "203.0.113.7"},
		{"untrusted peer cannot forward", "203.0.113.7:5000", "198.51.100.1", "203.0.113.7"},
		{"trusted proxy", "10.0.0.2:5000", "198.51.100.1", "198.51.100.1"},
		{"spoofed entry left of the real one", "10.0.0.2:5000", "1.1.1.1, 198.51.100.1", "198.51.100.1"},
		{"chain of trusted proxies", "10.0.0.2:5000", "198.51.100.1, 10.0.0.3", "198.51.100.1"},
		{"garbage stops the walk", "10.0.0.2:5000", "nonsense", "10.0.0.2"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/", nil)
			request.RemoteAddr = c.remote
			if c.forwarded != "" {
				request.Header.Set("X-Forwarded-For", c.forwarded)
			}
			if got := clientIP(request, trusted); got != c.want {
				t.Errorf("clientIP = %q, want %q", got, c.want)
			}
		})
	}
}

func TestDeviceLabel(t *testing.T) {
	cases := map[string]string{
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0 Safari/537.36 Edg/140.0":   "Edge on Windows",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 14_5) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Safari/605.1.15":      "Safari on macOS",
		"Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Safari/604": "Safari on iOS",
		"Mozilla/5.0 (Linux; Android 15) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0 Mobile Safari/537.36":                "Chrome on Android",
		"curl/8.9.1": "Unknown device",
	}

	for userAgent, want := range cases {
		if got := deviceLabel(userAgent); got != want {
			t.Errorf("deviceLabel(%q) = %q, want %q", userAgent, got, want)
		}
	}
}

func TestUnexpectedErrorsAreHidden(t *testing.T) {
	response := httptest.NewRecorder()
	s := &server{logger: slog.New(slog.NewTextHandler(io.Discard, nil))}

	s.writeError(response, httptest.NewRequest(http.MethodGet, "/", nil), errors.New("disk on fire at /var/db"))

	wantStatus(t, response, http.StatusInternalServerError)
	if strings.Contains(response.Body.String(), "disk") {
		t.Errorf("response leaks the error: %s", response.Body)
	}
}
