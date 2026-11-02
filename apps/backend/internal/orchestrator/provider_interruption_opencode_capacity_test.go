package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

const openCodeCapacityErrorMessage = "Internal error: Upstream request failed: [ttft_admission_refused] the cell is at capacity; retry shortly (after 21 s)"

// openCodeCapacityFailureFixture is the Codex-compatible capacity fixture with
// the observed OpenCode identity and the OpenCode live-session capability.
func openCodeCapacityFailureFixture(t *testing.T) (*Service, *mockAgentManager, watcher.AgentEventData) {
	t.Helper()
	svc, mgr, data := capacityContinuationFailureFixture(t)
	data.AgentID = "opencode-acp"
	data.ErrorMessage = openCodeCapacityErrorMessage
	data.ProviderError = &streams.ProviderError{
		Source: streams.ProviderErrorSourceACPPrompt, ProviderID: "opencode-acp",
		Message: openCodeCapacityErrorMessage,
	}
	data.CapacityContinuation = &streams.CapacityContinuationSnapshot{
		Support: streams.CapacityContinuationOpenCodeLiveSessionV1, PromptGeneration: data.PromptGeneration,
		EvidenceComplete: true, CompletedTools: 2,
	}
	return svc, mgr, data
}

func startOpenCodeCapacityContinuationEpisode(t *testing.T) (*Service, *mockAgentManager, *mockMessageCreator, watcher.AgentEventData, *transientRetryEntry) {
	t.Helper()
	svc, mgr, data := openCodeCapacityFailureFixture(t)
	t.Cleanup(svc.cancelAllTransientRetries)
	require.NoError(t, svc.repo.UpsertExecutorRunning(context.Background(), &models.ExecutorRunning{
		ID: "runtime-original", TaskID: "t1", SessionID: "s1", AgentExecutionID: "execution-1", Status: "ready",
	}))
	require.True(t, svc.handleTransientFailure(context.Background(), data))
	value, ok := svc.transientRetries.Load("s1")
	require.True(t, ok)
	entry := value.(*transientRetryEntry)
	t.Cleanup(entry.cancel)
	return svc, mgr, svc.messageCreator.(*mockMessageCreator), data, entry
}

// @covers AC-PLATFORM-TURN-CONTINUITY-003.2, AC-PLATFORM-TURN-CONTINUITY-003.4
func TestOpenCodeCapacityContinuationUsesSameRuntime(t *testing.T) {
	svc, mgr, data := openCodeCapacityFailureFixture(t)
	t.Cleanup(svc.cancelAllTransientRetries)
	mgr.launchAgentFunc = func(context.Context, *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
		return nil, errors.New("capacity continuation unexpectedly relaunched the runtime")
	}
	require.NoError(t, svc.repo.UpsertExecutorRunning(context.Background(), &models.ExecutorRunning{
		ID: "runtime-original", TaskID: "t1", SessionID: "s1", AgentExecutionID: "execution-1", Status: "ready",
	}))

	require.True(t, svc.handleTransientFailure(context.Background(), data))
	value, ok := svc.transientRetries.Load("s1")
	require.True(t, ok)
	entry := value.(*transientRetryEntry)
	t.Cleanup(entry.cancel)
	require.Equal(t, recoveryModeContinue, entry.mode)
	require.Equal(t, continuationPolicyCapacityLive, entry.continuationPolicy)
	require.NotNil(t, entry.retainedRuntime)
	require.True(t, entry.claim())
	svc.retryTransientPrompt(entry.retryCtx, "t1", "s1", "execution-1")

	mgr.mu.Lock()
	defer mgr.mu.Unlock()
	require.Empty(t, mgr.stopAgentWithReasonArgs)
	require.Empty(t, mgr.stopAgentArgs)
	require.Len(t, mgr.capturedPromptCalls, 1)
	require.True(t, mgr.capturedPromptCalls[0].DispatchOnly)
	require.Contains(t, mgr.capturedPrompts[0], "selected model was at capacity")
	require.Contains(t, mgr.capturedPrompts[0], "Preserve all completed actions")
	require.NotContains(t, mgr.capturedPrompts[0], "test original request")
	require.Equal(t, 1, entry.started)
}

// @covers AC-PLATFORM-TURN-CONTINUITY-003.2, AC-PLATFORM-TURN-CONTINUITY-003.4
func TestOpenCodeCapacityContinuationSupersession(t *testing.T) {
	svc, mgr, _, data, entry := startOpenCodeCapacityContinuationEpisode(t)
	_, err := svc.messageQueue.QueueMessage(context.Background(), "s1", "t1", "human queued prompt", "", "user", false, nil)
	require.NoError(t, err)
	require.True(t, entry.claim())
	svc.retryTransientPrompt(entry.retryCtx, "t1", "s1", "execution-1")
	assertCapacityContinuationRefusedWithoutRuntimeMutation(t, svc, mgr)
	require.Zero(t, entry.started)
	require.Equal(t, continuationPolicyCapacityLive, entry.continuationPolicy)
	require.Equal(t, data.PromptGeneration, data.CapacityContinuation.PromptGeneration)
}

// @covers AC-PLATFORM-TURN-CONTINUITY-003.1, AC-PLATFORM-TURN-CONTINUITY-003.6, AC-PLATFORM-TURN-CONTINUITY-003.8
func TestOpenCodeCapacityContinuationEvidence(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*watcher.AgentEventData)
		want   bool
	}{
		{name: "complete OpenCode evidence", want: true},
		{name: "unknown capability", mutate: func(d *watcher.AgentEventData) {
			d.CapacityContinuation.Support = "opencode_live_session_v2"
		}},
		{name: "mock-only capability", mutate: func(d *watcher.AgentEventData) {
			d.CapacityContinuation.Support = streams.CapacityContinuationMockLiveSessionV1
		}, want: true},
		{name: "missing retention", mutate: func(d *watcher.AgentEventData) {
			d.PromptFailureDisposition = ""
		}},
		{name: "stale generation", mutate: func(d *watcher.AgentEventData) {
			d.CapacityContinuation.PromptGeneration--
		}},
		{name: "incomplete evidence", mutate: func(d *watcher.AgentEventData) {
			d.CapacityContinuation.EvidenceComplete = false
		}},
		{name: "pending tool", mutate: func(d *watcher.AgentEventData) {
			d.CapacityContinuation.PendingTools = true
		}},
		{name: "failed tool", mutate: func(d *watcher.AgentEventData) {
			d.CapacityContinuation.FailedTools = true
		}},
		{name: "unknown outcome", mutate: func(d *watcher.AgentEventData) {
			d.CapacityContinuation.UnknownOutcomes = true
		}},
		{name: "pending permission", mutate: func(d *watcher.AgentEventData) {
			d.CapacityContinuation.PermissionPending = true
		}},
		{name: "background work", mutate: func(d *watcher.AgentEventData) {
			d.CapacityContinuation.UnaccountedBackground = true
		}},
		{name: "no completed tool", mutate: func(d *watcher.AgentEventData) {
			d.CapacityContinuation.CompletedTools = 0
		}},
		{name: "dynamic route", mutate: func(d *watcher.AgentEventData) {
			d.DynamicRouteAttempt = true
		}},
		{name: "office owner", mutate: func(d *watcher.AgentEventData) {
			d.OwnerKind = "office"
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc, _, data := openCodeCapacityFailureFixture(t)
			t.Cleanup(svc.cancelAllTransientRetries)
			if tc.mutate != nil {
				tc.mutate(&data)
			}
			require.Equal(t, tc.want, svc.capacityContinuationFailureHasEvidence(data))
		})
	}
}

// @covers AC-PLATFORM-TURN-CONTINUITY-003.3, AC-PLATFORM-TURN-CONTINUITY-003.7
func TestOpenCodeCapacityRetryHintFloor(t *testing.T) {
	now := time.Date(2026, 10, 28, 22, 27, 49, 0, time.UTC)
	capacityPolicy := transientRetryPolicy{windowEndsAt: now.Add(capacityRetryWindow)}
	hint := func(after time.Duration) *time.Time {
		value := now.Add(after)
		return &value
	}

	tests := []struct {
		name    string
		attempt int
		reset   *time.Time
		want    time.Duration
	}{
		{name: "21 seconds exceeds the 5 second slot", attempt: 1, reset: hint(21 * time.Second), want: 21 * time.Second},
		{name: "hint below the 5 second slot", attempt: 1, reset: hint(3 * time.Second), want: 5 * time.Second},
		{name: "9 seconds cannot shorten the 40 second slot", attempt: 4, reset: hint(9 * time.Second), want: 40 * time.Second},
		{name: "21 seconds exceeds the 20 second slot", attempt: 3, reset: hint(21 * time.Second), want: 21 * time.Second},
		{name: "nominal delay without a hint", attempt: 2, want: 10 * time.Second},
		{name: "stale hint uses the nominal delay", attempt: 2, reset: func() *time.Time {
			value := now.Add(-time.Minute)
			return &value
		}(), want: 10 * time.Second},
		{name: "hint beyond one minute uses the nominal delay", attempt: 2, reset: hint(5 * time.Minute), want: 10 * time.Second},
		{name: "later slots keep the slower ladder", attempt: 6, reset: hint(21 * time.Second), want: 2 * time.Minute},
		{name: "last ladder slot", attempt: 11, reset: hint(21 * time.Second), want: 5 * time.Minute},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			classified := &routingerr.Error{Code: routingerr.CodeModelCapacity, ResetHint: tc.reset}
			require.Equal(t, tc.want, capacityPolicy.delay(classified, tc.attempt, now))
		})
	}

	// Non-capacity short retries keep their existing hint behavior.
	shortPolicy := transientRetryPolicy{}
	require.Equal(t, 3*time.Second, shortPolicy.delay(
		&routingerr.Error{Code: routingerr.CodeRateLimited, ResetHint: hint(3 * time.Second)}, 1, now,
	))
}

// The observed relative hint reaches the durable notice as a floor instead of
// replacing the nominal ladder.
//
// @covers AC-PLATFORM-TURN-CONTINUITY-003.3, AC-PLATFORM-TURN-CONTINUITY-003.7
func TestOpenCodeCapacityRetryHintFloorNotice(t *testing.T) {
	now := time.Date(2026, 10, 28, 22, 27, 49, 0, time.UTC)
	svc, _, data := openCodeCapacityFailureFixture(t)
	t.Cleanup(svc.cancelAllTransientRetries)
	svc.config.RetryClock = fixedCodexRetryClock{now: now}
	data.ProviderError.OccurredAt = now
	require.NoError(t, svc.repo.UpsertExecutorRunning(context.Background(), &models.ExecutorRunning{
		ID: "runtime-original", TaskID: "t1", SessionID: "s1", AgentExecutionID: "execution-1", Status: "ready",
	}))

	require.True(t, svc.handleTransientFailure(context.Background(), data))
	value, ok := svc.transientRetries.Load("s1")
	require.True(t, ok)
	entry := value.(*transientRetryEntry)
	t.Cleanup(entry.cancel)
	require.Equal(t, now.Add(21*time.Second), entry.retryAt)

	messages := svc.messageCreator.(*mockMessageCreator)
	require.NotEmpty(t, messages.sessionMessages)
	notice := messages.sessionMessages[len(messages.sessionMessages)-1]
	require.Equal(t, 21, notice.metadata["retry_in_seconds"])
	require.Equal(t, 11, notice.metadata["max_attempts"])
	require.NotEmpty(t, notice.metadata["retry_window_ends_at"])
}

// Unresolved detached work must never dispatch an automatic continuation.
//
// @covers AC-PLATFORM-TURN-CONTINUITY-003.1, AC-PLATFORM-TURN-CONTINUITY-003.8
func TestOpenCodeCapacityUnresolvedBackgroundDoesNotDispatch(t *testing.T) {
	svc, mgr, data := openCodeCapacityFailureFixture(t)
	t.Cleanup(svc.cancelAllTransientRetries)
	require.NoError(t, svc.repo.UpsertExecutorRunning(context.Background(), &models.ExecutorRunning{
		ID: "runtime-original", TaskID: "t1", SessionID: "s1", AgentExecutionID: "execution-1", Status: "ready",
	}))
	data.CapacityContinuation.UnaccountedBackground = true

	require.False(t, svc.handleTransientFailure(context.Background(), data))
	_, owned := svc.transientRetries.Load("s1")
	require.False(t, owned, "unresolved work must not create a retry owner")
	mgr.mu.Lock()
	defer mgr.mu.Unlock()
	require.Empty(t, mgr.capturedPromptCalls)
	require.Empty(t, mgr.capturedPrompts)
	require.Empty(t, mgr.stopAgentArgs)
}
