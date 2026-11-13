package orchestrator

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

// @covers AC-PLATFORM-TURN-CONTINUITY-003.10
func TestCapacityContinuationKnownCommandFailure(t *testing.T) {
	svc, mgr, data := capacityContinuationFailureFixture(t)
	defer svc.cancelAllTransientRetries()
	data.CapacityContinuation = &streams.CapacityContinuationSnapshot{
		Support: streams.CapacityContinuationCodexLiveSessionV2, PromptGeneration: data.PromptGeneration,
		EvidenceComplete: true, CompletedTools: 1, KnownFailedShellTools: 2,
	}
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
	require.NotNil(t, entry.continuation)
	require.NotNil(t, entry.retainedRuntime)
	require.True(t, entry.claim())
	svc.retryTransientPrompt(entry.retryCtx, "t1", "s1", "execution-1")

	mgr.mu.Lock()
	defer mgr.mu.Unlock()
	require.Empty(t, mgr.stopAgentWithReasonArgs)
	require.Empty(t, mgr.stopAgentArgs)
	require.Len(t, mgr.capturedPromptCalls, 1)
	require.Equal(t, "execution-1", mgr.capturedPromptCalls[0].ExecutionID)
	require.True(t, mgr.capturedPromptCalls[0].DispatchOnly)
	require.Contains(t, mgr.capturedPrompts[0], "selected model was at capacity")
	require.NotContains(t, mgr.capturedPrompts[0], "test original request")
	require.Equal(t, 1, entry.started)
}

// @covers AC-PLATFORM-TURN-CONTINUITY-003.10
func TestCapacityContinuationKnownCommandFailureBlocksMixedEvidence(t *testing.T) {
	cases := []struct {
		name    string
		support streams.CapacityContinuationSupport
		mutate  func(*streams.CapacityContinuationSnapshot)
		reason  string
	}{
		{name: "pending tool", mutate: func(s *streams.CapacityContinuationSnapshot) { s.PendingTools = true }, reason: "unsafe_work"},
		{name: "blocking failed tool", mutate: func(s *streams.CapacityContinuationSnapshot) { s.FailedTools = true }, reason: "unsafe_work"},
		{name: "unknown outcome", mutate: func(s *streams.CapacityContinuationSnapshot) { s.UnknownOutcomes = true }, reason: "unsafe_work"},
		{name: "permission pending", mutate: func(s *streams.CapacityContinuationSnapshot) { s.PermissionPending = true }, reason: "unsafe_work"},
		{name: "background work", mutate: func(s *streams.CapacityContinuationSnapshot) { s.UnaccountedBackground = true }, reason: "unsafe_work"},
		{name: "v1 cannot claim v2 evidence", support: streams.CapacityContinuationCodexLiveSessionV1,
			mutate: func(s *streams.CapacityContinuationSnapshot) { s.CompletedTools = 0 }, reason: "unsafe_work"},
		{name: "unknown support", support: "future_live_session_v3",
			mutate: func(s *streams.CapacityContinuationSnapshot) { s.CompletedTools = 1; s.KnownFailedShellTools = 0 },
			reason: "unsupported_restore"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, _, data := capacityContinuationFailureFixture(t)
			support := tc.support
			if support == "" {
				support = streams.CapacityContinuationCodexLiveSessionV2
			}
			data.CapacityContinuation = &streams.CapacityContinuationSnapshot{
				Support: support, PromptGeneration: data.PromptGeneration, EvidenceComplete: true,
				CompletedTools: 1, KnownFailedShellTools: 1,
			}
			tc.mutate(data.CapacityContinuation)
			require.False(t, svc.handleTransientFailure(context.Background(), data))
			_, owned := svc.transientRetries.Load("s1")
			require.False(t, owned)
			require.Equal(t, tc.reason, svc.continuationRefusalReason(context.Background(), data))
		})
	}
}

// @covers AC-PLATFORM-TURN-CONTINUITY-003.10
func TestCapacityContinuationKnownCommandFailureKeepsEpisode(t *testing.T) {
	svc, mgr, _, data, entry := startCapacityContinuationEpisode(t)
	require.True(t, entry.claim())
	svc.retryTransientPrompt(entry.retryCtx, "t1", "s1", "execution-1")

	generation := mgr.currentPromptGeneration.Load()
	data.PromptGeneration = generation
	data.CapacityContinuation = &streams.CapacityContinuationSnapshot{
		Support: streams.CapacityContinuationCodexLiveSessionV2, PromptGeneration: generation,
		EvidenceComplete: true, KnownFailedShellTools: 1,
	}
	data.OutputObserved, data.EffectObserved = false, false
	svc.beginPromptAttempt("s1", "execution-1", generation, false)
	require.True(t, svc.handleTransientFailure(context.Background(), data),
		"a known failed command alone must keep the admitted continuation episode")
	value, ok := svc.transientRetries.Load("s1")
	require.True(t, ok)
	next := value.(*transientRetryEntry)
	t.Cleanup(next.cancel)
	require.Equal(t, 2, next.attempt)
	require.Equal(t, 1, next.started)
	require.Equal(t, continuationPolicyCapacityLive, next.continuationPolicy)
}
