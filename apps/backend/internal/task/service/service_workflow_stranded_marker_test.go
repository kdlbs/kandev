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

// TestService_MoveTaskWithOptionsRejectsZeroTransitionCommit proves that when an
// optioned move's atomic write finds the task already on the target step (a
// concurrent move landed it there between this call's read and its write), the
// committed workflow_move_pending marker is cleared and the move is reported as
// ErrMoveConflict. Left unfixed the marker strands in metadata — task.moved
// never fires on a zero-transition write, so nothing consumes it — while the
// caller was told its one-shot override was accepted.
func TestService_MoveTaskWithOptionsRejectsZeroTransitionCommit(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	seedMoveWorkflows(t, ctx, repo)
	seedMoveSteps(svc)
	createMoveTask(t, ctx, repo, "task-zero-transition", "wf-source", "step-source", nil)
	createMoveSession(t, ctx, repo, "session-zero", "task-zero-transition", models.TaskSessionStateWaitingForInput, models.ReviewStatusNone)

	raceRepo := &workflowMoveRaceRepo{Repository: repo}
	raceRepo.inject = func() {
		// A concurrent, legitimate move lands the task on step-review-target
		// before this call's own write commits, so that write produces no step
		// transition.
		if _, err := svc.MoveTaskWithOptions(ctx, "task-zero-transition", "wf-source", "step-review-target", 0, MoveTaskOptions{
			AllowActivePrimarySession: true,
		}); err != nil {
			t.Fatalf("concurrent legitimate move (injected mid-call) failed: %v", err)
		}
	}
	svc.tasks = raceRepo

	_, err := svc.MoveTaskWithOptions(ctx, "task-zero-transition", "wf-source", "step-review-target", 0, MoveTaskOptions{
		AllowActivePrimarySession: true,
		EntryOptions:              &workflowmove.EntryOptions{Instructions: "please review"},
	})
	if !errors.Is(err, workflowmove.ErrMoveConflict) {
		t.Fatalf("expected ErrMoveConflict on zero-transition committed write, got %v", err)
	}

	stored, err := repo.GetTask(ctx, "task-zero-transition")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if _, pending := stored.Metadata[models.MetaKeyWorkflowMovePending]; pending {
		t.Fatalf("expected stranded workflow_move_pending marker cleared, metadata=%+v", stored.Metadata)
	}
	if stored.WorkflowStepID != "step-review-target" {
		t.Fatalf("concurrent move must survive the rejected optioned move: got step %s, want step-review-target", stored.WorkflowStepID)
	}
}
