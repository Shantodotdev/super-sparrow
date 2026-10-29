package httpserver

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/shantodotdev/super-sparrow/server/internal/store"
)

func (s *server) listRounds(w http.ResponseWriter, r *http.Request) error {
	issue, err := s.issueFromPath(r)
	if err != nil {
		return err
	}
	rounds, err := s.store.ListReviewRounds(r.Context(), issue.ID)
	if err != nil {
		return err
	}

	views := make([]roundView, 0, len(rounds))
	for _, round := range rounds {
		views = append(views, newRoundView(round))
	}
	writeJSON(w, http.StatusOK, map[string]any{"rounds": views})
	return nil
}

func (s *server) startRound(w http.ResponseWriter, r *http.Request) error {
	var body struct {
		FromCommit          string `json:"from_commit"`
		ToCommit            string `json:"to_commit"`
		IncludesUncommitted bool   `json:"includes_uncommitted"`
		PlanCoverage        string `json:"plan_coverage"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		return err
	}

	issue, err := s.issueFromPath(r)
	if err != nil {
		return err
	}
	round, err := s.store.StartReviewRound(r.Context(), identityFrom(r.Context()).caller(), store.NewReviewRound{
		IssueID: issue.ID, FromCommit: body.FromCommit, ToCommit: body.ToCommit,
		IncludesUncommitted: body.IncludesUncommitted, PlanCoverage: body.PlanCoverage,
	})
	if err != nil {
		return err
	}

	writeJSON(w, http.StatusCreated, map[string]any{"round": newRoundView(round)})
	return nil
}

// getRound returns a round with its findings, most severe first.
func (s *server) getRound(w http.ResponseWriter, r *http.Request) error {
	_, round, err := s.roundFromPath(r)
	if err != nil {
		return err
	}
	findings, err := s.store.ListRoundFindings(r.Context(), round.ID)
	if err != nil {
		return err
	}

	writeJSON(w, http.StatusOK, map[string]any{"round": newRoundView(round), "findings": newFindingViews(findings)})
	return nil
}

func (s *server) finishRound(w http.ResponseWriter, r *http.Request) error {
	var body struct {
		Summary string `json:"summary"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		return err
	}

	_, round, err := s.roundFromPath(r)
	if err != nil {
		return err
	}
	round, err = s.store.FinishReviewRound(r.Context(), identityFrom(r.Context()).caller(), round.ID, body.Summary)
	if err != nil {
		return err
	}

	writeJSON(w, http.StatusOK, map[string]any{"round": newRoundView(round)})
	return nil
}

func (s *server) addFinding(w http.ResponseWriter, r *http.Request) error {
	var body struct {
		Severity  string `json:"severity"`
		Title     string `json:"title"`
		Problem   string `json:"problem"`
		Fix       string `json:"fix"`
		Evidence  string `json:"evidence"`
		FilePath  string `json:"file_path"`
		Line      int    `json:"line"`
		CommitSHA string `json:"commit_sha"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		return err
	}

	_, round, err := s.roundFromPath(r)
	if err != nil {
		return err
	}
	finding, err := s.store.AddFinding(r.Context(), identityFrom(r.Context()).caller(), store.NewFinding{
		ReviewRoundID: round.ID, Severity: store.Severity(body.Severity), Title: body.Title, Problem: body.Problem,
		Fix: body.Fix, Evidence: body.Evidence, FilePath: body.FilePath, Line: body.Line, CommitSHA: body.CommitSHA,
	})
	if err != nil {
		return err
	}

	writeJSON(w, http.StatusCreated, map[string]any{"finding": newFindingView(finding)})
	return nil
}

// listIssueFindings returns the findings of every round of an issue. ?status narrows them to a
// comma-separated list of statuses, and duplicates are left out unless ?include_duplicates=true.
func (s *server) listIssueFindings(w http.ResponseWriter, r *http.Request) error {
	issue, err := s.issueFromPath(r)
	if err != nil {
		return err
	}
	statuses, includeDuplicates, err := findingQuery(r)
	if err != nil {
		return err
	}
	findings, err := s.store.ListIssueFindings(r.Context(), issue.ID, statuses, !includeDuplicates)
	if err != nil {
		return renameField(err, "statuses", "status")
	}

	writeJSON(w, http.StatusOK, map[string]any{"findings": newFindingViews(findings)})
	return nil
}

// listProjectFindings returns the findings of every issue in a project, grouped by issue, newest
// issue first, with a summary of each issue they belong to. It filters like listIssueFindings.
func (s *server) listProjectFindings(w http.ResponseWriter, r *http.Request) error {
	project, err := s.projectFromPath(r)
	if err != nil {
		return err
	}
	statuses, includeDuplicates, err := findingQuery(r)
	if err != nil {
		return err
	}

	findings, err := s.store.ListProjectFindings(r.Context(), project.ID, statuses, !includeDuplicates)
	if err != nil {
		return renameField(err, "statuses", "status")
	}

	issues := []issueView{}
	seen := map[string]bool{}
	for _, finding := range findings {
		if seen[finding.IssueID] {
			continue
		}
		seen[finding.IssueID] = true

		issue, err := s.store.GetIssue(r.Context(), finding.IssueID)
		if err != nil {
			return err
		}
		issues = append(issues, newIssueSummaryView(issue))
	}

	writeJSON(w, http.StatusOK, map[string]any{"findings": newFindingViews(findings), "issues": issues})
	return nil
}

// findingQuery reads the finding list filters: ?status, a comma-separated list of statuses, and
// ?include_duplicates.
func findingQuery(r *http.Request) ([]store.FindingStatus, bool, error) {
	includeDuplicates, err := queryBool(r, "include_duplicates")
	if err != nil {
		return nil, false, err
	}

	var statuses []store.FindingStatus
	for _, status := range queryList(r, "status") {
		statuses = append(statuses, store.FindingStatus(status))
	}
	return statuses, includeDuplicates, nil
}

func (s *server) getFinding(w http.ResponseWriter, r *http.Request) error {
	_, finding, err := s.findingFromPath(r)
	if err != nil {
		return err
	}

	writeJSON(w, http.StatusOK, map[string]any{"finding": newFindingView(finding)})
	return nil
}

// changeFindingStatus moves a finding to a new status. When a later decision creates a follow-up
// issue, the response includes it.
func (s *server) changeFindingStatus(w http.ResponseWriter, r *http.Request) error {
	var body struct {
		Status        string `json:"status"`
		Reason        string `json:"reason"`
		FixedInCommit string `json:"fixed_in_commit"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		return err
	}

	_, finding, err := s.findingFromPath(r)
	if err != nil {
		return err
	}
	finding, err = s.store.ChangeFindingStatus(r.Context(), identityFrom(r.Context()).caller(), finding.ID,
		store.FindingChange{Status: store.FindingStatus(body.Status), Reason: body.Reason, FixedInCommit: body.FixedInCommit})
	if err != nil {
		return err
	}

	response := map[string]any{"finding": newFindingView(finding), "follow_up_issue": nil}
	if finding.FollowUpIssueID != "" {
		followUp, err := s.store.GetIssue(r.Context(), finding.FollowUpIssueID)
		if err != nil {
			return err
		}
		response["follow_up_issue"] = newIssueSummaryView(followUp)
	}
	writeJSON(w, http.StatusOK, response)
	return nil
}

// markDuplicate marks the finding as a duplicate of duplicate_of_finding_id, or clears the mark
// when it is empty.
func (s *server) markDuplicate(w http.ResponseWriter, r *http.Request) error {
	var body struct {
		DuplicateOfFindingID *string `json:"duplicate_of_finding_id"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		return err
	}
	if body.DuplicateOfFindingID == nil {
		return &store.ValidationError{Field: "duplicate_of_finding_id", Message: `is required; send "" to clear the mark`}
	}

	_, finding, err := s.findingFromPath(r)
	if err != nil {
		return err
	}
	if err := s.store.MarkDuplicate(r.Context(), identityFrom(r.Context()).caller(), finding.ID, *body.DuplicateOfFindingID); err != nil {
		return err
	}

	finding, err = s.store.GetFinding(r.Context(), finding.ID)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]any{"finding": newFindingView(finding)})
	return nil
}

// findingRef names a finding by its round and label, as in round 2, finding R3.
type findingRef struct {
	Round   int    `json:"round"`
	Finding string `json:"finding"`
}

// resolveFindings turns references into the IDs of the issue's findings. field names the request
// field the references came from.
func (s *server) resolveFindings(r *http.Request, issue store.Issue, field string, refs []findingRef) ([]string, error) {
	ids := make([]string, 0, len(refs))
	for _, ref := range refs {
		number, err := store.ParseFindingLabel(field, ref.Finding)
		if err != nil {
			return nil, err
		}

		finding, err := s.store.GetFindingByNumber(r.Context(), issue.ID, ref.Round, number)
		if errors.Is(err, store.ErrNotFound) {
			return nil, &store.ValidationError{Field: field, Message: fmt.Sprintf("%s has no finding R%d in round %d", issue.Key(), number, ref.Round)}
		}
		if err != nil {
			return nil, err
		}
		ids = append(ids, finding.ID)
	}
	return ids, nil
}

// postFixSummary reports a fix: it marks the named findings fixed in commit and adds summary as a
// comment on the issue.
func (s *server) postFixSummary(w http.ResponseWriter, r *http.Request) error {
	var body struct {
		Summary  string       `json:"summary"`
		Commit   string       `json:"commit"`
		Findings []findingRef `json:"findings"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		return err
	}

	issue, err := s.issueFromPath(r)
	if err != nil {
		return err
	}
	findingIDs, err := s.resolveFindings(r, issue, "findings", body.Findings)
	if err != nil {
		return err
	}

	comment, fixed, err := s.store.PostFixSummary(r.Context(), identityFrom(r.Context()).caller(), store.FixSummary{
		IssueID: issue.ID, Summary: body.Summary, Commit: body.Commit, FindingIDs: findingIDs,
	})
	if err != nil {
		return err
	}

	writeJSON(w, http.StatusCreated, map[string]any{"comment": newCommentView(comment), "findings": newFindingViews(fixed)})
	return nil
}
