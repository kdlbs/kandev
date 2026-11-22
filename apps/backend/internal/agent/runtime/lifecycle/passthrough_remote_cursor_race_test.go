package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/executor"
	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

type delayedSuccessfulCursorRepository struct {
	*tasksqlite.Repository
	entered chan struct{}
	release chan struct{}
	reads   atomic.Int32
}

func (w *delayedSuccessfulCursorRepository) GetExecutorRunningBySessionID(ctx context.Context, sessionID string) (*models.ExecutorRunning, error) {
	row, err := w.Repository.GetExecutorRunningBySessionID(ctx, sessionID)
	if w.reads.Add(1) == 1 {
		close(w.entered)
		<-w.release
	}
	return row, err
}

func TestSSHPassthroughCursorSaveCannotRestoreRetiredExecution(t *testing.T) {
	for _, kind := range []string{"started", "idle", "submitted"} {
		for _, prior := range []bool{false, true} {
			for _, replacement := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/prior=%t/replacement=%t", kind, prior, replacement), func(t *testing.T) {
					repo, _ := runOwnerTestRepository(t)
					mgr, _, execution := newPassthroughRunnerManager(t)
					sshPassthroughTestController(t, execution)
					mgr.executorRegistry.Register(&MockExecutor{name: executor.NameSSH})
					execution.Status = v1.AgentStatusReady
					if kind == "idle" {
						execution.Status = v1.AgentStatusRunning
					}
					target := agentctltypes.PassthroughTarget{ProcessID: "old-process", Generation: 1}
					execution.PassthroughProcessID = target.ProcessID
					execution.setMetadataValue(metadataKeySSHPassthroughState, remotePassthroughRecord{PassthroughTarget: target, InstanceID: execution.ID})
					if prior {
						require.NoError(t, repo.UpsertExecutorRunning(t.Context(), &models.ExecutorRunning{ID: execution.ID, AgentExecutionID: execution.ID, TaskID: execution.TaskID, SessionID: execution.SessionID, Status: models.ExecutorRunningStatusStopped}))
					}
					writer := &delayedSuccessfulCursorRepository{Repository: repo, entered: make(chan struct{}), release: make(chan struct{})}
					mgr.runningWriter = writer
					release := sync.OnceFunc(func() { close(writer.release) })
					t.Cleanup(release)
					saved := make(chan struct{})
					go func() {
						defer close(saved)
						mgr.handleRemotePassthroughEvent(execution, agentctltypes.PassthroughEvent{PassthroughTarget: target, InstanceID: execution.ID, SessionID: execution.SessionID, Sequence: 1, Kind: kind})
					}()
					select {
					case <-writer.entered:
					case <-time.After(lockOrderDeadlockGuard):
						t.Fatal("event did not reach cursor persistence")
					}
					var cleanupFinished atomic.Bool
					cleaned := make(chan error, 1)
					go func() {
						defer cleanupFinished.Store(true)
						err := mgr.cleanupStaleExecution(t.Context(), execution)
						if err == nil && replacement {
							err = mgr.executionStore.Add(&AgentExecution{ID: "replacement-execution", SessionID: execution.SessionID, Status: v1.AgentStatusReady})
							if err == nil {
								err = repo.UpsertExecutorRunning(t.Context(), &models.ExecutorRunning{ID: "replacement-execution", AgentExecutionID: "replacement-execution", TaskID: execution.TaskID, SessionID: execution.SessionID, Status: models.ExecutorRunningStatusRunning, ResumeToken: "replacement-conversation", Metadata: map[string]interface{}{"replacement": "preserved"}})
							}
						}
						cleaned <- err
					}()
					require.Eventually(t, func() bool { return cleanupFinished.Load() || goroutineParkedIn("cleanupStaleExecution") }, lockOrderDeadlockGuard, time.Millisecond)
					release()
					require.NoError(t, <-cleaned)
					select {
					case <-saved:
					case <-time.After(lockOrderDeadlockGuard):
						t.Fatal("cursor persistence and cleanup deadlocked")
					}
					row, err := repo.GetExecutorRunningBySessionID(t.Context(), execution.SessionID)
					if replacement {
						require.NoError(t, err)
						require.Equal(t, "replacement-execution", row.AgentExecutionID)
						require.Equal(t, "replacement-conversation", row.ResumeToken)
						require.Equal(t, map[string]interface{}{"replacement": "preserved"}, row.Metadata)
					} else if !errors.Is(err, models.ErrExecutorRunningNotFound) {
						require.NoError(t, err)
						require.Equal(t, models.ExecutorRunningStatusStopped, row.Status, "a cursor save cannot restore a retired runtime")
					}
				})
			}
		}
	}
}
