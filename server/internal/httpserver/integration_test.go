package httpserver

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
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

const webhookSecret = "a-long-enough-webhook-secret"

// withOptions rebuilds the fixture's handler with the frontend folder and webhook secret set.
func (a *apiTeam) withOptions(webDir string) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	publicURL, _ := url.Parse(origin)
	a.handler = New(Options{
		Logger: logger, Store: a.store, Auth: a.auth, PublicURL: publicURL, Now: time.Now,
		WebDir: webDir, GitHubWebhookSecret: webhookSecret,
	})
}

func TestFrontendServesFilesAndFallsBackToIndex(t *testing.T) {
	a := newAPITeam(t)
	dir := t.TempDir()
	for name, content := range map[string]string{
		ShellFile: "<!doctype html><title>app</title>", "assets/app-1a2b.js": "console.log(1)", ".env": "SECRET=1",
	} {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	a.withOptions(dir)

	for _, check := range []struct {
		path, want, cache string
		status            int
	}{
		{"/", "<title>app</title>", "no-cache", http.StatusOK},
		{"/WP-142/reviews/2", "<title>app</title>", "no-cache", http.StatusOK},
		{"/assets/app-1a2b.js", "console.log(1)", "immutable", http.StatusOK},
		{"/.env", "<title>app</title>", "no-cache", http.StatusOK},
		{"/missing.js", "", "", http.StatusNotFound},
	} {
		response := a.request(http.MethodGet, check.path, "", nil)
		if response.Code != check.status || !strings.Contains(response.Body.String(), check.want) ||
			!strings.Contains(response.Header().Get("Cache-Control"), check.cache) {
			t.Errorf("GET %s: status %d, cache %q, body %q", check.path, response.Code, response.Header().Get("Cache-Control"), response.Body)
		}
	}

	api := a.request(http.MethodGet, "/api/nothing-here", a.member, nil)
	if api.Code != http.StatusNotFound || !strings.Contains(api.Header().Get("Content-Type"), "json") {
		t.Errorf("unknown API path: status %d, type %q", api.Code, api.Header().Get("Content-Type"))
	}
}

func (a *apiTeam) deliver(event, body, signature string) *httptest.ResponseRecorder {
	a.t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/api/webhooks/github", strings.NewReader(body))
	request.Header.Set("X-GitHub-Event", event)
	request.Header.Set("X-Hub-Signature-256", signature)
	response := httptest.NewRecorder()
	a.handler.ServeHTTP(response, request)
	return response
}

func sign(body string) string {
	mac := hmac.New(sha256.New, []byte(webhookSecret))
	mac.Write([]byte(body))
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func TestGitHubWebhookClosesTheIssueOnMerge(t *testing.T) {
	a := newAPITeam(t)
	a.project()
	a.withOptions("")

	merged := `{"action":"closed","pull_request":{"html_url":"https://github.com/o/r/pull/3","merged":true,"head":{"ref":"feat/wp-1-meta"}}}`
	wantStatus(t, a.deliver("pull_request", merged, "sha256=00"), http.StatusUnauthorized)
	wantStatus(t, a.deliver("pull_request", merged, sign(merged+" ")), http.StatusUnauthorized)

	unmerged := strings.Replace(merged, `"merged":true`, `"merged":false`, 1)
	wantStatus(t, a.deliver("pull_request", unmerged, sign(unmerged)), http.StatusAccepted)
	wantStatus(t, a.deliver("ping", "{}", sign("{}")), http.StatusOK)

	response := a.deliver("pull_request", merged, sign(merged))
	wantStatus(t, response, http.StatusOK)
	if !strings.Contains(response.Body.String(), `"result":"closed"`) {
		t.Fatalf("delivery answered %s", response.Body)
	}

	var issue struct {
		Issue issueView `json:"issue"`
	}
	a.call(http.MethodGet, "/api/issues/WP-1", a.member, nil, http.StatusOK, &issue)
	if issue.Issue.Status != "done" || issue.Issue.PullRequestURL != "https://github.com/o/r/pull/3" {
		t.Fatalf("issue after merge %+v", issue.Issue)
	}

	other := `{"action":"closed","pull_request":{"html_url":"https://github.com/o/r/pull/4","merged":true,"head":{"ref":"chore/deps"}}}`
	wantStatus(t, a.deliver("pull_request", other, sign(other)), http.StatusAccepted)
}
