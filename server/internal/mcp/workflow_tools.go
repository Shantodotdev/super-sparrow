package mcp

import (
	"context"
	"strings"

	"github.com/shantodotdev/super-sparrow/server/internal/store"
)

// maxBatchFindings bounds the findings one add_findings or submit_review call adds.
const maxBatchFindings = 50

// workflowTools do in one call what agents otherwise do in several: move an issue, find the
// caller's own work, and record a whole review.
func (s *Server) workflowTools() []tool {
	findingList := array("The findings, in order.", object([]string{"severity", "title", "problem", "fix"}, findingFields(map[string]any{})))
	return []tool{
		{
			name: "update_issue",
			description: "Change an issue's status, title, priority, branch, or pull request URL; supply only what changes. " +
				"Move work forward, such as in_progress when starting and in_review when the code is ready. Returns the issue.",
			schema: object([]string{"issue_key"}, map[string]any{
				"issue_key":        issueKey,
				"status":           enum("New status.", issueStatuses...),
				"title":            text(""),
				"priority":         enum("New priority; none clears it.", "urgent", "high", "medium", "low", "none"),
				"branch_name":      text("Git branch; empty clears it."),
				"pull_request_url": text("Pull request URL; empty clears it."),
			}),
			run: bind((*Server).updateIssue),
		},
		{
			name: "list_my_issues",
			description: "List issues assigned to the person whose token you use (or to you), across projects, most recently updated first. " +
				"Without statuses it lists todo, in_progress, and in_review.",
			schema: object(nil, map[string]any{
				"project_key": text("Only this project."),
				"statuses":    array("Only these statuses.", enum("", issueStatuses...)),
			}),
			readOnly: true,
			run:      bind((*Server).listMyIssues),
		},
		{
			name:        "add_findings",
			description: "Add up to 50 findings to the round in progress (or `round`) atomically: if one is invalid, none is recorded. Returns their labels in order.",
			schema: object([]string{"issue_key", "findings"}, map[string]any{
				"issue_key": issueKey,
				"round":     integer("Round number; the round in progress if omitted.", 1),
				"findings":  findingList,
			}),
			run: bind((*Server).addFindings),
		},
		{
			name: "submit_review",
			description: "Review in one atomic call: start the next round, add the findings (none for a clean review), and finish it with the summary. " +
				"Nothing is recorded if a round is already in progress or anything is invalid. Returns the round with finding counts.",
			schema: object([]string{"issue_key", "summary"}, map[string]any{
				"issue_key":     issueKey,
				"plan_coverage": text("Which parts of the plan this review checked."),
				"summary":       text("Overall verdict, in markdown."),
				"findings":      findingList,
			}),
			run: bind((*Server).submitReview),
		},
	}
}

// updateIssue changes the status, title, priority, branch, or pull request of an issue. It cannot
// assign people, set milestones, or edit the plan.
func (s *Server) updateIssue(ctx context.Context, caller store.Caller, args struct {
	IssueKey       string  `json:"issue_key"`
	Status         *string `json:"status"`
	Title          *string `json:"title"`
	Priority       *string `json:"priority"`
	BranchName     *string `json:"branch_name"`
	PullRequestURL *string `json:"pull_request_url"`
}) (any, error) {
	if args.Status == nil && args.Title == nil && args.Priority == nil && args.BranchName == nil && args.PullRequestURL == nil {
		return nil, &store.ValidationError{Field: "fields", Message: "supply status, title, priority, branch_name, or pull_request_url"}
	}
	issue, err := s.issue(ctx, args.IssueKey)
	if err != nil {
		return nil, err
	}

	update := store.IssueUpdate{}
	if args.Status != nil {
		update.Status = store.Change(store.IssueStatus(*args.Status))
	}
	if args.Title != nil {
		update.Title = store.Change(*args.Title)
	}
	if args.Priority != nil {
		priority := *args.Priority
		if priority == "none" {
			priority = ""
		}
		update.Priority = store.Change(priority)
	}
	if args.BranchName != nil {
		update.BranchName = store.Change(*args.BranchName)
	}
	if args.PullRequestURL != nil {
		update.PullRequestURL = store.Change(*args.PullRequestURL)
	}

	issue, err = s.store.UpdateIssue(ctx, caller, issue.ID, update)
	if err != nil {
		return nil, err
	}
	return s.newIssueOutput(ctx, issue)
}

// listMyIssues pages through open issues assigned to the caller's agent or to the person who owns
// it, across projects or in one, most recently changed first.
func (s *Server) listMyIssues(ctx context.Context, caller store.Caller, args struct {
	pageArgs
	ProjectKey string   `json:"project_key"`
	Statuses   []string `json:"statuses"`
}) (any, error) {
	limit, offset, hash, err := args.bounds([]any{"my_issues", caller.ActorID, args.ProjectKey, args.Statuses}, pageSizeRows)
	if err != nil {
		return nil, err
	}

	// An agent's work is assigned to the person it works for, or to the agent itself.
	actor, err := s.store.GetActor(ctx, caller.ActorID)
	if err != nil {
		return nil, err
	}
	assignees := []string{actor.ID}
	if actor.OwnerActorID != "" {
		assignees = append(assignees, actor.OwnerActorID)
	}

	projectID := ""
	if strings.TrimSpace(args.ProjectKey) != "" {
		project, err := s.project(ctx, args.ProjectKey)
		if err != nil {
			return nil, err
		}
		projectID = project.ID
	}
	statuses := []store.IssueStatus{store.IssueTodo, store.IssueInProgress, store.IssueInReview}
	if len(args.Statuses) > 0 {
		statuses = nil
		for _, status := range args.Statuses {
			statuses = append(statuses, store.IssueStatus(status))
		}
	}

	issues, err := s.store.ListAssignedIssues(ctx, assignees, projectID, statuses, limit+1, offset)
	if err != nil {
		return nil, err
	}
	issues, cursor := pageSlice(issues, limit, offset, hash)
	rows := make([]map[string]any, 0, len(issues))
	for _, issue := range issues {
		rows = append(rows, issueRow(issue))
	}
	return map[string]any{"issues": rows, "next_cursor": cursor}, nil
}

// findingArgs is one finding in add_findings or submit_review.
type findingArgs struct {
	Severity string `json:"severity"`
	Title    string `json:"title"`
	Problem  string `json:"problem"`
	Fix      string `json:"fix"`
	Evidence string `json:"evidence"`
	FilePath string `json:"file_path"`
	Line     int    `json:"line"`
}

// addFindingRows adds the findings to the round, in order. Called inside a tool transaction, so a
// failure on any finding undoes the ones before it.
func (s *Server) addFindingRows(ctx context.Context, caller store.Caller, round store.ReviewRound, rows []findingArgs) ([]store.Finding, error) {
	if len(rows) > maxBatchFindings {
		return nil, &store.ValidationError{Field: "findings", Message: "must have at most 50 findings"}
	}

	added := make([]store.Finding, 0, len(rows))
	for _, row := range rows {
		finding, err := s.store.AddFinding(ctx, caller, store.NewFinding{
			ReviewRoundID: round.ID, Severity: store.Severity(row.Severity), Title: row.Title, Problem: row.Problem,
			Fix: row.Fix, Evidence: row.Evidence, FilePath: row.FilePath, Line: row.Line,
		})
		if err != nil {
			return nil, err
		}
		added = append(added, finding)
	}
	return added, nil
}

func findingLabels(findings []store.Finding) []string {
	labels := make([]string, 0, len(findings))
	for _, finding := range findings {
		labels = append(labels, finding.Label())
	}
	return labels
}

// addFindings adds several findings to a round in one transaction: one invalid finding adds none.
func (s *Server) addFindings(ctx context.Context, caller store.Caller, args struct {
	IssueKey string        `json:"issue_key"`
	Round    *int          `json:"round"`
	Findings []findingArgs `json:"findings"`
}) (any, error) {
	if len(args.Findings) == 0 {
		return nil, &store.ValidationError{Field: "findings", Message: "must have at least one finding"}
	}
	issue, err := s.issue(ctx, args.IssueKey)
	if err != nil {
		return nil, err
	}
	round, err := s.round(ctx, issue, args.Round)
	if err != nil {
		return nil, err
	}

	added, err := s.addFindingRows(ctx, caller, round, args.Findings)
	if err != nil {
		return nil, err
	}
	foundBy, err := newActorNames(s.store).name(ctx, caller.ActorID)
	if err != nil {
		return nil, err
	}
	return map[string]any{"issue_key": issue.Key(), "round": round.Number, "findings": findingLabels(added), "found_by": foundBy}, nil
}

// submitReview starts a round, adds its findings, and finishes it with a summary, all in one
// transaction, so a failed review leaves no round behind.
func (s *Server) submitReview(ctx context.Context, caller store.Caller, args struct {
	IssueKey     string        `json:"issue_key"`
	PlanCoverage string        `json:"plan_coverage"`
	Summary      string        `json:"summary"`
	Findings     []findingArgs `json:"findings"`
}) (any, error) {
	if strings.TrimSpace(args.Summary) == "" {
		return nil, &store.ValidationError{Field: "summary", Message: "is required"}
	}
	issue, err := s.issue(ctx, args.IssueKey)
	if err != nil {
		return nil, err
	}

	round, err := s.store.StartReviewRound(ctx, caller, store.NewReviewRound{IssueID: issue.ID, PlanCoverage: args.PlanCoverage})
	if err != nil {
		return nil, err
	}
	added, err := s.addFindingRows(ctx, caller, round, args.Findings)
	if err != nil {
		return nil, err
	}
	round, err = s.store.FinishReviewRound(ctx, caller, round.ID, args.Summary)
	if err != nil {
		return nil, err
	}

	return map[string]any{"issue_key": issue.Key(), "round": s.newRoundOutput(issue, round, added, true), "findings": findingLabels(added)}, nil
}
