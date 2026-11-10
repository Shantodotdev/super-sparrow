// Package files keeps attached files on disk, one file per attachment, named by the attachment's
// ID. Nothing from a request ever becomes part of a path.
package files

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
)

// SniffBytes is how much of a file Stage reads before it decides whether to accept the file.
const SniffBytes = 512

// ErrTooLarge means a file is bigger than the limit chosen for it.
var ErrTooLarge = errors.New("file is too large")

// ErrEmpty means a file has no bytes.
var ErrEmpty = errors.New("file is empty")

var idPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// Dir is the folder that holds the files. Files are stored at <root>/<first two characters of
// the ID>/<ID>, and uploads are first written to temporary files in the root itself, so the final
// rename never crosses a file system.
type Dir struct {
	root string
}

// New returns a Dir over root, which must be an existing folder. The server's settings check that
// at startup; New does not touch the disk.
func New(root string) *Dir { return &Dir{root: root} }

// Root returns the folder that holds the files.
func (d *Dir) Root() string { return d.root }

// Path returns the absolute path of the file stored for the attachment ID. It returns "" for
// anything that is not a lowercase UUID, so a malformed ID can never reach outside the folder.
func (d *Dir) Path(id string) string {
	if !idPattern.MatchString(id) {
		return ""
	}
	return filepath.Join(d.root, id[:2], id)
}

// Staged is an upload that has been fully written and hashed to a temporary file but is not yet
// stored under an ID. Commit or Discard it exactly once.
type Staged struct {
	dir    *Dir
	temp   string
	Size   int64
	SHA256 string
	// Head holds up to SniffBytes of the file's first bytes.
	Head []byte
}

// Stage copies src to a temporary file while counting its bytes and hashing it with SHA-256. It
// reads the first SniffBytes of src and passes them to limit, which returns the largest size the
// file may have, or an error to refuse the file before anything is written. A file over that size
// stops the copy, removes the temporary file, and returns ErrTooLarge; an empty one returns
// ErrEmpty. The data is synced to disk before Stage returns.
func (d *Dir) Stage(src io.Reader, limit func(head []byte) (int64, error)) (staged *Staged, err error) {
	head := make([]byte, SniffBytes)
	n, err := io.ReadFull(src, head)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("read upload: %w", err)
	}
	head = head[:n]
	if n == 0 {
		return nil, ErrEmpty
	}

	maxBytes, err := limit(head)
	if err != nil {
		return nil, err
	}

	temp, err := os.CreateTemp(d.root, ".upload-*")
	if err != nil {
		return nil, fmt.Errorf("create temporary file: %w", err)
	}
	defer func() {
		if err != nil {
			temp.Close()
			os.Remove(temp.Name())
		}
	}()

	hash := sha256.New()
	counted := &countingWriter{limit: maxBytes}
	if _, err = io.Copy(io.MultiWriter(counted, temp, hash), io.MultiReader(bytes.NewReader(head), src)); err != nil {
		if errors.Is(err, ErrTooLarge) {
			return nil, ErrTooLarge
		}
		return nil, fmt.Errorf("write upload: %w", err)
	}

	if err = temp.Sync(); err != nil {
		return nil, fmt.Errorf("sync upload: %w", err)
	}
	if err = temp.Close(); err != nil {
		return nil, fmt.Errorf("close upload: %w", err)
	}

	return &Staged{dir: d, temp: temp.Name(), Size: counted.size, SHA256: hex.EncodeToString(hash.Sum(nil)), Head: head}, nil
}

// Commit moves the upload to the final path for id. If a file is already there it is replaced.
func (s *Staged) Commit(id string) error {
	final := s.dir.Path(id)
	if final == "" {
		return fmt.Errorf("store file: %q is not an attachment ID", id)
	}

	if err := os.MkdirAll(filepath.Dir(final), 0o750); err != nil {
		return fmt.Errorf("create folder for %s: %w", id, err)
	}
	if err := os.Rename(s.temp, final); err != nil {
		return fmt.Errorf("move upload to %s: %w", final, err)
	}

	// Syncing the folder makes the rename survive a crash; the file's data is already synced.
	if folder, err := os.Open(filepath.Dir(final)); err == nil {
		folder.Sync()
		folder.Close()
	}
	return nil
}

// Discard removes the temporary file. It is safe to call after a failed or successful Commit.
func (s *Staged) Discard() {
	os.Remove(s.temp)
}

// Open opens the stored file for reading.
func (d *Dir) Open(id string) (*os.File, error) {
	path := d.Path(id)
	if path == "" {
		return nil, fmt.Errorf("open file: %q is not an attachment ID", id)
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open file %s: %w", id, err)
	}
	return file, nil
}

// Remove deletes the stored file. A file that is already gone is not an error.
func (d *Dir) Remove(id string) error {
	path := d.Path(id)
	if path == "" {
		return fmt.Errorf("remove file: %q is not an attachment ID", id)
	}

	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove file %s: %w", id, err)
	}
	return nil
}

// countingWriter counts bytes and refuses to go on once the file is over its limit, which stops
// the copy at the first byte past it.
type countingWriter struct {
	size  int64
	limit int64
}

func (c *countingWriter) Write(p []byte) (int, error) {
	c.size += int64(len(p))
	if c.size > c.limit {
		return 0, ErrTooLarge
	}
	return len(p), nil
}
