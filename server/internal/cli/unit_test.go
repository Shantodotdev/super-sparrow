package cli

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestDetectAgent(t *testing.T) {
	tests := []struct {
		name string
		vars map[string]string
		want string
	}{
		{"person", nil, ""},
		{"claude code", map[string]string{"CLAUDECODE": "1"}, agentClaudeCode},
		{"claude code needs 1", map[string]string{"CLAUDECODE": "0"}, ""},
		{"codex sandbox", map[string]string{"CODEX_SANDBOX": "seatbelt"}, agentCodex},
		{"codex network", map[string]string{"CODEX_SANDBOX_NETWORK_DISABLED": "1"}, agentCodex},
		{"codex thread", map[string]string{"CODEX_THREAD_ID": "abc"}, agentCodex},
	}
	for _, test := range tests {
		got := detectAgent(func(name string) string { return test.vars[name] })
		if got != test.want {
			t.Errorf("%s: detectAgent = %q, want %q", test.name, got, test.want)
		}
	}
}

func TestChooseAgent(t *testing.T) {
	vars := map[string]string{"CLAUDECODE": "1", "SPARROW_AGENT": "codex"}
	getenv := func(name string) string { return vars[name] }

	if name, _, err := chooseAgent("claude-code", getenv); err != nil || name != "claude-code" {
		t.Errorf("flag wins: %q, %v", name, err)
	}
	if name, source, err := chooseAgent("", getenv); err != nil || name != "codex" || source != "SPARROW_AGENT" {
		t.Errorf("env beats detection: %q %q, %v", name, source, err)
	}
	if _, _, err := chooseAgent("gpt", getenv); err == nil {
		t.Error("accepted an unknown agent")
	}
	if _, _, err := chooseAgent("", func(name string) string { return map[string]string{"SPARROW_AGENT": "human"}[name] }); err == nil {
		t.Error("accepted an unknown SPARROW_AGENT")
	}
	if name, _, err := chooseAgent("", func(name string) string { return map[string]string{"CLAUDECODE": "1"}[name] }); err != nil || name != "claude-code" {
		t.Errorf("detection: %q, %v", name, err)
	}
}

func TestParseServerURL(t *testing.T) {
	for _, good := range []string{"https://sparrow.example.com", "http://localhost:8080", "http://127.0.0.1:1", "http://[::1]:8080"} {
		if _, err := parseServerURL(good); err != nil {
			t.Errorf("%s: %v", good, err)
		}
	}
	for _, bad := range []string{"", "sparrow.example.com", "http://sparrow.example.com", "ftp://localhost", "https://user:pw@example.com", "https://example.com?x=1"} {
		if _, err := parseServerURL(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestParseDotEnv(t *testing.T) {
	input := "\ufeff# comment\n\nA=1\nexport B = two words # trailing\nC=\"quoted # not comment\\nnext\" # note\nD='single $x \\n'\nE=\nA=override\r\n"
	got, err := parseDotEnv(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"A": "override", "B": "two words", "C": "quoted # not comment\nnext", "D": `single $x \n`, "E": ""}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestParseDotEnvRejectsMalformedLines(t *testing.T) {
	tests := map[string]string{
		"A=1\nnot a pair\n": "line 2",
		"lower=1":           "line 1",
		"1A=1":              "line 1",
		"A=\"open":          "line 1",
		"A='open":           "line 1",
		"A=\"x\" y":         "line 1",
	}
	for input, want := range tests {
		if _, err := parseDotEnv(strings.NewReader(input)); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: error = %v, want %q", input, err, want)
		}
	}
}

// issueTool is a tool schema that covers every flag type.
const issueTool = `{"name":"update_issue","description":"Change an issue.\nSecond line.","inputSchema":{"type":"object","required":["issue_key"],"properties":{
"issue_key":{"type":"string","description":"The issue key."},
"limit":{"type":"integer"},
"ratio":{"type":"number"},
"archived":{"type":"boolean"},
"labels":{"type":"array","items":{"type":"string"}},
"items":{"type":"array","items":{"type":"object"}},
"meta":{"type":"object"},
"nullable":{"type":["null","string"]}}}}`

func issueSpec(t *testing.T) *toolSpec {
	t.Helper()
	var spec toolSpec
	if err := json.Unmarshal([]byte(issueTool), &spec); err != nil {
		t.Fatal(err)
	}
	return &spec
}

func TestBuildArguments(t *testing.T) {
	_, err := buildArguments(issueSpec(t), []string{"--issue-key", "WP-1", "--starts-with-dashes", "x"})
	if err == nil || !strings.Contains(err.Error(), "unknown flag --starts-with-dashes") || !strings.Contains(err.Error(), "--issue_key") {
		t.Fatalf("unknown flag error does not list flags: %v", err)
	}

	got, err := buildArguments(issueSpec(t), []string{
		"--issue-key", "WP-1", "--limit=5", "--ratio", "1.5", "--archived", "--labels", "a,b", "--labels", "c",
		"--items", `[{"x":1}]`, "--meta={\"k\":true}", "--nullable", "--value",
	})
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(got)
	want := `{"archived":true,"issue_key":"WP-1","items":[{"x":1}],"labels":["a","b","c"],"limit":5,"meta":{"k":true},"nullable":"--value","ratio":1.5}`
	if string(encoded) != want {
		t.Fatalf("arguments = %s\nwant        %s", encoded, want)
	}
}

func TestBuildArgumentsBooleansAndErrors(t *testing.T) {
	spec := issueSpec(t)
	got, err := buildArguments(spec, []string{"--issue_key", "WP-1", "--archived=false"})
	if err != nil || got["archived"] != false {
		t.Fatalf("archived=false: %v, %v", got, err)
	}

	tests := map[string][]string{
		"required missing": {"--limit", "1"},
		"bad integer":      {"--issue_key", "A", "--limit", "x"},
		"bad boolean":      {"--issue_key", "A", "--archived=maybe"},
		"bad json":         {"--issue_key", "A", "--meta", "{"},
		"missing value":    {"--issue_key"},
		"positional":       {"WP-1"},
	}
	for name, args := range tests {
		if _, err := buildArguments(spec, args); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if _, err := buildArguments(spec, []string{"--help"}); !errors.Is(err, errToolHelp) {
		t.Errorf("--help: %v", err)
	}
}

func TestToolHelpMarksRequiredFlags(t *testing.T) {
	help := toolHelp(issueSpec(t))
	for _, want := range []string{"update_issue", "Second line.", "--issue_key <string> (required)", "The issue key.", "--labels <string-list>", "--limit <integer>", "--archived <boolean>"} {
		if !strings.Contains(help, want) {
			t.Errorf("help lacks %q:\n%s", want, help)
		}
	}
	if strings.Contains(help, "--limit <integer> (required)") {
		t.Error("optional flag marked required")
	}
}

func TestRenderText(t *testing.T) {
	input := `{"issue":{"key":"WP-1","title":"Hello","empty":{},"none":[],"nothing":null,"done":false,"points":3,
"labels":["a","b"],"checklist":[{"text":"One","checked":true},{"text":"Two","checked":false}],
"matrix":[["x","y"],[]],"body":"line one\nline two\n"}}`
	got, err := renderText([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Join([]string{
		"issue:",
		"  key: WP-1",
		"  title: Hello",
		"  empty: {}",
		"  none: []",
		"  nothing: null",
		"  done: false",
		"  points: 3",
		"  labels:",
		"    - a",
		"    - b",
		"  checklist:",
		"    - text: One",
		"      checked: true",
		"    - text: Two",
		"      checked: false",
		"  matrix:",
		"    - - x",
		"      - y",
		"    - []",
		"  body:",
		"    line one",
		"    line two",
	}, "\n")
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}

	for _, scalar := range []string{`"text"`, `7`, `null`, `[]`} {
		if _, err := renderText([]byte(scalar)); err != nil {
			t.Errorf("%s: %v", scalar, err)
		}
	}
	if _, err := renderText([]byte(`{`)); err == nil {
		t.Error("accepted broken JSON")
	}
}

func TestParseGlobal(t *testing.T) {
	options, rest, err := parseGlobal([]string{"--json", "--agent", "codex", "whoami"})
	if err != nil || !options.json || options.agent != "codex" || !reflect.DeepEqual(rest, []string{"whoami"}) {
		t.Fatalf("%+v %v %v", options, rest, err)
	}
	if _, _, err := parseGlobal([]string{"--agent"}); err == nil {
		t.Error("accepted --agent without a value")
	}
	if _, _, err := parseGlobal([]string{"--bogus", "x"}); err == nil {
		t.Error("accepted an unknown global flag")
	}
}

func TestMergeEnvironmentLetsSecretsWin(t *testing.T) {
	got := mergeEnvironment([]string{"A=old", "B=keep"}, map[string]string{"A": "new", "C": "x=y"})
	want := []string{"B=keep", "A=new", "C=x=y"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}
