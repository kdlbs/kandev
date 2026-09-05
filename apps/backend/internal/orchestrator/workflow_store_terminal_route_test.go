package orchestrator

import (
	"context"
	"testing"

	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	"github.com/kandev/kandev/internal/workflow/routing"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

func TestWorkflowStoreTerminalRouteCommitsTaskStateAndPendingSettlement(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "terminal-task", "terminal-session", "step-pr")
	_, err := repo.DB().ExecContext(ctx, `
		CREATE TABLE pending_moves (
			id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL UNIQUE,
			task_id TEXT NOT NULL
		)
	`)
	require.NoError(t, err)
	_, err = repo.DB().ExecContext(ctx, `
		INSERT INTO pending_moves (id, session_id, task_id)
		VALUES ('pending-terminal', 'terminal-session', 'terminal-task')
	`)
	require.NoError(t, err)

	steps := newMockStepGetter()
	steps.steps["step-done"] = &wfmodels.WorkflowStep{
		ID: "step-done", WorkflowID: "wf1", Name: "Done", Position: 1,
	}
	store := newWorkflowStore(repo, steps, nil, noopPublisher, testLogger())
	operation := routing.Operation{
		ID: "terminal-engine-operation", TaskID: "terminal-task",
		Producer: routing.ProducerWorkflow, ExpectedStepID: "step-pr", TargetStepID: "step-done",
	}
	_, _, applied, err := store.applyTransitionIfAtStepRaw(
		routing.WithOperation(ctx, operation), "terminal-task", "step-pr", "step-done",
	)
	require.NoError(t, err)
	require.True(t, applied)

	task, err := repo.GetTask(ctx, "terminal-task")
	require.NoError(t, err)
	require.Equal(t, "step-done", task.WorkflowStepID)
	require.Equal(t, v1.TaskStateCompleted, task.State,
		"the terminal state must be part of the route transaction")
	var pending int
	require.NoError(t, repo.DB().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM pending_moves WHERE task_id = ?`, task.ID).Scan(&pending))
	require.Zero(t, pending, "terminal commit must settle pending routes in the same transaction")
}
