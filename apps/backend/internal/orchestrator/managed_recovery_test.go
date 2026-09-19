package orchestrator

import (
	"context"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestManagedFailureParksSessionWithoutTerminalError(t *testing.T) {
	svc, mc := newTransientTestService(t)
	armTransientPromptEvidence(svc)
	called := false
	svc.managedFailure = func(_ context.Context, data watcher.AgentEventData) (int, time.Time, error) {
		called = true
		require.True(t, data.EvidenceKnown)
		return 1, time.Now().Add(time.Minute), nil
	}
	svc.handleAgentFailed(context.Background(), watcher.AgentEventData{TaskID: "t1", SessionID: "s1", AgentExecutionID: "execution-1", PromptGeneration: 7, ErrorMessage: "temporary provider failure"})
	require.True(t, called, "execution must delegate managed conversation recovery before failing")
	session, err := svc.repo.GetTaskSession(context.Background(), "s1")
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateWaitingForInput, session.State)
	require.Empty(t, session.ErrorMessage)
	require.Len(t, mc.sessionMessages, 1)
	require.Equal(t, true, mc.sessionMessages[0].metadata["retrying"])
	require.Equal(t, "warning", mc.sessionMessages[0].metadata["variant"])
}

func TestManagedRecoveryCancelStopsDurableRetry(t *testing.T) {
	svc, _ := newTransientTestService(t)
	called := false
	svc.managedRetryCancel = func(_ context.Context, task, session string) bool {
		called = true
		return task == "t1" && session == "s1"
	}
	require.True(t, svc.CancelTransientRetry(context.Background(), "t1", "s1"))
	require.True(t, called)
}

func TestStopSessionCancelsDurableRecovery(t *testing.T) {
	svc, _ := newTransientTestService(t)
	called := false
	svc.managedRetryCancel = func(_ context.Context, task, session string) bool {
		called = true
		return task == "t1" && session == "s1"
	}
	_ = svc.StopSession(context.Background(), "s1", "user stop", true)
	require.True(t, called, "stopping a parked session must cancel its durable retry")
}

func TestManagedRetryStartingRemovesPersistedNotice(t *testing.T) {
	svc, tasks, _ := newPersistentTransientRetryTestService(t)
	createPersistedTransientRetryNotice(t, svc)
	require.Len(t, retryingMessages(t, tasks), 1)
	svc.ResolveManagedRecovery(context.Background(), "s1")
	require.Empty(t, retryingMessages(t, tasks))
}
