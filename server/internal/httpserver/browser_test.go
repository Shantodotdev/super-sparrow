package httpserver

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/shantodotdev/super-sparrow/server/internal/auth"
)

// TestBrowserSmoke drives the built SPA against a real HTTP server and temporary SQLite database.
func TestBrowserSmoke(t *testing.T) {
	chrome := os.Getenv("SPARROW_E2E_CHROME")
	if chrome == "" {
		t.Skip("set SPARROW_E2E_CHROME and build the frontend to run browser coverage")
	}
	a := newAPITeam(t)
	a.project()
	a.call(http.MethodPost, "/api/issues/WP-1/reviews", a.agent, map[string]string{}, http.StatusCreated, nil)
	a.call(http.MethodPost, "/api/issues/WP-1/reviews/1/findings", a.agent, map[string]string{"severity": "blocking", "title": "Browser finding", "problem": "Test problem", "fix": "Test fix"}, http.StatusCreated, nil)
	server := httptest.NewUnstartedServer(nil)
	publicURL, err := url.Parse("http://" + server.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	service := auth.New(a.store, a.mailer, publicURL, []byte("browser-test-code-key-0123456789"), logger, time.Now)
	t.Cleanup(service.Wait)
	webDir, err := filepath.Abs("../../../web/dist/client")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(webDir, ShellFile)); err != nil {
		t.Fatal("build the frontend first: ", err)
	}
	handler := New(Options{Logger: logger, Store: a.store, Auth: service, PublicURL: publicURL, Now: time.Now, WebDir: webDir})
	server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/__test/code" {
			service.Wait()
			match := codePattern.FindStringSubmatch(a.mailer.last().Text)
			if match == nil {
				http.NotFound(w, r)
				return
			}
			_, _ = io.WriteString(w, match[1])
			return
		}
		handler.ServeHTTP(w, r)
	})
	server.Start()
	defer server.Close()
	a.handler = handler
	issue := a.toolObject("get_issue", map[string]any{"issue_key": "WP-1"})
	round := a.toolObject("get_review_round", map[string]any{"issue_key": "WP-1", "round": 1})
	finding := a.toolObject("get_finding", map[string]any{"issue_key": "WP-1", "round": 1, "finding": "R1"})
	links, err := json.Marshal(map[string]any{"issue": issue["url"], "round": round["round"].(map[string]any)["url"], "finding": finding["url"]})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, "bun", "../../../web/e2e/smoke.ts")
	command.Env = append(os.Environ(), "SPARROW_E2E_URL="+server.URL, "SPARROW_E2E_MCP_LINKS="+string(links))
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("browser smoke: %v\n%s", err, output)
	}
	t.Log(string(output))
}
