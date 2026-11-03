package httpserver

import (
	"fmt"
	"net/http"

	"github.com/shantodotdev/super-sparrow/server/internal/store"
)

// subissueView exposes content and persistence metadata, without the full issue's properties.
type subissueView struct {
	ID                  string   `json:"id"`
	Key                 string   `json:"key"`
	IssueID             string   `json:"issue_id"`
	Title               string   `json:"title"`
	Description         string   `json:"description"`
	DescriptionRevision int      `json:"description_revision"`
	Status              string   `json:"status"`
	LabelIDs            []string `json:"label_ids"`
	CreatedByActorID    string   `json:"created_by_actor_id"`
}

func newSubissueView(subissue store.Subissue, issueKey string) subissueView {
	return subissueView{ID: subissue.ID, Key: fmt.Sprintf("%s.%d", issueKey, subissue.Number), IssueID: subissue.IssueID,
		Title: subissue.Title, Description: subissue.Description, DescriptionRevision: subissue.DescriptionRevision, Status: string(subissue.Status),
		LabelIDs: subissue.LabelIDs, CreatedByActorID: subissue.CreatedByID}
}

// listSubissues returns the lightweight subissues belonging to the issue in the path.
func (s *server) listSubissues(w http.ResponseWriter, r *http.Request) error {
	issue, err := s.issueFromPath(r)
	if err != nil {
		return err
	}
	subissues, err := s.store.ListSubissues(r.Context(), issue.ID)
	if err != nil {
		return err
	}
	views := make([]subissueView, 0, len(subissues))
	for _, subissue := range subissues {
		views = append(views, newSubissueView(subissue, issue.Key()))
	}
	writeJSON(w, http.StatusOK, map[string]any{"subissues": views})
	return nil
}

// createSubissue creates content under a full issue, using the session or token as actor.
func (s *server) createSubissue(w http.ResponseWriter, r *http.Request) error {
	var body struct {
		Title       string   `json:"title"`
		Description string   `json:"description"`
		Status      string   `json:"status"`
		LabelIDs    []string `json:"label_ids"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		return err
	}
	issue, err := s.issueFromPath(r)
	if err != nil {
		return err
	}
	subissue, err := s.store.CreateSubissue(r.Context(), identityFrom(r.Context()).caller(), issue.ID, store.NewSubissue{
		Title: body.Title, Description: body.Description, Status: store.IssueStatus(body.Status),
		LabelIDs: body.LabelIDs,
	})
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, map[string]any{"subissue": newSubissueView(subissue, issue.Key())})
	return nil
}

// updateSubissue edits only the supplied content fields, scoped to the parent issue.
func (s *server) updateSubissue(w http.ResponseWriter, r *http.Request) error {
	var body struct {
		Title               *string   `json:"title"`
		Description         *string   `json:"description"`
		DescriptionRevision int       `json:"description_revision"`
		Status              *string   `json:"status"`
		LabelIDs            *[]string `json:"label_ids"`
	}
	nulls, err := decodeUpdate(w, r, &body)
	if err != nil {
		return err
	}
	issue, err := s.issueFromPath(r)
	if err != nil {
		return err
	}
	update := store.SubissueUpdate{
		Title: patch(body.Title, nulls["title"]), Description: patch(body.Description, nulls["description"]),
		DescriptionRevision: body.DescriptionRevision, LabelIDs: patch(body.LabelIDs, nulls["label_ids"]),
	}
	if status := patch(body.Status, nulls["status"]); status.Set {
		update.Status = store.Change(store.IssueStatus(status.Value))
	}
	subissue, err := s.store.UpdateSubissue(r.Context(), identityFrom(r.Context()).caller(), issue.ID, r.PathValue("subissue"), update)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]any{"subissue": newSubissueView(subissue, issue.Key())})
	return nil
}
