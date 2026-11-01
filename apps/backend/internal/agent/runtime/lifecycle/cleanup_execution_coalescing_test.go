package lifecycle

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
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
