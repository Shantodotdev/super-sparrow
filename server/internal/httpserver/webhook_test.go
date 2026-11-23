package httpserver

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/shantodotdev/super-sparrow/server/internal/github"
	"github.com/shantodotdev/super-sparrow/server/internal/store"
)

// deliver posts a webhook delivery with the given headers. An empty id sends no delivery header.
func (a *apiTeam) deliver(event, id, body, signature string) *httptest.ResponseRecorder {
	a.t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/api/webhooks/github", strings.NewReader(body))
	request.Header.Set("X-GitHub-Event", event)
	request.Header.Set("X-Hub-Signature-256", signature)
	if id != "" {
		request.Header.Set("X-GitHub-Delivery", id)
	}
	response := httptest.NewRecorder()
	a.handler.ServeHTTP(response, request)
	return response
}

// deliverSigned signs the body and posts it.
func (a *apiTeam) deliverSigned(event, id, body string) *httptest.ResponseRecorder {
	a.t.Helper()
	return a.deliver(event, id, body, sign(body))
}

func sign(body string) string {
	mac := hmac.New(sha256.New, []byte(webhookSecret))
	mac.Write([]byte(body))
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

const repositoryJSON = `{"id":99,"full_name":"acme/widgets","private":true}`

func installationBody(action string) string {
	return fmt.Sprintf(`{"action":%q,"installation":{"id":7,"account":{"login":"acme"},"suspended_at":null},"repositories":[%s]}`, action, repositoryJSON)
}

// pullBody is a full pull_request delivery for pull request 3 of acme/widgets, branch
// feat/wp-1-meta.
func pullBody(action, state string, merged bool) string {
	mergedAt, mergedBy := "null", "null"
	if merged {
		mergedAt, mergedBy = `"2026-05-01T13:00:00Z"`, `{"login":"olive"}`
	}
	return fmt.Sprintf(`{"action":%q,"number":3,"pull_request":{"id":5001,"number":3,"title":"Add meta comments",
		"html_url":"https://github.com/acme/widgets/pull/3","state":%q,"draft":false,"merged":%t,"merged_at":%s,"closed_at":%s,
		"created_at":"2026-05-01T12:00:00Z","updated_at":"2026-05-01T12:30:00Z","additions":10,"deletions":2,"changed_files":3,
		"user":{"login":"maxm"},"merged_by":%s,"requested_reviewers":[{"login":"olive"}],
		"head":{"ref":"feat/wp-1-meta","sha":"abc123"},"base":{"ref":"main"}},
		"repository":%s,"sender":{"login":"maxm"},"installation":{"id":7}}`,
		action, state, merged, mergedAt, mergedAt, mergedBy, repositoryJSON)
}

func reviewBody(state string) string {
	return fmt.Sprintf(`{"action":"submitted","review":{"id":77,"user":{"login":"olive"},"state":%q,
		"html_url":"https://github.com/acme/widgets/pull/3#pullrequestreview-77","submitted_at":"2026-05-01T12:10:00Z"},
		"pull_request":{"id":5001,"number":3,"title":"Add meta comments","html_url":"https://github.com/acme/widgets/pull/3",
		"state":"open","draft":false,"merged_at":null,"closed_at":null,"created_at":"2026-05-01T12:00:00Z","updated_at":"2026-05-01T12:30:00Z",
		"user":{"login":"maxm"},"head":{"ref":"feat/wp-1-meta","sha":"abc123"},"base":{"ref":"main"}},
		"repository":%s,"installation":{"id":7}}`, state, repositoryJSON)
}

func TestGitHubWebhookChecksSignatureEventAndDeliveryID(t *testing.T) {
	a := newAPITeam(t)
	a.withOptions("")

	body := pullBody("opened", "open", false)
	wantStatus(t, a.deliver("pull_request", "d-1", body, "sha256=00"), http.StatusUnauthorized)
	wantStatus(t, a.deliver("pull_request", "d-1", body, sign(body+" ")), http.StatusUnauthorized)
	wantStatus(t, a.deliver("push", "d-1", "{}", ""), http.StatusUnauthorized)

	oversized := strings.Repeat(" ", maxWebhookBytes+1)
	wantStatus(t, a.deliverSigned("pull_request", "d-1", oversized), http.StatusBadRequest)

	wantField(t, a.deliverSigned("pull_request", "", body), "X-GitHub-Delivery")
	wantField(t, a.deliverSigned("pull_request", strings.Repeat("d", 101), body), "X-GitHub-Delivery")
	wantField(t, a.deliverSigned("pull_request", "d-2", "not json"), "body")

	push := a.deliverSigned("push", "d-3", "{}")
	wantStatus(t, push, http.StatusAccepted)
	if !strings.Contains(push.Body.String(), `"result":"ignored"`) {
		t.Fatalf("push answered %s", push.Body)
	}

	ping := a.deliverSigned("ping", "d-4", "{}")
	wantStatus(t, ping, http.StatusOK)
	if !strings.Contains(ping.Body.String(), `"result":"pong"`) {
		t.Fatalf("ping answered %s", ping.Body)
	}
}

func TestGitHubWebhookIsOffWithoutASecret(t *testing.T) {
	a := newAPITeam(t)
	wantStatus(t, a.deliver("ping", "d-1", "{}", sign("{}")), http.StatusNotFound)
}

// pullRequestsOf lists a project's pull requests in a state through the API.
func (a *apiTeam) pullRequestsOf(session, state string) []store.GitHubPullRequest {
	a.t.Helper()
	var body struct {
		PullRequests []store.GitHubPullRequest `json:"pull_requests"`
	}
	a.call(http.MethodGet, "/api/projects/WP/pull-requests?state="+state, session, nil, http.StatusOK, &body)
	return body.PullRequests
}

func (a *apiTeam) eventCount() int {
	a.t.Helper()
	events, err := a.store.ListEvents(context.Background(), store.EventFilter{Limit: 200})
	if err != nil {
		a.t.Fatalf("ListEvents: %v", err)
	}
	return len(events)
}

func TestGitHubPullRequestsFlowThroughTheWebhook(t *testing.T) {
	a := newAPITeam(t)
	a.project()
	a.withOptions("")

	wantStatus(t, a.deliverSigned("installation", "d-install", installationBody("created")), http.StatusAccepted)
	var linked struct {
		Repository store.GitHubRepository `json:"repository"`
	}
	a.call(http.MethodPut, "/api/github/repositories/99/project", a.owner, map[string]any{"project_key": "wp"}, http.StatusOK, &linked)
	if linked.Repository.ProjectKey != "WP" || !linked.Repository.Installed || linked.Repository.FullName != "acme/widgets" {
		t.Fatalf("linked repository %+v", linked.Repository)
	}

	opened := pullBody("opened", "open", false)
	response := a.deliverSigned("pull_request", "d-open", opened)
	wantStatus(t, response, http.StatusAccepted)
	if !strings.Contains(response.Body.String(), `"result":"applied"`) {
		t.Fatalf("opened answered %s", response.Body)
	}
	pulls := a.pullRequestsOf(a.member, "open")
	if len(pulls) != 1 || pulls[0].Number != 3 || pulls[0].IssueKey != "WP-1" || pulls[0].AuthorLogin != "maxm" || pulls[0].Additions != 10 {
		t.Fatalf("open pull requests %+v", pulls)
	}

	events := a.eventCount()
	wantStatus(t, a.deliverSigned("pull_request", "d-open", opened), http.StatusAccepted)
	if a.eventCount() != events || len(a.pullRequestsOf(a.member, "all")) != 1 {
		t.Fatal("a repeated delivery ID changed something")
	}

	wantStatus(t, a.deliverSigned("pull_request_review", "d-review", reviewBody("approved")), http.StatusAccepted)
	if pulls = a.pullRequestsOf(a.member, "open"); len(pulls) != 1 || pulls[0].ReviewState != store.ReviewApproved ||
		len(pulls[0].Reviews) != 1 || pulls[0].Reviews[0].ReviewerLogin != "olive" {
		t.Fatalf("after review %+v", pulls)
	}

	wantStatus(t, a.deliverSigned("pull_request", "d-merge", pullBody("closed", "closed", true)), http.StatusAccepted)
	merged := a.pullRequestsOf(a.member, "merged")
	if len(merged) != 1 || merged[0].MergedByLogin != "olive" || len(a.pullRequestsOf(a.member, "open")) != 0 {
		t.Fatalf("merged pull requests %+v", merged)
	}

	var issue struct {
		Issue issueView `json:"issue"`
	}
	a.call(http.MethodGet, "/api/issues/WP-1", a.member, nil, http.StatusOK, &issue)
	if issue.Issue.Status != "done" || issue.Issue.PullRequestURL != "https://github.com/acme/widgets/pull/3" {
		t.Fatalf("issue after merge %+v", issue.Issue)
	}

	unrelated := strings.ReplaceAll(pullBody("closed", "closed", true), "feat/wp-1-meta", "chore/deps")
	unrelated = strings.NewReplacer("5001", "5002", `"number":3`, `"number":4`, "pull/3", "pull/4").Replace(unrelated)
	wantStatus(t, a.deliverSigned("pull_request", "d-other", unrelated), http.StatusAccepted)
}

func TestPullRequestListNeedsAStateAndAProject(t *testing.T) {
	a := newAPITeam(t)
	a.project()

	wantField(t, a.request(http.MethodGet, "/api/projects/WP/pull-requests", a.member, nil), "state")
	wantField(t, a.request(http.MethodGet, "/api/projects/WP/pull-requests?state=stale", a.member, nil), "state")
	wantStatus(t, a.request(http.MethodGet, "/api/projects/NOPE/pull-requests?state=all", a.member, nil), http.StatusNotFound)
	wantStatus(t, a.request(http.MethodGet, "/api/projects/WP/pull-requests?state=all", "", nil), http.StatusUnauthorized)
	wantStatus(t, a.request(http.MethodGet, "/api/projects/WP/pull-requests?state=all", a.agent, nil), http.StatusOK)
}

func TestGitHubSettingsEndpointsAreOwnerOnly(t *testing.T) {
	a := newAPITeam(t)
	a.project()
	a.withOptions("")
	wantStatus(t, a.deliverSigned("installation", "d-install", installationBody("created")), http.StatusAccepted)

	for _, endpoint := range []struct {
		method, path string
		body         any
	}{
		{http.MethodGet, "/api/github", nil},
		{http.MethodPost, "/api/github/refresh", nil},
		{http.MethodPut, "/api/github/repositories/99/project", map[string]any{"project_key": "WP"}},
		{http.MethodPost, "/api/github/repositories/99/sync", nil},
		{http.MethodDelete, "/api/github/channels/WP", nil},
	} {
		wantStatus(t, a.request(endpoint.method, endpoint.path, a.member, endpoint.body), http.StatusForbidden)
		wantStatus(t, a.request(endpoint.method, endpoint.path, a.agent, endpoint.body), http.StatusForbidden)
		wantStatus(t, a.request(endpoint.method, endpoint.path, "", endpoint.body), http.StatusUnauthorized)
	}

	var settings struct {
		App struct {
			Configured bool    `json:"configured"`
			InstallURL *string `json:"install_url"`
		} `json:"app"`
		Repositories []store.GitHubRepository      `json:"repositories"`
		Accounts     []store.GitHubAccount         `json:"accounts"`
		Channels     []store.DiscordProjectChannel `json:"channels"`
	}
	a.call(http.MethodGet, "/api/github", a.owner, nil, http.StatusOK, &settings)
	if settings.App.Configured || settings.App.InstallURL != nil || len(settings.Repositories) != 1 || settings.Accounts == nil || settings.Channels == nil {
		t.Fatalf("settings %+v", settings)
	}
	if got := a.request(http.MethodGet, "/api/github", a.owner, nil).Body.String(); !strings.Contains(got, `"install_url":null`) {
		t.Fatalf("settings body %s", got)
	}

	// Without the App there is nothing to read from GitHub, so these refuse clearly.
	wantStatus(t, a.request(http.MethodPost, "/api/github/refresh", a.owner, nil), http.StatusNotFound)
	wantStatus(t, a.request(http.MethodPost, "/api/github/repositories/99/sync", a.owner, nil), http.StatusNotFound)

	wantField(t, a.request(http.MethodPut, "/api/github/repositories/99/project", a.owner, map[string]any{}), "project_key")
	wantField(t, a.request(http.MethodPut, "/api/github/repositories/abc/project", a.owner, map[string]any{"project_key": "WP"}), "id")
	wantStatus(t, a.request(http.MethodPut, "/api/github/repositories/12345/project", a.owner, map[string]any{"project_key": "WP"}), http.StatusNotFound)
	wantStatus(t, a.request(http.MethodDelete, "/api/github/channels/WP", a.owner, nil), http.StatusNotFound)

	a.call(http.MethodPut, "/api/github/repositories/99/project", a.owner, map[string]any{"project_key": "WP"}, http.StatusOK, nil)
	var unlinked struct {
		Repository store.GitHubRepository `json:"repository"`
	}
	a.call(http.MethodPut, "/api/github/repositories/99/project", a.owner, map[string]any{"project_key": nil}, http.StatusOK, &unlinked)
	if unlinked.Repository.ProjectKey != "" {
		t.Fatalf("repository still linked: %+v", unlinked.Repository)
	}
}

func TestGitHubLoginsBelongToTheirPersonOrAnOwner(t *testing.T) {
	a := newAPITeam(t)
	ctx := context.Background()
	if _, err := a.store.CreateHuman(ctx, store.Caller{ActorID: a.fixture.owner.ID},
		store.NewHuman{DisplayName: "Nina Newcomer", Email: "nina@example.com", Role: store.RoleMember}); err != nil {
		t.Fatalf("CreateHuman: %v", err)
	}
	nina := a.signIn("nina@example.com")

	var me struct {
		Actor actorView `json:"actor"`
	}
	a.call(http.MethodGet, "/api/me", a.member, nil, http.StatusOK, &me)
	max := me.Actor.ID
	a.call(http.MethodGet, "/api/me", nina, nil, http.StatusOK, &me)

	var set struct {
		Account *store.GitHubAccount `json:"account"`
	}
	a.call(http.MethodPut, "/api/people/"+max+"/github", a.member, map[string]any{"login": "@maxm"}, http.StatusOK, &set)
	if set.Account == nil || set.Account.Login != "maxm" || set.Account.PersonID != max {
		t.Fatalf("own login %+v", set.Account)
	}

	// Another member cannot set it, an agent cannot, and nobody takes a login a teammate has.
	wantStatus(t, a.request(http.MethodPut, "/api/people/"+max+"/github", nina, map[string]any{"login": "hijack"}), http.StatusForbidden)
	wantStatus(t, a.request(http.MethodPut, "/api/people/"+max+"/github", a.agent, map[string]any{"login": "hijack"}), http.StatusForbidden)
	wantStatus(t, a.request(http.MethodPut, "/api/people/"+me.Actor.ID+"/github", nina, map[string]any{"login": "maxm"}), http.StatusConflict)
	wantField(t, a.request(http.MethodPut, "/api/people/"+max+"/github", a.member, map[string]any{"login": "not valid"}), "login")
	wantField(t, a.request(http.MethodPut, "/api/people/"+max+"/github", a.member, map[string]any{}), "login")

	a.call(http.MethodPut, "/api/people/"+me.Actor.ID+"/github", a.owner, map[string]any{"login": "ninan"}, http.StatusOK, &set)
	if set.Account == nil || set.Account.Login != "ninan" {
		t.Fatalf("owner set %+v", set.Account)
	}

	var accounts struct {
		Accounts []store.GitHubAccount `json:"accounts"`
	}
	a.call(http.MethodGet, "/api/github/accounts", a.member, nil, http.StatusOK, &accounts)
	if len(accounts.Accounts) != 2 {
		t.Fatalf("accounts %+v", accounts.Accounts)
	}
	wantStatus(t, a.request(http.MethodGet, "/api/github/accounts", "", nil), http.StatusUnauthorized)

	a.call(http.MethodPut, "/api/people/"+max+"/github", a.member, map[string]any{"login": nil}, http.StatusOK, &set)
	if set.Account != nil {
		t.Fatalf("cleared login %+v", set.Account)
	}
	if got := a.request(http.MethodPut, "/api/people/"+max+"/github", a.member, map[string]any{"login": nil}).Body.String(); !strings.Contains(got, `"account":null`) {
		t.Fatalf("clear body %s", got)
	}
}

func TestGitHubSettingsOfferTheInstallLinkWhenTheAppIsConfigured(t *testing.T) {
	a := newAPITeam(t)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	publicURL, _ := url.Parse(origin)

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	app, err := github.NewApp(42, "my-sparrow", key, github.APIURL, http.DefaultClient, time.Now)
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	service, err := github.NewService(a.store, app, nil, logger)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	a.handler = New(Options{Logger: logger, Store: a.store, Auth: a.auth, PublicURL: publicURL, Now: time.Now,
		GitHubWebhookSecret: webhookSecret, GitHub: service})

	response := a.request(http.MethodGet, "/api/github", a.owner, nil)
	wantStatus(t, response, http.StatusOK)
	if !strings.Contains(response.Body.String(), `"app":{"configured":true,"install_url":"https://github.com/apps/my-sparrow/installations/new"}`) {
		t.Fatalf("settings body %s", response.Body)
	}
}
