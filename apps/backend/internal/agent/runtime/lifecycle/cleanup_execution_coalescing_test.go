package lifecycle

import (
	"context"
	"errors"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestPromotionRejectsExecutionRemovedByCleanup(t *testing.T) {
	mgr := newTestManager(t)
	execution := &AgentExecution{ID: "stopped", SessionID: "session", TaskID: "task"}
	require.NoError(t, mgr.executionStore.Add(execution))
	require.NoError(t, mgr.CleanupStaleExecutionBySessionIDIfCurrent(
		t.Context(), execution.SessionID, execution.ID, time.Time{}))
	err := mgr.promoteWorkspaceExecution(t.Context(), execution, &LaunchRequest{
		TaskID: execution.TaskID, SessionID: execution.SessionID,
	})
	require.ErrorIs(t, err, models.ErrExecutionRotated)
	require.Empty(t, execution.AgentCommand)
}

func TestConditionalCleanupRunsAfterWorkspacePeer(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
	}{{name: "success"}, {name: "failure", err: errors.New("workspace read failed")}} {
		t.Run(tc.name, func(t *testing.T) {
			mgr := newTestManager(t)
			execution := &AgentExecution{ID: "stopped", SessionID: "session", TaskID: "task"}
			require.NoError(t, mgr.executionStore.Add(execution))
			entered, release := make(chan struct{}), make(chan struct{})
			peer := mgr.ensureExecutionGroup.DoChan(execution.SessionID, func() (interface{}, error) {
				close(entered)
				<-release
				return execution, tc.err
			})
			<-entered
			ctx := &coalescedRecoveryWaitContext{Context: t.Context(), onWait: func() { close(release) }}
			require.NoError(t, mgr.CleanupStaleExecutionBySessionIDIfCurrent(
				ctx, execution.SessionID, execution.ID, time.Time{}))
			<-peer
			_, retained := mgr.executionStore.Get(execution.ID)
			require.False(t, retained, "a completed workspace read must not skip the requested cleanup")
		})
	}
}

type blockingCleanupWriter struct {
	ExecutorRunningWriter
	entered chan struct{}
	release chan struct{}
}

func (w *blockingCleanupWriter) DeleteExecutorRunningBySessionID(context.Context, string) error {
	close(w.entered)
	<-w.release
	return nil
}

func TestWorkspaceLookupWaitsForCleanupThenUsesItsOwnResult(t *testing.T) {
	for _, lookup := range []string{"session", "task session"} {
		t.Run(lookup, func(t *testing.T) {
			mgr := newTestManager(t)
			execution := &AgentExecution{ID: "stopped", SessionID: "session", TaskID: "task"}
			require.NoError(t, mgr.executionStore.Add(execution))
			writer := &blockingCleanupWriter{entered: make(chan struct{}), release: make(chan struct{})}
			mgr.SetExecutorRunningWriter(writer)
			cleaned := make(chan error, 1)
			go func() {
				cleaned <- mgr.CleanupStaleExecutionBySessionIDIfCurrent(
					t.Context(), execution.SessionID, execution.ID, time.Time{})
			}()
			<-writer.entered
			_, retained := mgr.executionStore.Get(execution.ID)
			require.False(t, retained, "cleanup has removed the cached execution but still owns the session slot")
			ctx := &coalescedRecoveryWaitContext{Context: t.Context(), onWait: func() { close(writer.release) }}
			require.NotPanics(t, func() {
				var got *AgentExecution
				var err error
				if lookup == "session" {
					got, err = mgr.GetOrEnsureExecution(ctx, execution.SessionID)
				} else {
					got, err = mgr.EnsureWorkspaceExecutionForSession(ctx, execution.TaskID, execution.SessionID)
				}
				require.Nil(t, got)
				require.ErrorContains(t, err, "workspace info provider not configured",
					"the lookup must run its own admission after cleanup finishes")
			})
			require.NoError(t, <-cleaned)
		})
	}
}

// TestWorkspaceLookupDoesNotReturnExecutionOwnedByPendingCleanup
// verifies cached reads join the cleanup operation.
func TestWorkspaceLookupDoesNotReturnExecutionOwnedByPendingCleanup(t *testing.T) {
	for _, lookup := range []string{"session", "task session", "environment"} {
		t.Run(lookup, func(t *testing.T) {
			mgr := newTestManager(t)
			execution := &AgentExecution{ID: "stopping", SessionID: "session", TaskID: "task", TaskEnvironmentID: "environment"}
			require.NoError(t, mgr.executionStore.Add(execution))
			entered, release := make(chan struct{}), make(chan struct{})
			var once sync.Once
			releaseCleanup := func() { once.Do(func() { close(release) }) }
			defer releaseCleanup()
			done := make(chan error, 1)
			go func() {
				_, err := mgr.doSerializedExecution(t.Context(), execution.SessionID, func(context.Context) (interface{}, error) {
					close(entered)
					<-release
					mgr.executionStore.Remove(execution.ID)
					return nil, nil
				})
				done <- err
			}()
			<-entered
			ctx := &coalescedRecoveryWaitContext{Context: t.Context(), onWait: releaseCleanup}
			var got *AgentExecution
			var err error
			switch lookup {
			case "session":
				got, err = mgr.GetOrEnsureExecution(ctx, execution.SessionID)
			case "task session":
				got, err = mgr.EnsureWorkspaceExecutionForSession(ctx, execution.TaskID, execution.SessionID)
			case "environment":
				got, err = mgr.GetOrEnsureExecutionForEnvironment(ctx, execution.TaskEnvironmentID)
			}
			require.True(t, got == nil, "workspace access must wait for the current cleanup owner")
			require.ErrorContains(t, err, "workspace info provider not configured")
			require.NoError(t, <-done)
		})
	}
}

// TestWorkspaceLookupWaitsForRuntimeRetirement
// verifies lookup blocks until the registered runtime retires.
func TestWorkspaceLookupWaitsForRuntimeRetirement(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		mgr := newTestManager(t)
		execution := &AgentExecution{ID: "retiring", SessionID: "session", TaskID: "task"}
		require.NoError(t, mgr.executionStore.Add(execution))
		execution.remoteInstanceLifecycleMu.Lock()
		var once sync.Once
		retire := func() {
			once.Do(func() {
				mgr.executionStore.Remove(execution.ID)
				execution.remoteInstanceLifecycleMu.Unlock()
			})
		}
		defer retire()
		type result struct {
			execution *AgentExecution
			err       error
		}
		finished := make(chan result, 1)
		go func() {
			got, err := mgr.EnsureWorkspaceExecutionForSession(t.Context(), execution.TaskID, execution.SessionID)
			finished <- result{execution: got, err: err}
		}()
		synctest.Wait()
		select {
		case <-finished:
			t.Fatal("workspace lookup returned while runtime retirement still owns the execution")
		default:
		}
		retire()
		got := <-finished
		require.Nil(t, got.execution)
		require.ErrorContains(t, got.err, "workspace info provider not configured")
	})
}

// TestCachedWorkspaceLookupHonorsCancellationDuringRetirement
// verifies cancelled admission does not wait for retirement.
func TestCachedWorkspaceLookupHonorsCancellationDuringRetirement(t *testing.T) {
	mgr := newTestManager(t)
	execution := &AgentExecution{ID: "retiring", SessionID: "session", TaskID: "task"}
	require.NoError(t, mgr.executionStore.Add(execution))
	execution.remoteInstanceLifecycleMu.Lock()
	var once sync.Once
	release := func() { once.Do(execution.remoteInstanceLifecycleMu.Unlock) }
	defer release()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	finished := make(chan error, 1)
	go func() {
		_, err := mgr.cachedWorkspaceExecution(ctx, execution.SessionID, "")
		finished <- err
	}()
	select {
	case err := <-finished:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("cancelled workspace lookup remained blocked on retirement")
	}
}

// TestEnvironmentWorkspaceLookupWaitsForRetirementBeforeSharingResult verifies
// a joined session lookup cannot receive the environment operation's retiring cache entry.
func TestEnvironmentWorkspaceLookupWaitsForRetirementBeforeSharingResult(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		mgr := newTestManager(t)
		execution := &AgentExecution{ID: "retiring", SessionID: "session", TaskID: "task", TaskEnvironmentID: "environment"}
		provider := &siblingDuringEnvironmentLookupProvider{mockWorkspaceInfoProvider: &mockWorkspaceInfoProvider{
			envInfos: map[string]*WorkspaceInfo{"environment": {
				TaskID: execution.TaskID, SessionID: execution.SessionID, TaskEnvironmentID: execution.TaskEnvironmentID,
				WorkspacePath: "/workspace/task", AgentID: "auggie",
			}},
		}}
		mgr.workspaceInfoProvider = provider
		// Publish the execution after the environment fast path has missed it.
		provider.onEnvironment = func() {
			require.NoError(t, mgr.executionStore.Add(execution))
			execution.remoteInstanceLifecycleMu.Lock()
		}
		var once sync.Once
		retire := func() {
			once.Do(func() {
				mgr.executionStore.Remove(execution.ID)
				execution.remoteInstanceLifecycleMu.Unlock()
			})
		}
		defer retire()
		type result struct {
			execution *AgentExecution
			err       error
		}
		environmentFinished := make(chan result, 1)
		go func() {
			got, err := mgr.GetOrEnsureExecutionForEnvironment(t.Context(), execution.TaskEnvironmentID)
			environmentFinished <- result{execution: got, err: err}
		}()
		synctest.Wait()
		sessionFinished := make(chan result, 1)
		go func() {
			got, err := mgr.GetOrEnsureExecution(t.Context(), execution.SessionID)
			sessionFinished <- result{execution: got, err: err}
		}()
		synctest.Wait()
		select {
		case <-environmentFinished:
			t.Fatal("environment operation returned while its cached runtime was retiring")
		case <-sessionFinished:
			t.Fatal("joined session lookup returned while its cached runtime was retiring")
		default:
		}
		retire()
		for _, finished := range []chan result{environmentFinished, sessionFinished} {
			got := <-finished
			require.Nil(t, got.execution)
			require.Error(t, got.err, "without a runtime, lookup must fail creation instead of sharing the retired execution")
		}
	})
}

// TestWorkspaceRestoreDoesNotBorrowTerminalCacheResult verifies explicit restore
// performs replacement even when it joins an ordinary cached workspace read.
func TestWorkspaceRestoreDoesNotBorrowTerminalCacheResult(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		mgr := newTestManager(t)
		retained := &AgentExecution{ID: "retained", SessionID: "session", TaskID: "task", Status: v1.AgentStatusStopped}
		require.NoError(t, mgr.executionStore.Add(retained))
		retained.remoteInstanceLifecycleMu.Lock()
		var once sync.Once
		release := func() { once.Do(retained.remoteInstanceLifecycleMu.Unlock) }
		defer release()
		type result struct {
			execution *AgentExecution
			err       error
		}
		readFinished, restoreFinished := make(chan result, 1), make(chan result, 1)
		go func() {
			got, err := mgr.GetOrEnsureExecution(t.Context(), retained.SessionID)
			readFinished <- result{execution: got, err: err}
		}()
		synctest.Wait()
		go func() {
			got, err := mgr.EnsureWorkspaceExecutionForSession(t.Context(), retained.TaskID, retained.SessionID)
			restoreFinished <- result{execution: got, err: err}
		}()
		synctest.Wait()
		release()
		read := <-readFinished
		require.NoError(t, read.err)
		require.Same(t, retained, read.execution)
		restore := <-restoreFinished
		require.Nil(t, restore.execution, "explicit restore must not borrow a terminal cache result")
		require.ErrorContains(t, restore.err, "workspace info provider not configured")
	})
}

// TestEnvironmentCacheLookupWaitsForRetirement verifies an existing environment
// cache entry is rechecked after removal or rebinding while its runtime retires.
func TestEnvironmentCacheLookupWaitsForRetirement(t *testing.T) {
	for _, mutation := range []string{"remove", "rebind"} {
		t.Run(mutation, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				mgr := newTestManager(t)
				execution := &AgentExecution{ID: "retiring", SessionID: "session", TaskID: "task", TaskEnvironmentID: "environment"}
				require.NoError(t, mgr.executionStore.Add(execution))
				execution.remoteInstanceLifecycleMu.Lock()
				var once sync.Once
				release := func() { once.Do(execution.remoteInstanceLifecycleMu.Unlock) }
				defer release()
				type result struct {
					execution *AgentExecution
					err       error
				}
				finished := make(chan result, 1)
				go func() {
					got, err := mgr.GetOrEnsureExecutionForEnvironment(t.Context(), "environment")
					finished <- result{execution: got, err: err}
				}()
				synctest.Wait()
				select {
				case <-finished:
					t.Fatal("environment cache lookup returned while runtime retirement held its lock")
				default:
				}
				if mutation == "remove" {
					mgr.executionStore.Remove(execution.ID)
				} else {
					require.NoError(t, mgr.executionStore.WithLock(execution.ID, func(current *AgentExecution) {
						current.TaskEnvironmentID = "replacement-environment"
					}))
				}
				release()
				got := <-finished
				require.True(t, got.execution == nil, "lookup must not return a removed or rebound execution")
				require.ErrorContains(t, got.err, "workspace info provider not configured")
			})
		})
	}
}

// TestEnvironmentSharedLookupRejectsReboundExecution verifies environment
// resolution cannot publish a different environment's cached session execution.
func TestEnvironmentSharedLookupRejectsReboundExecution(t *testing.T) {
	mgr := newTestManager(t)
	provider := &siblingDuringEnvironmentLookupProvider{mockWorkspaceInfoProvider: &mockWorkspaceInfoProvider{
		envInfos: map[string]*WorkspaceInfo{"requested": {
			TaskID: "task", SessionID: "session", TaskEnvironmentID: "requested",
			WorkspacePath: "/workspace/task", AgentID: "auggie",
		}},
	}}
	mgr.workspaceInfoProvider = provider
	provider.onEnvironment = func() {
		require.NoError(t, mgr.executionStore.Add(&AgentExecution{
			ID: "rebound", SessionID: "session", TaskID: "task", TaskEnvironmentID: "other",
		}))
	}
	got, err := mgr.GetOrEnsureExecutionForEnvironment(t.Context(), "requested")
	require.True(t, got == nil, "environment resolution must not return another environment's cached execution")
	require.ErrorContains(t, err, "no runtime configured")
}
