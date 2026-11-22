package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestRepairExecutorRunningDeadIfOwnerCurrentValidatesOwnerBeforeRepair(t *testing.T) {
	repo, owner, record := seedRemoteRecoveryOwnerCAS(t)
	ctx := context.Background()
	called := false
	err := repo.RepairExecutorRunningDeadIfOwnerCurrent(ctx, record.SessionID, record.AgentExecutionID,
		record.UpdatedAt, owner, func(context.Context) error {
			called = true
			return errors.New("owner changed")
		})
	require.ErrorContains(t, err, "owner changed")
	require.True(t, called)
	current, err := repo.GetExecutorRunningBySessionID(ctx, record.SessionID)
	require.NoError(t, err)
	require.Equal(t, models.ExecutorRunningStatusRunning, current.Status)

	require.NoError(t, repo.RepairExecutorRunningDeadIfOwnerCurrent(ctx, record.SessionID, record.AgentExecutionID,
		record.UpdatedAt, owner, func(context.Context) error { return nil }))
	current, err = repo.GetExecutorRunningBySessionID(ctx, record.SessionID)
	require.NoError(t, err)
	require.Equal(t, models.ExecutorRunningStatusStopped, current.Status)
	require.Equal(t, record.ResumeToken, current.ResumeToken)
}

func TestSettleStoppedRemoteExecutionIfOwnerCurrentCommitsRuntimeSessionAndTurnTogether(t *testing.T) {
	repo, owner, record := seedRemoteRecoveryOwnerCAS(t)
	ctx := context.Background()
	startedAt := time.Now().UTC().Add(-time.Minute)
	require.NoError(t, repo.UpdateTaskSessionState(ctx, record.SessionID, models.TaskSessionStateRunning, ""))
	require.NoError(t, repo.CreateTurn(ctx, &models.Turn{
		ID: "remote-recovery-owner-turn", TaskSessionID: record.SessionID, TaskID: record.TaskID,
		StartedAt: startedAt, CreatedAt: startedAt, UpdatedAt: startedAt,
	}))
	active, err := repo.GetActiveTurnBySessionID(ctx, record.SessionID)
	require.NoError(t, err)
	require.Equal(t, "remote-recovery-owner-turn", active.ID)
	tool := &models.Message{
		ID: "remote-recovery-owner-tool", TaskID: record.TaskID, TaskSessionID: record.SessionID,
		TurnID: active.ID, Type: models.MessageTypeToolCall,
		Metadata: map[string]interface{}{"tool_call_id": "remote-recovery-call", "status": "running"},
	}
	require.NoError(t, repo.CreateMessage(ctx, tool))

	changed, updatedAt, err := repo.SettleStoppedRemoteExecutionIfOwnerCurrent(
		ctx, record.SessionID, record.AgentExecutionID, record.UpdatedAt,
		models.TaskSessionStateRunning, active.ID, "", owner, func(context.Context) error { return nil },
	)
	require.NoError(t, err)
	require.True(t, changed)
	require.False(t, updatedAt.IsZero())
	runtime, err := repo.GetExecutorRunningBySessionID(ctx, record.SessionID)
	require.NoError(t, err)
	require.Equal(t, models.ExecutorRunningStatusStopped, runtime.Status)
	session, err := repo.GetTaskSession(ctx, record.SessionID)
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateWaitingForInput, session.State)
	require.Equal(t, updatedAt, session.UpdatedAt)
	turn, err := repo.GetTurn(ctx, active.ID)
	require.NoError(t, err)
	require.NotNil(t, turn.CompletedAt)
	require.Equal(t, turn.StartedAt, *turn.CompletedAt, "interrupted turn must have zero duration")
	settledTool, err := repo.GetMessage(ctx, tool.ID)
	require.NoError(t, err)
	require.Equal(t, "complete", settledTool.Metadata["status"], "interrupted-turn cleanup must settle its nonterminal tool calls")
}

func TestSettleStoppedRemoteExecutionIfOwnerCurrentCompletesProvenTerminalTurn(t *testing.T) {
	repo, owner, record := seedRemoteRecoveryOwnerCAS(t)
	ctx := context.Background()
	require.NoError(t, repo.UpdateTaskSessionState(ctx, record.SessionID, models.TaskSessionStateRunning, ""))
	startedAt := time.Now().UTC().Add(-time.Minute)
	require.NoError(t, repo.CreateTurn(ctx, &models.Turn{
		ID: "remote-recovery-owner-terminal-turn", TaskSessionID: record.SessionID, TaskID: record.TaskID,
		StartedAt: startedAt, CreatedAt: startedAt, UpdatedAt: startedAt,
	}))

	changed, _, err := repo.SettleStoppedRemoteExecutionIfOwnerCurrent(
		ctx, record.SessionID, record.AgentExecutionID, record.UpdatedAt,
		models.TaskSessionStateRunning, "remote-recovery-owner-terminal-turn", models.DeliverySubmissionFailed,
		owner, func(context.Context) error { return nil },
	)
	require.NoError(t, err)
	require.True(t, changed)
	turn, err := repo.GetTurn(ctx, "remote-recovery-owner-terminal-turn")
	require.NoError(t, err)
	require.NotNil(t, turn.CompletedAt)
	require.True(t, turn.CompletedAt.After(turn.StartedAt), "proven terminal output completes its original turn")
}

func TestSettleStoppedRemoteExecutionIfOwnerCurrentRollsBackWhenOwnerChanges(t *testing.T) {
	repo, owner, record := seedRemoteRecoveryOwnerCAS(t)
	ctx := context.Background()
	require.NoError(t, repo.UpdateTaskSessionState(ctx, record.SessionID, models.TaskSessionStateRunning, ""))
	startedAt := time.Now().UTC().Add(-time.Minute)
	require.NoError(t, repo.CreateTurn(ctx, &models.Turn{
		ID: "remote-recovery-owner-stale-turn", TaskSessionID: record.SessionID, TaskID: record.TaskID,
		StartedAt: startedAt, CreatedAt: startedAt, UpdatedAt: startedAt,
	}))
	ownerChanged := errors.New("owner changed")
	changed, _, err := repo.SettleStoppedRemoteExecutionIfOwnerCurrent(
		ctx, record.SessionID, record.AgentExecutionID, record.UpdatedAt,
		models.TaskSessionStateRunning, "remote-recovery-owner-stale-turn", "", owner,
		func(context.Context) error { return ownerChanged },
	)
	require.ErrorIs(t, err, ownerChanged)
	require.False(t, changed)
	runtime, err := repo.GetExecutorRunningBySessionID(ctx, record.SessionID)
	require.NoError(t, err)
	require.Equal(t, models.ExecutorRunningStatusRunning, runtime.Status)
	session, err := repo.GetTaskSession(ctx, record.SessionID)
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateRunning, session.State)
	turn, err := repo.GetTurn(ctx, "remote-recovery-owner-stale-turn")
	require.NoError(t, err)
	require.Nil(t, turn.CompletedAt)
}

func TestSettleStoppedRemoteExecutionIfOwnerCurrentPreservesExistingWaitingState(t *testing.T) {
	repo, owner, record := seedRemoteRecoveryOwnerCAS(t)
	ctx := context.Background()
	changed, _, err := repo.SettleStoppedRemoteExecutionIfOwnerCurrent(
		ctx, record.SessionID, record.AgentExecutionID, record.UpdatedAt,
		models.TaskSessionStateWaitingForInput, "", "", owner, func(context.Context) error { return nil },
	)
	require.NoError(t, err)
	require.False(t, changed)
	runtime, err := repo.GetExecutorRunningBySessionID(ctx, record.SessionID)
	require.NoError(t, err)
	require.Equal(t, models.ExecutorRunningStatusStopped, runtime.Status)
	session, err := repo.GetTaskSession(ctx, record.SessionID)
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateWaitingForInput, session.State)
}

func TestSettleStoppedRemoteExecutionIfOwnerCurrentRejectsChangedTurnAtomically(t *testing.T) {
	repo, owner, record := seedRemoteRecoveryOwnerCAS(t)
	ctx := context.Background()
	require.NoError(t, repo.UpdateTaskSessionState(ctx, record.SessionID, models.TaskSessionStateRunning, ""))
	err := repo.CreateTurn(ctx, &models.Turn{
		ID: "remote-recovery-owner-replaced-turn", TaskSessionID: record.SessionID, TaskID: record.TaskID,
		StartedAt: time.Now().UTC(),
	})
	require.NoError(t, err)
	_, _, err = repo.SettleStoppedRemoteExecutionIfOwnerCurrent(
		ctx, record.SessionID, record.AgentExecutionID, record.UpdatedAt,
		models.TaskSessionStateRunning, "different-turn", "", owner, func(context.Context) error { return nil },
	)
	require.Error(t, err)
	runtime, err := repo.GetExecutorRunningBySessionID(ctx, record.SessionID)
	require.NoError(t, err)
	require.Equal(t, models.ExecutorRunningStatusRunning, runtime.Status)
	session, err := repo.GetTaskSession(ctx, record.SessionID)
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateRunning, session.State)
	turn, err := repo.GetTurn(ctx, "remote-recovery-owner-replaced-turn")
	require.NoError(t, err)
	require.Nil(t, turn.CompletedAt)
}

func TestConfirmExecutorRunningAbsentIfOwnerCurrentRejectsReappearedRow(t *testing.T) {
	repo, owner, record := seedRemoteRecoveryOwnerCAS(t)
	ctx := context.Background()
	called := false
	err := repo.ConfirmExecutorRunningAbsentIfOwnerCurrent(ctx, record.SessionID, owner, func(context.Context) error {
		called = true
		return nil
	})
	require.ErrorIs(t, err, models.ErrExecutionRotated)
	require.False(t, called, "owner validation must not run while the executor row exists")

	require.NoError(t, repo.DeleteExecutorRunningBySessionID(ctx, record.SessionID))
	require.NoError(t, repo.ConfirmExecutorRunningAbsentIfOwnerCurrent(ctx, record.SessionID, owner, func(context.Context) error {
		return nil
	}))
}

func seedRemoteRecoveryOwnerCAS(t *testing.T) (*Repository, models.RemoteRecoveryOwnerLockIDs, *models.ExecutorRunning) {
	t.Helper()
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	const (
		taskID        = "remote-recovery-owner-task"
		workspaceID   = "ws-remote-recovery-owner-task"
		sessionID     = "remote-recovery-owner-session"
		environmentID = "remote-recovery-owner-environment"
		executionID   = "remote-recovery-owner-execution"
	)
	seedExecutorRunningCleanupTask(t, repo, taskID)
	require.NoError(t, repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: environmentID, TaskID: taskID, ExecutorType: string(models.ExecutorTypeRemoteDocker),
		ExecutorID: "remote-recovery-owner-executor", WorkspacePath: "/workspace/remote-recovery-owner",
		Status: models.TaskEnvironmentStatusReady,
	}))
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: sessionID, TaskID: taskID, TaskEnvironmentID: environmentID,
		State: models.TaskSessionStateWaitingForInput,
	}))
	row := &models.ExecutorRunning{
		ID: sessionID, SessionID: sessionID, TaskID: taskID, AgentExecutionID: executionID,
		Runtime: agentruntime.RuntimeRemoteDocker, Status: models.ExecutorRunningStatusRunning,
		ResumeToken: "resume-owner-cas", Resumable: true,
	}
	require.NoError(t, repo.UpsertExecutorRunning(ctx, row))
	current, err := repo.GetExecutorRunningBySessionID(ctx, sessionID)
	require.NoError(t, err)
	return repo, models.RemoteRecoveryOwnerLockIDs{
		TaskID: taskID, SessionID: sessionID, WorkspaceID: workspaceID,
		EnvironmentID: environmentID, EnvironmentOwnerTaskID: taskID,
	}, current
}
