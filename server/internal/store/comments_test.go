package store

import (
	"testing"
)

func TestCommentsAndReplies(t *testing.T) {
	tm := newTeam(t)
	r := tm.reviewed(t)

	question := got(tm.store.AddComment(tm.ctx, tm.member, NewComment{FindingID: r.finding.ID, Body: "Does this happen with one replica?"})).must(t)
	reply := got(tm.store.AddComment(tm.ctx, tm.agent, NewComment{FindingID: r.finding.ID, ParentCommentID: question.ID, Body: "No, it needs two."})).must(t)
	got(tm.store.AddComment(tm.ctx, tm.member, NewComment{IssueID: r.issue.ID, Body: "Plan approved."})).must(t)

	thread := got(tm.store.ListComments(tm.ctx, "", r.finding.ID)).must(t)
	if len(thread) != 2 || thread[0].ID != question.ID || thread[1].ParentCommentID != question.ID {
		t.Errorf("finding thread = %+v", thread)
	}
	if reply.AuthorActorID != tm.agent.ActorID {
		t.Errorf("reply author = %s, want the agent", reply.AuthorActorID)
	}

	_, err := tm.store.AddComment(tm.ctx, tm.member, NewComment{IssueID: r.issue.ID, ParentCommentID: question.ID, Body: "Wrong thread"})
	wantInvalid(t, err, "parent_comment_id")
}

func TestCommentNeedsExactlyOneTarget(t *testing.T) {
	tm := newTeam(t)
	r := tm.reviewed(t)

	_, err := tm.store.AddComment(tm.ctx, tm.member, NewComment{Body: "Nowhere"})
	wantInvalid(t, err, "target")

	_, err = tm.store.AddComment(tm.ctx, tm.member, NewComment{IssueID: r.issue.ID, FindingID: r.finding.ID, Body: "Both"})
	wantInvalid(t, err, "target")

	_, err = tm.store.AddComment(tm.ctx, tm.member, NewComment{IssueID: "missing", Body: "Missing"})
	wantErr(t, err, ErrNotFound)

	_, err = tm.store.AddComment(tm.ctx, tm.member, NewComment{IssueID: r.issue.ID, Body: "   "})
	wantInvalid(t, err, "body")
}

func TestOnlyTheAuthorEditsAComment(t *testing.T) {
	tm := newTeam(t)
	r := tm.reviewed(t)
	comment := got(tm.store.AddComment(tm.ctx, tm.agent, NewComment{IssueID: r.issue.ID, Body: "First draft"})).must(t)

	_, err := tm.store.EditComment(tm.ctx, tm.member, comment.ID, "Edited by someone else")
	wantErr(t, err, ErrForbidden)

	edited := got(tm.store.EditComment(tm.ctx, tm.agent, comment.ID, "Second draft")).must(t)
	if edited.Body != "Second draft" || edited.EditedAt == nil {
		t.Errorf("edited %+v", edited)
	}
}
