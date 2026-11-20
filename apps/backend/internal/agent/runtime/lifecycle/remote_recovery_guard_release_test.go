package lifecycle

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

// guardedRemote queues a remote recovery guard for a session on a backend whose
// recovery probe returns the given instances and outcome.
func guardedRemote(
	t *testing.T,
	name executor.Name,
	runtime agentruntime.Runtime,
	instances func(*models.ExecutorRunning) []*ExecutorInstance,
	outcome RecoveryCandidateOutcome,
) (*Manager, *models.ExecutorRunning, *remoteRecoveryRetry) {
	t.Helper()
	row := &models.ExecutorRunning{
		ID: "session-guard", SessionID: "session-guard", TaskID: "task-guard",
		AgentExecutionID: "execution-guard", Runtime: runtime, Status: models.ExecutorRunningStatusReady,
	}
	backend := &task08DetailedRecoveryBackend{MockExecutor: &MockExecutor{name: name}}
	backend.recoveryResult = func(records []*models.ExecutorRunning) ([]*ExecutorInstance, map[string]RecoveryCandidateOutcome, error) {
		var found []*ExecutorInstance
		if instances != nil {
			found = instances(records[0])
		}
		return found, map[string]RecoveryCandidateOutcome{records[0].SessionID: outcome}, nil
	}
	manager, _ := task08ManagerWithBackend(t, backend)
	manager.SetExecutorRunningWriter(&task08ExecutorInventoryWriter{rows: []*models.ExecutorRunning{row}})
	task08SetSnapshots(manager, []*models.ExecutorRunning{row})
	entry := manager.queueRemoteRecovery(row, nil)
	require.NotNil(t, entry)
	require.ErrorIs(t, manager.recoveryGuard.CheckLaunchAllowed(row.SessionID), ErrSessionRecoveryGuarded)
	return manager, row, entry
}

func liveInstance(row *models.ExecutorRunning) []*ExecutorInstance {
	return []*ExecutorInstance{{
		InstanceID: row.AgentExecutionID, SessionID: row.SessionID, TaskID: row.TaskID, RuntimeName: row.Runtime,
	}}
}

func TestReleaseRemoteRecoveryGuardRefusedWhileOldOwnerAlive(t *testing.T) {
	manager, row, _ := guardedRemote(t, executor.NameRemoteDocker, agentruntime.RuntimeRemoteDocker,
		liveInstance, "")

	released, err := manager.ReleaseRemoteRecoveryGuard(context.Background(), row.SessionID)

	require.ErrorIs(t, err, ErrRemoteOwnerNotProvenGone)
	require.False(t, released)
	require.ErrorIs(t, manager.recoveryGuard.CheckLaunchAllowed(row.SessionID), ErrSessionRecoveryGuarded,
		"a resume must stay refused while the old agent is alive")
	require.True(t, manager.HasPendingRemoteRecovery(row.SessionID, row.AgentExecutionID))
}

func TestReleaseRemoteRecoveryGuardWhenOwnerProvenGone(t *testing.T) {
	manager, row, _ := guardedRemote(t, executor.NameRemoteDocker, agentruntime.RuntimeRemoteDocker,
		nil, RecoveryOutcomeNoMatchingInstance)

	released, err := manager.ReleaseRemoteRecoveryGuard(context.Background(), row.SessionID)

	require.NoError(t, err)
	require.True(t, released)
	require.NoError(t, manager.recoveryGuard.CheckLaunchAllowed(row.SessionID))
	require.False(t, manager.HasPendingRemoteRecovery(row.SessionID, row.AgentExecutionID))
}

func TestReleaseRemoteRecoveryGuardRefusedWhenAbsenceIsUnprovable(t *testing.T) {
	for _, outcome := range []RecoveryCandidateOutcome{RecoveryOutcomeUnknown, RecoveryOutcomeEnumerationFailed} {
		manager, row, _ := guardedRemote(t, executor.NameRemoteDocker, agentruntime.RuntimeRemoteDocker, nil, outcome)
		released, err := manager.ReleaseRemoteRecoveryGuard(context.Background(), row.SessionID)
		require.ErrorIs(t, err, ErrRemoteOwnerNotProvenGone, string(outcome))
		require.False(t, released)
		require.ErrorIs(t, manager.recoveryGuard.CheckLaunchAllowed(row.SessionID), ErrSessionRecoveryGuarded)
	}
	// SSH no-match does not prove the remote process is gone.
	manager, row, _ := guardedRemote(t, executor.NameSSH, agentruntime.RuntimeSSH, nil, RecoveryOutcomeNoMatchingInstance)
	released, err := manager.ReleaseRemoteRecoveryGuard(context.Background(), row.SessionID)
	require.ErrorIs(t, err, ErrRemoteOwnerNotProvenGone)
	require.False(t, released)
	require.ErrorIs(t, manager.recoveryGuard.CheckLaunchAllowed(row.SessionID), ErrSessionRecoveryGuarded)
}

func TestRetryReleasesGuardOnlyWithProofOfAbsence(t *testing.T) {
	alive, aliveRow, aliveEntry := guardedRemote(t, executor.NameRemoteDocker, agentruntime.RuntimeRemoteDocker,
		nil, RecoveryOutcomeUnknown)
	for i := 0; i < 3; i++ {
		alive.retryRemoteRecovery(context.Background(), aliveEntry)
	}
	require.ErrorIs(t, alive.recoveryGuard.CheckLaunchAllowed(aliveRow.SessionID), ErrSessionRecoveryGuarded,
		"repeated retries without proof never release the guard")

	gone, goneRow, goneEntry := guardedRemote(t, executor.NameRemoteDocker, agentruntime.RuntimeRemoteDocker,
		nil, RecoveryOutcomeNoMatchingInstance)
	gone.retryRemoteRecovery(context.Background(), goneEntry)
	require.NoError(t, gone.recoveryGuard.CheckLaunchAllowed(goneRow.SessionID))
}

func TestReleaseRemoteRecoveryGuardUnknownSession(t *testing.T) {
	manager, _, _ := guardedRemote(t, executor.NameRemoteDocker, agentruntime.RuntimeRemoteDocker, nil, "")
	released, err := manager.ReleaseRemoteRecoveryGuard(context.Background(), "unknown")
	require.NoError(t, err)
	require.False(t, released)
}
