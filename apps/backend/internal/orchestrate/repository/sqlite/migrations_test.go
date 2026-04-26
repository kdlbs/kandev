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
	if _, err := taskrepo.NewWithDB(db, db); err != nil {
		t.Fatalf("init task repo: %v", err)
	}

	// Verify task columns
	taskColumns := []string{
		"assignee_agent_instance_id",
		"origin",
		"project_id",
		"requires_approval",
		"execution_policy",
		"execution_state",
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
	wsColumns := []string{"task_prefix", "task_sequence", "orchestrate_workflow_id"}
	for _, col := range wsColumns {
		var dummy interface{}
		err := db.QueryRow(`SELECT ` + col + ` FROM workspaces LIMIT 0`).Scan(&dummy)
		if err != nil && err.Error() != "sql: no rows in result set" {
			t.Errorf("workspace column %q missing: %v", col, err)
		}
	}

	// Verify session columns
	sessionColumns := []string{"cost_cents", "tokens_in", "tokens_out"}
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
