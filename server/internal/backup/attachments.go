package backup

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"

	"filippo.io/age"
	"github.com/shantodotdev/super-sparrow/server/internal/files"
)

// attachmentRow is what a database copy records about one attachment's file.
type attachmentRow struct {
	ID       string
	ByteSize int64
	SHA256   string
}

// AttachmentReport counts what RestoreAttachments did with the files a database refers to.
type AttachmentReport struct {
	// Restored files were downloaded, decrypted, and verified.
	Restored int
	// Skipped files were already in the target folder with the right size and hash.
	Skipped int
	// Missing lists the IDs of attachments whose object is not in the bucket.
	Missing []string
	// Failed lists the IDs of attachments that could not be restored for another reason, with it.
	Failed map[string]error
}

// readAttachments reads the attachments recorded in the database file at path.
func readAttachments(ctx context.Context, path string) ([]attachmentRow, error) {
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer db.Close()

	rows, err := db.QueryContext(ctx, `SELECT id, byte_size, sha256 FROM attachments ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("read attachments from %s: %w", path, err)
	}
	defer rows.Close()

	var attachments []attachmentRow
	for rows.Next() {
		var row attachmentRow
		if err := rows.Scan(&row.ID, &row.ByteSize, &row.SHA256); err != nil {
			return nil, fmt.Errorf("read an attachment from %s: %w", path, err)
		}
		attachments = append(attachments, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read attachments from %s: %w", path, err)
	}
	return attachments, nil
}

// RestoreAttachments restores the files of every attachment in the database at databasePath into
// dir, which must exist, using the same layout the server reads. A file already in dir with the
// right size and SHA-256 is skipped. Every other file is downloaded from its object, decrypted
// with identities, and checked against the database's size and SHA-256 before it is moved into
// place, so a damaged or wrong object never leaves a file behind. Files are processed one at a
// time and streamed, never held in memory.
//
// Problems with single files do not stop the run. They are counted in the report, and the
// returned error says how many files are missing or failed. Only a failure that affects every
// file, such as an unreadable database, returns an error with no report.
func RestoreAttachments(ctx context.Context, bucket *Bucket, databasePath string, identities []age.Identity, dir string) (AttachmentReport, error) {
	report := AttachmentReport{Failed: map[string]error{}}

	attachments, err := readAttachments(ctx, databasePath)
	if err != nil {
		return report, err
	}
	target := files.New(dir)

	for _, row := range attachments {
		if err := ctx.Err(); err != nil {
			return report, fmt.Errorf("restore attachments: %w", err)
		}

		skipped, err := restoreAttachment(ctx, bucket, target, identities, row)
		switch {
		case errors.Is(err, ErrNotFound):
			report.Missing = append(report.Missing, row.ID)
		case err != nil:
			report.Failed[row.ID] = err
		case skipped:
			report.Skipped++
		default:
			report.Restored++
		}
	}

	if len(report.Missing) > 0 || len(report.Failed) > 0 {
		return report, fmt.Errorf("%d attachment files are missing from the bucket and %d could not be restored", len(report.Missing), len(report.Failed))
	}
	return report, nil
}

// restoreAttachment restores one file and reports whether it was already in place.
func restoreAttachment(ctx context.Context, bucket *Bucket, target *files.Dir, identities []age.Identity, row attachmentRow) (bool, error) {
	if target.Path(row.ID) == "" {
		return false, fmt.Errorf("%q is not an attachment ID", row.ID)
	}

	present, err := fileMatches(target, row)
	if err != nil {
		return false, err
	}
	if present {
		return true, nil
	}

	body, err := bucket.Get(ctx, AttachmentKey(row.ID))
	if err != nil {
		return false, err
	}
	defer body.Close()

	plain, err := age.Decrypt(body, identities...)
	if err != nil {
		return false, fmt.Errorf("decrypt %s; is this the private key of the BACKUP_AGE_RECIPIENT it was made with? %w", row.ID, err)
	}

	// Stage stops the copy at one byte past the recorded size, so a wrong object cannot fill the disk.
	staged, err := target.Stage(plain, func([]byte) (int64, error) { return row.ByteSize, nil })
	if err != nil {
		return false, fmt.Errorf("write %s: %w", row.ID, err)
	}
	if staged.Size != row.ByteSize || staged.SHA256 != row.SHA256 {
		staged.Discard()
		return false, fmt.Errorf("%s does not match the database: got %d bytes with SHA-256 %s, want %d bytes with %s",
			row.ID, staged.Size, staged.SHA256, row.ByteSize, row.SHA256)
	}

	if err := staged.Commit(row.ID); err != nil {
		staged.Discard()
		return false, err
	}
	return false, nil
}

// fileMatches reports whether the file stored for row exists with the recorded size and SHA-256.
func fileMatches(target *files.Dir, row attachmentRow) (bool, error) {
	file, err := target.Open(row.ID)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer file.Close()

	hash := sha256.New()
	size, err := io.Copy(hash, file)
	if err != nil {
		return false, fmt.Errorf("read %s: %w", row.ID, err)
	}
	return size == row.ByteSize && hex.EncodeToString(hash.Sum(nil)) == row.SHA256, nil
}
