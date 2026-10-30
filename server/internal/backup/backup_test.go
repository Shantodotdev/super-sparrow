package backup

import (
	"context"
	"database/sql"
	"encoding/xml"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"filippo.io/age"
	"github.com/shantodotdev/super-sparrow/server/internal/database"
	"github.com/shantodotdev/super-sparrow/server/internal/store"
	"github.com/shantodotdev/super-sparrow/server/migrations"
)

// fakeS3 keeps objects in memory and answers the requests Bucket makes. It checks that every
// request is signed.
type fakeS3 struct {
	mu      sync.Mutex
	objects map[string][]byte
	failPut bool
}

func (f *fakeS3) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if !strings.HasPrefix(r.Header.Get("Authorization"), "AWS4-HMAC-SHA256 Credential=key-id/") {
		http.Error(w, "<Error><Code>AccessDenied</Code><Message>unsigned</Message></Error>", http.StatusForbidden)
		return
	}
	key := strings.TrimPrefix(r.URL.Path, "/bucket/")

	switch {
	case r.Method == http.MethodPut && f.failPut:
		http.Error(w, "<Error><Code>InternalError</Code><Message>try again</Message></Error>", http.StatusInternalServerError)
	case r.Method == http.MethodPut:
		data, _ := io.ReadAll(r.Body)
		f.objects[key] = data
	case r.Method == http.MethodGet && r.URL.Path == "/bucket":
		type content struct {
			Key  string
			Size int64
		}
		var listing struct {
			XMLName  xml.Name `xml:"ListBucketResult"`
			Contents []content
		}
		for name, data := range f.objects {
			if strings.HasPrefix(name, r.URL.Query().Get("prefix")) {
				listing.Contents = append(listing.Contents, content{Key: name, Size: int64(len(data))})
			}
		}
		sort.Slice(listing.Contents, func(i, j int) bool { return listing.Contents[i].Key < listing.Contents[j].Key })
		_ = xml.NewEncoder(w).Encode(listing)
	case r.Method == http.MethodGet:
		data, ok := f.objects[key]
		if !ok {
			http.Error(w, "<Error><Code>NoSuchKey</Code><Message>missing</Message></Error>", http.StatusNotFound)
			return
		}
		_, _ = w.Write(data)
	case r.Method == http.MethodDelete:
		delete(f.objects, key)
		w.WriteHeader(http.StatusNoContent)
	}
}

func TestBackupsUploadPruneAndRestore(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	db, err := database.Open(ctx, filepath.Join(dir, "sparrow.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := database.Migrate(ctx, db, migrations.Files); err != nil {
		t.Fatal(err)
	}
	// 04:00 in Dhaka is 22:00 UTC the day before, so the copy must take Dhaka's date.
	today := time.Date(2026, 3, 1, 4, 0, 0, 0, dhaka(t))
	clock := func() time.Time { return today }
	st := store.New(db, clock)
	owner, err := st.CreateFirstOwner(ctx, "Olive Owner", "olive@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateProject(ctx, store.Caller{ActorID: owner.ID}, store.NewProject{Key: "WP", Name: "WP Agent"}); err != nil {
		t.Fatal(err)
	}

	fake := &fakeS3{objects: map[string][]byte{
		"daily/sparrow-2026-02-01.db.gz.age": []byte("old"),
		"daily/sparrow-2026-02-27.db.gz.age": []byte("recent"),
		"daily/notes.txt":                    []byte("not a backup"),
	}}
	server := httptest.NewServer(fake)
	defer server.Close()
	endpoint, _ := url.Parse(server.URL)
	bucket := newBucket(endpoint, "auto", "key-id", "secret", "bucket", time.Now)

	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	runner := NewRunner(Options{
		Store: st, Bucket: bucket, Recipient: identity.Recipient(), WorkDir: dir, KeepDays: 3, DailyAt: 3 * time.Hour,
		TimeZone: dhaka(t), Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Now: clock,
	})

	if err := runner.Sync(ctx); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if !runner.dailyDue(ctx) {
		t.Fatal("the daily backup is not due after its time on a day without one")
	}

	// A failed daily backup stays due, so the next hourly sync tries it again.
	fake.failPut = true
	if err := runner.Daily(ctx); err == nil {
		t.Fatal("Daily succeeded while uploads fail")
	}
	if !runner.dailyDue(ctx) {
		t.Fatal("a failed daily backup is no longer due")
	}
	fake.failPut = false
	if err := runner.Daily(ctx); err != nil {
		t.Fatalf("Daily: %v", err)
	}
	if runner.dailyDue(ctx) {
		t.Fatal("the daily backup is still due after it ran")
	}

	var keys []string
	for key := range fake.objects {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	want := "daily/notes.txt,daily/sparrow-2026-02-27.db.gz.age,daily/sparrow-2026-03-01.db.gz.age,sync/sparrow.db.gz.age"
	if strings.Join(keys, ",") != want {
		t.Fatalf("bucket holds %v", keys)
	}
	if strings.Contains(string(fake.objects[SyncKey]), "WP Agent") {
		t.Fatal("the uploaded copy is readable without the key")
	}

	// A failed upload is recorded and leaves the previous copy in place.
	previous := fake.objects[SyncKey]
	fake.failPut = true
	if err := runner.Sync(ctx); err == nil || !strings.Contains(err.Error(), "InternalError") {
		t.Fatalf("failed Sync returned %v", err)
	}
	if string(fake.objects[SyncKey]) != string(previous) {
		t.Fatal("a failed upload replaced the sync copy")
	}
	status, err := st.GetBackupStatus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if status[store.BackupSync].LastRun.Result != store.BackupFailed || status[store.BackupSync].LastSuccess == nil {
		t.Fatalf("sync status %+v", status[store.BackupSync])
	}

	// Restore each copy into a fresh place and read it back.
	for _, name := range []string{"sync", "daily/2026-03-01"} {
		key, err := ResolveCopy(name)
		if err != nil {
			t.Fatal(err)
		}
		target := filepath.Join(t.TempDir(), "restored.db")
		if _, err := Restore(ctx, bucket, key, []age.Identity{identity}, target); err != nil {
			t.Fatalf("Restore %s: %v", name, err)
		}

		restored, err := sql.Open("sqlite", "file:"+target+"?mode=ro")
		if err != nil {
			t.Fatal(err)
		}
		var projectName string
		err = restored.QueryRow(`SELECT name FROM projects WHERE key = 'WP'`).Scan(&projectName)
		restored.Close()
		if err != nil || projectName != "WP Agent" {
			t.Fatalf("restored %s holds %q (%v)", name, projectName, err)
		}

		if _, err := Restore(ctx, bucket, key, []age.Identity{identity}, target); err == nil || !strings.Contains(err.Error(), "already exists") {
			t.Fatalf("restore over an existing database: %v", err)
		}
	}

	other, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Restore(ctx, bucket, SyncKey, []age.Identity{other}, filepath.Join(t.TempDir(), "x.db")); err == nil {
		t.Fatal("restored with the wrong key")
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 3 {
		t.Errorf("working files left behind: %v", entries)
	}
}

func TestDailyScheduleUsesTheTimeZone(t *testing.T) {
	runner := &Runner{dailyAt: 3 * time.Hour, timeZone: dhaka(t)}

	// 02:00 in Dhaka is an hour before the backup; 21:00 UTC is 03:00 in Dhaka.
	if got := runner.untilDaily(time.Date(2026, 3, 1, 2, 0, 0, 0, dhaka(t))); got != time.Hour {
		t.Errorf("before the time: %v", got)
	}
	if got := runner.untilDaily(time.Date(2026, 3, 1, 21, 0, 0, 0, time.UTC)); got != 24*time.Hour {
		t.Errorf("at the time, given in UTC: %v", got)
	}
	if got := runner.untilDaily(time.Date(2026, 3, 1, 20, 30, 0, 0, time.UTC)); got != 30*time.Minute {
		t.Errorf("half an hour before, given in UTC: %v", got)
	}

	before := &Runner{dailyAt: 3 * time.Hour, timeZone: dhaka(t), now: func() time.Time { return time.Date(2026, 3, 1, 2, 59, 0, 0, dhaka(t)) }}
	if before.dailyDue(context.Background()) {
		t.Error("the daily backup is due before its time")
	}
}

func TestResolveCopy(t *testing.T) {
	if key, _ := ResolveCopy("daily/2026-03-01"); key != "daily/sparrow-2026-03-01.db.gz.age" {
		t.Errorf("daily copy key %q", key)
	}
	if _, err := ResolveCopy("daily/yesterday"); err == nil {
		t.Error("accepted daily/yesterday")
	}
}

func dhaka(t *testing.T) *time.Location {
	t.Helper()
	location, err := time.LoadLocation("Asia/Dhaka")
	if err != nil {
		t.Fatal(err)
	}
	return location
}
