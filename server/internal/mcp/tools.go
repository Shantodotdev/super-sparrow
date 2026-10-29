package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/shantodotdev/super-sparrow/server/internal/checklist"
	"github.com/shantodotdev/super-sparrow/server/internal/store"
)

// tool is one MCP tool: its description for the agent and the function that runs it.
type tool struct {
	name        string
	title       string
	description string
	schema      map[string]any
	readOnly    bool
	run         func(ctx context.Context, caller store.Caller, args json.RawMessage) (any, error)
}

// unresolvedStatuses are the finding statuses that still need work: waiting for a decision,
// decided fix, or fixed and waiting for verification.
var unresolvedStatuses = []store.FindingStatus{store.FindingOpen, store.FindingFix, store.FindingFixed}

func (s *Server) toolList() []tool {
	return []tool{
		{
			name:  "get_issue_for_branch",
			title: "Find the issue for a branch",
			description: "Find the issue a git branch belongs to. Pass the current branch, from `git branch --show-current`. " +
				"It matches the branch recorded on an issue, or an issue key inside the name, as in feat/wp-142-meta-comments. " +
				"Returns the issue's key, status, review rounds, and how many findings are unresolved. Call get_plan next for its plan.",
			schema:   object([]string{"branch"}, map[string]any{"branch": text("The git branch name.")}),
			readOnly: true,
			run:      bind(s.getIssueForBranch),
		},
		{
			name:  "get_issue",
			title: "Get an issue",
			description: "Get an issue by its key, such as WP-142: its title, status, branch, pull request, checklist progress, " +
				"review rounds, and how many findings are unresolved. Call get_plan for the plan itself.",
			schema:   object([]string{"issue_key"}, map[string]any{"issue_key": issueKey}),
			readOnly: true,
			run:      bind(s.getIssue),
		},
		{
			name:  "get_plan",
			title: "Get an issue's plan",
			description: "Get an issue's plan, which is its description in markdown, and its checklist. Each checklist item has " +
				"an index; pass it to update_plan_checklist to tick the item.",
			schema:   object([]string{"issue_key"}, map[string]any{"issue_key": issueKey}),
			readOnly: true,
			run:      bind(s.getPlan),
		},
		{
			name:  "update_plan_checklist",
			title: "Tick plan checklist items",
			description: "Tick or untick items in an issue's plan checklist, by the index get_plan reports. Only those lines " +
				"change, so edits others made to the plan are kept. Returns the updated checklist.",
			schema: object([]string{"issue_key", "items"}, map[string]any{
				"issue_key": issueKey,
				"items": array("The items to change.", object([]string{"index", "checked"}, map[string]any{
					"index":   integer("The item's index from get_plan.", 0),
					"checked": boolean("true to tick the item, false to untick it."),
				})),
			}),
			run: bind(s.updatePlanChecklist),
		},
		{
			name:  "start_review_round",
			title: "Start a review round",
			description: "Start the next review round of an issue before adding findings. Pass the commit you reviewed up to " +
				"(`git rev-parse HEAD`). Leave from_commit out to start where the previous round ended, or at the start of the " +
				"branch for the first round. An issue has one round in progress at a time; finish it with finish_review_round. " +
				"Returns the round's number and its page.",
			schema: object([]string{"issue_key", "to_commit"}, map[string]any{
				"issue_key":            issueKey,
				"to_commit":            text("The last commit the review covers, as a full or short SHA."),
				"from_commit":          text("Where the reviewed range starts, if not where the previous round ended."),
				"includes_uncommitted": boolean("true when the review also covered uncommitted changes."),
				"plan_coverage":        text("Which parts of the plan this review checked, in a sentence or two."),
			}),
			run: bind(s.startReviewRound),
		},
		{
			name:  "add_finding",
			title: "Add a finding",
			description: "Add one finding to the review round in progress. Each finding is one problem with its own status and " +
				"discussion, so add separate findings for separate problems. Severity is blocking (must be fixed before merging), " +
				"should_fix, or nit. Give the file and line when the problem is in one place. Returns the finding's label, such as R3. " +
				"Leave round out to use the round in progress.",
			schema: object([]string{"issue_key", "severity", "title", "problem", "fix"}, map[string]any{
				"issue_key":  issueKey,
				"round":      integer("The round number. Leave it out to use the round in progress.", 1),
				"severity":   enum("How much the problem matters.", "blocking", "should_fix", "nit"),
				"title":      text("A one-line summary of the problem."),
				"problem":    text("What is wrong and why it matters, in markdown."),
				"fix":        text("How to fix it, in markdown."),
				"evidence":   text("What shows the problem: a failing case, a log, or a quote from the code."),
				"file_path":  text("The file the problem is in, relative to the repository root."),
				"line":       integer("The line in file_path.", 1),
				"commit_sha": text("The commit the file and line refer to."),
			}),
			run: bind(s.addFinding),
		},
		{
			name:  "finish_review_round",
			title: "Finish a review round",
			description: "Finish the review round in progress with a summary of the review. A finished round takes no more " +
				"findings. Returns the round with how many findings it has of each severity.",
			schema: object([]string{"issue_key", "summary"}, map[string]any{
				"issue_key": issueKey,
				"round":     integer("The round number. Leave it out to use the round in progress.", 1),
				"summary":   text("The review's overall verdict and summary, in markdown."),
			}),
			run: bind(s.finishReviewRound),
		},
		{
			name:  "list_open_findings",
			title: "List unresolved findings",
			description: "List an issue's unresolved findings across all of its review rounds, most severe first, without " +
				"duplicates. By default it lists findings waiting for a decision (open), decided to be fixed (fix), and fixed but " +
				"not yet verified (fixed). Each finding includes its discussion. When addressing a review, work on the findings with " +
				"status fix, and follow what the discussion says about how to fix them.",
			schema: object([]string{"issue_key"}, map[string]any{
				"issue_key": issueKey,
				"statuses": array("Only list findings with these statuses.",
					enum("A finding status.", "open", "fix", "wontfix", "later", "fixed", "verified")),
			}),
			readOnly: true,
			run:      bind(s.listOpenFindings),
		},
		{
			name:  "set_finding_status",
			title: "Set a finding's status",
			description: "Move a finding to a new status. Agents can mark a finding fixed once a person decided to fix it (give " +
				"fixed_in_commit), mark a fixed finding verified after checking the fix, or move a fixed finding back to fix when " +
				"the fix did not hold. Only a person decides fix, wontfix, or later, so those are refused for agents. To report " +
				"several fixes with an explanation, use post_fix_summary instead.",
			schema: object([]string{"issue_key", "round", "finding", "status"}, map[string]any{
				"issue_key":       issueKey,
				"round":           integer("The finding's round number.", 1),
				"finding":         findingLabel,
				"status":          enum("The new status.", "open", "fix", "wontfix", "later", "fixed", "verified"),
				"fixed_in_commit": text("The commit that fixed it, when the status is fixed."),
				"reason":          text("Why, when a person decides wontfix or later."),
			}),
			run: bind(s.setFindingStatus),
		},
		{
			name:  "add_comment",
			title: "Add a comment",
			description: "Add a markdown comment to an issue, or to one finding when round and finding are given. Pass " +
				"parent_comment_id to reply to a comment.",
			schema: object([]string{"issue_key", "body"}, map[string]any{
				"issue_key":         issueKey,
				"body":              text("The comment, in markdown."),
				"round":             integer("The finding's round number, to comment on a finding.", 1),
				"finding":           findingLabel,
				"parent_comment_id": text("The ID of the comment this replies to."),
			}),
			run: bind(s.addComment),
		},
		{
			name:  "list_comments",
			title: "List comments",
			description: "List the discussion on an issue, or on one finding when round and finding are given, oldest first. " +
				"Read a finding's discussion before fixing it: the developer may say there how they want it fixed. Each comment " +
				"has an id; pass it to add_comment as parent_comment_id to reply.",
			schema: object([]string{"issue_key"}, map[string]any{
				"issue_key": issueKey,
				"round":     integer("The finding's round number, to list a finding's comments.", 1),
				"finding":   findingLabel,
			}),
			readOnly: true,
			run:      bind(s.listComments),
		},
		{
			name:  "post_fix_summary",
			title: "Report a fix",
			description: "Report the fixes you made for a review: it marks each listed finding fixed in the given commit and " +
				"adds the summary as a comment on the issue, all at once. Each finding must have been decided fix by a person; " +
				"if one is not, nothing is recorded and the error names it. Commit your changes first and pass that commit.",
			schema: object([]string{"issue_key", "summary", "commit", "findings"}, map[string]any{
				"issue_key": issueKey,
				"summary":   text("What you changed and how it fixes each finding, in markdown."),
				"commit":    text("The commit with the fixes."),
				"findings": array("The findings this commit fixes.", object([]string{"round", "finding"}, map[string]any{
					"round":   integer("The finding's round number.", 1),
					"finding": findingLabel,
				})),
			}),
			run: bind(s.postFixSummary),
		},
		{
			name:  "create_issue",
			title: "Create an issue",
			description: "Create an issue in a project, such as a follow-up for work outside the current issue. The plan is the " +
				"issue's description in markdown; use `- [ ]` items for its checklist. Use status backlog for work nobody has " +
				"scheduled. Labels must already exist in the project. Returns the new issue's key.",
			schema: object([]string{"project_key", "title", "status"}, map[string]any{
				"project_key":      text("The project's key, such as WP."),
				"title":            text("The issue's title."),
				"status":           enum("Where the issue starts.", "backlog", "todo", "in_progress", "in_review", "done", "canceled"),
				"plan":             text("The plan, in markdown."),
				"parent_issue_key": text("The key of the issue this one belongs under, such as WP-142."),
				"branch_name":      text("The git branch for this issue, if one exists."),
				"labels":           array("Names of existing labels in the project.", text("A label name.")),
			}),
			run: bind(s.createIssue),
		},
	}
}

// Schemas, kept small so each tool's input reads clearly.

var (
	issueKey     = text("The issue's key, such as WP-142.")
	findingLabel = text("The finding's label within its round, such as R3.")
)

func object(required []string, properties map[string]any) map[string]any {
	return map[string]any{"type": "object", "properties": properties, "required": required, "additionalProperties": false}
}

func text(description string) map[string]any {
	return map[string]any{"type": "string", "description": description}
}

func enum(description string, values ...string) map[string]any {
	return map[string]any{"type": "string", "description": description, "enum": values}
}

func integer(description string, minimum int) map[string]any {
	return map[string]any{"type": "integer", "description": description, "minimum": minimum}
}

func boolean(description string) map[string]any {
	return map[string]any{"type": "boolean", "description": description}
}

func array(description string, items map[string]any) map[string]any {
	return map[string]any{"type": "array", "description": description, "items": items}
}

// bind decodes a tool's arguments into its argument type before calling it.
func bind[T any](fn func(ctx context.Context, caller store.Caller, args T) (any, error)) func(context.Context, store.Caller, json.RawMessage) (any, error) {
	return func(ctx context.Context, caller store.Caller, raw json.RawMessage) (any, error) {
		var args T
		if err := decodeArgs(raw, &args); err != nil {
			return nil, err
		}
		return fn(ctx, caller, args)
	}
}

// decodeArgs reads tool arguments strictly, so a misspelled argument is reported rather than
// ignored. Its errors name the argument at fault.
func decodeArgs(raw json.RawMessage, dst any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()

	err := decoder.Decode(dst)
	var typeErr *json.UnmarshalTypeError
	switch {
	case err == nil:
		if decoder.More() {
			return &store.ValidationError{Field: "arguments", Message: "must be one JSON object"}
		}
		return nil
	case errors.Is(err, io.EOF):
		return &store.ValidationError{Field: "arguments", Message: "are required"}
	case errors.As(err, &typeErr):
		return store.TypeMismatch(typeErr.Field, typeErr.Type)
	case strings.HasPrefix(err.Error(), "json: unknown field "):
		field := strings.Trim(strings.TrimPrefix(err.Error(), "json: unknown field "), `"`)
		return &store.ValidationError{Field: field, Message: "is not an argument of this tool"}
	}
	return &store.ValidationError{Field: "arguments", Message: "must be a JSON object"}
}

// Outputs. They name actors and link to pages, so an agent can report what it did.

type issueOutput struct {
	Key                string        `json:"key"`
	Title              string        `json:"title"`
	Status             string        `json:"status"`
	URL                string        `json:"url"`
	BranchName         string        `json:"branch_name,omitempty"`
	PullRequestURL     string        `json:"pull_request_url,omitempty"`
	Checklist          string        `json:"checklist"`
	ReviewRounds       []roundOutput `json:"review_rounds"`
	UnresolvedFindings int           `json:"unresolved_findings"`
}

type roundOutput struct {
	Round               int            `json:"round"`
	InProgress          bool           `json:"in_progress"`
	FromCommit          string         `json:"from_commit,omitempty"`
	ToCommit            string         `json:"to_commit"`
	IncludesUncommitted bool           `json:"includes_uncommitted"`
	Summary             string         `json:"summary,omitempty"`
	URL                 string         `json:"url"`
	Findings            map[string]int `json:"findings_by_severity,omitempty"`
}

type findingOutput struct {
	Round          int             `json:"round"`
	Finding        string          `json:"finding"`
	Severity       string          `json:"severity"`
	Status         string          `json:"status"`
	Title          string          `json:"title"`
	Problem        string          `json:"problem"`
	Fix            string          `json:"fix"`
	Evidence       string          `json:"evidence,omitempty"`
	FilePath       string          `json:"file_path,omitempty"`
	Line           int             `json:"line,omitempty"`
	CommitSHA      string          `json:"commit_sha,omitempty"`
	DecisionReason string          `json:"decision_reason,omitempty"`
	FixedInCommit  string          `json:"fixed_in_commit,omitempty"`
	FoundBy        *actorOutput    `json:"found_by"`
	DecidedBy      *actorOutput    `json:"decided_by,omitempty"`
	FixedBy        *actorOutput    `json:"fixed_by,omitempty"`
	VerifiedBy     *actorOutput    `json:"verified_by,omitempty"`
	FollowUpIssue  string          `json:"follow_up_issue,omitempty"`
	URL            string          `json:"url"`
	Comments       []commentOutput `json:"comments"`
}

// commentOutput is one comment in a discussion. ReplyTo is the ID of the comment it answers.
type commentOutput struct {
	ID        string       `json:"id"`
	Author    *actorOutput `json:"author"`
	ReplyTo   string       `json:"reply_to,omitempty"`
	Body      string       `json:"body"`
	CreatedAt time.Time    `json:"created_at"`
	Edited    bool         `json:"edited,omitempty"`
}

func (s *Server) issueURL(issue store.Issue) string {
	return strings.TrimRight(s.publicURL.String(), "/") + "/" + issue.Key()
}

func (s *Server) roundURL(issue store.Issue, round int) string {
	return fmt.Sprintf("%s/reviews/%d", s.issueURL(issue), round)
}

func (s *Server) newIssueOutput(ctx context.Context, issue store.Issue) (issueOutput, error) {
	rounds, err := s.store.ListReviewRounds(ctx, issue.ID)
	if err != nil {
		return issueOutput{}, err
	}
	unresolved, err := s.store.ListIssueFindings(ctx, issue.ID, unresolvedStatuses, true)
	if err != nil {
		return issueOutput{}, err
	}

	items := checklist.Items(issue.Plan)
	done := 0
	for _, item := range items {
		if item.Checked {
			done++
		}
	}

	output := issueOutput{
		Key: issue.Key(), Title: issue.Title, Status: string(issue.Status), URL: s.issueURL(issue),
		BranchName: issue.BranchName, PullRequestURL: issue.PullRequestURL,
		Checklist:    fmt.Sprintf("%d of %d items done", done, len(items)),
		ReviewRounds: make([]roundOutput, 0, len(rounds)), UnresolvedFindings: len(unresolved),
	}
	for _, round := range rounds {
		output.ReviewRounds = append(output.ReviewRounds, s.newRoundOutput(issue, round, nil))
	}
	return output, nil
}

func (s *Server) newRoundOutput(issue store.Issue, round store.ReviewRound, findings []store.Finding) roundOutput {
	output := roundOutput{
		Round: round.Number, InProgress: round.FinishedAt == nil, FromCommit: round.FromCommit, ToCommit: round.ToCommit,
		IncludesUncommitted: round.IncludesUncommitted, Summary: round.Summary, URL: s.roundURL(issue, round.Number),
	}
	if findings != nil {
		output.Findings = map[string]int{}
		for _, finding := range findings {
			output.Findings[string(finding.Severity)]++
		}
	}
	return output
}

// actorOutput names who did something. Owner is the person an agent works for, and is empty for
// people, so an agent's tool and its person stay two separate facts.
type actorOutput struct {
	Name  string `json:"name"`
	Kind  string `json:"kind"`
	Owner string `json:"owner,omitempty"`
}

// actorNames looks up actors, once per actor per tool call.
type actorNames struct {
	store  *store.Store
	actors map[string]*actorOutput
}

func newActorNames(st *store.Store) *actorNames {
	return &actorNames{store: st, actors: map[string]*actorOutput{}}
}

// name returns the actor with id, or nil when id is empty.
func (n *actorNames) name(ctx context.Context, id string) (*actorOutput, error) {
	if id == "" {
		return nil, nil
	}
	if output, ok := n.actors[id]; ok {
		return output, nil
	}

	actor, err := n.store.GetActor(ctx, id)
	if err != nil {
		return nil, err
	}
	output := &actorOutput{Name: actor.DisplayName, Kind: string(actor.Kind)}
	if actor.OwnerActorID != "" {
		owner, err := n.name(ctx, actor.OwnerActorID)
		if err != nil {
			return nil, err
		}
		output.Owner = owner.Name
	}

	n.actors[id] = output
	return output, nil
}

func (s *Server) newFindingOutputs(ctx context.Context, issue store.Issue, findings []store.Finding) ([]findingOutput, error) {
	names := newActorNames(s.store)
	outputs := make([]findingOutput, 0, len(findings))

	for _, f := range findings {
		output := findingOutput{
			Round: f.RoundNumber, Finding: f.Label(), Severity: string(f.Severity), Status: string(f.Status),
			Title: f.Title, Problem: f.Problem, Fix: f.Fix, Evidence: f.Evidence, FilePath: f.FilePath, Line: f.Line,
			CommitSHA: f.CommitSHA, DecisionReason: f.DecisionReason, FixedInCommit: f.FixedInCommit,
			URL: s.roundURL(issue, f.RoundNumber) + "#" + f.Label(),
		}

		var err error
		for _, field := range []struct {
			target **actorOutput
			id     string
		}{{&output.FoundBy, f.FoundByActorID}, {&output.DecidedBy, f.DecidedByActorID}, {&output.FixedBy, f.FixedByActorID}, {&output.VerifiedBy, f.VerifiedByActorID}} {
			if *field.target, err = names.name(ctx, field.id); err != nil {
				return nil, err
			}
		}
		if f.FollowUpIssueID != "" {
			followUp, err := s.store.GetIssue(ctx, f.FollowUpIssueID)
			if err != nil {
				return nil, err
			}
			output.FollowUpIssue = followUp.Key()
		}

		comments, err := s.store.ListComments(ctx, "", f.ID)
		if err != nil {
			return nil, err
		}
		if output.Comments, err = newCommentOutputs(ctx, names, comments); err != nil {
			return nil, err
		}

		outputs = append(outputs, output)
	}
	return outputs, nil
}

// newCommentOutputs names each comment's author. It always returns a list, so an empty
// discussion reads as [] rather than null.
func newCommentOutputs(ctx context.Context, names *actorNames, comments []store.Comment) ([]commentOutput, error) {
	outputs := make([]commentOutput, 0, len(comments))
	for _, comment := range comments {
		author, err := names.name(ctx, comment.AuthorActorID)
		if err != nil {
			return nil, err
		}
		outputs = append(outputs, commentOutput{
			ID: comment.ID, Author: author, ReplyTo: comment.ParentCommentID, Body: comment.Body,
			CreatedAt: comment.CreatedAt, Edited: comment.EditedAt != nil,
		})
	}
	return outputs, nil
}

// Lookups shared by the tools. Their errors name the tool argument at fault.

func (s *Server) issue(ctx context.Context, key string) (store.Issue, error) {
	if strings.TrimSpace(key) == "" {
		return store.Issue{}, &store.ValidationError{Field: "issue_key", Message: "is required"}
	}

	issue, err := s.store.GetIssueByKey(ctx, key)
	if validation, ok := errors.AsType[*store.ValidationError](err); ok {
		return store.Issue{}, &store.ValidationError{Field: "issue_key", Message: validation.Message}
	}
	if errors.Is(err, store.ErrNotFound) {
		return store.Issue{}, store.Refuse(store.ErrNotFound, "there is no issue %s", strings.ToUpper(key))
	}
	return issue, err
}

// round returns the numbered round, or the round in progress when number is nil.
func (s *Server) round(ctx context.Context, issue store.Issue, number *int) (store.ReviewRound, error) {
	if number == nil {
		round, err := s.store.GetOpenReviewRound(ctx, issue.ID)
		if errors.Is(err, store.ErrNotFound) {
			return store.ReviewRound{}, store.Refuse(store.ErrNotFound, "%s has no review round in progress; start one with start_review_round", issue.Key())
		}
		return round, err
	}

	round, err := s.store.GetReviewRoundByNumber(ctx, issue.ID, *number)
	if errors.Is(err, store.ErrNotFound) {
		return store.ReviewRound{}, store.Refuse(store.ErrNotFound, "%s has no review round %d", issue.Key(), *number)
	}
	return round, err
}

func (s *Server) finding(ctx context.Context, issue store.Issue, round int, label string) (store.Finding, error) {
	number, err := store.ParseFindingLabel("finding", label)
	if err != nil {
		return store.Finding{}, err
	}

	finding, err := s.store.GetFindingByNumber(ctx, issue.ID, round, number)
	if errors.Is(err, store.ErrNotFound) {
		return store.Finding{}, store.Refuse(store.ErrNotFound, "round %d of %s has no finding R%d", round, issue.Key(), number)
	}
	return finding, err
}

// commentedFinding returns the finding a comment tool names with round and finding, or nil when
// it names neither, which means the issue itself. Naming only one of them is an error.
func (s *Server) commentedFinding(ctx context.Context, issue store.Issue, round *int, label string) (*store.Finding, error) {
	switch {
	case label != "" && round == nil:
		return nil, &store.ValidationError{Field: "round", Message: "is required with finding"}
	case label == "" && round != nil:
		return nil, &store.ValidationError{Field: "finding", Message: "is required with round"}
	case label == "":
		return nil, nil
	}

	finding, err := s.finding(ctx, issue, *round, label)
	if err != nil {
		return nil, err
	}
	return &finding, nil
}

// The tools.

func (s *Server) getIssueForBranch(ctx context.Context, _ store.Caller, args struct {
	Branch string `json:"branch"`
}) (any, error) {
	issue, err := s.store.FindIssueForBranch(ctx, args.Branch)
	if errors.Is(err, store.ErrNotFound) {
		return nil, store.Refuse(store.ErrNotFound, "no issue matches branch %q; name the branch after the issue key, as in feat/wp-142-short-name", args.Branch)
	}
	if err != nil {
		return nil, err
	}
	return s.newIssueOutput(ctx, issue)
}

func (s *Server) getIssue(ctx context.Context, _ store.Caller, args struct {
	IssueKey string `json:"issue_key"`
}) (any, error) {
	issue, err := s.issue(ctx, args.IssueKey)
	if err != nil {
		return nil, err
	}
	return s.newIssueOutput(ctx, issue)
}

type planOutput struct {
	IssueKey  string           `json:"issue_key"`
	Title     string           `json:"title"`
	Plan      string           `json:"plan"`
	Checklist []checklist.Item `json:"checklist"`
}

func (s *Server) getPlan(ctx context.Context, _ store.Caller, args struct {
	IssueKey string `json:"issue_key"`
}) (any, error) {
	issue, err := s.issue(ctx, args.IssueKey)
	if err != nil {
		return nil, err
	}
	return planOutput{IssueKey: issue.Key(), Title: issue.Title, Plan: issue.Plan, Checklist: checklist.Items(issue.Plan)}, nil
}

func (s *Server) updatePlanChecklist(ctx context.Context, caller store.Caller, args struct {
	IssueKey string `json:"issue_key"`
	Items    []struct {
		Index   *int  `json:"index"`
		Checked *bool `json:"checked"`
	} `json:"items"`
}) (any, error) {
	issue, err := s.issue(ctx, args.IssueKey)
	if err != nil {
		return nil, err
	}

	changes := map[int]bool{}
	for _, item := range args.Items {
		if item.Index == nil || item.Checked == nil {
			return nil, &store.ValidationError{Field: "items", Message: "each item needs an index and checked"}
		}
		changes[*item.Index] = *item.Checked
	}

	issue, err = s.store.UpdatePlanChecklist(ctx, caller, issue.ID, changes)
	if err != nil {
		return nil, err
	}
	return map[string]any{"issue_key": issue.Key(), "checklist": checklist.Items(issue.Plan)}, nil
}

func (s *Server) startReviewRound(ctx context.Context, caller store.Caller, args struct {
	IssueKey            string `json:"issue_key"`
	ToCommit            string `json:"to_commit"`
	FromCommit          string `json:"from_commit"`
	IncludesUncommitted bool   `json:"includes_uncommitted"`
	PlanCoverage        string `json:"plan_coverage"`
}) (any, error) {
	issue, err := s.issue(ctx, args.IssueKey)
	if err != nil {
		return nil, err
	}

	round, err := s.store.StartReviewRound(ctx, caller, store.NewReviewRound{
		IssueID: issue.ID, FromCommit: args.FromCommit, ToCommit: args.ToCommit,
		IncludesUncommitted: args.IncludesUncommitted, PlanCoverage: args.PlanCoverage,
	})
	if err != nil {
		return nil, err
	}
	return s.newRoundOutput(issue, round, nil), nil
}

func (s *Server) addFinding(ctx context.Context, caller store.Caller, args struct {
	IssueKey  string `json:"issue_key"`
	Round     *int   `json:"round"`
	Severity  string `json:"severity"`
	Title     string `json:"title"`
	Problem   string `json:"problem"`
	Fix       string `json:"fix"`
	Evidence  string `json:"evidence"`
	FilePath  string `json:"file_path"`
	Line      int    `json:"line"`
	CommitSHA string `json:"commit_sha"`
}) (any, error) {
	issue, err := s.issue(ctx, args.IssueKey)
	if err != nil {
		return nil, err
	}
	round, err := s.round(ctx, issue, args.Round)
	if err != nil {
		return nil, err
	}

	finding, err := s.store.AddFinding(ctx, caller, store.NewFinding{
		ReviewRoundID: round.ID, Severity: store.Severity(args.Severity), Title: args.Title, Problem: args.Problem,
		Fix: args.Fix, Evidence: args.Evidence, FilePath: args.FilePath, Line: args.Line, CommitSHA: args.CommitSHA,
	})
	if err != nil {
		return nil, err
	}

	outputs, err := s.newFindingOutputs(ctx, issue, []store.Finding{finding})
	if err != nil {
		return nil, err
	}
	return outputs[0], nil
}

func (s *Server) finishReviewRound(ctx context.Context, caller store.Caller, args struct {
	IssueKey string `json:"issue_key"`
	Round    *int   `json:"round"`
	Summary  string `json:"summary"`
}) (any, error) {
	issue, err := s.issue(ctx, args.IssueKey)
	if err != nil {
		return nil, err
	}
	round, err := s.round(ctx, issue, args.Round)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(args.Summary) == "" {
		return nil, &store.ValidationError{Field: "summary", Message: "is required"}
	}

	round, err = s.store.FinishReviewRound(ctx, caller, round.ID, args.Summary)
	if err != nil {
		return nil, err
	}
	findings, err := s.store.ListRoundFindings(ctx, round.ID)
	if err != nil {
		return nil, err
	}
	return s.newRoundOutput(issue, round, findings), nil
}

func (s *Server) listOpenFindings(ctx context.Context, _ store.Caller, args struct {
	IssueKey string   `json:"issue_key"`
	Statuses []string `json:"statuses"`
}) (any, error) {
	issue, err := s.issue(ctx, args.IssueKey)
	if err != nil {
		return nil, err
	}

	statuses := unresolvedStatuses
	if len(args.Statuses) > 0 {
		statuses = nil
		for _, status := range args.Statuses {
			statuses = append(statuses, store.FindingStatus(status))
		}
	}

	findings, err := s.store.ListIssueFindings(ctx, issue.ID, statuses, true)
	if err != nil {
		return nil, err
	}
	outputs, err := s.newFindingOutputs(ctx, issue, findings)
	if err != nil {
		return nil, err
	}
	return map[string]any{"issue_key": issue.Key(), "findings": outputs}, nil
}

func (s *Server) setFindingStatus(ctx context.Context, caller store.Caller, args struct {
	IssueKey      string `json:"issue_key"`
	Round         *int   `json:"round"`
	Finding       string `json:"finding"`
	Status        string `json:"status"`
	FixedInCommit string `json:"fixed_in_commit"`
	Reason        string `json:"reason"`
}) (any, error) {
	if args.Round == nil {
		return nil, &store.ValidationError{Field: "round", Message: "is required"}
	}
	issue, err := s.issue(ctx, args.IssueKey)
	if err != nil {
		return nil, err
	}
	finding, err := s.finding(ctx, issue, *args.Round, args.Finding)
	if err != nil {
		return nil, err
	}

	finding, err = s.store.ChangeFindingStatus(ctx, caller, finding.ID, store.FindingChange{
		Status: store.FindingStatus(args.Status), Reason: args.Reason, FixedInCommit: args.FixedInCommit,
	})
	if err != nil {
		return nil, err
	}

	outputs, err := s.newFindingOutputs(ctx, issue, []store.Finding{finding})
	if err != nil {
		return nil, err
	}
	return outputs[0], nil
}

func (s *Server) addComment(ctx context.Context, caller store.Caller, args struct {
	IssueKey        string `json:"issue_key"`
	Body            string `json:"body"`
	Round           *int   `json:"round"`
	Finding         string `json:"finding"`
	ParentCommentID string `json:"parent_comment_id"`
}) (any, error) {
	issue, err := s.issue(ctx, args.IssueKey)
	if err != nil {
		return nil, err
	}

	finding, err := s.commentedFinding(ctx, issue, args.Round, args.Finding)
	if err != nil {
		return nil, err
	}
	input := store.NewComment{IssueID: issue.ID, ParentCommentID: args.ParentCommentID, Body: args.Body}
	if finding != nil {
		input.IssueID, input.FindingID = "", finding.ID
	}

	comment, err := s.store.AddComment(ctx, caller, input)
	if err != nil {
		return nil, err
	}
	return map[string]any{"comment_id": comment.ID, "issue_key": issue.Key(), "url": s.issueURL(issue)}, nil
}

func (s *Server) listComments(ctx context.Context, _ store.Caller, args struct {
	IssueKey string `json:"issue_key"`
	Round    *int   `json:"round"`
	Finding  string `json:"finding"`
}) (any, error) {
	issue, err := s.issue(ctx, args.IssueKey)
	if err != nil {
		return nil, err
	}

	finding, err := s.commentedFinding(ctx, issue, args.Round, args.Finding)
	if err != nil {
		return nil, err
	}
	issueID, findingID, about := issue.ID, "", issue.Key()
	if finding != nil {
		issueID, findingID = "", finding.ID
		about = fmt.Sprintf("%s round %d %s", issue.Key(), finding.RoundNumber, finding.Label())
	}

	comments, err := s.store.ListComments(ctx, issueID, findingID)
	if err != nil {
		return nil, err
	}
	outputs, err := newCommentOutputs(ctx, newActorNames(s.store), comments)
	if err != nil {
		return nil, err
	}
	return map[string]any{"about": about, "comments": outputs}, nil
}

func (s *Server) postFixSummary(ctx context.Context, caller store.Caller, args struct {
	IssueKey string `json:"issue_key"`
	Summary  string `json:"summary"`
	Commit   string `json:"commit"`
	Findings []struct {
		Round   int    `json:"round"`
		Finding string `json:"finding"`
	} `json:"findings"`
}) (any, error) {
	issue, err := s.issue(ctx, args.IssueKey)
	if err != nil {
		return nil, err
	}

	findingIDs := make([]string, 0, len(args.Findings))
	for _, ref := range args.Findings {
		finding, err := s.finding(ctx, issue, ref.Round, ref.Finding)
		if err != nil {
			return nil, err
		}
		findingIDs = append(findingIDs, finding.ID)
	}

	comment, fixed, err := s.store.PostFixSummary(ctx, caller, store.FixSummary{
		IssueID: issue.ID, Summary: args.Summary, Commit: args.Commit, FindingIDs: findingIDs,
	})
	if err != nil {
		return nil, err
	}

	outputs, err := s.newFindingOutputs(ctx, issue, fixed)
	if err != nil {
		return nil, err
	}
	return map[string]any{"comment_id": comment.ID, "issue_key": issue.Key(), "fixed": outputs}, nil
}

func (s *Server) createIssue(ctx context.Context, caller store.Caller, args struct {
	ProjectKey     string   `json:"project_key"`
	Title          string   `json:"title"`
	Status         string   `json:"status"`
	Plan           string   `json:"plan"`
	ParentIssueKey string   `json:"parent_issue_key"`
	BranchName     string   `json:"branch_name"`
	Labels         []string `json:"labels"`
}) (any, error) {
	if strings.TrimSpace(args.ProjectKey) == "" {
		return nil, &store.ValidationError{Field: "project_key", Message: "is required"}
	}
	project, err := s.store.GetProjectByKey(ctx, args.ProjectKey)
	if errors.Is(err, store.ErrNotFound) {
		return nil, store.Refuse(store.ErrNotFound, "there is no project %s", strings.ToUpper(args.ProjectKey))
	}
	if err != nil {
		return nil, err
	}

	input := store.NewIssue{
		ProjectID: project.ID, Title: args.Title, Status: store.IssueStatus(args.Status), Plan: args.Plan,
		BranchName: args.BranchName,
	}
	if args.ParentIssueKey != "" {
		parent, err := s.store.GetIssueByKey(ctx, args.ParentIssueKey)
		if err != nil || parent.ProjectID != project.ID {
			return nil, &store.ValidationError{Field: "parent_issue_key", Message: "must be an issue of project " + project.Key}
		}
		input.ParentIssueID = parent.ID
	}
	if len(args.Labels) > 0 {
		labels, err := s.store.ListLabels(ctx, project.ID)
		if err != nil {
			return nil, err
		}
		byName := map[string]string{}
		for _, label := range labels {
			byName[strings.ToLower(label.Name)] = label.ID
		}
		for _, name := range args.Labels {
			id, ok := byName[strings.ToLower(strings.TrimSpace(name))]
			if !ok {
				return nil, &store.ValidationError{Field: "labels", Message: fmt.Sprintf("project %s has no label %q", project.Key, name)}
			}
			input.LabelIDs = append(input.LabelIDs, id)
		}
	}

	issue, err := s.store.CreateIssue(ctx, caller, input)
	if err != nil {
		return nil, err
	}
	return s.newIssueOutput(ctx, issue)
}
