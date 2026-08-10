package sqlite_test

import (
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

// TestMigrate_PriorityRebuildPreservesExternalIDUniqueIndex is a regression
// test: the priority-to-TEXT rebuild (docs: taskPriorityMigrationStatements)
// recreates the tasks table via DROP TABLE + CREATE TABLE, which silently
// drops every index on the old table — uniq_tasks_external_id included —
// unless it is explicitly relisted among the indexes the rebuild recreates.
// Without that, task create-idempotency (external-id-idempotency spec) loses
// its uniqueness guarantee on any install that still needs this historical
// rebuild, with no compile-time or single-connection-serial-insert signal
// that anything is wrong.
func TestMigrate_PriorityRebuildPreservesExternalIDUniqueIndex(t *testing.T) {
	dbPath := t.TempDir() + "/test.db?_journal_mode=WAL"
	db, err := sqlx.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	// A legacy tasks table with INTEGER priority — this alone is enough to
	// trigger the rebuild — and no external_id column, matching a real
	// pre-this-feature install.
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
			metadata TEXT DEFAULT '{}',
			is_ephemeral INTEGER NOT NULL DEFAULT 0,
			parent_id TEXT DEFAULT '',
			archived_at TIMESTAMP,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			origin TEXT DEFAULT 'manual',
			project_id TEXT DEFAULT '',
			labels TEXT DEFAULT '[]',
			identifier TEXT
		);
	`); err != nil {
		t.Fatalf("seed legacy schema: %v", err)
	}

	if _, err := sqlite.NewWithDB(db, db, nil); err != nil {
		t.Fatalf("init office repo (run migrations): %v", err)
	}

	var indexName string
	if err := db.Get(&indexName, `
		SELECT name FROM sqlite_master WHERE type = 'index' AND name = 'uniq_tasks_external_id'
	`); err != nil {
		t.Fatalf("uniq_tasks_external_id index missing after priority rebuild: %v", err)
	}

	now := time.Now().UTC()
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO tasks (id, workspace_id, title, created_at, updated_at, external_id)
		VALUES (?, ?, ?, ?, ?, ?)
	`), "task-1", "ws-1", "First", now, now, "ext-1"); err != nil {
		t.Fatalf("first insert with external_id: %v", err)
	}
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO tasks (id, workspace_id, title, created_at, updated_at, external_id)
		VALUES (?, ?, ?, ?, ?, ?)
	`), "task-2", "ws-1", "Second", now, now, "ext-1"); err == nil {
		t.Fatal("second insert with the same (workspace_id, external_id) should violate uniq_tasks_external_id")
	}
}
