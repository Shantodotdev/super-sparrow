package mcp

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/shantodotdev/super-sparrow/server/internal/store"
)

func (s *Server) extraTools() []tool {
	status := enum("", issueStatuses...)
	return []tool{
		{
			name:        "list_projects",
			description: "List project keys, names, slugs, label names, and milestones with calculated statuses.",
			schema:      object(nil, map[string]any{}),
			readOnly:    true,
			run:         bind((*Server).listProjects),
		},
		{
			name:        "list_issues",
			description: "List a project's issues, newest first, as key, title, and status. Filter by statuses and search text. get_issue returns an issue's details and links.",
			schema:      object([]string{"project_key"}, map[string]any{"project_key": text(""), "statuses": array("Only these statuses.", status), "search": text("Case-insensitive title or key search.")}),
			readOnly:    true,
			run:         bind((*Server).listIssues),
		},
		{
			name:        "update_plan",
			description: "Replace a plan, naming the plan_revision from get_plan. A stale revision is refused: reload and reapply. To tick checklist items use update_plan_checklist.",
			schema:      object([]string{"issue_key", "plan", "plan_revision"}, map[string]any{"issue_key": issueKey, "plan": text("Complete plan markdown; empty clears it."), "plan_revision": integer("Revision the edit started from.", 1)}),
			run:         bind((*Server).updatePlan),
		},
		{
			name:        "list_subissues",
			description: "List an issue's lightweight subissues with descriptions and description revisions.",
			schema:      object([]string{"issue_key"}, map[string]any{"issue_key": issueKey}),
			readOnly:    true,
			run:         bind((*Server).listSubissues),
		},
		{
			name:        "create_subissue",
			description: "Create a lightweight task under an issue, sharing the parent's reviews and discussions. Use it for a work breakdown rather than create_issue.",
			schema:      object([]string{"issue_key", "title", "status"}, map[string]any{"issue_key": issueKey, "title": text(""), "description": text(""), "status": status, "labels": array("Existing label names.", text(""))}),
			run:         bind((*Server).createSubissue),
		},
		{
			name:        "update_subissue",
			description: "Update the supplied fields of a subissue. A description edit needs description_revision from list_subissues; stale edits are refused. Labels replace the set.",
			schema:      object([]string{"issue_key", "subissue_key"}, map[string]any{"issue_key": issueKey, "subissue_key": text("Key such as WP-142.1."), "title": text(""), "description": text("Complete markdown."), "description_revision": integer("Revision the edit started from.", 1), "status": status, "labels": array("Existing label names; empty removes all.", text(""))}),
			run:         bind((*Server).updateSubissue),
		},
		{
			name:        "get_review_round",
			description: "Get a round's summary, coverage, reviewer, timing, and a page of its findings in every status, including resolved ones and duplicates.",
			schema:      object([]string{"issue_key", "round"}, map[string]any{"issue_key": issueKey, "round": integer("", 1), "include_comments": boolean("Include each finding's first discussion page; true if omitted.")}),
			readOnly:    true,
			run:         bind((*Server).getReviewRound),
		},
		{
			name:        "get_finding",
			description: "Get one finding in any status, with its discussion. comments_next_cursor continues through list_comments.",
			schema:      object([]string{"issue_key", "round", "finding"}, map[string]any{"issue_key": issueKey, "round": integer("", 1), "finding": findingLabel, "include_comments": boolean("Include the discussion; true if omitted.")}),
			readOnly:    true,
			run:         bind((*Server).getFinding),
		},
		{
			name:        "list_review_rounds",
			description: "List an issue's review rounds with summaries. get_review_round reads a round's findings.",
			schema:      object([]string{"issue_key"}, map[string]any{"issue_key": issueKey}),
			readOnly:    true,
			run:         bind((*Server).listReviewRounds),
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
	limit, offset, hash, err := args.bounds("projects", pageSizeRows)
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
		var labelOutputs []string
		for _, label := range labels {
			labelOutputs = append(labelOutputs, label.Name)
		}
		milestones, err := s.store.ListMilestones(ctx, p.ID)
		if err != nil {
			return nil, fmt.Errorf("list milestones for project %s: %w", p.Key, err)
		}
		var milestoneOutputs []map[string]any
		for _, milestone := range milestones {
			milestoneOutputs = append(milestoneOutputs, map[string]any{
				"id": milestone.ID, "name": milestone.Name, "position": milestone.Position, "status": milestone.Status,
			})
		}
		project := map[string]any{"key": p.Key, "name": p.Name, "slug": p.Slug, "url": strings.TrimRight(s.publicURL.String(), "/") + "/" + p.Slug + "/issues"}
		if len(labelOutputs) > 0 {
			project["labels"] = labelOutputs
		}
		if len(milestoneOutputs) > 0 {
			project["milestones"] = milestoneOutputs
		}
		outputs = append(outputs, project)
	}
	return map[string]any{"projects": outputs, "next_cursor": cursor}, nil
}

func (s *Server) listIssues(ctx context.Context, _ store.Caller, args projectArgs) (any, error) {
	limit, offset, hash, err := args.pageArgs.bounds([]any{"issues", args.ProjectKey, args.Statuses, args.Search}, pageSizeRows)
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
				outputs = append(outputs, issueRow(issue))
			}
			before = issue.Number
		}
		if len(issues) < 100 {
			break
		}
	}
	return map[string]any{"issues": outputs, "next_cursor": cursor}, nil
}

// issueRow is one line of an issue list: enough to choose an issue, with no links.
func issueRow(issue store.Issue) map[string]any {
	return map[string]any{"key": issue.Key(), "title": issue.Title, "status": issue.Status}
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
	return newPlanOutput(issue, false), nil
}

// subissueOutput exposes stable keys and the revision needed for safe description edits.
type subissueOutput struct {
	Key         string            `json:"key"`
	Title       string            `json:"title"`
	Description string            `json:"description,omitempty"`
	Revision    int               `json:"description_revision"`
	Status      store.IssueStatus `json:"status"`
	Labels      []string          `json:"labels,omitempty"`
	CreatedBy   *actorOutput      `json:"created_by"`
}

func (s *Server) subissueOutput(ctx context.Context, issue store.Issue, sub store.Subissue) (subissueOutput, error) {
	actor, err := newActorNames(s.store).name(ctx, sub.CreatedByID)
	if err != nil {
		return subissueOutput{}, err
	}
	labels, err := s.labelNameList(ctx, issue.ProjectID, sub.LabelIDs)
	return subissueOutput{Key: fmt.Sprintf("%s.%d", issue.Key(), sub.Number), Title: sub.Title, Description: sub.Description, Revision: sub.DescriptionRevision, Status: sub.Status, Labels: labels, CreatedBy: actor}, err
}

// subissue finds a subissue of the issue by its key, such as WP-142.1, ignoring case.
func (s *Server) subissue(ctx context.Context, issue store.Issue, key string) (store.Subissue, error) {
	subs, err := s.store.ListSubissues(ctx, issue.ID)
	if err != nil {
		return store.Subissue{}, err
	}
	index := slices.IndexFunc(subs, func(sub store.Subissue) bool {
		return strings.EqualFold(fmt.Sprintf("%s.%d", issue.Key(), sub.Number), strings.TrimSpace(key))
	})
	if index < 0 {
		return store.Subissue{}, store.Refuse(store.ErrNotFound, "there is no subissue %s on %s", key, issue.Key())
	}
	return subs[index], nil
}

func (s *Server) listSubissues(ctx context.Context, _ store.Caller, args struct {
	pageArgs
	IssueKey string `json:"issue_key"`
}) (any, error) {
	issue, err := s.issue(ctx, args.IssueKey)
	if err != nil {
		return nil, err
	}
	limit, offset, hash, err := args.bounds([]string{"subissues", issue.ID}, pageSizeRows)
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
	IssueKey    string   `json:"issue_key"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Status      string   `json:"status"`
	Labels      []string `json:"labels"`
}) (any, error) {
	issue, err := s.issue(ctx, args.IssueKey)
	if err != nil {
		return nil, err
	}
	labelIDs, err := s.labelIDs(ctx, issue.ProjectID, issue.ProjectKey, "labels", args.Labels)
	if err != nil {
		return nil, err
	}
	sub, err := s.store.CreateSubissue(ctx, caller, issue.ID, store.NewSubissue{Title: args.Title, Description: args.Description, Status: store.IssueStatus(args.Status), LabelIDs: labelIDs})
	if err != nil {
		return nil, err
	}
	return s.subissueOutput(ctx, issue, sub)
}

func (s *Server) updateSubissue(ctx context.Context, caller store.Caller, args struct {
	IssueKey    string    `json:"issue_key"`
	SubissueKey string    `json:"subissue_key"`
	Title       *string   `json:"title"`
	Description *string   `json:"description"`
	Revision    int       `json:"description_revision"`
	Status      *string   `json:"status"`
	Labels      *[]string `json:"labels"`
}) (any, error) {
	if args.Title == nil && args.Description == nil && args.Status == nil && args.Labels == nil {
		return nil, &store.ValidationError{Field: "fields", Message: "supply title, description, status, or labels"}
	}
	issue, err := s.issue(ctx, args.IssueKey)
	if err != nil {
		return nil, err
	}
	current, err := s.subissue(ctx, issue, args.SubissueKey)
	if err != nil {
		return nil, err
	}
	update := store.SubissueUpdate{DescriptionRevision: args.Revision}
	if args.Labels != nil {
		labelIDs, err := s.labelIDs(ctx, issue.ProjectID, issue.ProjectKey, "labels", *args.Labels)
		if err != nil {
			return nil, err
		}
		update.LabelIDs = store.Change(labelIDs)
	}
	if args.Title != nil {
		update.Title = store.Change(*args.Title)
	}
	if args.Description != nil {
		update.Description = store.Change(*args.Description)
	}
	if args.Status != nil {
		update.Status = store.Change(store.IssueStatus(*args.Status))
	}
	sub, err := s.store.UpdateSubissue(ctx, caller, issue.ID, current.ID, update)
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
	limit, offset, hash, err := args.bounds([]any{"round", round.ID}, pageSizeDiscussion)
	if err != nil {
		return nil, err
	}
	findings, err := s.store.ListAgentFindings(ctx, issue.ID, round.ID, nil, false, limit+1, offset)
	if err != nil {
		return nil, err
	}
	findings, cursor := pageSlice(findings, limit, offset, hash)
	outputs, err := s.findingOutputs(ctx, issue, findings, findingView{Comments: args.IncludeComments == nil || *args.IncludeComments, Detail: true})
	if err != nil {
		return nil, err
	}
	reviewer, err := newActorNames(s.store).name(ctx, round.AuthorActorID)
	if err != nil {
		return nil, err
	}
	result := map[string]any{
		"issue_key": issue.Key(), "round": s.newRoundOutput(issue, round, nil, true), "reviewer": reviewer,
		"started_at": round.CreatedAt.UTC().Format(time.RFC3339), "findings": outputs, "next_cursor": cursor,
	}
	if round.PlanCoverage != "" {
		result["plan_coverage"] = round.PlanCoverage
	}
	if round.FinishedAt != nil {
		result["finished_at"] = round.FinishedAt.UTC().Format(time.RFC3339)
	}
	return result, nil
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
	outputs, err := s.findingOutputs(ctx, issue, []store.Finding{finding}, findingView{Comments: args.IncludeComments == nil || *args.IncludeComments, Detail: true, Link: true})
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
	limit, offset, hash, err := args.bounds([]string{"rounds", issue.ID}, pageSizeRows)
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
		outputs = append(outputs, s.newRoundOutput(issue, round, nil, false))
	}
	return map[string]any{"issue_key": issue.Key(), "review_rounds": outputs, "next_cursor": cursor}, nil
}
