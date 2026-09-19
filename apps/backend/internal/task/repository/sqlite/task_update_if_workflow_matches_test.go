package sqlite

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

// TestUpdateTaskIfWorkflowMatches_ReturnsNotFoundWhenTaskDeletedConcurrently
// pins the design's binding error-mapping table (AC-004.6: "NotFound is
// reserved for the addressed resource" and wins the precedence ladder over
// every other case). updateTaskTx discarded readTaskStepInTx's found bool, so
// a task deleted between a plugin's GetTask pre-read and this CAS write
// surfaced as fromWorkflowID="" — which never equals a non-empty
// expectedWorkflowID — firing the CAS-conflict branch
// (ErrWorkflowResolutionConflict, mapped to codes.Aborted) before the
// rows==0 check further down ever ran. Simulates the race directly: delete
// the task, then call UpdateTaskIfWorkflowMatches against the now-deleted
// row exactly as updateMovedTaskSameStep does.
func TestUpdateTaskIfWorkflowMatches_ReturnsNotFoundWhenTaskDeletedConcurrently(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "workspace-cas-notfound")
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "workflow-cas-notfound", WorkspaceID: "workspace-cas-notfound", Name: "Workflow"}); err != nil {
		t.Fatal(err)
	}
	seedCASWorkflowStep(t, repo, "workflow-cas-notfound", "step-source", 0)

	task := &models.Task{
		ID: "task-cas-deleted", WorkspaceID: "workspace-cas-notfound", WorkflowID: "workflow-cas-notfound",
		WorkflowStepID: "step-source", Title: "Deleted mid-flight",
	}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if err := repo.DeleteTask(ctx, task.ID); err != nil {
		t.Fatalf("DeleteTask: %v", err)
	}

	err := repo.UpdateTaskIfWorkflowMatches(ctx, task, "workflow-cas-notfound")
	if err == nil {
		t.Fatal("UpdateTaskIfWorkflowMatches on a deleted task: got nil error, want ErrTaskNotFound")
	}
	if !errors.Is(err, ErrTaskNotFound) {
		t.Fatalf("UpdateTaskIfWorkflowMatches on a deleted task: err = %v, want errors.Is(err, ErrTaskNotFound) (NotFound is reserved for the addressed resource and wins the precedence ladder)", err)
	}
	if errors.Is(err, ErrWorkflowResolutionConflict) {
		t.Fatalf("UpdateTaskIfWorkflowMatches on a deleted task: err = %v also matches ErrWorkflowResolutionConflict — the CAS-conflict branch must not fire before the not-found check", err)
	}
}
