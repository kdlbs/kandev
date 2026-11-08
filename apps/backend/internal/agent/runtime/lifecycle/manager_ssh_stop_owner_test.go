package lifecycle

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
)

// sshStopOwnerInventory is a persisted runtime inventory whose owner can be
// swapped between the authorization and settlement boundaries.
type sshStopOwnerInventory struct {
	captureExecutorRunningWriter
	rows []*models.ExecutorRunning
	err  error
}

func (i *sshStopOwnerInventory) ListExecutorsRunning(context.Context) ([]*models.ExecutorRunning, error) {
	if i.err != nil {
		return nil, i.err
	}
	return i.rows, nil
}

func testSSHStopRow(t *testing.T, executionID string) *models.ExecutorRunning {
	t.Helper()
	owner := testSSHLaunchOwner()
	receipt := testSSHLaunchReceipt(t, owner, 4242)
	row := &models.ExecutorRunning{
		ID:               owner.SessionID,
		SessionID:        owner.SessionID,
		TaskID:           owner.TaskID,
		Runtime:          agentruntime.RuntimeSSH,
		Status:           models.ExecutorRunningStatusRunning,
		AgentExecutionID: executionID,
		Metadata:         testSSHLaunchMetadata(receipt),
	}
	row.Metadata[MetadataKeySSHAgentctlInstanceID] = executionID
	return row
}

// @covers AC-EXECUTORS-SSH-EXECUTOR-001.24
func TestSSHStopRevalidatesExactOwnerBeforeEffects(t *testing.T) {
	row := testSSHStopRow(t, "execution-1")

	t.Run("unchanged owner passes", func(t *testing.T) {
		manager := &Manager{
			runningWriter: &sshStopOwnerInventory{rows: []*models.ExecutorRunning{row}},
			logger:        newNopLogger(t),
		}
		require.NoError(t, manager.revalidatePersistedSSHStopOwner(context.Background(), row))
	})

	t.Run("a successor execution fences settlement", func(t *testing.T) {
		successor := testSSHStopRow(t, "execution-2")
		manager := &Manager{
			runningWriter: &sshStopOwnerInventory{rows: []*models.ExecutorRunning{successor}},
			logger:        newNopLogger(t),
		}
		require.ErrorIs(t,
			manager.revalidatePersistedSSHStopOwner(context.Background(), row),
			errRemoteRecoveryFenced)
	})

	t.Run("volatile metadata does not fence the same owner", func(t *testing.T) {
		rewritten := testSSHStopRow(t, "execution-1")
		rewritten.Metadata[idleActivityMetadataKey] = map[string]interface{}{
			"execution_id": "execution-1",
			"activity_at":  "2026-11-08T00:00:00Z",
		}
		rewritten.Metadata["prepare_result"] = "rewritten"
		rewritten.Metadata[MetadataKeySSHLaunchReceipt] = row.Metadata[MetadataKeySSHLaunchReceipt]
		manager := &Manager{
			runningWriter: &sshStopOwnerInventory{rows: []*models.ExecutorRunning{rewritten}},
			logger:        newNopLogger(t),
		}
		require.NoError(t, manager.revalidatePersistedSSHStopOwner(context.Background(), row))
	})

	t.Run("a changed controller instance fences settlement", func(t *testing.T) {
		changed := testSSHStopRow(t, "execution-1")
		changed.Metadata[MetadataKeySSHAgentctlInstanceID] = "controller-2"
		manager := &Manager{
			runningWriter: &sshStopOwnerInventory{rows: []*models.ExecutorRunning{changed}},
			logger:        newNopLogger(t),
		}
		require.ErrorIs(t,
			manager.revalidatePersistedSSHStopOwner(context.Background(), row),
			errRemoteRecoveryFenced)
	})

	t.Run("a changed launch receipt fences settlement", func(t *testing.T) {
		changed := testSSHStopRow(t, "execution-1")
		replacement := testSSHLaunchReceipt(t, testSSHLaunchOwner(), 4343)
		applySSHLaunchReceiptMetadata(changed.Metadata, &replacement)
		manager := &Manager{
			runningWriter: &sshStopOwnerInventory{rows: []*models.ExecutorRunning{changed}},
			logger:        newNopLogger(t),
		}
		require.ErrorIs(t,
			manager.revalidatePersistedSSHStopOwner(context.Background(), row),
			errRemoteRecoveryFenced)
	})

	t.Run("a vanished row fences settlement", func(t *testing.T) {
		manager := &Manager{
			runningWriter: &sshStopOwnerInventory{},
			logger:        newNopLogger(t),
		}
		require.ErrorIs(t,
			manager.revalidatePersistedSSHStopOwner(context.Background(), row),
			errRemoteRecoveryFenced)
	})

	t.Run("an unreadable inventory fails closed", func(t *testing.T) {
		manager := &Manager{
			runningWriter: &sshStopOwnerInventory{err: errors.New("database unavailable")},
			logger:        newNopLogger(t),
		}
		require.Error(t, manager.revalidatePersistedSSHStopOwner(context.Background(), row))
	})
}

// @covers AC-EXECUTORS-SSH-EXECUTOR-001.24
func TestSSHStopRecordChecksOwnerBeforeExternalEffects(t *testing.T) {
	row := testSSHStopRow(t, "execution-1")
	successor := testSSHStopRow(t, "execution-2")
	manager := &Manager{
		runningWriter: &sshStopOwnerInventory{rows: []*models.ExecutorRunning{successor}},
		logger:        newNopLogger(t),
	}
	handled, err := manager.stopPersistedSSHRecord(context.Background(), row, "task deleted", true, nil)
	require.True(t, handled)
	require.ErrorIs(t, err, errRemoteRecoveryFenced,
		"a stale record must be fenced before the stop reaches the executor registry")
}

type fixedRemoteRecoverySnapshotProvider struct {
	snapshot *RemoteRecoverySnapshot
	err      error
}

func (p fixedRemoteRecoverySnapshotProvider) GetRemoteRecoverySnapshot(
	context.Context, string,
) (*RemoteRecoverySnapshot, error) {
	return p.snapshot, p.err
}

func (p fixedRemoteRecoverySnapshotProvider) GetWorkspaceInfoForSession(
	context.Context, string, string,
) (*WorkspaceInfo, error) {
	return nil, nil
}

func (p fixedRemoteRecoverySnapshotProvider) GetWorkspaceInfoForEnvironment(
	context.Context, string,
) (*WorkspaceInfo, error) {
	return nil, nil
}

func testArchiveFencedEntry(row *models.ExecutorRunning) *remoteRecoveryRetry {
	return &remoteRecoveryRetry{
		record: row,
		snapshot: &RemoteRecoverySnapshot{
			TaskID: row.TaskID, SessionID: row.SessionID,
			AgentExecutionID: row.AgentExecutionID, Runtime: string(agentruntime.RuntimeSSH),
			SessionState: models.TaskSessionStateWaitingForInput,
			RouteState:   "direct", RouteGeneration: 7,
			TaskWorkspaceID: "workspace-task", TaskOrigin: "local",
			WorkspaceOwnerID: "owner-1", WorkspaceOrgID: "org-1",
			TaskEnvironmentID: "env-1", EnvironmentOwnerTaskID: row.TaskID,
			EnvironmentOwnershipGen: 3, EnvironmentExecutorType: "ssh",
			EnvironmentExecutorID: "executor-1",
			RecoverySourcePresent: true, RecoverableDeliveryPending: true,
			RecoveryExecutionID: row.AgentExecutionID, RecoverySubmissionID: "submission-1",
			RecoveryStreamID: "stream-1", RecoveryIncarnationID: "incarnation-1",
			RecoveryHarnessGeneration: 2, RecoveryRevision: 4,
		},
	}
}

func archivedSnapshot(initial *RemoteRecoverySnapshot) *RemoteRecoverySnapshot {
	archived := *initial
	archived.SessionState = models.TaskSessionStateCancelled
	archived.TaskArchived = true
	archived.TaskResourceCleanupRunning = true
	return &archived
}

func runArchiveStopFallback(
	t *testing.T,
	entry *remoteRecoveryRetry,
	current *RemoteRecoverySnapshot,
	row *models.ExecutorRunning,
) (bool, *int, error) {
	t.Helper()
	manager := &Manager{
		runningWriter:         &sshStopOwnerInventory{rows: []*models.ExecutorRunning{row}},
		workspaceInfoProvider: fixedRemoteRecoverySnapshotProvider{snapshot: current},
		logger:                newNopLogger(t),
	}
	stopped := 0
	handled, err := manager.stopPersistedRemoteOwnerForRuntime(
		context.Background(), entry, StopReasonCascadeArchive, true,
		nil,
		func(context.Context, string, string, bool, *stopWaitWatch) (bool, error) {
			t.Fatal("a fenced snapshot must not dispatch the execution-ID stop")
			return false, nil
		},
		func(_ context.Context, record *models.ExecutorRunning, _ string, _ bool, _ *stopWaitWatch) (bool, error) {
			stopped++
			require.Equal(t, row.AgentExecutionID, record.AgentExecutionID)
			return true, nil
		},
	)
	return handled, &stopped, err
}

// @covers AC-EXECUTORS-SSH-EXECUTOR-001.24
func TestSSHStopUsesExactPersistedOwnerAfterSnapshotFence(t *testing.T) {
	row := testSSHStopRow(t, "execution-1")
	entry := testArchiveFencedEntry(row)
	handled, stopped, err := runArchiveStopFallback(t, entry, archivedSnapshot(entry.snapshot), row)
	require.NoError(t, err)
	require.True(t, handled)
	require.Equal(t, 1, *stopped, "the exact persisted owner must remain stoppable after the archive delta")
}

// @covers AC-EXECUTORS-SSH-EXECUTOR-001.24
func TestSSHStopAllowsArchiveRetryWithPinnedCancelledSnapshot(t *testing.T) {
	row := testSSHStopRow(t, "execution-1")
	entry := testArchiveFencedEntry(row)
	entry.snapshot = archivedSnapshot(entry.snapshot)
	handled, stopped, err := runArchiveStopFallback(t, entry, entry.snapshot, row)
	require.NoError(t, err)
	require.True(t, handled)
	require.Equal(t, 1, *stopped, "a retry whose pinned snapshot is already archive-cancelled must remain stoppable")
}

// @covers AC-EXECUTORS-SSH-EXECUTOR-001.24
func TestSSHStopRejectsUnrelatedSnapshotChanges(t *testing.T) {
	cases := map[string]func(*RemoteRecoverySnapshot){
		"environment ownership generation": func(snapshot *RemoteRecoverySnapshot) {
			snapshot.EnvironmentOwnershipGen++
		},
		"workspace owner": func(snapshot *RemoteRecoverySnapshot) {
			snapshot.WorkspaceOwnerID = "owner-2"
		},
		"route generation": func(snapshot *RemoteRecoverySnapshot) {
			snapshot.RouteGeneration++
		},
		"recovery revision": func(snapshot *RemoteRecoverySnapshot) {
			snapshot.RecoveryRevision++
		},
		"session completed instead of archive-cancelled": func(snapshot *RemoteRecoverySnapshot) {
			snapshot.SessionState = models.TaskSessionStateCompleted
		},
		"no running cleanup claim": func(snapshot *RemoteRecoverySnapshot) {
			snapshot.TaskResourceCleanupRunning = false
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			row := testSSHStopRow(t, "execution-1")
			entry := testArchiveFencedEntry(row)
			current := archivedSnapshot(entry.snapshot)
			mutate(current)
			handled, stopped, err := runArchiveStopFallback(t, entry, current, row)
			require.False(t, handled)
			require.ErrorIs(t, err, errRemoteRecoveryFenced)
			require.Zero(t, *stopped, "an unrelated ownership change must not reach the stop")
		})
	}
}

// @covers AC-EXECUTORS-SSH-EXECUTOR-001.24
func TestSSHStopRejectsSuccessorAfterSnapshotFence(t *testing.T) {
	row := testSSHStopRow(t, "execution-1")
	entry := testArchiveFencedEntry(row)
	successor := testSSHStopRow(t, "execution-2")
	archived := archivedSnapshot(entry.snapshot)
	manager := &Manager{
		runningWriter:         &sshStopOwnerInventory{rows: []*models.ExecutorRunning{successor}},
		workspaceInfoProvider: fixedRemoteRecoverySnapshotProvider{snapshot: archived},
		logger:                newNopLogger(t),
	}
	handled, err := manager.stopPersistedRemoteOwnerForRuntime(
		context.Background(), entry, StopReasonTaskArchived, true,
		nil,
		func(context.Context, string, string, bool, *stopWaitWatch) (bool, error) {
			t.Fatal("a successor must never reach the stop path")
			return false, nil
		},
		func(context.Context, *models.ExecutorRunning, string, bool, *stopWaitWatch) (bool, error) {
			t.Fatal("a successor must never reach the persisted-record stop path")
			return false, nil
		},
	)
	require.False(t, handled)
	require.ErrorIs(t, err, errRemoteRecoveryFenced)
}

// @covers AC-EXECUTORS-SSH-EXECUTOR-001.23
func TestSSHArchiveStopFailureRetainsPendingEntry(t *testing.T) {
	row := testSSHStopRow(t, "execution-1")
	entry := testArchiveFencedEntry(row)
	manager := &Manager{
		runningWriter:         &sshStopOwnerInventory{rows: []*models.ExecutorRunning{row}},
		workspaceInfoProvider: fixedRemoteRecoverySnapshotProvider{snapshot: archivedSnapshot(entry.snapshot)},
		logger:                newNopLogger(t),
	}
	manager.remoteRecoveryPending = map[string]*remoteRecoveryRetry{row.SessionID: entry}
	_, err := manager.stopPersistedRemoteOwnerForRuntime(
		context.Background(), entry, StopReasonCascadeArchive, true,
		nil,
		func(context.Context, string, string, bool, *stopWaitWatch) (bool, error) {
			t.Fatal("the archive delta must use the persisted record path")
			return false, nil
		},
		func(context.Context, *models.ExecutorRunning, string, bool, *stopWaitWatch) (bool, error) {
			return true, errors.New("teardown failed")
		},
	)
	require.Error(t, err)
	require.Same(t, entry, manager.pendingRemoteRecoveryForSession(row.SessionID),
		"a failed teardown must keep the pending entry and its recovery guard retryable")
}
