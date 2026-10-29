package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
)

const roundColumns = `id, issue_id, number, from_commit, to_commit, includes_uncommitted, plan_coverage, summary,
	author_actor_id, finished_at, created_at, updated_at`

const findingColumns = `f.id, f.review_round_id, r.issue_id, r.number, f.number, f.severity, f.title, f.problem, f.fix,
	f.evidence, f.file_path, f.line, f.commit_sha, f.status, f.decision_reason, f.decided_by_actor_id, f.decided_at,
	f.found_by_actor_id, f.fixed_by_actor_id, f.fixed_in_commit, f.fixed_at, f.verified_by_actor_id, f.verified_at,
	f.duplicate_of_finding_id, f.follow_up_issue_id, f.created_at, f.updated_at, p.key || '-' || i.number,
	(SELECT count(*) FROM comments c WHERE c.finding_id = f.id)`

const findingFrom = ` FROM findings f JOIN review_rounds r ON r.id = f.review_round_id
	JOIN issues i ON i.id = r.issue_id JOIN projects p ON p.id = i.project_id `

// UnresolvedFindingStatuses are the finding statuses that still need work: waiting for a
// decision, decided fix, or fixed and waiting for verification. Issue.UnresolvedFindings counts
// findings in them.
var UnresolvedFindingStatuses = []FindingStatus{FindingOpen, FindingFix, FindingFixed}

// findingOrder follows ORDER BY: findings by round, most severe first, then by number.
const findingOrder = ` r.number, CASE f.severity WHEN 'blocking' THEN 0 WHEN 'should_fix' THEN 1 ELSE 2 END, f.number`

// NewReviewRound describes a review round to start on an issue.
type NewReviewRound struct {
	IssueID string
	// FromCommit is where the reviewed range starts. When empty, a later round starts from the
	// previous round's ToCommit, and a first round reviews the whole branch.
	FromCommit          string
	ToCommit            string
	IncludesUncommitted bool
	PlanCoverage        string
}

// NewFinding describes a finding to add to a round in progress.
type NewFinding struct {
	ReviewRoundID string
	Severity      Severity
	Title         string
	Problem       string
	Fix           string
	Evidence      string
	FilePath      string
	Line          int
	CommitSHA     string
}

// FindingChange moves a finding to a new status.
type FindingChange struct {
	Status FindingStatus
	// Reason explains a decision. It is required for wontfix.
	Reason string
	// FixedInCommit is the commit that fixed the finding, when moving it to fixed.
	FixedInCommit string
}

// StartReviewRound starts the issue's next review round. An issue has at most one round in
// progress. People and agents can start rounds.
func (s *Store) StartReviewRound(ctx context.Context, by Caller, input NewReviewRound) (ReviewRound, error) {
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

	var round ReviewRound
	err = s.write(ctx, by, func(w *writeTx) error {
		issue, err := getIssue(w.ctx, w.tx, `i.id = ?`, input.IssueID)
		if err != nil {
			return err
		}

		var open int
		err = w.tx.QueryRowContext(w.ctx,
			`SELECT count(*) FROM review_rounds WHERE issue_id = ? AND finished_at IS NULL`, issue.ID).Scan(&open)
		if err != nil {
			return fmt.Errorf("check rounds in progress: %w", err)
		}
		if open > 0 {
			return conflict(issue.Key() + " already has a review round in progress")
		}

		if fromCommit == "" {
			err := w.tx.QueryRowContext(w.ctx,
				`SELECT to_commit FROM review_rounds WHERE issue_id = ? ORDER BY number DESC LIMIT 1`, issue.ID).Scan(&fromCommit)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("find previous round: %w", err)
			}
		}

		var number int
		err = w.tx.QueryRowContext(w.ctx, `
			UPDATE issues SET next_review_round_number = next_review_round_number + 1
			WHERE id = ? RETURNING next_review_round_number - 1`, issue.ID).Scan(&number)
		if err != nil {
			return fmt.Errorf("take round number: %w", err)
		}

		round = ReviewRound{
			ID: newID(), IssueID: issue.ID, Number: number, FromCommit: fromCommit, ToCommit: toCommit,
			IncludesUncommitted: input.IncludesUncommitted, PlanCoverage: coverage,
			AuthorActorID: w.caller.ID, CreatedAt: w.now, UpdatedAt: w.now,
		}
		_, err = w.exec(`
			INSERT INTO review_rounds (id, issue_id, number, from_commit, to_commit, includes_uncommitted,
				plan_coverage, summary, author_actor_id, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, '', ?, ?, ?)`,
			round.ID, issue.ID, number, nullString(fromCommit), toCommit, input.IncludesUncommitted,
			coverage, w.caller.ID, w.timestamp(), w.timestamp())
		if err != nil {
			return mapUniqueViolation(err, issue.Key()+" already has a review round in progress", "start review round")
		}

		return w.event(issue.ProjectID, TargetReviewRound, round.ID, "review_round.started", map[string]any{
			"issue_id": issue.ID, "number": number, "from_commit": fromCommit, "to_commit": toCommit,
			"includes_uncommitted": input.IncludesUncommitted,
		})
	})

	return round, err
}

// FinishReviewRound closes a round with its summary. After that, no findings can be added to
// it. The round's author or any person can finish it.
func (s *Store) FinishReviewRound(ctx context.Context, by Caller, roundID, summary string) (ReviewRound, error) {
	summary, err := optionalText("summary", summary, maxTextLength)
	if err != nil {
		return ReviewRound{}, err
	}

	var round ReviewRound
	err = s.write(ctx, by, func(w *writeTx) error {
		current, err := getRound(w.ctx, w.tx, `id = ?`, roundID)
		if err != nil {
			return err
		}
		if current.FinishedAt != nil {
			return Refuse(ErrInvalidTransition, "round %d is already finished", current.Number)
		}
		if w.caller.ID != current.AuthorActorID && w.caller.Kind != ActorHuman {
			return forbidden("only the round's author or a person can finish it")
		}

		_, err = w.exec(`UPDATE review_rounds SET summary = ?, finished_at = ?, updated_at = ? WHERE id = ?`,
			summary, w.timestamp(), w.timestamp(), current.ID)
		if err != nil {
			return fmt.Errorf("finish review round: %w", err)
		}

		round = current
		round.Summary, round.FinishedAt, round.UpdatedAt = summary, &w.now, w.now

		projectID, err := issueProjectID(w, current.IssueID)
		if err != nil {
			return err
		}
		return w.event(projectID, TargetReviewRound, current.ID, "review_round.finished",
			map[string]any{"issue_id": current.IssueID, "number": current.Number})
	})

	return round, err
}

// GetReviewRound returns the review round with the given ID.
func (s *Store) GetReviewRound(ctx context.Context, id string) (ReviewRound, error) {
	return getRound(ctx, s.db, `id = ?`, id)
}

// GetReviewRoundByNumber returns an issue's review round by its number, as in /WP-142/reviews/2.
func (s *Store) GetReviewRoundByNumber(ctx context.Context, issueID string, number int) (ReviewRound, error) {
	return getRound(ctx, s.db, `issue_id = ? AND number = ?`, issueID, number)
}

// GetOpenReviewRound returns the issue's review round in progress, or ErrNotFound when none is.
func (s *Store) GetOpenReviewRound(ctx context.Context, issueID string) (ReviewRound, error) {
	return getRound(ctx, s.db, `issue_id = ? AND finished_at IS NULL`, issueID)
}

// ListReviewRounds returns an issue's review rounds in order.
func (s *Store) ListReviewRounds(ctx context.Context, issueID string) ([]ReviewRound, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+roundColumns+` FROM review_rounds WHERE issue_id = ? ORDER BY number`, issueID)
	if err != nil {
		return nil, fmt.Errorf("list review rounds: %w", err)
	}
	defer rows.Close()

	var rounds []ReviewRound
	for rows.Next() {
		round, err := scanRound(rows)
		if err != nil {
			return nil, err
		}
		rounds = append(rounds, round)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list review rounds: %w", err)
	}
	return rounds, nil
}

func getRound(ctx context.Context, q querier, where string, args ...any) (ReviewRound, error) {
	return scanRound(q.QueryRowContext(ctx, `SELECT `+roundColumns+` FROM review_rounds WHERE `+where, args...))
}

func scanRound(row scanner) (ReviewRound, error) {
	var round ReviewRound
	var fromCommit, finished sql.NullString
	var created, updated string

	err := row.Scan(&round.ID, &round.IssueID, &round.Number, &fromCommit, &round.ToCommit, &round.IncludesUncommitted,
		&round.PlanCoverage, &round.Summary, &round.AuthorActorID, &finished, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return ReviewRound{}, notFound("review round")
	}
	if err != nil {
		return ReviewRound{}, fmt.Errorf("read review round: %w", err)
	}

	round.FromCommit = fromCommit.String
	if round.FinishedAt, err = parseOptionalTime(finished); err != nil {
		return ReviewRound{}, err
	}
	if round.CreatedAt, round.UpdatedAt, err = timestamps(created, updated); err != nil {
		return ReviewRound{}, err
	}
	return round, nil
}

func issueProjectID(w *writeTx, issueID string) (string, error) {
	var projectID string
	if err := w.tx.QueryRowContext(w.ctx, `SELECT project_id FROM issues WHERE id = ?`, issueID).Scan(&projectID); err != nil {
		return "", fmt.Errorf("find issue project: %w", err)
	}
	return projectID, nil
}

// AddFinding adds a finding with the round's next number to a round in progress. The caller is
// recorded as the finding's reviewer. People and agents can add findings.
func (s *Store) AddFinding(ctx context.Context, by Caller, input NewFinding) (Finding, error) {
	finding, err := validateNewFinding(input)
	if err != nil {
		return Finding{}, err
	}

	err = s.write(ctx, by, func(w *writeTx) error {
		round, err := getRound(w.ctx, w.tx, `id = ?`, input.ReviewRoundID)
		if err != nil {
			return err
		}
		if round.FinishedAt != nil {
			return Refuse(ErrInvalidTransition, "round %d is finished, so it takes no new findings", round.Number)
		}

		err = w.tx.QueryRowContext(w.ctx, `
			UPDATE review_rounds SET next_finding_number = next_finding_number + 1, updated_at = ?
			WHERE id = ? RETURNING next_finding_number - 1`, w.timestamp(), round.ID).Scan(&finding.Number)
		if err != nil {
			return fmt.Errorf("take finding number: %w", err)
		}

		finding.ID, finding.IssueID, finding.RoundNumber = newID(), round.IssueID, round.Number
		finding.Status, finding.FoundByActorID = FindingOpen, w.caller.ID
		finding.CreatedAt, finding.UpdatedAt = w.now, w.now
		_, err = w.exec(`
			INSERT INTO findings (id, review_round_id, number, severity, title, problem, fix, evidence, file_path, line,
				commit_sha, status, found_by_actor_id, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'open', ?, ?, ?)`,
			finding.ID, round.ID, finding.Number, finding.Severity, finding.Title, finding.Problem, finding.Fix,
			finding.Evidence, nullString(finding.FilePath), nullInt(finding.Line), nullString(finding.CommitSHA),
			w.caller.ID, w.timestamp(), w.timestamp())
		if err != nil {
			return fmt.Errorf("add finding: %w", err)
		}

		projectID, err := issueProjectID(w, round.IssueID)
		if err != nil {
			return err
		}
		return w.event(projectID, TargetFinding, finding.ID, "finding.added", map[string]any{
			"issue_id": round.IssueID, "round": round.Number, "number": finding.Number,
			"severity": finding.Severity, "title": finding.Title,
		})
	})

	return finding, err
}

func validateNewFinding(input NewFinding) (Finding, error) {
	var finding Finding
	var err error

	if !input.Severity.valid() {
		return finding, invalid("severity", "must be blocking, should_fix, or nit")
	}
	if finding.Title, err = requiredText("title", input.Title, maxTitleLength); err != nil {
		return finding, err
	}
	if finding.Problem, err = requiredText("problem", input.Problem, maxTextLength); err != nil {
		return finding, err
	}
	if finding.Fix, err = requiredText("fix", input.Fix, maxTextLength); err != nil {
		return finding, err
	}
	if finding.Evidence, err = optionalText("evidence", input.Evidence, maxTextLength); err != nil {
		return finding, err
	}
	if finding.FilePath, err = optionalText("file_path", input.FilePath, maxPathLength); err != nil {
		return finding, err
	}
	if input.Line < 0 {
		return finding, invalid("line", "must be a positive line number")
	}
	if input.Line > 0 && finding.FilePath == "" {
		return finding, invalid("line", "needs a file_path")
	}
	if finding.CommitSHA, err = optionalCommit("commit_sha", input.CommitSHA); err != nil {
		return finding, err
	}

	finding.ReviewRoundID, finding.Severity, finding.Line = input.ReviewRoundID, input.Severity, input.Line
	return finding, nil
}

// findingTransitions lists, for each status, the statuses a finding can move to next.
var findingTransitions = map[FindingStatus][]FindingStatus{
	FindingOpen:     {FindingFix, FindingWontfix, FindingLater},
	FindingFix:      {FindingOpen, FindingFixed},
	FindingWontfix:  {FindingOpen},
	FindingLater:    {FindingOpen},
	FindingFixed:    {FindingVerified, FindingFix},
	FindingVerified: {},
}

// isDecision reports whether moving from one status to another is a decision or its withdrawal,
// which only a person may make.
func isDecision(from, to FindingStatus) bool {
	switch to {
	case FindingFix:
		return from == FindingOpen
	case FindingWontfix, FindingLater, FindingOpen:
		return true
	}
	return false
}

// ChangeFindingStatus moves a finding along its lifecycle:
//
//	open → fix, wontfix, or later    (a person decides)
//	fix, wontfix, later → open       (a person withdraws the decision)
//	fix → fixed                      (whoever fixed it)
//	fixed → verified                 (whoever checked the fix)
//	fixed → fix                      (the fix did not hold)
//
// Deciding later creates a follow-up issue in the project's backlog, or reuses the one an
// earlier later decision created, and links it to the finding.
func (s *Store) ChangeFindingStatus(ctx context.Context, by Caller, findingID string, change FindingChange) (Finding, error) {
	change, err := validateFindingChange(change)
	if err != nil {
		return Finding{}, err
	}

	var finding Finding
	err = s.write(ctx, by, func(w *writeTx) error {
		current, err := getFinding(w.ctx, w.tx, `f.id = ?`, findingID)
		if err != nil {
			return err
		}

		finding, err = changeFindingStatus(w, current, change)
		return err
	})

	return finding, err
}

// validateFindingChange checks a status change and returns it with its text trimmed.
func validateFindingChange(change FindingChange) (FindingChange, error) {
	if !change.Status.valid() {
		return change, invalid("status", "must be open, fix, wontfix, later, fixed, or verified")
	}

	var err error
	if change.Reason, err = optionalText("reason", change.Reason, maxTextLength); err != nil {
		return change, err
	}
	if change.Status == FindingWontfix && change.Reason == "" {
		return change, invalid("reason", "is required for wontfix")
	}
	if change.FixedInCommit, err = optionalCommit("fixed_in_commit", change.FixedInCommit); err != nil {
		return change, err
	}
	return change, nil
}

// changeFindingStatus applies a validated change to current, checking that the move is allowed
// and that the caller may make it, and records the event.
func changeFindingStatus(w *writeTx, current Finding, change FindingChange) (Finding, error) {
	if current.Status == change.Status {
		return current, nil
	}

	allowed := findingTransitions[current.Status]
	if !slices.Contains(allowed, change.Status) {
		return Finding{}, Refuse(ErrInvalidTransition, "finding %s of round %d cannot move from %s to %s",
			current.Label(), current.RoundNumber, current.Status, change.Status)
	}
	if isDecision(current.Status, change.Status) {
		if err := w.requireHuman("decide on findings"); err != nil {
			return Finding{}, err
		}
	}

	finding, err := applyFindingChange(w, current, change.Status, change.Reason, change.FixedInCommit)
	if err != nil {
		return Finding{}, err
	}

	projectID, err := issueProjectID(w, current.IssueID)
	if err != nil {
		return Finding{}, err
	}
	err = w.event(projectID, TargetFinding, current.ID, "finding.status_changed", map[string]any{
		"issue_id": current.IssueID, "round": current.RoundNumber, "number": current.Number,
		"from": current.Status, "to": change.Status, "reason": change.Reason, "fixed_in_commit": change.FixedInCommit,
		"follow_up_issue_id": finding.FollowUpIssueID,
	})
	return finding, err
}

// FixSummary reports the fix an agent or person made on an issue: a comment describing it, and
// the findings it fixed, which move from fix to fixed in the given commit.
type FixSummary struct {
	IssueID    string
	Summary    string
	Commit     string
	FindingIDs []string
}

// PostFixSummary marks the summary's findings fixed and adds the summary as a comment on the
// issue, all in one transaction, so a refused finding leaves nothing half-reported. People and
// agents can post fix summaries.
func (s *Store) PostFixSummary(ctx context.Context, by Caller, input FixSummary) (Comment, []Finding, error) {
	body, err := requiredText("summary", input.Summary, maxCommentLength)
	if err != nil {
		return Comment{}, nil, err
	}
	commit, err := requiredCommit("commit", input.Commit)
	if err != nil {
		return Comment{}, nil, err
	}
	findingIDs := uniqueSorted(input.FindingIDs)
	if len(findingIDs) == 0 {
		return Comment{}, nil, invalid("findings", "must list at least one finding; post a comment for a summary that fixes none")
	}

	var comment Comment
	var fixed []Finding
	err = s.write(ctx, by, func(w *writeTx) error {
		issue, err := getIssue(w.ctx, w.tx, `i.id = ?`, input.IssueID)
		if err != nil {
			return err
		}

		for _, findingID := range findingIDs {
			current, err := getFinding(w.ctx, w.tx, `f.id = ?`, findingID)
			if errors.Is(err, ErrNotFound) || (err == nil && current.IssueID != issue.ID) {
				return invalid("findings", "must all be findings of %s", issue.Key())
			}
			if err != nil {
				return err
			}

			finding, err := changeFindingStatus(w, current, FindingChange{Status: FindingFixed, FixedInCommit: commit})
			if err != nil {
				return err
			}
			fixed = append(fixed, finding)
		}

		comment, err = insertComment(w, issue.ProjectID, issue.ID, NewComment{IssueID: issue.ID, Body: body})
		return err
	})

	return comment, fixed, err
}

// applyFindingChange writes the new status and the fields that go with it.
func applyFindingChange(w *writeTx, current Finding, to FindingStatus, reason, fixedIn string) (Finding, error) {
	next := current
	next.Status, next.UpdatedAt = to, w.now

	switch to {
	case FindingOpen:
		next.DecisionReason, next.DecidedByActorID, next.DecidedAt = "", "", nil
	case FindingFix, FindingWontfix, FindingLater:
		if current.Status == FindingOpen {
			next.DecisionReason, next.DecidedByActorID, next.DecidedAt = reason, w.caller.ID, &w.now
		}
		// A fix that did not hold goes back to fix, so its fix details no longer apply.
		next.FixedByActorID, next.FixedInCommit, next.FixedAt = "", "", nil
	case FindingFixed:
		next.FixedByActorID, next.FixedInCommit, next.FixedAt = w.caller.ID, fixedIn, &w.now
	case FindingVerified:
		next.VerifiedByActorID, next.VerifiedAt = w.caller.ID, &w.now
	}

	if to == FindingLater && current.FollowUpIssueID == "" {
		followUp, err := createFollowUpIssue(w, current)
		if err != nil {
			return Finding{}, err
		}
		next.FollowUpIssueID = followUp.ID
	}

	_, err := w.exec(`
		UPDATE findings SET status = ?, decision_reason = ?, decided_by_actor_id = ?, decided_at = ?,
			fixed_by_actor_id = ?, fixed_in_commit = ?, fixed_at = ?, verified_by_actor_id = ?, verified_at = ?,
			follow_up_issue_id = ?, updated_at = ?
		WHERE id = ?`,
		next.Status, nullString(next.DecisionReason), nullString(next.DecidedByActorID), formatOptionalTime(next.DecidedAt),
		nullString(next.FixedByActorID), nullString(next.FixedInCommit), formatOptionalTime(next.FixedAt),
		nullString(next.VerifiedByActorID), formatOptionalTime(next.VerifiedAt), nullString(next.FollowUpIssueID),
		w.timestamp(), current.ID)
	if err != nil {
		return Finding{}, fmt.Errorf("change finding status: %w", err)
	}

	return next, nil
}

// createFollowUpIssue creates the backlog issue a later decision asks for, in the finding's
// project, with the finding's text as its plan.
func createFollowUpIssue(w *writeTx, finding Finding) (Issue, error) {
	source, err := getIssue(w.ctx, w.tx, `i.id = ?`, finding.IssueID)
	if err != nil {
		return Issue{}, err
	}
	project, err := getProject(w.ctx, w.tx, `id = ?`, source.ProjectID)
	if err != nil {
		return Issue{}, err
	}

	title := finding.Title
	if runes := []rune(title); len(runes) > maxTitleLength {
		title = string(runes[:maxTitleLength])
	}

	var plan strings.Builder
	fmt.Fprintf(&plan, "Follow-up from %s, review round %d, %s.\n\n", source.Key(), finding.RoundNumber, finding.Label())
	fmt.Fprintf(&plan, "**Problem:** %s\n\n**Fix:** %s\n", finding.Problem, finding.Fix)
	if finding.Evidence != "" {
		fmt.Fprintf(&plan, "\n**Evidence:** %s\n", finding.Evidence)
	}

	return insertIssue(w, project, issueFields{title: title, plan: plan.String(), status: IssueBacklog, labelIDs: []string{}})
}

// MarkDuplicate marks a finding as a duplicate of another finding on the same issue, or clears
// the mark when duplicateOfID is empty. A finding that others duplicate cannot itself become a
// duplicate, so duplicates never chain. People and agents can mark duplicates.
func (s *Store) MarkDuplicate(ctx context.Context, by Caller, findingID, duplicateOfID string) error {
	return s.write(ctx, by, func(w *writeTx) error {
		finding, err := getFinding(w.ctx, w.tx, `f.id = ?`, findingID)
		if err != nil {
			return err
		}
		if finding.DuplicateOfFindingID == duplicateOfID {
			return nil
		}

		if duplicateOfID != "" {
			if err := checkDuplicateTarget(w, finding, duplicateOfID); err != nil {
				return err
			}
		}

		_, err = w.exec(`UPDATE findings SET duplicate_of_finding_id = ?, updated_at = ? WHERE id = ?`,
			nullString(duplicateOfID), w.timestamp(), finding.ID)
		if err != nil {
			return fmt.Errorf("mark duplicate: %w", err)
		}

		projectID, err := issueProjectID(w, finding.IssueID)
		if err != nil {
			return err
		}
		return w.event(projectID, TargetFinding, finding.ID, "finding.duplicate_marked", map[string]any{
			"issue_id": finding.IssueID, "round": finding.RoundNumber, "number": finding.Number,
			"from": finding.DuplicateOfFindingID, "to": duplicateOfID,
		})
	})
}

func checkDuplicateTarget(w *writeTx, finding Finding, duplicateOfID string) error {
	if duplicateOfID == finding.ID {
		return invalid("duplicate_of_finding_id", "a finding cannot duplicate itself")
	}

	target, err := getFinding(w.ctx, w.tx, `f.id = ?`, duplicateOfID)
	if errors.Is(err, ErrNotFound) {
		return invalid("duplicate_of_finding_id", "must be a finding on the same issue")
	}
	if err != nil {
		return err
	}
	if target.IssueID != finding.IssueID {
		return invalid("duplicate_of_finding_id", "must be a finding on the same issue")
	}
	if target.DuplicateOfFindingID != "" {
		return invalid("duplicate_of_finding_id", "is itself a duplicate; mark it as a duplicate of the original instead")
	}

	var dependents int
	err = w.tx.QueryRowContext(w.ctx, `SELECT count(*) FROM findings WHERE duplicate_of_finding_id = ?`, finding.ID).Scan(&dependents)
	if err != nil {
		return fmt.Errorf("check duplicates: %w", err)
	}
	if dependents > 0 {
		return invalid("finding_id", "other findings duplicate this one, so it cannot become a duplicate")
	}

	return nil
}

// GetFinding returns the finding with the given ID.
func (s *Store) GetFinding(ctx context.Context, id string) (Finding, error) {
	return getFinding(ctx, s.db, `f.id = ?`, id)
}

// ParseFindingLabel reads a finding's label within its round, such as R3, r3, or 3. field names
// the input it came from, for the error.
func ParseFindingLabel(field, value string) (int, error) {
	value = strings.TrimSpace(value)
	if len(value) > 0 && (value[0] == 'R' || value[0] == 'r') {
		value = value[1:]
	}

	number, err := strconv.Atoi(value)
	if err != nil || number < 1 {
		return 0, invalid(field, "must be a finding label such as R3")
	}
	return number, nil
}

// GetFindingByNumber returns a finding by its issue, round number, and number within the round,
// as in finding R3 of round 2.
func (s *Store) GetFindingByNumber(ctx context.Context, issueID string, round, number int) (Finding, error) {
	return getFinding(ctx, s.db, `r.issue_id = ? AND r.number = ? AND f.number = ?`, issueID, round, number)
}

// ListRoundFindings returns a round's findings, most severe first.
func (s *Store) ListRoundFindings(ctx context.Context, roundID string) ([]Finding, error) {
	return listFindings(ctx, s.db, `f.review_round_id = ? ORDER BY`+findingOrder, roundID)
}

// ListIssueFindings returns the findings of every round of an issue whose status is one of
// statuses, or all of them when statuses is empty. Duplicates are left out when excludeDuplicates
// is true, as the list of open findings across rounds needs.
func (s *Store) ListIssueFindings(ctx context.Context, issueID string, statuses []FindingStatus, excludeDuplicates bool) ([]Finding, error) {
	where, args, err := findingFilter(`r.issue_id = ?`, []any{issueID}, statuses, excludeDuplicates)
	if err != nil {
		return nil, err
	}
	return listFindings(ctx, s.db, where+` ORDER BY`+findingOrder, args...)
}

// ListProjectFindings returns the findings of every issue in a project, filtered like
// ListIssueFindings and grouped by issue, newest issue first, as the project's open findings
// list shows them.
func (s *Store) ListProjectFindings(ctx context.Context, projectID string, statuses []FindingStatus, excludeDuplicates bool) ([]Finding, error) {
	where, args, err := findingFilter(`i.project_id = ?`, []any{projectID}, statuses, excludeDuplicates)
	if err != nil {
		return nil, err
	}
	return listFindings(ctx, s.db, where+` ORDER BY i.number DESC,`+findingOrder, args...)
}

// findingFilter adds the status and duplicate filters to where and its args.
func findingFilter(where string, args []any, statuses []FindingStatus, excludeDuplicates bool) (string, []any, error) {
	if len(statuses) > 0 {
		placeholders := make([]string, len(statuses))
		for index, status := range statuses {
			if !status.valid() {
				return "", nil, invalid("statuses", "contains an unknown status %q", status)
			}
			placeholders[index] = "?"
			args = append(args, status)
		}
		where += ` AND f.status IN (` + strings.Join(placeholders, ", ") + `)`
	}
	if excludeDuplicates {
		where += ` AND f.duplicate_of_finding_id IS NULL`
	}
	return where, args, nil
}

func listFindings(ctx context.Context, q querier, where string, args ...any) ([]Finding, error) {
	rows, err := q.QueryContext(ctx, `SELECT `+findingColumns+findingFrom+`WHERE `+where, args...)
	if err != nil {
		return nil, fmt.Errorf("list findings: %w", err)
	}
	defer rows.Close()

	var findings []Finding
	for rows.Next() {
		finding, err := scanFinding(rows)
		if err != nil {
			return nil, err
		}
		findings = append(findings, finding)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list findings: %w", err)
	}
	return findings, nil
}

func getFinding(ctx context.Context, q querier, where string, args ...any) (Finding, error) {
	return scanFinding(q.QueryRowContext(ctx, `SELECT `+findingColumns+findingFrom+`WHERE `+where, args...))
}

func scanFinding(row scanner) (Finding, error) {
	var f Finding
	var filePath, commit, reason, decidedBy, decidedAt, fixedBy, fixedIn, fixedAt sql.NullString
	var verifiedBy, verifiedAt, duplicateOf, followUp sql.NullString
	var line sql.NullInt64
	var created, updated string

	err := row.Scan(&f.ID, &f.ReviewRoundID, &f.IssueID, &f.RoundNumber, &f.Number, &f.Severity, &f.Title, &f.Problem,
		&f.Fix, &f.Evidence, &filePath, &line, &commit, &f.Status, &reason, &decidedBy, &decidedAt,
		&f.FoundByActorID, &fixedBy, &fixedIn, &fixedAt, &verifiedBy, &verifiedAt, &duplicateOf, &followUp,
		&created, &updated, &f.IssueKey, &f.CommentCount)
	if errors.Is(err, sql.ErrNoRows) {
		return Finding{}, notFound("finding")
	}
	if err != nil {
		return Finding{}, fmt.Errorf("read finding: %w", err)
	}

	f.FilePath, f.Line, f.CommitSHA = filePath.String, int(line.Int64), commit.String
	f.DecisionReason, f.DecidedByActorID = reason.String, decidedBy.String
	f.FixedByActorID, f.FixedInCommit, f.VerifiedByActorID = fixedBy.String, fixedIn.String, verifiedBy.String
	f.DuplicateOfFindingID, f.FollowUpIssueID = duplicateOf.String, followUp.String

	for _, field := range []struct {
		target **time.Time
		value  sql.NullString
	}{{&f.DecidedAt, decidedAt}, {&f.FixedAt, fixedAt}, {&f.VerifiedAt, verifiedAt}} {
		if *field.target, err = parseOptionalTime(field.value); err != nil {
			return Finding{}, err
		}
	}
	if f.CreatedAt, f.UpdatedAt, err = timestamps(created, updated); err != nil {
		return Finding{}, err
	}

	return f, nil
}

func formatOptionalTime(value *time.Time) sql.NullString {
	if value == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: value.UTC().Format(timeLayout), Valid: true}
}
