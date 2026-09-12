package sqlite

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

// TestUpdateTaskPreservesPositionAcrossConcurrentReorder proves the fix for
// the stale-position clobber: a caller that read a task before a reorder of
// its step committed, and then writes an ordinary field change (not a
// position change) using that pre-reorder snapshot, must not undo the
// reorder. Simulated deterministically rather than with real goroutines: the
// "stale" struct is exactly what a caller's in-memory copy would hold after
// GetTask ran before the reorder.
func TestUpdateTaskPreservesPositionAcrossConcurrentReorder(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "ws-preserve")
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-preserve", WorkspaceID: "ws-preserve", Name: "Workflow"}); err != nil {
		t.Fatal(err)
	}
	seedReorderStep(t, repo, "step-preserve", "wf-preserve")
	mustCreateReorderTask(t, ctx, repo, "task-a", "ws-preserve", "wf-preserve", "step-preserve")
	mustCreateReorderTask(t, ctx, repo, "task-b", "ws-preserve", "wf-preserve", "step-preserve")

	staleTaskA, err := repo.GetTask(ctx, "task-a")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if staleTaskA.Position != 0 {
		t.Fatalf("staleTaskA.Position = %d, want 0 before any reorder", staleTaskA.Position)
	}

	if _, _, err := repo.ReorderStepTasks(ctx, "step-preserve", ReorderBandAdmitted,
		[]string{"task-b", "task-a"}); err != nil {
		t.Fatalf("ReorderStepTasks: %v", err)
	}
	if got := taskPosition(t, ctx, repo, "task-a"); got != 1 {
		t.Fatalf("task-a position after reorder = %d, want 1", got)
	}

	// staleTaskA still holds position 0 from before the reorder. An ordinary
	// field edit using it must not write that stale value back.
	staleTaskA.Title = "renamed while stale"
	if err := repo.UpdateTask(ctx, staleTaskA); err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}

	if got := taskPosition(t, ctx, repo, "task-a"); got != 1 {
		t.Fatalf("task-a position after stale UpdateTask = %d, want 1 (the reorder's value preserved)", got)
	}
	reloaded, err := repo.GetTask(ctx, "task-a")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if reloaded.Title != "renamed while stale" {
		t.Fatalf("title = %q, want the ordinary edit to still apply", reloaded.Title)
	}
}

// TestUpdateTaskWithExplicitPositionWritesLiteralValue proves the one
// escape hatch (the generic task-update API's explicit position field,
// which predates the reorder contract) still writes exactly what the caller
// supplies, so fixing the stale-write bug above does not silently disable
// it.
func TestUpdateTaskWithExplicitPositionWritesLiteralValue(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "ws-explicit")
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-explicit", WorkspaceID: "ws-explicit", Name: "Workflow"}); err != nil {
		t.Fatal(err)
	}
	seedReorderStep(t, repo, "step-explicit", "wf-explicit")
	task := mustCreateReorderTask(t, ctx, repo, "task-explicit", "ws-explicit", "wf-explicit", "step-explicit")

	task.Position = 42
	if err := repo.UpdateTaskWithExplicitPosition(ctx, task); err != nil {
		t.Fatalf("UpdateTaskWithExplicitPosition: %v", err)
	}

	if got := taskPosition(t, ctx, repo, "task-explicit"); got != 42 {
		t.Fatalf("position = %d, want 42 (caller-supplied value honored)", got)
	}
}

// TestUpdateTaskIfWorkflowMatchesPreservesPosition covers the same-step
// plugin-move path (updateMovedTaskSameStep's CAS-guarded branch), which
// AC-TASKS-KANBAN-TASK-REORDERING-001.28 requires to leave position
// unchanged: it must not reintroduce a stale position either.
func TestUpdateTaskIfWorkflowMatchesPreservesPosition(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "ws-cas-preserve")
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-cas-preserve", WorkspaceID: "ws-cas-preserve", Name: "Workflow"}); err != nil {
		t.Fatal(err)
	}
	seedReorderStep(t, repo, "step-cas-preserve", "wf-cas-preserve")
	mustCreateReorderTask(t, ctx, repo, "task-a", "ws-cas-preserve", "wf-cas-preserve", "step-cas-preserve")
	mustCreateReorderTask(t, ctx, repo, "task-b", "ws-cas-preserve", "wf-cas-preserve", "step-cas-preserve")

	staleTaskA, err := repo.GetTask(ctx, "task-a")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}

	if _, _, err := repo.ReorderStepTasks(ctx, "step-cas-preserve", ReorderBandAdmitted,
		[]string{"task-b", "task-a"}); err != nil {
		t.Fatalf("ReorderStepTasks: %v", err)
	}

	staleTaskA.Description = "same-step plugin move while stale"
	if err := repo.UpdateTaskIfWorkflowMatches(ctx, staleTaskA, "wf-cas-preserve"); err != nil {
		t.Fatalf("UpdateTaskIfWorkflowMatches: %v", err)
	}

	if got := taskPosition(t, ctx, repo, "task-a"); got != 1 {
		t.Fatalf("task-a position = %d, want 1 (the reorder's value preserved)", got)
	}
}
