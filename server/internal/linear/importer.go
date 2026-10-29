package linear

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/shantodotdev/super-sparrow/server/internal/store"
)

const (
	maxImportedComment = 20_000
	truncatedNote      = "\n\n*This comment was cut short on import; the full text is in Linear.*"
)

// Report counts what an import brought in. Warnings name what it skipped and why.
type Report struct {
	ProjectKey   string
	Milestones   int
	Issues       int
	Comments     int
	ReviewRounds int
	Findings     int
	Warnings     []string
}

func (r *Report) warn(format string, args ...any) {
	r.Warnings = append(r.Warnings, fmt.Sprintf(format, args...))
}

// Import brings a Linear project into the store as the system actor. Issues keep their numbers,
// so WP-142 in Linear is WP-142 here. Each comment is imported with its author and time written
// at the top, since Linear's users are not actors here, and a comment in the review round format
// also becomes a review round with its findings. Running it again imports only what is new.
//
// teamKey picks the team whose key becomes the project's key when the Linear project has more
// than one team; pass "" otherwise.
func Import(ctx context.Context, st *store.Store, client *Client, projectName, teamKey string) (Report, error) {
	importer := store.Caller{ActorID: store.SystemActorID}
	var report Report

	linearProject, err := client.FindProject(ctx, projectName)
	if err != nil {
		return report, err
	}
	key, err := chooseTeam(linearProject, teamKey)
	if err != nil {
		return report, err
	}
	report.ProjectKey = key

	project, err := st.ImportProject(ctx, importer, store.ImportLinear, store.ImportedProject{ExternalID: linearProject.ID, Key: key, Name: linearProject.Name})
	if err != nil {
		return report, fmt.Errorf("import project %s: %w", linearProject.Name, err)
	}

	issues, err := client.Issues(ctx, linearProject.ID)
	if err != nil {
		return report, err
	}
	sort.Slice(issues, func(i, j int) bool { return issues[i].Number < issues[j].Number })

	milestones, err := importMilestones(ctx, st, client, importer, project, linearProject.ID, issues, &report)
	if err != nil {
		return report, err
	}

	// Import every issue first, then link parents, since a child can come before its parent.
	imported := map[string]store.Issue{}
	for _, linearIssue := range issues {
		if linearIssue.Team.Key != key {
			report.warn("%s belongs to team %s, not %s, so it was skipped", linearIssue.Identifier, linearIssue.Team.Key, key)
			continue
		}

		issue, err := importIssue(ctx, st, importer, project, linearIssue, milestones)
		if err != nil {
			report.warn("%s was not imported: %v", linearIssue.Identifier, err)
			continue
		}
		imported[linearIssue.ID] = issue
		report.Issues++
	}

	for _, linearIssue := range issues {
		issue, ok := imported[linearIssue.ID]
		if !ok || linearIssue.Parent == nil || issue.ParentIssueID != "" {
			continue
		}
		parent, ok := imported[linearIssue.Parent.ID]
		if !ok {
			report.warn("%s has a parent outside this project, so it was imported without one", linearIssue.Identifier)
			continue
		}
		if _, err := st.UpdateIssue(ctx, importer, issue.ID, store.IssueUpdate{ParentIssueID: store.Change(parent.ID)}); err != nil {
			report.warn("%s could not be linked to its parent %s: %v", linearIssue.Identifier, parent.Key(), err)
		}
	}

	for _, linearIssue := range issues {
		if issue, ok := imported[linearIssue.ID]; ok {
			if err := importComments(ctx, st, client, importer, issue, linearIssue.ID, &report); err != nil {
				return report, err
			}
		}
	}

	return report, nil
}

func chooseTeam(project Project, teamKey string) (string, error) {
	switch {
	case teamKey != "":
		for _, team := range project.Teams {
			if strings.EqualFold(team, teamKey) {
				return strings.ToUpper(team), nil
			}
		}
		return "", fmt.Errorf("the Linear project %q does not belong to team %s; its teams are %s", project.Name, teamKey, strings.Join(project.Teams, ", "))
	case len(project.Teams) == 1:
		return strings.ToUpper(project.Teams[0]), nil
	case len(project.Teams) == 0:
		return "", fmt.Errorf("the Linear project %q belongs to no team", project.Name)
	}
	return "", fmt.Errorf("the Linear project %q belongs to teams %s; pass --team to choose one", project.Name, strings.Join(project.Teams, ", "))
}

// importMilestones imports the project's milestones in order. A milestone whose issues are all
// closed is done, one with any started issue is active, and the rest are planned.
func importMilestones(ctx context.Context, st *store.Store, client *Client, importer store.Caller, project store.Project,
	linearProjectID string, issues []Issue, report *Report) (map[string]string, error) {
	linearMilestones, err := client.Milestones(ctx, linearProjectID)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(linearMilestones, func(i, j int) bool { return linearMilestones[i].SortOrder < linearMilestones[j].SortOrder })

	ids := map[string]string{}
	for _, linearMilestone := range linearMilestones {
		milestone, err := st.ImportMilestone(ctx, importer, store.ImportLinear, store.ImportedMilestone{
			ExternalID: linearMilestone.ID, ProjectID: project.ID, Name: linearMilestone.Name,
			Status: milestoneStatus(linearMilestone.ID, issues),
		})
		if err != nil {
			report.warn("milestone %q was not imported: %v", linearMilestone.Name, err)
			continue
		}
		ids[linearMilestone.ID] = milestone.ID
		report.Milestones++
	}
	return ids, nil
}

func milestoneStatus(milestoneID string, issues []Issue) store.MilestoneStatus {
	total, closed, started := 0, 0, false
	for _, issue := range issues {
		if issue.ProjectMilestone == nil || issue.ProjectMilestone.ID != milestoneID {
			continue
		}
		total++
		switch issueStatus(issue) {
		case store.IssueDone, store.IssueCanceled:
			closed++
		case store.IssueInProgress, store.IssueInReview:
			started = true
		}
	}

	switch {
	case total > 0 && closed == total:
		return store.MilestoneDone
	case started || closed > 0:
		return store.MilestoneActive
	}
	return store.MilestonePlanned
}

func importIssue(ctx context.Context, st *store.Store, importer store.Caller, project store.Project, linearIssue Issue, milestones map[string]string) (store.Issue, error) {
	input := store.ImportedIssue{
		ExternalID: linearIssue.ID, ProjectID: project.ID, Number: linearIssue.Number, Title: linearIssue.Title,
		Plan: linearIssue.Description, Status: issueStatus(linearIssue), BranchName: linearIssue.BranchName,
		CompletedAt: linearIssue.CompletedAt,
	}
	if linearIssue.CanceledAt != nil {
		input.CompletedAt = linearIssue.CanceledAt
	}
	if linearIssue.ProjectMilestone != nil {
		input.MilestoneID = milestones[linearIssue.ProjectMilestone.ID]
	}
	if linearIssue.Assignee != nil && linearIssue.Assignee.Email != "" {
		if actor, err := st.GetActorByEmail(ctx, linearIssue.Assignee.Email); err == nil && actor.Active() {
			input.AssigneeActorID = actor.ID
		}
	}
	for _, label := range linearIssue.Labels.Nodes {
		imported, err := st.EnsureLabel(ctx, importer, project.ID, label.Name)
		if err != nil {
			return store.Issue{}, fmt.Errorf("label %q: %w", label.Name, err)
		}
		input.LabelIDs = append(input.LabelIDs, imported.ID)
	}

	return st.ImportIssue(ctx, importer, store.ImportLinear, input)
}

// issueStatus maps a Linear workflow state to an issue status. Linear has no review state type,
// so a started state whose name mentions review counts as in review.
func issueStatus(issue Issue) store.IssueStatus {
	switch issue.State.Type {
	case "unstarted":
		return store.IssueTodo
	case "started":
		if strings.Contains(strings.ToLower(issue.State.Name), "review") {
			return store.IssueInReview
		}
		return store.IssueInProgress
	case "completed":
		return store.IssueDone
	case "canceled":
		return store.IssueCanceled
	}
	return store.IssueBacklog
}

func importComments(ctx context.Context, st *store.Store, client *Client, importer store.Caller, issue store.Issue, linearIssueID string, report *Report) error {
	comments, err := client.Comments(ctx, linearIssueID)
	if err != nil {
		return err
	}
	sort.SliceStable(comments, func(i, j int) bool { return comments[i].CreatedAt.Before(comments[j].CreatedAt) })

	for _, comment := range comments {
		if strings.TrimSpace(comment.Body) == "" {
			continue
		}

		if _, err := st.ImportComment(ctx, importer, store.ImportLinear, store.ImportedComment{
			ExternalID: comment.ID, IssueID: issue.ID, Body: importedBody(comment),
		}); err != nil {
			report.warn("a comment on %s was not imported: %v", issue.Key(), err)
			continue
		}
		report.Comments++

		parsed, ok := ParseReviewRound(comment.Body)
		if !ok {
			continue
		}
		if parsed.ToCommit == "" {
			report.warn("review round %d on %s names no commit, so it was imported as a comment only", parsed.Number, issue.Key())
			continue
		}

		_, err := st.ImportReviewRound(ctx, importer, store.ImportLinear, store.ImportedReviewRound{
			ExternalID: comment.ID + "#review-round", IssueID: issue.ID, Number: parsed.Number,
			FromCommit: parsed.FromCommit, ToCommit: parsed.ToCommit, IncludesUncommitted: parsed.IncludesUncommitted,
			PlanCoverage: parsed.PlanCoverage, Summary: parsed.Summary, Findings: parsed.Findings,
		})
		var validation *store.ValidationError
		switch {
		case errors.As(err, &validation), errors.Is(err, store.ErrConflict):
			report.warn("review round %d on %s was imported as a comment only: %v", parsed.Number, issue.Key(), err)
		case err != nil:
			return fmt.Errorf("import review round %d on %s: %w", parsed.Number, issue.Key(), err)
		default:
			report.ReviewRounds++
			report.Findings += len(parsed.Findings)
		}
	}
	return nil
}

// importedBody puts the original author and time above the comment, and cuts a comment longer
// than comments here allow.
func importedBody(comment Comment) string {
	author := "someone"
	if comment.User != nil && comment.User.Name != "" {
		author = comment.User.Name
	}
	header := fmt.Sprintf("*Imported from Linear. Written by %s on %s.*\n\n", author, comment.CreatedAt.UTC().Format(time.DateTime+" UTC"))

	body := strings.TrimSpace(comment.Body)
	if runes := []rune(body); len(runes)+len([]rune(header)) > maxImportedComment {
		body = string(runes[:maxImportedComment-len([]rune(header))-len([]rune(truncatedNote))]) + truncatedNote
	}
	return header + body
}
