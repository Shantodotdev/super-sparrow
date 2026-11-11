package mcp

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/shantodotdev/super-sparrow/server/internal/store"
)

// failure returns the structured error of a tool result that must have failed.
func failure(t *testing.T, result map[string]any) map[string]any {
	t.Helper()
	if result["isError"] != true {
		t.Fatalf("expected a tool error, got %v", result)
	}
	return result["structuredContent"].(map[string]any)["error"].(map[string]any)
}

func (f *attachmentFixture) owner() store.Caller {
	f.t.Helper()
	actor, err := f.store.GetActor(f.ctx, f.agent.ActorID)
	if err != nil {
		f.t.Fatal(err)
	}
	return store.Caller{ActorID: actor.OwnerActorID}
}

func TestUpdateIssueMovesWorkAsTheAgent(t *testing.T) {
	f := newAttachmentFixture(t)

	moved := metadata(t, f.call("update_issue", map[string]any{"issue_key": f.first.Key(), "status": "in_progress", "priority": "high", "pull_request_url": "https://github.com/acme/app/pull/7"}))
	if moved["status"] != "in_progress" || moved["priority"] != "high" || moved["pull_request_url"] != "https://github.com/acme/app/pull/7" {
		t.Fatalf("updated issue %+v", moved)
	}
	cleared := metadata(t, f.call("update_issue", map[string]any{"issue_key": f.first.Key(), "priority": "none"}))
	if _, ok := cleared["priority"]; ok {
		t.Fatalf("priority none left %v", cleared["priority"])
	}

	events, err := f.store.ListEvents(f.ctx, store.EventFilter{ActorID: f.agent.ActorID, ProjectEventsOnly: true, Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	statusChanges := 0
	for _, event := range events {
		if event.Kind == "issue.status_changed" {
			statusChanges++
		}
	}
	if statusChanges != 1 {
		t.Fatalf("agent recorded %d status changes, want 1", statusChanges)
	}

	if got := failure(t, f.call("update_issue", map[string]any{"issue_key": f.first.Key()})); got["field"] != "fields" {
		t.Errorf("empty update answered %v", got)
	}
	if got := failure(t, f.call("update_issue", map[string]any{"issue_key": f.first.Key(), "status": "finished"})); got["field"] != "status" {
		t.Errorf("unknown status answered %v", got)
	}
	if got := failure(t, f.call("update_issue", map[string]any{"issue_key": f.first.Key(), "assignee": "me"})); got["field"] != "assignee" {
		t.Errorf("assignment answered %v", got)
	}
}

func TestUpdateIssueRetryKeyReturnsTheFirstResult(t *testing.T) {
	f := newAttachmentFixture(t)
	args := map[string]any{"issue_key": f.first.Key(), "status": "in_review", "idempotency_key": "move-1"}

	first := metadata(t, f.call("update_issue", args))
	f.call("update_issue", map[string]any{"issue_key": f.first.Key(), "status": "done"})
	second := metadata(t, f.call("update_issue", args))
	if second["status"] != first["status"] || second["status"] != "in_review" {
		t.Fatalf("retry answered %v after %v", second["status"], first["status"])
	}
}

func TestListMyIssuesFindsTheOwnersWork(t *testing.T) {
	f := newAttachmentFixture(t)
	owner := f.owner()
	for _, issue := range []store.Issue{f.first, f.second} {
		if _, err := f.store.UpdateIssue(f.ctx, owner, issue.ID, store.IssueUpdate{AssigneeActorID: store.Change(owner.ActorID)}); err != nil {
			t.Fatal(err)
		}
	}
	other, err := f.store.CreateHuman(f.ctx, store.Caller{ActorID: store.SystemActorID}, store.NewHuman{DisplayName: "Max Member", Email: "max@example.com", Role: store.RoleMember})
	if err != nil {
		t.Fatal(err)
	}
	third, err := f.store.CreateIssue(f.ctx, owner, store.NewIssue{ProjectID: f.first.ProjectID, Title: "Not mine", Status: store.IssueTodo, AssigneeActorID: other.ID})
	if err != nil {
		t.Fatal(err)
	}

	keys := func(args map[string]any) []string {
		result := metadata(t, f.call("list_my_issues", args))
		var found []string
		for _, row := range result["issues"].([]any) {
			found = append(found, row.(map[string]any)["key"].(string))
		}
		return found
	}
	if got := keys(map[string]any{}); len(got) != 2 {
		t.Fatalf("my issues %v", got)
	}
	if got := keys(map[string]any{"project_key": "WP"}); len(got) != 1 || got[0] != f.first.Key() {
		t.Fatalf("my WP issues %v", got)
	}
	for _, key := range keys(map[string]any{}) {
		if key == third.Key() {
			t.Fatalf("listed %s, assigned to someone else", key)
		}
	}

	// Finished work leaves the default list but stays reachable by status.
	f.call("update_issue", map[string]any{"issue_key": f.first.Key(), "status": "done"})
	if got := keys(map[string]any{}); len(got) != 1 {
		t.Fatalf("my open issues %v", got)
	}
	if got := keys(map[string]any{"statuses": []string{"done"}}); len(got) != 1 || got[0] != f.first.Key() {
		t.Fatalf("my done issues %v", got)
	}

	page := metadata(t, f.call("list_my_issues", map[string]any{"statuses": []string{"todo", "done"}, "limit": 1}))
	if page["next_cursor"] == "" || len(page["issues"].([]any)) != 1 {
		t.Fatalf("first page %+v", page)
	}
	if got := failure(t, f.call("list_my_issues", map[string]any{"project_key": "NOPE"})); got["code"] != "not_found" {
		t.Errorf("unknown project answered %v", got)
	}
}

func (f *attachmentFixture) findingCount(issue store.Issue) int {
	f.t.Helper()
	findings, err := f.store.ListIssueFindings(f.ctx, issue.ID, nil, false)
	if err != nil {
		f.t.Fatal(err)
	}
	return len(findings)
}

func review(titles ...string) []map[string]any {
	var findings []map[string]any
	for _, title := range titles {
		findings = append(findings, map[string]any{"severity": "should_fix", "title": title, "problem": "Problem text", "fix": "Fix text"})
	}
	return findings
}

func TestAddFindingsIsAtomic(t *testing.T) {
	f := newAttachmentFixture(t)
	if _, err := f.store.StartReviewRound(f.ctx, f.agent, store.NewReviewRound{IssueID: f.first.ID}); err != nil {
		t.Fatal(err)
	}

	tooLong := strings.Repeat("x", 5000)
	bad := f.call("add_findings", map[string]any{"issue_key": f.first.Key(), "findings": review("One", "Two", tooLong)})
	if got := failure(t, bad); got["code"] != "invalid_input" {
		t.Fatalf("invalid batch answered %v", got)
	}
	if count := f.findingCount(f.first); count != 0 {
		t.Fatalf("a failed batch left %d findings", count)
	}

	added := metadata(t, f.call("add_findings", map[string]any{"issue_key": f.first.Key(), "findings": review("One", "Two"), "idempotency_key": "batch-1"}))
	labels := added["findings"].([]any)
	if len(labels) != 2 || labels[0] != "R1" || labels[1] != "R2" || added["found_by"].(map[string]any)["kind"] != "agent" {
		t.Fatalf("added %+v", added)
	}
	f.call("add_findings", map[string]any{"issue_key": f.first.Key(), "findings": review("One", "Two"), "idempotency_key": "batch-1"})
	if count := f.findingCount(f.first); count != 2 {
		t.Fatalf("a retried batch left %d findings", count)
	}

	if got := failure(t, f.call("add_findings", map[string]any{"issue_key": f.first.Key(), "findings": []any{}})); got["field"] != "findings" {
		t.Errorf("empty batch answered %v", got)
	}
	many := make([]map[string]any, 0, 51)
	for range 51 {
		many = append(many, review("n")[0])
	}
	if got := failure(t, f.call("add_findings", map[string]any{"issue_key": f.first.Key(), "findings": many})); got["field"] != "findings" {
		t.Errorf("oversized batch answered %v", got)
	}
}

func TestSubmitReviewRunsAWholeReviewInOneCall(t *testing.T) {
	f := newAttachmentFixture(t)

	result := metadata(t, f.call("submit_review", map[string]any{
		"issue_key": f.first.Key(), "plan_coverage": "All of it", "summary": "Two problems.", "findings": review("One", "Two"),
		"idempotency_key": "review-1",
	}))
	round := result["round"].(map[string]any)
	if round["round"] != float64(1) || round["in_progress"] == true || round["summary"] != "Two problems." ||
		round["findings_by_severity"].(map[string]any)["should_fix"] != float64(2) || len(result["findings"].([]any)) != 2 {
		t.Fatalf("review %+v", result)
	}
	if again := metadata(t, f.call("submit_review", map[string]any{
		"issue_key": f.first.Key(), "plan_coverage": "All of it", "summary": "Two problems.", "findings": review("One", "Two"),
		"idempotency_key": "review-1",
	})); again["round"].(map[string]any)["round"] != float64(1) {
		t.Fatalf("retry answered %+v", again)
	}

	rounds, err := f.store.ListAgentRounds(f.ctx, f.first.ID, 10, 0)
	if err != nil || len(rounds) != 1 || rounds[0].FinishedAt == nil {
		t.Fatalf("rounds %+v, %v", rounds, err)
	}

	clean := metadata(t, f.call("submit_review", map[string]any{"issue_key": f.first.Key(), "summary": "Nothing found."}))
	if clean["round"].(map[string]any)["round"] != float64(2) || len(clean["findings"].([]any)) != 0 {
		t.Fatalf("clean review %+v", clean)
	}
}

func TestSubmitReviewRecordsNothingOnFailure(t *testing.T) {
	f := newAttachmentFixture(t)

	tooLong := strings.Repeat("x", 5000)
	if got := failure(t, f.call("submit_review", map[string]any{"issue_key": f.first.Key(), "summary": "S", "findings": review("One", tooLong)})); got["code"] != "invalid_input" {
		t.Fatalf("invalid review answered %v", got)
	}
	rounds, err := f.store.ListAgentRounds(f.ctx, f.first.ID, 10, 0)
	if err != nil || len(rounds) != 0 || f.findingCount(f.first) != 0 {
		t.Fatalf("a failed review left rounds %+v, %v", rounds, err)
	}

	// A round already in progress is not replaced.
	if _, err := f.store.StartReviewRound(f.ctx, f.agent, store.NewReviewRound{IssueID: f.first.ID}); err != nil {
		t.Fatal(err)
	}
	if got := failure(t, f.call("submit_review", map[string]any{"issue_key": f.first.Key(), "summary": "S", "findings": review("One")})); got["code"] == "" {
		t.Fatalf("review over an open round answered %v", got)
	}
	if rounds, _ := f.store.ListAgentRounds(f.ctx, f.first.ID, 10, 0); len(rounds) != 1 || f.findingCount(f.first) != 0 {
		t.Fatalf("rounds after refusal %+v", rounds)
	}
	if got := failure(t, f.call("submit_review", map[string]any{"issue_key": f.first.Key(), "summary": " "})); got["field"] != "summary" {
		t.Errorf("blank summary answered %v", got)
	}
}

func TestGetIssueIncludesWhatTheAgentAsksFor(t *testing.T) {
	f := newAttachmentFixture(t)
	plan := "## Goal\n\n- [ ] First step\n- [x] Second step\n"
	if _, err := f.store.UpdateIssue(f.ctx, f.agent, f.first.ID, store.IssueUpdate{Plan: store.Change(plan), PlanRevision: 1}); err != nil {
		t.Fatal(err)
	}
	f.call("submit_review", map[string]any{"issue_key": f.first.Key(), "summary": "One.", "findings": review("Race")})
	f.call("create_subissue", map[string]any{"issue_key": f.first.Key(), "title": "Part", "status": "todo"})

	bare := metadata(t, f.call("get_issue", map[string]any{"issue_key": f.first.Key()}))
	for _, key := range []string{"plan", "plan_revision", "checklist_items", "open_findings", "subissues"} {
		if _, ok := bare[key]; ok {
			t.Errorf("get_issue without include has %s", key)
		}
	}

	full := metadata(t, f.call("get_issue_for_branch", map[string]any{"branch": "feat/" + strings.ToLower(f.first.Key()) + "-work", "include": []string{"plan", "findings", "subissues"}}))
	if full["plan"] != strings.TrimSpace(plan) || full["plan_revision"] != float64(2) || full["checklist"] != "1 of 2 items done" {
		t.Fatalf("plan section %+v", full)
	}
	items := full["checklist_items"].([]any)
	if len(items) != 2 || items[0].(map[string]any)["checked"] != nil || items[1].(map[string]any)["checked"] != true {
		t.Fatalf("checklist items %+v", items)
	}
	findings := full["open_findings"].([]any)
	if len(findings) != 1 || findings[0].(map[string]any)["problem"] != "Problem text" || findings[0].(map[string]any)["comments_included"] != true {
		t.Fatalf("open findings %+v", findings)
	}
	if subs := full["subissues"].([]any); len(subs) != 1 || subs[0].(map[string]any)["title"] != "Part" {
		t.Fatalf("subissues %+v", subs)
	}

	only := metadata(t, f.call("get_issue", map[string]any{"issue_key": f.first.Key(), "include": []string{"plan"}}))
	if _, ok := only["open_findings"]; ok {
		t.Errorf("include plan returned findings")
	}
	if got := failure(t, f.call("get_issue", map[string]any{"issue_key": f.first.Key(), "include": []string{"everything"}})); got["field"] != "include" {
		t.Errorf("unknown section answered %v", got)
	}
}

func TestGetPlanChecklistOnlyOmitsTheMarkdown(t *testing.T) {
	f := newAttachmentFixture(t)
	if _, err := f.store.UpdateIssue(f.ctx, f.agent, f.first.ID, store.IssueUpdate{Plan: store.Change("Body\n\n- [ ] Step\n"), PlanRevision: 1}); err != nil {
		t.Fatal(err)
	}

	plan := metadata(t, f.call("get_plan", map[string]any{"issue_key": f.first.Key(), "checklist_only": true}))
	if _, ok := plan["plan"]; ok || plan["plan_revision"] != float64(2) || len(plan["checklist"].([]any)) != 1 {
		t.Fatalf("checklist only %+v", plan)
	}
	if full := metadata(t, f.call("get_plan", map[string]any{"issue_key": f.first.Key()})); full["plan"] != "Body\n\n- [ ] Step" {
		t.Fatalf("full plan %+v", full)
	}
}

func TestResponsesAreCompact(t *testing.T) {
	f := newAttachmentFixture(t)
	f.call("submit_review", map[string]any{"issue_key": f.first.Key(), "summary": "One.", "findings": review("Race")})

	for name, args := range map[string]map[string]any{
		"list_issues":        {"project_key": "WP"},
		"get_issue":          {"issue_key": f.first.Key()},
		"list_open_findings": {"issue_key": f.first.Key()},
	} {
		text := content(f.call(name, args))[0]["text"].(string)
		if strings.Contains(text, "\n") || strings.Contains(text, ": ") {
			t.Errorf("%s is not compact JSON: %.80s", name, text)
		}
	}

	rows := metadata(t, f.call("list_issues", map[string]any{"project_key": "WP"}))["issues"].([]any)
	if row := rows[0].(map[string]any); len(row) != 3 || row["url"] != nil {
		t.Errorf("list row %+v", row)
	}
	issue := metadata(t, f.call("get_issue", map[string]any{"issue_key": f.first.Key()}))
	if _, ok := issue["labels"]; ok {
		t.Errorf("empty labels were not omitted")
	}
	if rounds := issue["review_rounds"].([]any); rounds[0].(map[string]any)["url"] != nil {
		t.Errorf("round row %+v", rounds[0])
	}

	// A finding written by the agent comes back without the text the agent just sent.
	if _, err := f.store.StartReviewRound(f.ctx, f.agent, store.NewReviewRound{IssueID: f.first.ID}); err != nil {
		t.Fatal(err)
	}
	added := metadata(t, f.call("add_finding", map[string]any{"issue_key": f.first.Key(), "severity": "nit", "title": "T", "problem": "Long problem", "fix": "Long fix"}))
	if added["problem"] != nil || added["fix"] != nil || added["found_by"] == nil || added["url"] == nil {
		t.Errorf("add_finding echoed %+v", added)
	}
}

func TestToolListStaysWithinItsBudget(t *testing.T) {
	f := newAttachmentFixture(t)

	raw, err := json.Marshal(f.server.toolDescriptions())
	if err != nil {
		t.Fatal(err)
	}
	// About 6k tokens at four bytes each. Raise it only for a tool that earns the space.
	if len(raw) > 24000 {
		t.Errorf("tools/list is %d bytes", len(raw))
	}
	for _, description := range f.server.toolDescriptions() {
		if len(description["description"].(string)) > 330 {
			t.Errorf("%s has a %d byte description", description["name"], len(description["description"].(string)))
		}
	}
}

func TestListToolsAdvertiseTheirOwnPageSize(t *testing.T) {
	f := newAttachmentFixture(t)

	for _, description := range f.server.toolDescriptions() {
		properties := description["inputSchema"].(map[string]any)["properties"].(map[string]any)
		limit, paged := properties["limit"].(map[string]any)
		if paged != (pageSizes[description["name"].(string)] > 0) {
			t.Errorf("%s: limit presence %v", description["name"], paged)
		}
		if paged && !strings.Contains(limit["description"].(string), "Default ") {
			t.Errorf("%s: limit %v", description["name"], limit)
		}
	}
	if pageSizes["list_open_findings"] >= pageSizes["list_issues"] {
		t.Errorf("discussion lists default to %d, rows to %d", pageSizes["list_open_findings"], pageSizes["list_issues"])
	}
}
