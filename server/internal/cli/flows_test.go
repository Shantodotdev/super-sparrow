package cli

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/shantodotdev/super-sparrow/server/internal/release"
)

// recordedRequest is a request the fake server saw.
type recordedRequest struct {
	Method, Path, Query, Authorization, Agent string
	Body                                      string
}

// fakeServer is a stand-in for the Super Sparrow API that records requests.
type fakeServer struct {
	*httptest.Server
	mu       sync.Mutex
	requests []recordedRequest
	handlers map[string]http.HandlerFunc
}

// newFakeServer starts a server; handlers are keyed "METHOD /path".
func newFakeServer(t *testing.T, handlers map[string]http.HandlerFunc) *fakeServer {
	t.Helper()
	fake := &fakeServer{handlers: handlers}
	fake.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		fake.mu.Lock()
		fake.requests = append(fake.requests, recordedRequest{
			Method: r.Method, Path: r.URL.Path, Query: r.URL.RawQuery,
			Authorization: r.Header.Get("Authorization"), Agent: r.Header.Get("Sparrow-Agent"), Body: string(body),
		})
		fake.mu.Unlock()

		handler, ok := fake.handlers[r.Method+" "+r.URL.Path]
		if !ok {
			http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
			return
		}
		handler(w, r)
	}))
	t.Cleanup(fake.Close)
	return fake
}

// last returns the most recent request for the method and path.
func (f *fakeServer) last(t *testing.T, method, path string) recordedRequest {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := len(f.requests) - 1; i >= 0; i-- {
		if f.requests[i].Method == method && f.requests[i].Path == path {
			return f.requests[i]
		}
	}
	t.Fatalf("no %s %s request; saw %+v", method, path, f.requests)
	return recordedRequest{}
}

func writeJSON(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write([]byte(body))
}

// authServer fakes the sign-in endpoints.
func authServer(t *testing.T) *fakeServer {
	return newFakeServer(t, map[string]http.HandlerFunc{
		"POST /api/auth/code": func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 202, `{}`) },
		"POST /api/auth/cli/verify": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, 200, `{"token":"ssc_abc","session":{},"actor":{"display_name":"Olive","email":"olive@example.com"}}`)
		},
		"POST /api/auth/sign-out": func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) },
		"GET /api/me": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, 200, `{"actor":{"display_name":"Olive","kind":"human"}}`)
		},
	})
}

// shared makes several commands see the same keychain and config directory.
func shared(env *Environment, vars map[string]string, stdin string) *Environment {
	next := *env
	next.Getenv = func(name string) string { return vars[name] }
	next.Stdin = strings.NewReader(stdin)
	next.Stdout, next.Stderr = &strings.Builder{}, &strings.Builder{}
	return &next
}

func output(env *Environment) string { return env.Stdout.(*strings.Builder).String() }

func TestLoginStoresTheTokenInTheKeychain(t *testing.T) {
	server := authServer(t)
	env, _, _ := testEnvironment(t, nil, "123456\n")

	if err := Run(context.Background(), []string{"login", "--url", server.URL, "--email", "olive@example.com"}, env); err != nil {
		t.Fatal(err)
	}

	request := server.last(t, "POST", "/api/auth/cli/verify")
	var body map[string]string
	if err := json.Unmarshal([]byte(request.Body), &body); err != nil {
		t.Fatal(err)
	}
	if body["email"] != "olive@example.com" || body["code"] != "123456" || body["device_label"] != "test-host · CLI" {
		t.Fatalf("verify body = %v", body)
	}

	stored, err := env.store().load()
	if err != nil || stored.Token != "ssc_abc" || stored.URL != server.URL {
		t.Fatalf("stored = %+v, %v", stored, err)
	}
	if _, err := os.Stat(filepath.Join(mustConfigDir(t, env), "sparrow", "credentials.json")); err == nil {
		t.Error("wrote a credentials file although the keychain worked")
	}
	if strings.Contains(env.Stdout.(*bytes.Buffer).String()+env.Stderr.(*bytes.Buffer).String(), "ssc_abc") {
		t.Error("printed the token")
	}
}

func TestEmailLoginReadsURLFromEnvAndPromptsForTheCode(t *testing.T) {
	server := authServer(t)
	env, _, stderr := testEnvironment(t, map[string]string{"SPARROW_URL": server.URL}, " 123 456 \n")

	if err := Run(context.Background(), []string{"login", "--email", "olive@example.com"}, env); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stderr.String(), "Code: ") {
		t.Errorf("prompts = %q", stderr.String())
	}
	if code := server.last(t, "POST", "/api/auth/cli/verify").Body; !strings.Contains(code, `"123456"`) {
		t.Errorf("code not normalized: %s", code)
	}
}

func TestLoginRejectsBadInput(t *testing.T) {
	server := authServer(t)
	for name, test := range map[string]struct {
		args  []string
		stdin string
	}{
		"no url":      {[]string{"login", "--email", "a@b.c"}, "123456\n"},
		"plain http":  {[]string{"login", "--url", "http://example.com", "--email", "a@b.c"}, "123456\n"},
		"short code":  {[]string{"login", "--url", server.URL, "--email", "a@b.c"}, "123\n"},
		"no code":     {[]string{"login", "--url", server.URL, "--email", "a@b.c"}, ""},
		"unknown arg": {[]string{"login", "extra"}, ""},
	} {
		env, _, _ := testEnvironment(t, nil, test.stdin)
		if err := Run(context.Background(), test.args, env); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestLoginFallsBackToAFileWhenTheKeychainIsDown(t *testing.T) {
	server := authServer(t)
	env, _, stderr := testEnvironment(t, nil, "123456\n")
	env.Keyring = &memoryKeyring{failing: true}

	if err := Run(context.Background(), []string{"login", "--url", server.URL, "--email", "a@b.c"}, env); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(mustConfigDir(t, env), "sparrow", "credentials.json")
	if !strings.Contains(stderr.String(), "warning") || !strings.Contains(stderr.String(), path) {
		t.Errorf("no warning naming the file: %q", stderr.String())
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("credentials file = %v, %v", info, err)
		}
	}
	stored, err := env.store().load()
	if err != nil || stored.Token != "ssc_abc" {
		t.Fatalf("load from file: %+v, %v", stored, err)
	}

	// Signing out removes the file too.
	if err := Run(context.Background(), []string{"logout"}, shared(env, nil, "")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("credentials file survived logout: %v", err)
	}
}

func TestLogoutRevokesWithoutAnAgentHeader(t *testing.T) {
	server := authServer(t)
	env, _, _ := testEnvironment(t, nil, "123456\n")
	if err := Run(context.Background(), []string{"login", "--url", server.URL, "--email", "a@b.c"}, env); err != nil {
		t.Fatal(err)
	}

	inAgent := map[string]string{"CLAUDECODE": "1"}
	logout := shared(env, inAgent, "")
	if err := Run(context.Background(), []string{"logout"}, logout); err != nil {
		t.Fatal(err)
	}

	request := server.last(t, "POST", "/api/auth/sign-out")
	if request.Authorization != "Bearer ssc_abc" || request.Agent != "" {
		t.Fatalf("sign-out request = %+v", request)
	}
	if _, err := env.store().load(); err != errNotSignedIn {
		t.Errorf("login survived logout: %v", err)
	}
	if err := Run(context.Background(), []string{"logout"}, shared(env, nil, "")); err != nil {
		t.Errorf("second logout: %v", err)
	}
}

func TestWhoamiSendsTheDetectedAgentOnlyWithAStoredLogin(t *testing.T) {
	server := authServer(t)
	env, _, _ := testEnvironment(t, nil, "123456\n")
	if err := Run(context.Background(), []string{"login", "--url", server.URL, "--email", "a@b.c"}, env); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name      string
		vars      map[string]string
		args      []string
		wantAgent string
		wantInOut string
	}{
		{"person", nil, []string{"whoami"}, "", "agent: none"},
		{"detected", map[string]string{"CODEX_THREAD_ID": "1"}, []string{"whoami"}, "codex", "agent: codex (detected"},
		{"flag", map[string]string{"CLAUDECODE": "1"}, []string{"--agent", "codex", "whoami"}, "codex", "agent: codex (--agent)"},
	}
	for _, c := range cases {
		run := shared(env, c.vars, "")
		if err := Run(context.Background(), c.args, run); err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		request := server.last(t, "GET", "/api/me")
		if request.Agent != c.wantAgent || request.Authorization != "Bearer ssc_abc" {
			t.Errorf("%s: request = %+v", c.name, request)
		}
		if !strings.Contains(output(run), c.wantInOut) || !strings.Contains(output(run), "display_name: Olive") {
			t.Errorf("%s: output = %q", c.name, output(run))
		}
	}

	if err := Run(context.Background(), []string{"--agent", "bogus", "whoami"}, shared(env, nil, "")); err == nil {
		t.Error("accepted an unknown agent")
	}

	// An agent token wins over the stored login and never sends the agent header.
	token := shared(env, map[string]string{"SPARROW_URL": server.URL, "SPARROW_TOKEN": "ssp_zzz", "CLAUDECODE": "1"}, "")
	if err := Run(context.Background(), []string{"whoami"}, token); err != nil {
		t.Fatal(err)
	}
	request := server.last(t, "GET", "/api/me")
	if request.Authorization != "Bearer ssp_zzz" || request.Agent != "" {
		t.Errorf("agent token request = %+v", request)
	}
	if strings.Contains(output(token), "ssp_zzz") {
		t.Error("printed the token")
	}
}

func TestNoCredentialsNamesBothOptions(t *testing.T) {
	env, _, _ := testEnvironment(t, nil, "")
	err := Run(context.Background(), []string{"whoami"}, env)
	if err == nil || !strings.Contains(err.Error(), "sparrow login") || !strings.Contains(err.Error(), "SPARROW_TOKEN") {
		t.Fatalf("error = %v", err)
	}

	env, _, _ = testEnvironment(t, map[string]string{"SPARROW_TOKEN": "nope"}, "")
	if err := Run(context.Background(), []string{"whoami"}, env); err == nil || !strings.Contains(err.Error(), "ssp_") {
		t.Fatalf("bad token error = %v", err)
	}
}

func TestGenericToolCommands(t *testing.T) {
	server := newFakeServer(t, nil)
	server.handlers = map[string]http.HandlerFunc{}
	var calls []string
	server.handlers["POST /mcp"] = func(w http.ResponseWriter, r *http.Request) {
		server.mu.Lock()
		last := server.requests[len(server.requests)-1].Body
		server.mu.Unlock()
		calls = append(calls, last)
		switch {
		case strings.Contains(last, `"tools/list"`):
			writeJSON(w, 200, `{"jsonrpc":"2.0","id":1,"result":{"tools":[{"name":"get_issue","description":"Show an issue.\nMore.","inputSchema":{"type":"object","required":["issue_key"],"properties":{"issue_key":{"type":"string","description":"The key."},"verbose":{"type":"boolean"}}}}]}}`)
		case strings.Contains(last, `"missing"`):
			writeJSON(w, 200, `{"jsonrpc":"2.0","id":1,"result":{"isError":true,"content":[{"type":"text","text":"no such issue"}],"structuredContent":{"code":"not_found"}}}`)
		default:
			writeJSON(w, 200, `{"jsonrpc":"2.0","id":1,"result":{"structuredContent":{"issue":{"key":"WP-1","title":"Hi"}}}}`)
		}
	}
	vars := map[string]string{"SPARROW_URL": server.URL, "SPARROW_TOKEN": "ssp_t"}

	// Dashes and underscores both name the tool; the default output is text.
	for _, name := range []string{"get-issue", "get_issue"} {
		env, stdout, _ := testEnvironment(t, vars, "")
		if err := Run(context.Background(), []string{name, "--issue-key", "WP-1", "--verbose"}, env); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if stdout.String() != "issue:\n  key: WP-1\n  title: Hi\n" {
			t.Errorf("%s output = %q", name, stdout.String())
		}
	}
	if last := calls[len(calls)-1]; !strings.Contains(last, `"arguments":{"issue_key":"WP-1","verbose":true}`) {
		t.Errorf("tools/call = %s", last)
	}

	env, stdout, _ := testEnvironment(t, vars, "")
	if err := Run(context.Background(), []string{"--json", "get-issue", "--issue-key", "WP-1"}, env); err != nil || !strings.Contains(stdout.String(), `"key": "WP-1"`) {
		t.Errorf("json output = %q, %v", stdout.String(), err)
	}

	env, stdout, _ = testEnvironment(t, vars, "")
	if err := Run(context.Background(), []string{"get-issue", "--help"}, env); err != nil || !strings.Contains(stdout.String(), "--issue_key <string> (required)") {
		t.Errorf("help = %q, %v", stdout.String(), err)
	}

	env, stdout, _ = testEnvironment(t, vars, "")
	if err := Run(context.Background(), []string{"tools"}, env); err != nil || stdout.String() != "get_issue  Show an issue.\n" {
		t.Errorf("tools = %q, %v", stdout.String(), err)
	}

	env, _, _ = testEnvironment(t, vars, "")
	if err := Run(context.Background(), []string{"nope"}, env); err == nil || !strings.Contains(err.Error(), "unknown command") {
		t.Errorf("unknown tool: %v", err)
	}

	env, _, _ = testEnvironment(t, vars, "")
	err := Run(context.Background(), []string{"get-issue", "--issue-key", "missing"}, env)
	if err == nil || !strings.Contains(err.Error(), "not_found") || !strings.Contains(err.Error(), "no such issue") {
		t.Errorf("tool error = %v", err)
	}
}

// updateFixture serves a signed release and returns the environment to update with.
type updateFixture struct {
	server     *fakeServer
	env        *Environment
	executable string
	stdout     *bytes.Buffer
}

func newUpdateFixture(t *testing.T, currentVersion, releaseVersion string, tamper func(manifest []byte, binary []byte) ([]byte, []byte)) *updateFixture {
	t.Helper()
	public, private, err := release.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}

	binary := []byte("new sparrow binary")
	sum := sha256.Sum256(binary)
	target := runtime.GOOS + "-" + runtime.GOARCH
	name := "sparrow-" + target
	manifest, _ := json.Marshal(release.Manifest{Version: releaseVersion, Files: map[string]release.File{
		target: {Name: name, SHA256: hex.EncodeToString(sum[:]), Size: int64(len(binary))},
	}})
	signature, err := release.Sign(manifest, private)
	if err != nil {
		t.Fatal(err)
	}
	if tamper != nil {
		manifest, binary = tamper(manifest, binary)
	}

	server := newFakeServer(t, map[string]http.HandlerFunc{
		"GET /cli/manifest.json":     func(w http.ResponseWriter, r *http.Request) { w.Write(manifest) },
		"GET /cli/manifest.json.sig": func(w http.ResponseWriter, r *http.Request) { w.Write(signature) },
		"GET /cli/" + name:           func(w http.ResponseWriter, r *http.Request) { w.Write(binary) },
	})

	env, stdout, _ := testEnvironment(t, map[string]string{"SPARROW_URL": server.URL}, "")
	executable := filepath.Join(t.TempDir(), "sparrow")
	if err := os.WriteFile(executable, []byte("old sparrow binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	env.Version = currentVersion
	env.PublicKeys = []string{release.EncodeKey(public)}
	env.Executable = func() (string, error) { return executable, nil }
	return &updateFixture{server: server, env: env, executable: executable, stdout: stdout}
}

func (f *updateFixture) contents(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(f.executable)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestUpdateReplacesTheExecutable(t *testing.T) {
	fixture := newUpdateFixture(t, "v1.0.0", "v1.1.0", nil)
	if err := Run(context.Background(), []string{"update"}, fixture.env); err != nil {
		t.Fatal(err)
	}
	if fixture.contents(t) != "new sparrow binary" || !strings.Contains(fixture.stdout.String(), "v1.1.0") {
		t.Fatalf("executable = %q, output = %q", fixture.contents(t), fixture.stdout.String())
	}
	if runtime.GOOS != "windows" {
		info, _ := os.Stat(fixture.executable)
		if info.Mode().Perm() != 0o755 {
			t.Errorf("mode = %v", info.Mode())
		}
	}
	entries, _ := os.ReadDir(filepath.Dir(fixture.executable))
	if len(entries) != 1 {
		t.Errorf("leftover files: %v", entries)
	}
}

func TestUpdateAcceptsADevBuild(t *testing.T) {
	fixture := newUpdateFixture(t, "dev", "v0.0.1", nil)
	if err := Run(context.Background(), []string{"update"}, fixture.env); err != nil || fixture.contents(t) != "new sparrow binary" {
		t.Fatalf("dev update: %v, %q", err, fixture.contents(t))
	}
}

func TestUpdateRefusesBadReleases(t *testing.T) {
	tests := []struct {
		name    string
		current string
		version string
		tamper  func([]byte, []byte) ([]byte, []byte)
		keys    func(*Environment)
		want    string
	}{
		{"same version", "v1.1.0", "v1.1.0", nil, nil, "up to date"},
		{"downgrade", "v2.0.0", "v1.1.0", nil, nil, "up to date"},
		{"tampered manifest", "v1.0.0", "v1.1.0", func(m, b []byte) ([]byte, []byte) {
			return []byte(strings.Replace(string(m), "v1.1.0", "v9.9.9", 1)), b
		}, nil, "invalid"},
		{"tampered binary", "v1.0.0", "v1.1.0", func(m, b []byte) ([]byte, []byte) {
			return m, []byte("new sparrow binarX")
		}, nil, "does not match"},
		{"short binary", "v1.0.0", "v1.1.0", func(m, b []byte) ([]byte, []byte) { return m, b[:3] }, nil, "does not match"},
		{"unknown key", "v1.0.0", "v1.1.0", nil, func(e *Environment) {
			other, _, _ := release.GenerateKey()
			e.PublicKeys = []string{release.EncodeKey(other)}
		}, "unknown key"},
		{"no keys", "v1.0.0", "v1.1.0", nil, func(e *Environment) { e.PublicKeys = nil }, "no release signing key"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newUpdateFixture(t, test.current, test.version, test.tamper)
			if test.keys != nil {
				test.keys(fixture.env)
			}
			err := Run(context.Background(), []string{"update"}, fixture.env)
			if test.want == "up to date" {
				if err != nil || !strings.Contains(fixture.stdout.String(), "up to date") {
					t.Fatalf("err = %v, output = %q", err, fixture.stdout.String())
				}
			} else if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("err = %v, want %q", err, test.want)
			}
			if fixture.contents(t) != "old sparrow binary" {
				t.Errorf("executable changed to %q", fixture.contents(t))
			}
		})
	}
}

func TestUpdateRefusesPlainHTTPToARemoteServer(t *testing.T) {
	fixture := newUpdateFixture(t, "v1.0.0", "v1.1.0", nil)
	fixture.env.Getenv = func(name string) string { return map[string]string{"SPARROW_URL": "http://sparrow.example.com"}[name] }
	if err := Run(context.Background(), []string{"update"}, fixture.env); err == nil || !strings.Contains(err.Error(), "https") {
		t.Fatalf("err = %v", err)
	}
}

func TestReplaceExecutableOnWindowsKeepsTheOldFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sparrow.exe")
	if err := os.WriteFile(path, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := replaceExecutable(path, []byte("new"), "windows"); err != nil {
		t.Fatal(err)
	}
	current, _ := os.ReadFile(path)
	old, _ := os.ReadFile(path + ".old")
	if string(current) != "new" || string(old) != "old" {
		t.Fatalf("current = %q, old = %q", current, old)
	}
	// A second update replaces the earlier .old file.
	if err := replaceExecutable(path, []byte("newer"), "windows"); err != nil {
		t.Fatal(err)
	}
}

func TestReleaseKeygenAndSign(t *testing.T) {
	env, stdout, stderr := testEnvironment(t, nil, "")
	if err := Run(context.Background(), []string{"release", "keygen"}, env); err != nil {
		t.Fatal(err)
	}
	privateKey := strings.TrimSpace(stdout.String())
	if strings.Contains(stderr.String(), privateKey) || !strings.Contains(stderr.String(), "CLI_SIGNING_KEY") || !strings.Contains(stderr.String(), "Key ID") {
		t.Fatalf("keygen stderr = %q", stderr.String())
	}
	private, err := release.ParsePrivateKey(privateKey)
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	writeAllBinaries(t, dir)
	env, _, _ = testEnvironment(t, nil, privateKey+"\n")
	if err := Run(context.Background(), []string{"release", "sign", "--key-file", "-", "--version", "v1.2.3", "--dir", dir}, env); err != nil {
		t.Fatal(err)
	}

	manifest, _ := os.ReadFile(filepath.Join(dir, "manifest.json"))
	signature, _ := os.ReadFile(filepath.Join(dir, "manifest.json.sig"))
	parsed, err := release.Verify(manifest, signature, []string{release.EncodeKey(private.Public().(ed25519.PublicKey))})
	if err != nil || parsed.Version != "v1.2.3" {
		t.Fatalf("verify signed release: %v, %+v", err, parsed)
	}

	env, _, _ = testEnvironment(t, nil, "")
	if err := Run(context.Background(), []string{"release", "sign", "--key-file", "-", "--version", "1.2", "--dir", dir}, env); err == nil {
		t.Error("signed with an empty key and a bad version")
	}
}

func TestCredentialFileOverwriteKeepsPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permissions")
	}
	dir := t.TempDir()
	store := credentialStore{keyring: &memoryKeyring{failing: true}, configDir: func() (string, error) { return dir, nil }}
	path := filepath.Join(dir, "sparrow", "credentials.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := store.save(credentials{URL: "https://x.example", Token: "ssc_1"}); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v", info.Mode().Perm())
	}
}

func mustConfigDir(t *testing.T, env *Environment) string {
	t.Helper()
	dir, err := env.ConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

// writeAllBinaries writes a placeholder binary for every release target into dir.
func writeAllBinaries(t *testing.T, dir string) {
	t.Helper()
	for _, target := range release.Targets {
		name := "sparrow-" + target
		if strings.HasPrefix(target, "windows-") {
			name += ".exe"
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte("placeholder "+target), 0o755); err != nil {
			t.Fatal(err)
		}
	}
}
