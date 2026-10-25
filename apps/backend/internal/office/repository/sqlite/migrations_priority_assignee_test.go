package sqlite_test

import (
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

// TestMigrate_PriorityRebuildPreservesAssignee is a regression test for the
// same hazard the index test above covers, one column further along: the
// priority-to-TEXT rebuild recreates tasks via DROP TABLE + CREATE TABLE, so
// any column missing from taskPriorityMigrationStatements' shape and copy list
// is dropped along with the data in it. For the human assignee that would mean
// every existing assignment silently disappearing on an install that still
// needs this historical rebuild.
func TestMigrate_PriorityRebuildPreservesAssignee(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db") + "?_journal_mode=WAL"
	db, err := sqlx.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	// Legacy INTEGER-priority table (enough to trigger the rebuild) that
	// already carries an assignment, as an install upgrading mid-feature would.
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
			identifier TEXT,
			assignee_user_id TEXT NOT NULL DEFAULT ''
		);
	`); err != nil {
		t.Fatalf("seed legacy schema: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO tasks (id, workspace_id, title, assignee_user_id)
		VALUES ('task-1', 'ws-1', 'assigned task', 'user-42')
	`); err != nil {
		t.Fatalf("seed assigned task: %v", err)
	}

	if _, err := sqlite.NewWithDB(db, db, nil); err != nil {
		t.Fatalf("init office repo (run migrations): %v", err)
	}

	var assignee string
	if err := db.Get(&assignee, `SELECT assignee_user_id FROM tasks WHERE id = 'task-1'`); err != nil {
		t.Fatalf("tasks.assignee_user_id missing after priority rebuild: %v", err)
	}
	if assignee != "user-42" {
		t.Fatalf("assignee lost by priority rebuild: got %q, want %q", assignee, "user-42")
	}
}
