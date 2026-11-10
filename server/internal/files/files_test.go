package files

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testID = "01900000-0000-7000-8000-000000000001"

func allowUpTo(limit int64) func([]byte) (int64, error) {
	return func([]byte) (int64, error) { return limit, nil }
}

func countFiles(t *testing.T, dir string) int {
	t.Helper()
	count := 0
	filepath.WalkDir(dir, func(_ string, entry os.DirEntry, err error) error {
		if err == nil && !entry.IsDir() {
			count++
		}
		return err
	})
	return count
}

func TestStageCommitOpenRemove(t *testing.T) {
	dir := New(t.TempDir())
	data := bytes.Repeat([]byte("abc"), 1000)

	staged, err := dir.Stage(bytes.NewReader(data), allowUpTo(10_000))
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	if staged.Size != int64(len(data)) || staged.SHA256 != hex.EncodeToString(sum[:]) || len(staged.Head) != SniffBytes {
		t.Fatalf("staged %+v", staged)
	}
	if err := staged.Commit(testID); err != nil {
		t.Fatal(err)
	}

	if want := filepath.Join(dir.Root(), "01", testID); dir.Path(testID) != want {
		t.Fatalf("path %q, want %q", dir.Path(testID), want)
	}
	if countFiles(t, dir.Root()) != 1 {
		t.Fatalf("%d files, want the stored one only", countFiles(t, dir.Root()))
	}
	file, err := dir.Open(testID)
	if err != nil {
		t.Fatal(err)
	}
	stored, _ := io.ReadAll(file)
	file.Close()
	if !bytes.Equal(stored, data) {
		t.Fatal("stored bytes differ")
	}

	if err := dir.Remove(testID); err != nil {
		t.Fatal(err)
	}
	if err := dir.Remove(testID); err != nil {
		t.Fatalf("removing a missing file: %v", err)
	}
	if _, err := dir.Open(testID); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("open after remove: %v", err)
	}
}

func TestStageEnforcesTheLimitWhileStreaming(t *testing.T) {
	dir := New(t.TempDir())

	// Exactly the limit passes; one byte more fails and leaves nothing on disk.
	staged, err := dir.Stage(strings.NewReader(strings.Repeat("x", 2000)), allowUpTo(2000))
	if err != nil {
		t.Fatal(err)
	}
	staged.Discard()

	for _, size := range []int{2001, 600, 10_000} {
		src := &countingReader{r: strings.NewReader(strings.Repeat("x", size))}
		_, err = dir.Stage(src, allowUpTo(int64(size)-1))
		if !errors.Is(err, ErrTooLarge) {
			t.Fatalf("size %d: error %v", size, err)
		}
		if countFiles(t, dir.Root()) != 0 {
			t.Fatalf("size %d: temporary file left behind", size)
		}
	}
}

type countingReader struct {
	r io.Reader
	n int
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += n
	return n, err
}

func TestStageStopsReadingOnceOverTheLimit(t *testing.T) {
	dir := New(t.TempDir())
	src := &countingReader{r: io.LimitReader(zeros{}, 1<<30)}

	_, err := dir.Stage(src, allowUpTo(1000))
	if !errors.Is(err, ErrTooLarge) || src.n > 1<<20 {
		t.Fatalf("error %v after reading %d bytes", err, src.n)
	}
}

type zeros struct{}

func (zeros) Read(p []byte) (int, error) { clear(p); return len(p), nil }

func TestStageRefusalsLeaveNothing(t *testing.T) {
	dir := New(t.TempDir())

	if _, err := dir.Stage(strings.NewReader(""), allowUpTo(10)); !errors.Is(err, ErrEmpty) {
		t.Fatalf("empty: %v", err)
	}
	refusal := errors.New("not allowed")
	if _, err := dir.Stage(strings.NewReader("data"), func([]byte) (int64, error) { return 0, refusal }); !errors.Is(err, refusal) {
		t.Fatalf("refused: %v", err)
	}
	broken := io.MultiReader(strings.NewReader("data"), failingReader{})
	if _, err := dir.Stage(broken, allowUpTo(10)); err == nil || !errors.Is(err, errBroken) {
		t.Fatalf("broken: %v", err)
	}
	if countFiles(t, dir.Root()) != 0 {
		t.Fatal("files left behind")
	}
}

var errBroken = errors.New("connection reset")

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errBroken }

func TestStageHeadAndLimitDecision(t *testing.T) {
	dir := New(t.TempDir())
	var seen []byte

	staged, err := dir.Stage(strings.NewReader("hello"), func(head []byte) (int64, error) {
		seen = head
		return 5, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer staged.Discard()
	if string(seen) != "hello" || string(staged.Head) != "hello" {
		t.Fatalf("head %q / %q", seen, staged.Head)
	}
}

func TestCommitFailureLeavesTheTemporaryFileToDiscard(t *testing.T) {
	dir := New(t.TempDir())
	staged, err := dir.Stage(strings.NewReader("data"), allowUpTo(10))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir.Root(), "01"), nil, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := staged.Commit(testID); err == nil {
		t.Fatal("commit into a blocked folder succeeded")
	}
	staged.Discard()
	if countFiles(t, dir.Root()) != 1 {
		t.Fatal("only the blocking file should remain")
	}
}

func TestPathRefusesAnythingButAnID(t *testing.T) {
	dir := New(t.TempDir())
	for _, id := range []string{"", "../etc/passwd", "01/../../x", "0190AAAA-0000-7000-8000-00000000000A", testID + "/x"} {
		if dir.Path(id) != "" {
			t.Errorf("Path(%q) = %q", id, dir.Path(id))
		}
		if _, err := dir.Open(id); err == nil {
			t.Errorf("Open(%q) succeeded", id)
		}
		if err := dir.Remove(id); err == nil {
			t.Errorf("Remove(%q) succeeded", id)
		}
	}
}
