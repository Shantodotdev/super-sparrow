package store

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/shantodotdev/super-sparrow/server/internal/r2"
)

const (
	// MaxAttachmentBytes is the largest file a person or agent can attach.
	MaxAttachmentBytes = 100_000_000
	// MaxVideoBytes is the largest video a person or agent can attach.
	MaxVideoBytes = 5_000_000_000

	maxFilenameBytes = 255
	sniffBytes       = 512

	// uploadTTL is how long an upload link works. It starts the upload; a long transfer already
	// under way is not cut off when it passes.
	uploadTTL = 6 * time.Hour
	// downloadTTL is how long a download link works.
	downloadTTL = 10 * time.Minute
)

// Objects is the bucket that holds the bytes of attached files, one object per attachment, keyed
// by the attachment's ID. *r2.Bucket is the production implementation.
type Objects interface {
	// PresignPut returns a link for uploading the object with a PUT that carries exactly the
	// given Content-Type.
	PresignPut(key, contentType string, ttl time.Duration) string
	// PresignGet returns a link for downloading the object, answered with the given content
	// type and Content-Disposition.
	PresignGet(key, contentType, disposition string, ttl time.Duration) string
	// Head returns the object's size, or an error wrapping r2.ErrNotFound.
	Head(ctx context.Context, key string) (int64, error)
	// GetPrefix returns the first limit bytes of the object.
	GetPrefix(ctx context.Context, key string, limit int64) ([]byte, error)
	Delete(ctx context.Context, key string) error
}

// Attachment is a file attached to an issue. The bytes are in the bucket; this is what the
// database keeps about them. A pending attachment has a row and an upload link but no confirmed
// file yet, and is hidden everywhere except to the person who is uploading it.
type Attachment struct {
	ID          string
	ProjectID   string
	ProjectKey  string
	IssueID     string
	IssueKey    string
	Filename    string
	ContentType string
	ByteSize    int64
	Pending     bool
	CreatedByID string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// URLPath returns the path the API serves the file at.
func (a Attachment) URLPath() string { return "/api/attachments/" + a.ID }

// Upload tells a client where and how to send the bytes of a pending attachment.
type Upload struct {
	URL     string
	Headers map[string]string
}

const attachmentSelect = `SELECT a.id, a.project_id, p.key, a.issue_id, p.key || '-' || i.number, a.filename,
	a.content_type, a.byte_size, a.uploaded_at IS NULL, a.created_by_actor_id, a.created_at, a.updated_at
	FROM attachments a JOIN projects p ON p.id = a.project_id JOIN issues i ON i.id = a.issue_id`

// BeginAttachment records a pending attachment on the issue and returns the link the client sends
// the file to. The file goes straight to the bucket, never through this server. The declared
// size and type only choose the limit; CompleteAttachment checks the real file.
func (s *Store) BeginAttachment(ctx context.Context, by Caller, issueKey, filename, contentType string, size int64) (Attachment, Upload, error) {
	name, err := cleanFilename(filename)
	if err != nil {
		return Attachment{}, Upload{}, err
	}
	contentType = declaredContentType(contentType)
	if size < 1 {
		return Attachment{}, Upload{}, invalid("byte_size", "must be at least 1")
	}
	if size > attachmentLimit(contentType) {
		return Attachment{}, Upload{}, errTooLarge()
	}
	issue, err := s.GetIssueByKey(ctx, issueKey)
	if err != nil {
		return Attachment{}, Upload{}, err
	}

	var attachment Attachment
	err = s.write(ctx, by, func(w *writeTx) error {
		current, err := getIssue(w.ctx, w.tx, `i.id = ?`, issue.ID)
		if err != nil {
			return err
		}

		id := newID()
		_, err = w.exec(`INSERT INTO attachments (id, project_id, issue_id, filename, content_type, byte_size,
			uploaded_at, created_by_actor_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, NULL, ?, ?, ?)`,
			id, current.ProjectID, current.ID, name, contentType, size, w.caller.ID, w.timestamp(), w.timestamp())
		if err != nil {
			return fmt.Errorf("store attachment: %w", err)
		}
		attachment, err = getAttachment(w.ctx, w.tx, `a.id = ?`, id)
		return err
	})
	if err != nil {
		return Attachment{}, Upload{}, err
	}

	upload := Upload{
		URL:     s.objects.PresignPut(attachment.ID, contentType, uploadTTL),
		Headers: map[string]string{"Content-Type": contentType},
	}
	return attachment, upload, nil
}

// CompleteAttachment confirms that the pending attachment's file reached the bucket and makes the
// attachment visible. Only the person or agent that began it can complete it. The bucket is asked
// for the object's size and first bytes, so the type is sniffed from the file itself rather than
// trusted from the request. A file that breaks the size rules is deleted along with its row.
// Completing an attachment that is already complete returns it unchanged.
func (s *Store) CompleteAttachment(ctx context.Context, by Caller, id string) (Attachment, error) {
	attachment, err := getAttachment(ctx, s.db, `a.id = ?`, id)
	if err != nil {
		return Attachment{}, err
	}
	if attachment.CreatedByID != by.ActorID {
		return Attachment{}, notFound("attachment")
	}
	if !attachment.Pending {
		return attachment, nil
	}

	size, err := s.objects.Head(ctx, attachment.ID)
	if errors.Is(err, r2.ErrNotFound) {
		return Attachment{}, invalid("attachment", "has no uploaded file yet")
	}
	if err != nil {
		return Attachment{}, fmt.Errorf("check uploaded file of attachment %s: %w", id, err)
	}
	head, err := s.objects.GetPrefix(ctx, attachment.ID, sniffBytes)
	if err != nil {
		return Attachment{}, fmt.Errorf("read uploaded file of attachment %s: %w", id, err)
	}

	contentType := detectContentType(head)
	if size < 1 || size > attachmentLimit(contentType) {
		if err := s.discardAttachment(ctx, attachment.ID); err != nil {
			return Attachment{}, err
		}
		if size < 1 {
			return Attachment{}, invalid("body", "must hold a file")
		}
		return Attachment{}, errTooLarge()
	}

	err = s.write(ctx, by, func(w *writeTx) error {
		result, err := w.exec(`UPDATE attachments SET content_type = ?, byte_size = ?, uploaded_at = ?, updated_at = ?
			WHERE id = ? AND uploaded_at IS NULL`, contentType, size, w.timestamp(), w.timestamp(), attachment.ID)
		if err != nil {
			return fmt.Errorf("complete attachment %s: %w", id, err)
		}
		if changed, _ := result.RowsAffected(); changed == 0 {
			return nil
		}
		if attachment, err = getAttachment(w.ctx, w.tx, `a.id = ?`, attachment.ID); err != nil {
			return err
		}
		return w.event(attachment.ProjectID, TargetProject, attachment.ProjectID, "attachment.created", attachmentPayload(attachment))
	})
	if err != nil {
		return Attachment{}, err
	}
	return attachment, nil
}

// discardAttachment removes a pending attachment's row and object.
func (s *Store) discardAttachment(ctx context.Context, id string) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM attachments WHERE id = ? AND uploaded_at IS NULL`, id); err != nil {
		return fmt.Errorf("delete pending attachment %s: %w", id, err)
	}
	return s.removeAttachmentFiles(ctx, []string{id})
}

// GetAttachment returns a completed attachment's details, without its bytes.
func (s *Store) GetAttachment(ctx context.Context, id string) (Attachment, error) {
	attachment, err := getAttachment(ctx, s.db, `a.id = ?`, id)
	if err != nil {
		return Attachment{}, err
	}
	if attachment.Pending {
		return Attachment{}, notFound("attachment")
	}
	return attachment, nil
}

// DownloadURL returns a short-lived link to the attachment's file in the bucket, answered with
// the given content type and Content-Disposition.
func (s *Store) DownloadURL(a Attachment, contentType, disposition string) string {
	return s.objects.PresignGet(a.ID, contentType, disposition, downloadTTL)
}

// ReadAttachmentPrefix reads at most limit bytes from the start of the attachment's file and
// reports whether the file has more.
func (s *Store) ReadAttachmentPrefix(ctx context.Context, a Attachment, limit int64) ([]byte, bool, error) {
	data, err := s.objects.GetPrefix(ctx, a.ID, limit+1)
	if err != nil {
		return nil, false, fmt.Errorf("read attachment %s: %w", a.ID, err)
	}
	if int64(len(data)) > limit {
		return data[:limit], true, nil
	}
	return data, false, nil
}

// ListIssueAttachments returns the issue's completed attachments, oldest first.
func (s *Store) ListIssueAttachments(ctx context.Context, issueKey string) ([]Attachment, error) {
	issue, err := s.GetIssueByKey(ctx, issueKey)
	if err != nil {
		return nil, err
	}
	return listAttachments(ctx, s.db, `WHERE a.issue_id = ? AND a.uploaded_at IS NOT NULL`, issue.ID)
}

// DeleteAttachment removes an attachment and, once that is committed, its file. Only a person
// can delete one. A pending attachment can be abandoned by whoever began it.
func (s *Store) DeleteAttachment(ctx context.Context, by Caller, id string) error {
	pending, err := getAttachment(ctx, s.db, `a.id = ?`, id)
	if err != nil {
		return err
	}
	if pending.Pending && pending.CreatedByID == by.ActorID {
		return s.discardAttachment(ctx, id)
	}

	err = s.write(ctx, by, func(w *writeTx) error {
		if err := w.requireHuman("delete attachments"); err != nil {
			return err
		}
		attachment, err := getAttachment(w.ctx, w.tx, `a.id = ?`, id)
		if err != nil {
			return err
		}
		if attachment.Pending {
			return notFound("attachment")
		}

		if _, err := w.exec(`DELETE FROM attachments WHERE id = ?`, attachment.ID); err != nil {
			return fmt.Errorf("delete attachment %s: %w", id, err)
		}
		return w.event(attachment.ProjectID, TargetProject, attachment.ProjectID, "attachment.deleted", attachmentPayload(attachment))
	})
	if err != nil {
		return err
	}

	return s.removeAttachmentFiles(ctx, []string{id})
}

// attachmentIDsOfProject returns the IDs of the project's attachments, for removing their files
// after the project is deleted.
func attachmentIDsOfProject(w *writeTx, projectID string) ([]string, error) {
	rows, err := w.tx.QueryContext(w.ctx, `SELECT id FROM attachments WHERE project_id = ?`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list attachments of project: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("read attachment id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list attachments of project: %w", err)
	}
	return ids, nil
}

// removeAttachmentFiles deletes the objects of attachments whose rows are already gone. It tries
// every object and reports the first failure.
func (s *Store) removeAttachmentFiles(ctx context.Context, ids []string) error {
	var first error
	for _, id := range ids {
		if err := s.objects.Delete(ctx, id); err != nil && first == nil {
			first = err
		}
	}
	return first
}

// attachmentLimit is the largest file of the content type: videos may be bigger than the rest.
func attachmentLimit(contentType string) int64 {
	if strings.HasPrefix(contentType, "video/") {
		return MaxVideoBytes
	}
	return MaxAttachmentBytes
}

func errTooLarge() error {
	return Refuse(ErrTooLarge, "files must be at most %d bytes, and videos at most %d bytes", MaxAttachmentBytes, MaxVideoBytes)
}

// declaredContentType reduces the type a client declares to a bare media type, or the generic
// binary type when it is missing or malformed. It only picks the size limit and the upload
// header; the stored type is sniffed from the file.
func declaredContentType(raw string) string {
	mediaType, _, err := mime.ParseMediaType(raw)
	if err != nil || len(mediaType) > 255 {
		return "application/octet-stream"
	}
	return mediaType
}

func attachmentPayload(a Attachment) map[string]any {
	return map[string]any{
		"attachment_id": a.ID, "issue_id": a.IssueID, "issue_key": a.IssueKey,
		"filename": a.Filename, "content_type": a.ContentType, "byte_size": a.ByteSize,
	}
}

func getAttachment(ctx context.Context, q querier, where string, args ...any) (Attachment, error) {
	return scanAttachment(q.QueryRowContext(ctx, attachmentSelect+` WHERE `+where, args...))
}

func listAttachments(ctx context.Context, q querier, where string, args ...any) ([]Attachment, error) {
	rows, err := q.QueryContext(ctx, attachmentSelect+` `+where+` ORDER BY a.created_at, a.id`, args...)
	if err != nil {
		return nil, fmt.Errorf("list attachments: %w", err)
	}
	defer rows.Close()

	attachments := []Attachment{}
	for rows.Next() {
		attachment, err := scanAttachment(rows)
		if err != nil {
			return nil, err
		}
		attachments = append(attachments, attachment)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list attachments: %w", err)
	}
	return attachments, nil
}

func scanAttachment(row scanner) (Attachment, error) {
	var a Attachment
	var created, updated string
	err := row.Scan(&a.ID, &a.ProjectID, &a.ProjectKey, &a.IssueID, &a.IssueKey, &a.Filename, &a.ContentType,
		&a.ByteSize, &a.Pending, &a.CreatedByID, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return Attachment{}, notFound("attachment")
	}
	if err != nil {
		return Attachment{}, fmt.Errorf("read attachment: %w", err)
	}

	a.CreatedAt, a.UpdatedAt, err = timestamps(created, updated)
	return a, err
}

// cleanFilename reduces a client-supplied name to a plain file name: the part after the last
// slash or backslash, without control or invisible formatting characters, at most 255 bytes long.
// The name is only shown and offered for download; files are stored by ID.
func cleanFilename(raw string) (string, error) {
	if !utf8.ValidString(raw) {
		return "", invalid("filename", "must be valid UTF-8")
	}
	if cut := strings.LastIndexAny(raw, `/\`); cut >= 0 {
		raw = raw[cut+1:]
	}

	name := strings.TrimSpace(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return -1
		}
		return r
	}, raw))
	if name == "" || name == "." || name == ".." {
		return "", invalid("filename", "must name a file")
	}

	if len(name) > maxFilenameBytes {
		// Keep the extension when it is short, so the file still opens with the right program.
		extension := ""
		if dot := strings.LastIndexByte(name, '.'); dot > 0 && len(name)-dot <= 16 {
			extension = name[dot:]
		}
		name = truncateUTF8(name[:len(name)-len(extension)], maxFilenameBytes-len(extension)) + extension
	}
	return name, nil
}

// truncateUTF8 cuts text to at most limit bytes without splitting a character.
func truncateUTF8(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	for limit > 0 && !utf8.RuneStart(text[limit]) {
		limit--
	}
	return text[:limit]
}

// detectContentType names the type of a file from its first bytes, without parameters such as a
// charset. Go's sniffing misses QuickTime and some other video containers, which are added here.
func detectContentType(head []byte) string {
	detected := http.DetectContentType(head)
	if mediaType, _, err := mime.ParseMediaType(detected); err == nil {
		detected = mediaType
	}
	if detected != "application/octet-stream" && detected != "video/mp4" {
		return detected
	}

	// ISO base media files (MP4, MOV, 3GP) start with a box size, then the box name.
	if len(head) >= 12 && bytes.Equal(head[4:8], []byte("ftyp")) {
		switch brand := string(head[8:12]); {
		case brand == "M4A " || brand == "M4B " || brand == "M4P ":
			return "audio/mp4"
		case brand == "qt  ":
			return "video/quicktime"
		default:
			return "video/mp4"
		}
	}
	if len(head) >= 8 && (bytes.Equal(head[4:8], []byte("moov")) || bytes.Equal(head[4:8], []byte("wide"))) {
		return "video/quicktime"
	}
	// Matroska and WebM share the EBML header; Go reports both as WebM.
	if len(head) >= 4 && bytes.Equal(head[:4], []byte{0x1A, 0x45, 0xDF, 0xA3}) {
		return "video/webm"
	}
	return detected
}
