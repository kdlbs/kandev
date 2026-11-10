package orchestrator

import (
	"context"

	"github.com/kandev/kandev/internal/task/models"
	"go.uber.org/zap"
)

// stepCompletionReceiptOutcomeWriter persists a bounded receipt outcome for one
// exact accepted signal. The SQLite repository implements it.
type stepCompletionReceiptOutcomeWriter interface {
	SetStepCompletionReceiptOutcome(ctx context.Context, sessionID, signalID, outcome, reason string) (bool, error)
}

// markStepCompletionBlocked records a safe refusal on the completion receipt
// that owns this source step. A missing guard or legacy signal without
// identity stays silent; only the accepted source can be marked.
func (s *Service) markStepCompletionBlocked(ctx context.Context, taskID, fromStepID, sessionID, reason string) {
	guard, ok := models.WorkflowStepCompletionGuardFromContext(ctx, taskID, fromStepID)
	if !ok || guard.SignalID == "" || guard.SessionID == "" {
		return
	}
	writer, ok := s.repo.(stepCompletionReceiptOutcomeWriter)
	if !ok {
		return
	}
	if _, err := writer.SetStepCompletionReceiptOutcome(ctx, guard.SessionID, guard.SignalID, models.StepCompletionOutcomeBlocked, reason); err != nil {
		s.logger.Warn("failed to record blocked step-completion outcome",
			zap.String("task_id", taskID),
			zap.String("session_id", guard.SessionID),
			zap.Error(err))
	}
}
