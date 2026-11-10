package httpserver

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/shantodotdev/super-sparrow/server/internal/store"
)

type attachmentView struct {
	ID               string    `json:"id"`
	URL              string    `json:"url"`
	ContentType      string    `json:"content_type"`
	ByteSize         int       `json:"byte_size"`
	CreatedByActorID string    `json:"created_by_actor_id"`
	CreatedAt        time.Time `json:"created_at"`
}

func newAttachmentView(attachment store.Attachment) attachmentView {
	return attachmentView{ID: attachment.ID, URL: attachment.URLPath(), ContentType: attachment.ContentType,
		ByteSize: attachment.ByteSize, CreatedByActorID: attachment.CreatedByID, CreatedAt: attachment.CreatedAt}
}

type contextView struct {
	ID               string    `json:"id"`
	Key              string    `json:"key"`
	ProjectID        string    `json:"project_id"`
	ProjectKey       string    `json:"project_key"`
	ProjectSlug      string    `json:"project_slug"`
	Number           int       `json:"number"`
	Title            string    `json:"title"`
	Body             string    `json:"body"`
	BodyRevision     int       `json:"body_revision"`
	IssueID          string    `json:"issue_id"`
	IssueKey         string    `json:"issue_key"`
	CreatedByActorID string    `json:"created_by_actor_id"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

func newContextView(c store.Context) contextView {
	return contextView{ID: c.ID, Key: c.Key(), ProjectID: c.ProjectID, ProjectKey: c.ProjectKey, ProjectSlug: c.ProjectSlug,
		Number: c.Number, Title: c.Title, Body: c.Body, BodyRevision: c.BodyRevision, IssueID: c.IssueID, IssueKey: c.IssueKey,
		CreatedByActorID: c.CreatedByID, CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt}
}

// createAttachment stores the image in the request body for the project in the path. The body
// is the raw image, such as a screenshot pasted into an editor, not JSON.
func (s *server) createAttachment(w http.ResponseWriter, r *http.Request) error {
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, store.MaxAttachmentBytes))
	if _, tooLarge := errors.AsType[*http.MaxBytesError](err); tooLarge {
		return &store.ValidationError{Field: "body", Message: fmt.Sprintf("must be at most %d bytes", store.MaxAttachmentBytes)}
	}
	if err != nil {
		return fmt.Errorf("read attachment body: %w", err)
	}
	project, err := s.projectFromPath(r)
	if err != nil {
		return err
	}

	attachment, err := s.store.CreateAttachment(r.Context(), identityFrom(r.Context()).caller(), project.ID, data)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, map[string]any{"attachment": newAttachmentView(attachment)})
	return nil
}

// getAttachment serves an image. Each image is stored once and never changes, so browsers may
// keep it; the policy headers stop a browser from treating it as anything but an image.
func (s *server) getAttachment(w http.ResponseWriter, r *http.Request) error {
	attachment, err := s.store.GetAttachment(r.Context(), r.PathValue("id"))
	if err != nil {
		return err
	}

	w.Header().Set("Content-Type", attachment.ContentType)
	w.Header().Set("Content-Length", strconv.Itoa(len(attachment.Data)))
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	w.Header().Set("Content-Disposition", "inline")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(attachment.Data)
	return nil
}

func (s *server) listContexts(w http.ResponseWriter, r *http.Request) error {
	project, err := s.projectFromPath(r)
	if err != nil {
		return err
	}
	limit, err := queryLimit(r)
	if err != nil {
		return err
	}
	offset, err := queryInt(r, "offset")
	if err != nil {
		return err
	}

	filter := store.ContextFilter{ProjectID: project.ID, Search: r.URL.Query().Get("search"), Limit: limit, Offset: offset}
	if key := r.URL.Query().Get("issue"); key != "" {
		issue, err := s.store.GetIssueByKey(r.Context(), key)
		if err != nil {
			return renameField(err, "key", "issue")
		}
		filter.IssueID = issue.ID
	}

	contexts, err := s.store.ListContexts(r.Context(), filter)
	if err != nil {
		return err
	}
	views := make([]contextView, 0, len(contexts))
	for _, c := range contexts {
		views = append(views, newContextView(c))
	}
	writeJSON(w, http.StatusOK, map[string]any{"contexts": views})
	return nil
}

func (s *server) createContext(w http.ResponseWriter, r *http.Request) error {
	var body struct {
		Title   string `json:"title"`
		Body    string `json:"body"`
		IssueID string `json:"issue_id"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		return err
	}
	project, err := s.projectFromPath(r)
	if err != nil {
		return err
	}

	created, err := s.store.CreateContext(r.Context(), identityFrom(r.Context()).caller(), store.NewContext{
		ProjectID: project.ID, Title: body.Title, Body: body.Body, IssueID: body.IssueID,
	})
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, map[string]any{"context": newContextView(created)})
	return nil
}

func (s *server) getContext(w http.ResponseWriter, r *http.Request) error {
	found, err := s.store.GetContextByKey(r.Context(), r.PathValue("key"))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]any{"context": newContextView(found)})
	return nil
}

func (s *server) updateContext(w http.ResponseWriter, r *http.Request) error {
	var body struct {
		Title        *string `json:"title"`
		Body         *string `json:"body"`
		BodyRevision int     `json:"body_revision"`
		IssueID      *string `json:"issue_id"`
	}
	nulls, err := decodeUpdate(w, r, &body)
	if err != nil {
		return err
	}
	found, err := s.store.GetContextByKey(r.Context(), r.PathValue("key"))
	if err != nil {
		return err
	}

	updated, err := s.store.UpdateContext(r.Context(), identityFrom(r.Context()).caller(), found.ID, store.ContextUpdate{
		Title: patch(body.Title, nulls["title"]), Body: patch(body.Body, nulls["body"]), BodyRevision: body.BodyRevision,
		IssueID: patch(body.IssueID, nulls["issue_id"]),
	})
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]any{"context": newContextView(updated)})
	return nil
}

func (s *server) deleteContext(w http.ResponseWriter, r *http.Request) error {
	found, err := s.store.GetContextByKey(r.Context(), r.PathValue("key"))
	if err != nil {
		return err
	}
	if err := s.store.DeleteContext(r.Context(), identityFrom(r.Context()).caller(), found.ID); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}
