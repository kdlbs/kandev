package sqlite

// Coverage for CountStepEntries, the read counterpart to recordStepTransition
// that backs the REQ-TWS-001 step-entry number. It is a thin COUNT(*) over
// the existing ledger, so these cases focus on the boundary behaviour the
// spec calls out rather than re-testing the writer.

import (
	"context"
	"testing"
)

func TestCountStepEntries_ZeroRowsForUnknownStep(t *testing.T) {
	repo := newStepTransitionsTestRepo(t)
	ctx := context.Background()
	createStepTransitionsTestTaskWithCtx(t, repo, ctx, "task-cse-1", "wf-1", "step-a")

	count, err := repo.CountStepEntries(ctx, "task-cse-1", "step-never-visited")
	if err != nil {
		t.Fatalf("CountStepEntries: %v", err)
	}
	if count != 0 {
		t.Fatalf("count = %d, want 0", count)
	}
}

func TestCountStepEntries_GenesisRowCountsAsOneEntry(t *testing.T) {
	repo := newStepTransitionsTestRepo(t)
	ctx := context.Background()
	createStepTransitionsTestTaskWithCtx(t, repo, ctx, "task-cse-2", "wf-1", "step-a")

	count, err := repo.CountStepEntries(ctx, "task-cse-2", "step-a")
	if err != nil {
		t.Fatalf("CountStepEntries: %v", err)
	}
	if count != 1 {
		t.Fatalf("count = %d, want 1", count)
	}
}

func TestCountStepEntries_LeaveAndReturnIsASecondEntry(t *testing.T) {
	repo := newStepTransitionsTestRepo(t)
	ctx := context.Background()
	task := createStepTransitionsTestTaskWithCtx(t, repo, ctx, "task-cse-3", "wf-1", "step-a")

	task.WorkflowStepID = "step-b"
	if err := repo.UpdateTask(ctx, task); err != nil {
		t.Fatalf("UpdateTask to step-b: %v", err)
	}
	task.WorkflowStepID = "step-a"
	if err := repo.UpdateTask(ctx, task); err != nil {
		t.Fatalf("UpdateTask back to step-a: %v", err)
	}

	count, err := repo.CountStepEntries(ctx, "task-cse-3", "step-a")
	if err != nil {
		t.Fatalf("CountStepEntries: %v", err)
	}
	if count != 2 {
		t.Fatalf("count = %d, want 2 (genesis entry + the return)", count)
	}
}

func TestCountStepEntries_ScopedByTaskID(t *testing.T) {
	repo := newStepTransitionsTestRepo(t)
	ctx := context.Background()
	createStepTransitionsTestTaskWithCtx(t, repo, ctx, "task-cse-4a", "wf-1", "step-a")
	createStepTransitionsTestTaskWithCtx(t, repo, ctx, "task-cse-4b", "wf-1", "step-a")

	count, err := repo.CountStepEntries(ctx, "task-cse-4a", "step-a")
	if err != nil {
		t.Fatalf("CountStepEntries: %v", err)
	}
	if count != 1 {
		t.Fatalf("count = %d, want 1 (must not include the other task's row)", count)
	}
}

func TestCountStepEntries_EmptyTaskIDOrStepIDReturnsZero(t *testing.T) {
	repo := newStepTransitionsTestRepo(t)
	ctx := context.Background()
	createStepTransitionsTestTaskWithCtx(t, repo, ctx, "task-cse-5", "wf-1", "step-a")

	count, err := repo.CountStepEntries(ctx, "", "step-a")
	if err != nil {
		t.Fatalf("CountStepEntries with empty task id: %v", err)
	}
	if count != 0 {
		t.Fatalf("count = %d, want 0 for empty task id", count)
	}

	count, err = repo.CountStepEntries(ctx, "task-cse-5", "")
	if err != nil {
		t.Fatalf("CountStepEntries with empty step id: %v", err)
	}
	if count != 0 {
		t.Fatalf("count = %d, want 0 for empty step id", count)
	}
}
