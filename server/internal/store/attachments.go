package store

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/shantodotdev/super-sparrow/server/internal/files"
)

const (
	// MaxAttachmentBytes is the largest file a person or agent can attach.
	MaxAttachmentBytes = 100_000_000
	// MaxVideoBytes is the largest video a person or agent can attach.
	MaxVideoBytes = 5_000_000_000

	maxFilenameBytes = 255
)

// Attachment is a file attached to an issue. The bytes are on disk; this is what the database
// keeps about them.
type Attachment struct {
	ID          string
	ProjectID   string
	ProjectKey  string
	IssueID     string
	IssueKey    string
	Filename    string
	ContentType string
	ByteSize    int64
	SHA256      string
	CreatedByID string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// URLPath returns the path the API serves the file at.
func (a Attachment) URLPath() string { return "/api/attachments/" + a.ID }

const attachmentSelect = `SELECT a.id, a.project_id, p.key, a.issue_id, p.key || '-' || i.number, a.filename,
	a.content_type, a.byte_size, a.sha256, a.created_by_actor_id, a.created_at, a.updated_at
	FROM attachments a JOIN projects p ON p.id = a.project_id JOIN issues i ON i.id = a.issue_id`

// CreateAttachment stores the bytes of body as a file attached to the issue. The content type is
// read from the first bytes rather than trusted from the request, and the filename is reduced to
// a plain name. The file is streamed to disk, never held in memory, and is refused when it is
// over MaxAttachmentBytes, or over MaxVideoBytes for a video.
//
// The upload is written to a temporary file first. The row and its event are then saved, and the
// file is moved to its final place before the transaction commits, so a committed row always has
// its file. A failed save removes the temporary file; a crash may leave a file with no row, which
// is harmless.
func (s *Store) CreateAttachment(ctx context.Context, by Caller, issueKey, filename string, body io.Reader) (Attachment, error) {
	name, err := cleanFilename(filename)
	if err != nil {
		return Attachment{}, err
	}
	issue, err := s.GetIssueByKey(ctx, issueKey)
	if err != nil {
		return Attachment{}, err
	}

	// Stream the upload to disk before taking the database's write lock, so a slow upload never
	// blocks other changes.
	var contentType string
	staged, err := s.files.Stage(body, func(head []byte) (int64, error) {
		contentType = detectContentType(head)
		if strings.HasPrefix(contentType, "video/") {
			return MaxVideoBytes, nil
		}
		return MaxAttachmentBytes, nil
	})
	switch {
	case errors.Is(err, files.ErrEmpty):
		return Attachment{}, invalid("body", "must hold a file")
	case errors.Is(err, files.ErrTooLarge):
		return Attachment{}, Refuse(ErrTooLarge, "files must be at most %d bytes, and videos at most %d bytes", MaxAttachmentBytes, MaxVideoBytes)
	case err != nil:
		return Attachment{}, fmt.Errorf("stage attachment for %s: %w", issue.Key(), err)
	}
	defer staged.Discard()

	var attachment Attachment
	err = s.write(ctx, by, func(w *writeTx) error {
		current, err := getIssue(w.ctx, w.tx, `i.id = ?`, issue.ID)
		if err != nil {
			return err
		}

		id := newID()
		_, err = w.exec(`INSERT INTO attachments (id, project_id, issue_id, filename, content_type, byte_size, sha256,
			created_by_actor_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			id, current.ProjectID, current.ID, name, contentType, staged.Size, staged.SHA256, w.caller.ID, w.timestamp(), w.timestamp())
		if err != nil {
			return fmt.Errorf("store attachment: %w", err)
		}
		if attachment, err = getAttachment(w.ctx, w.tx, `a.id = ?`, id); err != nil {
			return err
		}

		err = w.event(current.ProjectID, TargetProject, current.ProjectID, "attachment.created", attachmentPayload(attachment))
		if err != nil {
			return err
		}

		// The last step before commit: if the move fails, the row and event roll back.
		return staged.Commit(id)
	})
	if err != nil {
		// The move may have happened before a failed commit; a file with no row is useless.
		if attachment.ID != "" {
			os.Remove(s.files.Path(attachment.ID))
		}
		return Attachment{}, err
	}
	return attachment, nil
}

// GetAttachment returns an attachment's details, without its bytes.
func (s *Store) GetAttachment(ctx context.Context, id string) (Attachment, error) {
	return getAttachment(ctx, s.db, `a.id = ?`, id)
}

// OpenAttachment opens an attachment's file for reading, with its details. The caller closes the
// file. The file is an *os.File so it can be served with http.ServeContent, which answers Range
// requests.
func (s *Store) OpenAttachment(ctx context.Context, id string) (*os.File, Attachment, error) {
	attachment, err := s.GetAttachment(ctx, id)
	if err != nil {
		return nil, Attachment{}, err
	}

	file, err := s.files.Open(attachment.ID)
	if err != nil {
		return nil, Attachment{}, fmt.Errorf("open attachment %s: %w", id, err)
	}
	return file, attachment, nil
}

// ListIssueAttachments returns the issue's attachments, oldest first.
func (s *Store) ListIssueAttachments(ctx context.Context, issueKey string) ([]Attachment, error) {
	issue, err := s.GetIssueByKey(ctx, issueKey)
	if err != nil {
		return nil, err
	}
	return listAttachments(ctx, s.db, `WHERE a.issue_id = ?`, issue.ID)
}

// ListAllAttachments returns every attachment, oldest first, which backups use to find the files.
func (s *Store) ListAllAttachments(ctx context.Context) ([]Attachment, error) {
	return listAttachments(ctx, s.db, ``)
}

// AttachmentPath returns the absolute path of the attachment's file on disk.
func (s *Store) AttachmentPath(id string) string { return s.files.Path(id) }

// DeleteAttachment removes an attachment and, once that is committed, its file. Only a person
// can delete one.
func (s *Store) DeleteAttachment(ctx context.Context, by Caller, id string) error {
	err := s.write(ctx, by, func(w *writeTx) error {
		if err := w.requireHuman("delete attachments"); err != nil {
			return err
		}
		attachment, err := getAttachment(w.ctx, w.tx, `a.id = ?`, id)
		if err != nil {
			return err
		}

		if _, err := w.exec(`DELETE FROM attachments WHERE id = ?`, attachment.ID); err != nil {
			return fmt.Errorf("delete attachment %s: %w", id, err)
		}
		return w.event(attachment.ProjectID, TargetProject, attachment.ProjectID, "attachment.deleted", attachmentPayload(attachment))
	})
	if err != nil {
		return err
	}

	return s.removeAttachmentFiles([]string{id})
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

// removeAttachmentFiles deletes the files of attachments whose rows are already gone. It tries
// every file and reports the first failure.
func (s *Store) removeAttachmentFiles(ids []string) error {
	var first error
	for _, id := range ids {
		if err := s.files.Remove(id); err != nil && first == nil {
			first = err
		}
	}
	return first
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
		&a.ByteSize, &a.SHA256, &a.CreatedByID, &created, &updated)
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
