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
	"strconv"
	"strings"
)

// Usage lists the client commands.
const Usage = `Client commands (need SPARROW_URL and SPARROW_TOKEN; SPARROW_MODEL is optional):
  issue get KEY                         Show an issue
  issue for-branch [BRANCH]             Find the issue for a branch (default: the current git branch)
  issue create --project KEY --title TEXT --status STATUS [--plan TEXT | --plan-file FILE]
               [--parent KEY] [--branch NAME]
  issue status KEY STATUS               Move an issue to backlog, todo, in_progress, in_review, done, or canceled
  plan get KEY                          Show an issue's plan and checklist
  plan check KEY INDEX... [--uncheck]   Tick checklist items by index
  review start KEY --to SHA [--from SHA] [--uncommitted] [--coverage TEXT]
  review get KEY ROUND                  Show a round and its findings
  review finish KEY ROUND (--summary TEXT | --summary-file FILE)
  finding add KEY ROUND --severity blocking|should_fix|nit --title TEXT --problem TEXT --fix TEXT
              [--evidence TEXT] [--file PATH [--line N]] [--commit SHA]
  finding list KEY [--status open,fix,fixed]
  finding status KEY ROUND FINDING STATUS [--commit SHA] [--reason TEXT]
  comment add KEY (--body TEXT | --body-file FILE) [--finding ROUND/R3] [--reply-to COMMENT_ID]
  fix-summary KEY --commit SHA (--summary TEXT | --summary-file FILE) --findings 1/R2,1/R3

A FILE of - reads standard input. Every command prints the API's JSON response.`

// IsCommand reports whether name is a client command.
func IsCommand(name string) bool {
	switch name {
	case "issue", "plan", "review", "finding", "comment", "fix-summary":
		return true
	}
	return false
}

// Run runs one client command. args starts with the command's name, such as issue.
func Run(ctx context.Context, args []string, getenv func(string) string, stdin io.Reader, stdout io.Writer) error {
	c, err := newClient(getenv)
	if err != nil {
		return err
	}

	r := &runner{client: c, stdin: stdin}
	data, err := r.run(ctx, args)
	if err != nil {
		return err
	}

	var pretty bytes.Buffer
	if err := json.Indent(&pretty, data, "", "  "); err != nil {
		return fmt.Errorf("format response: %w", err)
	}
	pretty.WriteByte('\n')
	_, err = stdout.Write(pretty.Bytes())
	return err
}

type runner struct {
	client *client
	stdin  io.Reader
}

func (r *runner) run(ctx context.Context, args []string) (json.RawMessage, error) {
	if len(args) == 0 {
		return nil, usageError("a command is required")
	}
	if args[0] == "fix-summary" {
		return r.fixSummary(ctx, args[1:])
	}
	if len(args) < 2 {
		return nil, usageError(args[0] + " needs a subcommand")
	}

	command, rest := args[0]+" "+args[1], args[2:]
	switch command {
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

type usageError string

func (e usageError) Error() string { return string(e) + "\n\n" + Usage }

func roundPath(key, round string) string {
	return "/api/issues/" + url.PathEscape(key) + "/reviews/" + url.PathEscape(round)
}

// anyCount tells parse to accept any number of positional arguments.
const anyCount = -1

// parse reads flags that may appear before, between, or after the positional arguments, and
// checks that exactly want positionals were given, unless want is anyCount.
func parse(flags *flag.FlagSet, args []string, want int) ([]string, error) {
	flags.SetOutput(io.Discard)
	var positional []string
	for {
		if err := flags.Parse(args); err != nil {
			return nil, usageError(err.Error())
		}
		args = flags.Args()
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

	items := make([]map[string]any, 0, len(positional)-1)
	for _, value := range positional[1:] {
		index, err := strconv.Atoi(value)
		if err != nil || index < 0 {
			return nil, usageError(fmt.Sprintf("item index %q must be a whole number", value))
		}
		items = append(items, map[string]any{"index": index, "checked": !*uncheck})
	}
	return r.client.do(ctx, http.MethodPatch, "/api/issues/"+url.PathEscape(positional[0])+"/checklist", nil, map[string]any{"items": items})
}

func (r *runner) reviewStart(ctx context.Context, args []string) (json.RawMessage, error) {
	flags := flag.NewFlagSet("review start", flag.ContinueOnError)
	to := flags.String("to", "", "")
	from := flags.String("from", "", "")
	uncommitted := flags.Bool("uncommitted", false, "")
	coverage := flags.String("coverage", "", "")
	positional, err := parse(flags, args, 1)
	if err != nil {
		return nil, err
	}
	if *to == "" {
		return nil, usageError("review start needs --to, the last reviewed commit")
	}

	return r.client.do(ctx, http.MethodPost, "/api/issues/"+url.PathEscape(positional[0])+"/reviews", nil, map[string]any{
		"to_commit": *to, "from_commit": *from, "includes_uncommitted": *uncommitted, "plan_coverage": *coverage,
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
	commit := flags.String("commit", "", "")
	positional, err := parse(flags, args, 2)
	if err != nil {
		return nil, err
	}

	return r.client.do(ctx, http.MethodPost, roundPath(positional[0], positional[1])+"/findings", nil, map[string]any{
		"severity": *severity, "title": *title, "problem": *problem, "fix": *fix, "evidence": *evidence,
		"file_path": *file, "line": *line, "commit_sha": *commit,
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
	commit := flags.String("commit", "", "")
	reason := flags.String("reason", "", "")
	positional, err := parse(flags, args, 4)
	if err != nil {
		return nil, err
	}

	path := roundPath(positional[0], positional[1]) + "/findings/" + url.PathEscape(positional[2]) + "/status"
	return r.client.do(ctx, http.MethodPost, path, nil, map[string]string{"status": positional[3], "fixed_in_commit": *commit, "reason": *reason})
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
	commit := flags.String("commit", "", "")
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
		map[string]any{"summary": text, "commit": *commit, "findings": refs})
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
