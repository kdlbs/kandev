package orchestrator

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/automation"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestInterruptionContinuationManualReason(t *testing.T) {
	for _, tc := range []struct {
		name, reason string
		mutate       func(*Service, *watcher.AgentEventData)
	}{
		{"disabled", "disabled", func(s *Service, _ *watcher.AgentEventData) { s.config.ProviderInterruptionContinuation = false }},
		{"unsafe", "unsafe_work", func(_ *Service, d *watcher.AgentEventData) { d.ContinuationSafety.Unsafe = true }},
		{"pending", "unsafe_work", func(_ *Service, d *watcher.AgentEventData) { d.ContinuationSafety.Pending = true }},
		{"unsupported", "unsupported_restore", func(_ *Service, d *watcher.AgentEventData) { d.ContinuationSafety.Support = "" }},
		{"unknown", "missing_evidence", func(_ *Service, d *watcher.AgentEventData) { d.ContinuationSafety = nil }},
		{"stale", "missing_evidence", func(_ *Service, d *watcher.AgentEventData) { d.PromptGeneration-- }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, mc, data := continuationFailureFixture(t)
			tc.mutate(svc, &data)
			require.NoError(t, svc.createRecoveryStatusMessage(context.Background(), data, ""))
			last := mc.sessionMessages[len(mc.sessionMessages)-1]
			require.Equal(t, tc.reason, last.metadata["recovery_reason"])
		})
	}
}

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
	_, claimed := svc.executionTeardownClaimFor("s1", "execution-1")
	require.False(t, claimed, "failed stops must release their teardown claim")
	latest := mc.sessionMessages[len(mc.sessionMessages)-1]
	require.Equal(t, true, latest.metadata["recovery_actions"])
}

func TestInterruptionContinuationPreparationLookupFailureStaysManual(t *testing.T) {
	svc, _, data := continuationFailureFixture(t)
	require.True(t, svc.handleTransientFailure(context.Background(), data))
	value, _ := svc.transientRetries.Load("s1")
	entry := value.(*transientRetryEntry)
	mgr := svc.agentManager.(*mockAgentManager)
	mgr.getExecutionIDForSessionFunc = func(context.Context, string) (string, error) { return "", errors.New("runtime lookup unavailable") }
	require.False(t, svc.retryContinuationPreparation(context.Background(), "t1", "s1", entry, errors.New("dial tcp: network is unreachable")))
	current, _ := svc.transientRetries.Load("s1")
	require.Same(t, entry, current, "uncertain liveness cannot publish a successor reservation")
}

func TestInterruptionContinuationCancellationRechecksRestoredExecution(t *testing.T) {
	svc, _, data := continuationFailureFixture(t)
	require.True(t, svc.handleTransientFailure(context.Background(), data))
	value, _ := svc.transientRetries.Load("s1")
	entry := value.(*transientRetryEntry)
	entry.restoredExecution = "restored"
	mgr := svc.agentManager.(*mockAgentManager)
	reads := 0
	mgr.getExecutionIDForSessionFunc = func(context.Context, string) (string, error) {
		reads++
		if reads == 1 {
			return "restored", nil
		}
		return "human-successor", nil
	}
	require.False(t, svc.cancelRestoredContinuation(context.Background(), "s1", entry))
	require.Zero(t, mgr.cancelAgentCalls.Load(), "cleanup cannot cancel the replacement observed at admission")
}

func TestInterruptionContinuationFailureFinalizesVisibleAutomationBeforeTurnClose(t *testing.T) {
	svc, _, entry := dispatchContinuationFixture(t)
	task, err := svc.repo.GetTask(context.Background(), "t1")
	require.NoError(t, err)
	task.Origin = models.TaskOriginAutomationTask
	require.NoError(t, svc.repo.UpdateTask(context.Background(), task))
	active, err := svc.turnService.GetActiveTurn(context.Background(), "s1")
	require.NoError(t, err)
	require.NotNil(t, active)
	runService := &continuationExactAutomationService{}
	svc.automationService = runService
	svc.settleContinuationFailureLocked(context.Background(), watcher.AgentEventData{TaskID: "t1", SessionID: "s1", AgentExecutionID: "replacement-1", ErrorMessage: "interrupted"}, entry)
	require.Equal(t, active.ID, runService.turnID)
	require.Equal(t, automation.RunStatusFailed, runService.status)
}

type continuationExactAutomationService struct {
	mockAutomationRunService
	automationRunBinding
	turnID string
	status automation.RunStatus
}

func (s *continuationExactAutomationService) MarkRunTerminalByBinding(_ context.Context, _, _, turnID string, status automation.RunStatus, _ string) error {
	s.turnID, s.status = turnID, status
	return nil
}
