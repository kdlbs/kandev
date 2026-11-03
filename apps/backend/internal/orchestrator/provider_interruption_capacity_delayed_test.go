package orchestrator

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestCapacityDelayedRetryPreservesInitiator(t *testing.T) {
	svc, mgr, data := capacityContinuationFailureFixture(t)
	t.Cleanup(svc.cancelAllTransientRetries)
	caller := authn.Identity{UserID: "collaborator", Role: authn.RoleMember, OrgID: "org-1"}
	mgr.currentPromptGeneration.Store(data.PromptGeneration - 1)
	svc.beginInteractivePromptAttempt(authn.WithIdentity(context.Background(), caller), "s1", "execution-1", false)
	mgr.currentPromptGeneration.Store(data.PromptGeneration)
	svc.observePromptAttempt("s1", "execution-1", data.PromptGeneration, true, true)
	svc.SetSessionPromptChecker(func(ctx context.Context, _ string) error {
		identity, known := authn.IdentityFromContext(ctx)
		require.True(t, known)
		require.Equal(t, caller, identity)
		return nil
	})
	require.NoError(t, svc.repo.UpsertExecutorRunning(context.Background(), &models.ExecutorRunning{
		ID: "runtime-original", TaskID: "t1", SessionID: "s1", AgentExecutionID: "execution-1", Status: "ready",
	}))
	require.True(t, svc.handleTransientFailure(context.Background(), data))
	value, _ := svc.transientRetries.Load("s1")
	entry := value.(*transientRetryEntry)
	require.True(t, entry.claim())
	svc.retryTransientPrompt(entry.retryCtx, "t1", "s1", "execution-1")
	data.PromptGeneration = mgr.currentPromptGeneration.Load()
	identity, known := svc.promptAttemptInitiator(data)
	require.True(t, known, "the automatic continuation must retain its initiating user")
	require.Equal(t, caller, identity)
	data.CapacityContinuation.PromptGeneration = data.PromptGeneration
	data.CapacityContinuation.CompletedTools = 0
	data.OutputObserved, data.EffectObserved = false, false
	require.True(t, svc.handleTransientFailure(context.Background(), data))
}

// @covers AC-PLATFORM-TURN-CONTINUITY-003.3
func TestCapacityDelayedRetryScheduleAndLimit(t *testing.T) {
	svc, mgr, messages, data, entry := startCapacityContinuationEpisode(t)
	delays := []int{5, 10, 20, 40, 60, 120, 240, 300, 300, 300, 300}
	for index, delay := range delays {
		notice := messages.sessionMessages[len(messages.sessionMessages)-1].metadata
		require.Equal(t, 11, notice["max_attempts"])
		require.Equal(t, delay, notice["retry_in_seconds"])
		require.NotEmpty(t, notice["retry_window_ends_at"])
		require.True(t, entry.claim())
		svc.retryTransientPrompt(entry.retryCtx, "t1", "s1", "execution-1")
		require.Equal(t, index+1, entry.started)
		generation := mgr.currentPromptGeneration.Load()
		data.PromptGeneration = generation
		data.CapacityContinuation.PromptGeneration = generation
		data.CapacityContinuation.CompletedTools = 0
		data.OutputObserved, data.EffectObserved = false, false
		svc.beginPromptAttempt("s1", "execution-1", generation, false)
		took := svc.handleTransientFailure(context.Background(), data)
		if index == len(delays)-1 {
			require.False(t, took, "there must be no twelfth automatic attempt")
		} else {
			require.True(t, took)
			next, ok := svc.transientRetries.Load("s1")
			require.True(t, ok)
			entry = next.(*transientRetryEntry)
		}
	}
	require.Len(t, mgr.capturedPromptCalls, 11)
}

// @covers AC-PLATFORM-TURN-CONTINUITY-003.3
func TestCapacityDelayedRetryDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		svc, mgr, _, _, entry := startCapacityContinuationEpisode(t)
		require.True(t, entry.claim(), "hold the timer dispatch until after suspension")
		time.Sleep(30 * time.Minute)
		svc.retryTransientPrompt(context.Background(), "t1", "s1", "execution-1")
		require.Empty(t, mgr.capturedPromptCalls, "deadline equality must refuse a delayed dispatch")
		require.Zero(t, entry.started)
	})
}

// @covers AC-PLATFORM-TURN-CONTINUITY-003.7
func TestCapacityDelayedRetryRestartRetirement(t *testing.T) {
	svc, messages, _ := newPersistentTransientRetryTestService(t)
	svc.createTransientRetryStatusMessage(context.Background(), watcher.AgentEventData{
		TaskID: "t1", SessionID: "s1", RecoveryMode: recoveryModeContinue, RecoveryAttemptsStarted: 8,
	}, &routingerr.Error{Code: routingerr.CodeModelCapacity}, 9, time.Minute, time.Now().Add(time.Minute))
	svc.reconcileExecutorSessionsOnStartup(context.Background())
	rows, err := messages.ListMessages(context.Background(), "s1")
	require.NoError(t, err)
	require.Empty(t, transientRetryNotices(rows, "t1", "s1"))
	require.Len(t, rows, 1)
	require.Equal(t, "restart_interrupted", rows[0].Metadata["recovery_disposition"])
	require.Equal(t, float64(8), rows[0].Metadata["attempts_started"])
	require.Empty(t, svc.agentManager.(*mockAgentManager).capturedPrompts)
}

// @covers AC-PLATFORM-TURN-CONTINUITY-003.8
func TestCapacityDelayedRetryOwnership(t *testing.T) {
	for _, reason := range []string{"cancel", "queued input", "unsafe tools", "different error", "missing evidence", "stale acceptance"} {
		t.Run(reason, func(t *testing.T) {
			svc, mgr, _, data, entry := startCapacityContinuationEpisode(t)
			require.True(t, entry.claim())
			switch reason {
			case "cancel":
				require.True(t, svc.CancelTransientRetry(context.Background(), "t1", "s1"))
			case "queued input":
				_, err := svc.messageQueue.QueueMessage(context.Background(), "s1", "t1", "user input", "", "user", false, nil)
				require.NoError(t, err)
			default:
				svc.retryTransientPrompt(entry.retryCtx, "t1", "s1", "execution-1")
				generation := mgr.currentPromptGeneration.Load()
				data.PromptGeneration, data.CapacityContinuation.PromptGeneration = generation, generation
				data.CapacityContinuation.CompletedTools = 0
				data.OutputObserved, data.EffectObserved = false, false
				svc.beginPromptAttempt("s1", "execution-1", generation, false)
				switch reason {
				case "unsafe tools":
					data.CapacityContinuation.PendingTools = true
				case "missing evidence":
					data.CapacityContinuation.EvidenceComplete = false
				case "stale acceptance":
					entry.mu.Lock()
					entry.acceptedGeneration--
					entry.mu.Unlock()
				default:
					data.ErrorMessage = overloaded529
					data.ProviderError.Message = overloaded529
				}
				require.False(t, svc.handleTransientFailure(context.Background(), data))
				require.Len(t, mgr.capturedPromptCalls, 1)
				return
			}
			svc.retryTransientPrompt(entry.retryCtx, "t1", "s1", "execution-1")
			require.Empty(t, mgr.capturedPromptCalls)
		})
	}
}

func TestCapacityDelayedRetryActiveTurnOutlivesDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		svc, mgr, _, data, entry := startCapacityContinuationEpisode(t)
		require.True(t, entry.claim())
		svc.retryTransientPrompt(entry.retryCtx, "t1", "s1", "execution-1")
		require.Equal(t, 1, entry.started)
		time.Sleep(31 * time.Minute)
		require.NoError(t, entry.retryCtx.Err(), "expiry must not interrupt admitted work")
		generation := mgr.currentPromptGeneration.Load()
		data.PromptGeneration, data.CapacityContinuation.PromptGeneration = generation, generation
		svc.beginPromptAttempt("s1", "execution-1", generation, false)
		require.False(t, svc.handleTransientFailure(context.Background(), data))
		require.Len(t, mgr.capturedPromptCalls, 1)
	})
}

func TestCapacityDelayedRetryTerminalWake(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		svc, mgr, messages, data, entry := startCapacityContinuationEpisode(t)
		require.True(t, entry.claim())
		svc.retryTransientPrompt(entry.retryCtx, "t1", "s1", "execution-1")
		time.Sleep(30*time.Minute - time.Second)
		generation := mgr.currentPromptGeneration.Load()
		data.PromptGeneration, data.CapacityContinuation.PromptGeneration = generation, generation
		svc.beginPromptAttempt("s1", "execution-1", generation, false)
		require.True(t, svc.handleTransientFailure(context.Background(), data))
		messages.mu.Lock()
		var terminalWakeFound bool
		for _, message := range messages.sessionMessages {
			if deadline, ok := message.metadata["retry_window_ends_at"]; ok && deadline == message.metadata["retry_at"] {
				terminalWakeFound = true
			}
		}
		messages.mu.Unlock()
		require.True(t, terminalWakeFound)
		time.Sleep(time.Second)
		synctest.Wait()
		require.Len(t, mgr.capturedPromptCalls, 1)
		_, owned := svc.transientRetries.Load("s1")
		require.False(t, owned)
		last := messages.sessionMessages[len(messages.sessionMessages)-1].metadata
		require.Equal(t, "window_expired", last["recovery_disposition"])
		require.Equal(t, 1, last["attempts_started"])
	})
}

func TestCapacityDelayedRetryFinalAdmissionDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		svc, mgr, messages, _, entry := startCapacityContinuationEpisode(t)
		require.True(t, entry.claim())
		checks := 0
		mgr.isAgentReadyFn = func(context.Context, string) bool {
			checks++
			if checks == 6 {
				time.Sleep(30 * time.Minute)
			}
			return true
		}
		svc.retryTransientPrompt(entry.retryCtx, "t1", "s1", "execution-1")
		require.GreaterOrEqual(t, checks, 6)
		require.Empty(t, mgr.capturedPromptCalls)
		require.Zero(t, entry.started)
		require.Equal(t, "window_expired", messages.sessionMessages[len(messages.sessionMessages)-1].metadata["recovery_disposition"])
	})
}

func TestCapacityDelayedRetryRestartRetirementWithLiveRuntime(t *testing.T) {
	svc, messages, session := liveCapacityRestartFixture(t)
	svc.retireInterruptedNoticeOnStartup(context.Background(), session)
	rows, err := messages.ListMessages(context.Background(), "s1")
	require.NoError(t, err)
	require.Len(t, rows, 1, "a live idle runtime still needs a truthful stopped-retry explanation")
	require.Equal(t, "restart_interrupted", rows[0].Metadata["recovery_disposition"])
	require.Equal(t, true, rows[0].Metadata["runtime_retained"])
	require.NotContains(t, rows[0].Metadata, "retry_window_ends_at")
	require.Empty(t, transientRetryNotices(rows, "t1", "s1"))
	require.Empty(t, svc.agentManager.(*mockAgentManager).capturedPrompts)
}

func TestCapacityDelayedRetryReplayAdmissionDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		svc, _ := newTransientTestService(t)
		t.Cleanup(svc.cancelAllTransientRetries)
		mgr := configureRetainedReplayRuntime(t, svc)
		armTransientPromptEvidence(svc)
		svc.rememberTurnPrompt("s1", "retry the request", "", false, nil)
		svc.handleAgentTurnFailed(context.Background(), retainedTurnFailureDataForReplay())
		value, ok := svc.transientRetries.Load("s1")
		require.True(t, ok)
		entry := value.(*transientRetryEntry)
		require.True(t, entry.claim())
		mgr.promptAdmissionEntered, mgr.promptAdmissionRelease = make(chan struct{}, 1), make(chan struct{})
		done := make(chan struct{})
		go func() { svc.retryTransientPrompt(entry.retryCtx, "t1", "s1", "execution-1"); close(done) }()
		<-mgr.promptAdmissionEntered
		time.Sleep(30 * time.Minute)
		close(mgr.promptAdmissionRelease)
		<-done
		require.Empty(t, mgr.capturedPromptCalls)
		require.Zero(t, entry.started, "an expired pre-provider admission is not a started attempt")
	})
}

func TestCapacityDelayedRetryReplayToContinuationPreservesDeadline(t *testing.T) {
	svc, mgr, data := capacityContinuationFailureFixture(t)
	require.NoError(t, svc.repo.UpsertExecutorRunning(context.Background(), &models.ExecutorRunning{
		ID: "live", TaskID: "t1", SessionID: "s1", AgentExecutionID: "execution-1", Status: "ready",
	}))
	data.OutputObserved, data.EffectObserved = false, false
	data.CapacityContinuation.CompletedTools = 0
	svc.rememberTurnPrompt("s1", "original request", "", false, nil)
	require.True(t, svc.handleTransientFailure(context.Background(), data))
	value, _ := svc.transientRetries.Load("s1")
	previous := value.(*transientRetryEntry)
	require.Equal(t, recoveryModeReplay, previous.mode)
	require.False(t, previous.policy.windowEndsAt.IsZero())
	require.True(t, previous.claim())
	svc.retryTransientPrompt(previous.retryCtx, "t1", "s1", "execution-1")
	generation := mgr.currentPromptGeneration.Load()
	data.PromptGeneration, data.CapacityContinuation.PromptGeneration = generation, generation
	data.OutputObserved, data.EffectObserved = true, true
	data.CapacityContinuation.CompletedTools = 2
	svc.beginPromptAttempt("s1", "execution-1", generation, false)
	svc.observePromptAttempt("s1", "execution-1", generation, true, true)
	require.True(t, svc.handleTransientFailure(context.Background(), data))
	value, _ = svc.transientRetries.Load("s1")
	next := value.(*transientRetryEntry)
	require.Equal(t, recoveryModeContinue, next.mode)
	require.Equal(t, previous.policy.windowEndsAt, next.policy.windowEndsAt)
	require.Equal(t, 1, next.started)
	require.Equal(t, 2, next.attempt)
}
