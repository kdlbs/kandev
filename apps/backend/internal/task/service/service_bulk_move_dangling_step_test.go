package service

import (
	"context"
	"testing"
)

// TestService_BulkMoveTasksToleratesDanglingSourceStep proves
// orderTasksForBulkMove no longer aborts the whole batch when one task's
// source step cannot be resolved (e.g. concurrently deleted): that task is
// ordered last instead, and every task in the batch still moves.
func TestService_BulkMoveTasksToleratesDanglingSourceStep(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	seedMoveWorkflows(t, ctx, repo)
	seedMoveSteps(svc)
	createMoveTask(t, ctx, repo, "task-bulk-resolvable", "wf-source", "step-source", nil)
	createMoveTask(t, ctx, repo, "task-bulk-dangling", "wf-source", "step-deleted", nil)

	_, err := svc.BulkMoveTasks(ctx, "wf-source", "", "wf-target", "step-target")
	if err != nil {
		t.Fatalf("BulkMoveTasks: %v", err)
	}

	for _, id := range []string{"task-bulk-resolvable", "task-bulk-dangling"} {
		task, err := repo.GetTask(ctx, id)
		if err != nil {
			t.Fatalf("GetTask(%s): %v", id, err)
		}
		if task.WorkflowStepID != "step-target" {
			t.Fatalf("task %s WorkflowStepID = %q, want step-target", id, task.WorkflowStepID)
		}
	}
}
