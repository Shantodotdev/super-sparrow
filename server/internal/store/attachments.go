package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// MaxAttachmentBytes is the largest image a person or agent can attach.
const MaxAttachmentBytes = 10 << 20

// attachmentTypes are the image formats an attachment may hold. SVG is left out because it can
// carry scripts.
var attachmentTypes = map[string]bool{"image/png": true, "image/jpeg": true, "image/gif": true, "image/webp": true}

// attachmentReferencePattern finds attachment URLs in Markdown, such as
// ![Screenshot](/api/attachments/0193…), with or without the server's address in front.
var attachmentReferencePattern = regexp.MustCompile(`/api/attachments/([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})`)

// Attachment is an image stored for a project. Markdown in plans, descriptions, and contexts
// refers to it by URL.
type Attachment struct {
	ID          string
	ProjectID   string
	ContentType string
	ByteSize    int
	// Data is the image itself. Listings leave it empty; GetAttachment fills it.
	Data        []byte
	CreatedByID string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// URLPath returns the path the API serves the image at.
func (a Attachment) URLPath() string { return "/api/attachments/" + a.ID }

// CreateAttachment stores an image for the project. The format is read from the bytes rather than
// trusted from the request, and only PNG, JPEG, GIF, and WebP are accepted.
func (s *Store) CreateAttachment(ctx context.Context, by Caller, projectID string, data []byte) (Attachment, error) {
	if len(data) == 0 {
		return Attachment{}, invalid("body", "must hold an image")
	}
	if len(data) > MaxAttachmentBytes {
		return Attachment{}, invalid("body", "must be at most %d bytes", MaxAttachmentBytes)
	}
	contentType := http.DetectContentType(data)
	if !attachmentTypes[contentType] {
		return Attachment{}, invalid("body", "must be a PNG, JPEG, GIF, or WebP image")
	}

	var attachment Attachment
	err := s.write(ctx, by, func(w *writeTx) error {
		project, err := getProject(w.ctx, w.tx, `id = ?`, projectID)
		if err != nil {
			return err
		}

		attachment = Attachment{ID: newID(), ProjectID: project.ID, ContentType: contentType, ByteSize: len(data),
			CreatedByID: w.caller.ID, CreatedAt: w.now, UpdatedAt: w.now}
		_, err = w.exec(`INSERT INTO attachments (id, project_id, content_type, byte_size, data, created_by_actor_id, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			attachment.ID, project.ID, contentType, len(data), data, w.caller.ID, w.timestamp(), w.timestamp())
		if err != nil {
			return fmt.Errorf("store attachment: %w", err)
		}

		return w.event(project.ID, TargetProject, project.ID, "attachment.created", map[string]any{
			"attachment_id": attachment.ID, "content_type": contentType, "byte_size": len(data),
		})
	})
	return attachment, err
}

// GetAttachment returns an image with its bytes.
func (s *Store) GetAttachment(ctx context.Context, id string) (Attachment, error) {
	var attachment Attachment
	var created, updated string
	err := s.db.QueryRowContext(ctx, `SELECT id, project_id, content_type, byte_size, data, created_by_actor_id, created_at, updated_at
		FROM attachments WHERE id = ?`, id).Scan(&attachment.ID, &attachment.ProjectID, &attachment.ContentType,
		&attachment.ByteSize, &attachment.Data, &attachment.CreatedByID, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return Attachment{}, notFound("attachment")
	}
	if err != nil {
		return Attachment{}, fmt.Errorf("read attachment %s: %w", id, err)
	}
	attachment.CreatedAt, attachment.UpdatedAt, err = timestamps(created, updated)
	return attachment, err
}

// ReferencedAttachments returns the project's images that markdown refers to, in the order they
// first appear, with their bytes. References to unknown images or to another project's images
// are skipped, so markdown cannot pull in images from elsewhere.
func (s *Store) ReferencedAttachments(ctx context.Context, projectID string, markdown ...string) ([]Attachment, error) {
	attachments := []Attachment{}
	seen := map[string]bool{}
	for _, text := range markdown {
		for _, match := range attachmentReferencePattern.FindAllStringSubmatch(strings.ToLower(text), -1) {
			id := match[1]
			if seen[id] {
				continue
			}
			seen[id] = true

			attachment, err := s.GetAttachment(ctx, id)
			if errors.Is(err, ErrNotFound) {
				continue
			}
			if err != nil {
				return nil, err
			}
			if attachment.ProjectID == projectID {
				attachments = append(attachments, attachment)
			}
		}
	}
	return attachments, nil
}
