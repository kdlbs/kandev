package sqlite_test

import (
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

func TestTaskMigrations_NewColumnsExist(t *testing.T) {
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer func() { _ = db.Close() }()

	// Initialize task repo (which runs all migrations)
	if _, err := taskrepo.NewWithDB(db, db, nil); err != nil {
		t.Fatalf("init task repo: %v", err)
	}

	// Verify task columns. The legacy requires_approval / execution_policy
	// / execution_state columns were dropped in Phase 4 of
	// task-model-unification (stage progression is owned by the workflow
	// engine now). assignee_agent_profile_id was dropped in Wave F of
	// ADR 0005 — per-task runner is now a 'runner' participant in
	// workflow_step_participants.
	taskColumns := []string{
		"origin",
		"project_id",
		"labels",
		"identifier",
	}
	for _, col := range taskColumns {
		var dummy interface{}
		err := db.QueryRow(`SELECT ` + col + ` FROM tasks LIMIT 0`).Scan(&dummy)
		// sql.ErrNoRows is expected (no rows), any other error means column doesn't exist
		if err != nil && err.Error() != "sql: no rows in result set" {
			t.Errorf("task column %q missing: %v", col, err)
		}
	}

	// Verify workspace columns
	wsColumns := []string{"task_prefix", "task_sequence", "office_workflow_id"}
	for _, col := range wsColumns {
		var dummy interface{}
		err := db.QueryRow(`SELECT ` + col + ` FROM workspaces LIMIT 0`).Scan(&dummy)
		if err != nil && err.Error() != "sql: no rows in result set" {
			t.Errorf("workspace column %q missing: %v", col, err)
		}
	}

	// Verify session columns (cost_subcents stores hundredths of a cent
	// per docs/specs/office-costs/spec.md — the column was renamed from
	// cost_cents pre-release; no migration kept).
	sessionColumns := []string{"cost_subcents", "tokens_in", "tokens_out"}
	for _, col := range sessionColumns {
		var dummy interface{}
		err := db.QueryRow(`SELECT ` + col + ` FROM task_sessions LIMIT 0`).Scan(&dummy)
		if err != nil && err.Error() != "sql: no rows in result set" {
			t.Errorf("session column %q missing: %v", col, err)
		}
	}

	// Verify workflow is_system column
	var dummy interface{}
	err = db.QueryRow(`SELECT is_system FROM workflows LIMIT 0`).Scan(&dummy)
	if err != nil && err.Error() != "sql: no rows in result set" {
		t.Errorf("workflow column is_system missing: %v", err)
	}
}
