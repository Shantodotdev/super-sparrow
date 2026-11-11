package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
)

// Usage lists the client commands.
const Usage = `Client commands:
  setup [--agents claude-code,codex] [--url URL]
                                        Sign in if needed and connect Claude Code and Codex
  login [--url URL] [--no-open]         Sign in by approving it in the desktop app
  login --email EMAIL [--url URL]       Sign in with an emailed code instead
  mcp                                   Serve the agent tools over stdio for an agent; setup
                                        registers it as: sparrow --agent claude-code mcp
  logout                                Revoke this computer's session and forget the login
  whoami                                Show who the CLI acts as
  tools                                 List every agent tool with its description
  TOOL-NAME [--flag value ...]          Run any agent tool, such as get-issue --key WP-1;
                                        TOOL-NAME --help lists its flags
  tool list                             Discover all agent tools as JSON
  tool call NAME (--arguments JSON | --arguments-file FILE)
                                        Run any MCP tool, including retries and pagination
  issue get KEY                         Show an issue
  issue for-branch [BRANCH]             Find the issue for a branch (default: the current git branch)
  issue create --project KEY --title TEXT --status STATUS [--plan TEXT | --plan-file FILE]
               [--parent KEY] [--branch NAME]
  issue status KEY STATUS               Move an issue to backlog, todo, in_progress, in_review, done, or canceled
  plan get KEY                          Show an issue's plan and checklist
  plan check KEY INDEX... [--uncheck]   Tick checklist items by index
  review start KEY [--coverage TEXT]
  review get KEY ROUND                  Show a round and its findings
  review finish KEY ROUND (--summary TEXT | --summary-file FILE)
  finding add KEY ROUND --severity blocking|should_fix|nit --title TEXT --problem TEXT --fix TEXT
              [--evidence TEXT] [--file PATH [--line N]]
  finding list KEY [--status open,fix,fixed]
  finding status KEY ROUND FINDING STATUS [--reason TEXT]
  comment add KEY (--body TEXT | --body-file FILE) [--finding ROUND/R3] [--reply-to COMMENT_ID]
  fix-summary KEY (--summary TEXT | --summary-file FILE) --findings 1/R2,1/R3
  update                                Replace this program with the latest signed release
  release keygen                        Make a release signing key pair
  release sign --key-file FILE --version vX.Y.Z --dir DIR
                                        Sign the release binaries in DIR

Global flags go before the command: --json prints raw JSON instead of text, and
--agent claude-code|codex (or SPARROW_AGENT) acts as that agent of the signed-in person; the CLI
detects Claude Code and Codex on its own. A cloud session can set SPARROW_URL and SPARROW_TOKEN
(an ssp_ agent token) instead of signing in; SPARROW_MODEL is optional. A FILE of - reads standard
input.`

var toolNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)

// IsCommand reports whether name could be a client command. Agent tool names are not known until
// the server is asked, so any lowercase word qualifies and Run reports the ones that match nothing.
func IsCommand(name string) bool { return toolNamePattern.MatchString(name) }

// ExitError reports the status the program exits with. For a child process it is the child's
// status; for a usage mistake it is 2, and Err holds the message, which Run has already printed.
type ExitError struct {
	Code int
	Err  error
}

func (e *ExitError) Error() string {
	if e.Err != nil {
		return e.Err.Error()
	}
	return fmt.Sprintf("command exited with status %d", e.Code)
}

func (e *ExitError) Unwrap() error { return e.Err }

// errHelp means a command was given --help or -h, which prints the usage.
var errHelp = errors.New("help requested")

// usageExitCode is the exit status of a command line that could not be understood.
const usageExitCode = 2

// builtinCommands are the hand-written commands that talk to the server, as opposed to agent
// tools whose names only the server knows.
var builtinCommands = map[string]bool{
	"whoami": true, "mcp": true, "tools": true, "fix-summary": true,
	"issue": true, "plan": true, "review": true, "finding": true, "comment": true, "tool": true,
}

// globalOptions are the flags that go before the command.
type globalOptions struct {
	json  bool
	help  bool
	agent string
}

// parseGlobal reads the leading global flags and returns the rest of the arguments.
func parseGlobal(args []string) (globalOptions, []string, error) {
	var options globalOptions
	for len(args) > 0 && strings.HasPrefix(args[0], "-") {
		name, value, hasValue := strings.Cut(strings.TrimLeft(args[0], "-"), "=")
		args = args[1:]

		switch name {
		case "json":
			options.json = true
			if hasValue && value != "true" {
				options.json = false
			}
		case "help", "h":
			options.help = true
		case "agent":
			if !hasValue {
				if len(args) == 0 {
					return options, nil, usageError("--agent needs a value: claude-code or codex")
				}
				value, args = args[0], args[1:]
			}
			if value == "" {
				return options, nil, usageError("--agent needs a value: claude-code or codex")
			}
			options.agent = value
		default:
			return options, nil, usageError("unknown global flag --" + name)
		}
	}
	return options, args, nil
}

// Run runs one client command. args starts with the command's name, such as issue. A command
// line that cannot be understood prints the problem and a pointer to `sparrow help` on standard
// error and returns an ExitError with status 2.
func Run(ctx context.Context, args []string, env *Environment) error {
	err := dispatch(ctx, args, env)
	if errors.Is(err, errHelp) {
		_, err = fmt.Fprintln(env.Stdout, Usage)
		return err
	}

	var usage usageError
	if errors.As(err, &usage) {
		fmt.Fprintln(env.Stderr, err)
		return &ExitError{Code: usageExitCode, Err: err}
	}
	return err
}

// dispatch parses the global flags and runs the command.
func dispatch(ctx context.Context, args []string, env *Environment) error {
	options, args, err := parseGlobal(args)
	if err != nil {
		return err
	}
	if options.help || (len(args) > 0 && args[0] == "help") {
		_, err := fmt.Fprintln(env.Stdout, Usage)
		return err
	}
	if len(args) == 0 {
		return usageError("a command is required")
	}

	if env.Executable != nil {
		if executable, err := env.Executable(); err == nil {
			removeOldExecutable(executable, runtime.GOOS)
		}
	}

	r := &runner{env: env, stdin: env.Stdin, out: env.Stdout, errOut: env.Stderr, asJSON: options.json}
	switch args[0] {
	case "login":
		return r.login(ctx, args[1:])
	case "logout":
		return r.logout(ctx, args[1:])
	case "update":
		return r.update(ctx, args[1:])
	case "release":
		return r.release(args[1:])
	case "setup":
		return r.setup(ctx, args[1:])
	}

	// Built-in commands answer --help without signing in; tools list their own flags.
	if builtinCommands[args[0]] && len(args) > 1 && (args[1] == "--help" || args[1] == "-h") {
		return errHelp
	}

	r.conn, err = env.connect(options.agent)
	if errors.Is(err, errNotSignedIn) && !builtinCommands[args[0]] {
		return fmt.Errorf("unknown command %q, or you are not signed in (agent tool names are known only after signing in): %w", args[0], err)
	}
	if err != nil {
		return err
	}
	r.client = r.conn.client

	data, err := r.run(ctx, args)
	if err != nil {
		return err
	}
	if data == nil {
		return nil
	}
	return r.print(data)
}

// print writes a JSON response: indented JSON with --json, readable text otherwise.
func (r *runner) print(data json.RawMessage) error {
	if !r.asJSON {
		text, err := renderText(data)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(r.out, text)
		return err
	}

	var pretty bytes.Buffer
	if err := json.Indent(&pretty, data, "", "  "); err != nil {
		return fmt.Errorf("format response: %w", err)
	}
	pretty.WriteByte('\n')
	_, err := r.out.Write(pretty.Bytes())
	return err
}

type runner struct {
	env    *Environment
	conn   *connection
	client *client
	stdin  io.Reader
	out    io.Writer
	errOut io.Writer
	asJSON bool
}

// run runs a command that needs the server. A nil result means the command printed its own output.
func (r *runner) run(ctx context.Context, args []string) (json.RawMessage, error) {
	switch args[0] {
	case "whoami":
		return r.whoami(ctx, args[1:])
	case "mcp":
		return nil, r.serveMCP(ctx, args[1:])
	case "tools":
		return r.listTools(ctx, args[1:])
	case "fix-summary":
		return r.fixSummary(ctx, args[1:])
	case "issue", "plan", "review", "finding", "comment", "tool":
		return r.runGroup(ctx, args)
	}
	return r.callTool(ctx, args[0], args[1:])
}

// runGroup runs the hand-written commands, which take a subcommand.
func (r *runner) runGroup(ctx context.Context, args []string) (json.RawMessage, error) {
	if len(args) < 2 {
		return nil, usageError(args[0] + " needs a subcommand")
	}

	command, rest := args[0]+" "+args[1], args[2:]
	switch command {
	case "tool list":
		if len(rest) != 0 {
			return nil, usageError("tool list takes no arguments")
		}
		return r.agentTool(ctx, "tools/list", nil)
	case "tool call":
		return r.callAgentTool(ctx, rest)
	case "issue get":
		return r.get(ctx, rest, 1, func(p []string) string { return "/api/issues/" + url.PathEscape(p[0]) })
	case "issue for-branch":
		return r.issueForBranch(ctx, rest)
	case "issue create":
		return r.issueCreate(ctx, rest)
	case "issue status":
		return r.issueStatus(ctx, rest)
	case "plan get":
		return r.get(ctx, rest, 1, func(p []string) string { return "/api/issues/" + url.PathEscape(p[0]) })
	case "plan check":
		return r.planCheck(ctx, rest)
	case "review start":
		return r.reviewStart(ctx, rest)
	case "review get":
		return r.get(ctx, rest, 2, func(p []string) string { return roundPath(p[0], p[1]) })
	case "review finish":
		return r.reviewFinish(ctx, rest)
	case "finding add":
		return r.findingAdd(ctx, rest)
	case "finding list":
		return r.findingList(ctx, rest)
	case "finding status":
		return r.findingStatus(ctx, rest)
	case "comment add":
		return r.commentAdd(ctx, rest)
	}
	return nil, usageError("unknown command " + command)
}

// usageError is a command line that could not be understood. Its text is a short hint, not the
// whole usage.
type usageError string

func (e usageError) Error() string { return string(e) + "\nRun `sparrow help` for the commands." }

func roundPath(key, round string) string {
	return "/api/issues/" + url.PathEscape(key) + "/reviews/" + url.PathEscape(round)
}

// anyCount tells parse to accept any number of positional arguments.
const anyCount = -1

// parse reads flags that may appear before, between, or after the positional arguments, and
// checks that exactly want positionals were given, unless want is anyCount. A -- ends the flags:
// everything after it is positional.
func parse(flags *flag.FlagSet, args []string, want int) ([]string, error) {
	flags.SetOutput(io.Discard)
	var positional []string
	for {
		if err := flags.Parse(args); errors.Is(err, flag.ErrHelp) {
			return nil, errHelp
		} else if err != nil {
			return nil, usageError(err.Error())
		}

		rest := flags.Args()
		if endedByDoubleDash(flags, args, len(args)-len(rest)) {
			positional = append(positional, rest...)
			break
		}
		args = rest
		if len(args) == 0 {
			break
		}
		positional, args = append(positional, args[0]), args[1:]
	}

	if want != anyCount && len(positional) != want {
		return nil, usageError(fmt.Sprintf("%s takes %d argument(s), got %d", flags.Name(), want, len(positional)))
	}
	return positional, nil
}

// endedByDoubleDash reports whether flag parsing stopped because it consumed a -- terminator,
// as opposed to a -- that is the value of the flag before it. consumed is how many of args the
// parser used.
func endedByDoubleDash(flags *flag.FlagSet, args []string, consumed int) bool {
	if consumed == 0 || args[consumed-1] != "--" {
		return false
	}
	if consumed >= 2 && !strings.Contains(args[consumed-2], "=") && strings.HasPrefix(args[consumed-2], "-") {
		if f := flags.Lookup(strings.TrimLeft(args[consumed-2], "-")); f != nil {
			boolean, ok := f.Value.(interface{ IsBoolFlag() bool })
			return ok && boolean.IsBoolFlag()
		}
	}
	return true
}

// text returns inline text, or the contents of file when it is set, where - is standard input.
func (r *runner) text(name, inline, file string, required bool) (string, error) {
	switch {
	case inline != "" && file != "":
		return "", usageError(fmt.Sprintf("give --%s or --%s-file, not both", name, name))
	case file == "-":
		data, err := io.ReadAll(r.stdin)
		return string(data), err
	case file != "":
		data, err := os.ReadFile(file)
		if err != nil {
			return "", fmt.Errorf("read --%s-file: %w", name, err)
		}
		return string(data), nil
	case inline == "" && required:
		return "", usageError(fmt.Sprintf("--%s or --%s-file is required", name, name))
	}
	return inline, nil
}

func (r *runner) get(ctx context.Context, args []string, want int, path func([]string) string) (json.RawMessage, error) {
	positional, err := parse(flag.NewFlagSet("this command", flag.ContinueOnError), args, want)
	if err != nil {
		return nil, err
	}
	return r.client.do(ctx, http.MethodGet, path(positional), nil, nil)
}

func (r *runner) issueForBranch(ctx context.Context, args []string) (json.RawMessage, error) {
	positional, err := parse(flag.NewFlagSet("issue for-branch", flag.ContinueOnError), args, anyCount)
	if err != nil {
		return nil, err
	}

	var branch string
	switch len(positional) {
	case 0:
		output, err := exec.CommandContext(ctx, "git", "branch", "--show-current").Output()
		if err != nil {
			return nil, fmt.Errorf("find the current git branch: %w", err)
		}
		if branch = strings.TrimSpace(string(output)); branch == "" {
			return nil, errors.New("git is not on a branch; pass the branch name")
		}
	case 1:
		branch = positional[0]
	default:
		return nil, usageError("issue for-branch takes at most one branch")
	}

	return r.client.do(ctx, http.MethodGet, "/api/issues/for-branch", url.Values{"branch": {branch}}, nil)
}

func (r *runner) issueCreate(ctx context.Context, args []string) (json.RawMessage, error) {
	flags := flag.NewFlagSet("issue create", flag.ContinueOnError)
	project := flags.String("project", "", "")
	title := flags.String("title", "", "")
	status := flags.String("status", "", "")
	plan := flags.String("plan", "", "")
	planFile := flags.String("plan-file", "", "")
	parent := flags.String("parent", "", "")
	branch := flags.String("branch", "", "")
	if _, err := parse(flags, args, 0); err != nil {
		return nil, err
	}
	if *project == "" || *title == "" || *status == "" {
		return nil, usageError("issue create needs --project, --title, and --status")
	}

	planText, err := r.text("plan", *plan, *planFile, false)
	if err != nil {
		return nil, err
	}
	body := map[string]any{"title": *title, "status": *status, "plan": planText, "branch_name": *branch}
	if *parent != "" {
		if body["parent_issue_id"], err = r.client.issueID(ctx, *parent); err != nil {
			return nil, err
		}
	}
	return r.client.do(ctx, http.MethodPost, "/api/projects/"+url.PathEscape(*project)+"/issues", nil, body)
}

func (r *runner) issueStatus(ctx context.Context, args []string) (json.RawMessage, error) {
	positional, err := parse(flag.NewFlagSet("issue status", flag.ContinueOnError), args, 2)
	if err != nil {
		return nil, err
	}
	return r.client.do(ctx, http.MethodPatch, "/api/issues/"+url.PathEscape(positional[0]), nil, map[string]string{"status": positional[1]})
}

func (r *runner) planCheck(ctx context.Context, args []string) (json.RawMessage, error) {
	flags := flag.NewFlagSet("plan check", flag.ContinueOnError)
	uncheck := flags.Bool("uncheck", false, "")
	positional, err := parse(flags, args, anyCount)
	if err != nil {
		return nil, err
	}
	if len(positional) < 2 {
		return nil, usageError("plan check needs an issue key and at least one item index")
	}

	issuePath := "/api/issues/" + url.PathEscape(positional[0])

	// The API ticks an item only when it names the item's text, so read the checklist the indexes
	// refer to. A command line has no earlier read to compare with, so this is the plan as it is now.
	raw, err := r.client.do(ctx, http.MethodGet, issuePath, nil, nil)
	if err != nil {
		return nil, err
	}
	var current struct {
		Issue struct {
			Checklist []struct {
				Text string `json:"text"`
			} `json:"checklist"`
		} `json:"issue"`
	}
	if err := json.Unmarshal(raw, &current); err != nil {
		return nil, fmt.Errorf("read the checklist: %w", err)
	}

	items := make([]map[string]any, 0, len(positional)-1)
	for _, value := range positional[1:] {
		index, err := strconv.Atoi(value)
		if err != nil || index < 0 {
			return nil, usageError(fmt.Sprintf("item index %q must be a whole number", value))
		}
		if index >= len(current.Issue.Checklist) {
			return nil, usageError(fmt.Sprintf("%s has no checklist item %d; it has %d", positional[0], index, len(current.Issue.Checklist)))
		}
		items = append(items, map[string]any{"index": index, "checked": !*uncheck, "text": current.Issue.Checklist[index].Text})
	}
	return r.client.do(ctx, http.MethodPatch, issuePath+"/checklist", nil, map[string]any{"items": items})
}

func (r *runner) reviewStart(ctx context.Context, args []string) (json.RawMessage, error) {
	flags := flag.NewFlagSet("review start", flag.ContinueOnError)
	coverage := flags.String("coverage", "", "")
	positional, err := parse(flags, args, 1)
	if err != nil {
		return nil, err
	}

	return r.client.do(ctx, http.MethodPost, "/api/issues/"+url.PathEscape(positional[0])+"/reviews", nil, map[string]any{
		"plan_coverage": *coverage,
	})
}

func (r *runner) reviewFinish(ctx context.Context, args []string) (json.RawMessage, error) {
	flags := flag.NewFlagSet("review finish", flag.ContinueOnError)
	summary := flags.String("summary", "", "")
	summaryFile := flags.String("summary-file", "", "")
	positional, err := parse(flags, args, 2)
	if err != nil {
		return nil, err
	}

	text, err := r.text("summary", *summary, *summaryFile, true)
	if err != nil {
		return nil, err
	}
	return r.client.do(ctx, http.MethodPost, roundPath(positional[0], positional[1])+"/finish", nil, map[string]string{"summary": text})
}

func (r *runner) findingAdd(ctx context.Context, args []string) (json.RawMessage, error) {
	flags := flag.NewFlagSet("finding add", flag.ContinueOnError)
	severity := flags.String("severity", "", "")
	title := flags.String("title", "", "")
	problem := flags.String("problem", "", "")
	fix := flags.String("fix", "", "")
	evidence := flags.String("evidence", "", "")
	file := flags.String("file", "", "")
	line := flags.Int("line", 0, "")
	positional, err := parse(flags, args, 2)
	if err != nil {
		return nil, err
	}

	return r.client.do(ctx, http.MethodPost, roundPath(positional[0], positional[1])+"/findings", nil, map[string]any{
		"severity": *severity, "title": *title, "problem": *problem, "fix": *fix, "evidence": *evidence,
		"file_path": *file, "line": *line,
	})
}

func (r *runner) findingList(ctx context.Context, args []string) (json.RawMessage, error) {
	flags := flag.NewFlagSet("finding list", flag.ContinueOnError)
	status := flags.String("status", "open,fix,fixed", "")
	positional, err := parse(flags, args, 1)
	if err != nil {
		return nil, err
	}
	return r.client.do(ctx, http.MethodGet, "/api/issues/"+url.PathEscape(positional[0])+"/findings", url.Values{"status": {*status}}, nil)
}

func (r *runner) findingStatus(ctx context.Context, args []string) (json.RawMessage, error) {
	flags := flag.NewFlagSet("finding status", flag.ContinueOnError)
	reason := flags.String("reason", "", "")
	positional, err := parse(flags, args, 4)
	if err != nil {
		return nil, err
	}

	path := roundPath(positional[0], positional[1]) + "/findings/" + url.PathEscape(positional[2]) + "/status"
	return r.client.do(ctx, http.MethodPost, path, nil, map[string]string{"status": positional[3], "reason": *reason})
}

func (r *runner) commentAdd(ctx context.Context, args []string) (json.RawMessage, error) {
	flags := flag.NewFlagSet("comment add", flag.ContinueOnError)
	body := flags.String("body", "", "")
	bodyFile := flags.String("body-file", "", "")
	finding := flags.String("finding", "", "")
	replyTo := flags.String("reply-to", "", "")
	positional, err := parse(flags, args, 1)
	if err != nil {
		return nil, err
	}

	text, err := r.text("body", *body, *bodyFile, true)
	if err != nil {
		return nil, err
	}
	path := "/api/issues/" + url.PathEscape(positional[0]) + "/comments"
	if *finding != "" {
		round, label, err := splitFindingRef(*finding)
		if err != nil {
			return nil, err
		}
		path = roundPath(positional[0], strconv.Itoa(round)) + "/findings/" + url.PathEscape(label) + "/comments"
	}
	return r.client.do(ctx, http.MethodPost, path, nil, map[string]string{"body": text, "parent_comment_id": *replyTo})
}

func (r *runner) fixSummary(ctx context.Context, args []string) (json.RawMessage, error) {
	flags := flag.NewFlagSet("fix-summary", flag.ContinueOnError)
	summary := flags.String("summary", "", "")
	summaryFile := flags.String("summary-file", "", "")
	findings := flags.String("findings", "", "")
	positional, err := parse(flags, args, 1)
	if err != nil {
		return nil, err
	}

	text, err := r.text("summary", *summary, *summaryFile, true)
	if err != nil {
		return nil, err
	}
	var refs []map[string]any
	for value := range strings.SplitSeq(*findings, ",") {
		if value = strings.TrimSpace(value); value == "" {
			continue
		}
		round, label, err := splitFindingRef(value)
		if err != nil {
			return nil, err
		}
		refs = append(refs, map[string]any{"round": round, "finding": label})
	}
	if len(refs) == 0 {
		return nil, usageError("fix-summary needs --findings, such as 1/R2,1/R3")
	}

	return r.client.do(ctx, http.MethodPost, "/api/issues/"+url.PathEscape(positional[0])+"/fix-summary", nil,
		map[string]any{"summary": text, "findings": refs})
}

// splitFindingRef reads a finding reference such as 2/R3: round 2, finding R3.
func splitFindingRef(value string) (int, string, error) {
	roundText, label, ok := strings.Cut(value, "/")
	round, err := strconv.Atoi(roundText)
	if !ok || err != nil || round < 1 || label == "" {
		return 0, "", usageError(fmt.Sprintf("finding %q must look like 2/R3: the round, a slash, and the finding", value))
	}
	return round, label, nil
}
