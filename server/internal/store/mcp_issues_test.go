package store

import "testing"

func TestListAssignedIssuesSpansProjectsAndFilters(t *testing.T) {
	tm := newTeam(t)
	first, second := tm.project(t, "AA"), tm.project(t, "BB")

	mine := []Issue{tm.issue(t, first.ID, "One"), tm.issue(t, second.ID, "Two"), tm.issue(t, second.ID, "Three")}
	for _, issue := range mine {
		got(tm.store.UpdateIssue(tm.ctx, tm.member, issue.ID, IssueUpdate{AssigneeActorID: Change(tm.member.ActorID)})).must(t)
	}
	got(tm.store.UpdateIssue(tm.ctx, tm.member, mine[2].ID, IssueUpdate{Status: Change(IssueDone)})).must(t)
	notMine := tm.issue(t, first.ID, "Someone else's")
	got(tm.store.UpdateIssue(tm.ctx, tm.member, notMine.ID, IssueUpdate{AssigneeActorID: Change(tm.owner.ActorID)})).must(t)

	all := got(tm.store.ListAssignedIssues(tm.ctx, []string{tm.member.ActorID}, "", nil, 10, 0)).must(t)
	if len(all) != 3 {
		t.Fatalf("assigned issues %d, want 3", len(all))
	}
	inProject := got(tm.store.ListAssignedIssues(tm.ctx, []string{tm.member.ActorID}, second.ID, []IssueStatus{IssueTodo}, 10, 0)).must(t)
	if len(inProject) != 1 || inProject[0].ID != mine[1].ID {
		t.Fatalf("issues in project %+v", inProject)
	}
	either := got(tm.store.ListAssignedIssues(tm.ctx, []string{tm.member.ActorID, tm.owner.ActorID}, "", nil, 10, 0)).must(t)
	if len(either) != 4 {
		t.Fatalf("issues of either assignee %d, want 4", len(either))
	}

	page := got(tm.store.ListAssignedIssues(tm.ctx, []string{tm.member.ActorID}, "", nil, 2, 2)).must(t)
	if len(page) != 1 {
		t.Fatalf("last page has %d issues", len(page))
	}
}

func TestListAssignedIssuesValidatesItsInput(t *testing.T) {
	tm := newTeam(t)

	_, err := tm.store.ListAssignedIssues(tm.ctx, nil, "", nil, 10, 0)
	wantInvalid(t, err, "assignee")
	_, err = tm.store.ListAssignedIssues(tm.ctx, []string{tm.member.ActorID}, "", []IssueStatus{"finished"}, 10, 0)
	wantInvalid(t, err, "statuses")
	_, err = tm.store.ListAssignedIssues(tm.ctx, []string{tm.member.ActorID}, "", nil, 0, 0)
	wantInvalid(t, err, "page")
}
