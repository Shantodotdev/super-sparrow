package httpserver

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/shantodotdev/super-sparrow/server/internal/store"
)

// uploadStallTimeout is how long an upload may go without sending a byte before it is cut off.
// The deadline moves forward with every read, so a large upload is never limited by its total
// time, only a stalled one.
const uploadStallTimeout = time.Minute

type attachmentView struct {
	ID               string    `json:"id"`
	ProjectKey       string    `json:"project_key"`
	IssueKey         string    `json:"issue_key"`
	Filename         string    `json:"filename"`
	ContentType      string    `json:"content_type"`
	ByteSize         int64     `json:"byte_size"`
	SHA256           string    `json:"sha256"`
	URL              string    `json:"url"`
	CreatedByActorID string    `json:"created_by_actor_id"`
	CreatedAt        time.Time `json:"created_at"`
}

func newAttachmentView(a store.Attachment) attachmentView {
	return attachmentView{ID: a.ID, ProjectKey: a.ProjectKey, IssueKey: a.IssueKey, Filename: a.Filename,
		ContentType: a.ContentType, ByteSize: a.ByteSize, SHA256: a.SHA256, URL: a.URLPath(),
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

// idleDeadlineBody pushes the connection's read deadline forward on every read.
type idleDeadlineBody struct {
	io.ReadCloser
	controller *http.ResponseController
}

func (b idleDeadlineBody) Read(p []byte) (int, error) {
	// Ignoring the error is deliberate: servers that cannot set a deadline have none to enforce.
	_ = b.controller.SetReadDeadline(time.Now().Add(uploadStallTimeout))
	return b.ReadCloser.Read(p)
}

// uploadAttachment attaches the request body, the raw file, to the issue in the path. The file's
// name comes from the filename query parameter. The body is streamed to disk, never held in
// memory, and is exempt from the server's read timeout so a large video can finish uploading.
func (s *server) uploadAttachment(w http.ResponseWriter, r *http.Request) error {
	filename := r.URL.Query().Get("filename")
	if strings.TrimSpace(filename) == "" {
		return &store.ValidationError{Field: "filename", Message: "is required"}
	}
	if r.ContentLength > store.MaxVideoBytes {
		return store.Refuse(store.ErrTooLarge, "files must be at most %d bytes, and videos at most %d bytes", store.MaxAttachmentBytes, store.MaxVideoBytes)
	}
	if r.ContentLength == 0 {
		return &store.ValidationError{Field: "body", Message: "must hold a file"}
	}

	controller := http.NewResponseController(w)
	if err := controller.SetWriteDeadline(time.Time{}); err != nil && !errors.Is(err, http.ErrNotSupported) {
		return fmt.Errorf("lift write deadline: %w", err)
	}
	if err := controller.SetReadDeadline(time.Now().Add(uploadStallTimeout)); err != nil && !errors.Is(err, http.ErrNotSupported) {
		return fmt.Errorf("extend read deadline: %w", err)
	}
	body := idleDeadlineBody{ReadCloser: http.MaxBytesReader(w, r.Body, store.MaxVideoBytes), controller: controller}

	attachment, err := s.store.CreateAttachment(r.Context(), identityFrom(r.Context()).caller(), r.PathValue("key"), filename, body)
	if _, tooLarge := errors.AsType[*http.MaxBytesError](err); tooLarge {
		return store.Refuse(store.ErrTooLarge, "files must be at most %d bytes, and videos at most %d bytes", store.MaxAttachmentBytes, store.MaxVideoBytes)
	}
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, map[string]any{"attachment": newAttachmentView(attachment)})
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

// downloadAttachment serves a file, with Range requests for seeking in videos and resuming
// downloads. Only images, video, audio, and PDF are shown in the page; every other type is a
// download with a generic content type, so a browser never runs an uploaded page or script.
func (s *server) downloadAttachment(w http.ResponseWriter, r *http.Request) error {
	file, attachment, err := s.store.OpenAttachment(r.Context(), r.PathValue("id"))
	if err != nil {
		return err
	}
	defer file.Close()

	controller := http.NewResponseController(w)
	if err := controller.SetWriteDeadline(time.Time{}); err != nil && !errors.Is(err, http.ErrNotSupported) {
		return fmt.Errorf("lift write deadline: %w", err)
	}
	if err := controller.SetReadDeadline(time.Time{}); err != nil && !errors.Is(err, http.ErrNotSupported) {
		return fmt.Errorf("lift read deadline: %w", err)
	}

	header := w.Header()
	header.Set("ETag", `"`+attachment.SHA256+`"`)
	header.Set("Cache-Control", "private, no-cache")
	header.Set("X-Content-Type-Options", "nosniff")
	header.Set("Content-Security-Policy", "sandbox; default-src 'none'")
	if isInline(attachment.ContentType) {
		header.Set("Content-Type", attachment.ContentType)
		header.Set("Content-Disposition", "inline")
	} else {
		header.Set("Content-Type", "application/octet-stream")
		header.Set("Content-Disposition", "attachment; filename*=UTF-8''"+encodeFilename(attachment.Filename))
	}

	http.ServeContent(w, r, "", attachment.CreatedAt, file)
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
