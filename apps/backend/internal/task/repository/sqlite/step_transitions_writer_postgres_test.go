package sqlite

// Postgres parity for the ledger writer: genesis row, a move via UpdateTask,
// AddTaskToWorkflow/RemoveTaskFromWorkflow, and the chain invariant. Skips
// unless KANDEV_TEST_POSTGRES_DSN is set; CI runs these in postgres-boot.

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/steptelemetry"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

func TestPostgresLedgerWriterGenesisMoveAttachDetach(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := steptelemetry.WithAttribution(context.Background(), steptelemetry.Attribution{
		Trigger: steptelemetry.TriggerManualMove, ActorKind: steptelemetry.ActorHuman, ActorID: "user-pg",
	})

	task := &models.Task{ID: "task-pg-ledger", WorkspaceID: "ws-pg-ledger", WorkflowID: "wf-pg", WorkflowStepID: "step-a", Title: "PG Task", Priority: "medium"}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	task.WorkflowStepID = "step-b"
	if err := repo.UpdateTask(ctx, task); err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}

	if err := repo.RemoveTaskFromWorkflow(ctx, task.ID, "wf-pg"); err != nil {
		t.Fatalf("RemoveTaskFromWorkflow: %v", err)
	}
	if err := repo.AddTaskToWorkflow(ctx, task.ID, "wf-pg-2", "step-c", 0); err != nil {
		t.Fatalf("AddTaskToWorkflow: %v", err)
	}

	rows := stepTransitionRowsForTask(t, repo, task.ID)
	if len(rows) != 4 {
		t.Fatalf("rows = %d, want 4 (genesis, move, detach, attach)", len(rows))
	}
	for i := 1; i < len(rows); i++ {
		prevTo := ""
		if rows[i-1].toWorkflowStepID != nil {
			prevTo = *rows[i-1].toWorkflowStepID
		}
		curFrom := ""
		if rows[i].fromWorkflowStepID != nil {
			curFrom = *rows[i].fromWorkflowStepID
		}
		if prevTo != curFrom {
			t.Fatalf("chain broken at row %d: prev to=%q, this from=%q", i, prevTo, curFrom)
		}
	}
	last := rows[len(rows)-1]
	if last.toWorkflowStepID == nil || *last.toWorkflowStepID != "step-c" {
		t.Fatalf("final to_workflow_step_id = %v, want step-c", last.toWorkflowStepID)
	}
}
