package lifecycle

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

type workspaceFailureWriter struct {
	captureExecutorRunningWriter
	episode *models.ExecutorFailureEpisode
	err     error
	calls   int
	delayed bool
}

func (w *workspaceFailureWriter) GetActiveExecutorFailure(_ context.Context, _ models.ExecutorObservationTarget) (*models.ExecutorFailureEpisode, error) {
	w.calls++
	if w.delayed && w.calls == 1 {
		return nil, nil
	}
	return w.episode, w.err
}

// @covers AC-EXECUTORS-FAILURE-VISIBILITY-001.6
func TestExecutorFailureBlocksPassiveWorkspaceCreation(t *testing.T) {
	for _, entry := range []string{"workspace", "execution", "cached"} {
		for _, unavailable := range []string{"active", "read-error", "delayed", "clear"} {
			t.Run(entry+"/"+unavailable, func(t *testing.T) {
				manager, backend := newEnvironmentExecutionTestManager(t, &mockWorkspaceInfoProvider{infos: map[string]*WorkspaceInfo{
					"session-1": {TaskID: "task-1", SessionID: "session-1", WorkspacePath: "/workspace/task", AgentID: "auggie"},
				}})
				manager.SetExecutorProfileReader(&fakeExecutorProfileReader{session: &models.TaskSession{ID: "session-1", TaskID: "task-1", State: models.TaskSessionStateFailed}})
				writer := &workspaceFailureWriter{episode: &models.ExecutorFailureEpisode{ID: "failure-1", TaskID: "task-1", SessionID: "session-1", State: "active"}}
				if unavailable == "read-error" {
					writer.episode = nil
					writer.err = errors.New("inventory unavailable")
				}
				writer.delayed = unavailable == "delayed"
				if unavailable == "clear" {
					writer.episode = nil
				}
				manager.SetExecutorRunningWriter(writer)
				if entry == "cached" {
					require.NoError(t, manager.executionStore.Add(&AgentExecution{ID: "execution-1", TaskID: "task-1", SessionID: "session-1"}))
				}
				var err error
				if entry == "workspace" {
					_, err = manager.EnsureWorkspaceExecutionForSession(t.Context(), "task-1", "session-1")
				} else {
					_, err = manager.GetOrEnsureExecution(t.Context(), "session-1")
				}
				if unavailable == "clear" {
					require.NoError(t, err)
					if entry == "cached" {
						require.Zero(t, backend.createCount.Load())
					} else {
						require.EqualValues(t, 1, backend.createCount.Load())
					}
					return
				}
				require.ErrorIs(t, err, ErrSessionWorkspaceNotReady)
				require.Zero(t, backend.createCount.Load())
				require.Nil(t, writer.running)
				require.Zero(t, writer.deleteCalls)
			})
		}
	}
}
