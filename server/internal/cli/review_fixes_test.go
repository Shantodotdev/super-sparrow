package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
)

func TestEmptyStringListIsAnEmptyArray(t *testing.T) {
	got, err := buildArguments(issueSpec(t), []string{"--issue-key", "WP-1", "--labels", ""})
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(got["labels"])
	if string(encoded) != "[]" {
		t.Fatalf("labels = %s, want []", encoded)
	}
}

func TestBuildArgumentsBooleanValuesAndStrictFlags(t *testing.T) {
	spec := issueSpec(t)
	for args, want := range map[string]bool{"--archived false": false, "--archived true": true, "--archived": true} {
		got, err := buildArguments(spec, append([]string{"--issue_key", "A"}, strings.Fields(args)...))
		if err != nil || got["archived"] != want {
			t.Errorf("%s: %v, %v", args, got["archived"], err)
		}
	}

	// A word after a boolean flag that is not true or false is not its value.
	if _, err := buildArguments(spec, []string{"--archived", "WP-1", "--issue_key", "A"}); err == nil {
		t.Error("treated a stray word as a boolean value")
	}

	rejected := map[string][]string{
		"repeated string":  {"--issue_key", "A", "--issue_key", "B"},
		"repeated integer": {"--issue_key", "A", "--limit", "1", "--limit=2"},
		"single dash":      {"-issue_key", "A"},
		"triple dash":      {"---issue_key", "A"},
	}
	for name, args := range rejected {
		if _, err := buildArguments(spec, args); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if _, err := buildArguments(spec, []string{"-h"}); !errors.Is(err, errToolHelp) {
		t.Errorf("-h: %v", err)
	}
}

func TestParseTreatsDoubleDashAsTheEndOfFlags(t *testing.T) {
	flags := flagSetWithProject()
	positional, err := parse(flags, []string{"--project", "WP", "a", "--", "--project", "-x"}, anyCount)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"a", "--project", "-x"}; !reflect.DeepEqual(positional, want) {
		t.Fatalf("positional = %v, want %v", positional, want)
	}

	// A -- that is a flag's value is not a terminator.
	flags = flagSetWithProject()
	positional, err = parse(flags, []string{"--project", "--", "a", "--project", "B"}, anyCount)
	if err != nil || len(positional) != 1 || positional[0] != "a" {
		t.Fatalf("value --: %v, %v", positional, err)
	}
}

func TestSavingToTheFileRemovesAStaleKeychainEntry(t *testing.T) {
	keyring := &memoryKeyring{}
	dir := t.TempDir()
	store := credentialStore{keyring: keyring, configDir: func() (string, error) { return dir, nil }}
	if _, err := store.save(credentials{URL: "https://old", Token: "ssc_old"}); err != nil {
		t.Fatal(err)
	}

	// The keychain now rejects writes but still holds the old entry and still deletes it.
	failingSet := &setFailingKeyring{memoryKeyring: keyring}
	store.keyring = failingSet
	if _, err := store.save(credentials{URL: "https://new", Token: "ssc_new"}); err != nil {
		t.Fatal(err)
	}
	if _, err := keyring.Get(keyringService, keyringUser); !errors.Is(err, errNoEntry) {
		t.Fatalf("stale keychain entry survived: %v", err)
	}

	store.keyring = keyring
	loaded, err := store.load()
	if err != nil || loaded.Token != "ssc_new" {
		t.Fatalf("load = %+v, %v", loaded, err)
	}
}

// setFailingKeyring wraps a keyring whose writes fail, like a locked keychain.
type setFailingKeyring struct{ *memoryKeyring }

func (k *setFailingKeyring) Set(string, string, string) error { return errKeychainDown }

func TestLoadReportsKeychainFailuresInsteadOfNotSignedIn(t *testing.T) {
	dir := t.TempDir()
	store := credentialStore{keyring: &memoryKeyring{failing: true}, configDir: func() (string, error) { return dir, nil }}

	_, err := store.load()
	if err == nil || errors.Is(err, errNotSignedIn) || !errors.Is(err, errKeychainDown) {
		t.Fatalf("load with a failing keychain and no file = %v", err)
	}

	store.keyring = &memoryKeyring{}
	if _, err := store.load(); !errors.Is(err, errNotSignedIn) {
		t.Fatalf("load with an empty keychain = %v", err)
	}

	// With a file, a failing keychain still lets the file login through.
	store.keyring = &memoryKeyring{failing: true}
	if _, err := store.save(credentials{URL: "https://x", Token: "ssc_x"}); err != nil {
		t.Fatal(err)
	}
	if loaded, err := store.load(); err != nil || loaded.Token != "ssc_x" {
		t.Fatalf("load from file = %+v, %v", loaded, err)
	}
}

func TestHelpWorksWithoutAConnection(t *testing.T) {
	for _, args := range [][]string{{"help"}, {"--help"}, {"-h"}, {"--json", "help"}} {
		env, stdout, _ := testEnvironment(t, nil, "")
		if err := Run(context.Background(), args, env); err != nil || !strings.Contains(stdout.String(), "Client commands:") {
			t.Errorf("%v: %v, %q", args, err, stdout.String())
		}
	}
}

func TestUnknownCommandWhenNotSignedInSaysSo(t *testing.T) {
	env, _, _ := testEnvironment(t, nil, "")
	err := Run(context.Background(), []string{"bogus-command"}, env)
	if err == nil || !strings.Contains(err.Error(), "unknown command") || !strings.Contains(err.Error(), "not signed in") {
		t.Fatalf("error = %v", err)
	}
}

func TestUsageErrorsPrintAShortHintAndExitWithStatus2(t *testing.T) {
	env, _, stderr := testEnvironment(t, nil, "")
	err := Run(context.Background(), []string{"login", "extra"}, env)

	var exit *ExitError
	if !errors.As(err, &exit) || exit.Code != 2 {
		t.Fatalf("error = %v", err)
	}
	if !strings.Contains(stderr.String(), "sparrow help") || strings.Contains(stderr.String(), "Client commands:") {
		t.Errorf("stderr = %q", stderr.String())
	}
}

func TestLoginRetriesAWrongCodeWithoutAnotherEmail(t *testing.T) {
	var codes, verifies atomic.Int32
	server := newFakeServer(t, map[string]http.HandlerFunc{
		"POST /api/auth/code": func(w http.ResponseWriter, r *http.Request) { codes.Add(1); writeJSON(w, 202, `{}`) },
		"POST /api/auth/cli/verify": func(w http.ResponseWriter, r *http.Request) {
			if verifies.Add(1) < 3 {
				writeJSON(w, 400, `{"error":"that code is wrong","field":"code"}`)
				return
			}
			writeJSON(w, 200, `{"token":"ssc_abc","actor":{"display_name":"Olive","email":"o@example.com"}}`)
		},
	})

	env, _, stderr := testEnvironment(t, nil, "111111\n222222\n333333\n")
	if err := Run(context.Background(), []string{"login", "--url", server.URL, "--email", "a@b.c"}, env); err != nil {
		t.Fatal(err)
	}
	if codes.Load() != 1 || verifies.Load() != 3 {
		t.Errorf("code requests = %d, verifies = %d", codes.Load(), verifies.Load())
	}
	if !strings.Contains(stderr.String(), "Signing in to "+server.URL) {
		t.Errorf("server address not shown: %q", stderr.String())
	}

	// A fourth wrong code is never tried.
	verifies.Store(-10)
	env, _, _ = testEnvironment(t, nil, "111111\n222222\n333333\n444444\n")
	if err := Run(context.Background(), []string{"login", "--url", server.URL, "--email", "a@b.c"}, env); err == nil {
		t.Fatal("login succeeded with only wrong codes")
	}
	if verifies.Load() != -7 {
		t.Errorf("verify attempts = %d, want 3", verifies.Load()+10)
	}
}

func TestLoginTruncatesTheDeviceLabelAndRevokesTheReplacedSession(t *testing.T) {
	var tokens atomic.Int32
	server := newFakeServer(t, map[string]http.HandlerFunc{
		"POST /api/auth/code": func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 202, `{}`) },
		"POST /api/auth/cli/verify": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, 200, fmt.Sprintf(`{"token":"ssc_%d","actor":{"display_name":"Olive","email":"o@example.com"}}`, tokens.Add(1)))
		},
		"POST /api/auth/sign-out": func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) },
	})

	env, _, _ := testEnvironment(t, nil, "123456\n")
	env.Hostname = func() (string, error) { return strings.Repeat("h", 300), nil }
	login := []string{"login", "--url", server.URL, "--email", "a@b.c"}
	if err := Run(context.Background(), login, env); err != nil {
		t.Fatal(err)
	}

	var body map[string]string
	if err := json.Unmarshal([]byte(server.last(t, "POST", "/api/auth/cli/verify").Body), &body); err != nil {
		t.Fatal(err)
	}
	if got := len([]rune(body["device_label"])); got != 100 {
		t.Errorf("device label has %d characters, want 100", got)
	}
	for _, request := range server.requests {
		if request.Path == "/api/auth/sign-out" {
			t.Fatal("signed out although nothing was replaced")
		}
	}

	if err := Run(context.Background(), login, shared(env, nil, "123456\n")); err != nil {
		t.Fatal(err)
	}
	if request := server.last(t, "POST", "/api/auth/sign-out"); request.Authorization != "Bearer ssc_1" {
		t.Errorf("revoked %q, want the first session", request.Authorization)
	}
	if stored, err := env.store().load(); err != nil || stored.Token != "ssc_2" {
		t.Errorf("stored = %+v, %v", stored, err)
	}
}

func TestWhoamiJSONNamesServerAuthAndAgent(t *testing.T) {
	server := authServer(t)
	env, _, _ := testEnvironment(t, nil, "123456\n")
	if err := Run(context.Background(), []string{"login", "--url", server.URL, "--email", "a@b.c"}, env); err != nil {
		t.Fatal(err)
	}

	decode := func(run *Environment, args ...string) map[string]any {
		t.Helper()
		if err := Run(context.Background(), append([]string{"--json"}, args...), run); err != nil {
			t.Fatal(err)
		}
		var fields map[string]any
		if err := json.Unmarshal([]byte(output(run)), &fields); err != nil {
			t.Fatal(err)
		}
		return fields
	}

	person := decode(shared(env, nil, ""), "whoami")
	if person["auth"] != "login" || person["server"] != server.URL || person["agent"] != nil || person["actor"] == nil {
		t.Errorf("person = %v", person)
	}
	agent := decode(shared(env, map[string]string{"CODEX_THREAD_ID": "1"}, ""), "whoami")
	if agent["agent"] != "codex" {
		t.Errorf("agent = %v", agent)
	}
	token := decode(shared(env, map[string]string{"SPARROW_URL": server.URL, "SPARROW_TOKEN": "ssp_z"}, ""), "whoami")
	if token["auth"] != "token" {
		t.Errorf("token = %v", token)
	}
}

func TestExplicitAgentIsIgnoredWithAnAgentTokenAndWarns(t *testing.T) {
	server := authServer(t)
	vars := map[string]string{"SPARROW_URL": server.URL, "SPARROW_TOKEN": "ssp_z", "SPARROW_AGENT": "codex"}
	env, _, stderr := testEnvironment(t, vars, "")
	if err := Run(context.Background(), []string{"whoami"}, env); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stderr.String(), "SPARROW_AGENT is ignored") {
		t.Errorf("stderr = %q", stderr.String())
	}

	// A detected agent is not something the person asked for, so no warning.
	vars = map[string]string{"SPARROW_URL": server.URL, "SPARROW_TOKEN": "ssp_z", "CLAUDECODE": "1"}
	env, _, stderr = testEnvironment(t, vars, "")
	if err := Run(context.Background(), []string{"whoami"}, env); err != nil || stderr.Len() != 0 {
		t.Errorf("detected agent: %v, %q", err, stderr.String())
	}
}

func TestFetchRefusesARedirectFromHTTPSToHTTP(t *testing.T) {
	https, err := http.NewRequest(http.MethodGet, "https://sparrow.example.com/cli/x", nil)
	if err != nil {
		t.Fatal(err)
	}
	downgrade, _ := http.NewRequest(http.MethodGet, "http://sparrow.example.com/cli/x", nil)
	upgrade, _ := http.NewRequest(http.MethodGet, "https://other.example.com/cli/x", nil)

	if err := refuseInsecureRedirect(downgrade, []*http.Request{https}); err == nil {
		t.Error("allowed https to http")
	}
	if err := refuseInsecureRedirect(upgrade, []*http.Request{https}); err != nil {
		t.Errorf("refused https to https: %v", err)
	}
	if err := refuseInsecureRedirect(upgrade, make([]*http.Request, 10)); err == nil {
		t.Error("allowed more than ten redirects")
	}
}

func TestRemoveOldExecutableOnlyOnWindows(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sparrow.exe")
	if err := os.WriteFile(path+".old", []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}

	removeOldExecutable(path, "linux")
	if _, err := os.Stat(path + ".old"); err != nil {
		t.Fatalf("removed the file on linux: %v", err)
	}
	removeOldExecutable(path, "windows")
	if _, err := os.Stat(path + ".old"); !os.IsNotExist(err) {
		t.Fatalf("left the old file: %v", err)
	}
}

func flagSetWithProject() *flag.FlagSet {
	flags := flag.NewFlagSet("test", flag.ContinueOnError)
	flags.String("project", "", "")
	return flags
}
