package database

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"slices"
	"strings"
)

var migrationName = regexp.MustCompile(`^[0-9]{4}_[a-z0-9_]+\.sql$`)

// Migrate applies every migration in fsys that has not been applied yet, in file name order.
//
// Migrations are files named like 0001_create_actors.sql. Files that do not end in .sql are
// ignored. Each migration runs in its own transaction together with the row that records it, so
// a failed migration leaves no trace and the next start retries it.
func Migrate(ctx context.Context, db *sql.DB, fsys fs.FS) error {
	_, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			name TEXT PRIMARY KEY,
			applied_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
		)`)
	if err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	names, err := migrationNames(fsys)
	if err != nil {
		return err
	}

	applied, err := appliedMigrations(ctx, db)
	if err != nil {
		return err
	}

	for _, name := range names {
		if applied[name] {
			continue
		}

		if err := applyMigration(ctx, db, fsys, name); err != nil {
			return err
		}
	}

	return nil
}

func migrationNames(fsys fs.FS) ([]string, error) {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, fmt.Errorf("list migrations: %w", err)
	}

	var names []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || path.Ext(name) != ".sql" {
			continue
		}
		if !migrationName.MatchString(name) {
			return nil, fmt.Errorf("migration %s must be named like 0001_create_actors.sql", name)
		}

		names = append(names, name)
	}

	slices.Sort(names)
	return names, nil
}

func appliedMigrations(ctx context.Context, db *sql.DB) (map[string]bool, error) {
	rows, err := db.QueryContext(ctx, `SELECT name FROM schema_migrations`)
	if err != nil {
		return nil, fmt.Errorf("read applied migrations: %w", err)
	}
	defer rows.Close()

	applied := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("read applied migrations: %w", err)
		}

		applied[name] = true
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read applied migrations: %w", err)
	}

	return applied, nil
}

func applyMigration(ctx context.Context, db *sql.DB, fsys fs.FS, name string) error {
	contents, err := fs.ReadFile(fsys, name)
	if err != nil {
		return fmt.Errorf("read migration %s: %w", name, err)
	}
	if strings.TrimSpace(string(contents)) == "" {
		return fmt.Errorf("migration %s is empty", name)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin migration %s: %w", name, err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, string(contents)); err != nil {
		return fmt.Errorf("apply migration %s: %w", name, err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations (name) VALUES (?)`, name); err != nil {
		return fmt.Errorf("record migration %s: %w", name, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration %s: %w", name, err)
	}

	return nil
}
