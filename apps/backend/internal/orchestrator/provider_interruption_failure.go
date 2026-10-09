package orchestrator

import (
	"context"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
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
	if started >= transientMaxAttempts && data.ContinuationSafety.SafeFor(data.PromptGeneration) {
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
