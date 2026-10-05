package orchestrator

import (
	"context"
	"errors"
	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestInterruptionContinuationBudgetNativeInitializationFailure(t *testing.T) {
	svc, mc, data := continuationFailureFixture(t)
	data.ProviderError = &streams.ProviderError{ProviderID: "cursor-acp", ModelID: "cursor-model"}
	mgr := installContinuationRestoreFixture(t, svc)
	launch := mgr.launchAgentFunc
	var liveMu sync.Mutex
	liveID := "execution-1"
	launches := 0
	mgr.launchAgentFunc = func(ctx context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
		result, err := launch(ctx, req)
		launches++
		if launches > 1 {
			result.AgentExecutionID = "replacement-2"
			running, readErr := svc.repo.GetExecutorRunningBySessionID(ctx, "s1")
			require.NoError(t, readErr)
			running.AgentExecutionID = result.AgentExecutionID
			require.NoError(t, svc.repo.UpsertExecutorRunning(ctx, running))
			mgr.currentPromptExecutionID = result.AgentExecutionID
		}
		liveMu.Lock()
		liveID = result.AgentExecutionID
		liveMu.Unlock()
		_, _, stateErr := svc.repo.UpdateTaskSessionStateIfCurrent(ctx, "s1", models.TaskSessionStateWaitingForInput, models.TaskSessionStateStarting, "")
		require.NoError(t, stateErr)
		return result, err
	}
	mgr.isAgentRunningFn = func(context.Context, string) bool {
		liveMu.Lock()
		defer liveMu.Unlock()
		return liveID != ""
	}
	mgr.getExecutionIDForSessionFunc = func(context.Context, string) (string, error) {
		liveMu.Lock()
		defer liveMu.Unlock()
		return liveID, nil
	}
	mgr.stopAgentWithReasonFunc = func(ctx context.Context, id, _ string, _ bool) error {
		liveMu.Lock()
		defer liveMu.Unlock()
		require.Equal(t, liveID, id, "cleanup must target the failed native execution")
		liveID = ""
		if running, err := svc.repo.GetExecutorRunningBySessionID(ctx, "s1"); err == nil && running != nil {
			running.AgentExecutionID = ""
			require.NoError(t, svc.repo.UpsertExecutorRunning(ctx, running))
		}
		return nil
	}
	starts := 0
	mgr.startAgentProcessFunc = func(ctx context.Context, _ string) error {
		starts++
		if starts == 1 {
			return routingerr.NewAgentStartupFailure(routingerr.PhaseSessionInit, "cursor-acp", errors.New("dial tcp: network is unreachable"))
		}
		_, _, stateErr := svc.repo.UpdateTaskSessionStateIfCurrent(ctx, "s1", models.TaskSessionStateStarting, models.TaskSessionStateWaitingForInput, "")
		require.NoError(t, stateErr)
		return nil
	}
	require.True(t, svc.handleTransientFailure(context.Background(), data))
	first, _ := svc.transientRetries.Load("s1")
	entry := first.(*transientRetryEntry)
	require.True(t, entry.claim())
	svc.retryTransientPrompt(entry.retryCtx, "t1", "s1", "execution-1")
	nextValue, owned := svc.transientRetries.Load("s1")
	require.True(t, owned, "the native initialization failure must remain retryable")
	next := nextValue.(*transientRetryEntry)
	parked, err := svc.repo.GetTaskSession(context.Background(), "s1")
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateWaitingForInput, parked.State, "a stopped restore must not retain the startup grace projection")
	require.Equal(t, 2, next.attempt)
	latest := mc.sessionMessages[len(mc.sessionMessages)-1]
	require.Equal(t, "cursor-acp", latest.metadata["provider_name"])
	require.Equal(t, "cursor-model", latest.metadata["model_id"])
	require.Empty(t, mgr.capturedPrompts)
	liveMu.Lock()
	require.Empty(t, liveID, "failed restore must be stopped before retry scheduling")
	liveMu.Unlock()
	require.NoError(t, svc.validateContinuationOwner(next.retryCtx, "t1", "s1", next))
	require.True(t, next.claim())
	svc.retryTransientPrompt(next.retryCtx, "t1", "s1", "")
	require.Len(t, mgr.capturedPrompts, 1)
	require.Equal(t, continuationInstruction, mgr.capturedPrompts[0])
}

func TestInterruptionContinuationReservationRetainsImmutableOwner(t *testing.T) {
	svc, _, data := continuationFailureFixture(t)
	require.True(t, svc.handleTransientFailure(context.Background(), data))
	value, _ := svc.transientRetries.Load("s1")
	previous := value.(*transientRetryEntry)
	state, release := svc.acquireTransientRetryNoticeState("s1")
	defer release()
	state.mu.Lock()
	defer state.mu.Unlock()
	next := svc.reserveTransientRetryWithMetadataLocked(state, "s1", 2, nil)
	require.Equal(t, recoveryModeContinue, next.mode, "published reservations must retain continuation ownership")
	require.Same(t, previous.continuation, next.continuation)
}

func TestInterruptionContinuationBudgetCountsEarlierReplay(t *testing.T) {
	svc, _, data := continuationFailureFixture(t)
	mgr := installContinuationRestoreFixture(t, svc)
	data.OutputObserved, data.EffectObserved, data.ContinuationSafety = false, false, nil
	svc.rememberTurnPrompt("s1", "test original request", "", false, nil)
	require.True(t, svc.handleTransientFailure(context.Background(), data))
	require.NoError(t, svc.repo.UpsertExecutorRunning(context.Background(), &models.ExecutorRunning{ID: "live", TaskID: "t1", SessionID: "s1", AgentExecutionID: "replacement-1", ResumeToken: "provider-session", Resumable: true}))
	value, _ := svc.transientRetries.Load("s1")
	entry := value.(*transientRetryEntry)
	require.True(t, entry.claim())
	svc.retryTransientPrompt(entry.retryCtx, "t1", "s1", "execution-1")
	require.Len(t, mgr.capturedPrompts, 1)
	require.Equal(t, 1, entry.started, "replay and continuation must share actual attempt accounting")
}

// @covers AC-PLATFORM-INTERRUPTION-CONTINUATION-002.1
func TestInterruptionContinuationBudgetRestoreFailures(t *testing.T) {
	svc, mc, data := continuationFailureFixture(t)
	mgr := svc.agentManager.(*mockAgentManager)
	mgr.getExecutionIDForSessionFunc = func(context.Context, string) (string, error) {
		return "", agentruntime.ErrNoExecutionForSession
	}
	launches := 0
	mgr.launchAgentFunc = func(context.Context, *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
		launches++
		return nil, errors.New("dial tcp: network is unreachable")
	}
	require.True(t, svc.handleTransientFailure(context.Background(), data))
	for attempt := 1; attempt <= transientMaxAttempts; attempt++ {
		value, ok := svc.transientRetries.Load("s1")
		require.True(t, ok, "retry owner must survive a transient restore failure")
		entry := value.(*transientRetryEntry)
		require.Equal(t, attempt, entry.attempt)
		require.True(t, entry.claim())
		svc.retryTransientPrompt(entry.retryCtx, "t1", "s1", "execution-1")
		require.Equal(t, attempt, launches)
	}
	_, owned := svc.transientRetries.Load("s1")
	require.False(t, owned, "five started attempts must exhaust one shared budget")
	require.Empty(t, mgr.capturedPrompts, "restore failure must not dispatch any prompt")
	last := mc.sessionMessages[len(mc.sessionMessages)-1]
	require.Equal(t, "exhausted", last.metadata["recovery_disposition"])
	require.Equal(t, 5, last.metadata["attempts_started"])
}
