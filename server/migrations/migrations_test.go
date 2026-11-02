package migrations_test

import (
	"context"
	"encoding/json"
	"io/fs"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/shantodotdev/super-sparrow/server/internal/database"
	"github.com/shantodotdev/super-sparrow/server/migrations"
)

func TestReviewCommitsAreRemovedFromExistingRecords(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := database.Migrate(ctx, db, upTo(t, "0012")); err != nil {
		t.Fatal(err)
	}

	_, err = db.Exec(`
		INSERT INTO projects (id, key, name, slug, created_at, updated_at)
		VALUES ('project', 'WP', 'Project', 'project', '2026-01-01T00:00:00.000Z', '2026-01-01T00:00:00.000Z');
		INSERT INTO issues (id, project_id, number, title, plan, status, created_by_actor_id, created_at, updated_at)
		SELECT 'issue', 'project', 1, 'Issue', '', 'in_review', id, created_at, updated_at FROM actors WHERE kind = 'system';
		INSERT INTO review_rounds (id, issue_id, number, from_commit, to_commit, includes_uncommitted,
			plan_coverage, summary, next_finding_number, author_actor_id, created_at, updated_at)
		SELECT 'round', 'issue', 1, 'abcdef1', 'abcdef2', 1, 'Full plan', 'Review summary', 2, id, created_at, updated_at
		FROM actors WHERE kind = 'system';
		INSERT INTO findings (id, review_round_id, number, severity, title, problem, fix, evidence, file_path, line,
			commit_sha, status, found_by_actor_id, decided_by_actor_id, decided_at, fixed_by_actor_id, fixed_in_commit,
			fixed_at, created_at, updated_at)
		SELECT 'finding', 'round', 1, 'blocking', 'Race', 'Problem', 'Fix', 'Evidence', 'a.go', 4, 'abcdef1', 'fixed',
			id, id, created_at, id, 'abcdef2', created_at, created_at, updated_at FROM actors WHERE kind = 'system';
		INSERT INTO comments (id, finding_id, body, author_actor_id, created_at, updated_at)
		SELECT 'comment', 'finding', 'Discussion', id, created_at, updated_at FROM actors WHERE kind = 'system';
		INSERT INTO events (id, project_id, target_type, target_id, kind, actor_id, payload, created_at)
		SELECT 'event', 'project', 'finding', 'finding', 'finding.status_changed', id,
			'{"issue_id":"issue","to":"fixed","reason":"Keep this","from_commit":"abcdef1","to_commit":"abcdef2","includes_uncommitted":true,"commit_sha":"abcdef1","fixed_in_commit":"abcdef2","commit":"abcdef2"}',
			created_at FROM actors WHERE kind = 'system';`)
	if err != nil {
		t.Fatal(err)
	}

	if err := database.Migrate(ctx, db, migrations.Files); err != nil {
		t.Fatal(err)
	}
	if err := database.Migrate(ctx, db, migrations.Files); err != nil {
		t.Fatal(err)
	}

	for table, columns := range map[string][]string{
		"review_rounds": {"from_commit", "to_commit", "includes_uncommitted"},
		"findings":      {"commit_sha", "fixed_in_commit"},
	} {
		for _, column := range columns {
			var count int
			if err := db.QueryRow(`SELECT count(*) FROM pragma_table_info(?) WHERE name = ?`, table, column).Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != 0 {
				t.Errorf("%s still has column %s", table, column)
			}
		}
	}
	var title, status, evidence, body, coverage, summary, actor string
	var line, nextNumber int
	err = db.QueryRow(`SELECT f.title, f.status, f.evidence, f.line, c.body, r.plan_coverage, r.summary,
		r.next_finding_number, f.fixed_by_actor_id FROM findings f JOIN review_rounds r ON r.id = f.review_round_id
		JOIN comments c ON c.finding_id = f.id WHERE f.id = 'finding'`).Scan(
		&title, &status, &evidence, &line, &body, &coverage, &summary, &nextNumber, &actor)
	if err != nil {
		t.Fatal(err)
	}
	if title != "Race" || status != "fixed" || evidence != "Evidence" || line != 4 || body != "Discussion" ||
		coverage != "Full plan" || summary != "Review summary" || nextNumber != 2 || actor != "00000000-0000-7000-8000-000000000000" {
		t.Fatal("migration changed review content, discussion, numbering or attribution")
	}
	var payload string
	var sequence int
	if err := db.QueryRow(`SELECT payload, sequence, actor_id FROM events WHERE id = 'event'`).Scan(&payload, &sequence, &actor); err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal([]byte(payload), &fields); err != nil {
		t.Fatal(err)
	}
	if len(fields) != 3 || fields["issue_id"] != "issue" || fields["to"] != "fixed" || fields["reason"] != "Keep this" || sequence != 1 || actor != "00000000-0000-7000-8000-000000000000" {
		t.Fatalf("migration changed event history or retained retired metadata: %s", payload)
	}
	if _, err := db.Exec(`UPDATE events SET payload = '{}' WHERE id = 'event'`); err == nil {
		t.Error("event update guard was not restored")
	}
	if _, err := db.Exec(`DELETE FROM events WHERE id = 'event'`); err == nil {
		t.Error("event delete guard was not preserved")
	}
	var violations int
	if err := db.QueryRow(`SELECT count(*) FROM pragma_foreign_key_check`).Scan(&violations); err != nil || violations != 0 {
		t.Fatalf("foreign key violations = %d: %v", violations, err)
	}
}

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

func TestMilestoneStatusColumnIsRemovedWithoutLosingWork(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := database.Migrate(ctx, db, upTo(t, "0014")); err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`
		INSERT INTO projects (id, key, name, slug, created_at, updated_at)
		VALUES ('project', 'WP', 'Project', 'project', '2026-01-01T00:00:00.000Z', '2026-01-01T00:00:00.000Z');
		INSERT INTO milestones (id, project_id, name, position, status, created_at, updated_at)
		VALUES ('milestone', 'project', 'Launch', 2, 'done', '2026-01-01T00:00:00.000Z', '2026-01-01T00:00:00.000Z');
		INSERT INTO issues (id, project_id, number, title, plan, status, milestone_id, created_by_actor_id, created_at, updated_at)
		SELECT 'issue', 'project', 1, 'Work', '', 'todo', 'milestone', id, created_at, updated_at FROM actors WHERE kind = 'system';
		INSERT INTO events (id, project_id, target_type, target_id, kind, actor_id, payload, created_at)
		SELECT 'event', 'project', 'milestone', 'milestone', 'milestone.created', id, '{"status":"done"}', created_at FROM actors WHERE kind = 'system';`)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Migrate(ctx, db, migrations.Files); err != nil {
		t.Fatal(err)
	}
	if err := database.Migrate(ctx, db, migrations.Files); err != nil {
		t.Fatal(err)
	}
	var columns int
	if err := db.QueryRow(`SELECT count(*) FROM pragma_table_info('milestones') WHERE name = 'status'`).Scan(&columns); err != nil || columns != 0 {
		t.Fatalf("milestone status columns = %d: %v", columns, err)
	}
	var name, milestoneID, payload, actor string
	var position int
	if err := db.QueryRow(`SELECT name, position FROM milestones WHERE id = 'milestone'`).Scan(&name, &position); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT milestone_id, created_by_actor_id FROM issues WHERE id = 'issue'`).Scan(&milestoneID, &actor); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT payload FROM events WHERE id = 'event'`).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	if name != "Launch" || position != 2 || milestoneID != "milestone" || actor != "00000000-0000-7000-8000-000000000000" || payload != `{"status":"done"}` {
		t.Fatal("migration changed milestone content, issue assignment, attribution, or history")
	}
	var violations int
	if err := db.QueryRow(`SELECT count(*) FROM pragma_foreign_key_check`).Scan(&violations); err != nil || violations != 0 {
		t.Fatalf("foreign key violations = %d: %v", violations, err)
	}
}
