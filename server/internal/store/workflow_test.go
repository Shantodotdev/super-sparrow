package store

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestUpdateIssueChangesFieldsStatusAndLabelsTogether(t *testing.T) {
	tm := newTeam(t)
	project := tm.project(t, "WP")
	issue := tm.issue(t, project.ID, "Meta comments")
	label := got(tm.store.CreateLabel(tm.ctx, tm.member, project.ID, "backend")).must(t)
	before := tm.eventCount(t)

	updated := got(tm.store.UpdateIssue(tm.ctx, tm.agent, issue.ID, IssueUpdate{
		Title: Change("Meta comments v2"), Status: Change(IssueInProgress), LabelIDs: Change([]string{label.ID}),
	})).must(t)

	if updated.Title != "Meta comments v2" || updated.Status != IssueInProgress || len(updated.LabelIDs) != 1 {
		t.Fatalf("updated issue %+v", updated)
	}
	if count := tm.eventCount(t); count != before+3 {
		t.Errorf("recorded %d events, want 3", count-before)
	}

	// A bad label refuses the whole update, so the title stays as it was.
	_, err := tm.store.UpdateIssue(tm.ctx, tm.agent, issue.ID, IssueUpdate{Title: Change("Lost"), LabelIDs: Change([]string{"missing"})})
	wantInvalid(t, err, "label_ids")
	if current := got(tm.store.GetIssue(tm.ctx, issue.ID)).must(t); current.Title != "Meta comments v2" {
		t.Errorf("title = %q after a refused update", current.Title)
	}
}

func TestUpdatePlanChecklistTicksOnlyNamedItems(t *testing.T) {
	tm := newTeam(t)
	project := tm.project(t, "WP")
	issue := got(tm.store.CreateIssue(tm.ctx, tm.member, NewIssue{
		ProjectID: project.ID, Title: "Plan", Status: IssueTodo, Plan: "- [ ] One\n- [ ] Two",
	})).must(t)

	updated := got(tm.store.UpdatePlanChecklist(tm.ctx, tm.agent, issue.ID, map[int]bool{1: true})).must(t)
	if updated.Plan != "- [ ] One\n- [x] Two" {
		t.Fatalf("plan = %q", updated.Plan)
	}
	if event := tm.lastEvent(t); event.Kind != "issue.checklist_changed" || event.ActorID != tm.agent.ActorID {
		t.Errorf("last event %s by %s", event.Kind, event.ActorID)
	}

	_, err := tm.store.UpdatePlanChecklist(tm.ctx, tm.agent, issue.ID, map[int]bool{2: true})
	wantInvalid(t, err, "items")
}

func TestPostFixSummaryMarksFindingsFixedAndComments(t *testing.T) {
	tm := newTeam(t)
	r := tm.reviewed(t)
	got(tm.store.ChangeFindingStatus(tm.ctx, tm.member, r.finding.ID, FindingChange{Status: FindingFix})).must(t)

	_, _, err := tm.store.PostFixSummary(tm.ctx, tm.agent, FixSummary{IssueID: r.issue.ID, Summary: "Nothing.", Commit: "abc1234"})
	wantInvalid(t, err, "findings")

	comment, fixed, err := tm.store.PostFixSummary(tm.ctx, tm.agent, FixSummary{
		IssueID: r.issue.ID, Summary: "Sent both rows from one job.", Commit: "abc1234", FindingIDs: []string{r.finding.ID},
	})
	if err != nil {
		t.Fatalf("PostFixSummary: %v", err)
	}
	if len(fixed) != 1 || fixed[0].Status != FindingFixed || fixed[0].FixedInCommit != "abc1234" || fixed[0].FixedByActorID != tm.agent.ActorID {
		t.Fatalf("fixed %+v", fixed)
	}
	if comment.IssueID != r.issue.ID || comment.AuthorActorID != tm.agent.ActorID {
		t.Errorf("comment %+v", comment)
	}
}

func TestPostFixSummaryRefusesAnUndecidedFindingAndWritesNothing(t *testing.T) {
	tm := newTeam(t)
	r := tm.reviewed(t)
	before := tm.eventCount(t)

	_, _, err := tm.store.PostFixSummary(tm.ctx, tm.agent, FixSummary{
		IssueID: r.issue.ID, Summary: "Fixed.", Commit: "abc1234", FindingIDs: []string{r.finding.ID},
	})
	wantErr(t, err, ErrInvalidTransition)
	if tm.eventCount(t) != before {
		t.Error("a refused fix summary recorded events")
	}
}

func TestIssueHistoryIncludesRoundsFindingsAndComments(t *testing.T) {
	tm := newTeam(t)
	r := tm.reviewed(t)
	other := tm.issue(t, r.issue.ProjectID, "Other")
	got(tm.store.AddComment(tm.ctx, tm.member, NewComment{FindingID: r.finding.ID, Body: "Agreed."})).must(t)
	got(tm.store.AddComment(tm.ctx, tm.member, NewComment{IssueID: other.ID, Body: "Elsewhere."})).must(t)

	events := got(tm.store.ListEvents(tm.ctx, EventFilter{IssueID: r.issue.ID})).must(t)

	var kinds []string
	for _, event := range events {
		kinds = append(kinds, event.Kind)
	}
	if strings.Join(kinds, ",") != "issue.created,review_round.started,finding.added,comment.added" {
		t.Fatalf("history = %v", kinds)
	}
	var payload map[string]any
	if err := json.Unmarshal(events[3].Payload, &payload); err != nil || payload["issue_id"] != r.issue.ID {
		t.Errorf("finding comment payload %s", events[3].Payload)
	}
}

func TestProjectEventsOnlyLeavesOutPeople(t *testing.T) {
	tm := newTeam(t)
	tm.project(t, "WP")

	for _, event := range got(tm.store.ListEvents(tm.ctx, EventFilter{ProjectEventsOnly: true})).must(t) {
		if event.TargetType == TargetActor {
			t.Fatalf("listed %s", event.Kind)
		}
	}
}

func TestChangedFiresAfterACommit(t *testing.T) {
	tm := newTeam(t)
	changed := tm.store.Changed()

	tm.project(t, "WP")

	select {
	case <-changed:
	default:
		t.Fatal("Changed did not fire after a write")
	}
}

func TestCloseIssueForMergedPullRequest(t *testing.T) {
	tm := newTeam(t)
	project := tm.project(t, "WP")
	byURL := got(tm.store.CreateIssue(tm.ctx, tm.member, NewIssue{
		ProjectID: project.ID, Title: "By URL", Status: IssueInReview, PullRequestURL: "https://github.com/o/r/pull/7",
	})).must(t)
	byBranch := tm.issue(t, project.ID, "By branch")

	issue, changed := closed(t)(tm.store.CloseIssueForMergedPullRequest(tm.ctx, tm.system, MergedPullRequest{URL: "https://github.com/o/r/pull/7", Branch: "main"}))
	if !changed || issue.ID != byURL.ID || issue.Status != IssueDone || issue.CompletedAt == nil {
		t.Fatalf("closed %+v (changed %v)", issue, changed)
	}

	issue, changed = closed(t)(tm.store.CloseIssueForMergedPullRequest(tm.ctx, tm.system, MergedPullRequest{
		URL: "https://github.com/o/r/pull/8", Branch: "feat/" + strings.ToLower(byBranch.Key()) + "-thing",
	}))
	if !changed || issue.ID != byBranch.ID || issue.PullRequestURL != "https://github.com/o/r/pull/8" {
		t.Fatalf("closed %+v (changed %v)", issue, changed)
	}

	_, changed = closed(t)(tm.store.CloseIssueForMergedPullRequest(tm.ctx, tm.system, MergedPullRequest{URL: "https://github.com/o/r/pull/8"}))
	if changed {
		t.Error("closing a done issue again reported a change")
	}

	_, _, err := tm.store.CloseIssueForMergedPullRequest(tm.ctx, tm.system, MergedPullRequest{URL: "https://github.com/o/r/pull/9", Branch: "chore/no-key"})
	wantErr(t, err, ErrNotFound)
}

// closed checks a CloseIssueForMergedPullRequest call that must succeed.
func closed(t *testing.T) func(Issue, bool, error) (Issue, bool) {
	return func(issue Issue, changed bool, err error) (Issue, bool) {
		t.Helper()
		if err != nil {
			t.Fatalf("CloseIssueForMergedPullRequest: %v", err)
		}
		return issue, changed
	}
}

func TestBackupRunsReportLastRunAndLastSuccess(t *testing.T) {
	tm := newTeam(t)

	first := got(tm.store.StartBackupRun(tm.ctx, BackupSync, "sync/sparrow.db.gz.age")).must(t)
	if err := tm.store.FinishBackupRun(tm.ctx, first.ID, 1234, nil); err != nil {
		t.Fatal(err)
	}
	second := got(tm.store.StartBackupRun(tm.ctx, BackupSync, "sync/sparrow.db.gz.age")).must(t)
	if err := tm.store.FinishBackupRun(tm.ctx, second.ID, 0, errors.New("upload refused")); err != nil {
		t.Fatal(err)
	}

	status := got(tm.store.GetBackupStatus(tm.ctx)).must(t)
	sync := status[BackupSync]
	if sync.LastRun == nil || sync.LastRun.Result != BackupFailed || sync.LastRun.Error != "upload refused" {
		t.Fatalf("last run %+v", sync.LastRun)
	}
	if sync.LastSuccess == nil || sync.LastSuccess.ID != first.ID || sync.LastSuccess.SizeBytes != 1234 {
		t.Fatalf("last success %+v", sync.LastSuccess)
	}
	if status[BackupDaily].LastRun != nil {
		t.Error("daily job reports a run it never had")
	}
}

func TestVacuumIntoWritesACopy(t *testing.T) {
	tm := newTeam(t)
	tm.project(t, "WP")
	path := filepath.Join(t.TempDir(), "copy.db")

	if err := tm.store.VacuumInto(tm.ctx, path); err != nil {
		t.Fatalf("VacuumInto: %v", err)
	}
	if err := tm.store.VacuumInto(tm.ctx, path); err == nil {
		t.Error("VacuumInto overwrote an existing file")
	}
}
