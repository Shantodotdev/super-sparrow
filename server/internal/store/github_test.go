package store

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"
)

var githubBaseTime = time.Date(2026, 3, 1, 8, 0, 0, 0, time.UTC)

func ghRepo(id int64, name string) GitHubRepository {
	return GitHubRepository{ID: id, FullName: "acme/" + name, URL: "https://github.com/acme/" + name}
}

func ghPull(repo GitHubRepository, id int64, number int, branch string) GitHubPullRequestInput {
	return GitHubPullRequestInput{
		Repository: repo, ID: id, Number: number, Title: fmt.Sprintf("Pull %d", number),
		URL:         fmt.Sprintf("%s/pull/%d", repo.URL, number),
		State:       PullRequestOpen,
		AuthorLogin: "octocat", HeadRef: branch, BaseRef: "main", HeadSHA: "sha-1",
		Additions: 10, Deletions: 2, ChangedFiles: 3,
		OpenedAt: githubBaseTime, UpdatedAt: githubBaseTime,
	}
}

func ghReview(id int64, login, state string, at time.Time) GitHubReviewInput {
	return GitHubReviewInput{ID: id, ReviewerLogin: login, State: state, URL: fmt.Sprintf("https://github.com/acme/app/pull/1#review-%d", id), SubmittedAt: at}
}

// installRepos installs the repositories through installation 1 and returns them.
func (tm *team) installRepos(t *testing.T, repos ...GitHubRepository) {
	t.Helper()
	err := tm.store.ApplyGitHubInstallation(tm.ctx, tm.owner, "", GitHubInstallationChange{
		Installation: GitHubInstallation{ID: 1, AccountLogin: "acme"}, Repositories: repos, Complete: true,
	})
	if err != nil {
		t.Fatalf("install repositories: %v", err)
	}
}

// linkedRepo installs a repository and links it to a new project, returning both.
func (tm *team) linkedRepo(t *testing.T, key string, repo GitHubRepository) Project {
	t.Helper()
	tm.installRepos(t, repo)
	project := tm.project(t, key)
	got(tm.store.LinkGitHubRepository(tm.ctx, Caller{ActorID: tm.owner.ActorID, Client: ClientBrowser}, repo.ID, key)).must(t)
	return project
}

func (tm *team) eventsOf(t *testing.T, target TargetType, id string) []Event {
	t.Helper()
	return got(tm.store.ListEvents(tm.ctx, EventFilter{TargetType: target, TargetID: id, Limit: 200})).must(t)
}

func eventKinds(events []Event) []string {
	kinds := []string{}
	for _, event := range events {
		kinds = append(kinds, event.Kind)
	}
	return kinds
}

func wantKinds(t *testing.T, events []Event, want ...string) {
	t.Helper()
	if kinds := eventKinds(events); !slices.Equal(kinds, want) {
		t.Fatalf("event kinds = %v, want %v", kinds, want)
	}
}

func eventPayload(t *testing.T, event Event) map[string]any {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	return payload
}

func (tm *team) browser(c Caller) Caller {
	c.Client = ClientBrowser
	return c
}

func TestApplyGitHubInstallationRecordsRepositories(t *testing.T) {
	tm := newTeam(t)
	app, lib := ghRepo(10, "app"), ghRepo(11, "lib")

	err := tm.store.ApplyGitHubInstallation(tm.ctx, tm.owner, "d1", GitHubInstallationChange{
		Installation: GitHubInstallation{ID: 1, AccountLogin: "acme"}, Repositories: []GitHubRepository{app, lib}, Complete: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	installations := got(tm.store.ListGitHubInstallations(tm.ctx)).must(t)
	if len(installations) != 1 || installations[0].AccountLogin != "acme" || installations[0].Suspended {
		t.Fatalf("installations = %+v", installations)
	}
	repos := got(tm.store.ListGitHubRepositories(tm.ctx)).must(t)
	if len(repos) != 2 || !repos[0].Installed || !repos[1].Installed || repos[0].FullName != "acme/app" {
		t.Fatalf("repositories = %+v", repos)
	}
	for _, id := range []string{"10", "11"} {
		events := tm.eventsOf(t, TargetRepository, id)
		wantKinds(t, events, "github.repository.added")
		if events[0].ProjectID != "" {
			t.Errorf("event for unlinked repository has project %q", events[0].ProjectID)
		}
		if events[0].ActorID != tm.owner.ActorID {
			t.Errorf("actor = %s, want the owner", events[0].ActorID)
		}
	}

	// Reporting the same repositories again records nothing.
	before := tm.eventCount(t)
	tm.installRepos(t, app, lib)
	if tm.eventCount(t) != before {
		t.Fatal("a repeated installation recorded events")
	}
}

func TestApplyGitHubInstallationCompleteRefreshDetachesMissingRepositories(t *testing.T) {
	tm := newTeam(t)
	app, lib := ghRepo(10, "app"), ghRepo(11, "lib")
	tm.installRepos(t, app, lib)
	project := tm.project(t, "WP")
	got(tm.store.LinkGitHubRepository(tm.ctx, tm.browser(tm.owner), 11, "WP")).must(t)

	tm.installRepos(t, app)

	lost := got(tm.store.GetGitHubRepository(tm.ctx, 11)).must(t)
	if lost.Installed || lost.ProjectID != project.ID {
		t.Fatalf("detached repository = %+v, want uninstalled and still linked", lost)
	}
	if !got(tm.store.GetGitHubRepository(tm.ctx, 10)).must(t).Installed {
		t.Fatal("a repository still reported was detached")
	}
	events := tm.eventsOf(t, TargetRepository, "11")
	wantKinds(t, events, "github.repository.added", "github.repository.linked", "github.repository.removed")
	if events[2].ProjectID != project.ID {
		t.Errorf("removed event project = %q, want %q", events[2].ProjectID, project.ID)
	}

	// An incomplete report never detaches what it leaves out.
	err := tm.store.ApplyGitHubInstallation(tm.ctx, tm.owner, "", GitHubInstallationChange{
		Installation: GitHubInstallation{ID: 1, AccountLogin: "acme"}, Repositories: []GitHubRepository{ghRepo(12, "extra")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !got(tm.store.GetGitHubRepository(tm.ctx, 10)).must(t).Installed {
		t.Fatal("an incomplete report detached a repository")
	}

	// Reattaching reports it added again.
	tm.installRepos(t, app, lib)
	wantKinds(t, tm.eventsOf(t, TargetRepository, "11"),
		"github.repository.added", "github.repository.linked", "github.repository.removed", "github.repository.added")
}

func TestApplyGitHubInstallationRepositoriesAddedAndRemoved(t *testing.T) {
	tm := newTeam(t)
	app, lib := ghRepo(10, "app"), ghRepo(11, "lib")
	tm.installRepos(t, app)

	err := tm.store.ApplyGitHubInstallation(tm.ctx, tm.system, "d-add", GitHubInstallationChange{
		Installation: GitHubInstallation{ID: 1, AccountLogin: "acme"}, Repositories: []GitHubRepository{lib},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got(tm.store.ListGitHubRepositories(tm.ctx)).must(t)) != 2 {
		t.Fatal("added repository was not saved")
	}
	events := tm.eventsOf(t, TargetRepository, "11")
	wantKinds(t, events, "github.repository.added")
	if events[0].ActorID != SystemActorID {
		t.Errorf("actor = %s, want system", events[0].ActorID)
	}

	err = tm.store.ApplyGitHubInstallation(tm.ctx, tm.system, "d-remove", GitHubInstallationChange{
		Installation: GitHubInstallation{ID: 1, AccountLogin: "acme"}, Removed: []int64{10, 999},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got(tm.store.GetGitHubRepository(tm.ctx, 10)).must(t).Installed {
		t.Fatal("removed repository is still installed")
	}
	if !got(tm.store.GetGitHubRepository(tm.ctx, 11)).must(t).Installed {
		t.Fatal("a repository that was not removed was detached")
	}
	wantKinds(t, tm.eventsOf(t, TargetRepository, "10"), "github.repository.added", "github.repository.removed")
}

func TestApplyGitHubInstallationDeletedLeavesRepositoriesUninstalled(t *testing.T) {
	tm := newTeam(t)
	tm.installRepos(t, ghRepo(10, "app"), ghRepo(11, "lib"))

	err := tm.store.ApplyGitHubInstallation(tm.ctx, tm.system, "d-del", GitHubInstallationChange{
		Installation: GitHubInstallation{ID: 1, AccountLogin: "acme"}, Deleted: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(got(tm.store.ListGitHubInstallations(tm.ctx)).must(t)) != 0 {
		t.Fatal("installation was not deleted")
	}
	repos := got(tm.store.ListGitHubRepositories(tm.ctx)).must(t)
	if len(repos) != 2 {
		t.Fatalf("repositories = %d, want them kept", len(repos))
	}
	for _, repo := range repos {
		if repo.Installed {
			t.Errorf("%s is still installed", repo.FullName)
		}
		wantKinds(t, tm.eventsOf(t, TargetRepository, fmt.Sprint(repo.ID)), "github.repository.added", "github.repository.removed")
	}
}

func TestApplyGitHubInstallationUpdatesAccountAndSuspension(t *testing.T) {
	tm := newTeam(t)
	tm.installRepos(t, ghRepo(10, "app"))

	err := tm.store.ApplyGitHubInstallation(tm.ctx, tm.system, "", GitHubInstallationChange{
		Installation: GitHubInstallation{ID: 1, AccountLogin: "acme-renamed", Suspended: true}, Complete: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	installations := got(tm.store.ListGitHubInstallations(tm.ctx)).must(t)
	if len(installations) != 1 || installations[0].AccountLogin != "acme-renamed" || !installations[0].Suspended {
		t.Fatalf("installations = %+v", installations)
	}
}

func TestApplyGitHubInstallationIgnoresDuplicateDelivery(t *testing.T) {
	tm := newTeam(t)
	change := GitHubInstallationChange{Installation: GitHubInstallation{ID: 1, AccountLogin: "acme"}, Repositories: []GitHubRepository{ghRepo(10, "app")}}

	if err := tm.store.ApplyGitHubInstallation(tm.ctx, tm.system, "same", change); err != nil {
		t.Fatal(err)
	}
	before := tm.eventCount(t)

	change.Repositories = []GitHubRepository{ghRepo(11, "lib")}
	if err := tm.store.ApplyGitHubInstallation(tm.ctx, tm.system, "same", change); err != nil {
		t.Fatal(err)
	}
	if tm.eventCount(t) != before || len(got(tm.store.ListGitHubRepositories(tm.ctx)).must(t)) != 1 {
		t.Fatal("a duplicate delivery changed data")
	}
}

func TestApplyGitHubInstallationRefusesNonOwners(t *testing.T) {
	tm := newTeam(t)
	change := GitHubInstallationChange{Installation: GitHubInstallation{ID: 1, AccountLogin: "acme"}, Repositories: []GitHubRepository{ghRepo(10, "app")}}

	for _, caller := range []Caller{tm.member, tm.agent} {
		wantErr(t, tm.store.ApplyGitHubInstallation(tm.ctx, caller, "d", change), ErrForbidden)
	}

	// A refused delivery is not claimed, so the system can still apply it.
	if err := tm.store.ApplyGitHubInstallation(tm.ctx, tm.system, "d", change); err != nil {
		t.Fatal(err)
	}
	if len(got(tm.store.ListGitHubRepositories(tm.ctx)).must(t)) != 1 {
		t.Fatal("the system could not apply the delivery after a refusal")
	}
}

func TestApplyGitHubInstallationValidates(t *testing.T) {
	tm := newTeam(t)
	good := GitHubInstallation{ID: 1, AccountLogin: "acme"}

	wantInvalid(t, tm.store.ApplyGitHubInstallation(tm.ctx, tm.system, "", GitHubInstallationChange{}), "installation")
	wantInvalid(t, tm.store.ApplyGitHubInstallation(tm.ctx, tm.system, "", GitHubInstallationChange{Installation: GitHubInstallation{ID: 1}}), "account_login")

	bad := ghRepo(10, "app")
	bad.URL = "https://example.com/acme/app"
	wantInvalid(t, tm.store.ApplyGitHubInstallation(tm.ctx, tm.system, "", GitHubInstallationChange{Installation: good, Repositories: []GitHubRepository{bad}}), "repository_url")

	bad = ghRepo(10, "app")
	bad.FullName = "noslash"
	wantInvalid(t, tm.store.ApplyGitHubInstallation(tm.ctx, tm.system, "", GitHubInstallationChange{Installation: good, Repositories: []GitHubRepository{bad}}), "repository")

	bad = ghRepo(0, "app")
	wantInvalid(t, tm.store.ApplyGitHubInstallation(tm.ctx, tm.system, "", GitHubInstallationChange{Installation: good, Repositories: []GitHubRepository{bad}}), "repository")

	long := GitHubInstallationChange{Installation: good}
	wantInvalid(t, tm.store.ApplyGitHubInstallation(tm.ctx, tm.system, strings.Repeat("x", 101), long), "delivery")
}

func TestLinkGitHubRepository(t *testing.T) {
	tm := newTeam(t)
	tm.installRepos(t, ghRepo(10, "app"))
	project := tm.project(t, "WP")
	other := tm.project(t, "OT")

	for name, caller := range map[string]Caller{
		"member in the app": {ActorID: tm.member.ActorID, Client: ClientBrowser},
		"owner in the CLI":  {ActorID: tm.owner.ActorID, Client: ClientCLI},
		"owner no client":   {ActorID: tm.owner.ActorID},
		"agent":             {ActorID: tm.agent.ActorID, Client: ClientBrowser},
		"system":            {ActorID: SystemActorID, Client: ClientBrowser},
	} {
		_, err := tm.store.LinkGitHubRepository(tm.ctx, caller, 10, "WP")
		if err == nil {
			t.Errorf("%s was allowed to link", name)
			continue
		}
		wantErr(t, err, ErrForbidden)
	}

	owner := tm.browser(tm.owner)
	linked := got(tm.store.LinkGitHubRepository(tm.ctx, owner, 10, "wp")).must(t)
	if linked.ProjectID != project.ID || linked.ProjectKey != "WP" {
		t.Fatalf("linked = %+v", linked)
	}
	events := tm.eventsOf(t, TargetRepository, "10")
	wantKinds(t, events, "github.repository.added", "github.repository.linked")
	if events[1].ProjectID != project.ID || eventPayload(t, events[1])["project_key"] != "WP" {
		t.Errorf("link event = project %q payload %s", events[1].ProjectID, events[1].Payload)
	}

	// Linking to the same project again is a no-op.
	before := tm.eventCount(t)
	got(tm.store.LinkGitHubRepository(tm.ctx, owner, 10, "WP")).must(t)
	if tm.eventCount(t) != before {
		t.Fatal("relinking the same project recorded an event")
	}

	// Moving to another project is recorded in the new project.
	moved := got(tm.store.LinkGitHubRepository(tm.ctx, owner, 10, "OT")).must(t)
	if moved.ProjectID != other.ID {
		t.Fatalf("moved = %+v", moved)
	}
	if event := tm.lastEvent(t); event.Kind != "github.repository.linked" || event.ProjectID != other.ID {
		t.Fatalf("move event = %+v", event)
	}

	// Unlinking is recorded and clears the project.
	unlinked := got(tm.store.LinkGitHubRepository(tm.ctx, owner, 10, "")).must(t)
	if unlinked.ProjectID != "" || unlinked.ProjectKey != "" {
		t.Fatalf("unlinked = %+v", unlinked)
	}
	event := tm.lastEvent(t)
	payload := eventPayload(t, event)
	if event.Kind != "github.repository.linked" || payload["project_key"] != nil {
		t.Fatalf("unlink event = %s %s", event.Kind, event.Payload)
	}
	if event.ProjectID != other.ID {
		t.Errorf("unlink event project = %q, want the project it left (%q)", event.ProjectID, other.ID)
	}
	before = tm.eventCount(t)
	got(tm.store.LinkGitHubRepository(tm.ctx, owner, 10, "")).must(t)
	if tm.eventCount(t) != before {
		t.Fatal("unlinking an unlinked repository recorded an event")
	}
}

func TestLinkGitHubRepositoryUnknowns(t *testing.T) {
	tm := newTeam(t)
	tm.installRepos(t, ghRepo(10, "app"))
	owner := tm.browser(tm.owner)

	_, err := tm.store.LinkGitHubRepository(tm.ctx, owner, 10, "NOPE")
	wantErr(t, err, ErrNotFound)
	_, err = tm.store.LinkGitHubRepository(tm.ctx, owner, 404, "")
	wantErr(t, err, ErrNotFound)
	if got(tm.store.GetGitHubRepository(tm.ctx, 10)).must(t).ProjectID != "" {
		t.Fatal("a failed link changed the repository")
	}
}

func TestApplyGitHubPullRequestLinksIssueByBranch(t *testing.T) {
	tm := newTeam(t)
	repo := ghRepo(10, "app")
	project := tm.linkedRepo(t, "WP", repo)
	issue := tm.issue(t, project.ID, "Add the thing")

	change := got(tm.store.ApplyGitHubPullRequest(tm.ctx, "d1", ghPull(repo, 100, 1, "feat/wp-1-add-thing"), GitHubAction{Name: "opened", Sender: "octocat"})).must(t)

	if change.Kind != "github.pull_request.opened" {
		t.Fatalf("kind = %q", change.Kind)
	}
	pr := change.PullRequest
	if pr.IssueID != issue.ID || pr.IssueKey != "WP-1" || pr.ProjectID != project.ID || pr.Repository != "acme/app" {
		t.Fatalf("pull request = %+v", pr)
	}
	if pr.ReviewState != ReviewNone || pr.ChecksState != ChecksNone || pr.State != PullRequestOpen || len(pr.RequestedReviewers) != 0 {
		t.Fatalf("pull request defaults = %+v", pr)
	}

	events := tm.eventsOf(t, TargetPullRequest, "100")
	wantKinds(t, events, "github.pull_request.opened")
	event := events[0]
	payload := eventPayload(t, event)
	if event.ProjectID != project.ID || event.ActorID != SystemActorID || event.TargetID != "100" {
		t.Errorf("event = %+v", event)
	}
	if payload["issue_id"] != issue.ID || payload["github_login"] != "octocat" || payload["number"] != float64(1) || payload["repository"] != "acme/app" {
		t.Errorf("payload = %s", event.Payload)
	}

	// The pull request's activity shows up in the issue history through its issue_id.
	history := got(tm.store.ListEvents(tm.ctx, EventFilter{IssueID: issue.ID, Limit: 50})).must(t)
	if !slices.Contains(eventKinds(history), "github.pull_request.opened") {
		t.Errorf("issue history = %v", eventKinds(history))
	}
}

func TestApplyGitHubPullRequestLinksIssueByURL(t *testing.T) {
	tm := newTeam(t)
	repo := ghRepo(10, "app")
	project := tm.linkedRepo(t, "WP", repo)
	issue := tm.issue(t, project.ID, "Something")
	got(tm.store.UpdateIssue(tm.ctx, tm.member, issue.ID, IssueUpdate{PullRequestURL: Change("https://github.com/acme/app/pull/7")})).must(t)

	change := got(tm.store.ApplyGitHubPullRequest(tm.ctx, "d1", ghPull(repo, 100, 7, "unrelated-branch"), GitHubAction{Name: "opened"})).must(t)
	if change.PullRequest.IssueID != issue.ID {
		t.Fatalf("pull request issue = %q, want %q", change.PullRequest.IssueID, issue.ID)
	}

	// A pull request that matches nothing stays unlinked.
	change = got(tm.store.ApplyGitHubPullRequest(tm.ctx, "d2", ghPull(repo, 101, 8, "no-key-here"), GitHubAction{Name: "opened"})).must(t)
	if change.PullRequest.IssueID != "" || change.PullRequest.IssueKey != "" {
		t.Fatalf("pull request linked to %q", change.PullRequest.IssueID)
	}
}

func TestApplyGitHubPullRequestEventKinds(t *testing.T) {
	tm := newTeam(t)
	repo := ghRepo(10, "app")
	tm.linkedRepo(t, "WP", repo)
	apply := func(delivery string, input GitHubPullRequestInput, action GitHubAction) GitHubPullRequestChange {
		t.Helper()
		return got(tm.store.ApplyGitHubPullRequest(tm.ctx, delivery, input, action)).must(t)
	}

	pull := ghPull(repo, 100, 1, "branch")
	apply("d1", pull, GitHubAction{Name: "opened"})

	if change := apply("d2", pull, GitHubAction{Name: "synchronize"}); change.Kind != "github.pull_request.pushed" {
		t.Errorf("synchronize kind = %q", change.Kind)
	}

	// edited records only a title change.
	if change := apply("d3", pull, GitHubAction{Name: "edited"}); change.Kind != "" {
		t.Errorf("edited without a new title kind = %q", change.Kind)
	}
	pull.Title = "Renamed"
	change := apply("d4", pull, GitHubAction{Name: "edited", Sender: "octocat"})
	if change.Kind != "github.pull_request.edited" {
		t.Errorf("edited with a new title kind = %q", change.Kind)
	}
	if got := eventPayload(t, tm.lastEvent(t)); got["previous_title"] != "Pull 1" || got["title"] != "Renamed" {
		t.Errorf("edited payload = %v", got)
	}

	change = apply("d5", pull, GitHubAction{Name: "review_requested", Sender: "octocat", Reviewer: "hubot"})
	if change.Kind != "github.pull_request.review_requested" || eventPayload(t, tm.lastEvent(t))["reviewer"] != "hubot" {
		t.Errorf("review_requested = %q %s", change.Kind, tm.lastEvent(t).Payload)
	}

	for _, name := range []string{"converted_to_draft", "ready_for_review"} {
		if change := apply("d-"+name, pull, GitHubAction{Name: name}); change.Kind != "github.pull_request."+name {
			t.Errorf("%s kind = %q", name, change.Kind)
		}
	}

	// Unknown actions on a known pull request record nothing, but the data is saved.
	pull.Title = "Silent"
	change = apply("d6", pull, GitHubAction{Name: "labeled"})
	if change.Kind != "" || change.PullRequest.Title != "Silent" {
		t.Errorf("labeled = %q %q", change.Kind, change.PullRequest.Title)
	}

	closedAt := githubBaseTime.Add(time.Hour)
	pull.State, pull.ClosedAt, pull.UpdatedAt = PullRequestClosed, &closedAt, closedAt
	change = apply("d7", pull, GitHubAction{Name: "closed", Sender: "octocat"})
	if change.Kind != "github.pull_request.closed" || change.PullRequest.State != PullRequestClosed || change.PullRequest.ClosedAt == nil {
		t.Errorf("closed = %q %+v", change.Kind, change.PullRequest)
	}

	pull.State, pull.ClosedAt = PullRequestOpen, nil
	if change := apply("d8", pull, GitHubAction{Name: "reopened"}); change.Kind != "github.pull_request.reopened" || change.PullRequest.ClosedAt != nil {
		t.Errorf("reopened = %q %+v", change.Kind, change.PullRequest)
	}

	// A pull request seen for the first time through an unrecorded action is still announced.
	other := ghPull(repo, 101, 2, "other")
	if change := apply("d9", other, GitHubAction{Name: "labeled"}); change.Kind != "github.pull_request.updated" {
		t.Errorf("first sight kind = %q", change.Kind)
	}
}

func TestApplyGitHubPullRequestMergeClosesLinkedIssue(t *testing.T) {
	tm := newTeam(t)
	repo := ghRepo(10, "app")
	project := tm.linkedRepo(t, "WP", repo)
	issue := tm.issue(t, project.ID, "Ship it")
	pull := ghPull(repo, 100, 1, "feat/wp-1-ship")
	got(tm.store.ApplyGitHubPullRequest(tm.ctx, "d1", pull, GitHubAction{Name: "opened"})).must(t)

	mergedAt := githubBaseTime.Add(time.Hour)
	pull.State, pull.MergedAt, pull.ClosedAt, pull.MergedByLogin, pull.UpdatedAt = PullRequestMerged, &mergedAt, &mergedAt, "hubot", mergedAt
	change := got(tm.store.ApplyGitHubPullRequest(tm.ctx, "d2", pull, GitHubAction{Name: "closed", Sender: "hubot"})).must(t)

	if change.Kind != "github.pull_request.merged" || change.PullRequest.State != PullRequestMerged || change.PullRequest.MergedAt == nil {
		t.Fatalf("change = %q %+v", change.Kind, change.PullRequest)
	}
	if eventPayload(t, tm.lastEvent(t))["merged_by"] != "hubot" {
		t.Errorf("merged payload = %s", tm.lastEvent(t).Payload)
	}
	done := got(tm.store.GetIssue(tm.ctx, issue.ID)).must(t)
	if done.Status != IssueDone || done.PullRequestURL != pull.URL {
		t.Fatalf("issue = status %s url %q", done.Status, done.PullRequestURL)
	}
	history := got(tm.store.ListEvents(tm.ctx, EventFilter{TargetType: TargetIssue, TargetID: issue.ID, Limit: 50})).must(t)
	var status Event
	for _, event := range history {
		if event.Kind == "issue.status_changed" {
			status = event
		}
	}
	if status.ID == "" || status.ActorID != SystemActorID || eventPayload(t, status)["reason"] != "pull_request_merged" {
		t.Errorf("status event = %+v", status)
	}
}

func TestApplyGitHubPullRequestMergeLeavesClosedIssuesAlone(t *testing.T) {
	tm := newTeam(t)
	repo := ghRepo(10, "app")
	project := tm.linkedRepo(t, "WP", repo)
	done := tm.issue(t, project.ID, "Already done")
	canceled := tm.issue(t, project.ID, "Canceled")
	got(tm.store.UpdateIssue(tm.ctx, tm.member, done.ID, IssueUpdate{Status: Change(IssueDone)})).must(t)
	got(tm.store.UpdateIssue(tm.ctx, tm.member, canceled.ID, IssueUpdate{Status: Change(IssueCanceled)})).must(t)

	merge := func(id int64, number int, branch string) GitHubPullRequestChange {
		pull := ghPull(repo, id, number, branch)
		at := githubBaseTime.Add(time.Hour)
		pull.State, pull.MergedAt, pull.ClosedAt = PullRequestMerged, &at, &at
		return got(tm.store.ApplyGitHubPullRequest(tm.ctx, fmt.Sprint("d", id), pull, GitHubAction{Name: "closed"})).must(t)
	}

	for _, test := range []struct {
		issue  Issue
		status IssueStatus
		pr     int64
		branch string
	}{{done, IssueDone, 100, "feat/wp-1-a"}, {canceled, IssueCanceled, 101, "feat/wp-2-b"}} {
		before := tm.eventCount(t)
		change := merge(test.pr, int(test.pr), test.branch)
		if change.Kind != "github.pull_request.merged" {
			t.Errorf("kind = %q", change.Kind)
		}
		after := got(tm.store.GetIssue(tm.ctx, test.issue.ID)).must(t)
		if after.Status != test.status || after.PullRequestURL != "" {
			t.Errorf("issue %s = status %s url %q", test.issue.Key(), after.Status, after.PullRequestURL)
		}
		// Only the pull request's own event is recorded.
		if tm.eventCount(t) != before+1 {
			t.Errorf("events recorded = %d, want 1", tm.eventCount(t)-before)
		}
	}

	// An existing pull request URL on the issue is kept.
	kept := tm.issue(t, project.ID, "Has url")
	got(tm.store.UpdateIssue(tm.ctx, tm.member, kept.ID, IssueUpdate{PullRequestURL: Change("https://github.com/acme/app/pull/55"), BranchName: Change("odd-branch")})).must(t)
	pull := ghPull(repo, 102, 56, "odd-branch")
	at := githubBaseTime.Add(time.Hour)
	pull.State, pull.MergedAt = PullRequestMerged, &at
	got(tm.store.ApplyGitHubPullRequest(tm.ctx, "d102", pull, GitHubAction{Name: "closed"})).must(t)
	after := got(tm.store.GetIssue(tm.ctx, kept.ID)).must(t)
	if after.Status != IssueDone || after.PullRequestURL != "https://github.com/acme/app/pull/55" {
		t.Errorf("issue = status %s url %q", after.Status, after.PullRequestURL)
	}
}

func TestApplyGitHubPullRequestIgnoresDuplicateDelivery(t *testing.T) {
	tm := newTeam(t)
	repo := ghRepo(10, "app")
	tm.linkedRepo(t, "WP", repo)
	pull := ghPull(repo, 100, 1, "branch")

	first := got(tm.store.ApplyGitHubPullRequest(tm.ctx, "same", pull, GitHubAction{Name: "opened"})).must(t)
	if first.Kind == "" {
		t.Fatal("first delivery recorded nothing")
	}
	before := tm.eventCount(t)

	pull.Title = "Different"
	second := got(tm.store.ApplyGitHubPullRequest(tm.ctx, "same", pull, GitHubAction{Name: "opened"})).must(t)
	if second.Kind != "" || second.PullRequest.ID != 0 {
		t.Fatalf("duplicate change = %+v", second)
	}
	if tm.eventCount(t) != before {
		t.Fatal("duplicate delivery recorded an event")
	}
	if title := got(tm.store.ListGitHubPullRequests(tm.ctx, "WP", "all")).must(t)[0].Title; title != "Pull 1" {
		t.Fatalf("duplicate delivery changed the title to %q", title)
	}
}

func TestApplyGitHubPullRequestValidates(t *testing.T) {
	tm := newTeam(t)
	repo := ghRepo(10, "app")
	apply := func(mutate func(*GitHubPullRequestInput)) error {
		pull := ghPull(repo, 100, 1, "branch")
		mutate(&pull)
		_, err := tm.store.ApplyGitHubPullRequest(tm.ctx, "", pull, GitHubAction{Name: "opened"})
		return err
	}

	wantInvalid(t, apply(func(p *GitHubPullRequestInput) { p.URL = "https://gitlab.com/acme/app/pull/1" }), "pull_request_url")
	wantInvalid(t, apply(func(p *GitHubPullRequestInput) { p.URL = "http://github.com/acme/app/pull/1" }), "pull_request_url")
	wantInvalid(t, apply(func(p *GitHubPullRequestInput) { p.URL = "" }), "pull_request_url")
	wantInvalid(t, apply(func(p *GitHubPullRequestInput) { p.URL = "javascript:alert(1)" }), "pull_request_url")
	wantInvalid(t, apply(func(p *GitHubPullRequestInput) { p.State = "draft" }), "state")
	wantInvalid(t, apply(func(p *GitHubPullRequestInput) { p.State = "" }), "state")
	wantInvalid(t, apply(func(p *GitHubPullRequestInput) { p.ID = 0 }), "pull_request")
	wantInvalid(t, apply(func(p *GitHubPullRequestInput) { p.Number = 0 }), "pull_request")
	wantInvalid(t, apply(func(p *GitHubPullRequestInput) { p.OpenedAt = time.Time{} }), "pull_request")
	wantInvalid(t, apply(func(p *GitHubPullRequestInput) { p.Repository.URL = "https://evil.test/x" }), "repository_url")
	wantInvalid(t, apply(func(p *GitHubPullRequestInput) { p.Title = strings.Repeat("t", 600) }), "title")

	// Nothing was stored by the refused deliveries.
	if len(got(tm.store.ListGitHubRepositories(tm.ctx)).must(t)) != 0 {
		t.Fatal("a refused delivery saved a repository")
	}

	// Titles are collapsed to one line, and reviewers are capped.
	pull := ghPull(repo, 100, 1, "branch")
	pull.Title = "  Fix\n the   thing "
	for i := 0; i < 80; i++ {
		pull.RequestedReviewers = append(pull.RequestedReviewers, fmt.Sprint("reviewer", i))
	}
	change := got(tm.store.ApplyGitHubPullRequest(tm.ctx, "", pull, GitHubAction{Name: "opened"})).must(t)
	if change.PullRequest.Title != "Fix the thing" || len(change.PullRequest.RequestedReviewers) != maxGitHubLogins {
		t.Fatalf("title %q, reviewers %d", change.PullRequest.Title, len(change.PullRequest.RequestedReviewers))
	}
}

func TestApplyGitHubPullRequestEventsOfUnlinkedRepositoryHaveNoProject(t *testing.T) {
	tm := newTeam(t)
	repo := ghRepo(10, "app")
	tm.installRepos(t, repo)

	got(tm.store.ApplyGitHubPullRequest(tm.ctx, "d1", ghPull(repo, 100, 1, "feat/wp-1-x"), GitHubAction{Name: "opened"})).must(t)

	events := tm.eventsOf(t, TargetPullRequest, "100")
	wantKinds(t, events, "github.pull_request.opened")
	if events[0].ProjectID != "" || events[0].ActorID != SystemActorID {
		t.Fatalf("event = %+v", events[0])
	}
}

func TestApplyGitHubReviewKeepsFullPullRequestDetails(t *testing.T) {
	tm := newTeam(t)
	repo := ghRepo(10, "app")
	tm.linkedRepo(t, "WP", repo)
	pull := ghPull(repo, 100, 1, "branch")
	at := githubBaseTime.Add(time.Hour)
	pull.State, pull.MergedAt, pull.MergedByLogin, pull.UpdatedAt = PullRequestMerged, &at, "hubot", at
	got(tm.store.ApplyGitHubPullRequest(tm.ctx, "d1", pull, GitHubAction{Name: "closed"})).must(t)

	partial := ghPull(repo, 100, 1, "branch")
	partial.State, partial.Additions, partial.Deletions, partial.ChangedFiles = PullRequestOpen, 0, 0, 0
	partial.Title, partial.UpdatedAt = "New title", githubBaseTime
	change := got(tm.store.ApplyGitHubReview(tm.ctx, "d2", partial, ghReview(1, "hubot", ReviewApproved, at), GitHubAction{Name: "submitted"})).must(t)

	pr := change.PullRequest
	if pr.Additions != 10 || pr.Deletions != 2 || pr.ChangedFiles != 3 || pr.State != PullRequestMerged || pr.MergedAt == nil || pr.MergedByLogin != "hubot" {
		t.Fatalf("partial delivery changed details: %+v", pr)
	}
	if pr.AuthorLogin != "octocat" || len(pr.RequestedReviewers) != 0 {
		t.Fatalf("partial delivery changed author or reviewers: %+v", pr)
	}
	if pr.Title != "New title" {
		t.Errorf("title = %q, want the reported one", pr.Title)
	}
	if !pr.UpdatedAt.Equal(at) {
		t.Errorf("updated_at went backwards to %v", pr.UpdatedAt)
	}
	if pr.ReviewState != ReviewApproved || change.Kind != "github.review.submitted" {
		t.Errorf("review = %q kind %q", pr.ReviewState, change.Kind)
	}
}

func TestApplyGitHubReviewForUnknownPullRequestCreatesIt(t *testing.T) {
	tm := newTeam(t)
	repo := ghRepo(10, "app")
	tm.linkedRepo(t, "WP", repo)

	change := got(tm.store.ApplyGitHubReview(tm.ctx, "d1", ghPull(repo, 100, 1, "b"), ghReview(1, "hubot", ReviewChangesRequested, githubBaseTime), GitHubAction{Name: "submitted"})).must(t)
	if change.PullRequest.ID != 100 || change.PullRequest.ReviewState != ReviewChangesRequested {
		t.Fatalf("pull request = %+v", change.PullRequest)
	}
}

func TestApplyGitHubReviewSummaryRules(t *testing.T) {
	tm := newTeam(t)
	repo := ghRepo(10, "app")
	tm.linkedRepo(t, "WP", repo)
	pull := ghPull(repo, 100, 1, "branch")
	got(tm.store.ApplyGitHubPullRequest(tm.ctx, "d0", pull, GitHubAction{Name: "opened"})).must(t)

	var id int64
	var delivery int
	review := func(login, state string) GitHubPullRequestChange {
		t.Helper()
		id++
		delivery++
		at := githubBaseTime.Add(time.Duration(id) * time.Minute)
		return got(tm.store.ApplyGitHubReview(tm.ctx, fmt.Sprint("r", delivery), pull, ghReview(id, login, state, at), GitHubAction{Name: "submitted"})).must(t)
	}
	want := func(change GitHubPullRequestChange, state string) {
		t.Helper()
		if change.PullRequest.ReviewState != state {
			t.Fatalf("review state = %q, want %q", change.PullRequest.ReviewState, state)
		}
	}

	want(review("alice", ReviewCommented), ReviewCommented)
	want(review("bob", ReviewApproved), ReviewApproved)
	want(review("carol", ReviewChangesRequested), ReviewChangesRequested)
	want(review("dave", ReviewApproved), ReviewChangesRequested)
	// A later approval by the same reviewer replaces their change request, whatever the case.
	want(review("Carol", ReviewApproved), ReviewApproved)
	want(review("erin", ReviewChangesRequested), ReviewChangesRequested)
	// A plain comment after a change request does not retract it.
	want(review("erin", ReviewCommented), ReviewChangesRequested)
}

func TestApplyGitHubReviewDismissedAndEdited(t *testing.T) {
	tm := newTeam(t)
	repo := ghRepo(10, "app")
	tm.linkedRepo(t, "WP", repo)
	pull := ghPull(repo, 100, 1, "branch")
	at := githubBaseTime.Add(time.Minute)

	got(tm.store.ApplyGitHubReview(tm.ctx, "r1", pull, ghReview(1, "alice", ReviewChangesRequested, at), GitHubAction{Name: "submitted"})).must(t)
	got(tm.store.ApplyGitHubReview(tm.ctx, "r2", pull, ghReview(2, "bob", ReviewApproved, at.Add(time.Minute)), GitHubAction{Name: "submitted"})).must(t)

	// An edited review changes its state but is not an event worth recording.
	before := tm.eventCount(t)
	change := got(tm.store.ApplyGitHubReview(tm.ctx, "r3", pull, ghReview(2, "bob", ReviewCommented, at.Add(time.Minute)), GitHubAction{Name: "edited"})).must(t)
	if change.Kind != "" || tm.eventCount(t) != before {
		t.Fatalf("edit recorded %q", change.Kind)
	}
	if change.PullRequest.ReviewState != ReviewChangesRequested {
		t.Errorf("state = %q", change.PullRequest.ReviewState)
	}

	// Dismissing the change request records a dismissal and falls back to the rest.
	change = got(tm.store.ApplyGitHubReview(tm.ctx, "r4", pull, ghReview(1, "alice", ReviewDismissed, at), GitHubAction{Name: "dismissed", Sender: "maintainer"})).must(t)
	if change.Kind != "github.review.dismissed" {
		t.Fatalf("kind = %q", change.Kind)
	}
	if change.PullRequest.ReviewState != ReviewCommented {
		t.Errorf("state after dismissal = %q, want commented", change.PullRequest.ReviewState)
	}
	event := tm.lastEvent(t)
	payload := eventPayload(t, event)
	if event.Kind != "github.review.dismissed" || payload["state"] != ReviewDismissed || payload["github_login"] != "alice" {
		t.Errorf("dismissal event = %s %s", event.Kind, event.Payload)
	}
}

func TestApplyGitHubReviewValidates(t *testing.T) {
	tm := newTeam(t)
	repo := ghRepo(10, "app")
	pull := ghPull(repo, 100, 1, "branch")
	apply := func(mutate func(*GitHubReviewInput)) error {
		review := ghReview(1, "alice", ReviewApproved, githubBaseTime)
		mutate(&review)
		_, err := tm.store.ApplyGitHubReview(tm.ctx, "", pull, review, GitHubAction{Name: "submitted"})
		return err
	}

	wantInvalid(t, apply(func(r *GitHubReviewInput) { r.ID = 0 }), "review")
	wantInvalid(t, apply(func(r *GitHubReviewInput) { r.State = "pending" }), "review_state")
	wantInvalid(t, apply(func(r *GitHubReviewInput) { r.ReviewerLogin = "" }), "reviewer")
	wantInvalid(t, apply(func(r *GitHubReviewInput) { r.URL = "https://example.com/r" }), "review_url")
	wantInvalid(t, apply(func(r *GitHubReviewInput) { r.SubmittedAt = time.Time{} }), "review")
}

func TestRecordGitHubComment(t *testing.T) {
	tm := newTeam(t)
	repo := ghRepo(10, "app")
	project := tm.linkedRepo(t, "WP", repo)
	got(tm.store.ApplyGitHubPullRequest(tm.ctx, "d0", ghPull(repo, 100, 1, "branch"), GitHubAction{Name: "opened"})).must(t)
	url := "https://github.com/acme/app/pull/1#issuecomment-5"

	long := strings.Repeat("word ", 200)
	if err := tm.store.RecordGitHubComment(tm.ctx, "c1", 10, 1, "hubot", url, "  Looks\n  good\t to me "); err != nil {
		t.Fatal(err)
	}
	event := tm.lastEvent(t)
	payload := eventPayload(t, event)
	if event.Kind != "github.comment.created" || event.TargetType != TargetPullRequest || event.TargetID != "100" ||
		event.ProjectID != project.ID || event.ActorID != SystemActorID {
		t.Fatalf("event = %+v", event)
	}
	if payload["body_excerpt"] != "Looks good to me" || payload["github_login"] != "hubot" || payload["comment_url"] != url {
		t.Errorf("payload = %s", event.Payload)
	}

	if err := tm.store.RecordGitHubComment(tm.ctx, "c2", 10, 1, "hubot", url, long); err != nil {
		t.Fatal(err)
	}
	excerpt := eventPayload(t, tm.lastEvent(t))["body_excerpt"].(string)
	if runes := []rune(excerpt); len(runes) != maxCommentQuote || !strings.HasSuffix(excerpt, "…") {
		t.Errorf("excerpt has %d runes, ends %q", len(runes), excerpt[max(0, len(excerpt)-6):])
	}

	// Multibyte text is cut on a character, never inside one.
	if err := tm.store.RecordGitHubComment(tm.ctx, "c3", 10, 1, "hubot", url, strings.Repeat("é", 400)); err != nil {
		t.Fatal(err)
	}
	if excerpt := eventPayload(t, tm.lastEvent(t))["body_excerpt"].(string); len([]rune(excerpt)) != maxCommentQuote {
		t.Errorf("multibyte excerpt has %d runes", len([]rune(excerpt)))
	}

	// A redelivery records nothing.
	before := tm.eventCount(t)
	if err := tm.store.RecordGitHubComment(tm.ctx, "c1", 10, 1, "hubot", url, "again"); err != nil || tm.eventCount(t) != before {
		t.Fatalf("duplicate comment: err %v, events +%d", err, tm.eventCount(t)-before)
	}

	// Unknown pull requests, and a number in another repository, are not found.
	wantErr(t, tm.store.RecordGitHubComment(tm.ctx, "c4", 10, 99, "hubot", url, "x"), ErrNotFound)
	wantErr(t, tm.store.RecordGitHubComment(tm.ctx, "c5", 11, 1, "hubot", url, "x"), ErrNotFound)
	// A refused comment does not use up its delivery ID.
	var count int
	if err := tm.store.db.QueryRow(`SELECT count(*) FROM github_deliveries WHERE id IN ('c4', 'c5')`).Scan(&count); err != nil || count != 0 {
		t.Errorf("claimed deliveries = %d (%v)", count, err)
	}

	wantInvalid(t, tm.store.RecordGitHubComment(tm.ctx, "c6", 10, 1, "hubot", "https://example.com/c", "x"), "comment_url")
	wantInvalid(t, tm.store.RecordGitHubComment(tm.ctx, "c7", 10, 1, "", url, "x"), "github_login")
}

func TestSetGitHubChecks(t *testing.T) {
	tm := newTeam(t)
	repo := ghRepo(10, "app")
	project := tm.linkedRepo(t, "WP", repo)
	pull := ghPull(repo, 100, 1, "branch")
	got(tm.store.ApplyGitHubPullRequest(tm.ctx, "d1", pull, GitHubAction{Name: "opened"})).must(t)
	second := ghPull(repo, 101, 2, "other")
	second.HeadSHA = "sha-2"
	got(tm.store.ApplyGitHubPullRequest(tm.ctx, "d2", second, GitHubAction{Name: "opened"})).must(t)

	changes := got(tm.store.SetGitHubChecks(tm.ctx, 10, "sha-1", ChecksPending)).must(t)
	if len(changes) != 1 || changes[0].Kind != "github.checks.changed" || changes[0].PullRequest.ID != 100 || changes[0].PullRequest.ChecksState != ChecksPending {
		t.Fatalf("changes = %+v", changes)
	}
	event := tm.lastEvent(t)
	payload := eventPayload(t, event)
	if event.TargetType != TargetPullRequest || event.TargetID != "100" || event.ProjectID != project.ID || event.ActorID != SystemActorID ||
		payload["checks_state"] != ChecksPending || payload["previous_checks_state"] != ChecksNone {
		t.Fatalf("event = %+v %s", event, event.Payload)
	}

	// The same state again, another commit, and another repository change nothing.
	before := tm.eventCount(t)
	for _, call := range []struct {
		repo int64
		sha  string
	}{{10, "sha-1"}, {10, "unknown"}, {11, "sha-1"}} {
		if changes := got(tm.store.SetGitHubChecks(tm.ctx, call.repo, call.sha, ChecksPending)).must(t); len(changes) != 0 {
			t.Errorf("%+v changed %d pull requests", call, len(changes))
		}
	}
	if tm.eventCount(t) != before {
		t.Fatal("unchanged checks recorded events")
	}
	if got(tm.store.ListGitHubPullRequests(tm.ctx, "WP", "all")).must(t)[0].ChecksState == "" {
		t.Fatal("checks not listed")
	}

	changes = got(tm.store.SetGitHubChecks(tm.ctx, 10, "sha-1", ChecksFailure)).must(t)
	if len(changes) != 1 || eventPayload(t, tm.lastEvent(t))["previous_checks_state"] != ChecksPending {
		t.Fatalf("changes = %+v", changes)
	}

	wantInvalid(t, func() error { _, err := tm.store.SetGitHubChecks(tm.ctx, 10, "sha-1", "error"); return err }(), "checks_state")

	// A push moves the head, so the earlier result no longer applies.
	pull.HeadSHA = "sha-3"
	change := got(tm.store.ApplyGitHubPullRequest(tm.ctx, "d3", pull, GitHubAction{Name: "synchronize"})).must(t)
	if change.PullRequest.ChecksState != ChecksNone {
		t.Fatalf("checks after push = %q", change.PullRequest.ChecksState)
	}
	// Delivering the same head again keeps its checks.
	got(tm.store.SetGitHubChecks(tm.ctx, 10, "sha-3", ChecksSuccess)).must(t)
	change = got(tm.store.ApplyGitHubPullRequest(tm.ctx, "d4", pull, GitHubAction{Name: "edited"})).must(t)
	if change.PullRequest.ChecksState != ChecksSuccess {
		t.Fatalf("checks after same-head delivery = %q", change.PullRequest.ChecksState)
	}
}

func TestSyncGitHubPullRequests(t *testing.T) {
	tm := newTeam(t)
	repo := ghRepo(10, "app")
	project := tm.linkedRepo(t, "WP", repo)
	issue := tm.issue(t, project.ID, "Synced")

	pulls := []GitHubPullRequestInput{ghPull(repo, 100, 1, "feat/wp-1-x"), ghPull(repo, 101, 2, "b"), ghPull(repo, 102, 3, "c")}
	pulls[2].State = PullRequestClosed
	reviews := map[int64][]GitHubReviewInput{
		100: {ghReview(1, "alice", ReviewApproved, githubBaseTime.Add(time.Minute)), ghReview(2, "bob", ReviewChangesRequested, githubBaseTime.Add(2*time.Minute))},
		101: {ghReview(3, "alice", ReviewCommented, githubBaseTime.Add(time.Minute))},
	}
	before := tm.eventCount(t)

	count := got(tm.store.SyncGitHubPullRequests(tm.ctx, tm.owner, 10, pulls, reviews)).must(t)
	if count != 3 {
		t.Fatalf("count = %d", count)
	}
	if tm.eventCount(t) != before+1 {
		t.Fatalf("events recorded = %d, want 1", tm.eventCount(t)-before)
	}
	event := tm.lastEvent(t)
	if event.Kind != "github.repository.synced" || event.TargetType != TargetRepository || event.TargetID != "10" ||
		event.ProjectID != project.ID || event.ActorID != tm.owner.ActorID || eventPayload(t, event)["count"] != float64(3) {
		t.Fatalf("event = %+v %s", event, event.Payload)
	}

	listed := got(tm.store.ListGitHubPullRequests(tm.ctx, "WP", "all")).must(t)
	byNumber := map[int]GitHubPullRequest{}
	for _, pr := range listed {
		byNumber[pr.Number] = pr
	}
	if len(listed) != 3 || byNumber[1].ReviewState != ReviewChangesRequested || len(byNumber[1].Reviews) != 2 ||
		byNumber[2].ReviewState != ReviewCommented || byNumber[3].ReviewState != ReviewNone || byNumber[1].IssueID != issue.ID {
		t.Fatalf("listed = %+v", listed)
	}

	// Syncing again updates in place.
	pulls[0].Title = "Updated"
	got(tm.store.SyncGitHubPullRequests(tm.ctx, tm.system, 10, pulls[:1], reviews)).must(t)
	if got(tm.store.ListGitHubPullRequests(tm.ctx, "WP", "all")).must(t)[0].Title == "" || len(got(tm.store.ListGitHubPullRequests(tm.ctx, "WP", "all")).must(t)) != 3 {
		t.Fatal("resync duplicated or lost pull requests")
	}
}

func TestSyncGitHubPullRequestsWithoutReviews(t *testing.T) {
	tm := newTeam(t)
	repo := ghRepo(10, "app")
	tm.linkedRepo(t, "WP", repo)

	if n := got(tm.store.SyncGitHubPullRequests(tm.ctx, tm.owner, 10, []GitHubPullRequestInput{ghPull(repo, 100, 1, "b")}, nil)).must(t); n != 1 {
		t.Fatalf("count = %d", n)
	}
	if n := got(tm.store.SyncGitHubPullRequests(tm.ctx, tm.owner, 10, nil, nil)).must(t); n != 0 {
		t.Fatalf("count = %d", n)
	}
}

func TestSyncGitHubPullRequestsRefusals(t *testing.T) {
	tm := newTeam(t)
	repo := ghRepo(10, "app")
	tm.linkedRepo(t, "WP", repo)
	pulls := []GitHubPullRequestInput{ghPull(repo, 100, 1, "b")}

	_, err := tm.store.SyncGitHubPullRequests(tm.ctx, tm.member, 10, pulls, map[int64][]GitHubReviewInput{})
	wantErr(t, err, ErrForbidden)
	_, err = tm.store.SyncGitHubPullRequests(tm.ctx, tm.agent, 10, pulls, map[int64][]GitHubReviewInput{})
	wantErr(t, err, ErrForbidden)
	_, err = tm.store.SyncGitHubPullRequests(tm.ctx, tm.owner, 404, nil, map[int64][]GitHubReviewInput{})
	wantErr(t, err, ErrNotFound)

	elsewhere := ghPull(ghRepo(11, "lib"), 200, 1, "b")
	_, err = tm.store.SyncGitHubPullRequests(tm.ctx, tm.owner, 10, []GitHubPullRequestInput{elsewhere}, map[int64][]GitHubReviewInput{})
	wantInvalid(t, err, "repository")

	badReview := map[int64][]GitHubReviewInput{100: {ghReview(1, "alice", "bogus", githubBaseTime)}}
	_, err = tm.store.SyncGitHubPullRequests(tm.ctx, tm.owner, 10, pulls, badReview)
	wantInvalid(t, err, "review_state")

	if len(got(tm.store.ListGitHubPullRequests(tm.ctx, "WP", "all")).must(t)) != 0 {
		t.Fatal("a refused sync saved pull requests")
	}
}

func TestListGitHubPullRequests(t *testing.T) {
	tm := newTeam(t)
	repo := ghRepo(10, "app")
	tm.linkedRepo(t, "WP", repo)
	otherRepo := ghRepo(11, "lib")
	tm.installRepos(t, repo, otherRepo)
	tm.project(t, "OT")
	got(tm.store.LinkGitHubRepository(tm.ctx, tm.browser(tm.owner), 11, "OT")).must(t)
	got(tm.store.SetGitHubLogin(tm.ctx, tm.member, tm.member.ActorID, "MaxDev")).must(t)
	issue := tm.issue(t, got(tm.store.GetGitHubRepository(tm.ctx, 10)).must(t).ProjectID, "Mine")

	add := func(id int64, number int, state string, age time.Duration, author string) {
		pull := ghPull(repo, id, number, "branch")
		if id == 100 {
			pull.HeadRef = "feat/wp-1-mine"
		}
		pull.State, pull.AuthorLogin, pull.UpdatedAt = state, author, githubBaseTime.Add(age)
		got(tm.store.ApplyGitHubPullRequest(tm.ctx, "", pull, GitHubAction{Name: "labeled"})).must(t)
	}
	add(100, 1, PullRequestOpen, 3*time.Hour, "maxdev")
	add(101, 2, PullRequestMerged, 5*time.Hour, "stranger")
	add(102, 3, PullRequestClosed, time.Hour, "octocat")
	add(103, 4, PullRequestOpen, 4*time.Hour, "octocat")
	other := ghPull(otherRepo, 200, 1, "x")
	got(tm.store.ApplyGitHubPullRequest(tm.ctx, "", other, GitHubAction{Name: "opened"})).must(t)
	at := githubBaseTime.Add(time.Minute)
	got(tm.store.ApplyGitHubReview(tm.ctx, "", ghPull(repo, 100, 1, "feat/wp-1-mine"), ghReview(9, "alice", ReviewApproved, at), GitHubAction{Name: "submitted"})).must(t)

	numbers := func(state string) []int {
		var out []int
		for _, pr := range got(tm.store.ListGitHubPullRequests(tm.ctx, "wp", state)).must(t) {
			out = append(out, pr.Number)
		}
		return out
	}
	for state, want := range map[string][]int{
		"all":    {2, 4, 1, 3},
		"open":   {4, 1},
		"merged": {2},
		"closed": {3},
	} {
		if have := numbers(state); !slices.Equal(have, want) {
			t.Errorf("%s = %v, want %v", state, have, want)
		}
	}

	var mine GitHubPullRequest
	for _, pr := range got(tm.store.ListGitHubPullRequests(tm.ctx, "WP", "all")).must(t) {
		if pr.Number == 1 {
			mine = pr
		}
	}
	if mine.AuthorPersonID != tm.member.ActorID || mine.IssueKey != "WP-1" || mine.IssueID != issue.ID || mine.Repository != "acme/app" {
		t.Errorf("pull request = %+v", mine)
	}
	if len(mine.Reviews) != 1 || mine.Reviews[0].ReviewerLogin != "alice" || mine.Reviews[0].State != ReviewApproved {
		t.Errorf("reviews = %+v", mine.Reviews)
	}
	for _, pr := range got(tm.store.ListGitHubPullRequests(tm.ctx, "WP", "all")).must(t) {
		if pr.Number != 1 && (pr.AuthorPersonID != "" || pr.IssueKey != "" || pr.Reviews == nil || len(pr.Reviews) != 0) {
			t.Errorf("pull request %d = %+v", pr.Number, pr)
		}
	}
	if have := got(tm.store.ListGitHubPullRequests(tm.ctx, "OT", "all")).must(t); len(have) != 1 || have[0].Number != 1 || have[0].Repository != "acme/lib" {
		t.Errorf("other project = %+v", have)
	}

	_, err := tm.store.ListGitHubPullRequests(tm.ctx, "WP", "draft")
	wantInvalid(t, err, "state")
	_, err = tm.store.ListGitHubPullRequests(tm.ctx, "NOPE", "all")
	wantErr(t, err, ErrNotFound)
}

func TestSetGitHubLogin(t *testing.T) {
	tm := newTeam(t)
	member, owner := tm.member.ActorID, tm.owner.ActorID

	account := got(tm.store.SetGitHubLogin(tm.ctx, tm.member, member, " @MaxDev ")).must(t)
	if account == nil || account.Login != "MaxDev" || account.PersonID != member {
		t.Fatalf("account = %+v", account)
	}
	event := tm.lastEvent(t)
	if event.Kind != "github.account.linked" || event.TargetType != TargetActor || event.TargetID != member || event.ActorID != member {
		t.Fatalf("event = %+v", event)
	}
	if payload := eventPayload(t, event); payload["login"] != "MaxDev" || payload["previous_login"] != nil {
		t.Errorf("payload = %v", payload)
	}

	// Saving the same login records nothing; changing it records the previous one.
	before := tm.eventCount(t)
	got(tm.store.SetGitHubLogin(tm.ctx, tm.member, member, "MaxDev")).must(t)
	if tm.eventCount(t) != before {
		t.Fatal("an unchanged login recorded an event")
	}
	got(tm.store.SetGitHubLogin(tm.ctx, tm.member, member, "max-2")).must(t)
	if payload := eventPayload(t, tm.lastEvent(t)); payload["previous_login"] != "MaxDev" || payload["login"] != "max-2" {
		t.Errorf("payload = %v", payload)
	}

	// Owners set anyone's; members set only their own.
	got(tm.store.SetGitHubLogin(tm.ctx, tm.owner, member, "max-3")).must(t)
	if event := tm.lastEvent(t); event.ActorID != owner {
		t.Errorf("actor = %s, want the owner", event.ActorID)
	}
	_, err := tm.store.SetGitHubLogin(tm.ctx, tm.member, owner, "olive")
	wantErr(t, err, ErrForbidden)
	_, err = tm.store.SetGitHubLogin(tm.ctx, tm.agent, member, "agent-set")
	wantErr(t, err, ErrForbidden)
	_, err = tm.store.SetGitHubLogin(tm.ctx, tm.system, member, "system-set")
	wantErr(t, err, ErrForbidden)

	// Another person cannot take a login in use, in any letter case.
	got(tm.store.SetGitHubLogin(tm.ctx, tm.owner, owner, "Olive")).must(t)
	_, err = tm.store.SetGitHubLogin(tm.ctx, tm.member, member, "olive")
	wantErr(t, err, ErrConflict)
	if accounts := got(tm.store.ListGitHubAccounts(tm.ctx)).must(t); len(accounts) != 2 || accounts[0].Login != "max-3" || accounts[1].Login != "Olive" {
		t.Errorf("accounts = %+v", accounts)
	}

	// An empty login clears the link, once.
	if cleared := got(tm.store.SetGitHubLogin(tm.ctx, tm.member, member, "  ")).must(t); cleared != nil {
		t.Fatalf("cleared = %+v", cleared)
	}
	event = tm.lastEvent(t)
	if payload := eventPayload(t, event); event.Kind != "github.account.linked" || payload["login"] != nil || payload["previous_login"] != "max-3" {
		t.Errorf("clear event = %s", event.Payload)
	}
	before = tm.eventCount(t)
	got(tm.store.SetGitHubLogin(tm.ctx, tm.member, member, "")).must(t)
	if tm.eventCount(t) != before {
		t.Fatal("clearing a missing login recorded an event")
	}
	// The freed login is available again.
	got(tm.store.SetGitHubLogin(tm.ctx, tm.member, member, "max-3")).must(t)
}

func TestSetGitHubLoginValidates(t *testing.T) {
	tm := newTeam(t)
	for _, login := range []string{"-lead", "trail-", "has space", "dou--ble", "under_score", "dot.ted", strings.Repeat("a", 40), "name/x", "@@name", "émile"} {
		_, err := tm.store.SetGitHubLogin(tm.ctx, tm.member, tm.member.ActorID, login)
		if err == nil {
			t.Errorf("login %q was accepted", login)
			continue
		}
		wantInvalid(t, err, "login")
	}
	for _, login := range []string{"a", "a-b", "A1", strings.Repeat("a", 39)} {
		got(tm.store.SetGitHubLogin(tm.ctx, tm.member, tm.member.ActorID, login)).must(t)
	}

	_, err := tm.store.SetGitHubLogin(tm.ctx, tm.owner, tm.agent.ActorID, "agentlogin")
	wantInvalid(t, err, "person")
	_, err = tm.store.SetGitHubLogin(tm.ctx, tm.owner, "nobody", "ghost")
	wantErr(t, err, ErrNotFound)

	if err := tm.store.DisableActor(tm.ctx, tm.owner, tm.member.ActorID); err != nil {
		t.Fatal(err)
	}
	_, err = tm.store.SetGitHubLogin(tm.ctx, tm.owner, tm.member.ActorID, "disabled-one")
	wantInvalid(t, err, "person")
}

// discordMember links the member to a Discord account in an approved guild and returns the
// caller that account acts as.
func (tm *team) discordMember(t *testing.T, guildID, userID string) Caller {
	t.Helper()
	source := DiscordIdentity{GuildID: guildID, UserID: userID, Configured: true}
	err := tm.store.SaveDiscordSetup(tm.ctx, source, DiscordSetup{InteractionID: "link-" + guildID + userID, UserID: userID, UserName: "max", PersonID: tm.member.ActorID})
	if err != nil {
		t.Fatalf("link Discord account: %v", err)
	}
	caller, _, err := tm.store.DiscordCaller(tm.ctx, source, "")
	if err != nil {
		t.Fatalf("Discord caller: %v", err)
	}
	return caller
}

func TestSetGitHubLoginByDiscordAgent(t *testing.T) {
	tm := newTeam(t)
	discord := tm.discordMember(t, "111", "200")

	account := got(tm.store.SetGitHubLogin(tm.ctx, discord, tm.member.ActorID, "maxdev")).must(t)
	if account == nil || account.Login != "maxdev" {
		t.Fatalf("account = %+v", account)
	}
	event := tm.lastEvent(t)
	if event.ActorID != discord.ActorID || event.ActorID == tm.member.ActorID {
		t.Fatalf("actor = %s, want the Discord agent %s", event.ActorID, discord.ActorID)
	}

	_, err := tm.store.SetGitHubLogin(tm.ctx, discord, tm.owner.ActorID, "olive")
	wantErr(t, err, ErrForbidden)

	// The same agent reached some other way gets no special right over its owner's login.
	plain := Caller{ActorID: discord.ActorID}
	_, err = tm.store.SetGitHubLogin(tm.ctx, plain, tm.member.ActorID, "other")
	wantErr(t, err, ErrForbidden)
}

func TestDiscordProjectChannels(t *testing.T) {
	tm := newTeam(t)
	project := tm.project(t, "WP")
	tm.project(t, "OT")
	approved := DiscordIdentity{GuildID: "111", UserID: "200", Configured: true}

	// A server that is unknown or not approved cannot set a channel.
	_, err := tm.store.SetDiscordProjectChannel(tm.ctx, DiscordIdentity{GuildID: "999", UserID: "200"}, "WP", "300")
	wantErr(t, err, ErrForbidden)
	if err := tm.store.RequestDiscordGuild(tm.ctx, DiscordIdentity{GuildID: "222", UserID: "200"}, "Pending guild"); err != nil {
		t.Fatal(err)
	}
	_, err = tm.store.SetDiscordProjectChannel(tm.ctx, DiscordIdentity{GuildID: "222", UserID: "200"}, "WP", "300")
	wantErr(t, err, ErrForbidden)
	if len(got(tm.store.ListDiscordProjectChannels(tm.ctx)).must(t)) != 0 {
		t.Fatal("a refused request saved a channel")
	}

	// Input checks.
	_, err = tm.store.SetDiscordProjectChannel(tm.ctx, approved, "WP", "not-a-number")
	wantInvalid(t, err, "channel")
	_, err = tm.store.SetDiscordProjectChannel(tm.ctx, approved, "NOPE", "300")
	wantErr(t, err, ErrNotFound)

	// An unlinked member of an approved server acts as the system.
	saved := got(tm.store.SetDiscordProjectChannel(tm.ctx, approved, "wp", "300")).must(t)
	if saved.ID != project.ID {
		t.Fatalf("project = %+v", saved)
	}
	event := tm.lastEvent(t)
	payload := eventPayload(t, event)
	if event.Kind != "discord.channel.set" || event.ProjectID != project.ID || event.ActorID != SystemActorID ||
		payload["channel_id"] != "300" || payload["guild_id"] != "111" || payload["discord_user_id"] != "200" {
		t.Fatalf("event = %+v %s", event, event.Payload)
	}

	// A linked member acts as their Discord agent.
	discord := tm.discordMember(t, "111", "201")
	got(tm.store.SetDiscordProjectChannel(tm.ctx, DiscordIdentity{GuildID: "111", UserID: "201", Configured: true}, "OT", "301")).must(t)
	if event := tm.lastEvent(t); event.ActorID != discord.ActorID {
		t.Errorf("actor = %s, want %s", event.ActorID, discord.ActorID)
	}

	// Setting again replaces the channel.
	got(tm.store.SetDiscordProjectChannel(tm.ctx, approved, "WP", "302")).must(t)
	channel := got(tm.store.DiscordChannelForProject(tm.ctx, project.ID)).must(t)
	if channel.ChannelID != "302" || channel.GuildID != "111" || channel.ProjectKey != "WP" || channel.GuildName != "" {
		t.Fatalf("channel = %+v", channel)
	}

	// An approved server shows its name.
	got(tm.store.SetDiscordGuildApproval(tm.ctx, tm.owner, "222", false, DiscordGuildApproved)).must(t)
	got(tm.store.SetDiscordProjectChannel(tm.ctx, DiscordIdentity{GuildID: "222", UserID: "200"}, "WP", "303")).must(t)
	if channel := got(tm.store.DiscordChannelForProject(tm.ctx, project.ID)).must(t); channel.GuildName != "Pending guild" || channel.ChannelID != "303" {
		t.Fatalf("channel = %+v", channel)
	}

	channels := got(tm.store.ListDiscordProjectChannels(tm.ctx)).must(t)
	if len(channels) != 2 || channels[0].ProjectKey != "OT" || channels[0].ChannelID != "301" || channels[1].ProjectKey != "WP" {
		t.Fatalf("channels = %+v", channels)
	}
	_, err = tm.store.DiscordChannelForProject(tm.ctx, "no-such-project")
	wantErr(t, err, ErrNotFound)
}

func TestRemoveDiscordProjectChannel(t *testing.T) {
	tm := newTeam(t)
	project := tm.project(t, "WP")
	got(tm.store.SetDiscordProjectChannel(tm.ctx, DiscordIdentity{GuildID: "111", UserID: "200", Configured: true}, "WP", "300")).must(t)

	for name, caller := range map[string]Caller{
		"member in the app": {ActorID: tm.member.ActorID, Client: ClientBrowser},
		"owner in the CLI":  {ActorID: tm.owner.ActorID, Client: ClientCLI},
		"owner no client":   {ActorID: tm.owner.ActorID},
		"system":            {ActorID: SystemActorID, Client: ClientBrowser},
	} {
		wantErrNamed(t, name, tm.store.RemoveDiscordProjectChannel(tm.ctx, caller, "WP"), ErrForbidden)
	}
	wantErr(t, tm.store.RemoveDiscordProjectChannel(tm.ctx, tm.browser(tm.owner), "NOPE"), ErrNotFound)

	if err := tm.store.RemoveDiscordProjectChannel(tm.ctx, tm.browser(tm.owner), "wp"); err != nil {
		t.Fatal(err)
	}
	event := tm.lastEvent(t)
	if event.Kind != "discord.channel.removed" || event.ProjectID != project.ID || event.ActorID != tm.owner.ActorID {
		t.Fatalf("event = %+v", event)
	}
	_, err := tm.store.DiscordChannelForProject(tm.ctx, project.ID)
	wantErr(t, err, ErrNotFound)
	wantErr(t, tm.store.RemoveDiscordProjectChannel(tm.ctx, tm.browser(tm.owner), "WP"), ErrNotFound)
}

func wantErrNamed(t *testing.T, name string, err, target error) {
	t.Helper()
	if err == nil {
		t.Errorf("%s: no error, want %v", name, target)
		return
	}
	wantErr(t, err, target)
}

func TestDiscordUsersForGitHubLogins(t *testing.T) {
	tm := newTeam(t)
	tm.discordMember(t, "111", "200")
	got(tm.store.SetGitHubLogin(tm.ctx, tm.member, tm.member.ActorID, "MaxDev")).must(t)
	// The owner has a GitHub login but no Discord link.
	got(tm.store.SetGitHubLogin(tm.ctx, tm.owner, tm.owner.ActorID, "olive")).must(t)

	users := got(tm.store.DiscordUsersForGitHubLogins(tm.ctx, "111", []string{"maxdev", "olive", "stranger", ""})).must(t)
	if len(users) != 1 || users["maxdev"] != "200" {
		t.Fatalf("users = %v", users)
	}

	// Keys are lower case whatever case the caller used.
	users = got(tm.store.DiscordUsersForGitHubLogins(tm.ctx, "111", []string{"MAXDEV"})).must(t)
	if users["maxdev"] != "200" {
		t.Fatalf("users = %v", users)
	}

	// A different server has no link for this teammate.
	if users := got(tm.store.DiscordUsersForGitHubLogins(tm.ctx, "999", []string{"maxdev"})).must(t); len(users) != 0 {
		t.Fatalf("other guild users = %v", users)
	}

	// A disabled teammate is left out.
	if err := tm.store.DisableActor(tm.ctx, tm.owner, tm.member.ActorID); err != nil {
		t.Fatal(err)
	}
	if users := got(tm.store.DiscordUsersForGitHubLogins(tm.ctx, "111", []string{"maxdev"})).must(t); len(users) != 0 {
		t.Fatalf("disabled teammate users = %v", users)
	}
}
