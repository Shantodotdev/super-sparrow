package backup

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log/slog"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"filippo.io/age"
	"github.com/shantodotdev/super-sparrow/server/internal/database"
	"github.com/shantodotdev/super-sparrow/server/internal/files"
	"github.com/shantodotdev/super-sparrow/server/internal/store"
	"github.com/shantodotdev/super-sparrow/server/migrations"
)

type attachmentFixture struct {
	t        *testing.T
	ctx      context.Context
	workDir  string
	filesDir string
	store    *store.Store
	caller   store.Caller
	issueKey string
	fake     *fakeS3
	bucket   *Bucket
	identity *age.X25519Identity
	runner   *Runner
	now      time.Time
}

func newAttachmentFixture(t *testing.T) *attachmentFixture {
	t.Helper()
	ctx := context.Background()
	workDir := t.TempDir()

	db, err := database.Open(ctx, filepath.Join(workDir, "sparrow.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := database.Migrate(ctx, db, migrations.Files); err != nil {
		t.Fatal(err)
	}

	f := &attachmentFixture{t: t, ctx: ctx, workDir: workDir, filesDir: t.TempDir(), now: time.Date(2026, 3, 1, 4, 0, 0, 0, time.UTC)}
	clock := func() time.Time { return f.now }
	f.store = store.New(db, clock, f.filesDir)

	owner, err := f.store.CreateFirstOwner(ctx, "Olive Owner", "olive@example.com")
	if err != nil {
		t.Fatal(err)
	}
	f.caller = store.Caller{ActorID: owner.ID}
	project, err := f.store.CreateProject(ctx, f.caller, store.NewProject{Key: "WP", Name: "WP Agent"})
	if err != nil {
		t.Fatal(err)
	}
	issue, err := f.store.CreateIssue(ctx, f.caller, store.NewIssue{ProjectID: project.ID, Title: "Fix header", Status: store.IssueTodo})
	if err != nil {
		t.Fatal(err)
	}
	f.issueKey = issue.Key()

	f.fake = &fakeS3{objects: map[string][]byte{}, modified: map[string]time.Time{}}
	server := httptest.NewServer(f.fake)
	t.Cleanup(server.Close)
	endpoint, _ := url.Parse(server.URL)
	f.bucket = newBucket(endpoint, "auto", "key-id", "secret", "bucket", time.Now)

	f.identity, err = age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	f.runner = NewRunner(Options{
		Store: f.store, Bucket: f.bucket, Recipient: f.identity.Recipient(), WorkDir: workDir, KeepDays: 3, DailyAt: 3 * time.Hour,
		TimeZone: time.UTC, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Now: clock,
	})
	return f
}

// attach stores a PNG with some marker bytes as an attachment and returns it with its bytes.
func (f *attachmentFixture) attach(name string, marker string) (store.Attachment, []byte) {
	f.t.Helper()
	data := append(append([]byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR"), make([]byte, 64)...), marker...)
	attachment, err := f.store.CreateAttachment(f.ctx, f.caller, f.issueKey, name, bytes.NewReader(data))
	if err != nil {
		f.t.Fatal(err)
	}
	return attachment, data
}

func (f *attachmentFixture) assertNoTempFiles() {
	f.t.Helper()
	entries, err := os.ReadDir(f.workDir)
	if err != nil {
		f.t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".sparrow-backup-") {
			f.t.Errorf("working folder left behind: %s", entry.Name())
		}
	}
}

func TestAttachmentsAreUploadedBeforeTheDatabaseOnlyWhenMissing(t *testing.T) {
	f := newAttachmentFixture(t)
	first, firstData := f.attach("one.png", "one")
	second, _ := f.attach("two.png", "two")

	if err := f.runner.Sync(f.ctx); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	if len(f.fake.puts) != 3 || f.fake.puts[2] != SyncKey {
		t.Fatalf("uploads in order: %v", f.fake.puts)
	}
	for _, attachment := range []store.Attachment{first, second} {
		if _, ok := f.fake.objects[AttachmentKey(attachment.ID)]; !ok {
			t.Fatalf("attachment %s was not uploaded", attachment.ID)
		}
	}
	if bytes.Contains(f.fake.objects[AttachmentKey(first.ID)], []byte("IHDR")) {
		t.Fatal("an uploaded attachment is readable without the key")
	}

	reader, err := age.Decrypt(bytes.NewReader(f.fake.objects[AttachmentKey(first.ID)]), f.identity)
	if err != nil {
		t.Fatal(err)
	}
	if plain, _ := io.ReadAll(reader); !bytes.Equal(plain, firstData) {
		t.Fatal("the uploaded object does not decrypt to the file")
	}

	// A second run finds both files in the bucket and uploads only the database again.
	f.fake.puts = nil
	if err := f.runner.Sync(f.ctx); err != nil {
		t.Fatalf("second Sync: %v", err)
	}
	if len(f.fake.puts) != 1 || f.fake.puts[0] != SyncKey {
		t.Fatalf("second run uploaded %v", f.fake.puts)
	}

	// An object of an impossible size is uploaded again.
	f.fake.objects[AttachmentKey(second.ID)] = []byte("x")
	f.fake.puts = nil
	if err := f.runner.Sync(f.ctx); err != nil {
		t.Fatal(err)
	}
	if len(f.fake.puts) != 2 || f.fake.puts[0] != AttachmentKey(second.ID) {
		t.Fatalf("damaged object run uploaded %v", f.fake.puts)
	}
	f.assertNoTempFiles()
}

func TestFailedAttachmentUploadFailsTheJobAndKeepsTheOldCopy(t *testing.T) {
	f := newAttachmentFixture(t)
	attachment, _ := f.attach("one.png", "one")
	f.fake.objects[SyncKey] = []byte("previous copy")

	f.fake.failPutPrefix = attachmentPrefix
	err := f.runner.Sync(f.ctx)
	if err == nil || !strings.Contains(err.Error(), attachment.ID) {
		t.Fatalf("Sync returned %v", err)
	}
	if string(f.fake.objects[SyncKey]) != "previous copy" {
		t.Fatal("the database copy was replaced although an attachment failed")
	}
	f.assertNoTempFiles()

	status, err := f.store.GetBackupStatus(f.ctx)
	if err != nil || status[store.BackupSync].LastRun.Result != store.BackupFailed {
		t.Fatalf("status %+v (%v)", status[store.BackupSync], err)
	}

	// The next job retries and succeeds.
	f.fake.failPutPrefix = ""
	if err := f.runner.Sync(f.ctx); err != nil {
		t.Fatalf("retry: %v", err)
	}
	f.assertNoTempFiles()
}

func TestMissingOrChangedAttachmentFileFailsTheJob(t *testing.T) {
	f := newAttachmentFixture(t)
	attachment, _ := f.attach("one.png", "one")

	if err := os.WriteFile(f.store.AttachmentPath(attachment.ID), []byte("short"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := f.runner.Sync(f.ctx); err == nil || !strings.Contains(err.Error(), "database says") {
		t.Fatalf("Sync with a changed file returned %v", err)
	}

	if err := os.Remove(f.store.AttachmentPath(attachment.ID)); err != nil {
		t.Fatal(err)
	}
	if err := f.runner.Sync(f.ctx); err == nil || !strings.Contains(err.Error(), attachment.ID) {
		t.Fatalf("Sync with a missing file returned %v", err)
	}
	if _, ok := f.fake.objects[SyncKey]; ok {
		t.Fatal("a database copy was uploaded that refers to an attachment that was not")
	}
	f.assertNoTempFiles()
}

func TestPruneKeepsLiveAndRecentAttachmentObjects(t *testing.T) {
	f := newAttachmentFixture(t)
	live, _ := f.attach("one.png", "one")

	oldOrphan := AttachmentKey("11111111-1111-4111-8111-111111111111")
	recentOrphan := AttachmentKey("22222222-2222-4222-8222-222222222222")
	unknownTime := AttachmentKey("33333333-3333-4333-8333-333333333333")
	stray := attachmentPrefix + "notes.txt"
	f.fake.objects[oldOrphan] = []byte("old")
	f.fake.objects[recentOrphan] = []byte("recent")
	f.fake.objects[unknownTime] = []byte("unknown")
	f.fake.objects[stray] = []byte("stray")
	f.fake.modified[oldOrphan] = f.now.AddDate(0, 0, -10)
	f.fake.modified[recentOrphan] = f.now.AddDate(0, 0, -1)
	f.fake.modified[AttachmentKey(live.ID)] = f.now.AddDate(0, 0, -100)
	f.fake.modified[stray] = f.now.AddDate(0, 0, -100)

	// The live attachment's object is old, but it is referenced, so it must stay.
	f.fake.objects[AttachmentKey(live.ID)] = []byte("live")

	if err := f.runner.Daily(f.ctx); err != nil {
		t.Fatalf("Daily: %v", err)
	}

	if _, ok := f.fake.objects[oldOrphan]; ok {
		t.Error("an old unreferenced object was kept")
	}
	for _, key := range []string{recentOrphan, unknownTime, stray, AttachmentKey(live.ID)} {
		if _, ok := f.fake.objects[key]; !ok {
			t.Errorf("%s was deleted", key)
		}
	}
}

func TestRestoreAttachmentsVerifiesAndReports(t *testing.T) {
	f := newAttachmentFixture(t)
	good, goodData := f.attach("one.png", "one")
	present, _ := f.attach("two.png", "two")
	gone, _ := f.attach("three.png", "three")
	wrong, _ := f.attach("four.png", "four")
	if err := f.runner.Daily(f.ctx); err != nil {
		t.Fatalf("Daily: %v", err)
	}

	// One object disappears, one is replaced by a different file's bytes.
	delete(f.fake.objects, AttachmentKey(gone.ID))
	f.fake.objects[AttachmentKey(wrong.ID)] = f.fake.objects[AttachmentKey(good.ID)]

	databasePath := filepath.Join(t.TempDir(), "restored.db")
	key, _ := ResolveCopy("daily/2026-03-01")
	if _, err := Restore(f.ctx, f.bucket, key, []age.Identity{f.identity}, databasePath); err != nil {
		t.Fatal(err)
	}

	// The target already holds a correct copy of one file.
	targetDir := t.TempDir()
	presentPath := files.New(targetDir).Path(present.ID)
	if err := os.MkdirAll(filepath.Dir(presentPath), 0o750); err != nil {
		t.Fatal(err)
	}
	presentData, err := os.ReadFile(f.store.AttachmentPath(present.ID))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(presentPath, presentData, 0o600); err != nil {
		t.Fatal(err)
	}
	f.fake.puts = nil

	report, err := RestoreAttachments(f.ctx, f.bucket, databasePath, []age.Identity{f.identity}, targetDir)
	if err == nil || !strings.Contains(err.Error(), "1 attachment files are missing") {
		t.Fatalf("RestoreAttachments returned %v", err)
	}
	if report.Restored != 1 || report.Skipped != 1 || len(report.Missing) != 1 || report.Missing[0] != gone.ID || len(report.Failed) != 1 {
		t.Fatalf("report %+v", report)
	}
	if failure := report.Failed[wrong.ID]; failure == nil || !strings.Contains(failure.Error(), "does not match the database") {
		t.Fatalf("wrong object failure: %v", failure)
	}

	restored, err := os.ReadFile(files.New(targetDir).Path(good.ID))
	if err != nil || !bytes.Equal(restored, goodData) {
		t.Fatalf("restored file differs (%v)", err)
	}
	sum := sha256.Sum256(restored)
	if hex.EncodeToString(sum[:]) != good.SHA256 {
		t.Fatal("restored file has the wrong hash")
	}
	if _, err := os.Stat(files.New(targetDir).Path(wrong.ID)); err == nil {
		t.Error("a file that failed verification was kept")
	}

	// Nothing but the verified files is left in the target, not even temporary ones.
	count := 0
	_ = filepath.WalkDir(targetDir, func(_ string, entry os.DirEntry, err error) error {
		if err == nil && !entry.IsDir() {
			count++
		}
		return err
	})
	if count != 2 {
		t.Errorf("target holds %d files", count)
	}

	// Running again skips the files that are now in place and reports the same problems.
	second, err := RestoreAttachments(f.ctx, f.bucket, databasePath, []age.Identity{f.identity}, targetDir)
	if err == nil || second.Restored != 0 || second.Skipped != 2 || len(second.Missing) != 1 || len(second.Failed) != 1 {
		t.Fatalf("second run %+v (%v)", second, err)
	}
}
