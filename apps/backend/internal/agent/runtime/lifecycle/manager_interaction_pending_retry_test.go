package lifecycle

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/executor"
	lifecycleevents "github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

type pendingStopStatusWriter struct {
	captureExecutorRunningWriter
	calls        atomic.Int32
	persisted    atomic.Bool
	allow        chan struct{}
	retryEntered chan struct{}
}

func (w *pendingStopStatusWriter) UpdateExecutorRunningStatusIfCurrent(_ context.Context, sessionID, executionID, status string) error {
	if sessionID != "pending-stop-session" || executionID != "pending-stop-execution" || status != models.ExecutorRunningStatusStopped {
		return errors.New("terminal persistence used an unexpected execution generation or status")
	}
	if w.calls.Add(1) == 1 {
		return errors.New("transient terminal persistence failure")
	}
	close(w.retryEntered)
	<-w.allow
	w.persisted.Store(true)
	return nil
}

func pendingStopFixture(t *testing.T, backend ExecutorBackend) (*Manager, *MockEventBus, *AgentExecution, *remoteRecoveryRetry) {
	t.Helper()
	mgr, events := task08ManagerWithBackend(t, backend)
	row := &models.ExecutorRunning{
		SessionID: "pending-stop-session", TaskID: "pending-stop-task",
		AgentExecutionID: "pending-stop-execution", Runtime: executor.NameSprites,
	}
	entry := mgr.queueRemoteRecovery(row, task08RemoteSnapshot(row))
	exec := &AgentExecution{
		ID: row.AgentExecutionID, SessionID: row.SessionID,
		TaskID: row.TaskID, RuntimeName: executor.NameSprites,
	}
	require.NoError(t, mgr.executionStore.Add(exec))
	return mgr, events, exec, entry
}

func TestStopPendingRecoveryRetriesPersistenceBeforeCleanupAndPublication(t *testing.T) {
	mgr, events, exec, entry := pendingStopFixture(t, &MockExecutor{name: executor.NameSprites})
	w := &pendingStopStatusWriter{allow: make(chan struct{}), retryEntered: make(chan struct{})}
	mgr.SetExecutorRunningWriter(w)
	var once sync.Once
	release := func() { once.Do(func() { close(w.allow) }) }
	t.Cleanup(func() {
		release()
		require.NoError(t, mgr.Stop())
	})
	published := make(chan error, 1)
	events.OnPublish = func(subject string, _ *bus.Event) {
		if subject != lifecycleevents.AgentStopped {
			return
		}
		if !w.persisted.Load() {
			published <- errors.New("publication preceded terminal persistence")
			return
		}
		if _, tracked := mgr.executionStore.Get(exec.ID); tracked {
			published <- errors.New("publication preceded execution removal")
			return
		}
		if mgr.pendingRemoteRecoveryForSession(exec.SessionID) != nil {
			published <- errors.New("publication preceded recovery completion")
			return
		}
		published <- mgr.recoveryGuard.CheckLaunchAllowed(exec.SessionID)
	}
	require.Error(t, mgr.StopAgentWithReason(context.Background(), exec.ID, StopReasonTaskDeleted, true))
	select {
	case <-w.retryEntered:
	case <-time.After(2 * time.Second):
		t.Fatal("retry never entered")
	}
	_, tracked := mgr.executionStore.Get(exec.ID)
	require.True(t, tracked)
	require.Same(t, entry, mgr.pendingRemoteRecoveryForSession(exec.SessionID))
	require.ErrorIs(t, mgr.recoveryGuard.CheckLaunchAllowed(exec.SessionID), ErrSessionRecoveryGuarded)
	select {
	case <-published:
		t.Fatal("stop published while terminal persistence was blocked")
	default:
	}
	release()
	select {
	case err := <-published:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("no stop publication")
	}
	require.NoError(t, mgr.Stop())
	require.EqualValues(t, 2, w.calls.Load())
}

func TestStopPendingRecoveryFailedRuntimeStopRetainsOwner(t *testing.T) {
	backend := &retryableStopBackend{MockExecutor: MockExecutor{name: executor.NameSprites}, stopErr: errors.New("runtime stop failure")}
	mgr, eventBus, exec, entry := pendingStopFixture(t, backend)
	require.ErrorIs(t, mgr.StopAgentWithReason(context.Background(), exec.ID, StopReasonTaskDeleted, true), backend.stopErr)
	require.Empty(t, eventBus.PublishedEvents)
	_, tracked := mgr.executionStore.Get(exec.ID)
	require.True(t, tracked)
	require.Same(t, entry, mgr.pendingRemoteRecoveryForSession(exec.SessionID))
	require.ErrorIs(t, mgr.recoveryGuard.CheckLaunchAllowed(exec.SessionID), ErrSessionRecoveryGuarded)
}

func TestStopPendingRecoveryStaleCompletionPreservesSuccessor(t *testing.T) {
	mgr, eventBus, exec, entry := pendingStopFixture(t, &MockExecutor{name: executor.NameSprites})
	successorRow := cloneRemoteRecoveryRecord(entry.record)
	successorRow.AgentExecutionID = "successor-execution"
	successor := mgr.queueRemoteRecovery(successorRow, task08RemoteSnapshot(successorRow))
	mgr.finishStoppedExecution(context.Background(), exec, entry)
	_, tracked := mgr.executionStore.Get(exec.ID)
	require.False(t, tracked)
	require.Len(t, eventBus.PublishedEvents, 1)
	require.Equal(t, lifecycleevents.AgentStopped, eventBus.PublishedEvents[0].Type)
	require.Same(t, successor, mgr.pendingRemoteRecoveryForSession(exec.SessionID))
	require.ErrorIs(t, mgr.recoveryGuard.CheckLaunchAllowed(exec.SessionID), ErrSessionRecoveryGuarded)
}

func TestStopPendingRecoveryForeignOwnerFailsClosed(t *testing.T) {
	mgr, eventBus, exec, entry := pendingStopFixture(t, &MockExecutor{name: executor.NameSprites})
	mgr.removeRemoteRecovery(entry, true)
	foreignRow := cloneRemoteRecoveryRecord(entry.record)
	foreignRow.SessionID = "foreign-session"
	foreign := mgr.queueRemoteRecovery(foreignRow, task08RemoteSnapshot(foreignRow))
	require.ErrorContains(t, mgr.StopAgentWithReason(context.Background(), exec.ID, StopReasonTaskDeleted, true), "no longer owns")
	require.Empty(t, eventBus.PublishedEvents)
	_, tracked := mgr.executionStore.Get(exec.ID)
	require.True(t, tracked)
	require.Same(t, foreign, mgr.pendingRemoteRecoveryForSession(foreignRow.SessionID))
	require.ErrorIs(t, mgr.recoveryGuard.CheckLaunchAllowed(foreignRow.SessionID), ErrSessionRecoveryGuarded)
}

func TestStopPendingRecoveryReplacedExecutionPreservesOwner(t *testing.T) {
	mgr, eventBus, exec, entry := pendingStopFixture(t, &MockExecutor{name: executor.NameSprites})
	mgr.executionStore.Remove(exec.ID)
	replacement := &AgentExecution{ID: exec.ID, SessionID: exec.SessionID, TaskID: exec.TaskID, RuntimeName: executor.NameSprites}
	require.NoError(t, mgr.executionStore.Add(replacement))
	mgr.finishStoppedExecution(context.Background(), exec, entry)
	require.Empty(t, eventBus.PublishedEvents)
	actual, tracked := mgr.executionStore.Get(exec.ID)
	require.True(t, tracked)
	require.Same(t, replacement, actual)
	require.Same(t, entry, mgr.pendingRemoteRecoveryForSession(exec.SessionID))
	require.ErrorIs(t, mgr.recoveryGuard.CheckLaunchAllowed(exec.SessionID), ErrSessionRecoveryGuarded)
}
