package httpserver

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/shantodotdev/super-sparrow/server/internal/store"
)

type attachmentView struct {
	ID               string    `json:"id"`
	ProjectKey       string    `json:"project_key"`
	IssueKey         string    `json:"issue_key"`
	Filename         string    `json:"filename"`
	ContentType      string    `json:"content_type"`
	ByteSize         int64     `json:"byte_size"`
	URL              string    `json:"url"`
	CreatedByActorID string    `json:"created_by_actor_id"`
	CreatedAt        time.Time `json:"created_at"`
}

func newAttachmentView(a store.Attachment) attachmentView {
	return attachmentView{ID: a.ID, ProjectKey: a.ProjectKey, IssueKey: a.IssueKey, Filename: a.Filename,
		ContentType: a.ContentType, ByteSize: a.ByteSize, URL: a.URLPath(),
		CreatedByActorID: a.CreatedByID, CreatedAt: a.CreatedAt}
}

// inlineTypes are the content types a browser may show in the page. Everything else, including
// HTML, SVG, and scripts, is sent as a download.
var inlineTypes = map[string]bool{
	"image/png": true, "image/jpeg": true, "image/gif": true, "image/webp": true, "application/pdf": true,
}

func isInline(contentType string) bool {
	return inlineTypes[contentType] || strings.HasPrefix(contentType, "video/") || strings.HasPrefix(contentType, "audio/")
}

type beginAttachmentRequest struct {
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	ByteSize    int64  `json:"byte_size"`
}

// beginAttachment records a pending attachment and answers with the link the client sends the
// file to. The file goes straight to the bucket; completeAttachment confirms it.
func (s *server) beginAttachment(w http.ResponseWriter, r *http.Request) error {
	var req beginAttachmentRequest
	if err := decodeJSON(w, r, &req); err != nil {
		return err
	}

	attachment, upload, err := s.store.BeginAttachment(r.Context(), identityFrom(r.Context()).caller(), r.PathValue("key"),
		req.Filename, req.ContentType, req.ByteSize)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"attachment": newAttachmentView(attachment),
		"upload":     map[string]any{"url": upload.URL, "method": http.MethodPut, "headers": upload.Headers},
	})
	return nil
}

func (s *server) completeAttachment(w http.ResponseWriter, r *http.Request) error {
	attachment, err := s.store.CompleteAttachment(r.Context(), identityFrom(r.Context()).caller(), r.PathValue("id"))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]any{"attachment": newAttachmentView(attachment)})
	return nil
}

func (s *server) listAttachments(w http.ResponseWriter, r *http.Request) error {
	attachments, err := s.store.ListIssueAttachments(r.Context(), r.PathValue("key"))
	if err != nil {
		return err
	}

	views := make([]attachmentView, 0, len(attachments))
	for _, attachment := range attachments {
		views = append(views, newAttachmentView(attachment))
	}
	writeJSON(w, http.StatusOK, map[string]any{"attachments": views})
	return nil
}

// downloadAttachment sends the caller to a short-lived signed link to the file in the bucket,
// which answers Range requests for seeking in videos and resuming downloads. Only images, video,
// audio, and PDF are shown in the page; every other type is forced to download with a generic
// content type, so a browser never runs an uploaded page or script.
func (s *server) downloadAttachment(w http.ResponseWriter, r *http.Request) error {
	attachment, err := s.store.GetAttachment(r.Context(), r.PathValue("id"))
	if err != nil {
		return err
	}

	contentType, disposition := "application/octet-stream", "attachment; filename*=UTF-8''"+encodeFilename(attachment.Filename)
	if isInline(attachment.ContentType) {
		contentType, disposition = attachment.ContentType, "inline"
	}

	w.Header().Set("Cache-Control", "private, no-store")
	http.Redirect(w, r, s.store.DownloadURL(attachment, contentType, disposition), http.StatusFound)
	return nil
}

func (s *server) deleteAttachment(w http.ResponseWriter, r *http.Request) error {
	if err := s.store.DeleteAttachment(r.Context(), identityFrom(r.Context()).caller(), r.PathValue("id")); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// encodeFilename percent-encodes a name for the filename* parameter of Content-Disposition
// (RFC 5987): everything but unreserved characters.
func encodeFilename(name string) string {
	var out strings.Builder
	for _, b := range []byte(name) {
		switch {
		case b >= 'a' && b <= 'z', b >= 'A' && b <= 'Z', b >= '0' && b <= '9', strings.IndexByte("-._~", b) >= 0:
			out.WriteByte(b)
		default:
			out.WriteString("%" + strings.ToUpper(strconv.FormatInt(int64(b)+0x100, 16)[1:]))
		}
	}
	return out.String()
}
