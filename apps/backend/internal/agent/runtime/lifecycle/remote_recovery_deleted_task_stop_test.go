package lifecycle

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
)

type deletedTaskStopInventory struct {
	task08ExecutorInventoryWriter
	listErr error
}

func (w *deletedTaskStopInventory) ListExecutorsRunning(
	ctx context.Context,
) ([]*models.ExecutorRunning, error) {
	if w.listErr != nil {
		return nil, w.listErr
	}
	return w.task08ExecutorInventoryWriter.ListExecutorsRunning(ctx)
}

type deletedTaskSessionSnapshotProvider struct {
	task08RemoteSnapshotProvider
	err error
}

func (p *deletedTaskSessionSnapshotProvider) GetRemoteRecoverySnapshot(
	context.Context,
	string,
) (*RemoteRecoverySnapshot, error) {
	return nil, p.err
}

type pendingDeletedSSHStopFixture struct {
	manager *Manager
	store   *deletedTaskStopInventory
	server  *fakeSSHServer
	row     *models.ExecutorRunning
}

func newPendingDeletedSSHStopFixture(t *testing.T) pendingDeletedSSHStopFixture {
	t.Helper()
	server := newFakeSSHServer(t, newSSHScriptedHandler(t,
		sshScriptRule{match: "ps -p 4242 -o command=", result: sshOut("/opt/kandev/bin/agentctl --workdir /remote/task")},
		sshScriptRule{match: "cat -- '/remote/session/agentctl.pid'", result: sshOut("4242")},
		sshScriptRule{match: "kill 4242", result: sshOK},
	).handle)
	metadata := sshConnectionMetadata(t, server)
	metadata[MetadataKeySSHRemoteAgentctlPID] = "4242"
	metadata[MetadataKeySSHRemoteSessionDir] = "/remote/session"
	metadata[MetadataKeySSHRemoteTaskDir] = "/remote/task"
	row := &models.ExecutorRunning{
		ID: "session-delete-stop", SessionID: "session-delete-stop", TaskID: "task-delete-stop",
		ExecutorID: "executor-delete-stop", AgentExecutionID: "execution-delete-stop",
		Runtime: agentruntime.RuntimeSSH, Status: models.ExecutorRunningStatusReady, Metadata: metadata,
	}
	store := &deletedTaskStopInventory{task08ExecutorInventoryWriter: task08ExecutorInventoryWriter{
		invariantWriter: invariantWriter{prior: row},
		rows:            []*models.ExecutorRunning{row},
	}}
	log := newTestRegistryLogger()
	registry := NewExecutorRegistry(log)
	registry.Register(NewSSHExecutor(nil, nil, nil, log))
	manager := NewManager(newTestRegistry(), &MockEventBus{}, registry, nil, nil, nil, ExecutorFallbackWarn, "", log)
	cleanupManagerStopCh(t, manager)
	manager.SetExecutorRunningWriter(store)
	manager.workspaceInfoProvider = &deletedTaskSessionSnapshotProvider{
		task08RemoteSnapshotProvider: task08RemoteSnapshotProvider{},
		err:                          models.ErrTaskSessionNotFound,
	}
	entry := manager.queueRemoteRecovery(row, task08RemoteSnapshot(row))
	require.NotNil(t, entry)
	return pendingDeletedSSHStopFixture{manager: manager, store: store, server: server, row: row}
}

func TestPendingSSHStopAfterTaskDeletionUsesExactPersistedOwner(t *testing.T) {
	fixture := newPendingDeletedSSHStopFixture(t)

	err := fixture.manager.StopAgentWithReason(
		context.Background(), fixture.row.AgentExecutionID, StopReasonTaskDeleted, true,
	)

	require.NoError(t, err)
	_, killed := fixture.server.lastCommandContaining("kill 4242")
	require.True(t, killed, "task deletion must stop the exact persisted remote agentctl")
	require.True(t, fixture.store.deleted, "successful explicit stop must retire the executor row")
	require.Nil(t, fixture.store.prior)
	require.NoError(t, fixture.manager.recoveryGuard.CheckLaunchAllowed(fixture.row.SessionID))
}

func TestPendingSSHStopAfterTaskDeletionRejectsChangedPersistedOwner(t *testing.T) {
	tests := []struct {
		name   string
		change func(*models.ExecutorRunning)
	}{
		{name: "replacement execution", change: func(row *models.ExecutorRunning) { row.AgentExecutionID = "replacement" }},
		{name: "replacement task", change: func(row *models.ExecutorRunning) { row.TaskID = "replacement-task" }},
		{name: "replacement session", change: func(row *models.ExecutorRunning) { row.SessionID = "replacement-session" }},
		{name: "replacement executor", change: func(row *models.ExecutorRunning) { row.ExecutorID = "replacement-executor" }},
		{name: "replacement runtime", change: func(row *models.ExecutorRunning) { row.Runtime = agentruntime.RuntimeSprites }},
		{name: "changed remote metadata", change: func(row *models.ExecutorRunning) {
			row.Metadata[MetadataKeySSHRemoteSessionDir] = "/remote/replaced-session"
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newPendingDeletedSSHStopFixture(t)
			current := cloneRemoteRecoveryRecord(fixture.row)
			tt.change(current)
			fixture.store.rows = []*models.ExecutorRunning{current}

			err := fixture.manager.StopAgentWithReason(
				context.Background(), fixture.row.AgentExecutionID, StopReasonTaskDeleted, true,
			)

			require.Error(t, err)
			require.Empty(t, fixture.server.commands(), "mismatched persisted ownership must never reach remote stop")
			require.False(t, fixture.store.deleted)
			require.ErrorIs(t, fixture.manager.recoveryGuard.CheckLaunchAllowed(fixture.row.SessionID), ErrSessionRecoveryGuarded)
		})
	}
}

func TestPendingSSHStopAfterTaskDeletionRejectsMissingPersistedOwner(t *testing.T) {
	fixture := newPendingDeletedSSHStopFixture(t)
	fixture.store.rows = nil

	err := fixture.manager.StopAgentWithReason(
		context.Background(), fixture.row.AgentExecutionID, StopReasonTaskDeleted, true,
	)

	require.Error(t, err)
	require.Empty(t, fixture.server.commands(), "a missing persisted owner must never reach remote stop")
	require.False(t, fixture.store.deleted)
	require.ErrorIs(t, fixture.manager.recoveryGuard.CheckLaunchAllowed(fixture.row.SessionID), ErrSessionRecoveryGuarded)
}

func TestPendingSSHStopAfterTaskDeletionRejectsDuplicatePersistedOwner(t *testing.T) {
	fixture := newPendingDeletedSSHStopFixture(t)
	fixture.store.rows = []*models.ExecutorRunning{
		fixture.row,
		cloneRemoteRecoveryRecord(fixture.row),
	}

	err := fixture.manager.StopAgentWithReason(
		context.Background(), fixture.row.AgentExecutionID, StopReasonTaskDeleted, true,
	)

	require.ErrorContains(t, err, "multiple persisted runtime rows")
	require.Empty(t, fixture.server.commands(), "ambiguous ownership must never reach remote stop")
	require.False(t, fixture.store.deleted)
	require.ErrorIs(t, fixture.manager.recoveryGuard.CheckLaunchAllowed(fixture.row.SessionID), ErrSessionRecoveryGuarded)
}

func TestPendingSSHStopAfterTaskDeletionFailsClosedOnInventoryReadError(t *testing.T) {
	fixture := newPendingDeletedSSHStopFixture(t)
	fixture.store.listErr = errors.New("executor inventory unavailable")

	err := fixture.manager.StopAgentWithReason(
		context.Background(), fixture.row.AgentExecutionID, StopReasonTaskDeleted, true,
	)

	require.ErrorContains(t, err, "executor inventory unavailable")
	require.Empty(t, fixture.server.commands())
	require.False(t, fixture.store.deleted)
	require.ErrorIs(t, fixture.manager.recoveryGuard.CheckLaunchAllowed(fixture.row.SessionID), ErrSessionRecoveryGuarded)
}

func TestPendingSSHStopWithMissingSessionOnlyUsesDeletionOwnerFallback(t *testing.T) {
	for _, reason := range []string{"user stop", StopReasonBackendShutdown, StopReasonRecoverableAgentFailure} {
		t.Run(reason, func(t *testing.T) {
			fixture := newPendingDeletedSSHStopFixture(t)

			err := fixture.manager.StopAgentWithReason(context.Background(), fixture.row.AgentExecutionID, reason, true)

			if reason == "user stop" {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Empty(t, fixture.server.commands(), "only an explicit task deletion may use persisted-row ownership")
			require.False(t, fixture.store.deleted)
		})
	}
}

func TestPendingSSHStopAfterTaskDeletionDoesNotFallbackOnSnapshotReadError(t *testing.T) {
	fixture := newPendingDeletedSSHStopFixture(t)
	readErr := errors.New("snapshot store unavailable")
	fixture.manager.workspaceInfoProvider = &deletedTaskSessionSnapshotProvider{
		err: readErr,
	}

	err := fixture.manager.StopAgentWithReason(
		context.Background(), fixture.row.AgentExecutionID, StopReasonTaskDeleted, true,
	)

	require.ErrorIs(t, err, readErr)
	require.Empty(t, fixture.server.commands(), "an unrelated snapshot error must never reach remote stop")
	require.False(t, fixture.store.deleted)
	require.ErrorIs(t, fixture.manager.recoveryGuard.CheckLaunchAllowed(fixture.row.SessionID), ErrSessionRecoveryGuarded)
}
