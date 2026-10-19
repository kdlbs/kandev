package sqlite

// Postgres parity for task_step_transitions: BIGSERIAL id, replay safety, and
// the deliberate absence of a workflow/workflow_steps FK. Skips unless
// KANDEV_TEST_POSTGRES_DSN is set; CI runs these in postgres-boot.

import (
	"context"
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
	for _, name := range []string{"idx_task_step_transitions_task_id", "idx_task_step_transitions_from_workflow", "idx_task_step_transitions_to_workflow"} {
		var indexCount int
		if err := db.Get(&indexCount, `SELECT COUNT(*) FROM pg_indexes WHERE schemaname = current_schema() AND indexname = $1`, name); err != nil || indexCount != 1 {
			t.Fatalf("index %s: count=%d err=%v", name, indexCount, err)
		}
	}
	if _, err := db.Exec(db.Rebind(`INSERT INTO task_step_transitions
		(task_id, to_workflow_id, to_workflow_step_id, trigger, actor_kind, contract_version, occurred_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`), "task-pre-ledger-pg", "wf-pg", "draft", "test", "system", 1, now); err != nil {
		t.Fatal(err)
	}
	groups, err := repo.ListWorkflowTransitionGroups(context.Background(), "ws-pre-ledger-pg", "wf-pg", "", 10)
	if err != nil || len(groups) != 1 || groups[0].Kind != "entry" {
		t.Fatalf("postgres groups = %+v, err=%v", groups, err)
	}
}

// @covers AC-TASKS-STEP-TRANSITION-READ-001.1
func TestPostgresTaskStepTransitionRecordsOldestFirstWithNames(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	now := time.Now().UTC()
	if _, err := db.Exec(db.Rebind(`INSERT INTO tasks (id, workspace_id, title, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`),
		"task-records-pg", "ws-records-pg", "Records", now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(db.Rebind(`INSERT INTO workflow_steps (id, workflow_id, name, position) VALUES (?, ?, ?, ?)`),
		"draft-pg", "wf-pg", "Draft", 0); err != nil {
		t.Fatal(err)
	}
	for _, move := range [][3]any{{nil, "draft-pg", "user-pg"}, {"draft-pg", "gone-pg", nil}} {
		if _, err := db.Exec(db.Rebind(`INSERT INTO task_step_transitions
			(task_id, from_workflow_id, from_workflow_step_id, to_workflow_id, to_workflow_step_id, trigger, actor_kind, actor_id, contract_version, occurred_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`), "task-records-pg", "wf-pg", move[0], "wf-pg", move[1], "manual_move", "human", move[2], 1, now); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := repo.ListTaskStepTransitionRecords(context.Background(), "task-records-pg", 0, 10)
	if err != nil || len(rows) != 2 || rows[0].ID >= rows[1].ID {
		t.Fatalf("postgres records = %+v, err=%v", rows, err)
	}
	if rows[0].ToWorkflowStepName == nil || *rows[0].ToWorkflowStepName != "Draft" || rows[0].ActorID == nil || *rows[0].ActorID != "user-pg" {
		t.Fatalf("first record = %+v", rows[0])
	}
	if rows[1].FromWorkflowStepName == nil || rows[1].ToWorkflowStepName != nil || rows[1].ActorID != nil {
		t.Fatalf("second record = %+v", rows[1])
	}
}
