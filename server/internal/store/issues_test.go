package store

import (
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/shantodotdev/super-sparrow/server/internal/checklist"
)

func TestIssueNumbersIncreasePerProject(t *testing.T) {
	tm := newTeam(t)
	wp := tm.project(t, "WP")
	ss := tm.project(t, "SS")

	first := tm.issue(t, wp.ID, "First")
	second := tm.issue(t, wp.ID, "Second")
	other := tm.issue(t, ss.ID, "Other project")

	if first.Key() != "WP-1" || second.Key() != "WP-2" || other.Key() != "SS-1" {
		t.Errorf("keys = %s, %s, %s", first.Key(), second.Key(), other.Key())
	}
}

func TestConcurrentIssuesGetDistinctNumbers(t *testing.T) {
	tm := newTeam(t)
	project := tm.project(t, "WP")

	const writers = 25
	numbers := make([]int, writers)
	errs := make([]error, writers)

	var wg sync.WaitGroup
	for index := range writers {
		wg.Go(func() {
			issue, err := tm.store.CreateIssue(tm.ctx, tm.agent, NewIssue{ProjectID: project.ID, Title: "Parallel", Status: IssueBacklog})
			numbers[index], errs[index] = issue.Number, err
		})
	}
	wg.Wait()

	for _, err := range errs {
		if err != nil {
			t.Fatalf("concurrent CreateIssue: %v", err)
		}
	}
	slices.Sort(numbers)
	for index, number := range numbers {
		if number != index+1 {
			t.Fatalf("numbers = %v, want 1 to %d with no gaps or repeats", numbers, writers)
		}
	}
}

func TestIssueNumbersAreNeverReused(t *testing.T) {
	tm := newTeam(t)
	project := tm.project(t, "WP")
	tm.issue(t, project.ID, "First")

	// A failed creation rolls back, so the next issue still gets the next number.
	_, err := tm.store.CreateIssue(tm.ctx, tm.member, NewIssue{ProjectID: project.ID, Title: "Bad", Status: IssueTodo, MilestoneID: "missing"})
	wantInvalid(t, err, "milestone_id")

	if next := tm.issue(t, project.ID, "Second"); next.Number != 2 {
		t.Errorf("number = %d, want 2", next.Number)
	}
}

func TestGetIssueByKey(t *testing.T) {
	tm := newTeam(t)
	project := tm.project(t, "WP")
	issue := tm.issue(t, project.ID, "Meta comments")

	found := got(tm.store.GetIssueByKey(tm.ctx, "wp-1")).must(t)
	if found.ID != issue.ID || found.ProjectKey != "WP" {
		t.Errorf("found %+v", found)
	}

	_, err := tm.store.GetIssueByKey(tm.ctx, "WP-99")
	wantErr(t, err, ErrNotFound)

	_, err = tm.store.GetIssueByKey(tm.ctx, "WP142")
	wantInvalid(t, err, "key")
}

func TestFindIssueForBranch(t *testing.T) {
	tm := newTeam(t)
	wp := tm.project(t, "WP")
	ss := tm.project(t, "SS")
	keyed := tm.issue(t, wp.ID, "Keyed")
	recorded := got(tm.store.CreateIssue(tm.ctx, tm.member, NewIssue{ProjectID: ss.ID, Title: "Recorded", Status: IssueTodo, BranchName: "spike/inbox-redesign"})).must(t)

	cases := map[string]string{
		"feat/wp-1-meta-comments": keyed.ID,
		"WP-1":                    keyed.ID,
		"fix/xx-9-wp-1-follow-up": keyed.ID,
		"spike/inbox-redesign":    recorded.ID,
	}
	for branch, want := range cases {
		t.Run(branch, func(t *testing.T) {
			found := got(tm.store.FindIssueForBranch(tm.ctx, branch)).must(t)
			if found.ID != want {
				t.Errorf("found %s, want %s", found.Key(), want)
			}
		})
	}

	for _, branch := range []string{"main", "feat/wp-99-missing", "feat/wp1-no-dash"} {
		_, err := tm.store.FindIssueForBranch(tm.ctx, branch)
		wantErr(t, err, ErrNotFound)
	}
}

func TestBranchNamesAreUniquePerProject(t *testing.T) {
	tm := newTeam(t)
	project := tm.project(t, "WP")
	got(tm.store.CreateIssue(tm.ctx, tm.member, NewIssue{ProjectID: project.ID, Title: "A", Status: IssueTodo, BranchName: "feat/wp-1-a"})).must(t)

	_, err := tm.store.CreateIssue(tm.ctx, tm.member, NewIssue{ProjectID: project.ID, Title: "B", Status: IssueTodo, BranchName: "feat/wp-1-a"})
	wantErr(t, err, ErrConflict)

	_, err = tm.store.CreateIssue(tm.ctx, tm.member, NewIssue{ProjectID: project.ID, Title: "C", Status: IssueTodo, BranchName: "bad branch"})
	wantInvalid(t, err, "branch_name")
}

func TestParentIssuesCannotFormACycle(t *testing.T) {
	tm := newTeam(t)
	project := tm.project(t, "WP")
	other := tm.project(t, "SS")
	grandparent := tm.issue(t, project.ID, "Grandparent")
	parent := got(tm.store.CreateIssue(tm.ctx, tm.member, NewIssue{ProjectID: project.ID, Title: "Parent", Status: IssueTodo, ParentIssueID: grandparent.ID})).must(t)
	child := got(tm.store.CreateIssue(tm.ctx, tm.member, NewIssue{ProjectID: project.ID, Title: "Child", Status: IssueTodo, ParentIssueID: parent.ID})).must(t)
	foreign := tm.issue(t, other.ID, "Foreign")

	_, err := tm.store.UpdateIssue(tm.ctx, tm.member, grandparent.ID, IssueUpdate{ParentIssueID: Change(child.ID)})
	wantInvalid(t, err, "parent_issue_id")

	_, err = tm.store.UpdateIssue(tm.ctx, tm.member, grandparent.ID, IssueUpdate{ParentIssueID: Change(grandparent.ID)})
	wantInvalid(t, err, "parent_issue_id")

	_, err = tm.store.UpdateIssue(tm.ctx, tm.member, child.ID, IssueUpdate{ParentIssueID: Change(foreign.ID)})
	wantInvalid(t, err, "parent_issue_id")

	cleared := got(tm.store.UpdateIssue(tm.ctx, tm.member, child.ID, IssueUpdate{ParentIssueID: Change("")})).must(t)
	if cleared.ParentIssueID != "" {
		t.Errorf("parent = %q, want cleared", cleared.ParentIssueID)
	}
}

func TestIssueReferencesMustFitTheProject(t *testing.T) {
	tm := newTeam(t)
	project := tm.project(t, "WP")
	other := tm.project(t, "SS")
	foreignMilestone := got(tm.store.CreateMilestone(tm.ctx, tm.member, NewMilestone{ProjectID: other.ID, Name: "M", Status: MilestonePlanned})).must(t)
	foreignLabel := got(tm.store.CreateLabel(tm.ctx, tm.member, other.ID, "Bug")).must(t)

	_, err := tm.store.CreateIssue(tm.ctx, tm.member, NewIssue{ProjectID: project.ID, Title: "T", Status: IssueTodo, MilestoneID: foreignMilestone.ID})
	wantInvalid(t, err, "milestone_id")

	_, err = tm.store.CreateIssue(tm.ctx, tm.member, NewIssue{ProjectID: project.ID, Title: "T", Status: IssueTodo, LabelIDs: []string{foreignLabel.ID}})
	wantInvalid(t, err, "label_ids")

	_, err = tm.store.CreateIssue(tm.ctx, tm.member, NewIssue{ProjectID: project.ID, Title: "T", Status: IssueTodo, AssigneeActorID: SystemActorID})
	wantInvalid(t, err, "assignee_actor_id")
}

func TestSetIssueStatusRecordsCompletion(t *testing.T) {
	tm := newTeam(t)
	project := tm.project(t, "WP")
	issue := tm.issue(t, project.ID, "Ship it")

	if err := tm.store.SetIssueStatus(tm.ctx, tm.agent, issue.ID, IssueDone); err != nil {
		t.Fatalf("SetIssueStatus: %v", err)
	}
	if done := got(tm.store.GetIssue(tm.ctx, issue.ID)).must(t); done.CompletedAt == nil {
		t.Error("done issue has no completion time")
	}

	if err := tm.store.SetIssueStatus(tm.ctx, tm.agent, issue.ID, IssueInProgress); err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if reopened := got(tm.store.GetIssue(tm.ctx, issue.ID)).must(t); reopened.CompletedAt != nil {
		t.Error("reopened issue kept its completion time")
	}

	wantInvalid(t, tm.store.SetIssueStatus(tm.ctx, tm.agent, issue.ID, "shipped"), "status")
}

func TestSetIssueLabelsReplacesTheSet(t *testing.T) {
	tm := newTeam(t)
	project := tm.project(t, "WP")
	bug := got(tm.store.CreateLabel(tm.ctx, tm.member, project.ID, "Bug")).must(t)
	api := got(tm.store.CreateLabel(tm.ctx, tm.member, project.ID, "API")).must(t)
	issue := tm.issue(t, project.ID, "Labelled")

	if err := tm.store.SetIssueLabels(tm.ctx, tm.agent, issue.ID, []string{bug.ID, api.ID, bug.ID}); err != nil {
		t.Fatalf("SetIssueLabels: %v", err)
	}
	if got := got(tm.store.GetIssue(tm.ctx, issue.ID)).must(t).LabelIDs; len(got) != 2 {
		t.Errorf("labels = %v, want two without the repeat", got)
	}

	if err := tm.store.SetIssueLabels(tm.ctx, tm.agent, issue.ID, []string{api.ID}); err != nil {
		t.Fatalf("SetIssueLabels: %v", err)
	}
	if got := got(tm.store.GetIssue(tm.ctx, issue.ID)).must(t).LabelIDs; !slices.Equal(got, []string{api.ID}) {
		t.Errorf("labels = %v, want only API", got)
	}
}

func TestUpdateIssueRecordsOnlyChangedFields(t *testing.T) {
	tm := newTeam(t)
	project := tm.project(t, "WP")
	issue := tm.issue(t, project.ID, "Old title")

	updated := got(tm.store.UpdateIssue(tm.ctx, tm.agent, issue.ID, IssueUpdate{
		Title:        Change("New title"),
		Plan:         Change("## Goal\n\nShip it."),
		PlanRevision: 1,
	})).must(t)
	stored := got(tm.store.GetIssue(tm.ctx, issue.ID)).must(t)
	if updated.Title != "New title" || updated.Plan != "## Goal\n\nShip it." || !updated.UpdatedAt.Equal(stored.UpdatedAt) {
		t.Errorf("returned %+v, stored %+v", updated, stored)
	}

	event := tm.lastEvent(t)
	want := `{"plan":{"revision":2},"title":{"from":"Old title","to":"New title"}}`
	if event.Kind != "issue.updated" || string(event.Payload) != want || event.ReportedModel != "claude-test-model" {
		t.Errorf("event %s %s model %q", event.Kind, event.Payload, event.ReportedModel)
	}

	before := tm.eventCount(t)
	got(tm.store.UpdateIssue(tm.ctx, tm.agent, issue.ID, IssueUpdate{Title: Change("New title")})).must(t)
	if tm.eventCount(t) != before {
		t.Error("an update that changed nothing recorded an event")
	}
}

func TestPlanEditsMadeOnAnOldRevisionAreRefused(t *testing.T) {
	tm := newTeam(t)
	project := tm.project(t, "WP")
	issue := got(tm.store.CreateIssue(tm.ctx, tm.member, NewIssue{
		ProjectID: project.ID, Title: "Plan", Status: IssueTodo, Plan: "- [ ] Migration\n- [ ] Store",
	})).must(t)
	if issue.PlanRevision != 1 {
		t.Fatalf("new plan revision = %d, want 1", issue.PlanRevision)
	}

	// The agent ticks an item while the person edits the plan they loaded at revision 1.
	ticked := got(tm.store.UpdatePlanChecklist(tm.ctx, tm.agent, issue.ID, map[int]checklist.Change{0: {Checked: true, Text: "Migration"}})).must(t)
	if ticked.PlanRevision != 2 {
		t.Fatalf("ticking moved the revision to %d, want 2", ticked.PlanRevision)
	}
	_, err := tm.store.UpdateIssue(tm.ctx, tm.member, issue.ID, IssueUpdate{
		Plan: Change("- [ ] Migration\n- [ ] Store\n- [ ] API"), PlanRevision: 1,
	})
	wantErr(t, err, ErrConflict)
	if !strings.Contains(Explain(err), "now at revision 2") {
		t.Errorf("conflict says %q", Explain(err))
	}
	if fresh := got(tm.store.GetIssue(tm.ctx, issue.ID)).must(t); fresh.Plan != "- [x] Migration\n- [ ] Store" {
		t.Errorf("the refused edit changed the plan to %q", fresh.Plan)
	}

	// Saved again from the current revision, the edit keeps the tick.
	saved := got(tm.store.UpdateIssue(tm.ctx, tm.member, issue.ID, IssueUpdate{
		Plan: Change("- [x] Migration\n- [ ] Store\n- [ ] API"), PlanRevision: 2,
	})).must(t)
	if saved.PlanRevision != 3 {
		t.Errorf("saved revision = %d, want 3", saved.PlanRevision)
	}

	// A plan without its revision is refused, and other fields need none.
	_, err = tm.store.UpdateIssue(tm.ctx, tm.member, issue.ID, IssueUpdate{Plan: Change("Rewritten")})
	wantInvalid(t, err, "plan_revision")
	got(tm.store.UpdateIssue(tm.ctx, tm.member, issue.ID, IssueUpdate{Title: Change("Renamed")})).must(t)
}

func TestListIssuesFiltersAndPages(t *testing.T) {
	tm := newTeam(t)
	project := tm.project(t, "WP")
	for range 5 {
		tm.issue(t, project.ID, "Todo")
	}
	done := tm.issue(t, project.ID, "Done")
	if err := tm.store.SetIssueStatus(tm.ctx, tm.member, done.ID, IssueDone); err != nil {
		t.Fatalf("SetIssueStatus: %v", err)
	}

	page := got(tm.store.ListIssues(tm.ctx, IssueFilter{ProjectID: project.ID, Statuses: []IssueStatus{IssueTodo}, Limit: 3})).must(t)
	if len(page) != 3 || page[0].Number != 5 || page[2].Number != 3 {
		t.Fatalf("first page = %v", issueNumbers(page))
	}

	next := got(tm.store.ListIssues(tm.ctx, IssueFilter{ProjectID: project.ID, Statuses: []IssueStatus{IssueTodo}, Limit: 3, BeforeNumber: 3})).must(t)
	if !slices.Equal(issueNumbers(next), []int{2, 1}) {
		t.Errorf("second page = %v", issueNumbers(next))
	}

	_, err := tm.store.ListIssues(tm.ctx, IssueFilter{ProjectID: project.ID, Limit: MaximumPageLimit + 1})
	wantInvalid(t, err, "limit")

	_, err = tm.store.ListIssues(tm.ctx, IssueFilter{})
	wantInvalid(t, err, "project_id")
}

func issueNumbers(issues []Issue) []int {
	numbers := make([]int, len(issues))
	for index, issue := range issues {
		numbers[index] = issue.Number
	}
	return numbers
}

func TestIssuePriorityPersistsAndRecordsActor(t *testing.T) {
	tm := newTeam(t)
	project := tm.project(t, "WP")
	issue := got(tm.store.CreateIssue(tm.ctx, tm.member, NewIssue{ProjectID: project.ID, Title: "Prioritized", Status: IssueTodo, Priority: "high"})).must(t)
	if loaded := got(tm.store.GetIssue(tm.ctx, issue.ID)).must(t); loaded.Priority != "high" {
		t.Fatalf("priority = %q", loaded.Priority)
	}
	updated := got(tm.store.UpdateIssue(tm.ctx, tm.agent, issue.ID, IssueUpdate{Priority: Change("urgent")})).must(t)
	if updated.Priority != "urgent" || updated.PlanRevision != issue.PlanRevision {
		t.Fatalf("updated %+v", updated)
	}
	events := got(tm.store.ListEvents(tm.ctx, EventFilter{TargetType: TargetIssue, TargetID: issue.ID, Limit: MaximumPageLimit})).must(t)
	found := false
	for _, event := range events {
		if event.Kind == "issue.updated" && event.ActorID == tm.agent.ActorID && strings.Contains(string(event.Payload), "priority") {
			found = true
		}
	}
	if !found {
		t.Fatal("priority update did not record the agent actor")
	}
	_, err := tm.store.UpdateIssue(tm.ctx, tm.member, issue.ID, IssueUpdate{Priority: Change("invalid")})
	wantInvalid(t, err, "priority")
	_, err = tm.store.CreateIssue(tm.ctx, tm.member, NewIssue{ProjectID: project.ID, Title: "Invalid", Status: IssueTodo, Priority: "invalid"})
	wantInvalid(t, err, "priority")
	cleared := got(tm.store.UpdateIssue(tm.ctx, tm.member, issue.ID, IssueUpdate{Priority: Change("")})).must(t)
	if cleared.Priority != "" {
		t.Fatalf("cleared priority = %q", cleared.Priority)
	}
}
