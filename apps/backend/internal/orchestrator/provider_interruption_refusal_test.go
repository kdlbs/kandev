package orchestrator

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/stretchr/testify/require"
)

func TestInterruptionContinuationOwnershipAbsentSessionStillConfirmsExactRestoreTeardown(t *testing.T) {
	svc, _, data := continuationFailureFixture(t)
	mgr := svc.agentManager.(*mockAgentManager)
	require.True(t, svc.handleTransientFailure(context.Background(), data))
	value, _ := svc.transientRetries.Load("s1")
	entry := value.(*transientRetryEntry)
	require.True(t, entry.claim())
	entry.restoredExecution = "failed-restore"
	mgr.getExecutionIDForSessionFunc = func(context.Context, string) (string, error) {
		return "", lifecycle.ErrNoExecutionForSession
	}
	mgr.stopAgentWithReasonErr = lifecycle.ErrExecutionNotFound
	require.True(t, svc.stopFailedContinuationRestore(context.Background(), "t1", "s1", entry))
	require.Len(t, mgr.stopAgentWithReasonArgs, 1)
	require.Equal(t, "failed-restore", mgr.stopAgentWithReasonArgs[0].ExecutionID)
}

func TestInterruptionContinuationBudgetUnsafeReplacementEndsEpisode(t *testing.T) {
	svc, mgr, _ := dispatchContinuationFixture(t)
	generation := mgr.currentPromptGeneration.Load()
	registry := svc.resumeAttemptStore()
	registry.mu.Lock()
	origin := (&resumeAttempt{id: registry.nextID}).identity()
	registry.mu.Unlock()
	require.True(t, svc.resumeAttemptAllowsExecution("s1", "replacement-1", origin))
	svc.handleAgentFailed(context.Background(), watcher.AgentEventData{
		TaskID: "t1", SessionID: "s1", OwnerKind: "task", AgentExecutionID: "replacement-1", AgentID: "cursor-acp", PromptGeneration: generation,
		AttemptID: origin, ErrorMessage: cursorRetriableConnectionStalled, EvidenceKnown: true, EffectObserved: true,
		ContinuationSafety: &streams.ContinuationSafetySnapshot{Support: streams.ContinuationNativeSavedHistoryV1, Known: true, Unsafe: true, PromptGeneration: generation},
	})
	_, owned := svc.transientRetries.Load("s1")
	require.False(t, owned, "unsafe replacement work must terminate the episode")
	mc := svc.messageCreator.(*mockMessageCreator)
	latest := mc.sessionMessages[len(mc.sessionMessages)-1]
	require.Equal(t, "manual", latest.metadata["recovery_disposition"])
	require.Equal(t, 1, latest.metadata["attempts_started"])
}

func TestInterruptionContinuationOwnershipTeardownFailurePreventsLaunch(t *testing.T) {
	svc, mc, data := continuationFailureFixture(t)
	mgr := installContinuationRestoreFixture(t, svc)
	mgr.stopAgentWithReasonErr = errors.New("runtime is still running")
	require.True(t, svc.handleTransientFailure(context.Background(), data))
	value, _ := svc.transientRetries.Load("s1")
	entry := value.(*transientRetryEntry)
	require.True(t, entry.claim())
	svc.retryTransientPrompt(entry.retryCtx, "t1", "s1", "execution-1")
	require.Empty(t, mgr.capturedPrompts)
	latest := mc.sessionMessages[len(mc.sessionMessages)-1]
	require.Equal(t, true, latest.metadata["recovery_actions"])
}
