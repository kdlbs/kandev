package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	workflowrepo "github.com/kandev/kandev/internal/workflow/repository"
	"github.com/kandev/kandev/internal/workflow/routing"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

func newWorkflowRoutesPostgresRepo(t *testing.T) *Repository {
	t.Helper()
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	require.NoError(t, err)
	return repo
}

func TestPostgresTrustedWriterCreatesCurrentCoordinatorGrant(t *testing.T) {
	repo := newWorkflowRoutesPostgresRepo(t)
	ctx := context.Background()
	now := time.Now().UTC()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{
		ID: "pg-grant-workspace", Name: "Grant", OwnerID: "owner", CreatedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{
		ID: "pg-coordinator", WorkspaceID: "pg-grant-workspace", Title: "Coordinator",
		Origin: models.TaskOriginAutomationRun, Metadata: map[string]interface{}{"automation_id": "pg-automation"},
		CreatedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, repo.DesignateAutomationCoordinator(ctx, "pg-coordinator", "pg-automation"))
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "pg-coordinator-session", TaskID: "pg-coordinator", State: models.TaskSessionStateRunning,
		StartedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: "pg-coordinator-execution", SessionID: "pg-coordinator-session", TaskID: "pg-coordinator",
		ExecutorID: "executor", Status: models.ExecutorRunningStatusRunning,
		AgentExecutionID: "pg-coordinator-execution",
	}))

	granted, err := repo.IsCurrentCoordinatorGrant(
		ctx, "pg-grant-workspace", "pg-coordinator", "pg-coordinator-session",
		"pg-coordinator-execution", "pg-automation",
	)
	require.NoError(t, err)
	require.True(t, granted)
	granted, err = repo.IsCurrentCoordinatorGrant(
		ctx, "pg-grant-workspace", "pg-coordinator", "pg-coordinator-session",
		"pg-coordinator-execution", "different-automation",
	)
	require.NoError(t, err)
	require.False(t, granted)
}

func TestPostgresTerminalRouteAtomicallySettlesStateAndPendingRow(t *testing.T) {
	repo := newWorkflowRoutesPostgresRepo(t)
	ctx := context.Background()
	now := time.Now().UTC()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "pg-route-workspace", Name: "Route"}))
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{
		ID: "pg-route-workflow", WorkspaceID: "pg-route-workspace", Name: "Route",
	}))
	steps, err := workflowrepo.NewWithDB(repo.db, repo.db, nil)
	require.NoError(t, err)
	for _, step := range []*wfmodels.WorkflowStep{
		{ID: "pg-step-pr", WorkflowID: "pg-route-workflow", Name: "PR", Position: 0},
		{ID: "pg-step-done", WorkflowID: "pg-route-workflow", Name: "Done", Position: 1},
	} {
		require.NoError(t, steps.CreateStep(ctx, step))
	}
	task := &models.Task{
		ID: "pg-terminal-task", WorkspaceID: "pg-route-workspace", WorkflowID: "pg-route-workflow",
		WorkflowStepID: "pg-step-pr", Title: "Terminal", State: v1.TaskStateInProgress,
		CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, repo.CreateTask(ctx, task))
	_, err = repo.db.ExecContext(ctx, `
		CREATE TABLE pending_moves (
			id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL UNIQUE,
			task_id TEXT NOT NULL
		)
	`)
	require.NoError(t, err)
	_, err = repo.db.ExecContext(ctx, `
		INSERT INTO pending_moves (id, session_id, task_id)
		VALUES ('pg-pending', 'pg-session', 'pg-terminal-task')
	`)
	require.NoError(t, err)
	task.WorkflowStepID = "pg-step-done"
	completed := v1.TaskStateCompleted
	op := routing.Operation{
		ID: "pg-terminal-operation", TaskID: task.ID, Producer: routing.ProducerWorkflow,
		ExpectedStepID: "pg-step-pr", TargetStepID: "pg-step-done",
	}
	_, applied, err := repo.UpdateTaskWithWorkflowStepAdmissionAndStateIfAtStep(
		routing.WithOperation(ctx, op), task, "pg-step-pr", "pg-step-done", 0,
		&completed, false, "pg-route-workflow",
	)
	require.NoError(t, err)
	require.True(t, applied)
	stored, err := repo.GetTask(ctx, task.ID)
	require.NoError(t, err)
	require.Equal(t, v1.TaskStateCompleted, stored.State)
	var pending int
	require.NoError(t, repo.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM pending_moves WHERE task_id = $1`, task.ID).Scan(&pending))
	require.Zero(t, pending)
}

func TestPostgresWorkflowRouteEffectLeaseCanBeRenewed(t *testing.T) {
	repo := newWorkflowRoutesPostgresRepo(t)
	ctx := context.Background()
	now := time.Date(2026, 8, 31, 15, 0, 0, 0, time.UTC)
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "pg-effect-workspace", Name: "Effect"}))
	task := &models.Task{ID: "pg-effect-task", WorkspaceID: "pg-effect-workspace", Title: "Effect"}
	require.NoError(t, repo.CreateTask(ctx, task))
	require.NoError(t, repo.RecordWorkflowRouteOperation(ctx, routing.Operation{
		ID: "pg-effect-operation", TaskID: task.ID, Producer: routing.ProducerManualMove,
		ExpectedStepID: "pr", TargetStepID: "done", Outcome: routing.OutcomeCommitted,
		TransitionID: 1, EffectID: "pg-effect",
	}))
	claimed, err := repo.ClaimWorkflowRouteEffect(ctx, "pg-effect", "worker-a", now, time.Minute)
	require.NoError(t, err)
	require.True(t, claimed)
	renewed, err := repo.RenewWorkflowRouteEffect(ctx, "pg-effect", "worker-a", now.Add(45*time.Second))
	require.NoError(t, err)
	require.True(t, renewed)
	claimed, err = repo.ClaimWorkflowRouteEffect(ctx, "pg-effect", "worker-b", now.Add(90*time.Second), time.Minute)
	require.NoError(t, err)
	require.False(t, claimed)
}
