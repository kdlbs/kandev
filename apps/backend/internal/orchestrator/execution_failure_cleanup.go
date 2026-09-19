package orchestrator

import (
	"context"

	"go.uber.org/zap"
)

func (s *Service) cleanupAgentExecutionWithReason(executionID, taskID, sessionID, reason string) {
	if executionID == "" {
		return
	}
	if !s.claimForcedExecutionCleanup(sessionID, executionID) {
		s.logger.Debug("skipping duplicate execution teardown",
			zap.String("execution_id", executionID),
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID))
		return
	}
	ctx := context.Background()
	if !s.isExecutionCompleted(sessionID, executionID) {
		// Direct crash/forced-cleanup callers may reach this boundary without a
		// preceding lifecycle event. Preserve an existing completed marker (and
		// its allowed terminal stream), otherwise tombstone trailing frames.
		s.markExecutionFailed(sessionID, executionID)
	}
	// Defensive terminal-boundary retirement. Normal lifecycle events run this
	// first; the repeated forced-cleanup call is idempotent.
	s.retireExecutionActivityAndPublish(ctx, taskID, sessionID, executionID)
	if err := s.executor.StopExecution(ctx, executionID, reason, true); err != nil {
		s.logger.Debug("agent execution cleanup after terminal state",
			zap.String("execution_id", executionID),
			zap.String("task_id", taskID),
			zap.Error(err))
	}
}
