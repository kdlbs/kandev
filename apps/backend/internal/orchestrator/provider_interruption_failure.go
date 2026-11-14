package orchestrator

import (
	"context"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	"go.uber.org/zap"
)

// The session guard fences this settlement against a successor dispatch.
func (s *Service) settleContinuationFailureLocked(ctx context.Context, data watcher.AgentEventData, entry *transientRetryEntry) func(context.Context) {
	s.resetTransientRetry(data.SessionID)
	data.RecoveryMode = recoveryModeContinue
	data.RecoveryDisposition = recoveryDispositionManual
	entry.mu.Lock()
	started := entry.started
	entry.mu.Unlock()
	if started >= entry.policy.maxAttempts() && continuationExhaustionEvidence(entry, data) {
		data.RecoveryDisposition = recoveryDispositionExhausted
	}
	nextState := models.TaskSessionStateWaitingForInput
	s.finalizeAutomationRun(ctx, data.TaskID, false, agentFailureMessage(data))
	if err := s.settleContinuationInterruption(ctx, data); err != nil {
		nextState = models.TaskSessionStateFailed
		s.logger.Warn("failed to persist interrupted continuation", zap.String("session_id", data.SessionID), zap.Error(err))
	}
	_ = s.persistLastAgentError(ctx, data)
	_ = s.createContinuationRecoveryMessage(ctx, data, entry)
	s.retireExecutionActivityAndPublish(context.WithoutCancel(ctx), data.TaskID, data.SessionID, data.AgentExecutionID)
	s.updateTaskSessionState(ctx, data.TaskID, data.SessionID, nextState, data.ErrorMessage, false)
	return func(workerCtx context.Context) {
		s.cleanupAgentExecutionWithReason(workerCtx, data.AgentExecutionID, data.TaskID, data.SessionID, agentruntime.StopReasonRecoverableAgentFailure)
	}
}

func continuationExhaustionEvidence(entry *transientRetryEntry, data watcher.AgentEventData) bool {
	safety := data.ContinuationSafety
	if safety == nil || !safety.SafeFor(data.PromptGeneration) {
		return false
	}
	if !safety.Pending {
		return true
	}
	if entry == nil || entry.mode != recoveryModeContinue ||
		entry.continuationPolicy != continuationPolicyAgentManagedPendingLive ||
		entry.continuation == nil || entry.retainedRuntime == nil ||
		data.PromptFailureDisposition != streams.PromptFailureDispositionRetainRuntime ||
		!data.PromptFailureDisposition.Valid() {
		return false
	}
	entry.mu.Lock()
	defer entry.mu.Unlock()
	return entry.started > 0 && entry.acceptedExecution == data.AgentExecutionID &&
		entry.acceptedGeneration == data.PromptGeneration
}
