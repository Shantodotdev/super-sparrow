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
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/shantodotdev/super-sparrow/server/internal/auth"
	"github.com/shantodotdev/super-sparrow/server/internal/database"
	"github.com/shantodotdev/super-sparrow/server/internal/email"
	"github.com/shantodotdev/super-sparrow/server/internal/httpserver"
	"github.com/shantodotdev/super-sparrow/server/internal/release"
	"github.com/shantodotdev/super-sparrow/server/internal/store"
	"github.com/shantodotdev/super-sparrow/server/internal/store/storetest"
	"github.com/shantodotdev/super-sparrow/server/migrations"
)

const e2eOwnerEmail = "olive@example.com"

var e2eCodePattern = regexp.MustCompile(`\b[0-9]{6}\b`)

// captureMail keeps the sign-in codes the server emails.
type captureMail struct{ codes chan string }

func (m *captureMail) Send(_ context.Context, message email.Message) error {
	if code := e2eCodePattern.FindString(message.Text); code != "" {
		m.codes <- code
	}
	return nil
}

// codeReader is standard input that yields the next emailed code when first read, so a test can
// feed `sparrow login` a code that does not exist until the command has asked for it.
type codeReader struct {
	codes   chan string
	pending *strings.Reader
}

func (r *codeReader) Read(p []byte) (int, error) {
	if r.pending == nil {
		select {
		case code := <-r.codes:
			r.pending = strings.NewReader(code + "\n")
		case <-time.After(10 * time.Second):
			return 0, errors.New("no sign-in code was emailed")
		}
	}
	return r.pending.Read(p)
}

// e2eWorld is a real server with a store, the CLI's in-memory keychain, and the owner's data.
type e2eWorld struct {
	t         *testing.T
	url       string
	st        *store.Store
	mail      *captureMail
	keyring   *memoryKeyring
	configDir string
	owner     store.Actor
	projectID string
}

// result is what a CLI invocation produced.
type result struct {
	stdout string
	stderr string
	err    error
}

// cli runs one command in a process environment of vars and the given standard input. Every call
// shares the world's keychain, as separate invocations on one computer do.
func (w *e2eWorld) cli(vars map[string]string, stdin io.Reader, args ...string) result {
	w.t.Helper()
	var stdout, stderr bytes.Buffer
	env := &Environment{
		Getenv:     func(name string) string { return vars[name] },
		Stdin:      stdin,
		Stdout:     &stdout,
		Stderr:     &stderr,
		Version:    "dev",
		Keyring:    w.keyring,
		ConfigDir:  func() (string, error) { return w.configDir, nil },
		Hostname:   func() (string, error) { return "e2e-host", nil },
		HTTPClient: &http.Client{},
	}
	err := Run(context.Background(), args, env)
	return result{stdout: stdout.String(), stderr: stderr.String(), err: err}
}

// login signs the owner in through the real emailed-code flow.
func (w *e2eWorld) login() {
	w.t.Helper()
	got := w.cli(nil, &codeReader{codes: w.mail.codes}, "login", "--url", w.url, "--email", e2eOwnerEmail)
	if got.err != nil {
		w.t.Fatalf("login: %v\n%s", got.err, got.stderr)
	}
}

// ok runs a command and fails the test when it errors.
func (w *e2eWorld) ok(vars map[string]string, stdin string, args ...string) string {
	w.t.Helper()
	got := w.cli(vars, strings.NewReader(stdin), args...)
	if got.err != nil {
		w.t.Fatalf("%v: %v\n%s", args, got.err, got.stderr)
	}
	return got.stdout
}

// agentActor returns the owner's agent called name.
func (w *e2eWorld) agentActor(name string) (store.Actor, bool) {
	w.t.Helper()
	actors, err := w.st.ListActors(context.Background())
	if err != nil {
		w.t.Fatal(err)
	}
	for _, actor := range actors {
		if actor.Kind == store.ActorAgent && actor.AgentName == name && actor.OwnerActorID == w.owner.ID {
			return actor, true
		}
	}
	return store.Actor{}, false
}

// whoami returns the actor /api/me reports for the stored login.
func (w *e2eWorld) whoami(vars map[string]string, extra ...string) store.Actor {
	w.t.Helper()
	var body struct {
		Actor struct {
			ID          string `json:"id"`
			Kind        string `json:"kind"`
			DisplayName string `json:"display_name"`
			AgentName   string `json:"agent_name"`
			OwnerID     string `json:"owner_actor_id"`
		} `json:"actor"`
	}
	out := w.ok(vars, "", append([]string{"--json"}, append(extra, "whoami")...)...)
	if err := json.Unmarshal([]byte(out), &body); err != nil {
		w.t.Fatalf("decode %q: %v", out, err)
	}
	return store.Actor{ID: body.Actor.ID, Kind: store.ActorKind(body.Actor.Kind), DisplayName: body.Actor.DisplayName, AgentName: body.Actor.AgentName, OwnerActorID: body.Actor.OwnerID}
}

func TestE2ELoginStoresCredentialsAndIdentifiesTheCaller(t *testing.T) {
	w := newE2E(t, "")
	w.login()

	stored, err := w.keyring.Get(keyringService, keyringUser)
	if err != nil {
		t.Fatalf("keyring holds no login: %v", err)
	}
	var creds credentials
	if err := json.Unmarshal([]byte(stored), &creds); err != nil {
		t.Fatal(err)
	}
	if creds.URL != w.url || !strings.HasPrefix(creds.Token, "ssc_") {
		t.Fatalf("stored credentials = %+v", creds)
	}
	if _, err := os.Stat(filepath.Join(w.configDir, "sparrow", "credentials.json")); err == nil {
		t.Fatal("the fallback credentials file exists although the keychain worked")
	}

	person := w.whoami(nil)
	if person.ID != w.owner.ID || person.Kind != store.ActorHuman {
		t.Fatalf("whoami = %+v, want the owner", person)
	}
	text := w.ok(nil, "", "whoami")
	if !strings.Contains(text, "Olive Owner") || !strings.Contains(text, "none (acting as the person)") {
		t.Fatalf("whoami text:\n%s", text)
	}

	claude := w.whoami(map[string]string{"CLAUDECODE": "1"})
	if claude.Kind != store.ActorAgent || claude.AgentName != "claude-code" || claude.OwnerActorID != w.owner.ID {
		t.Fatalf("with CLAUDECODE=1 whoami = %+v, want the owner's claude-code agent", claude)
	}
	if _, ok := w.agentActor("claude-code"); !ok {
		t.Fatal("the store has no claude-code agent for the owner")
	}

	codex := w.whoami(map[string]string{"SPARROW_AGENT": "codex"})
	if codex.Kind != store.ActorAgent || codex.AgentName != "codex" || codex.OwnerActorID != w.owner.ID {
		t.Fatalf("with SPARROW_AGENT=codex whoami = %+v, want the owner's codex agent", codex)
	}

	flagged := w.whoami(nil, "--agent", "codex")
	if flagged.AgentName != "codex" {
		t.Fatalf("with --agent codex whoami = %+v", flagged)
	}

	got := w.cli(nil, strings.NewReader(""), "--agent", "bogus", "whoami")
	if got.err == nil || !strings.Contains(got.err.Error(), "bogus") {
		t.Fatalf("--agent bogus error = %v", got.err)
	}
	got = w.cli(map[string]string{"SPARROW_AGENT": "bogus"}, strings.NewReader(""), "whoami")
	if got.err == nil || !strings.Contains(got.err.Error(), "SPARROW_AGENT") {
		t.Fatalf("SPARROW_AGENT=bogus error = %v", got.err)
	}
}

func TestE2ELoginRejectsAWrongCode(t *testing.T) {
	w := newE2E(t, "")

	got := w.cli(nil, strings.NewReader("000000\n"), "login", "--url", w.url, "--email", e2eOwnerEmail)
	if got.err == nil {
		t.Fatal("login with a wrong code succeeded")
	}
	if _, err := w.keyring.Get(keyringService, keyringUser); err == nil {
		t.Fatal("a failed login stored credentials")
	}
}

func TestE2EToolCommandsRunThroughMCPAndRecordTheActor(t *testing.T) {
	w := newE2E(t, "")
	w.login()
	ctx := context.Background()
	claude := map[string]string{"CLAUDECODE": "1"}

	// Outside an agent, a tool command runs as the signed-in person.
	w.ok(nil, "", "create-issue", "--project-key", "WP", "--title", "By the person through a tool", "--status", "todo")
	byTool, err := w.st.GetIssueByKey(ctx, "WP-1")
	if err != nil {
		t.Fatal(err)
	}
	if byTool.CreatedByID != w.owner.ID {
		t.Fatalf("WP-1 created by %s, want the person %s", byTool.CreatedByID, w.owner.ID)
	}

	var first struct {
		Issue struct {
			Key string `json:"key"`
		} `json:"issue"`
	}
	created := w.ok(nil, "", "--json", "issue", "create", "--project", "WP", "--title", "By the person", "--status", "todo")
	if err := json.Unmarshal([]byte(created), &first); err != nil || first.Issue.Key != "WP-2" {
		t.Fatalf("created issue = %s (%v)", created, err)
	}

	// A write inside Claude Code is the person's claude-code agent.
	w.ok(claude, "", "create-issue", "--project-key", "WP", "--title", "By the agent", "--status", "todo")

	byPerson, err := w.st.GetIssueByKey(ctx, "WP-2")
	if err != nil {
		t.Fatal(err)
	}
	if byPerson.CreatedByID != w.owner.ID {
		t.Fatalf("WP-2 created by %s, want the person %s", byPerson.CreatedByID, w.owner.ID)
	}
	agent, ok := w.agentActor("claude-code")
	if !ok {
		t.Fatal("no claude-code agent was created")
	}
	byAgent, err := w.st.GetIssueByKey(ctx, "WP-3")
	if err != nil {
		t.Fatal(err)
	}
	if byAgent.CreatedByID != agent.ID {
		t.Fatalf("WP-3 created by %s, want the agent %s", byAgent.CreatedByID, agent.ID)
	}

	events, err := w.st.ListEvents(ctx, store.EventFilter{IssueID: byAgent.ID, Limit: 50})
	if err != nil || len(events) == 0 {
		t.Fatalf("events = %v, %v", events, err)
	}
	for _, event := range events {
		if event.ActorID != agent.ID {
			t.Fatalf("event %s actor %s, want the agent %s", event.Kind, event.ActorID, agent.ID)
		}
	}

	// Reads, JSON output, flags, and help.
	var shown struct {
		Issue struct {
			Key   string `json:"key"`
			Title string `json:"title"`
		} `json:"issue"`
		Key   string `json:"key"`
		Title string `json:"title"`
	}
	out := w.ok(claude, "", "--json", "get-issue", "--issue-key", "WP-1")
	if err := json.Unmarshal([]byte(out), &shown); err != nil {
		t.Fatalf("get-issue --json is not JSON: %v\n%s", err, out)
	}
	if !strings.Contains(out, "By the person") || !strings.Contains(out, "WP-1") {
		t.Fatalf("get-issue output:\n%s", out)
	}
	if text := w.ok(claude, "", "get-issue", "--issue-key=WP-1"); !strings.Contains(text, "By the person") {
		t.Fatalf("get-issue text:\n%s", text)
	}

	projects := w.ok(claude, "", "--json", "list-projects")
	if !json.Valid([]byte(projects)) || !strings.Contains(projects, "WP Agent") {
		t.Fatalf("list-projects:\n%s", projects)
	}
	if text := w.ok(claude, "", "list-projects"); !strings.Contains(text, "WP") {
		t.Fatalf("list-projects text:\n%s", text)
	}

	got := w.cli(claude, strings.NewReader(""), "get-issue", "--bogus", "x")
	if got.err == nil || !strings.Contains(got.err.Error(), "unknown flag --bogus") || !strings.Contains(got.err.Error(), "--issue_key") {
		t.Fatalf("unknown flag error = %v", got.err)
	}
	got = w.cli(claude, strings.NewReader(""), "get-issue")
	if got.err == nil || !strings.Contains(got.err.Error(), "--issue_key") {
		t.Fatalf("missing flag error = %v", got.err)
	}
	got = w.cli(claude, strings.NewReader(""), "no-such-tool")
	if got.err == nil || !strings.Contains(got.err.Error(), "unknown command") {
		t.Fatalf("unknown tool error = %v", got.err)
	}

	help := w.ok(claude, "", "get-issue", "--help")
	if !strings.Contains(help, "--issue_key <string> (required)") {
		t.Fatalf("get-issue --help:\n%s", help)
	}
	help = w.ok(claude, "", "create-issue", "--help")
	for _, want := range []string{"--project_key <string> (required)", "--title <string> (required)", "--status <string> (required)", "--plan <string>"} {
		if !strings.Contains(help, want) {
			t.Fatalf("create-issue --help lacks %q:\n%s", want, help)
		}
	}
	if strings.Contains(help, "--plan <string> (required)") {
		t.Fatalf("plan is shown as required:\n%s", help)
	}
}

func TestE2ELogoutRevokesTheSessionAndClearsTheKeychain(t *testing.T) {
	w := newE2E(t, "")
	w.login()

	stored, err := w.keyring.Get(keyringService, keyringUser)
	if err != nil {
		t.Fatal(err)
	}
	var creds credentials
	if err := json.Unmarshal([]byte(stored), &creds); err != nil {
		t.Fatal(err)
	}
	status := func() int {
		request, _ := http.NewRequest(http.MethodGet, w.url+"/api/me", nil)
		request.Header.Set("Authorization", "Bearer "+creds.Token)
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		return response.StatusCode
	}
	if got := status(); got != http.StatusOK {
		t.Fatalf("session before logout: HTTP %d", got)
	}

	if out := w.ok(nil, "", "logout"); !strings.Contains(out, "Signed out") {
		t.Fatalf("logout output: %q", out)
	}
	if got := status(); got != http.StatusUnauthorized {
		t.Fatalf("old token after logout: HTTP %d, want 401", got)
	}
	if _, err := w.keyring.Get(keyringService, keyringUser); !errors.Is(err, errNoEntry) {
		t.Fatalf("keyring after logout: %v", err)
	}

	got := w.cli(nil, strings.NewReader(""), "whoami")
	if got.err == nil || !strings.Contains(got.err.Error(), "not signed in") {
		t.Fatalf("whoami after logout: %v", got.err)
	}
	if out := w.ok(nil, "", "logout"); !strings.Contains(out, "Not signed in") {
		t.Fatalf("second logout: %q", out)
	}
}

// releaseDir signs a fake release v9.9.9 for this platform and returns its folder, the binary's
// content, and the public key.
func releaseDir(t *testing.T, binary []byte) (dir, publicKey string) {
	t.Helper()
	dir = t.TempDir()

	public, private, err := release.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	writeAllBinaries(t, dir)
	name := "sparrow-" + runtime.GOOS + "-" + runtime.GOARCH
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	if err := os.WriteFile(filepath.Join(dir, name), binary, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest, err := release.BuildManifest("v9.9.9", dir)
	if err != nil {
		t.Fatal(err)
	}
	signature, err := release.Sign(manifest, private)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), manifest, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json.sig"), signature, 0o644); err != nil {
		t.Fatal(err)
	}
	return dir, release.EncodeKey(public)
}

// update runs `sparrow update` against the world's server as a build of version, replacing exe.
func (w *e2eWorld) update(publicKeys []string, version, exe string) result {
	w.t.Helper()
	var stdout, stderr bytes.Buffer
	env := &Environment{
		Getenv:     func(name string) string { return map[string]string{"SPARROW_URL": w.url}[name] },
		Stdin:      strings.NewReader(""),
		Stdout:     &stdout,
		Stderr:     &stderr,
		Version:    version,
		Keyring:    w.keyring,
		ConfigDir:  func() (string, error) { return w.configDir, nil },
		Executable: func() (string, error) { return exe, nil },
		Hostname:   func() (string, error) { return "e2e-host", nil },
		PublicKeys: publicKeys,
		HTTPClient: &http.Client{},
	}
	err := Run(context.Background(), []string{"update"}, env)
	return result{stdout: stdout.String(), stderr: stderr.String(), err: err}
}

func newExecutable(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "sparrow")
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func assertContent(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("%s holds %q, want %q", path, got, want)
	}
}

func TestE2EUpdateReplacesTheExecutableWithTheSignedRelease(t *testing.T) {
	binary := []byte("new sparrow build v9.9.9")
	dir, publicKey := releaseDir(t, binary)
	w := newE2E(t, dir)
	exe := newExecutable(t, "old build")

	got := w.update([]string{publicKey}, "v1.0.0", exe)
	if got.err != nil {
		t.Fatalf("update: %v", got.err)
	}
	if !strings.Contains(got.stdout, "v1.0.0 to v9.9.9") {
		t.Fatalf("update output: %q", got.stdout)
	}
	assertContent(t, exe, string(binary))
	if info, err := os.Stat(exe); err != nil || info.Mode().Perm()&0o100 == 0 {
		t.Fatalf("updated file is not executable: %v %v", info, err)
	}

	// Already current: nothing is replaced.
	exe = newExecutable(t, "current build")
	got = w.update([]string{publicKey}, "v9.9.9", exe)
	if got.err != nil || !strings.Contains(got.stdout, "up to date") {
		t.Fatalf("same version: %v %q", got.err, got.stdout)
	}
	assertContent(t, exe, "current build")
}

func TestE2EUpdateRefusesUntrustedOrTamperedReleases(t *testing.T) {
	binary := []byte("new sparrow build v9.9.9")
	dir, publicKey := releaseDir(t, binary)
	w := newE2E(t, dir)

	otherPublic, _, err := release.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	exe := newExecutable(t, "old build")
	got := w.update([]string{release.EncodeKey(otherPublic)}, "v1.0.0", exe)
	if got.err == nil || !strings.Contains(got.err.Error(), "unknown key") {
		t.Fatalf("wrong key: %v", got.err)
	}
	assertContent(t, exe, "old build")

	got = w.update(nil, "v1.0.0", exe)
	if got.err == nil || !strings.Contains(got.err.Error(), "no release signing key") {
		t.Fatalf("no key: %v", got.err)
	}
	assertContent(t, exe, "old build")

	// A binary changed after signing, with the same size and with a different size.
	name := "sparrow-" + runtime.GOOS + "-" + runtime.GOARCH
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	for _, tampered := range [][]byte{[]byte("evil sparrow build v9.9.9"), []byte("new sparrow build v9.9.X")} {
		if err := os.WriteFile(filepath.Join(dir, name), tampered, 0o755); err != nil {
			t.Fatal(err)
		}
		got = w.update([]string{publicKey}, "v1.0.0", exe)
		if got.err == nil || !strings.Contains(got.err.Error(), "does not match the signed manifest") && !strings.Contains(got.err.Error(), "larger than") {
			t.Fatalf("tampered binary %q: %v", tampered, got.err)
		}
		assertContent(t, exe, "old build")
	}

	// A manifest changed after signing fails its signature.
	manifestPath := filepath.Join(dir, "manifest.json")
	manifest, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, bytes.Replace(manifest, []byte("v9.9.9"), []byte("v9.9.8"), 1), 0o644); err != nil {
		t.Fatal(err)
	}
	got = w.update([]string{publicKey}, "v1.0.0", exe)
	if got.err == nil || !strings.Contains(got.err.Error(), "signature is invalid") {
		t.Fatalf("tampered manifest: %v", got.err)
	}
	assertContent(t, exe, "old build")

	// A server without releases reports it.
	bare := newE2E(t, "")
	got = bare.update([]string{publicKey}, "v1.0.0", exe)
	if got.err == nil || !strings.Contains(got.err.Error(), "HTTP 404") {
		t.Fatalf("no releases: %v", got.err)
	}
	assertContent(t, exe, "old build")
}

// newE2E starts the real HTTP handler on a fresh database with project WP. cliDir is the release
// folder the server offers, or "".
func newE2E(t *testing.T, cliDir string) *e2eWorld {
	t.Helper()
	ctx := context.Background()

	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "e2e.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := database.Migrate(ctx, db, migrations.Files); err != nil {
		t.Fatal(err)
	}

	st := store.New(db, time.Now, storetest.NewObjects())
	owner, err := st.CreateFirstOwner(ctx, "Olive Owner", e2eOwnerEmail)
	if err != nil {
		t.Fatal(err)
	}
	project, err := st.CreateProject(ctx, store.Caller{ActorID: owner.ID}, store.NewProject{Key: "WP", Name: "WP Agent"})
	if err != nil {
		t.Fatal(err)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	server := httptest.NewUnstartedServer(nil)
	publicURL, _ := url.Parse("http://" + server.Listener.Addr().String())
	mail := &captureMail{codes: make(chan string, 16)}
	service := auth.New(st, mail, publicURL, []byte("test-sign-in-code-key-0123456789"), logger, time.Now)
	server.Config.Handler = httpserver.New(httpserver.Options{
		Logger: logger, DB: db, Store: st, Auth: service, PublicURL: publicURL, Now: time.Now, Version: "test", CLIDir: cliDir,
	})
	server.Start()
	t.Cleanup(func() { server.Close(); service.Wait() })

	return &e2eWorld{
		t: t, url: server.URL, st: st, mail: mail, keyring: &memoryKeyring{}, configDir: t.TempDir(),
		owner: owner, projectID: project.ID,
	}
}
