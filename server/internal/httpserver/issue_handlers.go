package httpserver

import (
	"net/http"

	"github.com/shantodotdev/super-sparrow/server/internal/store"
)

// listIssues returns a page of a project's issues, newest first. It filters by ?status (a
// comma-separated list), ?milestone, and ?assignee, and pages with ?before and ?limit; next_before
// is the value of ?before that fetches the next page, or null on the last one.
func (s *server) listIssues(w http.ResponseWriter, r *http.Request) error {
	project, err := s.projectFromPath(r)
	if err != nil {
		return err
	}

	filter := store.IssueFilter{ProjectID: project.ID, MilestoneID: r.URL.Query().Get("milestone"), AssigneeID: r.URL.Query().Get("assignee")}
	for _, status := range queryList(r, "status") {
		filter.Statuses = append(filter.Statuses, store.IssueStatus(status))
	}
	if filter.BeforeNumber, err = queryInt(r, "before"); err != nil {
		return err
	}
	if filter.Limit, err = queryInt(r, "limit"); err != nil {
		return err
	}

	issues, err := s.store.ListIssues(r.Context(), filter)
	if err != nil {
		return err
	}

	views := make([]issueView, 0, len(issues))
	for _, issue := range issues {
		views = append(views, newIssueSummaryView(issue))
	}
	var nextBefore *int
	if len(issues) == pageSize(filter.Limit) {
		nextBefore = &issues[len(issues)-1].Number
	}
	writeJSON(w, http.StatusOK, map[string]any{"issues": views, "next_before": nextBefore})
	return nil
}

func (s *server) createIssue(w http.ResponseWriter, r *http.Request) error {
	var body struct {
		Title           string   `json:"title"`
		Plan            string   `json:"plan"`
		Status          string   `json:"status"`
		ParentIssueID   string   `json:"parent_issue_id"`
		MilestoneID     string   `json:"milestone_id"`
		AssigneeActorID string   `json:"assignee_actor_id"`
		BranchName      string   `json:"branch_name"`
		PullRequestURL  string   `json:"pull_request_url"`
		LabelIDs        []string `json:"label_ids"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		return err
	}

	project, err := s.projectFromPath(r)
	if err != nil {
		return err
	}
	issue, err := s.store.CreateIssue(r.Context(), identityFrom(r.Context()).caller(), store.NewIssue{
		ProjectID: project.ID, Title: body.Title, Plan: body.Plan, Status: store.IssueStatus(body.Status),
		ParentIssueID: body.ParentIssueID, MilestoneID: body.MilestoneID, AssigneeActorID: body.AssigneeActorID,
		BranchName: body.BranchName, PullRequestURL: body.PullRequestURL, LabelIDs: body.LabelIDs,
	})
	if err != nil {
		return err
	}

	writeJSON(w, http.StatusCreated, map[string]any{"issue": newIssueView(issue)})
	return nil
}

func (s *server) getIssue(w http.ResponseWriter, r *http.Request) error {
	issue, err := s.issueFromPath(r)
	if err != nil {
		return err
	}

	writeJSON(w, http.StatusOK, map[string]any{"issue": newIssueView(issue)})
	return nil
}

// issueForBranch finds the issue a branch belongs to, from ?branch=feat/wp-142-meta-comments.
func (s *server) issueForBranch(w http.ResponseWriter, r *http.Request) error {
	issue, err := s.store.FindIssueForBranch(r.Context(), r.URL.Query().Get("branch"))
	if err != nil {
		return err
	}

	writeJSON(w, http.StatusOK, map[string]any{"issue": newIssueView(issue)})
	return nil
}

// updateIssue changes the fields the body names. A field that is absent stays as it is; an
// empty string clears an optional field, and label_ids replaces the whole set.
func (s *server) updateIssue(w http.ResponseWriter, r *http.Request) error {
	var body struct {
		Title           *string   `json:"title"`
		Plan            *string   `json:"plan"`
		Status          *string   `json:"status"`
		ParentIssueID   *string   `json:"parent_issue_id"`
		MilestoneID     *string   `json:"milestone_id"`
		AssigneeActorID *string   `json:"assignee_actor_id"`
		BranchName      *string   `json:"branch_name"`
		PullRequestURL  *string   `json:"pull_request_url"`
		LabelIDs        *[]string `json:"label_ids"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		return err
	}

	issue, err := s.issueFromPath(r)
	if err != nil {
		return err
	}

	update := store.IssueUpdate{
		Title: optionalField(body.Title), Plan: optionalField(body.Plan), ParentIssueID: optionalField(body.ParentIssueID),
		MilestoneID: optionalField(body.MilestoneID), AssigneeActorID: optionalField(body.AssigneeActorID),
		BranchName: optionalField(body.BranchName), PullRequestURL: optionalField(body.PullRequestURL),
		LabelIDs: optionalField(body.LabelIDs),
	}
	if body.Status != nil {
		update.Status = store.Change(store.IssueStatus(*body.Status))
	}

	issue, err = s.store.UpdateIssue(r.Context(), identityFrom(r.Context()).caller(), issue.ID, update)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]any{"issue": newIssueView(issue)})
	return nil
}

// checklistChange is one item to tick or untick, by its index in the plan's checklist.
type checklistChange struct {
	Index   *int  `json:"index"`
	Checked *bool `json:"checked"`
}

// updateChecklist ticks or unticks plan checklist items without rewriting the rest of the plan.
func (s *server) updateChecklist(w http.ResponseWriter, r *http.Request) error {
	var body struct {
		Items []checklistChange `json:"items"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		return err
	}

	changes, err := checklistChanges(body.Items)
	if err != nil {
		return err
	}
	issue, err := s.issueFromPath(r)
	if err != nil {
		return err
	}

	issue, err = s.store.UpdatePlanChecklist(r.Context(), identityFrom(r.Context()).caller(), issue.ID, changes)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]any{"issue": newIssueView(issue)})
	return nil
}

func checklistChanges(items []checklistChange) (map[int]bool, error) {
	changes := map[int]bool{}
	for _, item := range items {
		if item.Index == nil || item.Checked == nil {
			return nil, &store.ValidationError{Field: "items", Message: "each item needs an index and checked"}
		}
		changes[*item.Index] = *item.Checked
	}
	return changes, nil
}
