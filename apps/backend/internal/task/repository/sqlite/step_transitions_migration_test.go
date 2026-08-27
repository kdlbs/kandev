package sqlite

// Migration coverage for task_step_transitions, mirroring
// task_external_id_migration_test.go: a fresh boot creates the table, an
// existing task gains no rows, and the schema-init step is replay-safe.

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	dbutil "github.com/kandev/kandev/internal/db"
)

func TestStepTransitionsSchemaCreatesTableAndIsReplaySafe(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "step-transitions-migration.db")
	dbConn, err := dbutil.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = db.Close() })

	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("initialize schema: %v", err)
	}

	now := time.Now().UTC()
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO tasks (id, workspace_id, title, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`), "task-pre-ledger", "ws-pre-ledger", "Pre-existing task", now, now); err != nil {
		t.Fatalf("seed pre-existing task: %v", err)
	}

	var count int
	if err := db.Get(&count, db.Rebind(`
		SELECT COUNT(*) FROM task_step_transitions WHERE task_id = ?
	`), "task-pre-ledger"); err != nil {
		t.Fatalf("query task_step_transitions: %v", err)
	}
	if count != 0 {
		t.Fatalf("pre-existing task has %d ledger rows, want 0 (nothing is backfilled)", count)
	}

	var tableName string
	if err := db.Get(&tableName, `
		SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'task_step_transitions'
	`); err != nil {
		t.Fatalf("task_step_transitions table is missing: %v", err)
	}

	for _, idx := range []string{"idx_task_step_transitions_task", "idx_task_step_transitions_occurred"} {
		var indexName string
		if err := db.Get(&indexName, db.Rebind(`
			SELECT name FROM sqlite_master WHERE type = 'index' AND name = ?
		`), idx); err != nil {
			t.Fatalf("index %s is missing: %v", idx, err)
		}
	}

	// Replay: schema init must be idempotent across repeated boots.
	if err := repo.initStepTransitionsSchema(); err != nil {
		t.Fatalf("replay initStepTransitionsSchema: %v", err)
	}
	if err := repo.initStepTransitionsSchema(); err != nil {
		t.Fatalf("replay initStepTransitionsSchema twice: %v", err)
	}
	if err := repo.runMigrations(); err != nil {
		t.Fatalf("replay runMigrations: %v", err)
	}
	if err := repo.runMigrations(); err != nil {
		t.Fatalf("replay runMigrations twice: %v", err)
	}

	var countAfterReplay int
	if err := db.Get(&countAfterReplay, db.Rebind(`
		SELECT COUNT(*) FROM task_step_transitions WHERE task_id = ?
	`), "task-pre-ledger"); err != nil {
		t.Fatalf("query task_step_transitions after replay: %v", err)
	}
	if countAfterReplay != 0 {
		t.Fatalf("pre-existing task has %d ledger rows after replay, want 0", countAfterReplay)
	}
}

func TestStepTransitionsSchemaHasNoForeignKeyToWorkflowObjects(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "step-transitions-no-fk.db")
	dbConn, err := dbutil.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = db.Close() })

	if _, err := NewWithDB(db, db, nil); err != nil {
		t.Fatalf("initialize schema: %v", err)
	}

	// A row referencing a workflow_id/step_id with no matching row in
	// workflows/workflow_steps must insert cleanly — there is deliberately
	// no FK to either table, so historical rows survive step/workflow
	// deletion.
	now := time.Now().UTC()
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO tasks (id, workspace_id, title, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`), "task-no-fk", "ws-no-fk", "Task", now, now); err != nil {
		t.Fatalf("seed task: %v", err)
	}
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO task_step_transitions
			(task_id, from_workflow_id, from_workflow_step_id, to_workflow_id, to_workflow_step_id, trigger, actor_kind, actor_id, contract_version, occurred_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`), "task-no-fk", nil, nil, "workflow-does-not-exist", "step-does-not-exist", "task_created", "system", "", 1, now); err != nil {
		t.Fatalf("insert row referencing nonexistent workflow/step: %v", err)
	}
}
