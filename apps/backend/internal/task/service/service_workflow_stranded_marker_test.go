package service

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	workflowmove "github.com/kandev/kandev/internal/workflow/move"
)

// seedStrandedMarker plants a workflow_move_pending marker on a task so tests
// can exercise recovery from a move that never cleared its own marker.
func seedStrandedMarker(t *testing.T, ctx context.Context, repo interface {
	GetTask(context.Context, string) (*models.Task, error)
	UpdateTask(context.Context, *models.Task) error
}, taskID, fromStepID string) {
	t.Helper()
	task, err := repo.GetTask(ctx, taskID)
	if err != nil {
		t.Fatalf("GetTask(%s): %v", taskID, err)
	}
	if task.Metadata == nil {
		task.Metadata = map[string]interface{}{}
	}
	task.Metadata[models.MetaKeyWorkflowMovePending] = map[string]interface{}{
		"from_step_id": fromStepID,
		"move_id":      "stranded-move",
		"options":      "{}",
	}
	if err := repo.UpdateTask(ctx, task); err != nil {
		t.Fatalf("UpdateTask(%s): %v", taskID, err)
	}
}

// TestService_MoveTaskClearsStrandedPendingMarker proves a plain move (no entry
// options) is never blocked by a stranded marker and clears it, so a task can
// never get permanently stuck behind a pending marker that failed to complete.
func TestService_MoveTaskClearsStrandedPendingMarker(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	seedMoveWorkflows(t, ctx, repo)
	seedMoveSteps(svc)
	createMoveTask(t, ctx, repo, "task-stranded", "wf-source", "step-source", nil)
	seedStrandedMarker(t, ctx, repo, "task-stranded", "step-source")

	moved, err := svc.MoveTask(ctx, "task-stranded", "wf-source", "step-review-target", 0)
	if err != nil {
		t.Fatalf("plain move should clear a stranded marker, got: %v", err)
	}
	if moved.Task.WorkflowStepID != "step-review-target" {
		t.Fatalf("expected step-review-target, got %s", moved.Task.WorkflowStepID)
	}
	stored, err := repo.GetTask(ctx, "task-stranded")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if _, pending := stored.Metadata[models.MetaKeyWorkflowMovePending]; pending {
		t.Fatalf("expected stranded marker cleared, metadata=%+v", stored.Metadata)
	}
}

// TestService_MoveTaskWithOptionsConflictsWithPendingMarker proves a new
// optioned move still fails closed when a marker is already in flight, so two
// concurrent one-shot moves cannot both persist their options.
func TestService_MoveTaskWithOptionsConflictsWithPendingMarker(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	seedMoveWorkflows(t, ctx, repo)
	seedMoveSteps(svc)
	createMoveTask(t, ctx, repo, "task-conflict", "wf-source", "step-source", nil)
	createMoveSession(t, ctx, repo, "session-conflict", "task-conflict", models.TaskSessionStateWaitingForInput, models.ReviewStatusNone)
	seedStrandedMarker(t, ctx, repo, "task-conflict", "step-source")

	_, err := svc.MoveTaskWithOptions(ctx, "task-conflict", "wf-source", "step-review-target", 0, MoveTaskOptions{
		AllowActivePrimarySession: true,
		EntryOptions:              &workflowmove.EntryOptions{Instructions: "please review"},
	})
	if !errors.Is(err, workflowmove.ErrMoveConflict) {
		t.Fatalf("expected ErrMoveConflict, got %v", err)
	}
}
