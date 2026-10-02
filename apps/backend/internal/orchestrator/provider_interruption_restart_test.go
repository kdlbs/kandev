package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestInterruptionContinuationShutdownRetainsNoticeForReconciliation(t *testing.T) {
	svc, taskSvc, _ := newPersistentTransientRetryTestService(t)
	svc.scheduleTransientRetry("t1", "s1", "execution-1", 1, time.Hour)
	value, _ := svc.transientRetries.Load("s1")
	entry := value.(*transientRetryEntry)
	entry.mode = recoveryModeContinue
	svc.createTransientRetryStatusMessage(context.Background(), watcher.AgentEventData{
		TaskID: "t1", SessionID: "s1", RecoveryMode: recoveryModeContinue,
	}, nil, 1, time.Hour, time.Now().UTC().Add(time.Hour))
	svc.cancelAllTransientRetries()
	require.Error(t, entry.retryCtx.Err(), "shutdown stops unaccepted scheduling")
	require.Len(t, retryingMessages(t, taskSvc), 1, "startup needs the persisted unfinished episode")
	svc.reconcileExecutorSessionsOnStartup(context.Background())
	messages, err := taskSvc.ListMessages(context.Background(), "s1")
	require.NoError(t, err)
	require.Len(t, messages, 1)
	require.Equal(t, "restart_interrupted", messages[0].Metadata["recovery_disposition"])
	require.Equal(t, float64(0), messages[0].Metadata["attempts_started"])
	require.Empty(t, svc.agentManager.(*mockAgentManager).capturedPrompts)
}

func TestInterruptionContinuationRestartManualUnlessAdopted(t *testing.T) {
	for _, adopted := range []bool{false, true} {
		t.Run(map[bool]string{false: "abandoned", true: "adopted live"}[adopted], func(t *testing.T) {
			svc, taskSvc, _ := newPersistentTransientRetryTestService(t)
			svc.SetRetrackedSessionChecker(func(string) bool { return adopted })
			svc.createTransientRetryStatusMessage(context.Background(), watcher.AgentEventData{
				TaskID: "t1", SessionID: "s1", RecoveryMode: recoveryModeContinue, RecoveryAttemptsStarted: 2,
			}, nil, 3, time.Second, time.Now().UTC().Add(time.Second))
			svc.reconcileExecutorSessionsOnStartup(context.Background())
			messages, err := taskSvc.ListMessages(context.Background(), "s1")
			require.NoError(t, err)
			require.Empty(t, transientRetryNotices(messages, "t1", "s1"))
			mgr := svc.agentManager.(*mockAgentManager)
			require.Empty(t, mgr.capturedPrompts)
			require.Empty(t, mgr.stopAgentWithReasonArgs)
			session, err := svc.repo.GetTaskSession(context.Background(), "s1")
			require.NoError(t, err)
			if adopted {
				require.Equal(t, models.TaskSessionStateRunning, session.State)
				require.Empty(t, messages, "adopted live work must not acquire a manual error")
				return
			}
			require.Equal(t, models.TaskSessionStateWaitingForInput, session.State)
			require.Len(t, messages, 1)
			require.Equal(t, "restart_interrupted", messages[0].Metadata["recovery_disposition"])
			require.Equal(t, float64(2), messages[0].Metadata["attempts_started"])
		})
	}
}

func TestInterruptionContinuationRestartRetiresStaleNoticeWithoutDispatch(t *testing.T) {
	svc, taskSvc, _ := newPersistentTransientRetryTestService(t)
	createPersistedTransientRetryNotice(t, svc)
	svc.reconcileExecutorSessionsOnStartup(context.Background())
	messages, err := taskSvc.ListMessages(context.Background(), "s1")
	require.NoError(t, err)
	require.Empty(t, transientRetryNotices(messages, "t1", "s1"))
	mgr := svc.agentManager.(*mockAgentManager)
	require.Empty(t, mgr.capturedPrompts)
	require.Empty(t, mgr.stopAgentWithReasonArgs)
	session, err := svc.repo.GetTaskSession(context.Background(), "s1")
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateRunning, session.State, "notice cleanup preserves the adopted session projection")
}
