package orchestrator

import (
	"context"
	"errors"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"go.uber.org/zap"
)

func (s *Service) recordCompletionGateRefusal(ctx context.Context, taskID, fromStepID, sessionID string, err error) {
	if !errors.Is(err, repoerrors.ErrTaskCompletionGateBlocked) {
		return
	}
	s.markStepCompletionBlocked(ctx, taskID, fromStepID, sessionID, models.StepCompletionReasonTransitionRefused)

	// A refused move has no committed transition to settle its delivery.
	// Record the no-op against the original turn and source policy.
	effect := workflowEffectFromContext(ctx)
	if !isBoundAgentDeliveryWorkflowEffect(effect) || effect.WorkflowPolicyUpdatedAt == nil ||
		effect.TaskID != taskID || effect.SessionID != sessionID || effect.WorkflowStepID != fromStepID {
		return
	}
	source := &models.Task{ID: taskID, WorkflowID: effect.WorkflowID, WorkflowStepID: fromStepID}
	s.recordDeliveryWorkflowNoOp(ctx, source, effect, models.DeliveryWorkflowOutcomeNoAutomaticTransition)
}

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
