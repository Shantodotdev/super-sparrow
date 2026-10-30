package httpserver

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/shantodotdev/super-sparrow/server/internal/auth"
	"github.com/shantodotdev/super-sparrow/server/internal/store"
)

// apiTeam is a fixture whose owner and member are signed in, and whose member has an agent token.
type apiTeam struct {
	*fixture
	owner  string
	member string
	agent  string
}

// newAPITeam adds the member, signs both people in, and creates the member's token.
func newAPITeam(t *testing.T) *apiTeam {
	t.Helper()
	f := newFixture(t)

	_, err := f.store.CreateHuman(context.Background(), store.Caller{ActorID: f.owner.ID},
		store.NewHuman{DisplayName: "Max Member", Email: "max@example.com", Role: store.RoleMember})
	if err != nil {
		t.Fatalf("CreateHuman: %v", err)
	}

	team := &apiTeam{fixture: f, owner: f.signIn("olive@example.com"), member: f.signIn("max@example.com")}
	team.agent, _ = f.createToken(team.member)
	return team
}

// call sends a request and checks its status, then decodes the response into dst when given.
func (a *apiTeam) call(method, path, credential string, body any, status int, dst any) {
	a.t.Helper()
	response := a.request(method, path, credential, body)
	wantStatus(a.t, response, status)
	if dst != nil {
		decode(a.t, response, dst)
	}
}

// project creates project WP with one issue, WP-1, whose plan has two checklist items.
func (a *apiTeam) project() issueView {
	a.t.Helper()
	a.call(http.MethodPost, "/api/projects", a.member, map[string]string{"key": "wp", "name": "WP Agent"}, http.StatusCreated, nil)

	var created struct {
		Issue issueView `json:"issue"`
	}
	a.call(http.MethodPost, "/api/projects/WP/issues", a.agent, map[string]any{
		"title": "Meta comments", "status": "todo", "plan": "- [ ] Migration\n- [ ] Store", "branch_name": "feat/wp-1-meta",
	}, http.StatusCreated, &created)
	return created.Issue
}

func TestProjectsMilestonesAndLabels(t *testing.T) {
	a := newAPITeam(t)
	a.project()

	var projects struct {
		Projects []projectView `json:"projects"`
	}
	a.call(http.MethodGet, "/api/projects", a.agent, nil, http.StatusOK, &projects)
	if len(projects.Projects) != 1 || projects.Projects[0].Key != "WP" {
		t.Fatalf("projects %+v", projects.Projects)
	}

	a.call(http.MethodPatch, "/api/projects/wp", a.member, map[string]string{"name": "WP Agent 2"}, http.StatusOK, nil)
	a.call(http.MethodPost, "/api/projects/WP/milestones", a.member, map[string]string{"name": "Version 01", "status": "active"}, http.StatusCreated, nil)
	a.call(http.MethodPost, "/api/projects/WP/labels", a.agent, map[string]string{"name": "backend"}, http.StatusCreated, nil)

	var milestones struct {
		Milestones []milestoneView `json:"milestones"`
	}
	a.call(http.MethodGet, "/api/projects/WP/milestones", a.agent, nil, http.StatusOK, &milestones)
	a.call(http.MethodPatch, "/api/milestones/"+milestones.Milestones[0].ID, a.member, map[string]string{"status": "done"}, http.StatusOK, nil)
	wantField(t, a.request(http.MethodPatch, "/api/milestones/"+milestones.Milestones[0].ID, a.member, map[string]any{"position": nil}), "position")

	var labels struct {
		Labels []labelView `json:"labels"`
	}
	a.call(http.MethodGet, "/api/projects/WP/labels", a.agent, nil, http.StatusOK, &labels)
	if len(labels.Labels) != 1 {
		t.Fatalf("labels %+v", labels.Labels)
	}

	a.call(http.MethodGet, "/api/projects/NOPE", a.member, nil, http.StatusNotFound, nil)
	wantField(t, a.request(http.MethodPost, "/api/projects", a.member, map[string]string{"key": "1x", "name": "Bad"}), "key")
	wantField(t, a.request(http.MethodPost, "/api/projects/WP/milestones", a.member, map[string]string{"name": "M", "status": "late"}), "status")
}

func TestWrongMethodsAndBearerSpelling(t *testing.T) {
	a := newAPITeam(t)
	a.project()

	for path, allow := range map[string]string{"/api/health": "GET", "/api/issues/WP-1": "GET, PATCH"} {
		response := a.request(http.MethodPut, path, a.agent, nil)
		if response.Code != http.StatusMethodNotAllowed || response.Header().Get("Allow") != allow {
			t.Errorf("PUT %s: status %d, Allow %q, want 405 and %q", path, response.Code, response.Header().Get("Allow"), allow)
		}
	}
	wantStatus(t, a.request(http.MethodPut, "/api/nothing-here", a.agent, nil), http.StatusNotFound)

	request := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	request.Header.Set("Authorization", "bearer "+a.agent)
	response := httptest.NewRecorder()
	a.handler.ServeHTTP(response, request)
	wantStatus(t, response, http.StatusOK)
}

func TestAgentsAreRefusedPersonAndOwnerActions(t *testing.T) {
	a := newAPITeam(t)
	a.project()

	for _, attempt := range []struct {
		method, path string
		body         any
	}{
		{http.MethodPost, "/api/projects", map[string]string{"key": "AB", "name": "Agent project"}},
		{http.MethodPatch, "/api/projects/WP", map[string]string{"name": "Renamed"}},
		{http.MethodDelete, "/api/projects/WP", nil},
		{http.MethodPost, "/api/projects/WP/milestones", map[string]string{"name": "M", "status": "planned"}},
		{http.MethodGet, "/api/backups", nil},
	} {
		if response := a.request(attempt.method, attempt.path, a.agent, attempt.body); response.Code != http.StatusForbidden {
			t.Errorf("agent %s %s: status %d", attempt.method, attempt.path, response.Code)
		}
	}

	// Members are refused owner-only actions too; the owner can delete.
	a.call(http.MethodDelete, "/api/projects/WP", a.member, nil, http.StatusForbidden, nil)
	a.call(http.MethodDelete, "/api/projects/WP", a.owner, nil, http.StatusNoContent, nil)
}

func TestIssuesListUpdateAndBranchLookup(t *testing.T) {
	a := newAPITeam(t)
	issue := a.project()
	if issue.Key != "WP-1" || issue.Plan == nil || len(issue.Checklist) != 2 || issue.ChecklistProgress.Total != 2 {
		t.Fatalf("created issue %+v", issue)
	}

	var found struct {
		Issue issueView `json:"issue"`
	}
	a.call(http.MethodGet, "/api/issues/for-branch?branch=feat/wp-1-meta", a.agent, nil, http.StatusOK, &found)
	if found.Issue.ID != issue.ID {
		t.Fatalf("branch lookup found %s", found.Issue.Key)
	}
	a.call(http.MethodGet, "/api/issues/for-branch?branch=chore/nothing", a.agent, nil, http.StatusNotFound, nil)

	var updated struct {
		Issue issueView `json:"issue"`
	}
	a.call(http.MethodPatch, "/api/issues/wp-1", a.agent, map[string]any{"status": "in_progress", "branch_name": ""}, http.StatusOK, &updated)
	if updated.Issue.Status != "in_progress" || updated.Issue.BranchName != "" || updated.Issue.Title != "Meta comments" {
		t.Fatalf("updated issue %+v", updated.Issue)
	}
	wantField(t, a.request(http.MethodPatch, "/api/issues/WP-1", a.agent, map[string]any{"status": "later"}), "status")
	wantField(t, a.request(http.MethodPatch, "/api/issues/WP-1", a.agent, map[string]any{"titel": "typo"}), "titel")
	wantField(t, a.request(http.MethodGet, "/api/issues/not-a-key", a.agent, nil), "key")
	wantField(t, a.request(http.MethodGet, "/api/issues/WP-01", a.agent, nil), "key")
	wantField(t, a.request(http.MethodPatch, "/api/issues/WP-1", a.agent, map[string]any{"title": nil}), "title")
	wantField(t, a.request(http.MethodPatch, "/api/issues/WP-1", a.agent, map[string]any{"title": 5}), "title")

	a.call(http.MethodPatch, "/api/issues/WP-1", a.agent, map[string]any{"branch_name": "feat/wp-1-meta"}, http.StatusOK, nil)
	a.call(http.MethodPatch, "/api/issues/WP-1", a.agent, map[string]any{"branch_name": nil}, http.StatusOK, &updated)
	if updated.Issue.BranchName != "" {
		t.Fatalf("null left branch_name %q", updated.Issue.BranchName)
	}

	var checked struct {
		Issue issueView `json:"issue"`
	}
	a.call(http.MethodPatch, "/api/issues/WP-1/checklist", a.agent, map[string]any{"items": []map[string]any{{"index": 1, "checked": true, "text": "Store"}}}, http.StatusOK, &checked)
	if !checked.Issue.Checklist[1].Checked || checked.Issue.ChecklistProgress.Done != 1 {
		t.Fatalf("checklist %+v", checked.Issue.Checklist)
	}
	wantField(t, a.request(http.MethodPatch, "/api/issues/WP-1/checklist", a.agent, map[string]any{"items": []map[string]any{{"index": 9, "checked": true, "text": "Store"}}}), "items")
	wantField(t, a.request(http.MethodPatch, "/api/issues/WP-1/checklist", a.agent, map[string]any{"items": []map[string]any{{"index": 1, "checked": true}}}), "items")
	wantStatus(t, a.request(http.MethodPatch, "/api/issues/WP-1/checklist", a.agent, map[string]any{"items": []map[string]any{{"index": 0, "checked": true, "text": "Store"}}}), http.StatusConflict)

	// The tick moved the plan to revision 2, so an edit of revision 1 is refused with 409.
	if checked.Issue.PlanRevision != 2 {
		t.Fatalf("plan revision after a tick = %d", checked.Issue.PlanRevision)
	}
	wantStatus(t, a.request(http.MethodPatch, "/api/issues/WP-1", a.member, map[string]any{"plan": "- [ ] Migration", "plan_revision": 1}), http.StatusConflict)
	wantField(t, a.request(http.MethodPatch, "/api/issues/WP-1", a.member, map[string]any{"plan": "- [ ] Migration"}), "plan_revision")
	a.call(http.MethodPatch, "/api/issues/WP-1", a.member, map[string]any{"plan": "- [ ] Migration\n- [x] Store\n- [ ] API", "plan_revision": 2}, http.StatusOK, &updated)
	if updated.Issue.PlanRevision != 3 || len(updated.Issue.Checklist) != 3 {
		t.Fatalf("saved plan %+v", updated.Issue)
	}

	for range 2 {
		a.call(http.MethodPost, "/api/projects/WP/issues", a.member, map[string]any{"title": "More", "status": "backlog"}, http.StatusCreated, nil)
	}
	var page struct {
		Issues     []issueView `json:"issues"`
		NextBefore *int        `json:"next_before"`
	}
	a.call(http.MethodGet, "/api/projects/WP/issues?limit=2", a.member, nil, http.StatusOK, &page)
	if len(page.Issues) != 2 || page.Issues[0].Plan != nil || page.NextBefore == nil || *page.NextBefore != 2 {
		t.Fatalf("first page %+v, next %v", page.Issues, page.NextBefore)
	}
	a.call(http.MethodGet, "/api/projects/WP/issues?limit=3", a.member, nil, http.StatusOK, &page)
	if len(page.Issues) != 3 || page.NextBefore != nil {
		t.Fatalf("a full last page %+v, next %v", page.Issues, page.NextBefore)
	}
	wantField(t, a.request(http.MethodGet, "/api/projects/WP/issues?limit=0", a.member, nil), "limit")
	wantField(t, a.request(http.MethodGet, "/api/projects/WP/issues?status=bogus", a.member, nil), "status")
	wantField(t, a.request(http.MethodGet, "/api/events?target_type=bogus", a.member, nil), "target_type")
	wantStatus(t, a.request(http.MethodGet, "/api/events?actor=nobody", a.member, nil), http.StatusNotFound)
	a.call(http.MethodGet, "/api/projects/WP/issues?limit=2&before=2", a.member, nil, http.StatusOK, &page)
	if len(page.Issues) != 1 || page.NextBefore != nil {
		t.Fatalf("last page %+v, next %v", page.Issues, page.NextBefore)
	}
	a.call(http.MethodGet, "/api/projects/WP/issues?status=backlog", a.member, nil, http.StatusOK, &page)
	if len(page.Issues) != 2 {
		t.Fatalf("backlog has %d issues", len(page.Issues))
	}
}

func TestReviewLoopThroughTheAPI(t *testing.T) {
	a := newAPITeam(t)
	a.project()

	var started struct {
		Round roundView `json:"round"`
	}
	a.call(http.MethodPost, "/api/issues/WP-1/reviews", a.agent, map[string]any{"to_commit": "a1b2c3d", "plan_coverage": "All of it"}, http.StatusCreated, &started)
	if started.Round.Number != 1 || started.Round.AuthorActorID == "" {
		t.Fatalf("round %+v", started.Round)
	}
	wantField(t, a.request(http.MethodPost, "/api/issues/WP-1/reviews/1/findings", a.agent, map[string]any{"severity": "huge", "title": "T", "problem": "P", "fix": "F"}), "severity")

	for _, severity := range []string{"nit", "blocking"} {
		a.call(http.MethodPost, "/api/issues/WP-1/reviews/1/findings", a.agent, map[string]any{
			"severity": severity, "title": "Problem " + severity, "problem": "P", "fix": "F", "file_path": "a.go", "line": 3,
		}, http.StatusCreated, nil)
	}
	a.call(http.MethodPost, "/api/issues/WP-1/reviews/1/finish", a.agent, map[string]string{"summary": "Two findings."}, http.StatusOK, nil)
	a.call(http.MethodPost, "/api/issues/WP-1/reviews/1/findings", a.agent, map[string]any{"severity": "nit", "title": "T", "problem": "P", "fix": "F"}, http.StatusConflict, nil)

	var round struct {
		Round    roundView     `json:"round"`
		Findings []findingView `json:"findings"`
	}
	a.call(http.MethodGet, "/api/issues/WP-1/reviews/1", a.member, nil, http.StatusOK, &round)
	if len(round.Findings) != 2 || round.Findings[0].Label != "R2" || round.Findings[0].Severity != "blocking" {
		t.Fatalf("findings %+v", round.Findings)
	}

	// Only a person decides; the agent then fixes and reports.
	a.call(http.MethodPost, "/api/issues/WP-1/reviews/1/findings/R2/status", a.agent, map[string]string{"status": "fix"}, http.StatusForbidden, nil)
	a.call(http.MethodPost, "/api/issues/WP-1/reviews/1/findings/r2/status", a.member, map[string]string{"status": "fix"}, http.StatusOK, nil)
	wantField(t, a.request(http.MethodPost, "/api/issues/WP-1/reviews/1/findings/R1/status", a.member, map[string]string{"status": "wontfix"}), "reason")

	var later struct {
		Finding       findingView `json:"finding"`
		FollowUpIssue *issueView  `json:"follow_up_issue"`
	}
	a.call(http.MethodPost, "/api/issues/WP-1/reviews/1/findings/1/status", a.member, map[string]string{"status": "later"}, http.StatusOK, &later)
	if later.FollowUpIssue == nil || later.FollowUpIssue.Status != "backlog" || later.Finding.FollowUpIssueID != later.FollowUpIssue.ID {
		t.Fatalf("later decision %+v, follow-up %+v", later.Finding, later.FollowUpIssue)
	}

	var fix struct {
		Comment  commentView   `json:"comment"`
		Findings []findingView `json:"findings"`
	}
	a.call(http.MethodPost, "/api/issues/WP-1/fix-summary", a.agent, map[string]any{
		"summary": "Fixed the race.", "commit": "e4f5a6b", "findings": []map[string]any{{"round": 1, "finding": "R2"}},
	}, http.StatusCreated, &fix)
	if len(fix.Findings) != 1 || fix.Findings[0].Status != "fixed" || fix.Comment.Body != "Fixed the race." {
		t.Fatalf("fix summary %+v", fix)
	}
	wantField(t, a.request(http.MethodPost, "/api/issues/WP-1/fix-summary", a.agent, map[string]any{
		"summary": "S", "commit": "e4f5a6b", "findings": []map[string]any{{"round": 1, "finding": "R9"}},
	}), "findings")

	var open struct {
		Findings []findingView `json:"findings"`
	}
	a.call(http.MethodGet, "/api/issues/WP-1/findings?status=open,fix,fixed", a.agent, nil, http.StatusOK, &open)
	if len(open.Findings) != 1 || open.Findings[0].Label != "R2" {
		t.Fatalf("open findings %+v", open.Findings)
	}

	// The project's list has the same findings, with the issues they belong to and their counts.
	a.call(http.MethodPost, "/api/issues/WP-1/reviews/1/findings/R2/comments", a.member, map[string]string{"body": "Thanks."}, http.StatusCreated, nil)
	var projectFindings struct {
		Findings []findingView `json:"findings"`
		Issues   []issueView   `json:"issues"`
	}
	a.call(http.MethodGet, "/api/projects/WP/findings?status=open,fix,fixed", a.member, nil, http.StatusOK, &projectFindings)
	if len(projectFindings.Findings) != 1 || projectFindings.Findings[0].IssueKey != "WP-1" || projectFindings.Findings[0].CommentCount != 1 {
		t.Fatalf("project findings %+v", projectFindings.Findings)
	}
	if len(projectFindings.Issues) != 1 || projectFindings.Issues[0].Key != "WP-1" ||
		projectFindings.Issues[0].UnresolvedFindings != 1 || projectFindings.Issues[0].CommentCount != 1 {
		t.Fatalf("project findings' issues %+v", projectFindings.Issues)
	}
	a.call(http.MethodGet, "/api/projects/WP/findings", a.member, nil, http.StatusOK, &projectFindings)
	if len(projectFindings.Findings) != 2 {
		t.Fatalf("all project findings %+v", projectFindings.Findings)
	}
	wantField(t, a.request(http.MethodGet, "/api/projects/WP/findings?status=bogus", a.member, nil), "status")
	a.call(http.MethodGet, "/api/projects/XX/findings", a.member, nil, http.StatusNotFound, nil)

	var list struct {
		Issues []issueView `json:"issues"`
	}
	a.call(http.MethodGet, "/api/projects/WP/issues", a.member, nil, http.StatusOK, &list)
	if first := list.Issues[len(list.Issues)-1]; first.Key != "WP-1" || first.UnresolvedFindings != 1 || first.CommentCount != 1 {
		t.Fatalf("issue list counts %+v", first)
	}

	// A second round can mark its finding as a duplicate of one from the first.
	a.call(http.MethodPost, "/api/issues/WP-1/reviews", a.agent, map[string]any{"to_commit": "e4f5a6b"}, http.StatusCreated, nil)
	a.call(http.MethodPost, "/api/issues/WP-1/reviews/2/findings", a.agent, map[string]any{"severity": "nit", "title": "Again", "problem": "P", "fix": "F"}, http.StatusCreated, nil)
	var duplicate struct {
		Finding findingView `json:"finding"`
	}
	a.call(http.MethodPut, "/api/issues/WP-1/reviews/2/findings/R1/duplicate", a.agent, map[string]string{"duplicate_of_finding_id": round.Findings[0].ID}, http.StatusOK, &duplicate)
	if duplicate.Finding.DuplicateOfFindingID != round.Findings[0].ID {
		t.Fatalf("duplicate %+v", duplicate.Finding)
	}
	wantField(t, a.request(http.MethodPut, "/api/issues/WP-1/reviews/2/findings/R1/duplicate", a.agent, map[string]string{}), "duplicate_of_finding_id")
	a.call(http.MethodGet, "/api/issues/WP-1/reviews/2/findings/R1", a.agent, nil, http.StatusOK, nil)
	a.call(http.MethodGet, "/api/issues/WP-1/reviews/9", a.agent, nil, http.StatusNotFound, nil)
}

func TestCommentsAndHistory(t *testing.T) {
	a := newAPITeam(t)
	a.project()
	a.call(http.MethodPost, "/api/issues/WP-1/reviews", a.agent, map[string]any{"to_commit": "a1b2c3d"}, http.StatusCreated, nil)
	a.call(http.MethodPost, "/api/issues/WP-1/reviews/1/findings", a.agent, map[string]any{"severity": "nit", "title": "T", "problem": "P", "fix": "F"}, http.StatusCreated, nil)

	var comment struct {
		Comment commentView `json:"comment"`
	}
	a.call(http.MethodPost, "/api/issues/WP-1/comments", a.member, map[string]string{"body": "Looks good."}, http.StatusCreated, &comment)
	a.call(http.MethodPost, "/api/issues/WP-1/comments", a.agent, map[string]string{"body": "Thanks.", "parent_comment_id": comment.Comment.ID}, http.StatusCreated, nil)
	a.call(http.MethodPost, "/api/issues/WP-1/reviews/1/findings/R1/comments", a.agent, map[string]string{"body": "On the finding."}, http.StatusCreated, nil)
	a.call(http.MethodPatch, "/api/comments/"+comment.Comment.ID, a.agent, map[string]string{"body": "Not mine."}, http.StatusForbidden, nil)
	a.call(http.MethodPatch, "/api/comments/"+comment.Comment.ID, a.member, map[string]string{"body": "Looks great."}, http.StatusOK, nil)
	wantField(t, a.request(http.MethodPost, "/api/issues/WP-1/comments", a.member, map[string]string{"body": " "}), "body")

	var comments struct {
		Comments []commentView `json:"comments"`
	}
	a.call(http.MethodGet, "/api/issues/WP-1/comments", a.agent, nil, http.StatusOK, &comments)
	if len(comments.Comments) != 2 || comments.Comments[0].Body != "Looks great." || comments.Comments[0].EditedAt == nil {
		t.Fatalf("issue comments %+v", comments.Comments)
	}
	a.call(http.MethodGet, "/api/issues/WP-1/reviews/1/findings/R1/comments", a.agent, nil, http.StatusOK, &comments)
	if len(comments.Comments) != 1 {
		t.Fatalf("finding comments %+v", comments.Comments)
	}

	var history struct {
		Events    []eventView `json:"events"`
		NextAfter int64       `json:"next_after"`
	}
	a.call(http.MethodGet, "/api/issues/WP-1/events", a.member, nil, http.StatusOK, &history)
	var kinds []string
	for _, event := range history.Events {
		kinds = append(kinds, event.Kind)
	}
	want := "issue.created,review_round.started,finding.added,comment.added,comment.added,comment.added,comment.edited"
	if strings.Join(kinds, ",") != want {
		t.Fatalf("history %v", kinds)
	}
	if history.Events[1].ReportedModel != "" || history.NextAfter != history.Events[len(history.Events)-1].Sequence {
		t.Errorf("history page %+v", history)
	}

	// People see team events; agents only see project events.
	var all struct {
		Events []eventView `json:"events"`
	}
	a.call(http.MethodGet, "/api/events?limit=200", a.agent, nil, http.StatusOK, &all)
	for _, event := range all.Events {
		if event.TargetType == "actor" {
			t.Fatalf("agent saw %s", event.Kind)
		}
	}
	a.call(http.MethodGet, "/api/events?target_type=actor&limit=200", a.owner, nil, http.StatusOK, &all)
	if len(all.Events) == 0 {
		t.Error("the owner sees no team events")
	}
	a.call(http.MethodGet, "/api/events?project=WP&limit=2", a.agent, nil, http.StatusOK, &all)
	if len(all.Events) != 2 || all.Events[0].Kind != "project.created" {
		t.Fatalf("project events %+v", all.Events)
	}
	wantField(t, a.request(http.MethodGet, "/api/events?after=-1", a.agent, nil), "after")
}

func TestReportedModelComesFromTheHeader(t *testing.T) {
	a := newAPITeam(t)
	a.project()

	request := httptest.NewRequest(http.MethodPost, "/api/issues/WP-1/comments", strings.NewReader(`{"body":"From a model."}`))
	request.Header.Set("Authorization", "Bearer "+a.agent)
	request.Header.Set(modelHeader, "claude-opus-5-5")
	response := httptest.NewRecorder()
	a.handler.ServeHTTP(response, request)
	wantStatus(t, response, http.StatusCreated)

	events, err := a.store.ListEvents(context.Background(), store.EventFilter{TargetType: store.TargetComment, Limit: 1})
	if err != nil || len(events) != 1 || events[0].ReportedModel != "claude-opus-5-5" {
		t.Fatalf("comment events %+v (%v)", events, err)
	}
}

func TestBackupStatusForPeople(t *testing.T) {
	a := newAPITeam(t)

	var status struct {
		Enabled bool                       `json:"enabled"`
		Jobs    map[string]json.RawMessage `json:"jobs"`
	}
	a.call(http.MethodGet, "/api/backups", a.member, nil, http.StatusOK, &status)
	if status.Enabled || len(status.Jobs) != 2 {
		t.Fatalf("backup status %+v", status)
	}
}

func TestEventStreamSendsNewEvents(t *testing.T) {
	a := newAPITeam(t)
	a.project()
	server := httptest.NewServer(a.handler)
	t.Cleanup(server.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	request, _ := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/api/events/stream?project=WP", nil)
	request.Header.Set("Authorization", "Bearer "+a.agent)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || response.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("stream status %d, type %q", response.StatusCode, response.Header.Get("Content-Type"))
	}

	// Earlier events are not replayed; a comment made now arrives.
	lines := bufio.NewScanner(response.Body)
	if !lines.Scan() || lines.Text() != "retry: 3000" {
		t.Fatalf("first line %q", lines.Text())
	}
	a.call(http.MethodPost, "/api/issues/WP-1/comments", a.member, map[string]string{"body": "Live."}, http.StatusCreated, nil)

	for lines.Scan() {
		data, ok := strings.CutPrefix(lines.Text(), "data: ")
		if !ok {
			continue
		}
		var event eventView
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			t.Fatalf("decode %q: %v", data, err)
		}
		if event.Kind != "comment.added" {
			t.Fatalf("first streamed event is %s", event.Kind)
		}
		return
	}
	t.Fatalf("stream ended without the event: %v", lines.Err())
}

func TestEventStreamCatchesUpFromLastEventID(t *testing.T) {
	a := newAPITeam(t)
	a.project()
	server := httptest.NewServer(a.handler)
	t.Cleanup(server.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	request, _ := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/api/events/stream?project=WP", nil)
	request.Header.Set("Authorization", "Bearer "+a.agent)
	request.Header.Set("Last-Event-ID", "0")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}
	defer response.Body.Close()

	lines := bufio.NewScanner(response.Body)
	for lines.Scan() {
		if strings.HasPrefix(lines.Text(), "data: ") {
			if !strings.Contains(lines.Text(), `"kind":"project.created"`) {
				t.Fatalf("first replayed event %s", lines.Text())
			}
			return
		}
	}
	t.Fatalf("stream replayed nothing: %v", lines.Err())
}

func TestEventStreamEndsWhenAccessIsRevoked(t *testing.T) {
	cases := map[string]func(a *apiTeam) (credential string, revoke func()){
		"revoked token": func(a *apiTeam) (string, func()) {
			secret, id := a.createToken(a.member)
			return secret, func() { a.call(http.MethodDelete, "/api/tokens/"+id, a.member, nil, http.StatusNoContent, nil) }
		},
		"signed out session": func(a *apiTeam) (string, func()) {
			session := a.signIn("max@example.com")
			return session, func() { a.call(http.MethodDelete, "/api/sessions", a.member, nil, http.StatusOK, nil) }
		},
	}

	for name, setup := range cases {
		t.Run(name, func(t *testing.T) {
			a := newAPITeam(t)
			a.project()
			server := httptest.NewServer(a.handler)
			t.Cleanup(server.Close)
			credential, revoke := setup(a)

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			request, _ := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/api/events/stream?project=WP", nil)
			if strings.HasPrefix(credential, auth.TokenPrefix) {
				request.Header.Set("Authorization", "Bearer "+credential)
			} else {
				request.AddCookie(&http.Cookie{Name: "__Host-sparrow_session", Value: credential})
			}
			response, err := http.DefaultClient.Do(request)
			if err != nil {
				t.Fatalf("open stream: %v", err)
			}
			defer response.Body.Close()

			lines := bufio.NewScanner(response.Body)
			if !lines.Scan() || lines.Text() != "retry: 3000" {
				t.Fatalf("first line %q", lines.Text())
			}

			// The change after the revocation wakes the stream, which must end instead of sending it.
			revoke()
			a.call(http.MethodPost, "/api/issues/WP-1/comments", a.owner, map[string]string{"body": "Not for you."}, http.StatusCreated, nil)
			for lines.Scan() {
				if strings.HasPrefix(lines.Text(), "data: ") {
					t.Fatalf("stream sent %s after access was revoked", lines.Text())
				}
			}
			if ctx.Err() != nil {
				t.Fatal("stream stayed open after access was revoked")
			}
		})
	}
}

func TestLabelColorAPI(t *testing.T) {
	a := newAPITeam(t)
	a.project()
	var created struct {
		Label labelView `json:"label"`
	}
	a.call(http.MethodPost, "/api/projects/WP/labels", a.member, map[string]string{"name": "Urgent", "color": "#F2A9AE"}, http.StatusCreated, &created)
	if created.Label.Color != "#f2a9ae" {
		t.Fatalf("created label %+v", created.Label)
	}
	var listed struct {
		Labels []labelView `json:"labels"`
	}
	a.call(http.MethodGet, "/api/projects/WP/labels", a.member, nil, http.StatusOK, &listed)
	if len(listed.Labels) != 1 || listed.Labels[0].Color != created.Label.Color {
		t.Fatalf("listed labels %+v", listed.Labels)
	}
	wantField(t, a.request(http.MethodPost, "/api/projects/WP/labels", a.member, map[string]string{"name": "Invalid", "color": "not-a-color"}), "color")
	a.call(http.MethodPost, "/api/projects/WP/labels", a.agent, map[string]string{"name": "Legacy"}, http.StatusCreated, &created)
	if created.Label.Color != "#a7c0ff" {
		t.Fatalf("legacy label %+v", created.Label)
	}
}
