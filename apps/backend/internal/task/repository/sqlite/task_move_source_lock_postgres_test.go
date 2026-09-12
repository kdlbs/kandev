package sqlite

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/kandev/kandev/internal/testutil"
)

// TestPostgresCrossStepMoveLocksSourceStepAgainstConcurrentReorder proves
// updateTaskWithWorkflowStepAdmission locks the step a task is LEAVING, not
// only the one it is entering (REQ-TASKS-KANBAN-TASK-REORDERING-001.26): a
// ReorderStepTasks call racing a cross-step move of the source step must
// either serialize behind the move and see the departure (step_changed) or
// commit first and hand the move a still-consistent step. Before this lock
// existed, the two transactions could interleave on Postgres with the
// reorder's read of source-step membership straddling the move's write,
// letting the reorder commit against membership that had already changed
// underneath it with no conflict signal at all.
// Skips unless KANDEV_TEST_POSTGRES_DSN is set.
func TestPostgresCrossStepMoveLocksSourceStepAgainstConcurrentReorder(t *testing.T) {
	const (
		sourceStep = "move-lock-source"
		targetStep = "move-lock-target"
	)
	db := openIsolatedPostgresMultiConn(t, testutil.PostgresDSNFromEnv(t), 2)
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "move-lock-ws", Name: "Move lock workspace"}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "move-lock-workflow", WorkspaceID: "move-lock-ws", Name: "Move lock workflow"}); err != nil {
		t.Fatalf("seed workflow: %v", err)
	}
	for _, step := range []string{sourceStep, targetStep} {
		if _, err := db.Exec(db.Rebind(`
			INSERT INTO workflow_steps (id, workflow_id, name, position)
			VALUES (?, ?, ?, ?)
		`), step, "move-lock-workflow", step, 0); err != nil {
			t.Fatalf("seed workflow step %s: %v", step, err)
		}
	}

	moving := &models.Task{
		ID: "move-lock-moving", WorkspaceID: "move-lock-ws", WorkflowID: "move-lock-workflow",
		WorkflowStepID: sourceStep, Title: "Moving", WIPAdmitted: true,
	}
	stays := &models.Task{
		ID: "move-lock-stays", WorkspaceID: "move-lock-ws", WorkflowID: "move-lock-workflow",
		WorkflowStepID: sourceStep, Title: "Stays", WIPAdmitted: true,
	}
	for _, task := range []*models.Task{moving, stays} {
		if err := repo.CreateTask(ctx, task); err != nil {
			t.Fatalf("seed task %s: %v", task.ID, err)
		}
	}

	start := make(chan struct{})
	var wg sync.WaitGroup
	var moveErr error
	var reorderErr error

	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		_, moveErr = repo.UpdateTaskWithWorkflowStepAdmission(ctx, moving, sourceStep, targetStep, 0)
	}()
	go func() {
		defer wg.Done()
		<-start
		_, _, reorderErr = repo.ReorderStepTasks(ctx, sourceStep, ReorderBandAdmitted, []string{moving.ID, stays.ID})
	}()
	close(start)
	wg.Wait()

	if moveErr != nil {
		t.Fatalf("cross-step move must always succeed (unlimited target): %v", moveErr)
	}
	if reorderErr != nil && !errors.Is(reorderErr, repoerrors.ErrStepChanged) {
		t.Fatalf("reorder error = %v, want nil or ErrStepChanged", reorderErr)
	}

	stored, err := repo.GetTask(ctx, moving.ID)
	if err != nil {
		t.Fatalf("reload moved task: %v", err)
	}
	if stored.WorkflowStepID != targetStep {
		t.Fatalf("moved task ended up in step %q, want %q", stored.WorkflowStepID, targetStep)
	}
}
