package github

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/shantodotdev/super-sparrow/server/internal/database"
	"github.com/shantodotdev/super-sparrow/server/internal/store"
	"github.com/shantodotdev/super-sparrow/server/internal/store/storetest"
	"github.com/shantodotdev/super-sparrow/server/migrations"
)

const (
	appID          = 4242
	installationID = 7
	repositoryID   = 99
	headSHA        = "abc123def456"
	pullID         = 5001
)

var baseTime = time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

// fakeGitHub is an httptest GitHub API. Routes map "METHOD /path" to a JSON answer, and every
// request is recorded.
type fakeGitHub struct {
	t      *testing.T
	server *httptest.Server
	key    *rsa.PrivateKey
	now    func() time.Time

	mu       sync.Mutex
	routes   map[string]any
	requests []string
	tokens   int
	badToken string
}

func newFakeGitHub(t *testing.T, key *rsa.PrivateKey, now func() time.Time) *fakeGitHub {
	t.Helper()
	fake := &fakeGitHub{t: t, key: key, now: now, routes: map[string]any{}}
	fake.server = httptest.NewServer(http.HandlerFunc(fake.serve))
	t.Cleanup(fake.server.Close)
	return fake
}

func (f *fakeGitHub) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	route := r.Method + " " + r.URL.RequestURI()
	f.requests = append(f.requests, route)
	if r.Header.Get("Accept") != "application/vnd.github+json" || r.Header.Get("X-GitHub-Api-Version") != "2022-11-28" || r.Header.Get("User-Agent") != "super-sparrow" {
		http.Error(w, `{"message":"bad headers"}`, http.StatusBadRequest)
		return
	}
	credential := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")

	if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/access_tokens") {
		if err := verifyJWT(credential, &f.key.PublicKey, f.now()); err != nil {
			f.t.Errorf("access token request: %v", err)
			http.Error(w, `{"message":"bad jwt"}`, http.StatusUnauthorized)
			return
		}
		f.tokens++
		_ = json.NewEncoder(w).Encode(map[string]any{
			"token": fmt.Sprintf("token-%d", f.tokens), "expires_at": f.now().Add(time.Hour).Format(time.RFC3339),
		})
		return
	}

	if strings.HasPrefix(r.URL.Path, "/app/installations") && !strings.Contains(r.URL.Path, "/access_tokens") {
		if err := verifyJWT(credential, &f.key.PublicKey, f.now()); err != nil {
			f.t.Errorf("app request: %v", err)
			http.Error(w, `{"message":"bad jwt"}`, http.StatusUnauthorized)
			return
		}
	} else if !strings.HasPrefix(credential, "token-") || credential == f.badToken {
		http.Error(w, `{"message":"Bad credentials"}`, http.StatusUnauthorized)
		return
	}

	answer, ok := f.routes[route]
	if !ok {
		http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound)
		return
	}
	_ = json.NewEncoder(w).Encode(answer)
}

func (f *fakeGitHub) set(route string, answer any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.routes[route] = answer
}

func (f *fakeGitHub) tokenRequests() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.tokens
}

func (f *fakeGitHub) requested(route string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Contains(f.requests, route)
}

// verifyJWT checks an RS256 token's signature and claims against the public key.
func verifyJWT(token string, key *rsa.PublicKey, now time.Time) error {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return fmt.Errorf("token has %d parts", len(parts))
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return err
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(key, crypto.SHA256, digest[:], signature); err != nil {
		return fmt.Errorf("signature: %w", err)
	}

	var header map[string]string
	var claims struct {
		Iat int64  `json:"iat"`
		Exp int64  `json:"exp"`
		Iss string `json:"iss"`
	}
	for i, target := range []any{&header, &claims} {
		raw, err := base64.RawURLEncoding.DecodeString(parts[i])
		if err != nil {
			return err
		}
		if err := json.Unmarshal(raw, target); err != nil {
			return err
		}
	}
	switch {
	case header["alg"] != "RS256":
		return fmt.Errorf("alg %q", header["alg"])
	case claims.Iss != "4242":
		return fmt.Errorf("iss %q", claims.Iss)
	case claims.Iat != now.Add(-time.Minute).Unix():
		return fmt.Errorf("iat %d", claims.Iat)
	case claims.Exp != now.Add(9*time.Minute).Unix():
		return fmt.Errorf("exp %d", claims.Exp)
	}
	return nil
}

type recordingNotifier struct {
	mu    sync.Mutex
	kinds []string
}

func (n *recordingNotifier) PullRequestChanged(_ context.Context, change store.GitHubPullRequestChange) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.kinds = append(n.kinds, change.Kind)
}

func (n *recordingNotifier) seen() []string {
	n.mu.Lock()
	defer n.mu.Unlock()
	return slices.Clone(n.kinds)
}

type harness struct {
	t        *testing.T
	store    *store.Store
	service  *Service
	fake     *fakeGitHub
	notifier *recordingNotifier
	owner    store.Actor
	by       store.Caller
	clock    *time.Time
}

// newHarness builds a service on a real database. withApp false leaves the GitHub App off.
func newHarness(t *testing.T, withApp bool) *harness {
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
	st := store.New(db, time.Now, storetest.NewObjects())
	owner, err := st.CreateFirstOwner(ctx, "Olive Owner", "olive@example.com")
	if err != nil {
		t.Fatalf("CreateFirstOwner: %v", err)
	}

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	clock := baseTime
	now := func() time.Time { return clock }
	h := &harness{t: t, store: st, owner: owner, notifier: &recordingNotifier{}, clock: &clock,
		by: store.Caller{ActorID: owner.ID, Client: store.ClientBrowser}}

	var app *App
	if withApp {
		h.fake = newFakeGitHub(t, key, now)
		app, err = NewApp(appID, "my-sparrow", key, h.fake.server.URL, h.fake.server.Client(), now)
		if err != nil {
			t.Fatalf("NewApp: %v", err)
		}
	}
	h.service, err = NewService(st, app, h.notifier, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	t.Cleanup(h.service.Wait)
	return h
}

func (h *harness) deliver(event, id string, payload any) string {
	h.t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		h.t.Fatalf("encode payload: %v", err)
	}
	result, err := h.service.HandleDelivery(context.Background(), event, id, body)
	if err != nil {
		h.t.Fatalf("deliver %s %s: %v", event, id, err)
	}
	return result
}

// project creates project WP with issue WP-1 on branch feat/wp-1-meta.
func (h *harness) project() {
	h.t.Helper()
	ctx := context.Background()
	project, err := h.store.CreateProject(ctx, h.by, store.NewProject{Key: "WP", Name: "WP Agent"})
	if err != nil {
		h.t.Fatalf("CreateProject: %v", err)
	}
	_, err = h.store.CreateIssue(ctx, h.by, store.NewIssue{ProjectID: project.ID, Title: "Meta", Status: store.IssueInProgress, BranchName: "feat/wp-1-meta"})
	if err != nil {
		h.t.Fatalf("CreateIssue: %v", err)
	}
}

func (h *harness) kinds() []string {
	h.t.Helper()
	events, err := h.store.ListEvents(context.Background(), store.EventFilter{Limit: 200})
	if err != nil {
		h.t.Fatalf("ListEvents: %v", err)
	}
	kinds := make([]string, 0, len(events))
	for _, event := range events {
		kinds = append(kinds, event.Kind)
	}
	return kinds
}

func (h *harness) pulls(state string) []store.GitHubPullRequest {
	h.t.Helper()
	pulls, err := h.store.ListGitHubPullRequests(context.Background(), "WP", state)
	if err != nil {
		h.t.Fatalf("ListGitHubPullRequests: %v", err)
	}
	return pulls
}

func repositoryObject() map[string]any {
	return map[string]any{"id": repositoryID, "full_name": "acme/widgets", "private": true}
}

func installationEvent(action string) map[string]any {
	return map[string]any{
		"action":       action,
		"installation": map[string]any{"id": installationID, "account": map[string]any{"login": "acme"}, "suspended_at": nil},
		"repositories": []any{repositoryObject()},
	}
}

func pullObject(state string, merged bool) map[string]any {
	pull := map[string]any{
		"id": pullID, "number": 3, "title": "Add meta comments", "html_url": "https://github.com/acme/widgets/pull/3",
		"state": state, "draft": false, "merged": merged, "merged_at": nil, "closed_at": nil,
		"created_at": baseTime.Format(time.RFC3339), "updated_at": baseTime.Add(time.Minute).Format(time.RFC3339),
		"additions": 10, "deletions": 2, "changed_files": 3,
		"user":                map[string]any{"login": "maxm"},
		"merged_by":           nil,
		"requested_reviewers": []any{map[string]any{"login": "olive"}},
		"head":                map[string]any{"ref": "feat/wp-1-meta", "sha": headSHA},
		"base":                map[string]any{"ref": "main"},
	}
	if merged {
		pull["merged_at"] = baseTime.Add(time.Hour).Format(time.RFC3339)
		pull["closed_at"] = baseTime.Add(time.Hour).Format(time.RFC3339)
		pull["merged_by"] = map[string]any{"login": "olive"}
	}
	return pull
}

func pullEvent(action string, pull map[string]any) map[string]any {
	return map[string]any{
		"action": action, "pull_request": pull, "repository": repositoryObject(),
		"sender": map[string]any{"login": "maxm"}, "installation": map[string]any{"id": installationID},
	}
}

// install records the installation and links its repository to project WP.
func (h *harness) install() {
	h.t.Helper()
	h.project()
	if result := h.deliver("installation", "d-install", installationEvent("created")); result != "applied" {
		h.t.Fatalf("installation result %q", result)
	}
	if _, err := h.store.LinkGitHubRepository(context.Background(), h.by, repositoryID, "WP"); err != nil {
		h.t.Fatalf("LinkGitHubRepository: %v", err)
	}
}

func TestAppSignsJWTAndCachesInstallationTokens(t *testing.T) {
	h := newHarness(t, true)
	h.fake.set("GET /repos/acme/widgets/ping", map[string]string{"ok": "yes"})

	var answer map[string]string
	for range 2 {
		if err := h.service.app.getAsInstallation(context.Background(), installationID, "/repos/acme/widgets/ping", &answer); err != nil {
			t.Fatalf("getAsInstallation: %v", err)
		}
	}
	if answer["ok"] != "yes" || h.fake.tokenRequests() != 1 {
		t.Fatalf("answer %v after %d token requests, want one", answer, h.fake.tokenRequests())
	}

	*h.clock = baseTime.Add(time.Hour - 30*time.Second)
	if err := h.service.app.getAsInstallation(context.Background(), installationID, "/repos/acme/widgets/ping", &answer); err != nil {
		t.Fatalf("getAsInstallation near expiry: %v", err)
	}
	if h.fake.tokenRequests() != 2 {
		t.Fatalf("token requests = %d, want a fresh token inside the last minute", h.fake.tokenRequests())
	}
}

func TestAppRetriesOnceWhenTheCachedTokenWasRevoked(t *testing.T) {
	h := newHarness(t, true)
	h.fake.set("GET /repos/acme/widgets/ping", map[string]string{"ok": "yes"})

	var answer map[string]string
	if err := h.service.app.getAsInstallation(context.Background(), installationID, "/repos/acme/widgets/ping", &answer); err != nil {
		t.Fatalf("first read: %v", err)
	}
	h.fake.mu.Lock()
	h.fake.badToken = "token-1"
	h.fake.mu.Unlock()

	if err := h.service.app.getAsInstallation(context.Background(), installationID, "/repos/acme/widgets/ping", &answer); err != nil {
		t.Fatalf("read after revocation: %v", err)
	}
	if h.fake.tokenRequests() != 2 {
		t.Fatalf("token requests = %d, want 2", h.fake.tokenRequests())
	}
}

func TestAppErrorsNameTheRequestButNotTheCredential(t *testing.T) {
	h := newHarness(t, true)

	err := h.service.app.getAsInstallation(context.Background(), installationID, "/repos/acme/missing", nil)
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != http.StatusNotFound {
		t.Fatalf("error %v, want a 404 APIError", err)
	}
	if strings.Contains(err.Error(), "token-") || !strings.Contains(err.Error(), "/repos/acme/missing") {
		t.Fatalf("error text %q", err)
	}
}

func TestNewAppRequiresEveryDependency(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now
	for name, build := range map[string]func() (*App, error){
		"id":     func() (*App, error) { return NewApp(0, "s", key, "u", http.DefaultClient, now) },
		"slug":   func() (*App, error) { return NewApp(1, "", key, "u", http.DefaultClient, now) },
		"key":    func() (*App, error) { return NewApp(1, "s", nil, "u", http.DefaultClient, now) },
		"url":    func() (*App, error) { return NewApp(1, "s", key, "", http.DefaultClient, now) },
		"client": func() (*App, error) { return NewApp(1, "s", key, "u", nil, now) },
		"clock":  func() (*App, error) { return NewApp(1, "s", key, "u", http.DefaultClient, nil) },
	} {
		if _, err := build(); err == nil {
			t.Errorf("NewApp accepted a missing %s", name)
		}
	}
}

func TestCombineChecks(t *testing.T) {
	run := func(status, conclusion string) checkRun { return checkRun{Status: status, Conclusion: conclusion} }
	for _, test := range []struct {
		name   string
		runs   []checkRun
		status combinedStatus
		want   string
	}{
		{"nothing", nil, combinedStatus{State: "pending"}, store.ChecksNone},
		{"all green", []checkRun{run("completed", "success"), run("completed", "neutral"), run("completed", "skipped")}, combinedStatus{}, store.ChecksSuccess},
		{"failed run", []checkRun{run("completed", "success"), run("completed", "timed_out")}, combinedStatus{}, store.ChecksFailure},
		{"cancelled", []checkRun{run("completed", "cancelled")}, combinedStatus{}, store.ChecksFailure},
		{"action required", []checkRun{run("completed", "action_required")}, combinedStatus{}, store.ChecksFailure},
		{"startup failure", []checkRun{run("completed", "startup_failure")}, combinedStatus{}, store.ChecksFailure},
		{"running", []checkRun{run("completed", "success"), run("in_progress", "")}, combinedStatus{}, store.ChecksPending},
		{"failure beats pending", []checkRun{run("queued", ""), run("completed", "failure")}, combinedStatus{}, store.ChecksFailure},
		{"status failure", nil, combinedStatus{State: "failure", TotalCount: 1}, store.ChecksFailure},
		{"status error", []checkRun{run("completed", "success")}, combinedStatus{State: "error", TotalCount: 2}, store.ChecksFailure},
		{"status pending", []checkRun{run("completed", "success")}, combinedStatus{State: "pending", TotalCount: 1}, store.ChecksPending},
		{"status success", nil, combinedStatus{State: "success", TotalCount: 1}, store.ChecksSuccess},
	} {
		if got := combineChecks(test.runs, test.status); got != test.want {
			t.Errorf("%s: %s, want %s", test.name, got, test.want)
		}
	}
}

func TestPullRequestLifecycleThroughDeliveries(t *testing.T) {
	h := newHarness(t, true)
	h.install()
	h.fake.set("GET /repos/acme/widgets/commits/"+headSHA+"/check-runs?per_page=100", map[string]any{
		"check_runs": []any{map[string]any{"status": "completed", "conclusion": "success"}},
	})
	h.fake.set("GET /repos/acme/widgets/commits/"+headSHA+"/status", map[string]any{"state": "pending", "total_count": 0})

	opened := pullEvent("opened", pullObject("open", false))
	if result := h.deliver("pull_request", "d-open", opened); result != "applied" {
		t.Fatalf("opened result %q", result)
	}
	pulls := h.pulls("open")
	if len(pulls) != 1 {
		t.Fatalf("open pull requests = %d", len(pulls))
	}
	pull := pulls[0]
	if pull.Number != 3 || pull.AuthorLogin != "maxm" || pull.HeadRef != "feat/wp-1-meta" || pull.BaseRef != "main" ||
		pull.Additions != 10 || pull.ChangedFiles != 3 || pull.IssueKey != "WP-1" || len(pull.RequestedReviewers) != 1 ||
		pull.RequestedReviewers[0] != "olive" || pull.ChecksState != store.ChecksSuccess {
		t.Fatalf("stored pull request %+v", pull)
	}

	// The same delivery again changes nothing and tells nobody.
	h.deliver("pull_request", "d-open", opened)
	h.service.Wait()
	if seen := h.notifier.seen(); len(seen) != 1 || seen[0] != "github.pull_request.opened" {
		t.Fatalf("notifications %v", seen)
	}

	review := map[string]any{
		"action": "submitted", "pull_request": pullObject("open", false), "repository": repositoryObject(),
		"installation": map[string]any{"id": installationID},
		"review": map[string]any{
			"id": 77, "user": map[string]any{"login": "olive"}, "state": "APPROVED",
			"html_url": "https://github.com/acme/widgets/pull/3#pullrequestreview-77", "submitted_at": baseTime.Add(2 * time.Minute).Format(time.RFC3339),
		},
	}
	if result := h.deliver("pull_request_review", "d-review", review); result != "applied" {
		t.Fatalf("review result %q", result)
	}
	if got := h.pulls("open")[0]; got.ReviewState != store.ReviewApproved || len(got.Reviews) != 1 || got.Reviews[0].ReviewerLogin != "olive" {
		t.Fatalf("after review %+v", got)
	}

	pending := map[string]any{"action": "submitted", "pull_request": pullObject("open", false), "repository": repositoryObject(),
		"review": map[string]any{"id": 78, "user": map[string]any{"login": "olive"}, "state": "pending", "html_url": "https://github.com/acme/widgets/pull/3", "submitted_at": baseTime.Format(time.RFC3339)}}
	if result := h.deliver("pull_request_review", "d-pending", pending); result != "ignored" {
		t.Fatalf("pending review result %q", result)
	}

	comment := map[string]any{
		"action": "created", "repository": repositoryObject(), "pull_request": map[string]any{"number": 3},
		"comment": map[string]any{"user": map[string]any{"login": "olive"}, "html_url": "https://github.com/acme/widgets/pull/3#discussion_r1", "body": "Please rename this"},
	}
	if result := h.deliver("pull_request_review_comment", "d-comment", comment); result != "applied" {
		t.Fatalf("review comment result %q", result)
	}
	issueComment := map[string]any{
		"action": "created", "repository": repositoryObject(),
		"issue":   map[string]any{"number": 3, "pull_request": map[string]any{"url": "https://api.github.com/repos/acme/widgets/pulls/3"}},
		"comment": map[string]any{"user": map[string]any{"login": "olive"}, "html_url": "https://github.com/acme/widgets/pull/3#issuecomment-1", "body": "Looks good"},
	}
	if result := h.deliver("issue_comment", "d-issue-comment", issueComment); result != "applied" {
		t.Fatalf("issue comment result %q", result)
	}
	plain := map[string]any{"action": "created", "repository": repositoryObject(), "issue": map[string]any{"number": 3},
		"comment": issueComment["comment"]}
	if result := h.deliver("issue_comment", "d-plain", plain); result != "ignored" {
		t.Fatalf("comment on a plain issue result %q", result)
	}
	unknown := map[string]any{"action": "created", "repository": repositoryObject(), "pull_request": map[string]any{"number": 404},
		"comment": issueComment["comment"]}
	if result := h.deliver("pull_request_review_comment", "d-unknown", unknown); result != "ignored" {
		t.Fatalf("comment on an unknown pull request result %q", result)
	}

	merged := pullEvent("closed", pullObject("closed", true))
	h.deliver("pull_request", "d-merge", merged)
	h.service.Wait()
	if got := h.pulls("merged"); len(got) != 1 || got[0].MergedByLogin != "olive" || got[0].MergedAt == nil {
		t.Fatalf("merged pull requests %+v", got)
	}
	issue, err := h.store.GetIssueByKey(context.Background(), "WP-1")
	if err != nil || issue.Status != store.IssueDone {
		t.Fatalf("issue after merge: %+v, %v", issue.Status, err)
	}
	if seen := h.notifier.seen(); len(seen) != 2 || seen[1] != "github.pull_request.merged" {
		t.Fatalf("notifications %v", seen)
	}

	for _, want := range []string{"github.pull_request.opened", "github.review.submitted", "github.comment.created", "github.pull_request.merged"} {
		if !slices.Contains(h.kinds(), want) {
			t.Errorf("no %s event in %v", want, h.kinds())
		}
	}
}

func TestChecksDeliveriesRefreshTheCommit(t *testing.T) {
	h := newHarness(t, true)
	h.install()
	h.deliver("pull_request", "d-open", pullEvent("opened", pullObject("open", false)))

	runs := "GET /repos/acme/widgets/commits/" + headSHA + "/check-runs?per_page=100"
	status := "GET /repos/acme/widgets/commits/" + headSHA + "/status"
	h.fake.set(runs, map[string]any{"check_runs": []any{map[string]any{"status": "completed", "conclusion": "failure"}}})
	h.fake.set(status, map[string]any{"state": "success", "total_count": 1})

	for event, payload := range map[string]any{
		"check_suite": map[string]any{"repository": repositoryObject(), "check_suite": map[string]any{"head_sha": headSHA}},
		"check_run":   map[string]any{"repository": repositoryObject(), "check_run": map[string]any{"head_sha": headSHA}},
		"status":      map[string]any{"repository": repositoryObject(), "sha": headSHA},
	} {
		if result := h.deliver(event, "", payload); result != "applied" {
			t.Fatalf("%s result %q", event, result)
		}
	}
	if got := h.pulls("open")[0].ChecksState; got != store.ChecksFailure {
		t.Fatalf("checks state %q", got)
	}
	if !slices.Contains(h.kinds(), "github.checks.changed") {
		t.Fatalf("no checks event in %v", h.kinds())
	}
}

func TestDeliveriesWithoutTheAppIgnoreChecks(t *testing.T) {
	h := newHarness(t, false)
	h.install()

	if result := h.deliver("pull_request", "d-open", pullEvent("opened", pullObject("open", false))); result != "applied" {
		t.Fatalf("opened result %q", result)
	}
	if result := h.deliver("check_run", "", map[string]any{"repository": repositoryObject(), "check_run": map[string]any{"head_sha": headSHA}}); result != "ignored" {
		t.Fatalf("check_run result %q", result)
	}
	if h.service.InstallURL() != "" || h.service.Configured() {
		t.Fatal("service without an App reports one")
	}
	if _, err := h.service.SyncRepository(context.Background(), h.by, repositoryID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("SyncRepository without an App: %v", err)
	}
	if err := h.service.Refresh(context.Background(), h.by); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("Refresh without an App: %v", err)
	}
}

func TestInstallationDeliveries(t *testing.T) {
	h := newHarness(t, true)
	ctx := context.Background()

	if result := h.deliver("ping", "d-ping", map[string]any{}); result != "pong" {
		t.Fatalf("ping result %q", result)
	}
	if result := h.deliver("push", "d-push", map[string]any{}); result != "ignored" {
		t.Fatalf("push result %q", result)
	}
	h.deliver("installation", "d-1", installationEvent("created"))

	repositories, err := h.store.ListGitHubRepositories(ctx)
	if err != nil || len(repositories) != 1 || repositories[0].FullName != "acme/widgets" || !repositories[0].Installed ||
		!repositories[0].Private || repositories[0].URL != "https://github.com/acme/widgets" {
		t.Fatalf("repositories %+v, %v", repositories, err)
	}

	h.deliver("installation", "d-2", installationEvent("suspend"))
	installations, _ := h.store.ListGitHubInstallations(ctx)
	if len(installations) != 1 || !installations[0].Suspended || installations[0].AccountLogin != "acme" {
		t.Fatalf("installations after suspend %+v", installations)
	}
	h.deliver("installation", "d-3", installationEvent("unsuspend"))
	if installations, _ = h.store.ListGitHubInstallations(ctx); installations[0].Suspended {
		t.Fatal("installation still suspended")
	}

	h.deliver("installation_repositories", "d-4", map[string]any{
		"action": "removed", "installation": map[string]any{"id": installationID, "account": map[string]any{"login": "acme"}},
		"repositories_removed": []any{repositoryObject()},
	})
	if repositories, _ = h.store.ListGitHubRepositories(ctx); repositories[0].Installed {
		t.Fatal("repository still installed")
	}
	h.deliver("installation_repositories", "d-5", map[string]any{
		"action": "added", "installation": map[string]any{"id": installationID, "account": map[string]any{"login": "acme"}},
		"repositories_added": []any{map[string]any{"id": 100, "full_name": "acme/gadgets", "private": false}},
	})
	if repositories, _ = h.store.ListGitHubRepositories(ctx); len(repositories) != 2 {
		t.Fatalf("repositories after add %+v", repositories)
	}

	h.deliver("installation", "d-6", installationEvent("deleted"))
	if installations, _ = h.store.ListGitHubInstallations(ctx); len(installations) != 0 {
		t.Fatalf("installations after delete %+v", installations)
	}
}

func TestDeliveryBodiesMustMatchTheirEvent(t *testing.T) {
	h := newHarness(t, false)
	_, err := h.service.HandleDelivery(context.Background(), "pull_request", "d-1", []byte("not json"))
	var validation *store.ValidationError
	if !errors.As(err, &validation) || validation.Field != "body" {
		t.Fatalf("error %v, want a body validation error", err)
	}
}

func TestInstallCreatedSyncsLinkedRepositories(t *testing.T) {
	h := newHarness(t, true)
	h.project()
	ctx := context.Background()

	// The repository is known and linked, as after the App was reinstalled.
	h.deliver("installation", "d-1", installationEvent("created"))
	if _, err := h.store.LinkGitHubRepository(ctx, h.by, repositoryID, "WP"); err != nil {
		t.Fatal(err)
	}
	h.fake.set("GET /repos/acme/widgets/pulls?state=all&sort=updated&direction=desc&per_page=50", []any{map[string]any{"number": 3}})
	h.fake.set("GET /repos/acme/widgets/pulls/3", pullObject("open", false))
	h.fake.set("GET /repos/acme/widgets/pulls/3/reviews?per_page=100", []any{})

	h.deliver("installation", "d-2", installationEvent("created"))
	h.service.Wait()
	if len(h.pulls("all")) != 1 {
		t.Fatalf("pull requests after reinstall: %+v", h.pulls("all"))
	}
}

func TestSyncRepositoryLoadsPullRequestsReviewsAndChecks(t *testing.T) {
	h := newHarness(t, true)
	h.install()
	ctx := context.Background()

	merged := pullObject("closed", true)
	merged["id"], merged["number"], merged["head"] = 5002, 2, map[string]any{"ref": "chore/deps", "sha": "ffff"}
	h.fake.set("GET /repos/acme/widgets/pulls?state=all&sort=updated&direction=desc&per_page=50",
		[]any{map[string]any{"number": 3}, map[string]any{"number": 2}})
	h.fake.set("GET /repos/acme/widgets/pulls/3", pullObject("open", false))
	h.fake.set("GET /repos/acme/widgets/pulls/2", merged)
	h.fake.set("GET /repos/acme/widgets/pulls/3/reviews?per_page=100", []any{
		map[string]any{"id": 1, "user": map[string]any{"login": "olive"}, "state": "CHANGES_REQUESTED", "html_url": "https://github.com/acme/widgets/pull/3#r1", "submitted_at": baseTime.Format(time.RFC3339)},
		map[string]any{"id": 2, "user": nil, "state": "PENDING", "html_url": "https://github.com/acme/widgets/pull/3#r2", "submitted_at": nil},
	})
	h.fake.set("GET /repos/acme/widgets/pulls/2/reviews?per_page=100", []any{})
	h.fake.set("GET /repos/acme/widgets/commits/"+headSHA+"/check-runs?per_page=100", map[string]any{"check_runs": []any{map[string]any{"status": "in_progress"}}})
	h.fake.set("GET /repos/acme/widgets/commits/"+headSHA+"/status", map[string]any{"state": "pending", "total_count": 0})

	count, err := h.service.SyncRepository(ctx, h.by, repositoryID)
	if err != nil || count != 2 {
		t.Fatalf("SyncRepository = %d, %v", count, err)
	}
	open := h.pulls("open")
	if len(open) != 1 || open[0].ReviewState != store.ReviewChangesRequested || open[0].ChecksState != store.ChecksPending || len(open[0].Reviews) != 1 {
		t.Fatalf("open pull requests %+v", open)
	}
	if got := h.pulls("merged"); len(got) != 1 || got[0].Number != 2 {
		t.Fatalf("merged pull requests %+v", got)
	}
	if h.fake.requested("GET /repos/acme/widgets/commits/ffff/status") {
		t.Fatal("checks were read for a merged pull request")
	}
}

func TestRefreshStoresInstallationsAndRepositories(t *testing.T) {
	h := newHarness(t, true)
	ctx := context.Background()

	h.fake.set("GET /app/installations?per_page=100&page=1", []any{
		map[string]any{"id": installationID, "account": map[string]any{"login": "acme"}, "suspended_at": nil},
		map[string]any{"id": 8, "account": map[string]any{"login": "frozen"}, "suspended_at": "2026-05-01T00:00:00Z"},
	})
	h.fake.set("GET /installation/repositories?per_page=100&page=1", map[string]any{"repositories": []any{repositoryObject()}})

	if err := h.service.Refresh(ctx, h.by); err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	installations, _ := h.store.ListGitHubInstallations(ctx)
	if len(installations) != 2 || installations[0].Suspended || !installations[1].Suspended || installations[1].AccountLogin != "frozen" {
		t.Fatalf("installations %+v", installations)
	}
	repositories, _ := h.store.ListGitHubRepositories(ctx)
	if len(repositories) != 1 || !repositories[0].Installed {
		t.Fatalf("repositories %+v", repositories)
	}
	if h.service.InstallURL() != "https://github.com/apps/my-sparrow/installations/new" {
		t.Fatalf("install URL %q", h.service.InstallURL())
	}
}
