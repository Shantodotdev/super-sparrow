package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/shantodotdev/super-sparrow/server/internal/store"
)

// cliWith runs one command like cli, after change adjusts the environment.
func (w *e2eWorld) cliWith(change func(*Environment), stdin io.Reader, args ...string) result {
	w.t.Helper()
	var stdout, stderr bytes.Buffer
	env := &Environment{
		Getenv:     func(string) string { return "" },
		Stdin:      stdin,
		Stdout:     &stdout,
		Stderr:     &stderr,
		Version:    "dev",
		Keyring:    w.keyring,
		ConfigDir:  func() (string, error) { return w.configDir, nil },
		Hostname:   func() (string, error) { return "e2e-host", nil },
		HTTPClient: &http.Client{},
	}
	change(env)
	err := Run(context.Background(), args, env)
	return result{stdout: stdout.String(), stderr: stderr.String(), err: err}
}

// appSession signs the owner in as the desktop app does and returns its session cookie.
func (w *e2eWorld) appSession() *http.Cookie {
	w.t.Helper()
	post := func(path string, body map[string]string) *http.Response {
		encoded, _ := json.Marshal(body)
		response, err := http.Post(w.url+path, "application/json", bytes.NewReader(encoded))
		if err != nil {
			w.t.Fatal(err)
		}
		return response
	}

	post("/api/auth/code", map[string]string{"email": e2eOwnerEmail}).Body.Close()
	var code string
	select {
	case code = <-w.mail.codes:
	case <-time.After(10 * time.Second):
		w.t.Fatal("no sign-in code was emailed")
	}
	response := post("/api/auth/verify", map[string]string{"email": e2eOwnerEmail, "code": code})
	defer response.Body.Close()
	for _, cookie := range response.Cookies() {
		if strings.HasSuffix(cookie.Name, "sparrow_session") {
			return cookie
		}
	}
	w.t.Fatalf("app sign-in set no session cookie (status %d)", response.StatusCode)
	return nil
}

// decideInApp approves or denies the request behind an approval link as the owner in the app.
func (w *e2eWorld) decideInApp(link string, approve bool) {
	parsed, err := url.Parse(link)
	if err != nil || parsed.Scheme != "super-sparrow" {
		w.t.Errorf("approval link %q", link)
		return
	}
	action := "deny"
	if approve {
		action = "approve"
	}

	request, _ := http.NewRequest(http.MethodPost, w.url+"/api/auth/cli/requests/"+parsed.Query().Get("code")+"/"+action, nil)
	request.AddCookie(w.appSession())
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		w.t.Error(err)
		return
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		w.t.Errorf("%s: status %d", action, response.StatusCode)
	}
}

func TestE2ELoginIsApprovedInTheApp(t *testing.T) {
	w := newE2E(t, "")

	got := w.cliWith(func(env *Environment) {
		env.OpenURL = func(link string) error {
			go w.decideInApp(link, true)
			return nil
		}
	}, strings.NewReader(""), "login", "--url", w.url)
	if got.err != nil {
		t.Fatalf("login: %v\n%s", got.err, got.stderr)
	}
	if !strings.Contains(got.stderr, "Code: ") || !strings.Contains(got.stdout, "Signed in to "+w.url+" as Olive Owner") {
		t.Fatalf("stdout %q stderr %q", got.stdout, got.stderr)
	}
	if me := w.whoami(nil); me.ID != w.owner.ID {
		t.Fatalf("the CLI acts as %+v", me)
	}
}

func TestE2ELoginDeniedInTheAppStoresNothing(t *testing.T) {
	w := newE2E(t, "")

	got := w.cliWith(func(env *Environment) {
		env.OpenURL = func(link string) error {
			go w.decideInApp(link, false)
			return nil
		}
	}, strings.NewReader(""), "login", "--url", w.url)
	if got.err == nil || !strings.Contains(got.err.Error(), "denied") {
		t.Fatalf("login: %v", got.err)
	}
	if _, err := w.keyring.Get(keyringService, keyringUser); err == nil {
		t.Fatal("a denied login was stored")
	}
}

func TestLoginUsesTheReleaseServerURL(t *testing.T) {
	env := &Environment{Getenv: func(string) string { return "" }, ReleaseServerURL: "https://sparrow.example.com"}
	got, err := env.serverURL("")
	if err != nil || got.String() != "https://sparrow.example.com" {
		t.Fatalf("serverURL = %v, %v", got, err)
	}

	env.ReleaseServerURL = ""
	if _, err := env.serverURL(""); err == nil {
		t.Fatal("a build without a server and no --url was accepted")
	}
}

// mcpSession runs `sparrow --agent claude-code mcp` with messages on standard input and returns
// the answers by request id.
func (w *e2eWorld) mcpSession(messages ...string) map[string]json.RawMessage {
	w.t.Helper()
	got := w.cli(nil, strings.NewReader(strings.Join(messages, "\n")+"\n"), "--agent", "claude-code", "mcp")
	if got.err != nil {
		w.t.Fatalf("mcp: %v\n%s", got.err, got.stderr)
	}

	answers := map[string]json.RawMessage{}
	scanner := bufio.NewScanner(strings.NewReader(got.stdout))
	scanner.Buffer(make([]byte, 0, 1<<20), 1<<20)
	for scanner.Scan() {
		var answer struct {
			ID json.RawMessage `json:"id"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &answer); err != nil {
			w.t.Fatalf("answer %q is not one JSON message: %v", scanner.Text(), err)
		}
		answers[string(answer.ID)] = append(json.RawMessage(nil), scanner.Bytes()...)
	}
	return answers
}

func TestE2EMCPBridgeActsAsThePersonsAgent(t *testing.T) {
	w := newE2E(t, "")
	w.login()

	answers := w.mcpSession(
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":"three","method":"tools/call","params":{"name":"create_issue","arguments":{"project_key":"WP","title":"From the bridge","status":"todo","idempotency_key":"bridge-1"}}}`,
		`not json`,
	)

	if len(answers) != 4 {
		t.Fatalf("want answers for 1, 2, \"three\", and the bad line; got %v", answers)
	}
	if !strings.Contains(string(answers["2"]), `"create_issue"`) {
		t.Fatalf("tools/list: %s", answers["2"])
	}
	if strings.Contains(string(answers[`"three"`]), `"isError":true`) || strings.Contains(string(answers[`"three"`]), `"error"`) {
		t.Fatalf("create_issue: %s", answers[`"three"`])
	}
	if !strings.Contains(string(answers["null"]), "-32700") {
		t.Fatalf("bad line: %s", answers["null"])
	}

	// The issue is recorded as the owner's Claude Code, never as the owner.
	agent, ok := w.agentActor("claude-code")
	if !ok {
		t.Fatal("no Claude Code agent was created for the owner")
	}
	issues, err := w.st.ListIssues(context.Background(), store.IssueFilter{ProjectID: w.projectID})
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 1 || issues[0].CreatedByID != agent.ID {
		t.Fatalf("issues %+v, want one created by %s", issues, agent.ID)
	}
}

func TestMCPBridgeNeedsAnAgentForAStoredLogin(t *testing.T) {
	w := newE2E(t, "")
	w.login()

	got := w.cli(nil, strings.NewReader(""), "mcp")
	if got.err == nil || !strings.Contains(got.err.Error(), "--agent") {
		t.Fatalf("mcp without an agent: %v", got.err)
	}
}

func TestMCPBridgeReportsARejectedLoginToTheAgent(t *testing.T) {
	w := newE2E(t, "")
	w.login()
	w.ok(nil, "", "logout")
	// Put back a login whose session is revoked.
	w.keyring.Set(keyringService, keyringUser, `{"url":"`+w.url+`","token":"ssc_`+strings.Repeat("a", 43)+`"}`)

	answers := w.mcpSession(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	if !strings.Contains(string(answers["1"]), "sparrow login") {
		t.Fatalf("answer %s", answers["1"])
	}
}

// recordedCommands is a fake for running claude and codex.
type recordedCommands struct {
	mu    sync.Mutex
	lines []string
}

func (c *recordedCommands) run(_ context.Context, name string, args ...string) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lines = append(c.lines, name+" "+strings.Join(args, " "))
	return nil, nil
}

func TestE2ESetupRegistersInstalledAgents(t *testing.T) {
	w := newE2E(t, "")
	w.login()
	commands := &recordedCommands{}

	got := w.cliWith(func(env *Environment) {
		env.Executable = func() (string, error) { return "/opt/bin/sparrow", nil }
		env.LookPath = func(program string) (string, error) {
			if program == "claude" {
				return "/usr/bin/claude", nil
			}
			return "", io.EOF
		}
		env.Command = commands.run
	}, strings.NewReader(""), "setup")
	if got.err != nil {
		t.Fatalf("setup: %v\n%s\n%s", got.err, got.stdout, got.stderr)
	}

	want := []string{
		"claude mcp remove --scope user super-sparrow",
		"claude mcp add --scope user super-sparrow -- /opt/bin/sparrow --agent claude-code mcp",
	}
	if strings.Join(commands.lines, "\n") != strings.Join(want, "\n") {
		t.Fatalf("commands:\n%s", strings.Join(commands.lines, "\n"))
	}
	if !strings.Contains(got.stdout, "✓ Claude Code connected as") {
		t.Fatalf("stdout %q", got.stdout)
	}
	if !strings.Contains(got.stderr, "Using the stored login") {
		t.Fatalf("setup signed in again: %q", got.stderr)
	}
}

func TestSetupRefusesAnAgentThatIsNotInstalled(t *testing.T) {
	w := newE2E(t, "")
	got := w.cliWith(func(env *Environment) {
		env.Executable = func() (string, error) { return "/opt/bin/sparrow", nil }
		env.LookPath = func(string) (string, error) { return "", io.EOF }
		env.Command = (&recordedCommands{}).run
	}, strings.NewReader(""), "setup", "--agents", "codex")
	if got.err == nil || !strings.Contains(got.err.Error(), "Codex is not installed") {
		t.Fatalf("setup: %v", got.err)
	}
}

func TestE2EMCPBridgeSkipsAnOversizeMessageAndKeepsServing(t *testing.T) {
	w := newE2E(t, "")
	w.login()

	huge := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"list_projects","arguments":{"x":"` +
		strings.Repeat("a", maxMCPMessage) + `"}}}`
	answers := w.mcpSession(huge, `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)

	if !strings.Contains(string(answers["null"]), "-32600") {
		t.Fatalf("oversize message: %s", answers["null"])
	}
	if !strings.Contains(string(answers["2"]), `"tools"`) {
		t.Fatalf("the next message was not served: %v", answers)
	}
}

func TestSubcommandHelpPrintsTheUsage(t *testing.T) {
	w := newE2E(t, "")
	for _, command := range []string{"login", "setup", "mcp"} {
		got := w.cli(nil, strings.NewReader(""), command, "--help")
		if got.err != nil || !strings.Contains(got.stdout, "Client commands:") {
			t.Errorf("%s --help: %v, %q", command, got.err, got.stdout)
		}
	}
}
