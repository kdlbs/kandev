package lifecycle

import (
	"testing"

	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestHasPendingRemoteRecoveryRequiresExactCurrentGuardedOwner(t *testing.T) {
	manager, _ := task08ManagerWithBackend(t, NewSSHExecutor(nil, nil, nil, newTestLogger()))

	first := &models.ExecutorRunning{
		SessionID: "session-pending-query", AgentExecutionID: "execution-first",
		Runtime: agentruntime.RuntimeSSH,
	}
	firstEntry := manager.queueRemoteRecovery(first, nil)
	require.NotNil(t, firstEntry)
	require.True(t, manager.HasPendingRemoteRecovery(first.SessionID, first.AgentExecutionID))
	require.False(t, manager.WasSessionRetracked(first.SessionID),
		"pending remote ownership must not imply successful re-tracking")
	require.False(t, manager.HasPendingRemoteRecovery("other-session", first.AgentExecutionID))
	require.False(t, manager.HasPendingRemoteRecovery(first.SessionID, ""))
	require.False(t, manager.HasPendingRemoteRecovery(first.SessionID, "execution-other"))

	current := cloneRemoteRecoveryRecord(first)
	current.AgentExecutionID = "execution-current"
	currentEntry := manager.queueRemoteRecovery(current, nil)
	require.NotNil(t, currentEntry)
	require.NotSame(t, firstEntry, currentEntry)
	require.False(t, manager.HasPendingRemoteRecovery(first.SessionID, first.AgentExecutionID),
		"a replaced execution must not be reported as the current pending owner")
	require.True(t, manager.HasPendingRemoteRecovery(current.SessionID, current.AgentExecutionID))
	require.False(t, manager.WasSessionRetracked(current.SessionID),
		"a current pending owner remains distinct from successful re-tracking")

	manager.recoveryGuard.Release(current.SessionID)
	require.False(t, manager.HasPendingRemoteRecovery(current.SessionID, current.AgentExecutionID),
		"an unguarded stale entry is not pending recovery")

	manager.recoveryGuard.RetainAsRemotePending(current.SessionID)
	manager.recoveryGuard.RetainAsUnstoppable(current.SessionID)
	require.False(t, manager.HasPendingRemoteRecovery(current.SessionID, current.AgentExecutionID),
		"an unstoppable guard is not a retryable pending remote owner")
	require.True(t, manager.completeRemoteRecovery(currentEntry))
	require.False(t, manager.HasPendingRemoteRecovery(current.SessionID, current.AgentExecutionID),
		"completed recovery is no longer pending")
}
