package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/shantodotdev/super-sparrow/server/internal/checklist"
	"github.com/shantodotdev/super-sparrow/server/internal/store"
)

// tool is one MCP tool: its description for the agent and the function that runs it.
type tool struct {
	name        string
	description string
	schema      map[string]any
	readOnly    bool
	// run takes the server to work through, so a write can run on a store scoped to its
	// transaction without rebuilding the tool list.
	run func(s *Server, ctx context.Context, caller store.Caller, args json.RawMessage) (any, error)
}

func (s *Server) toolList() []tool {
	return append([]tool{
		{
			name: "get_issue_for_branch",
			description: "Find the issue for a git branch (from `git branch --show-current`): the recorded branch, or an issue key in the name, as in feat/wp-142-meta. " +
				"Returns what get_issue returns, including its include options.",
			schema:   object([]string{"branch"}, map[string]any{"branch": text(""), "include": includeOption}),
			readOnly: true,
			run:      bind((*Server).getIssueForBranch),
		},
		{
			name: "get_issue",
			description: "Get an issue by key: title, status, branch, pull request, checklist progress, review rounds, unresolved finding count. " +
				"include adds the plan with checklist items (plan), unresolved findings with discussions (findings), or subissues (subissues) in the same call.",
			schema:   object([]string{"issue_key"}, map[string]any{"issue_key": issueKey, "include": includeOption}),
			readOnly: true,
			run:      bind((*Server).getIssue),
		},
		{
			name: "get_plan",
			description: "Get an issue's plan markdown, plan_revision, and checklist items. Pass an item's index and text to update_plan_checklist. " +
				"checklist_only omits the markdown.",
			schema: object([]string{"issue_key"}, map[string]any{
				"issue_key":      issueKey,
				"checklist_only": boolean("Return only the checklist items and revision."),
			}),
			readOnly: true,
			run:      bind((*Server).getPlan),
		},
		{
			name: "update_plan_checklist",
			description: "Tick or untick several plan checklist items at once, by the index and text get_plan reports. Only those lines change. " +
				"If an item is no longer at its index, nothing changes and the error says where it is. Returns the updated checklist.",
			schema: object([]string{"issue_key", "items"}, map[string]any{
				"issue_key": issueKey,
				"items": array("Items to change.", object([]string{"index", "checked", "text"}, map[string]any{
					"index":   integer("", 0),
					"checked": boolean("true to tick, false to untick."),
					"text":    text("As get_plan shows it."),
				})),
			}),
			run: bind((*Server).updatePlanChecklist),
		},
		{
			name: "start_review_round",
			description: "Start the next review round of an issue. Only one round can be in progress; finish it with finish_review_round. " +
				"To review in a single call, use submit_review.",
			schema: object([]string{"issue_key"}, map[string]any{
				"issue_key":     issueKey,
				"plan_coverage": text("Which parts of the plan this review checked."),
			}),
			run: bind((*Server).startReviewRound),
		},
		{
			name: "add_finding",
			description: "Add one finding (one problem) to the round in progress, or to `round`. Severity: blocking (fix before merging), should_fix, or nit. " +
				"Give file_path and line when the problem is in one place. Returns the label, such as R3. For several findings use add_findings.",
			schema: object([]string{"issue_key", "severity", "title", "problem", "fix"}, findingFields(map[string]any{
				"issue_key": issueKey,
				"round":     integer("Round number; the round in progress if omitted.", 1),
			})),
			run: bind((*Server).addFinding),
		},
		{
			name:        "finish_review_round",
			description: "Finish the round in progress with a summary. A finished round takes no more findings. Returns the round with its finding counts by severity.",
			schema: object([]string{"issue_key", "summary"}, map[string]any{
				"issue_key": issueKey,
				"round":     integer("Round number; the round in progress if omitted.", 1),
				"summary":   text("Overall verdict, in markdown."),
			}),
			run: bind((*Server).finishReviewRound),
		},
		{
			name: "list_open_findings",
			description: "List an issue's unresolved findings across rounds, most severe first, without duplicates; by default open, fix, and fixed. " +
				"Each has its discussion, which may say how to fix it. Work on the findings with status fix.",
			schema: object([]string{"issue_key"}, map[string]any{
				"issue_key":        issueKey,
				"include_comments": boolean("Include each finding's first discussion page; true if omitted."),
				"statuses": array("Only these statuses.",
					enum("", "open", "fix", "wontfix", "later", "fixed", "verified")),
			}),
			readOnly: true,
			run:      bind((*Server).listOpenFindings),
		},
		{
			name: "set_finding_status",
			description: "Mark a person-approved finding fixed, a fixed one verified after checking it, or a fixed one back to fix when the fix failed. " +
				"Only a person decides fix, wontfix, or later. For several fixes use post_fix_summary.",
			schema: object([]string{"issue_key", "round", "finding", "status"}, map[string]any{
				"issue_key": issueKey,
				"round":     integer("", 1),
				"finding":   findingLabel,
				"status":    enum("New status.", "open", "fix", "wontfix", "later", "fixed", "verified"),
				"reason":    text("Why, for wontfix or later."),
			}),
			run: bind((*Server).setFindingStatus),
		},
		{
			name: "mark_duplicate",
			description: "Mark a finding as a duplicate of an earlier finding on the same issue, so the problem is decided and fixed once through the original. " +
				"Duplicates are left out of list_open_findings. clear: true removes the mark.",
			schema: object([]string{"issue_key", "round", "finding"}, map[string]any{
				"issue_key":            issueKey,
				"round":                integer("The duplicate's round.", 1),
				"finding":              findingLabel,
				"duplicate_of_round":   integer("The original's round.", 1),
				"duplicate_of_finding": text("The original's label, such as R1."),
				"clear":                boolean("true to remove the mark instead."),
			}),
			run: bind((*Server).markDuplicate),
		},
		{
			name:        "add_comment",
			description: "Comment on an issue, or on a finding when round and finding are given. parent_comment_id replies to a comment.",
			schema: object([]string{"issue_key", "body"}, map[string]any{
				"issue_key":         issueKey,
				"body":              text(""),
				"round":             integer("Finding's round, to comment on a finding.", 1),
				"finding":           findingLabel,
				"parent_comment_id": text("Comment id to reply to."),
			}),
			run: bind((*Server).addComment),
		},
		{
			name: "list_comments",
			description: "List the discussion on an issue, or on a finding when round and finding are given, oldest first. " +
				"Read a finding's discussion before fixing it. Pass a comment id to add_comment as parent_comment_id to reply.",
			schema: object([]string{"issue_key"}, map[string]any{
				"issue_key": issueKey,
				"round":     integer("Finding's round, to list a finding's comments.", 1),
				"finding":   findingLabel,
			}),
			readOnly: true,
			run:      bind((*Server).listComments),
		},
		{
			name: "post_fix_summary",
			description: "Mark each listed finding fixed and add the summary as an issue comment, atomically. Each finding must be decided fix by a person, " +
				"or nothing is recorded and the error names it.",
			schema: object([]string{"issue_key", "summary", "findings"}, map[string]any{
				"issue_key": issueKey,
				"summary":   text("What you changed and how it fixes each finding, in markdown."),
				"findings": array("Findings fixed.", object([]string{"round", "finding"}, map[string]any{
					"round":   integer("", 1),
					"finding": findingLabel,
				})),
			}),
			run: bind((*Server).postFixSummary),
		},
		{
			name: "create_issue",
			description: "Create an issue, such as a follow-up outside the current issue. The plan is markdown; use `- [ ]` items for a checklist. " +
				"Labels must exist (list_labels, create_label). Returns the issue with its key.",
			schema: object([]string{"project_key", "title", "status"}, map[string]any{
				"project_key":      text(""),
				"title":            text(""),
				"status":           enum("Starting status; backlog for unscheduled work.", issueStatuses...),
				"plan":             text(""),
				"parent_issue_key": text("Parent full issue for a linked follow-up. For a work breakdown use create_subissue."),
				"branch_name":      text(""),
				"labels":           array("Existing label names.", text("")),
			}),
			run: bind((*Server).createIssue),
		},
	}, slices.Concat(s.extraTools(), s.labelTools(), s.attachmentTools(), s.workflowTools())...)
}

// Schemas, kept small so each tool's input reads clearly.

var (
	issueKey      = map[string]any{"type": "string"}
	findingLabel  = text("Label such as R3.")
	issueStatuses = []string{"backlog", "todo", "in_progress", "in_review", "done", "canceled"}
	includeOption = array("Extra sections to return in the same call.", enum("", "plan", "findings", "subissues"))
)

// findingFields adds the fields that describe one finding to a tool's properties.
func findingFields(properties map[string]any) map[string]any {
	properties["severity"] = enum("blocking, should_fix, or nit.", "blocking", "should_fix", "nit")
	properties["title"] = text("")
	properties["problem"] = text("What is wrong and why it matters, in markdown.")
	properties["fix"] = text("How to fix it, in markdown.")
	properties["evidence"] = text("Failing case, log, or code quote.")
	properties["file_path"] = text("File, relative to the repository root.")
	properties["line"] = integer("", 1)
	return properties
}

func object(required []string, properties map[string]any) map[string]any {
	schema := map[string]any{"type": "object", "properties": properties}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

// described adds a description to a property schema unless it is empty.
func described(schema map[string]any, description string) map[string]any {
	if description != "" {
		schema["description"] = description
	}
	return schema
}

func text(description string) map[string]any {
	return described(map[string]any{"type": "string"}, description)
}

func enum(description string, values ...string) map[string]any {
	return described(map[string]any{"type": "string", "enum": values}, description)
}

func integer(description string, minimum int) map[string]any {
	return described(map[string]any{"type": "integer", "minimum": minimum}, description)
}

func boolean(description string) map[string]any {
	return described(map[string]any{"type": "boolean"}, description)
}

func array(description string, items map[string]any) map[string]any {
	return described(map[string]any{"type": "array", "items": items}, description)
}

// bind decodes a tool's arguments into its argument type before calling it.
func bind[T any](fn func(s *Server, ctx context.Context, caller store.Caller, args T) (any, error)) func(*Server, context.Context, store.Caller, json.RawMessage) (any, error) {
	return func(s *Server, ctx context.Context, caller store.Caller, raw json.RawMessage) (any, error) {
		var args T
		if err := decodeArgs(raw, &args); err != nil {
			return nil, err
		}
		return fn(s, ctx, caller, args)
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

// Outputs. They name actors and link to pages, so an agent can report what it did. Empty values
// are omitted, and links appear only where an agent is likely to report them.

type issueOutput struct {
	Key                   string           `json:"key"`
	Title                 string           `json:"title"`
	Status                string           `json:"status"`
	Priority              string           `json:"priority,omitempty"`
	URL                   string           `json:"url"`
	BranchName            string           `json:"branch_name,omitempty"`
	PullRequestURL        string           `json:"pull_request_url,omitempty"`
	Checklist             string           `json:"checklist"`
	Labels                []string         `json:"labels,omitempty"`
	ReviewRounds          []roundOutput    `json:"review_rounds,omitempty"`
	UnresolvedFindings    int              `json:"unresolved_findings"`
	ReviewRoundsTruncated bool             `json:"review_rounds_truncated,omitempty"`
	Plan                  *string          `json:"plan,omitempty"`
	PlanRevision          int              `json:"plan_revision,omitempty"`
	ChecklistItems        []checklistItem  `json:"checklist_items,omitempty"`
	OpenFindings          []findingOutput  `json:"open_findings,omitempty"`
	OpenFindingsTruncated bool             `json:"open_findings_truncated,omitempty"`
	Subissues             []subissueOutput `json:"subissues,omitempty"`
	SubissuesTruncated    bool             `json:"subissues_truncated,omitempty"`
}

// checklistItem is a checklist item as agents see it: checked is left out while false.
type checklistItem struct {
	Index   int    `json:"index"`
	Text    string `json:"text"`
	Checked bool   `json:"checked,omitempty"`
}

func checklistItems(plan string) []checklistItem {
	items := checklist.Items(plan)
	outputs := make([]checklistItem, 0, len(items))
	for _, item := range items {
		outputs = append(outputs, checklistItem{Index: item.Index, Text: item.Text, Checked: item.Checked})
	}
	return outputs
}

type roundOutput struct {
	Round      int            `json:"round"`
	InProgress bool           `json:"in_progress,omitempty"`
	Summary    string         `json:"summary,omitempty"`
	URL        string         `json:"url,omitempty"`
	Findings   map[string]int `json:"findings_by_severity,omitempty"`
}

type findingOutput struct {
	Round              int             `json:"round"`
	Finding            string          `json:"finding"`
	Severity           string          `json:"severity"`
	Status             string          `json:"status"`
	Title              string          `json:"title"`
	Problem            string          `json:"problem,omitempty"`
	Fix                string          `json:"fix,omitempty"`
	Evidence           string          `json:"evidence,omitempty"`
	FilePath           string          `json:"file_path,omitempty"`
	Line               int             `json:"line,omitempty"`
	DecisionReason     string          `json:"decision_reason,omitempty"`
	FoundBy            *actorOutput    `json:"found_by,omitempty"`
	DecidedBy          *actorOutput    `json:"decided_by,omitempty"`
	FixedBy            *actorOutput    `json:"fixed_by,omitempty"`
	VerifiedBy         *actorOutput    `json:"verified_by,omitempty"`
	FollowUpIssue      string          `json:"follow_up_issue,omitempty"`
	DuplicateOf        string          `json:"duplicate_of,omitempty"`
	URL                string          `json:"url,omitempty"`
	Comments           []commentOutput `json:"comments,omitempty"`
	CommentsNextCursor string          `json:"comments_next_cursor,omitempty"`
	CommentsIncluded   bool            `json:"comments_included"`
}

// commentOutput is one comment in a discussion. ReplyTo is the ID of the comment it answers.
// CreatedAt is left out of discussions embedded in findings.
type commentOutput struct {
	ID        string       `json:"id"`
	Author    *actorOutput `json:"author"`
	ReplyTo   string       `json:"reply_to,omitempty"`
	Body      string       `json:"body"`
	CreatedAt string       `json:"created_at,omitempty"`
	Edited    bool         `json:"edited,omitempty"`
}

// findingView chooses how much of each finding findingOutputs includes.
type findingView struct {
	// Comments includes a bounded first page of each finding's discussion.
	Comments bool
	// Detail includes the problem, fix, and evidence text. A write that echoes a finding the
	// agent just sent leaves it out.
	Detail bool
	// Link includes the finding's page URL.
	Link bool
}

func (s *Server) issueURL(issue store.Issue) string {
	return strings.TrimRight(s.publicURL.String(), "/") + "/" + url.PathEscape(issue.ProjectSlug) + "/issues/" + issue.Key()
}

func (s *Server) roundURL(issue store.Issue, round int) string {
	return fmt.Sprintf("%s/reviews/%d", s.issueURL(issue), round)
}

// maxIssueSection bounds the review rounds, findings, and subissues that get_issue includes. A
// longer list sets its truncated flag, and the list tools page through it.
const maxIssueSection = 50

// maxIncludedFindings bounds the findings get_issue includes with their discussions.
const maxIncludedFindings = 20

func (s *Server) newIssueOutput(ctx context.Context, issue store.Issue) (issueOutput, error) {
	return s.issueView(ctx, issue, nil)
}

// issueView describes an issue and adds the sections named by include: plan, findings, or subissues.
func (s *Server) issueView(ctx context.Context, issue store.Issue, include []string) (issueOutput, error) {
	rounds, err := s.store.ListAgentRounds(ctx, issue.ID, maxIssueSection+1, 0)
	if err != nil {
		return issueOutput{}, err
	}
	labels, err := s.labelNameList(ctx, issue.ProjectID, issue.LabelIDs)
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
		Key: issue.Key(), Title: issue.Title, Status: string(issue.Status), Priority: issue.Priority, URL: s.issueURL(issue),
		BranchName: issue.BranchName, PullRequestURL: issue.PullRequestURL,
		Checklist: fmt.Sprintf("%d of %d items done", done, len(items)), Labels: labels,
		UnresolvedFindings: issue.UnresolvedFindings,
	}
	if len(rounds) > maxIssueSection {
		output.ReviewRoundsTruncated = true
		rounds = rounds[:maxIssueSection]
	}
	for _, round := range rounds {
		output.ReviewRounds = append(output.ReviewRounds, s.newRoundOutput(issue, round, nil, false))
	}

	if slices.Contains(include, "plan") {
		plan := issue.Plan
		output.Plan, output.PlanRevision, output.ChecklistItems = &plan, issue.PlanRevision, checklistItems(issue.Plan)
	}
	if slices.Contains(include, "findings") {
		findings, err := s.store.ListAgentFindings(ctx, issue.ID, "", store.UnresolvedFindingStatuses, true, maxIncludedFindings+1, 0)
		if err != nil {
			return issueOutput{}, err
		}
		if len(findings) > maxIncludedFindings {
			output.OpenFindingsTruncated = true
			findings = findings[:maxIncludedFindings]
		}
		if output.OpenFindings, err = s.findingOutputs(ctx, issue, findings, findingView{Comments: true, Detail: true}); err != nil {
			return issueOutput{}, err
		}
	}
	if slices.Contains(include, "subissues") {
		subs, err := s.store.ListAgentSubissues(ctx, issue.ID, maxIssueSection+1, 0)
		if err != nil {
			return issueOutput{}, err
		}
		if len(subs) > maxIssueSection {
			output.SubissuesTruncated = true
			subs = subs[:maxIssueSection]
		}
		for _, sub := range subs {
			subOutput, err := s.subissueOutput(ctx, issue, sub)
			if err != nil {
				return issueOutput{}, err
			}
			output.Subissues = append(output.Subissues, subOutput)
		}
	}
	return output, nil
}

// newRoundOutput describes a round. link adds its page URL; findings, when given, adds the
// finding counts by severity.
func (s *Server) newRoundOutput(issue store.Issue, round store.ReviewRound, findings []store.Finding, link bool) roundOutput {
	output := roundOutput{Round: round.Number, InProgress: round.FinishedAt == nil, Summary: round.Summary}
	if link {
		output.URL = s.roundURL(issue, round.Number)
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

// echoFindingOutputs describes findings an agent just changed: who did what and the new status,
// without the text the agent supplied.
func (s *Server) echoFindingOutputs(ctx context.Context, issue store.Issue, findings []store.Finding) ([]findingOutput, error) {
	return s.findingOutputs(ctx, issue, findings, findingView{Link: true})
}

// findingOutputs describes findings as view directs.
func (s *Server) findingOutputs(ctx context.Context, issue store.Issue, findings []store.Finding, view findingView) ([]findingOutput, error) {
	names := newActorNames(s.store)
	outputs := make([]findingOutput, 0, len(findings))

	for _, f := range findings {
		output := findingOutput{
			Round: f.RoundNumber, Finding: f.Label(), Severity: string(f.Severity), Status: string(f.Status),
			Title: f.Title, FilePath: f.FilePath, Line: f.Line, DecisionReason: f.DecisionReason,
		}
		if view.Detail {
			output.Problem, output.Fix, output.Evidence = f.Problem, f.Fix, f.Evidence
		}
		if view.Link {
			output.URL = s.roundURL(issue, f.RoundNumber) + "?finding=" + f.Label()
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
		if f.DuplicateOfFindingID != "" {
			original, err := s.store.GetFinding(ctx, f.DuplicateOfFindingID)
			if err != nil {
				return nil, err
			}
			output.DuplicateOf = fmt.Sprintf("round %d %s", original.RoundNumber, original.Label())
		}
		if f.FollowUpIssueID != "" {
			followUp, err := s.store.GetIssue(ctx, f.FollowUpIssueID)
			if err != nil {
				return nil, err
			}
			output.FollowUpIssue = followUp.Key()
		}

		output.CommentsIncluded = view.Comments
		if view.Comments {
			comments, err := s.store.ListAgentComments(ctx, "", f.ID, 51, 0)
			if err != nil {
				return nil, err
			}
			_, _, hash, _ := (pageArgs{}).bounds([]string{"comments", issue.ID, f.ID}, pageSizeRows)
			comments, output.CommentsNextCursor = pageSlice(comments, 50, 0, hash)
			if output.Comments, err = newCommentOutputs(ctx, names, comments, false); err != nil {
				return nil, err
			}
		}

		outputs = append(outputs, output)
	}
	return outputs, nil
}

// newCommentOutputs names each comment's author. withTime adds when each was written.
func newCommentOutputs(ctx context.Context, names *actorNames, comments []store.Comment, withTime bool) ([]commentOutput, error) {
	outputs := make([]commentOutput, 0, len(comments))
	for _, comment := range comments {
		author, err := names.name(ctx, comment.AuthorActorID)
		if err != nil {
			return nil, err
		}
		output := commentOutput{ID: comment.ID, Author: author, ReplyTo: comment.ParentCommentID, Body: comment.Body, Edited: comment.EditedAt != nil}
		if withTime {
			output.CreatedAt = comment.CreatedAt.UTC().Format(time.RFC3339)
		}
		outputs = append(outputs, output)
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
	Branch  string   `json:"branch"`
	Include []string `json:"include"`
}) (any, error) {
	issue, err := s.store.FindIssueForBranch(ctx, args.Branch)
	if errors.Is(err, store.ErrNotFound) {
		return nil, store.Refuse(store.ErrNotFound, "no issue matches branch %q; name the branch after the issue key, as in feat/wp-142-short-name", args.Branch)
	}
	if err != nil {
		return nil, err
	}
	return s.issueView(ctx, issue, args.Include)
}

func (s *Server) getIssue(ctx context.Context, _ store.Caller, args struct {
	IssueKey string   `json:"issue_key"`
	Include  []string `json:"include"`
}) (any, error) {
	issue, err := s.issue(ctx, args.IssueKey)
	if err != nil {
		return nil, err
	}
	return s.issueView(ctx, issue, args.Include)
}

type planOutput struct {
	IssueKey  string          `json:"issue_key"`
	Title     string          `json:"title"`
	Plan      *string         `json:"plan,omitempty"`
	Revision  int             `json:"plan_revision"`
	Checklist []checklistItem `json:"checklist"`
}

// newPlanOutput describes an issue's plan; checklistOnly leaves out the markdown.
func newPlanOutput(issue store.Issue, checklistOnly bool) planOutput {
	output := planOutput{IssueKey: issue.Key(), Title: issue.Title, Revision: issue.PlanRevision, Checklist: checklistItems(issue.Plan)}
	if !checklistOnly {
		plan := issue.Plan
		output.Plan = &plan
	}
	return output
}

func (s *Server) getPlan(ctx context.Context, _ store.Caller, args struct {
	IssueKey      string `json:"issue_key"`
	ChecklistOnly bool   `json:"checklist_only"`
}) (any, error) {
	issue, err := s.issue(ctx, args.IssueKey)
	if err != nil {
		return nil, err
	}
	return newPlanOutput(issue, args.ChecklistOnly), nil
}

func (s *Server) updatePlanChecklist(ctx context.Context, caller store.Caller, args struct {
	IssueKey string `json:"issue_key"`
	Items    []struct {
		Index   *int    `json:"index"`
		Checked *bool   `json:"checked"`
		Text    *string `json:"text"`
	} `json:"items"`
}) (any, error) {
	issue, err := s.issue(ctx, args.IssueKey)
	if err != nil {
		return nil, err
	}

	changes := map[int]checklist.Change{}
	for _, item := range args.Items {
		if item.Index == nil || item.Checked == nil || item.Text == nil {
			return nil, &store.ValidationError{Field: "items", Message: "each item needs an index, checked, and its text"}
		}
		if _, exists := changes[*item.Index]; exists {
			return nil, &store.ValidationError{Field: "items", Message: "must not repeat a checklist index"}
		}
		changes[*item.Index] = checklist.Change{Checked: *item.Checked, Text: *item.Text}
	}

	issue, err = s.store.UpdatePlanChecklist(ctx, caller, issue.ID, changes)
	if err != nil {
		return nil, err
	}
	return map[string]any{"issue_key": issue.Key(), "checklist": checklistItems(issue.Plan)}, nil
}

func (s *Server) startReviewRound(ctx context.Context, caller store.Caller, args struct {
	IssueKey     string `json:"issue_key"`
	PlanCoverage string `json:"plan_coverage"`
}) (any, error) {
	issue, err := s.issue(ctx, args.IssueKey)
	if err != nil {
		return nil, err
	}

	round, err := s.store.StartReviewRound(ctx, caller, store.NewReviewRound{
		IssueID: issue.ID, PlanCoverage: args.PlanCoverage,
	})
	if err != nil {
		return nil, err
	}
	return s.newRoundOutput(issue, round, nil, true), nil
}

func (s *Server) addFinding(ctx context.Context, caller store.Caller, args struct {
	IssueKey string `json:"issue_key"`
	Round    *int   `json:"round"`
	Severity string `json:"severity"`
	Title    string `json:"title"`
	Problem  string `json:"problem"`
	Fix      string `json:"fix"`
	Evidence string `json:"evidence"`
	FilePath string `json:"file_path"`
	Line     int    `json:"line"`
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
		Fix: args.Fix, Evidence: args.Evidence, FilePath: args.FilePath, Line: args.Line,
	})
	if err != nil {
		return nil, err
	}

	outputs, err := s.echoFindingOutputs(ctx, issue, []store.Finding{finding})
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
	return s.newRoundOutput(issue, round, findings, true), nil
}

func (s *Server) listOpenFindings(ctx context.Context, _ store.Caller, args struct {
	pageArgs
	IssueKey        string   `json:"issue_key"`
	Statuses        []string `json:"statuses"`
	IncludeComments *bool    `json:"include_comments"`
}) (any, error) {
	issue, err := s.issue(ctx, args.IssueKey)
	if err != nil {
		return nil, err
	}
	limit, offset, hash, err := args.bounds([]any{"open_findings", issue.ID, args.Statuses}, pageSizeDiscussion)
	if err != nil {
		return nil, err
	}
	statuses := store.UnresolvedFindingStatuses
	if len(args.Statuses) > 0 {
		statuses = nil
		for _, status := range args.Statuses {
			statuses = append(statuses, store.FindingStatus(status))
		}
	}
	findings, err := s.store.ListAgentFindings(ctx, issue.ID, "", statuses, true, limit+1, offset)
	if err != nil {
		return nil, err
	}
	findings, cursor := pageSlice(findings, limit, offset, hash)
	outputs, err := s.findingOutputs(ctx, issue, findings, findingView{Comments: args.IncludeComments == nil || *args.IncludeComments, Detail: true})
	if err != nil {
		return nil, err
	}
	return map[string]any{"issue_key": issue.Key(), "findings": outputs, "next_cursor": cursor}, nil
}

func (s *Server) setFindingStatus(ctx context.Context, caller store.Caller, args struct {
	IssueKey string `json:"issue_key"`
	Round    *int   `json:"round"`
	Finding  string `json:"finding"`
	Status   string `json:"status"`
	Reason   string `json:"reason"`
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
		Status: store.FindingStatus(args.Status), Reason: args.Reason,
	})
	if err != nil {
		return nil, err
	}

	outputs, err := s.echoFindingOutputs(ctx, issue, []store.Finding{finding})
	if err != nil {
		return nil, err
	}
	return outputs[0], nil
}

func (s *Server) markDuplicate(ctx context.Context, caller store.Caller, args struct {
	IssueKey           string `json:"issue_key"`
	Round              *int   `json:"round"`
	Finding            string `json:"finding"`
	DuplicateOfRound   *int   `json:"duplicate_of_round"`
	DuplicateOfFinding string `json:"duplicate_of_finding"`
	Clear              bool   `json:"clear"`
}) (any, error) {
	if args.Round == nil {
		return nil, &store.ValidationError{Field: "round", Message: "is required"}
	}
	named := args.DuplicateOfRound != nil || args.DuplicateOfFinding != ""
	switch {
	case args.Clear && named:
		return nil, &store.ValidationError{Field: "clear", Message: "cannot be given with duplicate_of_round and duplicate_of_finding"}
	case !args.Clear && (args.DuplicateOfRound == nil || args.DuplicateOfFinding == ""):
		return nil, &store.ValidationError{Field: "duplicate_of_finding", Message: "and duplicate_of_round are required, or pass clear: true"}
	}

	issue, err := s.issue(ctx, args.IssueKey)
	if err != nil {
		return nil, err
	}
	finding, err := s.finding(ctx, issue, *args.Round, args.Finding)
	if err != nil {
		return nil, err
	}
	originalID := ""
	if !args.Clear {
		original, err := s.finding(ctx, issue, *args.DuplicateOfRound, args.DuplicateOfFinding)
		if err != nil {
			return nil, err
		}
		originalID = original.ID
	}

	if err := s.store.MarkDuplicate(ctx, caller, finding.ID, originalID); err != nil {
		return nil, err
	}
	finding, err = s.store.GetFinding(ctx, finding.ID)
	if err != nil {
		return nil, err
	}
	outputs, err := s.echoFindingOutputs(ctx, issue, []store.Finding{finding})
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
	link := s.issueURL(issue) + "?tab=comments"
	if finding != nil {
		link = s.roundURL(issue, finding.RoundNumber) + "?finding=" + finding.Label()
	}
	return map[string]any{"comment_id": comment.ID, "issue_key": issue.Key(), "url": link}, nil
}

func (s *Server) listComments(ctx context.Context, _ store.Caller, args struct {
	pageArgs
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

	limit, offset, hash, err := args.bounds([]string{"comments", issue.ID, findingID}, pageSizeDiscussion)
	if err != nil {
		return nil, err
	}
	comments, err := s.store.ListAgentComments(ctx, issueID, findingID, limit+1, offset)
	if err != nil {
		return nil, err
	}
	comments, cursor := pageSlice(comments, limit, offset, hash)
	outputs, err := newCommentOutputs(ctx, newActorNames(s.store), comments, true)
	if err != nil {
		return nil, err
	}
	return map[string]any{"about": about, "comments": outputs, "next_cursor": cursor}, nil
}

func (s *Server) postFixSummary(ctx context.Context, caller store.Caller, args struct {
	IssueKey string `json:"issue_key"`
	Summary  string `json:"summary"`
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
		IssueID: issue.ID, Summary: args.Summary, FindingIDs: findingIDs,
	})
	if err != nil {
		return nil, err
	}

	outputs, err := s.echoFindingOutputs(ctx, issue, fixed)
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
	project, err := s.project(ctx, args.ProjectKey)
	if err != nil {
		return nil, err
	}

	input := store.NewIssue{
		ProjectID: project.ID, Title: args.Title, Status: store.IssueStatus(args.Status), Plan: args.Plan,
		BranchName: args.BranchName,
	}
	if args.ParentIssueKey != "" {
		parent, err := s.store.GetIssueByKey(ctx, args.ParentIssueKey)
		_, malformed := errors.AsType[*store.ValidationError](err)
		switch {
		case malformed, errors.Is(err, store.ErrNotFound), err == nil && parent.ProjectID != project.ID:
			return nil, &store.ValidationError{Field: "parent_issue_key", Message: "must be an issue of project " + project.Key}
		case err != nil:
			return nil, err
		}
		input.ParentIssueID = parent.ID
	}
	if input.LabelIDs, err = s.labelIDs(ctx, project.ID, project.Key, "labels", args.Labels); err != nil {
		return nil, err
	}

	issue, err := s.store.CreateIssue(ctx, caller, input)
	if err != nil {
		return nil, err
	}
	return s.newIssueOutput(ctx, issue)
}
