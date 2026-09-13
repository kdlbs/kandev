package sqlite

import (
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
)

// TestMigrateTasksRemoveWorkflowFK_AssignmentGenerationSurvivesRebuild is the
// replay regression test flagged (but not written) in the run-dedup-generation
// build: migrateTasksRemoveWorkflowFK (base_migrations.go) recreates `tasks`
// from an explicit 19-column list that predates assignment_generation. The
// tasks.assignment_generation ADD COLUMN is sequenced strictly AFTER that
// rebuild in runMigrations() precisely so a database still carrying the
// legacy `FOREIGN KEY (workflow_id)` DDL does not have the new column
// silently dropped for the remainder of that boot (the same hazard class as
// the task_sessions.name comment in the same file). This had zero coverage
// before this test: nothing exercised migrateTasksRemoveWorkflowFK's rebuild
// path at all.
func TestMigrateTasksRemoveWorkflowFK_AssignmentGenerationSurvivesRebuild(t *testing.T) {
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	// Seed a pre-migration `tasks` table: the legacy FK clause that triggers
	// the rebuild, the 19 columns migrateTasksRemoveWorkflowFK's SELECT list
	// expects, and no assignment_generation column at all — matching a real
	// database that predates this feature.
	if _, err := db.Exec(`
		CREATE TABLE tasks (
			id TEXT PRIMARY KEY,
			workspace_id TEXT NOT NULL DEFAULT '',
			workflow_id TEXT NOT NULL DEFAULT '',
			workflow_step_id TEXT NOT NULL DEFAULT '',
			title TEXT NOT NULL,
			description TEXT DEFAULT '',
			state TEXT DEFAULT 'TODO',
			priority INTEGER DEFAULT 0,
			position INTEGER DEFAULT 0,
			wip_admitted INTEGER NOT NULL DEFAULT 1,
			queued_for_step_id TEXT NOT NULL DEFAULT '',
			queued_at TIMESTAMP,
			metadata TEXT DEFAULT '{}',
			is_ephemeral INTEGER NOT NULL DEFAULT 0,
			parent_id TEXT DEFAULT '',
			autopilot_enabled INTEGER NOT NULL DEFAULT 0,
			archived_at TIMESTAMP,
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL,
			FOREIGN KEY (workflow_id) REFERENCES workflows(id)
		);
		INSERT INTO tasks (id, workspace_id, title, created_at, updated_at)
			VALUES ('task-legacy-fk', 'ws-legacy', 'Legacy FK task', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);
	`); err != nil {
		t.Fatalf("seed legacy schema: %v", err)
	}

	if _, err := NewWithDB(db, db, nil); err != nil {
		t.Fatalf("init task repo (run migrations): %v", err)
	}

	// The rebuild must have fired: the legacy FK clause is gone.
	var tableSQL string
	if err := db.QueryRow(
		`SELECT sql FROM sqlite_master WHERE type='table' AND name='tasks'`,
	).Scan(&tableSQL); err != nil {
		t.Fatalf("read tasks schema: %v", err)
	}
	if strings.Contains(tableSQL, "FOREIGN KEY (workflow_id)") {
		t.Fatalf("tasks table still carries the legacy workflow_id FK after migration: %s", tableSQL)
	}

	// assignment_generation must be present post-rebuild, not silently
	// dropped by the rebuild's pre-feature explicit column list.
	rows, err := db.Queryx(`PRAGMA table_info(tasks)`)
	if err != nil {
		t.Fatalf("pragma table_info: %v", err)
	}
	found := false
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt *string
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			_ = rows.Close()
			t.Fatalf("scan pragma: %v", err)
		}
		if name == "assignment_generation" {
			found = true
		}
	}
	_ = rows.Close()
	if !found {
		t.Fatalf("assignment_generation column missing after migrating a legacy FK-bearing tasks table")
	}

	// The pre-existing row must survive the rebuild, with the new column
	// defaulted to 0.
	var gotTitle string
	var gotGeneration int64
	if err := db.QueryRow(
		`SELECT title, assignment_generation FROM tasks WHERE id = ?`, "task-legacy-fk",
	).Scan(&gotTitle, &gotGeneration); err != nil {
		t.Fatalf("query migrated row: %v", err)
	}
	if gotTitle != "Legacy FK task" {
		t.Fatalf("title = %q, want %q (row lost during rebuild)", gotTitle, "Legacy FK task")
	}
	if gotGeneration != 0 {
		t.Fatalf("assignment_generation = %d, want 0 for a pre-existing row", gotGeneration)
	}
}
