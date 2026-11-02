package mcp

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/shantodotdev/super-sparrow/server/internal/checklist"
	"github.com/shantodotdev/super-sparrow/server/internal/store"
)

func (s *Server) extraTools() []tool {
	status := enum("Issue status.", "backlog", "todo", "in_progress", "in_review", "done", "canceled")
	return []tool{
		{
			name:        "list_projects",
			title:       "List projects",
			description: "Discover project keys, names, slugs, and existing labels before creating or finding issues.",
			schema:      object(nil, map[string]any{}),
			readOnly:    true,
			run:         bind(s.listProjects),
		},
		{
			name:        "list_issues",
			title:       "List project issues",
			description: "Discover issues in a project, newest first. Filter by statuses and search text. Follow next_cursor with the same filters.",
			schema:      object([]string{"project_key"}, map[string]any{"project_key": text("Project key from list_projects."), "statuses": array("Allowed statuses.", status), "search": text("Case-insensitive title or key search.")}),
			readOnly:    true,
			run:         bind(s.listIssues),
		},
		{
			name:        "update_plan",
			title:       "Edit an issue's plan",
			description: "Replace a plan using plan_revision from get_plan. A stale revision is refused; reload and reapply the edit. Checklist ticks have a separate tool.",
			schema:      object([]string{"issue_key", "plan", "plan_revision"}, map[string]any{"issue_key": issueKey, "plan": text("Complete plan markdown. An empty string clears it."), "plan_revision": integer("Starting revision from get_plan.", 1)}),
			run:         bind(s.updatePlan),
		},
		{
			name:        "list_subissues",
			title:       "List subissues",
			description: "List the lightweight subissues of an issue, including descriptions and their revisions.",
			schema:      object([]string{"issue_key"}, map[string]any{"issue_key": issueKey}),
			readOnly:    true,
			run:         bind(s.listSubissues),
		},
		{
			name:        "create_subissue",
			title:       "Create a subissue",
			description: "Create a lightweight task under an issue. It shares its parent's reviews and discussions. Use this rather than create_issue for a work breakdown.",
			schema:      object([]string{"issue_key", "title", "status"}, map[string]any{"issue_key": issueKey, "title": text("Subissue title."), "description": text("Task description in markdown."), "status": status}),
			run:         bind(s.createSubissue),
		},
		{
			name:        "update_subissue",
			title:       "Edit a subissue",
			description: "Update supplied fields of a subissue. Description edits require description_revision from list_subissues. Stale edits are refused.",
			schema:      object([]string{"issue_key", "subissue_key"}, map[string]any{"issue_key": issueKey, "subissue_key": text("Key from list_subissues, such as WP-142.1."), "title": text("New title."), "description": text("Complete description markdown."), "description_revision": integer("Starting description revision.", 1), "status": status}),
			run:         bind(s.updateSubissue),
		},
		{
			name:        "get_review_round",
			title:       "Get a review round",
			description: "Get the review summary, plan coverage, reviewer, timestamps, and a page of findings, including resolved findings and duplicates.",
			schema:      object([]string{"issue_key", "round"}, map[string]any{"issue_key": issueKey, "round": integer("Round number.", 1), "include_comments": boolean("Include the first discussion page for each finding; true when omitted.")}),
			readOnly:    true,
			run:         bind(s.getReviewRound),
		},
		{
			name:        "get_finding",
			title:       "Get a finding",
			description: "Read a finding in any status, including duplicates and resolved findings, with its discussion. Follow comments_next_cursor using list_comments.",
			schema:      object([]string{"issue_key", "round", "finding"}, map[string]any{"issue_key": issueKey, "round": integer("Round number.", 1), "finding": findingLabel, "include_comments": boolean("Include discussion; true when omitted.")}),
			readOnly:    true,
			run:         bind(s.getFinding),
		},
		{
			name:        "list_review_rounds",
			title:       "List review rounds",
			description: "List an issue's review rounds and summaries. Use get_review_round to read the findings and review details.",
			schema:      object([]string{"issue_key"}, map[string]any{"issue_key": issueKey}),
			readOnly:    true,
			run:         bind(s.listReviewRounds),
		},
	}
}

// projectArgs filters issue discovery within a project.
type projectArgs struct {
	pageArgs
	ProjectKey string   `json:"project_key"`
	Statuses   []string `json:"statuses"`
	Search     string   `json:"search"`
}

func (s *Server) listProjects(ctx context.Context, _ store.Caller, args pageArgs) (any, error) {
	limit, offset, hash, err := args.bounds("projects")
	if err != nil {
		return nil, err
	}
	projects, err := s.store.ListAgentProjects(ctx, limit+1, offset)
	if err != nil {
		return nil, err
	}
	projects, cursor := pageSlice(projects, limit, offset, hash)
	outputs := []map[string]any{}
	for _, p := range projects {
		labels, err := s.store.ListLabels(ctx, p.ID)
		if err != nil {
			return nil, err
		}
		labelOutputs := []string{}
		for _, label := range labels {
			labelOutputs = append(labelOutputs, label.Name)
		}
		outputs = append(outputs, map[string]any{"key": p.Key, "name": p.Name, "slug": p.Slug, "labels": labelOutputs, "url": strings.TrimRight(s.publicURL.String(), "/") + "/" + p.Slug + "/issues"})
	}
	return map[string]any{"projects": outputs, "next_cursor": cursor}, nil
}

func (s *Server) listIssues(ctx context.Context, _ store.Caller, args projectArgs) (any, error) {
	limit, offset, hash, err := args.pageArgs.bounds([]any{"issues", args.ProjectKey, args.Statuses, args.Search})
	if err != nil {
		return nil, err
	}
	project, err := s.store.GetProjectByKey(ctx, args.ProjectKey)
	if err != nil {
		return nil, err
	}
	statuses := []store.IssueStatus{}
	for _, status := range args.Statuses {
		statuses = append(statuses, store.IssueStatus(status))
	}
	// Read bounded database pages while applying text search, retaining the source position.
	outputs := []map[string]any{}
	before := offset
	cursor := ""
	for {
		issues, err := s.store.ListIssues(ctx, store.IssueFilter{ProjectID: project.ID, Statuses: statuses, BeforeNumber: before, Limit: 100})
		if err != nil {
			return nil, err
		}
		if len(issues) == 0 {
			break
		}
		for _, issue := range issues {
			if strings.Contains(strings.ToLower(issue.Title+" "+issue.Key()), strings.ToLower(args.Search)) {
				if len(outputs) == limit {
					cursor = nextCursor(hash, before)
					return map[string]any{"issues": outputs, "next_cursor": cursor}, nil
				}
				outputs = append(outputs, map[string]any{"key": issue.Key(), "title": issue.Title, "status": issue.Status, "url": s.issueURL(issue)})
			}
			before = issue.Number
		}
		if len(issues) < 100 {
			break
		}
	}
	return map[string]any{"issues": outputs, "next_cursor": cursor}, nil
}

func (s *Server) updatePlan(ctx context.Context, caller store.Caller, args struct {
	IssueKey string `json:"issue_key"`
	Plan     string `json:"plan"`
	Revision int    `json:"plan_revision"`
}) (any, error) {
	issue, err := s.issue(ctx, args.IssueKey)
	if err != nil {
		return nil, err
	}
	issue, err = s.store.UpdateIssue(ctx, caller, issue.ID, store.IssueUpdate{Plan: store.Change(args.Plan), PlanRevision: args.Revision})
	if err != nil {
		return nil, err
	}
	return planOutput{IssueKey: issue.Key(), Title: issue.Title, Plan: issue.Plan, Revision: issue.PlanRevision, Checklist: checklist.Items(issue.Plan)}, nil
}

// subissueOutput exposes stable keys and the revision needed for safe description edits.
type subissueOutput struct {
	Key         string            `json:"key"`
	Title       string            `json:"title"`
	Description string            `json:"description"`
	Revision    int               `json:"description_revision"`
	Status      store.IssueStatus `json:"status"`
	CreatedBy   *actorOutput      `json:"created_by"`
}

func (s *Server) subissueOutput(ctx context.Context, issue store.Issue, sub store.Subissue) (subissueOutput, error) {
	actor, err := newActorNames(s.store).name(ctx, sub.CreatedByID)
	return subissueOutput{Key: fmt.Sprintf("%s.%d", issue.Key(), sub.Number), Title: sub.Title, Description: sub.Description, Revision: sub.DescriptionRevision, Status: sub.Status, CreatedBy: actor}, err
}

func (s *Server) listSubissues(ctx context.Context, _ store.Caller, args struct {
	pageArgs
	IssueKey string `json:"issue_key"`
}) (any, error) {
	issue, err := s.issue(ctx, args.IssueKey)
	if err != nil {
		return nil, err
	}
	limit, offset, hash, err := args.bounds([]string{"subissues", issue.ID})
	if err != nil {
		return nil, err
	}
	subs, err := s.store.ListAgentSubissues(ctx, issue.ID, limit+1, offset)
	if err != nil {
		return nil, err
	}
	subs, cursor := pageSlice(subs, limit, offset, hash)
	outputs := []subissueOutput{}
	for _, sub := range subs {
		output, err := s.subissueOutput(ctx, issue, sub)
		if err != nil {
			return nil, err
		}
		outputs = append(outputs, output)
	}
	return map[string]any{"issue_key": issue.Key(), "subissues": outputs, "next_cursor": cursor}, nil
}

func (s *Server) createSubissue(ctx context.Context, caller store.Caller, args struct {
	IssueKey    string `json:"issue_key"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Status      string `json:"status"`
}) (any, error) {
	issue, err := s.issue(ctx, args.IssueKey)
	if err != nil {
		return nil, err
	}
	sub, err := s.store.CreateSubissue(ctx, caller, issue.ID, store.NewSubissue{Title: args.Title, Description: args.Description, Status: store.IssueStatus(args.Status)})
	if err != nil {
		return nil, err
	}
	return s.subissueOutput(ctx, issue, sub)
}

func (s *Server) updateSubissue(ctx context.Context, caller store.Caller, args struct {
	IssueKey    string  `json:"issue_key"`
	SubissueKey string  `json:"subissue_key"`
	Title       *string `json:"title"`
	Description *string `json:"description"`
	Revision    int     `json:"description_revision"`
	Status      *string `json:"status"`
}) (any, error) {
	if args.Title == nil && args.Description == nil && args.Status == nil {
		return nil, &store.ValidationError{Field: "fields", Message: "supply title, description, or status"}
	}
	issue, err := s.issue(ctx, args.IssueKey)
	if err != nil {
		return nil, err
	}
	subs, err := s.store.ListSubissues(ctx, issue.ID)
	if err != nil {
		return nil, err
	}
	index := slices.IndexFunc(subs, func(sub store.Subissue) bool {
		return strings.EqualFold(fmt.Sprintf("%s.%d", issue.Key(), sub.Number), args.SubissueKey)
	})
	if index < 0 {
		return nil, store.Refuse(store.ErrNotFound, "there is no subissue %s on %s", args.SubissueKey, issue.Key())
	}
	update := store.SubissueUpdate{DescriptionRevision: args.Revision}
	if args.Title != nil {
		update.Title = store.Change(*args.Title)
	}
	if args.Description != nil {
		update.Description = store.Change(*args.Description)
	}
	if args.Status != nil {
		update.Status = store.Change(store.IssueStatus(*args.Status))
	}
	sub, err := s.store.UpdateSubissue(ctx, caller, issue.ID, subs[index].ID, update)
	if err != nil {
		return nil, err
	}
	return s.subissueOutput(ctx, issue, sub)
}

type reviewArgs struct {
	pageArgs
	IssueKey        string `json:"issue_key"`
	Round           int    `json:"round"`
	IncludeComments *bool  `json:"include_comments"`
}

func (s *Server) getReviewRound(ctx context.Context, _ store.Caller, args reviewArgs) (any, error) {
	issue, err := s.issue(ctx, args.IssueKey)
	if err != nil {
		return nil, err
	}
	round, err := s.round(ctx, issue, &args.Round)
	if err != nil {
		return nil, err
	}
	limit, offset, hash, err := args.bounds([]any{"round", round.ID})
	if err != nil {
		return nil, err
	}
	findings, err := s.store.ListAgentFindings(ctx, issue.ID, round.ID, nil, false, limit+1, offset)
	if err != nil {
		return nil, err
	}
	findings, cursor := pageSlice(findings, limit, offset, hash)
	outputs, err := s.findingOutputs(ctx, issue, findings, args.IncludeComments == nil || *args.IncludeComments)
	if err != nil {
		return nil, err
	}
	reviewer, err := newActorNames(s.store).name(ctx, round.AuthorActorID)
	if err != nil {
		return nil, err
	}
	return map[string]any{"issue_key": issue.Key(), "round": s.newRoundOutput(issue, round, nil), "plan_coverage": round.PlanCoverage, "reviewer": reviewer, "started_at": round.CreatedAt, "finished_at": round.FinishedAt, "findings": outputs, "next_cursor": cursor}, nil
}

func (s *Server) getFinding(ctx context.Context, _ store.Caller, args struct {
	IssueKey        string `json:"issue_key"`
	Round           int    `json:"round"`
	Finding         string `json:"finding"`
	IncludeComments *bool  `json:"include_comments"`
}) (any, error) {
	issue, err := s.issue(ctx, args.IssueKey)
	if err != nil {
		return nil, err
	}
	finding, err := s.finding(ctx, issue, args.Round, args.Finding)
	if err != nil {
		return nil, err
	}
	outputs, err := s.findingOutputs(ctx, issue, []store.Finding{finding}, args.IncludeComments == nil || *args.IncludeComments)
	if err != nil {
		return nil, err
	}
	return outputs[0], nil
}

func (s *Server) listReviewRounds(ctx context.Context, _ store.Caller, args struct {
	pageArgs
	IssueKey string `json:"issue_key"`
}) (any, error) {
	issue, err := s.issue(ctx, args.IssueKey)
	if err != nil {
		return nil, err
	}
	limit, offset, hash, err := args.bounds([]string{"rounds", issue.ID})
	if err != nil {
		return nil, err
	}
	rounds, err := s.store.ListAgentRounds(ctx, issue.ID, limit+1, offset)
	if err != nil {
		return nil, err
	}
	rounds, cursor := pageSlice(rounds, limit, offset, hash)
	outputs := []roundOutput{}
	for _, round := range rounds {
		outputs = append(outputs, s.newRoundOutput(issue, round, nil))
	}
	return map[string]any{"issue_key": issue.Key(), "review_rounds": outputs, "next_cursor": cursor}, nil
}
