package lifecycle

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestStopTrackedPendingRecoveryCompletesExactGeneration(t *testing.T) {
	row := &models.ExecutorRunning{
		SessionID: "pending-session", TaskID: "pending-task",
		AgentExecutionID: "pending-execution", Runtime: executor.NameSprites,
	}
	manager, _ := task08ManagerWithBackend(t, &MockExecutor{name: executor.NameSprites})
	entry := manager.queueRemoteRecovery(row, task08RemoteSnapshot(row))
	execution := &AgentExecution{
		ID: row.AgentExecutionID, TaskID: row.TaskID, SessionID: row.SessionID,
		RuntimeName: executor.NameSprites,
	}
	require.NoError(t, manager.executionStore.Add(execution))

	require.NoError(t, manager.StopAgentWithReason(context.Background(), execution.ID, StopReasonTaskDeleted, true))
	_, tracked := manager.executionStore.Get(execution.ID)
	require.False(t, tracked)
	require.Nil(t, manager.pendingRemoteRecoveryForSession(row.SessionID))
	require.NoError(t, manager.recoveryGuard.CheckLaunchAllowed(row.SessionID))
	require.NotNil(t, entry)
}
