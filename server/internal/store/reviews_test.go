package store

import (
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
)

// reviewed is an issue with one round in progress and one open finding in it.
type reviewed struct {
	issue   Issue
	round   ReviewRound
	finding Finding
}

func (tm *team) reviewed(t *testing.T) reviewed {
	t.Helper()
	return tm.reviewedIn(t, "WP")
}

func (tm *team) reviewedIn(t *testing.T, projectKey string) reviewed {
	t.Helper()
	project := tm.project(t, projectKey)
	issue := tm.issue(t, project.ID, "Meta comments")
	round := got(tm.store.StartReviewRound(tm.ctx, tm.agent, NewReviewRound{IssueID: issue.ID, ToCommit: "a1b2c3d"})).must(t)
	finding := got(tm.store.AddFinding(tm.ctx, tm.agent, NewFinding{
		ReviewRoundID: round.ID, Severity: SeverityBlocking, Title: "Text can arrive before its file",
		Problem: "The worker sends whichever row it claims first.", Fix: "Send both from one job.",
		Evidence: "Two replicas claim both rows.", FilePath: "apps/api/src/staff-reply.ts", Line: 189, CommitSHA: "A1B2C3D",
	})).must(t)
	return reviewed{issue: issue, round: round, finding: finding}
}

func (tm *team) addFinding(t *testing.T, roundID string, severity Severity) Finding {
	t.Helper()
	return got(tm.store.AddFinding(tm.ctx, tm.agent, NewFinding{ReviewRoundID: roundID, Severity: severity, Title: "T", Problem: "P", Fix: "F"})).must(t)
}

func TestReviewRoundsAndFindingsAreNumbered(t *testing.T) {
	tm := newTeam(t)
	r := tm.reviewed(t)
	second := tm.addFinding(t, r.round.ID, SeverityNit)

	if r.round.Number != 1 || r.finding.Label() != "R1" || second.Label() != "R2" {
		t.Errorf("round %d, findings %s and %s", r.round.Number, r.finding.Label(), second.Label())
	}
	if r.finding.CommitSHA != "a1b2c3d" || r.finding.FoundByActorID != tm.agent.ActorID {
		t.Errorf("finding %+v", r.finding)
	}

	got(tm.store.FinishReviewRound(tm.ctx, tm.agent, r.round.ID, "Two findings.")).must(t)
	next := got(tm.store.StartReviewRound(tm.ctx, tm.agent, NewReviewRound{IssueID: r.issue.ID, ToCommit: "e4f5a6b"})).must(t)
	restarted := tm.addFinding(t, next.ID, SeverityShouldFix)

	if next.Number != 2 || restarted.Label() != "R1" {
		t.Errorf("second round %d starts at %s, want 2 and R1", next.Number, restarted.Label())
	}
}

func TestALaterRoundStartsFromThePreviousRange(t *testing.T) {
	tm := newTeam(t)
	r := tm.reviewed(t)
	got(tm.store.FinishReviewRound(tm.ctx, tm.agent, r.round.ID, "")).must(t)

	next := got(tm.store.StartReviewRound(tm.ctx, tm.agent, NewReviewRound{IssueID: r.issue.ID, ToCommit: "e4f5a6b", IncludesUncommitted: true})).must(t)
	if next.FromCommit != "a1b2c3d" || !next.IncludesUncommitted {
		t.Errorf("second round %+v, want it to start at a1b2c3d", next)
	}

	if r.round.FromCommit != "" {
		t.Errorf("first round from = %q, want the whole branch", r.round.FromCommit)
	}
}

func TestAnIssueHasOneRoundInProgress(t *testing.T) {
	tm := newTeam(t)
	r := tm.reviewed(t)

	_, err := tm.store.StartReviewRound(tm.ctx, tm.agent, NewReviewRound{IssueID: r.issue.ID, ToCommit: "e4f5a6b"})
	wantErr(t, err, ErrConflict)

	_, err = tm.store.StartReviewRound(tm.ctx, tm.agent, NewReviewRound{IssueID: r.issue.ID, ToCommit: "not-a-sha"})
	wantInvalid(t, err, "to_commit")
}

func TestAFinishedRoundTakesNoFindings(t *testing.T) {
	tm := newTeam(t)
	r := tm.reviewed(t)
	got(tm.store.FinishReviewRound(tm.ctx, tm.agent, r.round.ID, "Done.")).must(t)

	_, err := tm.store.AddFinding(tm.ctx, tm.agent, NewFinding{ReviewRoundID: r.round.ID, Severity: SeverityNit, Title: "T", Problem: "P", Fix: "F"})
	wantErr(t, err, ErrInvalidTransition)

	_, err = tm.store.FinishReviewRound(tm.ctx, tm.agent, r.round.ID, "Again.")
	wantErr(t, err, ErrInvalidTransition)
}

func TestOnlyTheAuthorOrAPersonFinishesARound(t *testing.T) {
	tm := newTeam(t)
	r := tm.reviewed(t)
	codex := got(tm.store.CreateAgent(tm.ctx, tm.owner, NewAgent{DisplayName: "Codex (Olive)", AgentName: "codex"})).must(t)

	_, err := tm.store.FinishReviewRound(tm.ctx, Caller{ActorID: codex.ID}, r.round.ID, "")
	wantErr(t, err, ErrForbidden)

	got(tm.store.FinishReviewRound(tm.ctx, tm.owner, r.round.ID, "Closed by a person.")).must(t)
}

func TestConcurrentFindingsGetDistinctNumbers(t *testing.T) {
	tm := newTeam(t)
	r := tm.reviewed(t)

	const writers = 20
	var wg sync.WaitGroup
	for range writers {
		wg.Go(func() {
			if _, err := tm.store.AddFinding(tm.ctx, tm.agent, NewFinding{ReviewRoundID: r.round.ID, Severity: SeverityNit, Title: "T", Problem: "P", Fix: "F"}); err != nil {
				t.Errorf("concurrent AddFinding: %v", err)
			}
		})
	}
	wg.Wait()

	findings := got(tm.store.ListRoundFindings(tm.ctx, r.round.ID)).must(t)
	numbers := make([]int, len(findings))
	for index, finding := range findings {
		numbers[index] = finding.Number
	}
	slices.Sort(numbers)
	for index, number := range numbers {
		if number != index+1 {
			t.Fatalf("numbers = %v, want 1 to %d", numbers, writers+1)
		}
	}
}

func TestFindingValidation(t *testing.T) {
	tm := newTeam(t)
	r := tm.reviewed(t)

	cases := map[string]NewFinding{
		"severity":   {Severity: "major", Title: "T", Problem: "P", Fix: "F"},
		"title":      {Severity: SeverityNit, Title: " ", Problem: "P", Fix: "F"},
		"problem":    {Severity: SeverityNit, Title: "T", Fix: "F"},
		"fix":        {Severity: SeverityNit, Title: "T", Problem: "P"},
		"line":       {Severity: SeverityNit, Title: "T", Problem: "P", Fix: "F", Line: 4},
		"commit_sha": {Severity: SeverityNit, Title: "T", Problem: "P", Fix: "F", CommitSHA: "main"},
	}
	for field, input := range cases {
		t.Run(field, func(t *testing.T) {
			input.ReviewRoundID = r.round.ID
			_, err := tm.store.AddFinding(tm.ctx, tm.agent, input)
			wantInvalid(t, err, field)
		})
	}
}

func TestFindingTransitions(t *testing.T) {
	statuses := []FindingStatus{FindingOpen, FindingFix, FindingWontfix, FindingLater, FindingFixed, FindingVerified}
	paths := map[FindingStatus][]FindingStatus{
		FindingOpen:     {},
		FindingFix:      {FindingFix},
		FindingWontfix:  {FindingWontfix},
		FindingLater:    {FindingLater},
		FindingFixed:    {FindingFix, FindingFixed},
		FindingVerified: {FindingFix, FindingFixed, FindingVerified},
	}

	for _, from := range statuses {
		for _, to := range statuses {
			if from == to {
				continue
			}

			t.Run(fmt.Sprintf("%s to %s", from, to), func(t *testing.T) {
				tm := newTeam(t)
				r := tm.reviewed(t)
				for _, step := range paths[from] {
					got(tm.store.ChangeFindingStatus(tm.ctx, tm.member, r.finding.ID, FindingChange{Status: step, Reason: "because"})).must(t)
				}

				_, err := tm.store.ChangeFindingStatus(tm.ctx, tm.member, r.finding.ID, FindingChange{Status: to, Reason: "because"})
				if slices.Contains(findingTransitions[from], to) {
					if err != nil {
						t.Fatalf("allowed move failed: %v", err)
					}
				} else {
					wantErr(t, err, ErrInvalidTransition)
				}
			})
		}
	}
}

func TestOnlyPeopleDecideOnFindings(t *testing.T) {
	tm := newTeam(t)
	r := tm.reviewed(t)

	for _, decision := range []FindingStatus{FindingFix, FindingWontfix, FindingLater} {
		_, err := tm.store.ChangeFindingStatus(tm.ctx, tm.agent, r.finding.ID, FindingChange{Status: decision, Reason: "agent says so"})
		wantErr(t, err, ErrForbidden)
	}

	decided := got(tm.store.ChangeFindingStatus(tm.ctx, tm.member, r.finding.ID, FindingChange{Status: FindingFix})).must(t)
	if decided.DecidedByActorID != tm.member.ActorID || decided.DecidedAt == nil {
		t.Errorf("decision not recorded: %+v", decided)
	}

	_, err := tm.store.ChangeFindingStatus(tm.ctx, tm.agent, r.finding.ID, FindingChange{Status: FindingOpen})
	wantErr(t, err, ErrForbidden)
}

func TestAgentsFixAndVerifyFindings(t *testing.T) {
	tm := newTeam(t)
	r := tm.reviewed(t)
	got(tm.store.ChangeFindingStatus(tm.ctx, tm.member, r.finding.ID, FindingChange{Status: FindingFix})).must(t)

	fixed := got(tm.store.ChangeFindingStatus(tm.ctx, tm.agent, r.finding.ID, FindingChange{Status: FindingFixed, FixedInCommit: "E4F5A6B"})).must(t)
	if fixed.FixedByActorID != tm.agent.ActorID || fixed.FixedInCommit != "e4f5a6b" || fixed.FixedAt == nil {
		t.Errorf("fix not recorded: %+v", fixed)
	}

	// The fix did not hold, so the finding goes back to fix and loses its fix details.
	reopened := got(tm.store.ChangeFindingStatus(tm.ctx, tm.agent, r.finding.ID, FindingChange{Status: FindingFix})).must(t)
	if reopened.FixedByActorID != "" || reopened.FixedAt != nil || reopened.DecidedByActorID != tm.member.ActorID {
		t.Errorf("after a failed fix: %+v", reopened)
	}

	got(tm.store.ChangeFindingStatus(tm.ctx, tm.agent, r.finding.ID, FindingChange{Status: FindingFixed})).must(t)
	verified := got(tm.store.ChangeFindingStatus(tm.ctx, tm.agent, r.finding.ID, FindingChange{Status: FindingVerified})).must(t)
	if verified.VerifiedByActorID != tm.agent.ActorID || verified.VerifiedAt == nil {
		t.Errorf("verification not recorded: %+v", verified)
	}

	stored := got(tm.store.GetFinding(tm.ctx, r.finding.ID)).must(t)
	if stored.Status != FindingVerified || stored.FixedByActorID != tm.agent.ActorID {
		t.Errorf("stored finding %+v", stored)
	}
}

func TestWontfixNeedsAReason(t *testing.T) {
	tm := newTeam(t)
	r := tm.reviewed(t)

	_, err := tm.store.ChangeFindingStatus(tm.ctx, tm.member, r.finding.ID, FindingChange{Status: FindingWontfix, Reason: "  "})
	wantInvalid(t, err, "reason")

	decided := got(tm.store.ChangeFindingStatus(tm.ctx, tm.member, r.finding.ID, FindingChange{Status: FindingWontfix, Reason: "The tombstone covers this."})).must(t)
	if decided.DecisionReason != "The tombstone covers this." {
		t.Errorf("reason = %q", decided.DecisionReason)
	}

	withdrawn := got(tm.store.ChangeFindingStatus(tm.ctx, tm.member, r.finding.ID, FindingChange{Status: FindingOpen})).must(t)
	if withdrawn.DecisionReason != "" || withdrawn.DecidedByActorID != "" {
		t.Errorf("withdrawn decision kept its details: %+v", withdrawn)
	}
}

func TestLaterCreatesOneFollowUpIssue(t *testing.T) {
	tm := newTeam(t)
	r := tm.reviewed(t)

	later := got(tm.store.ChangeFindingStatus(tm.ctx, tm.member, r.finding.ID, FindingChange{Status: FindingLater})).must(t)
	followUp := got(tm.store.GetIssue(tm.ctx, later.FollowUpIssueID)).must(t)
	if followUp.Status != IssueBacklog || followUp.Title != r.finding.Title || followUp.CreatedByID != tm.member.ActorID {
		t.Errorf("follow-up %+v", followUp)
	}
	if !strings.Contains(followUp.Plan, "Follow-up from WP-1, review round 1, R1.") || !strings.Contains(followUp.Plan, r.finding.Fix) {
		t.Errorf("follow-up plan = %q", followUp.Plan)
	}

	got(tm.store.ChangeFindingStatus(tm.ctx, tm.member, r.finding.ID, FindingChange{Status: FindingOpen})).must(t)
	again := got(tm.store.ChangeFindingStatus(tm.ctx, tm.member, r.finding.ID, FindingChange{Status: FindingLater})).must(t)
	if again.FollowUpIssueID != followUp.ID {
		t.Errorf("deciding later again made another follow-up issue")
	}

	backlog := got(tm.store.ListIssues(tm.ctx, IssueFilter{ProjectID: r.issue.ProjectID, Statuses: []IssueStatus{IssueBacklog}})).must(t)
	if len(backlog) != 1 {
		t.Errorf("backlog has %d issues, want 1", len(backlog))
	}
}

func TestMarkDuplicate(t *testing.T) {
	tm := newTeam(t)
	r := tm.reviewed(t)
	copy := tm.addFinding(t, r.round.ID, SeverityShouldFix)
	third := tm.addFinding(t, r.round.ID, SeverityNit)
	other := tm.reviewedIn(t, "SS")

	if err := tm.store.MarkDuplicate(tm.ctx, tm.agent, copy.ID, r.finding.ID); err != nil {
		t.Fatalf("MarkDuplicate: %v", err)
	}

	wantInvalid(t, tm.store.MarkDuplicate(tm.ctx, tm.agent, third.ID, copy.ID), "duplicate_of_finding_id")
	wantInvalid(t, tm.store.MarkDuplicate(tm.ctx, tm.agent, r.finding.ID, third.ID), "finding_id")
	wantInvalid(t, tm.store.MarkDuplicate(tm.ctx, tm.agent, third.ID, third.ID), "duplicate_of_finding_id")
	wantInvalid(t, tm.store.MarkDuplicate(tm.ctx, tm.agent, third.ID, other.finding.ID), "duplicate_of_finding_id")

	open := got(tm.store.ListIssueFindings(tm.ctx, r.issue.ID, []FindingStatus{FindingOpen}, true)).must(t)
	if len(open) != 2 {
		t.Errorf("open findings without duplicates = %d, want 2", len(open))
	}

	if err := tm.store.MarkDuplicate(tm.ctx, tm.agent, copy.ID, ""); err != nil {
		t.Fatalf("clear duplicate: %v", err)
	}
	if cleared := got(tm.store.GetFinding(tm.ctx, copy.ID)).must(t); cleared.DuplicateOfFindingID != "" {
		t.Error("duplicate mark not cleared")
	}
}

func TestIssueFindingsAreOrderedBySeverityWithinRounds(t *testing.T) {
	tm := newTeam(t)
	r := tm.reviewed(t)
	nit := tm.addFinding(t, r.round.ID, SeverityNit)
	shouldFix := tm.addFinding(t, r.round.ID, SeverityShouldFix)
	got(tm.store.FinishReviewRound(tm.ctx, tm.agent, r.round.ID, "")).must(t)
	next := got(tm.store.StartReviewRound(tm.ctx, tm.agent, NewReviewRound{IssueID: r.issue.ID, ToCommit: "e4f5a6b"})).must(t)
	blocking := tm.addFinding(t, next.ID, SeverityBlocking)

	findings := got(tm.store.ListIssueFindings(tm.ctx, r.issue.ID, nil, false)).must(t)
	got := make([]string, len(findings))
	for index, finding := range findings {
		got[index] = finding.ID
	}
	want := []string{r.finding.ID, shouldFix.ID, nit.ID, blocking.ID}
	if !slices.Equal(got, want) {
		t.Errorf("order = %v, want %v", got, want)
	}
}
