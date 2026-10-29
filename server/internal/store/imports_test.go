package store

import "testing"

func TestImportKeepsNumbersAndRunsTwiceSafely(t *testing.T) {
	tm := newTeam(t)

	project := got(tm.store.ImportProject(tm.ctx, tm.system, ImportLinear, ImportedProject{ExternalID: "p1", Key: "WP", Name: "WP Agent"})).must(t)
	again := got(tm.store.ImportProject(tm.ctx, tm.system, ImportLinear, ImportedProject{ExternalID: "p1", Key: "WP", Name: "WP Agent"})).must(t)
	if again.ID != project.ID {
		t.Fatal("a second import made a second project")
	}

	milestone := got(tm.store.ImportMilestone(tm.ctx, tm.system, ImportLinear, ImportedMilestone{
		ExternalID: "m1", ProjectID: project.ID, Name: "Version 01", Status: MilestoneActive,
	})).must(t)
	input := ImportedIssue{
		ExternalID: "i142", ProjectID: project.ID, Number: 142, Title: "Meta comments", Status: IssueDone, MilestoneID: milestone.ID,
	}
	issue := got(tm.store.ImportIssue(tm.ctx, tm.system, ImportLinear, input)).must(t)
	if issue.Key() != "WP-142" || issue.CompletedAt == nil || issue.CreatedByID != SystemActorID {
		t.Fatalf("imported %+v", issue)
	}
	if again := got(tm.store.ImportIssue(tm.ctx, tm.system, ImportLinear, input)).must(t); again.ID != issue.ID {
		t.Fatal("a second import made a second issue")
	}

	// New issues continue after the highest imported number.
	next := tm.issue(t, project.ID, "After import")
	if next.Number != 143 {
		t.Errorf("next issue number %d, want 143", next.Number)
	}

	_, err := tm.store.ImportIssue(tm.ctx, tm.system, ImportLinear, ImportedIssue{
		ExternalID: "other", ProjectID: project.ID, Number: 142, Title: "Clash", Status: IssueTodo,
	})
	wantErr(t, err, ErrConflict)
}

func TestImportReviewRoundWithFindings(t *testing.T) {
	tm := newTeam(t)
	project := tm.project(t, "WP")
	issue := tm.issue(t, project.ID, "Meta comments")

	input := ImportedReviewRound{
		ExternalID: "c1", IssueID: issue.ID, Number: 2, ToCommit: "a1b2c3d", Summary: "Two findings.",
		Findings: []ImportedFinding{
			{Number: 1, Severity: SeverityBlocking, Title: "Race", Problem: "P", Fix: "F", Status: FindingVerified},
			{Number: 3, Severity: SeverityNit, Title: "Name", Problem: "P", Fix: "F", Status: FindingWontfix},
		},
	}
	round := got(tm.store.ImportReviewRound(tm.ctx, tm.system, ImportLinear, input)).must(t)
	if round.Number != 2 || round.FinishedAt == nil {
		t.Fatalf("imported round %+v", round)
	}

	findings := got(tm.store.ListRoundFindings(tm.ctx, round.ID)).must(t)
	if len(findings) != 2 || findings[0].VerifiedByActorID != SystemActorID || findings[1].DecisionReason != importedWontfixReason {
		t.Fatalf("findings %+v", findings)
	}
	if again := got(tm.store.ImportReviewRound(tm.ctx, tm.system, ImportLinear, input)).must(t); again.ID != round.ID {
		t.Fatal("a second import made a second round")
	}

	next := got(tm.store.StartReviewRound(tm.ctx, tm.agent, NewReviewRound{IssueID: issue.ID, ToCommit: "e4f5a6b"})).must(t)
	if next.Number != 3 || next.FromCommit != "a1b2c3d" {
		t.Errorf("next round %d from %s, want 3 from a1b2c3d", next.Number, next.FromCommit)
	}
}
