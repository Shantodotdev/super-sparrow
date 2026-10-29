package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/shantodotdev/super-sparrow/server/internal/store"
)

// rpc posts one JSON-RPC request to /mcp with the agent token and returns the decoded response.
func (a *apiTeam) rpc(method string, params any) map[string]any {
	a.t.Helper()
	response := a.request(http.MethodPost, "/mcp", a.agent, map[string]any{"jsonrpc": "2.0", "id": 1, "method": method, "params": params})
	wantStatus(a.t, response, http.StatusOK)

	var body map[string]any
	decode(a.t, response, &body)
	if body["error"] != nil {
		a.t.Fatalf("%s: %v", method, body["error"])
	}
	return body["result"].(map[string]any)
}

// tool calls an MCP tool and returns its text output and whether it reported an error.
func (a *apiTeam) tool(name string, args map[string]any) (string, bool) {
	a.t.Helper()
	result := a.rpc("tools/call", map[string]any{"name": name, "arguments": args})
	content := result["content"].([]any)[0].(map[string]any)
	return content["text"].(string), result["isError"].(bool)
}

// mustTool calls a tool that must succeed and decodes its output into dst when given.
func (a *apiTeam) mustTool(name string, args map[string]any, dst any) {
	a.t.Helper()
	text, failed := a.tool(name, args)
	if failed {
		a.t.Fatalf("%s failed: %s", name, text)
	}
	if dst != nil {
		if err := json.Unmarshal([]byte(text), dst); err != nil {
			a.t.Fatalf("%s output %q: %v", name, text, err)
		}
	}
}

func TestMCPHandshakeAndToolList(t *testing.T) {
	a := newAPITeam(t)

	initialized := a.rpc("initialize", map[string]any{
		"protocolVersion": "2025-06-18", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "test", "version": "1"},
	})
	if initialized["protocolVersion"] != "2025-06-18" || initialized["instructions"] == "" {
		t.Fatalf("initialize %+v", initialized)
	}
	if fallback := a.rpc("initialize", map[string]any{"protocolVersion": "1999-01-01"}); fallback["protocolVersion"] != "2025-11-25" {
		t.Errorf("unknown version answered with %v", fallback["protocolVersion"])
	}

	notification := a.request(http.MethodPost, "/mcp", a.agent, map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"})
	wantStatus(t, notification, http.StatusAccepted)

	tools := a.rpc("tools/list", map[string]any{})["tools"].([]any)
	var names []string
	for _, entry := range tools {
		tool := entry.(map[string]any)
		if tool["description"] == "" || tool["inputSchema"].(map[string]any)["type"] != "object" {
			t.Errorf("tool %v lacks a description or schema", tool["name"])
		}
		names = append(names, tool["name"].(string))
	}
	want := "get_issue_for_branch,get_issue,get_plan,update_plan_checklist,start_review_round,add_finding," +
		"finish_review_round,list_open_findings,set_finding_status,add_comment,list_comments,post_fix_summary,create_issue"
	if strings.Join(names, ",") != want {
		t.Fatalf("tools %v", names)
	}
}

func TestMCPRefusesSessionsAndOtherMethods(t *testing.T) {
	a := newAPITeam(t)

	wantStatus(t, a.request(http.MethodPost, "/mcp", a.member, map[string]any{"jsonrpc": "2.0", "id": 1, "method": "ping"}), http.StatusForbidden)
	wantStatus(t, a.request(http.MethodPost, "/mcp", "", map[string]any{"jsonrpc": "2.0", "id": 1, "method": "ping"}), http.StatusUnauthorized)
	wantStatus(t, a.request(http.MethodGet, "/mcp", a.agent, nil), http.StatusMethodNotAllowed)

	response := a.request(http.MethodPost, "/mcp", a.agent, map[string]any{"jsonrpc": "2.0", "id": 2, "method": "resources/list"})
	var body struct {
		Error struct {
			Code int `json:"code"`
		} `json:"error"`
	}
	decode(t, response, &body)
	if body.Error.Code != -32601 {
		t.Fatalf("unknown method answered %+v, want JSON-RPC's method not found", body)
	}

	request := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":3,"method":"ping"}`))
	request.Header.Set("Authorization", "Bearer "+a.agent)
	request.Header.Set("MCP-Protocol-Version", "1999-01-01")
	recorder := httptest.NewRecorder()
	a.handler.ServeHTTP(recorder, request)
	wantStatus(t, recorder, http.StatusBadRequest)
}

func TestMCPToolsRunTheReviewLoopAsTheAgent(t *testing.T) {
	a := newAPITeam(t)
	a.project()
	a.call(http.MethodPost, "/api/projects/WP/labels", a.member, map[string]string{"name": "backend"}, http.StatusCreated, nil)

	// Find the issue and work through its plan.
	var issue issueOutputForTest
	a.mustTool("get_issue_for_branch", map[string]any{"branch": "feat/wp-1-meta"}, &issue)
	if issue.Key != "WP-1" || issue.URL != origin+"/WP-1" || issue.Checklist != "0 of 2 items done" {
		t.Fatalf("issue for branch %+v", issue)
	}
	a.mustTool("get_issue", map[string]any{"issue_key": "wp-1"}, &issue)

	var plan struct {
		Plan      string `json:"plan"`
		Checklist []struct {
			Index int `json:"index"`
		} `json:"checklist"`
	}
	a.mustTool("get_plan", map[string]any{"issue_key": "WP-1"}, &plan)
	if len(plan.Checklist) != 2 {
		t.Fatalf("plan %+v", plan)
	}
	a.mustTool("update_plan_checklist", map[string]any{"issue_key": "WP-1", "items": []map[string]any{{"index": 0, "checked": true}}}, nil)

	// Review it.
	var round struct {
		Round int    `json:"round"`
		URL   string `json:"url"`
	}
	a.mustTool("start_review_round", map[string]any{"issue_key": "WP-1", "to_commit": "a1b2c3d"}, &round)
	if round.Round != 1 || round.URL != origin+"/WP-1/reviews/1" {
		t.Fatalf("round %+v", round)
	}
	var finding struct {
		Finding string `json:"finding"`
		FoundBy string `json:"found_by"`
	}
	a.mustTool("add_finding", map[string]any{
		"issue_key": "WP-1", "severity": "blocking", "title": "Race", "problem": "P", "fix": "F", "file_path": "a.go", "line": 4,
	}, &finding)
	if finding.Finding != "R1" || finding.FoundBy != "Claude Code (Max Member)" {
		t.Fatalf("finding %+v", finding)
	}
	a.mustTool("add_comment", map[string]any{"issue_key": "WP-1", "round": 1, "finding": "R1", "body": "Seen in CI."}, nil)
	a.mustTool("finish_review_round", map[string]any{"issue_key": "WP-1", "summary": "One blocking finding."}, nil)

	// The agent cannot decide; once the person decides fix, it fixes and reports.
	if text, failed := a.tool("set_finding_status", map[string]any{"issue_key": "WP-1", "round": 1, "finding": "R1", "status": "fix"}); !failed || !strings.Contains(text, "only a person") {
		t.Fatalf("agent decision answered %q", text)
	}
	a.call(http.MethodPost, "/api/issues/WP-1/reviews/1/findings/R1/status", a.member, map[string]string{"status": "fix"}, http.StatusOK, nil)
	a.call(http.MethodPost, "/api/issues/WP-1/reviews/1/findings/R1/comments", a.member, map[string]string{"body": "Fix it in the job, not the handler."}, http.StatusCreated, nil)

	// The agent reads how the person wants it fixed, with the finding and on its own.
	var open struct {
		Findings []struct {
			Status   string                 `json:"status"`
			Comments []commentOutputForTest `json:"comments"`
		} `json:"findings"`
	}
	a.mustTool("list_open_findings", map[string]any{"issue_key": "WP-1", "statuses": []string{"fix"}}, &open)
	if len(open.Findings) != 1 || len(open.Findings[0].Comments) != 2 || open.Findings[0].Comments[1].Body != "Fix it in the job, not the handler." {
		t.Fatalf("findings to fix %+v", open)
	}
	var thread struct {
		About    string                 `json:"about"`
		Comments []commentOutputForTest `json:"comments"`
	}
	a.mustTool("list_comments", map[string]any{"issue_key": "WP-1", "round": 1, "finding": "R1"}, &thread)
	if thread.About != "WP-1 round 1 R1" || len(thread.Comments) != 2 {
		t.Fatalf("finding thread %+v", thread)
	}
	personComment := thread.Comments[1]
	a.mustTool("add_comment", map[string]any{"issue_key": "WP-1", "round": 1, "finding": "R1", "parent_comment_id": personComment.ID, "body": "Will do."}, nil)
	a.mustTool("list_comments", map[string]any{"issue_key": "WP-1", "round": 1, "finding": "R1"}, &thread)
	if reply := thread.Comments[2]; reply.ReplyTo != personComment.ID || reply.Author != "Claude Code (Max Member)" {
		t.Fatalf("reply %+v", reply)
	}
	a.mustTool("list_comments", map[string]any{"issue_key": "WP-1"}, &thread)
	if thread.About != "WP-1" || len(thread.Comments) != 0 {
		t.Fatalf("issue thread %+v", thread)
	}
	a.mustTool("post_fix_summary", map[string]any{
		"issue_key": "WP-1", "summary": "Sent both from one job.", "commit": "e4f5a6b",
		"findings": []map[string]any{{"round": 1, "finding": "R1"}},
	}, nil)
	a.mustTool("set_finding_status", map[string]any{"issue_key": "WP-1", "round": 1, "finding": "R1", "status": "verified"}, nil)

	var followUp issueOutputForTest
	a.mustTool("create_issue", map[string]any{"project_key": "WP", "title": "Follow-up", "status": "backlog", "parent_issue_key": "WP-1", "labels": []string{"Backend"}}, &followUp)
	if followUp.Key != "WP-2" {
		t.Fatalf("created %+v", followUp)
	}

	// Every tool write was recorded as the agent.
	agent := a.agentActor()
	events, err := a.store.ListEvents(context.Background(), store.EventFilter{ActorID: agent.ID, ProjectEventsOnly: true, Limit: 200})
	if err != nil {
		t.Fatal(err)
	}
	var kinds []string
	for _, event := range events {
		kinds = append(kinds, event.Kind)
	}
	want := "issue.created,issue.checklist_changed,review_round.started,finding.added,comment.added,review_round.finished," +
		"comment.added,finding.status_changed,comment.added,finding.status_changed,issue.created"
	if strings.Join(kinds, ",") != want {
		t.Fatalf("agent events %v", kinds)
	}
}

func TestMCPToolErrorsNameTheArgument(t *testing.T) {
	a := newAPITeam(t)
	a.project()

	for _, attempt := range []struct {
		tool string
		args map[string]any
		want string
	}{
		{"get_issue", map[string]any{}, "issue_key is required"},
		{"get_issue", map[string]any{"issue_key": "WP-1", "extra": true}, "extra is not an argument"},
		{"get_issue", map[string]any{"issue_key": 5}, "issue_key must be text"},
		{"add_finding", map[string]any{"issue_key": "WP-1", "severity": "nit", "title": "T", "problem": "P", "fix": "F"}, "no review round in progress"},
		{"start_review_round", map[string]any{"issue_key": "WP-1", "to_commit": "nope"}, "to_commit must be a commit SHA"},
		{"set_finding_status", map[string]any{"issue_key": "WP-1", "finding": "R1", "status": "fixed"}, "round is required"},
		{"create_issue", map[string]any{"project_key": "WP", "title": "T", "status": "someday"}, "status must be"},
		{"get_issue", map[string]any{"issue_key": "WP-99"}, "there is no issue WP-99"},
		{"list_comments", map[string]any{"issue_key": "WP-1", "finding": "R1"}, "round is required with finding"},
	} {
		text, failed := a.tool(attempt.tool, attempt.args)
		if !failed || !strings.Contains(text, attempt.want) {
			t.Errorf("%s(%v) = %q (error %v), want %q", attempt.tool, attempt.args, text, failed, attempt.want)
		}
	}
}

type commentOutputForTest struct {
	ID      string `json:"id"`
	Author  string `json:"author"`
	ReplyTo string `json:"reply_to"`
	Body    string `json:"body"`
}

type issueOutputForTest struct {
	Key       string `json:"key"`
	URL       string `json:"url"`
	Checklist string `json:"checklist"`
}

// agentActor returns the member's Claude Code agent.
func (a *apiTeam) agentActor() store.Actor {
	a.t.Helper()
	actors, err := a.store.ListActors(context.Background())
	if err != nil {
		a.t.Fatal(err)
	}
	for _, actor := range actors {
		if actor.Kind == store.ActorAgent {
			return actor
		}
	}
	a.t.Fatal("no agent")
	return store.Actor{}
}
