package sqlite_test

import (
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

// TestMigrate_PriorityRebuildPreservesAssignmentGeneration is the regression
// test for the same rebuild hazard TestMigrate_PriorityRebuildPreservesAssignee
// covers one column over: taskPriorityMigrationStatements' INSERT ... SELECT
// carries tasks.assignment_generation through
// COALESCE(assignment_generation,0) (docs/specs/office/system-design/
// run-dedup-generation-02.md#failure-and-recovery). That expression had zero
// coverage before this test — the sibling assignee test seeds a table where
// runTaskPriorityRecreate's own defensive ALTER TABLE ADD COLUMN supplies the
// column fresh (every row defaults to 0), which never exercises copying a
// genuinely non-zero value through the recreate. Here the legacy table
// already carries the column with a non-zero value, as an install upgrading
// mid-feature (assignment_generation shipped, priority rebuild still pending)
// would.
func TestMigrate_PriorityRebuildPreservesAssignmentGeneration(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db") + "?_journal_mode=WAL"
	db, err := sqlx.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

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
			assignee_user_id TEXT NOT NULL DEFAULT '',
			assignment_generation INTEGER NOT NULL DEFAULT 0
		);
	`); err != nil {
		t.Fatalf("seed legacy schema: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO tasks (id, workspace_id, title, assignment_generation)
		VALUES ('task-1', 'ws-1', 'reassigned task', 3)
	`); err != nil {
		t.Fatalf("seed reassigned task: %v", err)
	}

	if _, err := sqlite.NewWithDB(db, db, nil); err != nil {
		t.Fatalf("init office repo (run migrations): %v", err)
	}

	var generation int64
	if err := db.Get(&generation, `SELECT assignment_generation FROM tasks WHERE id = 'task-1'`); err != nil {
		t.Fatalf("tasks.assignment_generation missing after priority rebuild: %v", err)
	}
	if generation != 3 {
		t.Fatalf("assignment_generation lost by priority rebuild: got %d, want 3", generation)
	}
}
