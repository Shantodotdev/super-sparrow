package httpserver

import (
	"net/http"

	"github.com/shantodotdev/super-sparrow/server/internal/store"
)

type newCommentBody struct {
	Body            string `json:"body"`
	ParentCommentID string `json:"parent_comment_id"`
}

func (s *server) listIssueComments(w http.ResponseWriter, r *http.Request) error {
	issue, err := s.issueFromPath(r)
	if err != nil {
		return err
	}
	return s.writeComments(w, r, issue.ID, "")
}

func (s *server) listFindingComments(w http.ResponseWriter, r *http.Request) error {
	_, finding, err := s.findingFromPath(r)
	if err != nil {
		return err
	}
	return s.writeComments(w, r, "", finding.ID)
}

func (s *server) writeComments(w http.ResponseWriter, r *http.Request, issueID, findingID string) error {
	comments, err := s.store.ListComments(r.Context(), issueID, findingID)
	if err != nil {
		return err
	}

	views := make([]commentView, 0, len(comments))
	for _, comment := range comments {
		views = append(views, newCommentView(comment))
	}
	writeJSON(w, http.StatusOK, map[string]any{"comments": views})
	return nil
}

func (s *server) addIssueComment(w http.ResponseWriter, r *http.Request) error {
	var body newCommentBody
	if err := decodeJSON(w, r, &body); err != nil {
		return err
	}

	issue, err := s.issueFromPath(r)
	if err != nil {
		return err
	}
	return s.addComment(w, r, store.NewComment{IssueID: issue.ID, ParentCommentID: body.ParentCommentID, Body: body.Body})
}

func (s *server) addFindingComment(w http.ResponseWriter, r *http.Request) error {
	var body newCommentBody
	if err := decodeJSON(w, r, &body); err != nil {
		return err
	}

	_, finding, err := s.findingFromPath(r)
	if err != nil {
		return err
	}
	return s.addComment(w, r, store.NewComment{FindingID: finding.ID, ParentCommentID: body.ParentCommentID, Body: body.Body})
}

func (s *server) addComment(w http.ResponseWriter, r *http.Request, input store.NewComment) error {
	comment, err := s.store.AddComment(r.Context(), identityFrom(r.Context()).caller(), input)
	if err != nil {
		return err
	}

	writeJSON(w, http.StatusCreated, map[string]any{"comment": newCommentView(comment)})
	return nil
}

func (s *server) editComment(w http.ResponseWriter, r *http.Request) error {
	var body struct {
		Body string `json:"body"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		return err
	}

	comment, err := s.store.EditComment(r.Context(), identityFrom(r.Context()).caller(), r.PathValue("id"), body.Body)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]any{"comment": newCommentView(comment)})
	return nil
}
