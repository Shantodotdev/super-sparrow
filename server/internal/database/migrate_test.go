package database

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"testing/fstest"
)

func openTestDatabase(t *testing.T) *sql.DB {
	t.Helper()

	db, err := Open(context.Background(), filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	return db
}

func tableExists(t *testing.T, db *sql.DB, name string) bool {
	t.Helper()

	var count int
	err := db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, name).Scan(&count)
	if err != nil {
		t.Fatalf("check table %s: %v", name, err)
	}

	return count == 1
}

func TestMigrateAppliesInOrderOnce(t *testing.T) {
	db := openTestDatabase(t)
	fsys := fstest.MapFS{
		"0002_add_name.sql": {Data: []byte(`ALTER TABLE things ADD COLUMN name TEXT;`)},
		"0001_things.sql":   {Data: []byte(`CREATE TABLE things (id INTEGER PRIMARY KEY);`)},
		"README.md":         {Data: []byte(`not a migration`)},
	}

	for range 2 {
		if err := Migrate(context.Background(), db, fsys); err != nil {
			t.Fatalf("Migrate: %v", err)
		}
	}

	var applied int
	if err := db.QueryRow(`SELECT count(*) FROM schema_migrations`).Scan(&applied); err != nil {
		t.Fatalf("count migrations: %v", err)
	}
	if applied != 2 {
		t.Errorf("applied %d migrations, want 2", applied)
	}

	if _, err := db.Exec(`INSERT INTO things (name) VALUES ('a')`); err != nil {
		t.Errorf("both migrations should have run: %v", err)
	}
}

func TestMigrateRollsBackAFailedMigration(t *testing.T) {
	db := openTestDatabase(t)
	fsys := fstest.MapFS{
		"0001_broken.sql": {Data: []byte(`CREATE TABLE half_done (id INTEGER); NOT VALID SQL;`)},
	}

	if err := Migrate(context.Background(), db, fsys); err == nil {
		t.Fatal("Migrate accepted invalid SQL")
	}

	if tableExists(t, db, "half_done") {
		t.Error("a failed migration left its table behind")
	}
}

func TestMigrateRejectsBadNames(t *testing.T) {
	db := openTestDatabase(t)
	fsys := fstest.MapFS{
		"create_things.sql": {Data: []byte(`CREATE TABLE things (id INTEGER);`)},
	}

	if err := Migrate(context.Background(), db, fsys); err == nil {
		t.Fatal("Migrate accepted a migration without a number")
	}
}
