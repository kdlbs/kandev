package sqlite

import (
	"context"
	"errors"
	"testing"

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
