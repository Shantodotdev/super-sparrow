package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/shantodotdev/super-sparrow/server/internal/store"
)

// toolObject decodes a successful structured result and checks the text fallback agrees.
func (a *apiTeam) toolObject(name string, args map[string]any) map[string]any {
	a.t.Helper()
	result := a.rpc("tools/call", map[string]any{"name": name, "arguments": args})
	if result["isError"] == true {
		a.t.Fatalf("%s: %+v", name, result)
	}
	output := result["structuredContent"].(map[string]any)
	text := result["content"].([]any)[0].(map[string]any)["text"].(string)
	var fallback map[string]any
	if err := json.Unmarshal([]byte(text), &fallback); err != nil {
		a.t.Fatal(err)
	}
	if !reflect.DeepEqual(output, fallback) {
		a.t.Fatalf("%s structured and text results disagree", name)
	}
	return output
}

func TestMCPDiscoveryAndSubissueEditing(t *testing.T) {
	a := newAPITeam(t)
	a.project()
	projects := a.toolObject("list_projects", map[string]any{})["projects"].([]any)
	if len(projects) != 1 || projects[0].(map[string]any)["slug"] != "wp-agent" {
		t.Fatalf("projects %+v", projects)
	}
	a.mustTool("create_issue", map[string]any{"project_key": "WP", "title": "Second", "status": "backlog"}, nil)
	first := a.toolObject("list_issues", map[string]any{"project_key": "WP", "limit": 1})
	if first["issues"].([]any)[0].(map[string]any)["key"] != "WP-2" || first["next_cursor"] == "" {
		t.Fatalf("first page %+v", first)
	}
	second := a.toolObject("list_issues", map[string]any{"project_key": "WP", "limit": 1, "cursor": first["next_cursor"]})
	if second["issues"].([]any)[0].(map[string]any)["key"] != "WP-1" || second["next_cursor"] != "" {
		t.Fatalf("second page %+v", second)
	}
	filtered := a.toolObject("list_issues", map[string]any{"project_key": "WP", "search": "META", "statuses": []string{"todo"}})
	if len(filtered["issues"].([]any)) != 1 {
		t.Fatalf("filtered %+v", filtered)
	}
	sub := a.toolObject("create_subissue", map[string]any{"issue_key": "WP-1", "title": "Design", "status": "todo", "description": "Draft"})
	if sub["key"] != "WP-1.1" || sub["created_by"].(map[string]any)["kind"] != "agent" {
		t.Fatalf("subissue %+v", sub)
	}
	a.mustTool("update_subissue", map[string]any{"issue_key": "WP-1", "subissue_key": sub["key"], "description": "Approved draft", "description_revision": 1, "status": "done"}, nil)
	result := a.rpc("tools/call", map[string]any{"name": "update_subissue", "arguments": map[string]any{"issue_key": "WP-1", "subissue_key": sub["key"], "description": "Stale", "description_revision": 1}})
	if result["structuredContent"].(map[string]any)["error"].(map[string]any)["code"] != "conflict" {
		t.Fatalf("stale update %+v", result)
	}
	subs := a.toolObject("list_subissues", map[string]any{"issue_key": "WP-1"})["subissues"].([]any)
	if len(subs) != 1 || subs[0].(map[string]any)["description"] != "Approved draft" {
		t.Fatalf("subissues %+v", subs)
	}
	events, err := a.store.ListEvents(context.Background(), store.EventFilter{ActorID: a.agentActor().ID, ProjectEventsOnly: true, Limit: 200})
	if err != nil {
		t.Fatal(err)
	}
	kinds := []string{}
	for _, event := range events {
		kinds = append(kinds, event.Kind)
	}
	if strings.Join(kinds, ",") != "issue.created,issue.created,issue.subissue_created,issue.subissue_updated" {
		t.Fatalf("agent events %v", kinds)
	}
}

func TestMCPPlanRevisionProtectsEdits(t *testing.T) {
	a := newAPITeam(t)
	a.project()
	a.mustTool("update_plan", map[string]any{"issue_key": "WP-1", "plan": "- [ ] New plan", "plan_revision": 1}, nil)
	stale := a.rpc("tools/call", map[string]any{"name": "update_plan", "arguments": map[string]any{"issue_key": "WP-1", "plan": "Lost edit", "plan_revision": 1}})
	if stale["structuredContent"].(map[string]any)["error"].(map[string]any)["code"] != "conflict" {
		t.Fatalf("stale plan %+v", stale)
	}
	plan := a.toolObject("get_plan", map[string]any{"issue_key": "WP-1"})
	if plan["plan"] != "- [ ] New plan" || plan["plan_revision"] != float64(2) {
		t.Fatalf("plan %+v", plan)
	}
	a.mustTool("update_plan", map[string]any{"issue_key": "WP-1", "plan": "", "plan_revision": 2}, nil)
}

func TestMCPRetryReceiptsAreAtomicAndPersistent(t *testing.T) {
	a := newAPITeam(t)
	a.project()
	args := map[string]any{"project_key": "WP", "title": "Retry safe", "status": "todo", "idempotency_key": "create-once"}
	first := a.toolObject("create_issue", args)
	second := a.toolObject("create_issue", args)
	if first["key"] != "WP-2" || second["key"] != first["key"] {
		t.Fatalf("retry %+v %+v", first, second)
	}
	args["title"] = "Different request"
	result := a.rpc("tools/call", map[string]any{"name": "create_issue", "arguments": args})
	if result["structuredContent"].(map[string]any)["error"].(map[string]any)["code"] != "conflict" {
		t.Fatalf("key reused %+v", result)
	}
	// Failed requests consume neither a receipt nor an issue number.
	failed := map[string]any{"project_key": "WP", "title": "", "status": "todo", "idempotency_key": "recoverable"}
	if _, bad := a.tool("create_issue", failed); !bad {
		t.Fatal("accepted empty title")
	}
	failed["title"] = "Corrected"
	created := a.toolObject("create_issue", failed)
	if created["key"] != "WP-3" {
		t.Fatalf("corrected %+v", created)
	}
	events, err := a.store.ListEvents(context.Background(), store.EventFilter{ActorID: a.agentActor().ID, ProjectEventsOnly: true, Limit: 200})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 {
		t.Fatalf("retry produced %d events", len(events))
	}
}

func TestMCPConcurrentRetriesCreateOneComment(t *testing.T) {
	a := newAPITeam(t)
	a.project()
	var group sync.WaitGroup
	for range 8 {
		group.Go(func() {
			a.mustTool("add_comment", map[string]any{"issue_key": "WP-1", "body": "Only once", "idempotency_key": "parallel-comment"}, nil)
		})
	}
	group.Wait()
	comments := a.toolObject("list_comments", map[string]any{"issue_key": "WP-1"})["comments"].([]any)
	if len(comments) != 1 {
		t.Fatalf("concurrent retries created %d comments", len(comments))
	}
}

func TestMCPRetrievesResolvedDuplicatesAndPagedDiscussions(t *testing.T) {
	a := newAPITeam(t)
	a.project()
	a.mustTool("start_review_round", map[string]any{"issue_key": "WP-1", "plan_coverage": "Everything"}, nil)
	for _, title := range []string{"Original", "Duplicate"} {
		a.mustTool("add_finding", map[string]any{"issue_key": "WP-1", "severity": "nit", "title": title, "problem": "P", "fix": "F"}, nil)
	}
	a.mustTool("mark_duplicate", map[string]any{"issue_key": "WP-1", "round": 1, "finding": "R2", "duplicate_of_round": 1, "duplicate_of_finding": "R1"}, nil)
	a.call(http.MethodPost, "/api/issues/WP-1/reviews/1/findings/R1/status", a.member, map[string]any{"status": "wontfix", "reason": "Intentional"}, http.StatusOK, nil)
	for range 52 {
		a.mustTool("add_comment", map[string]any{"issue_key": "WP-1", "round": 1, "finding": "R1", "body": "Discussion"}, nil)
	}
	finding := a.toolObject("get_finding", map[string]any{"issue_key": "WP-1", "round": 1, "finding": "R1"})
	if finding["status"] != "wontfix" || len(finding["comments"].([]any)) != 50 || finding["comments_next_cursor"] == "" {
		t.Fatalf("finding %+v", finding)
	}
	page := a.toolObject("list_comments", map[string]any{"issue_key": "WP-1", "round": 1, "finding": "R1", "cursor": finding["comments_next_cursor"]})
	if len(page["comments"].([]any)) != 2 || page["next_cursor"] != "" {
		t.Fatalf("discussion %+v", page)
	}
	round := a.toolObject("get_review_round", map[string]any{"issue_key": "WP-1", "round": 1, "limit": 1, "include_comments": false})
	if round["plan_coverage"] != "Everything" || len(round["findings"].([]any)) != 1 || round["next_cursor"] == "" {
		t.Fatalf("round %+v", round)
	}
	next := a.toolObject("get_review_round", map[string]any{"issue_key": "WP-1", "round": 1, "limit": 1, "cursor": round["next_cursor"], "include_comments": false})
	if next["findings"].([]any)[0].(map[string]any)["duplicate_of"] != "round 1 R1" {
		t.Fatalf("duplicate %+v", next)
	}
	a.mustTool("finish_review_round", map[string]any{"issue_key": "WP-1", "summary": "Done"}, nil)
	rounds := a.toolObject("list_review_rounds", map[string]any{"issue_key": "WP-1"})["review_rounds"].([]any)
	if len(rounds) != 1 || rounds[0].(map[string]any)["summary"] != "Done" {
		t.Fatalf("rounds %+v", rounds)
	}
}

func TestMCPStructuredValidationErrors(t *testing.T) {
	a := newAPITeam(t)
	a.project()
	for _, attempt := range []struct {
		name        string
		args        map[string]any
		code, field string
	}{
		{"get_issue", map[string]any{"issue_key": nil}, "invalid_input", "issue_key"},
		{"get_issue", map[string]any{"issue_key": "WP-1", "actor_id": "fake"}, "invalid_input", "actor_id"},
		{"get_finding", map[string]any{"issue_key": "WP-1", "round": 0, "finding": "R1"}, "invalid_input", "round"},
		{"list_issues", map[string]any{"project_key": "WP", "limit": 101}, "invalid_input", "limit"},
		{"list_comments", map[string]any{"issue_key": "WP-1", "cursor": "nonsense"}, "invalid_input", "cursor"},
		{"create_issue", map[string]any{"project_key": "WP", "title": "T", "status": "todo", "idempotency_key": ""}, "invalid_input", "idempotency_key"},
		{"get_issue", map[string]any{"issue_key": "WP-999"}, "not_found", ""},
	} {
		result := a.rpc("tools/call", map[string]any{"name": attempt.name, "arguments": attempt.args})
		failure := result["structuredContent"].(map[string]any)["error"].(map[string]any)
		if result["isError"] != true || failure["code"] != attempt.code || failure["field"] != attempt.field {
			t.Errorf("%s: %+v", attempt.name, result)
		}
	}
}

func TestMCPRejectsMalformedProtocolMessages(t *testing.T) {
	a := newAPITeam(t)
	for _, attempt := range []struct {
		body any
		code int
	}{
		{nil, -32600}, {true, -32600}, {map[string]any{}, -32600},
		{map[string]any{"jsonrpc": "1.0", "id": 1, "method": "ping"}, -32600},
		{map[string]any{"jsonrpc": "2.0", "id": true, "method": "ping"}, -32600},
		{map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": true}, -32602},
	} {
		response := a.request(http.MethodPost, "/mcp", a.agent, attempt.body)
		var body struct {
			Error struct {
				Code int `json:"code"`
			} `json:"error"`
		}
		decode(t, response, &body)
		// A missing HTTP body is a parse error; JSON null is an invalid request.
		want := attempt.code
		if attempt.body == nil {
			want = -32700
		}
		if body.Error.Code != want {
			t.Errorf("%v returned %d, want %d", attempt.body, body.Error.Code, want)
		}
	}
}
