package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ImportSource names where imported records came from.
type ImportSource string

// ImportLinear marks records imported from Linear.
const ImportLinear ImportSource = "linear"

// Each Import method records a link from the source's ID to the new record in the same
// transaction, and returns the linked record without changing it when the source ID was imported
// before, so an import can be run again safely. Imports are written as the caller, normally the
// system actor, and record ".imported" events.

// ImportedProject is a project to import.
type ImportedProject struct {
	ExternalID string
	Key        string
	Name       string
}

// ImportProject imports a project, or links an existing project with the same key.
func (s *Store) ImportProject(ctx context.Context, by Caller, source ImportSource, input ImportedProject) (Project, error) {
	key := strings.ToUpper(strings.TrimSpace(input.Key))
	if !projectKeyPattern.MatchString(key) {
		return Project{}, invalid("key", "must be 2 to 10 capital letters and digits, starting with a letter")
	}
	name, err := requiredText("name", input.Name, maxNameLength)
	if err != nil {
		return Project{}, err
	}

	var project Project
	err = s.write(ctx, by, func(w *writeTx) error {
		id, err := importedTarget(w, source, input.ExternalID)
		if err != nil {
			return err
		}
		if id != "" {
			project, err = getProject(w.ctx, w.tx, `id = ?`, id)
			return err
		}

		// A project made by hand with the same key is linked rather than duplicated.
		project, err = getProject(w.ctx, w.tx, `key = ?`, key)
		if errors.Is(err, ErrNotFound) {
			project, err = insertImportedProject(w, source, input.ExternalID, key, name)
		}
		if err != nil {
			return err
		}

		return linkImport(w, source, input.ExternalID, TargetProject, project.ID)
	})

	return project, err
}

func insertImportedProject(w *writeTx, source ImportSource, externalID, key, name string) (Project, error) {
	project := Project{ID: newID(), Key: key, Name: name, NextIssueNumber: 1, CreatedAt: w.now, UpdatedAt: w.now}
	_, err := w.exec(`INSERT INTO projects (id, key, name, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		project.ID, key, name, w.timestamp(), w.timestamp())
	if err != nil {
		return Project{}, fmt.Errorf("import project: %w", err)
	}

	err = w.event(project.ID, TargetProject, project.ID, "project.imported",
		map[string]any{"source": source, "external_id": externalID, "key": key, "name": name})
	return project, err
}

// ImportedMilestone is a milestone to import.
type ImportedMilestone struct {
	ExternalID string
	ProjectID  string
	Name       string
	Status     MilestoneStatus
}

// ImportMilestone imports a milestone after the project's others, or links an existing milestone
// with the same name.
func (s *Store) ImportMilestone(ctx context.Context, by Caller, source ImportSource, input ImportedMilestone) (Milestone, error) {
	name, err := requiredText("name", input.Name, maxNameLength)
	if err != nil {
		return Milestone{}, err
	}
	if !input.Status.valid() {
		return Milestone{}, invalid("status", "must be planned, active, or done")
	}

	var milestone Milestone
	err = s.write(ctx, by, func(w *writeTx) error {
		id, err := importedTarget(w, source, input.ExternalID)
		if err != nil {
			return err
		}
		if id != "" {
			milestone, err = scanMilestone(w.tx.QueryRowContext(w.ctx, `SELECT `+milestoneColumns+` FROM milestones WHERE id = ?`, id))
			return err
		}

		// A milestone made by hand with the same name is linked rather than duplicated.
		milestone, err = scanMilestone(w.tx.QueryRowContext(w.ctx,
			`SELECT `+milestoneColumns+` FROM milestones WHERE project_id = ? AND name = ?`, input.ProjectID, name))
		if errors.Is(err, ErrNotFound) {
			milestone, err = insertImportedMilestone(w, source, input.ExternalID, input.ProjectID, name, input.Status)
		}
		if err != nil {
			return err
		}

		return linkImport(w, source, input.ExternalID, TargetMilestone, milestone.ID)
	})

	return milestone, err
}

func insertImportedMilestone(w *writeTx, source ImportSource, externalID, projectID, name string, status MilestoneStatus) (Milestone, error) {
	milestone := Milestone{ID: newID(), ProjectID: projectID, Name: name, Status: status, CreatedAt: w.now, UpdatedAt: w.now}
	err := w.tx.QueryRowContext(w.ctx,
		`SELECT coalesce(max(position) + 1, 0) FROM milestones WHERE project_id = ?`, projectID).Scan(&milestone.Position)
	if err != nil {
		return Milestone{}, fmt.Errorf("find milestone position: %w", err)
	}

	_, err = w.exec(`
		INSERT INTO milestones (id, project_id, name, position, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		milestone.ID, projectID, name, milestone.Position, status, w.timestamp(), w.timestamp())
	if err != nil {
		return Milestone{}, fmt.Errorf("import milestone: %w", err)
	}

	err = w.event(projectID, TargetMilestone, milestone.ID, "milestone.imported",
		map[string]any{"source": source, "external_id": externalID, "name": name, "status": status})
	return milestone, err
}

// EnsureLabel returns the project's label with this name, creating it when there is none.
// People and agents can create labels.
func (s *Store) EnsureLabel(ctx context.Context, by Caller, projectID, name string) (Label, error) {
	name, err := requiredText("name", name, maxLabelLength)
	if err != nil {
		return Label{}, err
	}

	labels, err := s.ListLabels(ctx, projectID)
	if err != nil {
		return Label{}, err
	}
	for _, label := range labels {
		if label.Name == name {
			return label, nil
		}
	}

	return s.CreateLabel(ctx, by, projectID, name)
}

// ImportedIssue is an issue to import with the number it had at its source, so its key stays the
// same, such as WP-142.
type ImportedIssue struct {
	ExternalID      string
	ProjectID       string
	Number          int
	Title           string
	Plan            string
	Status          IssueStatus
	MilestoneID     string
	AssigneeActorID string
	BranchName      string
	LabelIDs        []string
	// CompletedAt is when the issue was done or canceled at its source, if it was.
	CompletedAt *time.Time
}

// ImportIssue imports an issue with its source number and moves the project's counter past it.
// It fails with ErrConflict when the project already has an issue with that number that was not
// imported from this source ID.
func (s *Store) ImportIssue(ctx context.Context, by Caller, source ImportSource, input ImportedIssue) (Issue, error) {
	if input.Number < 1 {
		return Issue{}, invalid("number", "must be at least 1")
	}
	fields, err := validateNewIssue(NewIssue{
		ProjectID: input.ProjectID, Title: input.Title, Plan: input.Plan, Status: input.Status,
		MilestoneID: input.MilestoneID, AssigneeActorID: input.AssigneeActorID, BranchName: input.BranchName,
		LabelIDs: input.LabelIDs,
	})
	if err != nil {
		return Issue{}, err
	}

	var issue Issue
	err = s.write(ctx, by, func(w *writeTx) error {
		id, err := importedTarget(w, source, input.ExternalID)
		if err != nil {
			return err
		}
		if id != "" {
			issue, err = getIssue(w.ctx, w.tx, `i.id = ?`, id)
			return err
		}

		project, err := getProject(w.ctx, w.tx, `id = ?`, input.ProjectID)
		if err != nil {
			return err
		}
		if err := checkIssueReferences(w, project.ID, "", "", fields.milestoneID, fields.assigneeID); err != nil {
			return err
		}
		if err := checkLabels(w, project.ID, fields.labelIDs); err != nil {
			return err
		}

		issue = Issue{
			ID: newID(), ProjectID: project.ID, ProjectKey: project.Key, Number: input.Number,
			Title: fields.title, Plan: fields.plan, Status: fields.status, MilestoneID: fields.milestoneID,
			AssigneeActorID: fields.assigneeID, BranchName: fields.branch, LabelIDs: fields.labelIDs,
			CreatedByID: w.caller.ID, CreatedAt: w.now, UpdatedAt: w.now,
		}
		completedAt := sql.NullString{}
		if issue.Status.closed() {
			completed := w.now
			if input.CompletedAt != nil {
				completed = input.CompletedAt.UTC().Truncate(time.Millisecond)
			}
			issue.CompletedAt = &completed
			completedAt = formatOptionalTime(&completed)
		}

		_, err = w.exec(`
			INSERT INTO issues (id, project_id, number, title, plan, status, milestone_id, assignee_actor_id,
				branch_name, created_by_actor_id, completed_at, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			issue.ID, project.ID, issue.Number, issue.Title, issue.Plan, issue.Status, nullString(issue.MilestoneID),
			nullString(issue.AssigneeActorID), nullString(issue.BranchName), w.caller.ID, completedAt,
			w.timestamp(), w.timestamp())
		if err != nil {
			return mapUniqueViolation(err, fmt.Sprintf("%s already exists or uses branch %q, so it was not imported", issue.Key(), issue.BranchName), "import issue")
		}
		if err := insertIssueLabels(w, issue.ID, issue.LabelIDs); err != nil {
			return err
		}

		_, err = w.exec(`UPDATE projects SET next_issue_number = max(next_issue_number, ?), updated_at = ? WHERE id = ?`,
			issue.Number+1, w.timestamp(), project.ID)
		if err != nil {
			return fmt.Errorf("move issue counter: %w", err)
		}

		err = w.event(project.ID, TargetIssue, issue.ID, "issue.imported", map[string]any{
			"source": source, "external_id": input.ExternalID, "key": issue.Key(), "title": issue.Title, "status": issue.Status,
		})
		if err != nil {
			return err
		}
		return linkImport(w, source, input.ExternalID, TargetIssue, issue.ID)
	})

	return issue, err
}

// ImportedComment is a comment to import onto an issue.
type ImportedComment struct {
	ExternalID string
	IssueID    string
	Body       string
}

// ImportComment imports a comment onto an issue, written as the caller. The importer puts the
// original author and time into the body, since they are not actors here.
func (s *Store) ImportComment(ctx context.Context, by Caller, source ImportSource, input ImportedComment) (Comment, error) {
	body, err := requiredText("body", input.Body, maxCommentLength)
	if err != nil {
		return Comment{}, err
	}

	var comment Comment
	err = s.write(ctx, by, func(w *writeTx) error {
		id, err := importedTarget(w, source, input.ExternalID)
		if err != nil {
			return err
		}
		if id != "" {
			comment, err = getComment(w.ctx, w.tx, id)
			return err
		}

		projectID, issueID, err := commentTarget(w, input.IssueID, "")
		if err != nil {
			return err
		}
		if comment, err = insertComment(w, projectID, issueID, NewComment{IssueID: issueID, Body: body}); err != nil {
			return err
		}
		return linkImport(w, source, input.ExternalID, TargetComment, comment.ID)
	})

	return comment, err
}

// ImportedReviewRound is a finished review round to import with its findings, numbered as they
// were at the source.
type ImportedReviewRound struct {
	ExternalID          string
	IssueID             string
	Number              int
	FromCommit          string
	ToCommit            string
	IncludesUncommitted bool
	PlanCoverage        string
	Summary             string
	Findings            []ImportedFinding
}

// ImportedFinding is one finding of an imported round.
type ImportedFinding struct {
	Number         int
	Severity       Severity
	Title          string
	Problem        string
	Fix            string
	Evidence       string
	FilePath       string
	Line           int
	CommitSHA      string
	Status         FindingStatus
	DecisionReason string
}

// importedWontfixReason stands in for a wontfix decision whose reason the source never gave.
const importedWontfixReason = "Imported without a reason."

// ImportReviewRound imports a finished round and its findings in one transaction. Decisions,
// fixes, and verifications the source records are attributed to the caller. A later decision
// creates no follow-up issue, since the source tracked its follow-up separately.
func (s *Store) ImportReviewRound(ctx context.Context, by Caller, source ImportSource, input ImportedReviewRound) (ReviewRound, error) {
	if input.Number < 1 {
		return ReviewRound{}, invalid("number", "must be at least 1")
	}
	fromCommit, err := optionalCommit("from_commit", input.FromCommit)
	if err != nil {
		return ReviewRound{}, err
	}
	toCommit, err := requiredCommit("to_commit", input.ToCommit)
	if err != nil {
		return ReviewRound{}, err
	}
	coverage, err := optionalText("plan_coverage", input.PlanCoverage, maxPlanCoverage)
	if err != nil {
		return ReviewRound{}, err
	}
	summary, err := optionalText("summary", input.Summary, maxTextLength)
	if err != nil {
		return ReviewRound{}, err
	}
	findings, err := validateImportedFindings(input.Findings)
	if err != nil {
		return ReviewRound{}, err
	}

	var round ReviewRound
	err = s.write(ctx, by, func(w *writeTx) error {
		id, err := importedTarget(w, source, input.ExternalID)
		if err != nil {
			return err
		}
		if id != "" {
			round, err = getRound(w.ctx, w.tx, `id = ?`, id)
			return err
		}

		issue, err := getIssue(w.ctx, w.tx, `i.id = ?`, input.IssueID)
		if err != nil {
			return err
		}

		round = ReviewRound{
			ID: newID(), IssueID: issue.ID, Number: input.Number, FromCommit: fromCommit, ToCommit: toCommit,
			IncludesUncommitted: input.IncludesUncommitted, PlanCoverage: coverage, Summary: summary,
			AuthorActorID: w.caller.ID, FinishedAt: &w.now, CreatedAt: w.now, UpdatedAt: w.now,
		}
		nextFinding := 1
		for _, finding := range findings {
			nextFinding = max(nextFinding, finding.Number+1)
		}

		_, err = w.exec(`
			INSERT INTO review_rounds (id, issue_id, number, from_commit, to_commit, includes_uncommitted,
				plan_coverage, summary, next_finding_number, author_actor_id, finished_at, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			round.ID, issue.ID, round.Number, nullString(fromCommit), toCommit, input.IncludesUncommitted,
			coverage, summary, nextFinding, w.caller.ID, w.timestamp(), w.timestamp(), w.timestamp())
		if err != nil {
			return mapUniqueViolation(err, fmt.Sprintf("%s already has review round %d, so it was not imported", issue.Key(), round.Number), "import review round")
		}
		_, err = w.exec(`UPDATE issues SET next_review_round_number = max(next_review_round_number, ?) WHERE id = ?`,
			round.Number+1, issue.ID)
		if err != nil {
			return fmt.Errorf("move round counter: %w", err)
		}

		for _, finding := range findings {
			if err := insertImportedFinding(w, round.ID, finding); err != nil {
				return err
			}
		}

		err = w.event(issue.ProjectID, TargetReviewRound, round.ID, "review_round.imported", map[string]any{
			"source": source, "external_id": input.ExternalID, "issue_id": issue.ID, "number": round.Number,
			"findings": len(findings),
		})
		if err != nil {
			return err
		}
		return linkImport(w, source, input.ExternalID, TargetReviewRound, round.ID)
	})

	return round, err
}

// validateImportedFindings checks each finding like a new one, plus its number and status, and
// refuses two findings with one number.
func validateImportedFindings(inputs []ImportedFinding) ([]ImportedFinding, error) {
	seen := map[int]bool{}
	findings := make([]ImportedFinding, 0, len(inputs))
	for _, input := range inputs {
		checked, err := validateNewFinding(NewFinding{
			Severity: input.Severity, Title: input.Title, Problem: input.Problem, Fix: input.Fix,
			Evidence: input.Evidence, FilePath: input.FilePath, Line: input.Line, CommitSHA: input.CommitSHA,
		})
		if err != nil {
			return nil, err
		}
		if input.Number < 1 || seen[input.Number] {
			return nil, invalid("findings", "need distinct numbers of at least 1")
		}
		if !input.Status.valid() {
			return nil, invalid("status", "must be open, fix, wontfix, later, fixed, or verified")
		}
		seen[input.Number] = true

		reason, err := optionalText("reason", input.DecisionReason, maxTextLength)
		if err != nil {
			return nil, err
		}
		if input.Status == FindingWontfix && reason == "" {
			reason = importedWontfixReason
		}

		findings = append(findings, ImportedFinding{
			Number: input.Number, Severity: checked.Severity, Title: checked.Title, Problem: checked.Problem,
			Fix: checked.Fix, Evidence: checked.Evidence, FilePath: checked.FilePath, Line: checked.Line,
			CommitSHA: checked.CommitSHA, Status: input.Status, DecisionReason: reason,
		})
	}
	return findings, nil
}

// insertImportedFinding inserts a finding in its imported status, filling in the actor and time
// that each status needs.
func insertImportedFinding(w *writeTx, roundID string, finding ImportedFinding) error {
	now := sql.NullString{String: w.timestamp(), Valid: true}
	caller := sql.NullString{String: w.caller.ID, Valid: true}
	var decidedBy, decidedAt, fixedBy, fixedAt, verifiedBy, verifiedAt sql.NullString

	if finding.Status != FindingOpen {
		decidedBy, decidedAt = caller, now
	}
	if finding.Status == FindingFixed || finding.Status == FindingVerified {
		fixedBy, fixedAt = caller, now
	}
	if finding.Status == FindingVerified {
		verifiedBy, verifiedAt = caller, now
	}

	_, err := w.exec(`
		INSERT INTO findings (id, review_round_id, number, severity, title, problem, fix, evidence, file_path, line,
			commit_sha, status, decision_reason, decided_by_actor_id, decided_at, found_by_actor_id,
			fixed_by_actor_id, fixed_at, verified_by_actor_id, verified_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		newID(), roundID, finding.Number, finding.Severity, finding.Title, finding.Problem, finding.Fix,
		finding.Evidence, nullString(finding.FilePath), nullInt(finding.Line), nullString(finding.CommitSHA),
		finding.Status, nullString(finding.DecisionReason), decidedBy, decidedAt, w.caller.ID,
		fixedBy, fixedAt, verifiedBy, verifiedAt, w.timestamp(), w.timestamp())
	if err != nil {
		return fmt.Errorf("import finding R%d: %w", finding.Number, err)
	}
	return nil
}

// importedTarget returns the ID of the record imported from the source ID, or "" when there is none.
func importedTarget(w *writeTx, source ImportSource, externalID string) (string, error) {
	if externalID == "" {
		return "", invalid("external_id", "is required")
	}

	var id string
	err := w.tx.QueryRowContext(w.ctx, `SELECT target_id FROM import_links WHERE source = ? AND external_id = ?`,
		source, externalID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("find imported record: %w", err)
	}
	return id, nil
}

func linkImport(w *writeTx, source ImportSource, externalID string, target TargetType, targetID string) error {
	_, err := w.exec(`
		INSERT INTO import_links (id, source, external_id, target_type, target_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		newID(), source, externalID, target, targetID, w.timestamp(), w.timestamp())
	if err != nil {
		return fmt.Errorf("record import link: %w", err)
	}
	return nil
}
