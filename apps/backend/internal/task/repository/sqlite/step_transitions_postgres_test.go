package sqlite

// Postgres parity for task_step_transitions: BIGSERIAL id, replay safety, and
// the deliberate absence of a workflow/workflow_steps FK. Skips unless
// KANDEV_TEST_POSTGRES_DSN is set; CI runs these in postgres-boot.

import (
	"testing"
	"time"

	"github.com/kandev/kandev/internal/testutil"
)

func TestPostgresStepTransitionsSchemaCreatesTableAndIsReplaySafe(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}

	now := time.Now().UTC()
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO tasks (id, workspace_id, title, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`), "task-pre-ledger-pg", "ws-pre-ledger-pg", "Pre-existing task", now, now); err != nil {
		t.Fatalf("seed pre-existing task: %v", err)
	}

	var count int
	if err := db.Get(&count, db.Rebind(`
		SELECT COUNT(*) FROM task_step_transitions WHERE task_id = ?
	`), "task-pre-ledger-pg"); err != nil {
		t.Fatalf("query task_step_transitions: %v", err)
	}
	if count != 0 {
		t.Fatalf("pre-existing task has %d ledger rows, want 0", count)
	}

	if err := repo.initStepTransitionsSchema(); err != nil {
		t.Fatalf("replay initStepTransitionsSchema: %v", err)
	}
	if err := repo.initStepTransitionsSchema(); err != nil {
		t.Fatalf("replay initStepTransitionsSchema twice: %v", err)
	}
}
