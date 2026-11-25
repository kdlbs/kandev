package lifecycle

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestValidateRemoteRecoveryAllowsExactTokenlessFailedDockerAbsenceCandidate(t *testing.T) {
	row := &models.ExecutorRunning{
		ID: "session-tokenless-docker", SessionID: "session-tokenless-docker", TaskID: "task-tokenless-docker",
		AgentExecutionID: "execution-tokenless-docker", Runtime: agentruntime.RuntimeRemoteDocker,
		ContainerID: "recorded-container", Resumable: false,
	}
	snapshot := task08RemoteSnapshot(row)
	snapshot.SessionState = models.TaskSessionStateFailed
	require.NoError(t, validateRemoteRecoverySnapshot(snapshot, row),
		"exact tokenless Docker ownership must reach executor evidence checks")

	withoutContainer := cloneRemoteRecoveryRecord(row)
	withoutContainer.ContainerID = ""
	require.NoError(t, validateRemoteRecoverySnapshot(snapshot, withoutContainer),
		"the failed session must reach executor evidence checks")
	_, _, requestOK := remoteDockerRecoveryRequest(withoutContainer)
	require.False(t, requestOK, "absence cannot be attributed without the exact recorded container")

	withResume := cloneRemoteRecoveryRecord(row)
	withResume.Resumable = true
	withResume.ResumeToken = "resume-token"
	require.NoError(t, validateRemoteRecoverySnapshot(snapshot, withResume))
	require.Error(t, validateFailedRemoteRecoveryAgentProof(snapshot, nil),
		"resumable failed sessions still require authenticated stopped-agent evidence")
}

func TestRemoteRecoveryFailedTokenlessDockerCannotSettleAbsenceWithoutRecordedContainer(t *testing.T) {
	ctx := context.Background()
	repo, _ := runOwnerTestRepository(t)
	row := seedRemoteAbsenceRecord(t, repo, agentruntime.RuntimeRemoteDocker, "failed-tokenless-no-container")
	row.Resumable = false
	row.ResumeToken = ""
	row.ContainerID = ""
	require.NoError(t, repo.UpsertExecutorRunning(ctx, row))
	require.NoError(t, repo.UpdateTaskSessionState(ctx, row.SessionID, models.TaskSessionStateFailed, "agent process exited"))
	snapshot := task08RemoteSnapshot(row)
	snapshot.SessionState = models.TaskSessionStateFailed
	manager, _ := task08ManagerWithBackend(t, &MockExecutor{name: executor.NameRemoteDocker})
	manager.SetExecutorRunningWriter(repo)
	entry := manager.queueRemoteRecovery(row, snapshot)
	require.True(t, manager.beginRemoteRecovery(entry))
	t.Cleanup(func() { require.NoError(t, manager.Stop()) })

	settled, err := manager.settleRemoteRecoveryAbsence(ctx, entry, row, true, RecoveryOutcomeRecordedResourceAbsent)
	require.NoError(t, err)
	require.False(t, settled)
	require.ErrorIs(t, manager.recoveryGuard.CheckLaunchAllowed(row.SessionID), ErrSessionRecoveryGuarded)
	manager.finishRemoteRecoveryAttempt(entry)
}

func TestRemoteRecoveryAbsenceDoesNotSettleFailedSessionWithoutDeliveryProof(t *testing.T) {
	ctx := context.Background()
	row := &models.ExecutorRunning{
		ID: "session-failed-no-delivery", SessionID: "session-failed-no-delivery",
		TaskID: "task-failed-no-delivery", AgentExecutionID: "execution-failed-no-delivery",
		Runtime: agentruntime.RuntimePluginRemote, Status: models.ExecutorRunningStatusRunning,
	}
	snapshot := &RemoteRecoverySnapshot{
		SessionID: row.SessionID, TaskID: row.TaskID, AgentExecutionID: row.AgentExecutionID,
		Runtime: string(row.Runtime), SessionState: models.TaskSessionStateFailed,
	}
	manager, _ := task08ManagerWithBackend(t, &MockExecutor{name: executor.NamePluginRemote})
	t.Cleanup(func() { require.NoError(t, manager.Stop()) })
	entry := manager.queueRemoteRecovery(row, snapshot)
	require.NotNil(t, entry)

	settled, err := manager.settleRemoteRecoveryAbsence(ctx, entry, row, true, RecoveryOutcomeNoMatchingInstance)
	require.NoError(t, err)
	require.False(t, settled, "absence cannot replace authenticated stopped-process proof")
	require.ErrorIs(t, manager.recoveryGuard.CheckLaunchAllowed(row.SessionID), ErrSessionRecoveryGuarded)
}

func TestRemoteRecoveryFailedTokenlessDockerSettlesDefinitiveContainerAbsence(t *testing.T) {
	ctx := context.Background()
	repo, _ := runOwnerTestRepository(t)
	row := seedRemoteAbsenceRecord(t, repo, agentruntime.RuntimeRemoteDocker, "failed-tokenless-exited")
	row.Resumable = false
	row.ResumeToken = ""
	row.ContainerID = "recorded-exited-container"
	require.NoError(t, repo.UpsertExecutorRunning(ctx, row))
	require.NoError(t, repo.UpdateTaskSessionState(ctx, row.SessionID, models.TaskSessionStateFailed, "agent process exited"))
	snapshot := task08RemoteSnapshot(row)
	snapshot.SessionState = models.TaskSessionStateFailed
	backend := &task08DetailedRecoveryBackend{
		MockExecutor: &MockExecutor{name: executor.NameRemoteDocker},
		recoveryResult: func([]*models.ExecutorRunning) ([]*ExecutorInstance, map[string]RecoveryCandidateOutcome, error) {
			return nil, map[string]RecoveryCandidateOutcome{row.SessionID: RecoveryOutcomeRecordedResourceAbsent}, nil
		},
	}
	manager, _ := task08ManagerWithBackend(t, backend)
	manager.SetExecutorRunningWriter(repo)
	manager.workspaceInfoProvider = &task08RemoteSnapshotProvider{snapshots: map[string]*RemoteRecoverySnapshot{row.SessionID: snapshot}}
	manager.SetPassthroughLookup(alwaysNotPassthrough)
	manager.SetRecoveryDeadline(time.Second)
	t.Cleanup(func() { require.NoError(t, manager.Stop()) })

	require.NoError(t, manager.Start(ctx))
	require.NoError(t, manager.recoveryGuard.CheckLaunchAllowed(row.SessionID),
		"the exact recorded Docker container is definitively absent after its outage")
	current, err := repo.GetExecutorRunningBySessionID(ctx, row.SessionID)
	require.NoError(t, err)
	require.Equal(t, models.ExecutorRunningStatusStopped, current.Status)
	require.Empty(t, current.ResumeToken)
	session, err := repo.GetTaskSession(ctx, row.SessionID)
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateFailed, session.State,
		"absence settles runtime ownership but does not fabricate a session outcome")
}

func TestRemoteRecoveryFailedTokenlessDockerKeepsGuardForMissingInstanceInLiveContainer(t *testing.T) {
	ctx := context.Background()
	repo, _ := runOwnerTestRepository(t)
	row := seedRemoteAbsenceRecord(t, repo, agentruntime.RuntimeRemoteDocker, "failed-tokenless-missing-instance")
	row.Resumable = false
	row.ResumeToken = ""
	require.NoError(t, repo.UpsertExecutorRunning(ctx, row))
	require.NoError(t, repo.UpdateTaskSessionState(ctx, row.SessionID, models.TaskSessionStateFailed, "agent process exited"))
	snapshot := task08RemoteSnapshot(row)
	snapshot.SessionState = models.TaskSessionStateFailed
	manager, _ := task08ManagerWithBackend(t, &MockExecutor{name: executor.NameRemoteDocker})
	manager.SetExecutorRunningWriter(repo)
	entry := manager.queueRemoteRecovery(row, snapshot)
	require.True(t, manager.beginRemoteRecovery(entry))
	t.Cleanup(func() { require.NoError(t, manager.Stop()) })

	settled, err := manager.settleRemoteRecoveryAbsence(ctx, entry, row, true, RecoveryOutcomeNoMatchingInstance)
	require.NoError(t, err)
	require.False(t, settled, "a missing agentctl instance does not prove that its live container exited")
	require.ErrorIs(t, manager.recoveryGuard.CheckLaunchAllowed(row.SessionID), ErrSessionRecoveryGuarded)
	current, err := repo.GetExecutorRunningBySessionID(ctx, row.SessionID)
	require.NoError(t, err)
	require.Equal(t, models.ExecutorRunningStatusRunning, current.Status)
	manager.finishRemoteRecoveryAttempt(entry)
}
