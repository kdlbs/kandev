package orchestrator

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestInterruptionContinuationFifthRestoreDispatches(t *testing.T) {
	svc, _, data := continuationFailureFixture(t)
	mgr := installContinuationRestoreFixture(t, svc)
	require.True(t, svc.handleTransientFailure(context.Background(), data))
	value, _ := svc.transientRetries.Load("s1")
	entry := value.(*transientRetryEntry)
	entry.attempt, entry.started = transientMaxAttempts, transientMaxAttempts-1
	require.True(t, entry.claim())
	svc.retryTransientPrompt(entry.retryCtx, "t1", "s1", "execution-1")
	require.Equal(t, transientMaxAttempts, entry.started)
	require.Len(t, mgr.capturedPrompts, 1, "the final counted restoration must still dispatch its continuation")
}

func TestCapacityDelayedRetryChangedErrorSettlesEpisode(t *testing.T) {
	for _, late := range []bool{false, true} {
		name := "within window"
		if late {
			name = "after window"
		}
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				svc, mgr, messages, data, entry := startCapacityContinuationEpisode(t)
				require.True(t, entry.claim())
				svc.retryTransientPrompt(entry.retryCtx, "t1", "s1", "execution-1")
				if late {
					time.Sleep(31 * time.Minute)
				}
				data.PromptGeneration = mgr.currentPromptGeneration.Load()
				data.ErrorMessage, data.ProviderError.Message = overloaded529, overloaded529
				svc.handleAgentTurnFailed(context.Background(), data)
				_, owned := svc.transientRetries.Load("s1")
				require.False(t, owned)
				session, err := svc.repo.GetTaskSession(context.Background(), "s1")
				require.NoError(t, err)
				require.Equal(t, models.TaskSessionStateWaitingForInput, session.State)
				last := messages.sessionMessages[len(messages.sessionMessages)-1].metadata
				require.Equal(t, recoveryDispositionRefused, last["recovery_disposition"])
				require.Equal(t, 1, last["attempts_started"])
				require.Len(t, mgr.capturedPromptCalls, 1)
			})
		})
	}
}

func TestCapacityDelayedRetryRejectedReplayDoesNotCount(t *testing.T) {
	svc, _ := newTransientTestService(t)
	t.Cleanup(svc.cancelAllTransientRetries)
	installInterruptionTurnService(t, svc)
	mgr := configureRetainedReplayRuntime(t, svc)
	armTransientPromptEvidence(svc)
	svc.rememberTurnPrompt("s1", "retry the request", "", false, nil)
	svc.handleAgentTurnFailed(context.Background(), retainedTurnFailureDataForReplay())
	value, _ := svc.transientRetries.Load("s1")
	entry := value.(*transientRetryEntry)
	mgr.promptErr = errors.New("agentctl rejected prompt before provider dispatch")
	require.True(t, entry.claim())
	svc.retryTransientPrompt(entry.retryCtx, "t1", "s1", "execution-1")
	require.Zero(t, entry.started)
	_, owned := svc.transientRetries.Load("s1")
	require.False(t, owned)
}

func TestCapacityDelayedRetryRestartNoticeUpdateFailure(t *testing.T) {
	svc, messages, session := liveCapacityRestartFixture(t)
	svc.transientRetryMessages = &capacityNoticeUpdateFailure{TransientRetryMessageService: messages}
	svc.retireInterruptedNoticeOnStartup(context.Background(), session)
	rows, err := messages.ListMessages(context.Background(), "s1")
	require.NoError(t, err)
	require.Empty(t, transientRetryNotices(rows, "t1", "s1"), "failed conversion must still attempt stale-action cleanup")
}

type capacityNoticeUpdateFailure struct {
	TransientRetryMessageService
}

func (*capacityNoticeUpdateFailure) UpdateMessage(context.Context, *models.Message) error {
	return errors.New("notice update unavailable")
}

func liveCapacityRestartFixture(t *testing.T) (*Service, TransientRetryMessageService, *models.TaskSession) {
	t.Helper()
	svc, messages, _ := newPersistentTransientRetryTestService(t)
	svc.createTransientRetryStatusMessage(context.Background(), watcher.AgentEventData{
		TaskID: "t1", SessionID: "s1", RecoveryMode: recoveryModeContinue, RecoveryAttemptsStarted: 6,
	}, &routingerr.Error{Code: routingerr.CodeModelCapacity}, 7, time.Minute, time.Now().Add(time.Minute))
	notices := retryingMessages(t, messages)
	require.Len(t, notices, 1)
	notices[0].Metadata["retry_window_ends_at"] = time.Now().Add(30 * time.Minute).Format(time.RFC3339Nano)
	notices[0].Metadata["max_attempts"] = capacityMaxAttempts
	require.NoError(t, messages.UpdateMessage(context.Background(), notices[0]))
	require.NoError(t, svc.repo.UpdateTaskSessionState(context.Background(), "s1", models.TaskSessionStateWaitingForInput, ""))
	session, err := svc.repo.GetTaskSession(context.Background(), "s1")
	require.NoError(t, err)
	svc.agentManager.(*mockAgentManager).isAgentRunning = true
	return svc, messages, session
}
