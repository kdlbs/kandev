package orchestrator

import (
	"context"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"

	"go.uber.org/zap"
)

// cleanupAgentExecutionWithReason reports whether teardown is complete and
// workflow recovery may proceed, including a previously completed exact claim.
func (s *Service) cleanupAgentExecutionWithReason(ctx context.Context, executionID, taskID, sessionID, reason string) bool {
	if ctx.Err() != nil {
		return false
	}
	if executionID == "" {
		return true
	}
	if reason == "agent completed" && s.lspLeases != nil && s.lspLeases.HasActiveLSPLeaseForExecution(executionID) {
		s.logger.Debug("deferring execution teardown while its language server lease is active",
			zap.String("execution_id", executionID),
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID))
		return false
	}
	if !s.claimForcedExecutionCleanup(sessionID, executionID) {
		s.logger.Debug("skipping duplicate execution teardown",
			zap.String("execution_id", executionID),
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID))
		claim, ok := s.executionTeardownClaimFor(sessionID, executionID)
		return ok && claim.cleanupCompleted
	}
	claimKey := terminalExecutionKey(sessionID, executionID)
	claim, claimed := s.executionTeardownClaimFor(sessionID, executionID)
	if !s.isExecutionCompleted(sessionID, executionID) {
		// Direct crash/forced-cleanup callers may reach this boundary without a
		// preceding lifecycle event. Preserve an existing completed marker (and
		// its allowed terminal stream), otherwise tombstone trailing frames.
		s.markExecutionFailed(sessionID, executionID)
	}
	// Defensive terminal-boundary retirement. Normal lifecycle events run this
	// first; the repeated forced-cleanup call is idempotent.
	s.retireExecutionActivityAndPublish(ctx, taskID, sessionID, executionID)
	if s.lspLeases != nil {
		s.lspLeases.StopLSPLeasesForExecution(executionID)
	}
	if err := s.executor.StopExecution(ctx, executionID, reason, true); err != nil && !agentruntime.IsNotFound(err) {
		s.logger.Debug("agent execution cleanup after terminal state",
			zap.String("execution_id", executionID),
			zap.String("task_id", taskID),
			zap.Error(err))
		if claimed && (reason == agentruntime.StopReasonRecoverableAgentFailure || reason == agentruntime.StopReasonAgentBootstrapFailed) {
			// A failed teardown remains retryable without releasing a newer claim.
			s.executionTeardownClaims.CompareAndDelete(claimKey, claim)
		}
		return false
	}
	if claimed {
		s.completeExecutionTeardownClaim(sessionID, executionID, claim)
	}
	return true
}

// startAgentFailureRecovery shares shutdown ownership with dynamic fallback
// launches. The worker must leave the failure publisher before stopping an
// execution, because that publisher can still hold the lifecycle prompt lock.
func (s *Service) startAgentFailureRecovery(recoverFailure func(context.Context)) {
	s.dynamicSuccessorMu.Lock()
	if s.dynamicSuccessorStopped {
		s.dynamicSuccessorMu.Unlock()
		return
	}
	if s.dynamicSuccessorCtx == nil {
		s.dynamicSuccessorCtx, s.dynamicSuccessorCancel = context.WithCancel(context.Background())
	}
	workerCtx := s.dynamicSuccessorCtx
	s.dynamicSuccessorWorkers.Add(1)
	s.dynamicSuccessorMu.Unlock()
	go func() {
		defer s.dynamicSuccessorWorkers.Done()
		ctx, cancel := context.WithTimeout(workerCtx, dynamicSuccessorDetachedTimeout)
		defer cancel()
		recoverFailure(ctx)
	}()
}

// completeExecutionTeardownClaim records successful cleanup only while the
// caller still owns the same claim, preserving any newer teardown owner.
func (s *Service) completeExecutionTeardownClaim(sessionID, executionID string, claim executionTeardownClaim) {
	completed := claim
	completed.cleanupCompleted = true
	s.executionTeardownClaims.CompareAndSwap(terminalExecutionKey(sessionID, executionID), claim, completed)
}
