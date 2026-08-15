package sqlite

// Postgres parity for the ledger writer: genesis row, a move via UpdateTask,
// AddTaskToWorkflow/RemoveTaskFromWorkflow, and the chain invariant. Skips
// unless KANDEV_TEST_POSTGRES_DSN is set; CI runs these in postgres-boot.

import (
	"context"
	"sync"
	"testing"

	"golang.org/x/sync/errgroup"

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
	if err := repo.CreateWorkspace(context.Background(), &models.Workspace{ID: "ws-pg-ledger", Name: "PG Ledger Workspace"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}

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

// TestPostgresLedgerWriterConcurrentMovesProduceIntactChain is the Postgres
// counterpart of TestChainConcurrentMovesProduceTwoRowsWithIntactChain
// (step_transitions_chain_test.go), barrier-controlled rather than relying on
// goroutine-scheduling luck to exercise readTaskStepInTx's real Postgres
// "FOR UPDATE" lock contention: both movers block on the same gate so they
// race into the lock as close to simultaneously as genuinely concurrent
// callers would. This is the scenario that caught a real bug during this
// PR's review — occurred_at was stamped before the transactional lock, so
// two racing movers could commit in one order but record timestamps in the
// other, breaking the (occurred_at, id) chain invariant only under real
// concurrent Postgres connections (SQLite's single-writer pool serializes
// regardless, which is why no SQLite test could have caught it).
func TestPostgresLedgerWriterConcurrentMovesProduceIntactChain(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := steptelemetry.WithAttribution(context.Background(), steptelemetry.Attribution{
		Trigger: steptelemetry.TriggerManualMove, ActorKind: steptelemetry.ActorHuman, ActorID: "user-pg-concurrent",
	})
	if err := repo.CreateWorkspace(context.Background(), &models.Workspace{ID: "ws-pg-concurrent", Name: "PG Concurrent Workspace"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}

	task := &models.Task{ID: "task-pg-concurrent", WorkspaceID: "ws-pg-concurrent", WorkflowID: "wf-pg-concurrent", WorkflowStepID: "step-a", Title: "PG Concurrent Task", Priority: "medium"}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	var start sync.WaitGroup
	start.Add(1)
	var g errgroup.Group
	g.Go(func() error {
		start.Wait()
		moved := *task
		moved.WorkflowStepID = "step-b"
		return repo.UpdateTask(ctx, &moved)
	})
	g.Go(func() error {
		start.Wait()
		moved := *task
		moved.WorkflowStepID = "step-c"
		return repo.UpdateTask(ctx, &moved)
	})
	start.Done()
	if err := g.Wait(); err != nil {
		t.Fatalf("concurrent UpdateTask: %v", err)
	}

	rows := assertChainIntact(t, repo, task.ID)
	if len(rows) != 3 {
		t.Fatalf("rows = %d, want 3 (genesis + 2 concurrent moves)", len(rows))
	}

	final, err := repo.GetTask(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if final.WorkflowStepID != "step-b" && final.WorkflowStepID != "step-c" {
		t.Fatalf("final workflow_step_id = %q, want step-b or step-c (last committed writer wins)", final.WorkflowStepID)
	}
}
