package httpserver

import (
	"crypto/sha256"
	"encoding/base64"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/shantodotdev/super-sparrow/server/internal/github"
)

const webhookSecret = "a-long-enough-webhook-secret"

const objectOrigin = "https://objects.test"

// withOptions rebuilds the fixture's handler with the frontend folder and webhook secret set.
func (a *apiTeam) withOptions(webDir string) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	publicURL, _ := url.Parse(origin)
	service, err := github.NewService(a.store, nil, nil, logger)
	if err != nil {
		a.t.Fatalf("NewService: %v", err)
	}
	a.handler = New(Options{
		Logger: logger, Store: a.store, Auth: a.auth, PublicURL: publicURL, Now: time.Now,
		WebDir: webDir, ObjectOrigin: objectOrigin, GitHubWebhookSecret: webhookSecret, GitHub: service,
	})
}

func TestFrontendServesFilesAndFallsBackToIndex(t *testing.T) {
	a := newAPITeam(t)
	dir := t.TempDir()
	for name, content := range map[string]string{
		ShellFile: "<!doctype html><title>app</title>", "assets/app-1a2b.js": "console.log(1)", ".env": "SECRET=1",
		"avatars/v1/cat-96.webp": "avatar", "avatars/cat.png": "legacy",
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
		{"/avatars/v1/cat-96.webp", "avatar", "immutable", http.StatusOK},
		{"/avatars/cat.png", "legacy", "no-cache", http.StatusOK},
		{"/.env", "<title>app</title>", "no-cache", http.StatusOK},
		{"/missing.js", "", "", http.StatusNotFound},
	} {
		response := a.page(check.path)
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

func TestShellPageAllowsOnlyItsOwnInlineScripts(t *testing.T) {
	a := newAPITeam(t)
	dir := t.TempDir()
	inline := `self.$_TSR={buffer:[]};`
	shell := `<!doctype html><script data-tsr-stream-part="">` + inline + `</script>` +
		`<script type="module" async="" src="/assets/index.js"></script><script></script>`
	if err := os.WriteFile(filepath.Join(dir, ShellFile), []byte(shell), 0o600); err != nil {
		t.Fatal(err)
	}
	a.withOptions(dir)

	response := a.page("/WP-1")
	policy := response.Header().Get("Content-Security-Policy")
	sum := sha256.Sum256([]byte(inline))
	want := "script-src 'self' 'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "';"
	if response.Code != http.StatusOK || !strings.Contains(policy, want) || strings.Contains(policy, "unsafe-inline") ||
		!strings.Contains(policy, "frame-ancestors 'none'") ||
		!strings.Contains(policy, "img-src 'self' data: blob: "+objectOrigin+";") ||
		!strings.Contains(policy, "media-src 'self' "+objectOrigin+";") ||
		!strings.Contains(policy, "connect-src 'self' "+objectOrigin+" ") {
		t.Fatalf("status %d, policy %q, want %q in it", response.Code, policy, want)
	}
	if response.Header().Get("Content-Type") != "text/html; charset=utf-8" {
		t.Errorf("shell served as %q", response.Header().Get("Content-Type"))
	}
}
