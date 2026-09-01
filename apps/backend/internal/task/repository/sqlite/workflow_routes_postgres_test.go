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

// TestPostgresWorkflowRouteOperationPendingToCommittedFillsIdentity mirrors
// TestWorkflowRouteOperationPendingToCommittedFillsTurnAndTarget on
// PostgreSQL: the real step_complete producer's pending pre-record (real
// turn_id, placeholder target_step_id, task's real workspace_id) must still
// upsert cleanly when the turn-end commit reuses the same OperationID with
// the real target_step_id but an unset workspace_id/turn_id in its context.
func TestPostgresWorkflowRouteOperationPendingToCommittedFillsIdentity(t *testing.T) {
	repo := newWorkflowRoutesPostgresRepo(t)
	ctx := context.Background()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "pg-two-phase-workspace", Name: "TwoPhase"}))
	task := &models.Task{
		ID: "pg-two-phase-task", WorkspaceID: "pg-two-phase-workspace",
		WorkflowStepID: "pg-step-work", Title: "TwoPhase",
	}
	require.NoError(t, repo.CreateTask(ctx, task))

	const opID = "pg-shared-step-complete-op"
	const turnID = "pg-turn-123"

	require.NoError(t, repo.RecordWorkflowRouteOperation(ctx, routing.Operation{
		ID: opID, TaskID: task.ID, WorkspaceID: task.WorkspaceID,
		Producer: routing.ProducerStepComplete, ExpectedStepID: "pg-step-work",
		ObservedStepID: "pg-step-work", SessionID: "pg-session-route", TurnID: turnID,
		ActorKind: "agent", ActorID: "pg-session-route", Outcome: routing.OutcomePending,
	}))

	task.WorkflowStepID = "pg-step-done"
	require.NoError(t, repo.UpdateTask(routing.WithOperation(ctx, routing.Operation{
		ID: opID, TaskID: task.ID, Producer: routing.ProducerStepComplete,
		ExpectedStepID: "pg-step-work", TargetStepID: "pg-step-done",
		SessionID: "pg-session-route", ActorKind: "agent", ActorID: "pg-session-route",
	}), task))

	readback, found, err := repo.GetWorkflowRouteOperation(ctx, opID)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, routing.OutcomeCommitted, readback.Outcome)
	require.Equal(t, "pg-step-done", readback.TargetStepID)
	require.Equal(t, turnID, readback.TurnID, "the pending turn_id must survive the commit fill")
	require.Equal(t, task.WorkspaceID, readback.WorkspaceID)

	stored, err := repo.GetTask(ctx, task.ID)
	require.NoError(t, err)
	require.Equal(t, "pg-step-done", stored.WorkflowStepID, "the task must actually advance, not roll back")
}

func TestPostgresWorkflowRouteOperationRejectsConflictingProgressiveIdentity(t *testing.T) {
	repo := newWorkflowRoutesPostgresRepo(t)
	ctx := context.Background()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "pg-conflict-workspace", Name: "Conflict"}))
	task := &models.Task{
		ID: "pg-conflict-task", WorkspaceID: "pg-conflict-workspace",
		WorkflowStepID: "pg-step-work", Title: "Conflict",
	}
	require.NoError(t, repo.CreateTask(ctx, task))

	operation := routing.Operation{
		ID: "pg-conflicting-step-complete-op", TaskID: task.ID, WorkspaceID: task.WorkspaceID,
		Producer: routing.ProducerStepComplete, ExpectedStepID: "pg-step-work",
		ObservedStepID: "pg-step-work", TargetStepID: "pg-step-done",
		SessionID: "pg-session-route", TurnID: "pg-turn-123",
		ActorKind: "agent", ActorID: "pg-session-route", Outcome: routing.OutcomePending,
	}
	require.NoError(t, repo.RecordWorkflowRouteOperation(ctx, operation))

	conflicting := operation
	conflicting.TurnID = "pg-turn-456"
	require.ErrorIs(t, repo.RecordWorkflowRouteOperation(ctx, conflicting), routing.ErrOperationIdentityConflict)
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
	begun, err := repo.BeginWorkflowRouteEffect(ctx, "pg-effect", "worker-a", now.Add(90*time.Second))
	require.NoError(t, err)
	require.True(t, begun)
	claimed, err = repo.ClaimWorkflowRouteEffect(ctx, "pg-effect", "worker-b", now.Add(10*time.Minute), time.Minute)
	require.NoError(t, err)
	require.False(t, claimed, "executing effects are non-reclaimable on PostgreSQL")
	completed, err := repo.CompleteWorkflowRouteEffect(ctx, "pg-effect", "worker-a", now.Add(10*time.Minute))
	require.NoError(t, err)
	require.True(t, completed)
}
