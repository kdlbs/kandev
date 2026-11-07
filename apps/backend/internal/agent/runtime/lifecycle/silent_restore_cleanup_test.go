package lifecycle

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/executor"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

type restoreCheckpointCleaner interface {
	CleanupFailedRestoreIfCurrent(context.Context, string, string, time.Time, func(context.Context) error) error
}

func TestSilentRestoreCleanupCheckpointsBeforeRemovingInventory(t *testing.T) {
	for _, failure := range []string{"", "stop", "checkpoint", "replacement", "unknown runtime", "joined workspace operation"} {
		t.Run(failure, func(t *testing.T) {
			ctx := t.Context()
			repo, _ := runOwnerTestRepository(t)
			mgr := newTestManager(t)
			mgr.SetExecutorRunningWriter(repo)
			backend := &runOwnerRecoveryExecutor{MockExecutor: MockExecutor{name: executor.NameStandalone}}
			mgr.executorRegistry = NewExecutorRegistry(mgr.logger)
			mgr.executorRegistry.Register(backend)
			e := &AgentExecution{ID: "candidate", SessionID: "restore-session", TaskID: "restore-task", Status: v1.AgentStatusFailed, RuntimeName: executor.NameStandalone, standaloneInstanceID: "candidate"}
			require.NoError(t, mgr.executionStore.Add(e))
			require.NoError(t, mgr.persistExecutorRunningResult(ctx, e))
			row, err := repo.GetExecutorRunningBySessionID(ctx, e.SessionID)
			require.NoError(t, err)
			failed := errors.New("injected failure")
			if failure == "unknown runtime" {
				e.RuntimeName = ""
			}
			if failure == "stop" {
				backend.stopErr = failed
			}
			if failure == "replacement" {
				row.AgentExecutionID = "replacement"
				require.NoError(t, repo.UpsertExecutorRunning(ctx, row))
			}
			cleaner, ok := any(mgr).(restoreCheckpointCleaner)
			require.True(t, ok, "runtime must persist death proof before removing candidate inventory")
			checkpoints := 0
			var checkpointStoppedID, checkpointExecutionID string
			var checkpointReadErr error
			checkpoint := func(ctx context.Context) error {
				checkpoints++
				checkpointStoppedID = backend.stoppedID
				current, readErr := repo.GetExecutorRunningBySessionID(ctx, e.SessionID)
				checkpointReadErr = readErr
				if readErr != nil {
					return readErr
				}
				if current != nil {
					checkpointExecutionID = current.AgentExecutionID
				}
				if failure == "checkpoint" {
					return failed
				}
				return nil
			}
			runCleanup := func(ctx context.Context) error {
				return cleaner.CleanupFailedRestoreIfCurrent(ctx, e.SessionID, e.ID, row.UpdatedAt, checkpoint)
			}
			if failure == "joined workspace operation" {
				err = cleanupAfterWorkspacePeer(t, mgr, e.SessionID, runCleanup)
			} else {
				err = runCleanup(ctx)
			}
			if checkpoints > 0 {
				require.Equal(t, e.ID, checkpointStoppedID)
				require.NoError(t, checkpointReadErr)
				require.Equal(t, e.ID, checkpointExecutionID)
			}
			if failure == "" || failure == "joined workspace operation" {
				require.NoError(t, err)
				require.Equal(t, 1, checkpoints)
				_, tracked := mgr.executionStore.Get(e.ID)
				require.False(t, tracked)
			} else {
				require.Error(t, err)
				_, tracked := mgr.executionStore.Get(e.ID)
				require.True(t, tracked)
				_, readErr := repo.GetExecutorRunningBySessionID(ctx, e.SessionID)
				require.NoError(t, readErr)
				if failure != "checkpoint" {
					require.Zero(t, checkpoints)
				}
			}
		})
	}
}

// Observing Done proves that the caller submitted its singleflight operation
// and reached the result wait before the workspace peer is released.
type cleanupWaitContext struct {
	context.Context
	once    sync.Once
	waiting chan struct{}
}

func (c *cleanupWaitContext) Done() <-chan struct{} {
	c.once.Do(func() { close(c.waiting) })
	return c.Context.Done()
}
func cleanupAfterWorkspacePeer(t *testing.T, mgr *Manager, sessionID string, cleanup func(context.Context) error) error {
	t.Helper()
	entered, release, peerDone := make(chan struct{}), make(chan struct{}), make(chan struct{})
	go func() {
		defer close(peerDone)
		_, _ = mgr.doCoalescedExecution(t.Context(), sessionID, func(context.Context) (interface{}, error) { close(entered); <-release; return nil, nil })
	}()
	<-entered
	ctx := &cleanupWaitContext{Context: t.Context(), waiting: make(chan struct{})}
	result := make(chan error, 1)
	go func() { result <- cleanup(ctx) }()
	<-ctx.waiting
	close(release)
	<-peerDone
	return <-result
}
