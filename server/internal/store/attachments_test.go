package store

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

func pngBytes() []byte {
	return append([]byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR"), make([]byte, 64)...)
}

func mp4Bytes(size int) []byte {
	return append([]byte("\x00\x00\x00\x18ftypmp42\x00\x00\x00\x00mp42isom"), make([]byte, size)...)
}

// leftovers counts files under dir, including temporary uploads.
func leftovers(t *testing.T, dir string) int {
	t.Helper()
	count := 0
	err := filepath.WalkDir(dir, func(_ string, entry os.DirEntry, err error) error {
		if err == nil && !entry.IsDir() {
			count++
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return count
}

func TestCreateAttachmentStoresFileAndRecordsActorAndEvent(t *testing.T) {
	tm := newTeam(t)
	project := tm.project(t, "WP")
	issue := tm.issue(t, project.ID, "Fix header")
	data := pngBytes()

	attachment := got(tm.store.CreateAttachment(tm.ctx, tm.agent, issue.Key(), "../../etc/shot\x00.png", bytes.NewReader(data))).must(t)

	sum := sha256.Sum256(data)
	if attachment.Filename != "shot.png" || attachment.ContentType != "image/png" || attachment.ByteSize != int64(len(data)) ||
		attachment.SHA256 != hex.EncodeToString(sum[:]) || attachment.CreatedByID != tm.agent.ActorID ||
		attachment.IssueKey != issue.Key() || attachment.ProjectKey != "WP" || attachment.URLPath() != "/api/attachments/"+attachment.ID {
		t.Fatalf("attachment %+v", attachment)
	}

	file, opened, err := tm.store.OpenAttachment(tm.ctx, attachment.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	stored, _ := io.ReadAll(file)
	if opened.ID != attachment.ID || !bytes.Equal(stored, data) {
		t.Fatalf("stored bytes differ")
	}
	if tm.store.AttachmentPath(attachment.ID) != filepath.Join(tm.files, attachment.ID[:2], attachment.ID) {
		t.Fatalf("path %s", tm.store.AttachmentPath(attachment.ID))
	}
	if leftovers(t, tm.files) != 1 {
		t.Fatalf("want exactly the stored file, found %d files", leftovers(t, tm.files))
	}

	var kind, actor, payload string
	err = tm.store.db.QueryRow(`SELECT kind, actor_id, payload FROM events ORDER BY sequence DESC LIMIT 1`).Scan(&kind, &actor, &payload)
	if err != nil || kind != "attachment.created" || actor != tm.agent.ActorID ||
		!strings.Contains(payload, `"issue_key":"WP-1"`) || !strings.Contains(payload, `"filename":"shot.png"`) {
		t.Fatalf("event %s by %s: %s (%v)", kind, actor, payload, err)
	}

	list := got(tm.store.ListIssueAttachments(tm.ctx, issue.Key())).must(t)
	all := got(tm.store.ListAllAttachments(tm.ctx)).must(t)
	if len(list) != 1 || len(all) != 1 || list[0].ID != attachment.ID {
		t.Fatalf("lists %v %v", list, all)
	}
}

func TestCreateAttachmentSniffsTheContentType(t *testing.T) {
	tm := newTeam(t)
	issue := tm.issue(t, tm.project(t, "WP").ID, "Fix header")

	cases := map[string]struct {
		data []byte
		want string
	}{
		"html":      {[]byte("<!DOCTYPE html><html><script>alert(1)</script></html>"), "text/html"},
		"text":      {[]byte("hello"), "text/plain"},
		"pdf":       {[]byte("%PDF-1.7 rest"), "application/pdf"},
		"mp4":       {mp4Bytes(10), "video/mp4"},
		"quicktime": {append([]byte("\x00\x00\x00\x14ftypqt  \x00\x00\x00\x00qt  "), 0), "video/quicktime"},
		"webm":      {append([]byte{0x1A, 0x45, 0xDF, 0xA3}, make([]byte, 20)...), "video/webm"},
		"binary":    {[]byte{0, 1, 2, 3, 0xff, 0xfe}, "application/octet-stream"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			// The filename claims an image; the bytes decide.
			attachment := got(tm.store.CreateAttachment(tm.ctx, tm.member, issue.Key(), "picture.png", bytes.NewReader(c.data))).must(t)
			if attachment.ContentType != c.want {
				t.Fatalf("content type %q, want %q", attachment.ContentType, c.want)
			}
		})
	}
}

func TestCreateAttachmentRefusesBadInput(t *testing.T) {
	tm := newTeam(t)
	issue := tm.issue(t, tm.project(t, "WP").ID, "Fix header")
	before := tm.eventCount(t)

	_, err := tm.store.CreateAttachment(tm.ctx, tm.member, issue.Key(), "empty.txt", bytes.NewReader(nil))
	wantInvalid(t, err, "body")
	for _, name := range []string{"", "   ", "..", "dir/", "\x00\x01"} {
		_, err = tm.store.CreateAttachment(tm.ctx, tm.member, issue.Key(), name, strings.NewReader("x"))
		wantInvalid(t, err, "filename")
	}
	_, err = tm.store.CreateAttachment(tm.ctx, tm.member, issue.Key(), "bad\xff.txt", strings.NewReader("x"))
	wantInvalid(t, err, "filename")
	_, err = tm.store.CreateAttachment(tm.ctx, tm.member, "WP-99", "a.txt", strings.NewReader("x"))
	wantErr(t, err, ErrNotFound)
	_, err = tm.store.CreateAttachment(tm.ctx, tm.member, "nonsense", "a.txt", strings.NewReader("x"))
	wantInvalid(t, err, "key")

	if tm.eventCount(t) != before || leftovers(t, tm.files) != 0 {
		t.Fatalf("a refused upload left events or files behind")
	}
}

func TestCleanFilename(t *testing.T) {
	long := strings.Repeat("é", 200) + ".png"
	cases := map[string]string{
		`C:\Users\me\shot.png`: "shot.png",
		"/etc/passwd":          "passwd",
		"a\tb\u202Ec.txt":      "abc.txt",
		"  spaced name.txt  ":  "spaced name.txt",
		"report \u00fcber.pdf": "report \u00fcber.pdf",
	}
	for raw, want := range cases {
		if got, err := cleanFilename(raw); err != nil || got != want {
			t.Errorf("cleanFilename(%q) = %q, %v; want %q", raw, got, err, want)
		}
	}

	name, err := cleanFilename(long)
	if err != nil || len(name) > maxFilenameBytes || !strings.HasSuffix(name, ".png") || strings.ContainsRune(name, '\uFFFD') {
		t.Errorf("long name %q (%d bytes), %v", name, len(name), err)
	}
}

func TestAttachmentSizeLimits(t *testing.T) {
	if testing.Short() {
		t.Skip("writes over 100 MB")
	}
	tm := newTeam(t)
	issue := tm.issue(t, tm.project(t, "WP").ID, "Fix header")

	// A file of exactly the limit is accepted; one byte more is refused and leaves nothing.
	exact := io.MultiReader(strings.NewReader("hello"), io.LimitReader(zeros{}, MaxAttachmentBytes-5))
	attachment := got(tm.store.CreateAttachment(tm.ctx, tm.member, issue.Key(), "big.bin", exact)).must(t)
	if attachment.ByteSize != MaxAttachmentBytes {
		t.Fatalf("size %d", attachment.ByteSize)
	}
	if err := tm.store.DeleteAttachment(tm.ctx, tm.member, attachment.ID); err != nil {
		t.Fatal(err)
	}

	over := io.MultiReader(strings.NewReader("hello"), io.LimitReader(zeros{}, MaxAttachmentBytes-4))
	_, err := tm.store.CreateAttachment(tm.ctx, tm.member, issue.Key(), "big.bin", over)
	wantErr(t, err, ErrTooLarge)
	if leftovers(t, tm.files) != 0 {
		t.Fatalf("an oversized upload left a file behind")
	}

	// A video may be larger than an ordinary file.
	video := io.MultiReader(bytes.NewReader(mp4Bytes(10)), io.LimitReader(zeros{}, MaxAttachmentBytes))
	attachment = got(tm.store.CreateAttachment(tm.ctx, tm.member, issue.Key(), "clip.mp4", video)).must(t)
	if attachment.ContentType != "video/mp4" || attachment.ByteSize <= MaxAttachmentBytes {
		t.Fatalf("video %+v", attachment)
	}
}

type zeros struct{}

func (zeros) Read(p []byte) (int, error) { clear(p); return len(p), nil }

func TestAnInterruptedUploadLeavesNothing(t *testing.T) {
	tm := newTeam(t)
	issue := tm.issue(t, tm.project(t, "WP").ID, "Fix header")

	// The connection drops after the first bytes.
	failing := io.MultiReader(bytes.NewReader(mp4Bytes(10)), errReader{})
	_, err := tm.store.CreateAttachment(tm.ctx, tm.member, issue.Key(), "clip.mp4", failing)
	if err == nil || !errors.Is(err, errBoom) {
		t.Fatalf("error %v", err)
	}
	if leftovers(t, tm.files) != 0 {
		t.Fatal("a failed upload left a file behind")
	}
}

var errBoom = errors.New("connection reset")

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errBoom }

func TestAgentsCannotDeleteAttachments(t *testing.T) {
	tm := newTeam(t)
	issue := tm.issue(t, tm.project(t, "WP").ID, "Fix header")
	attachment := got(tm.store.CreateAttachment(tm.ctx, tm.agent, issue.Key(), "a.txt", strings.NewReader("hello"))).must(t)

	wantErr(t, tm.store.DeleteAttachment(tm.ctx, tm.agent, attachment.ID), ErrForbidden)
	if _, err := os.Stat(tm.store.AttachmentPath(attachment.ID)); err != nil {
		t.Fatalf("file removed by a refused delete: %v", err)
	}

	if err := tm.store.DeleteAttachment(tm.ctx, tm.member, attachment.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(tm.store.AttachmentPath(attachment.ID)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("file still there: %v", err)
	}
	_, err := tm.store.GetAttachment(tm.ctx, attachment.ID)
	wantErr(t, err, ErrNotFound)
	wantErr(t, tm.store.DeleteAttachment(tm.ctx, tm.member, attachment.ID), ErrNotFound)

	var kind, actor string
	if err := tm.store.db.QueryRow(`SELECT kind, actor_id FROM events ORDER BY sequence DESC LIMIT 1`).Scan(&kind, &actor); err != nil ||
		kind != "attachment.deleted" || actor != tm.member.ActorID {
		t.Fatalf("event %s by %s (%v)", kind, actor, err)
	}
}

func TestDeletingAProjectRemovesItsAttachmentFiles(t *testing.T) {
	tm := newTeam(t)
	doomed := tm.issue(t, tm.project(t, "WP").ID, "Doomed")
	other := tm.project(t, "OT")
	kept := tm.issue(t, other.ID, "Kept")

	gone := got(tm.store.CreateAttachment(tm.ctx, tm.member, doomed.Key(), "a.txt", strings.NewReader("one"))).must(t)
	stays := got(tm.store.CreateAttachment(tm.ctx, tm.member, kept.Key(), "b.txt", strings.NewReader("two"))).must(t)

	if err := tm.store.DeleteProject(tm.ctx, tm.owner, doomed.ProjectID); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(tm.store.AttachmentPath(gone.ID)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("file of the deleted project remains: %v", err)
	}
	if _, err := os.Stat(tm.store.AttachmentPath(stays.ID)); err != nil {
		t.Fatalf("file of another project removed: %v", err)
	}
	all := got(tm.store.ListAllAttachments(tm.ctx)).must(t)
	if len(all) != 1 || all[0].ID != stays.ID {
		t.Fatalf("rows %v", all)
	}
}

func TestFailedSavesLeaveNoRowAndNoFile(t *testing.T) {
	tm := newTeam(t)
	issue := tm.issue(t, tm.project(t, "WP").ID, "Fix header")
	before := tm.eventCount(t)

	// A disabled caller fails inside the transaction, after the upload was staged.
	disabled := Caller{ActorID: "00000000-0000-7000-8000-0000000000ff"}
	_, err := tm.store.CreateAttachment(tm.ctx, disabled, issue.Key(), "a.txt", strings.NewReader("hello"))
	wantErr(t, err, ErrForbidden)

	// Files where the final folders should be make the move fail, which must roll back the row.
	for _, c := range "0123456789abcdef" {
		for _, d := range "0123456789abcdef" {
			if err := os.WriteFile(filepath.Join(tm.files, string(c)+string(d)), nil, 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}
	_, err = tm.store.CreateAttachment(tm.ctx, tm.member, issue.Key(), "a.txt", strings.NewReader("hello"))
	if err == nil {
		t.Fatal("the move into a blocked folder succeeded")
	}

	var rows int
	if err := tm.store.db.QueryRow(`SELECT count(*) FROM attachments`).Scan(&rows); err != nil || rows != 0 {
		t.Fatalf("rows %d (%v)", rows, err)
	}
	if tm.eventCount(t) != before {
		t.Fatal("a failed save left an event behind")
	}
	// Only the blocking files themselves remain; no temporary upload does.
	entries, _ := os.ReadDir(tm.files)
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".upload-") {
			t.Fatalf("temporary file %s left behind", entry.Name())
		}
	}
}
