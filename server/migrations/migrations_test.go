package migrations_test

import (
	"context"
	"io/fs"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/shantodotdev/super-sparrow/server/internal/database"
	"github.com/shantodotdev/super-sparrow/server/migrations"
)

// upTo returns the migrations named before stop, so a test can build a database as it was before
// stop ran.
func upTo(t *testing.T, stop string) fstest.MapFS {
	t.Helper()

	files := fstest.MapFS{}
	entries, err := fs.ReadDir(migrations.Files, ".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) != ".sql" || entry.Name() >= stop {
			continue
		}
		data, err := fs.ReadFile(migrations.Files, entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		files[entry.Name()] = &fstest.MapFile{Data: data}
	}
	return files
}

func TestAgentNamesLoseTheirOwner(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	if err := database.Migrate(ctx, db, upTo(t, "0005")); err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`
		INSERT INTO actors (id, kind, display_name, email, role, created_at, updated_at)
		VALUES ('person', 'human', 'Shanto', 'shanto@example.com', 'owner', '2026-01-01T00:00:00.000Z', '2026-01-01T00:00:00.000Z');
		INSERT INTO actors (id, kind, display_name, agent_name, owner_actor_id, created_at, updated_at) VALUES
			('claude', 'agent', 'Claude Code (Shanto)', 'claude-code', 'person', '2026-01-01T00:00:00.000Z', '2026-01-01T00:00:00.000Z'),
			('codex', 'agent', 'Codex (Shanto)', 'codex', 'person', '2026-01-01T00:00:00.000Z', '2026-01-01T00:00:00.000Z'),
			('other', 'agent', 'aider (Shanto)', 'aider', 'person', '2026-01-01T00:00:00.000Z', '2026-01-01T00:00:00.000Z');`)
	if err != nil {
		t.Fatal(err)
	}

	if err := database.Migrate(ctx, db, migrations.Files); err != nil {
		t.Fatal(err)
	}

	for id, want := range map[string]string{"person": "Shanto", "claude": "Claude Code", "codex": "Codex", "other": "aider"} {
		var name, updated string
		if err := db.QueryRow(`SELECT display_name, updated_at FROM actors WHERE id = ?`, id).Scan(&name, &updated); err != nil {
			t.Fatal(err)
		}
		if name != want {
			t.Errorf("%s is named %q, want %q", id, name, want)
		}
		if changed := id != "person"; changed == (updated == "2026-01-01T00:00:00.000Z") {
			t.Errorf("%s updated_at = %s", id, updated)
		}
	}
}

func TestProjectSlugsBackfillExistingNames(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := database.Migrate(ctx, db, upTo(t, "0007")); err != nil {
		t.Fatal(err)
	}
	for _, project := range []struct{ id, key, name string }{
		{"01", "AA", "Super Sparrow-2"}, {"02", "BB", "Super Sparrow"},
		{"03", "CC", "Super Sparrow!"}, {"04", "DD", "Settings"}, {"05", "EE", "বাংলা"},
	} {
		if _, err := db.Exec(`INSERT INTO projects(id,key,name,created_at,updated_at) VALUES(?,?,?,'2026-01-01T00:00:00.000Z','2026-01-01T00:00:00.000Z')`, project.id, project.key, project.name); err != nil {
			t.Fatal(err)
		}
	}
	if err := database.Migrate(ctx, db, migrations.Files); err != nil {
		t.Fatal(err)
	}
	if err := database.Migrate(ctx, db, migrations.Files); err != nil {
		t.Fatal(err)
	}
	for id, want := range map[string]string{"01": "super-sparrow-2", "02": "super-sparrow", "03": "super-sparrow-3", "04": "project-settings", "05": "ee"} {
		var slug string
		if err := db.QueryRow(`SELECT slug FROM projects WHERE id=?`, id).Scan(&slug); err != nil {
			t.Fatal(err)
		}
		if slug != want {
			t.Errorf("project %s slug = %q, want %q", id, slug, want)
		}
	}
	if _, err := db.Exec(`UPDATE projects SET slug='changed' WHERE id='01'`); err == nil {
		t.Error("slug update was accepted")
	}
}

func TestLabelColorsBackfillExistingLabels(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := database.Migrate(ctx, db, upTo(t, "0008")); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO projects (id,key,name,slug,created_at,updated_at) VALUES ('project','WP','WP','wp','2026-01-01T00:00:00.000Z','2026-01-01T00:00:00.000Z')`); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Bug", "backend", "Other"} {
		if _, err := db.Exec(`INSERT INTO labels(id,project_id,name,created_at,updated_at) VALUES (?, 'project', ?, '2026-01-01T00:00:00.000Z', '2026-01-01T00:00:00.000Z')`, name, name); err != nil {
			t.Fatal(err)
		}
	}
	if err := database.Migrate(ctx, db, migrations.Files); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{"Bug": "#f2a9ae", "backend": "#a1e4cd", "Other": "#a7c0ff"} {
		var color string
		if err := db.QueryRow(`SELECT color FROM labels WHERE name=?`, name).Scan(&color); err != nil {
			t.Fatal(err)
		}
		if color != want {
			t.Errorf("%s color=%s, want %s", name, color, want)
		}
	}
	if _, err := db.Exec(`UPDATE labels SET color='invalid' WHERE name='Bug'`); err == nil {
		t.Fatal("database accepted an invalid color")
	}
}
