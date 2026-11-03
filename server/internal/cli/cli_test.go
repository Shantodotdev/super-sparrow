package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/shantodotdev/super-sparrow/server/internal/auth"
	"github.com/shantodotdev/super-sparrow/server/internal/database"
	"github.com/shantodotdev/super-sparrow/server/internal/email"
	"github.com/shantodotdev/super-sparrow/server/internal/httpserver"
	"github.com/shantodotdev/super-sparrow/server/internal/secretbox"
	"github.com/shantodotdev/super-sparrow/server/internal/store"
	"github.com/shantodotdev/super-sparrow/server/migrations"
)

type noMail struct{}

func (noMail) Send(context.Context, email.Message) error { return nil }

// cliServer runs the API with project WP and returns the environment a client needs.
func cliServer(t *testing.T) (map[string]string, *store.Store, store.Caller) {
	t.Helper()
	ctx := context.Background()

	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := database.Migrate(ctx, db, migrations.Files); err != nil {
		t.Fatal(err)
	}

	st := store.New(db, time.Now)
	box, err := secretbox.New(bytes.Repeat([]byte{7}, secretbox.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	st.UseSecretBox(box)
	owner, err := st.CreateFirstOwner(ctx, "Olive Owner", "olive@example.com")
	if err != nil {
		t.Fatal(err)
	}
	person := store.Caller{ActorID: owner.ID}
	if _, err := st.CreateProject(ctx, person, store.NewProject{Key: "WP", Name: "WP Agent"}); err != nil {
		t.Fatal(err)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	server := httptest.NewUnstartedServer(nil)
	publicURL, _ := url.Parse("http://" + server.Listener.Addr().String())
	service := auth.New(st, noMail{}, publicURL, []byte("test-sign-in-code-key-0123456789"), logger, time.Now)
	server.Config.Handler = httpserver.New(httpserver.Options{
		Logger: logger, DB: db, Store: st, Auth: service, PublicURL: publicURL, Now: time.Now, Version: "test",
	})
	server.Start()
	t.Cleanup(server.Close)

	token, err := service.CreateToken(ctx, person, "codex", "ci")
	if err != nil {
		t.Fatal(err)
	}
	return map[string]string{"SPARROW_URL": server.URL, "SPARROW_TOKEN": token.Secret, "SPARROW_MODEL": "gpt-test"}, st, person
}

// memoryKeyring is a Keyring held in memory, so tests never touch the real keychain. A failing
// keyring reports every operation as unavailable.
type memoryKeyring struct {
	values  map[string]string
	failing bool
}

var errKeychainDown = errors.New("keychain unavailable")

func (k *memoryKeyring) Get(service, user string) (string, error) {
	if k.failing {
		return "", errKeychainDown
	}
	if value, ok := k.values[service+"/"+user]; ok {
		return value, nil
	}
	return "", errNoEntry
}

func (k *memoryKeyring) Set(service, user, secret string) error {
	if k.failing {
		return errKeychainDown
	}
	if k.values == nil {
		k.values = map[string]string{}
	}
	k.values[service+"/"+user] = secret
	return nil
}

func (k *memoryKeyring) Delete(service, user string) error {
	if k.failing {
		return errKeychainDown
	}
	if _, ok := k.values[service+"/"+user]; !ok {
		return errNoEntry
	}
	delete(k.values, service+"/"+user)
	return nil
}

// testEnvironment returns an environment with an in-memory keychain and a temporary config
// directory. vars are the process environment.
func testEnvironment(t *testing.T, vars map[string]string, stdin string) (*Environment, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	configDir := t.TempDir()
	return &Environment{
		Getenv:     func(name string) string { return vars[name] },
		Stdin:      strings.NewReader(stdin),
		Stdout:     &stdout,
		Stderr:     &stderr,
		Version:    "dev",
		Keyring:    &memoryKeyring{},
		ConfigDir:  func() (string, error) { return configDir, nil },
		Hostname:   func() (string, error) { return "test-host", nil },
		HTTPClient: &http.Client{},
	}, &stdout, &stderr
}

// run runs a client command with --json and decodes its JSON output into dst when given.
func run(t *testing.T, vars map[string]string, stdin string, dst any, args ...string) error {
	t.Helper()
	env, stdout, _ := testEnvironment(t, vars, stdin)
	err := Run(context.Background(), append([]string{"--json"}, args...), env)
	if err == nil && dst != nil {
		if decodeErr := json.Unmarshal(stdout.Bytes(), dst); decodeErr != nil {
			t.Fatalf("decode %q: %v", stdout.String(), decodeErr)
		}
	}
	return err
}

func mustRun(t *testing.T, env map[string]string, stdin string, dst any, args ...string) {
	t.Helper()
	if err := run(t, env, stdin, dst, args...); err != nil {
		t.Fatalf("%v: %v", args, err)
	}
}

func TestCLIRunsTheReviewLoop(t *testing.T) {
	env, st, person := cliServer(t)
	ctx := context.Background()

	var created struct {
		Issue struct {
			Key string `json:"key"`
			ID  string `json:"id"`
		} `json:"issue"`
	}
	mustRun(t, env, "- [ ] One\n- [ ] Two\n", &created, "issue", "create", "--project", "WP", "--title", "Meta", "--status", "todo",
		"--plan-file", "-", "--branch", "feat/wp-1-meta")
	if created.Issue.Key != "WP-1" {
		t.Fatalf("created %+v", created)
	}

	mustRun(t, env, "", nil, "issue", "for-branch", "feat/wp-1-meta")
	mustRun(t, env, "", nil, "plan", "check", "WP-1", "0", "1")
	mustRun(t, env, "", nil, "plan", "check", "WP-1", "1", "--uncheck")
	mustRun(t, env, "", nil, "issue", "status", "WP-1", "in_review")
	mustRun(t, env, "", nil, "review", "start", "WP-1")
	mustRun(t, env, "", nil, "finding", "add", "WP-1", "1", "--severity", "should_fix", "--title", "Name", "--problem", "P", "--fix", "F",
		"--file", "a.go", "--line", "2")
	mustRun(t, env, "", nil, "comment", "add", "WP-1", "--finding", "1/R1", "--body", "Agreed.")
	mustRun(t, env, "", nil, "review", "finish", "WP-1", "1", "--summary", "One finding.")
	mustRun(t, env, "", nil, "review", "get", "WP-1", "1")

	finding, err := st.GetFindingByNumber(ctx, created.Issue.ID, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.ChangeFindingStatus(ctx, person, finding.ID, store.FindingChange{Status: store.FindingFix}); err != nil {
		t.Fatal(err)
	}

	var open struct {
		Findings []struct {
			Label string `json:"label"`
		} `json:"findings"`
	}
	mustRun(t, env, "", &open, "finding", "list", "WP-1", "--status", "fix")
	if len(open.Findings) != 1 || open.Findings[0].Label != "R1" {
		t.Fatalf("findings to fix %+v", open)
	}
	mustRun(t, env, "", nil, "fix-summary", "WP-1", "--summary", "Renamed.", "--findings", "1/R1")
	mustRun(t, env, "", nil, "finding", "status", "WP-1", "1", "R1", "verified")

	var plan struct {
		Issue struct {
			Checklist []struct {
				Checked bool `json:"checked"`
			} `json:"checklist"`
		} `json:"issue"`
	}
	mustRun(t, env, "", &plan, "plan", "get", "WP-1")
	if len(plan.Issue.Checklist) != 2 || !plan.Issue.Checklist[0].Checked || plan.Issue.Checklist[1].Checked {
		t.Fatalf("checklist %+v", plan.Issue.Checklist)
	}

	// The changes are the agent's, with the model it reported.
	events, err := st.ListEvents(ctx, store.EventFilter{TargetType: store.TargetFinding, Limit: 200})
	if err != nil {
		t.Fatal(err)
	}
	last := events[len(events)-1]
	actor, err := st.GetActor(ctx, last.ActorID)
	if err != nil || actor.Kind != store.ActorAgent || actor.AgentName != "codex" || last.ReportedModel != "gpt-test" {
		t.Fatalf("last finding event by %+v with model %q", actor, last.ReportedModel)
	}
}

func TestCLIReportsErrorsWithTheirField(t *testing.T) {
	env, _, _ := cliServer(t)

	err := run(t, env, "", nil, "issue", "create", "--project", "WP", "--title", "T", "--status", "someday")
	var failure *apiError
	if !errors.As(err, &failure) || failure.Field != "status" || failure.Status != 400 {
		t.Fatalf("error = %v", err)
	}

	if err := run(t, env, "", nil, "fix-summary", "WP-1", "--summary", "S", "--findings", "R1"); err == nil || !strings.Contains(err.Error(), "must look like 2/R3") {
		t.Errorf("bad finding reference: %v", err)
	}
	if err := run(t, env, "", nil, "issue", "frobnicate"); err == nil || !strings.Contains(err.Error(), "unknown command") {
		t.Errorf("unknown command: %v", err)
	}
}

func TestCLINeedsItsSettings(t *testing.T) {
	err := run(t, nil, "", nil, "issue", "get", "WP-1")
	if err == nil || !strings.Contains(err.Error(), "sparrow login") || !strings.Contains(err.Error(), "SPARROW_TOKEN") {
		t.Fatalf("error = %v", err)
	}
}

func TestCLIRejectsRetiredCommitFlags(t *testing.T) {
	env, _, _ := cliServer(t)
	for _, args := range [][]string{
		{"review", "start", "WP-1", "--to", "abc1234"},
		{"review", "start", "WP-1", "--from", "abc1234"},
		{"review", "start", "WP-1", "--uncommitted"},
		{"finding", "add", "WP-1", "1", "--commit", "abc1234"},
		{"finding", "status", "WP-1", "1", "R1", "fixed", "--commit", "abc1234"},
		{"fix-summary", "WP-1", "--commit", "abc1234"},
	} {
		if err := run(t, env, "", nil, args...); err == nil || !strings.Contains(err.Error(), "flag provided but not defined") {
			t.Errorf("%v: %v", args, err)
		}
	}
}

func TestCLICallsAllAgentTools(t *testing.T) {
	env, st, _ := cliServer(t)
	var tools struct {
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
	}
	mustRun(t, env, "", &tools, "tool", "list")
	if len(tools.Tools) < 27 {
		t.Fatalf("listed %d tools", len(tools.Tools))
	}
	var issue struct {
		Key string `json:"key"`
	}
	args := `{"project_key":"WP","title":"Tool issue","status":"todo","idempotency_key":"cli-retry"}`
	mustRun(t, env, args, &issue, "tool", "call", "create_issue", "--arguments-file", "-")
	if issue.Key != "WP-1" {
		t.Fatalf("issue %+v", issue)
	}
	mustRun(t, env, "", &issue, "tool", "call", "create_issue", "--arguments", args)
	mustRun(t, env, "", nil, "tool", "call", "create_subissue", "--arguments", `{"issue_key":"WP-1","title":"Task","status":"todo"}`)
	var listed struct {
		Subissues []struct {
			Key string `json:"key"`
		} `json:"subissues"`
	}
	mustRun(t, env, "", &listed, "tool", "call", "list_subissues", "--arguments", `{"issue_key":"WP-1"}`)
	if len(listed.Subissues) != 1 || listed.Subissues[0].Key != "WP-1.1" {
		t.Fatalf("subissues %+v", listed)
	}
	if err := run(t, env, "", nil, "tool", "call", "get_issue", "--arguments", `{"issue_key":null}`); err == nil || !strings.Contains(err.Error(), "issue_key") {
		t.Fatalf("tool error %v", err)
	}
	if err := run(t, env, "", nil, "tool", "call", "get_issue", "--arguments", `[]`); err == nil {
		t.Fatal("CLI accepted non-object arguments")
	}
	events, err := st.ListEvents(context.Background(), store.EventFilter{ProjectEventsOnly: true, Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	agentWrites := 0
	for _, event := range events {
		if event.Kind == "issue.created" || event.Kind == "issue.subissue_created" {
			actor, err := st.GetActor(context.Background(), event.ActorID)
			if err != nil {
				t.Fatal(err)
			}
			if actor.Kind != store.ActorAgent || actor.DisplayName != "Codex" {
				t.Fatalf("wrong actor %+v", actor)
			}
			agentWrites++
		}
	}
	if agentWrites != 2 {
		t.Fatalf("CLI recorded %d agent writes", agentWrites)
	}
}
