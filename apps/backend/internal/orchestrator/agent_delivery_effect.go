package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
)

type workflowEffectContextKey struct{}

type agentDeliveryEffectReader interface {
	GetAgentDeliveryEffect(ctx context.Context, effectKey string) (*models.AgentDeliveryEffect, error)
}

type workflowMoveAdmissionEffectRepository interface {
	UpdateTaskWithWorkflowStepAdmissionAndEffect(
		ctx context.Context,
		task *models.Task,
		sourceStepID, targetStepID string,
		limit int,
		effect *models.AgentDeliveryEffect,
	) (admitted, applied bool, err error)
}

type workflowMoveAdmissionCASEffectRepository interface {
	UpdateTaskWithWorkflowStepAdmissionIfAtStepAndEffect(
		ctx context.Context,
		task *models.Task,
		expectedStepID, targetStepID string,
		limit int,
		effect *models.AgentDeliveryEffect,
	) (applied bool, err error)
}

func withWorkflowEffect(ctx context.Context, effect *models.AgentDeliveryEffect) context.Context {
	if effect == nil {
		return ctx
	}
	return context.WithValue(ctx, workflowEffectContextKey{}, effect)
}

func workflowEffectFromContext(ctx context.Context) *models.AgentDeliveryEffect {
	if ctx == nil {
		return nil
	}
	effect, _ := ctx.Value(workflowEffectContextKey{}).(*models.AgentDeliveryEffect)
	return effect
}

func workflowEffectForEvent(data watcher.AgentEventData) *models.AgentDeliveryEffect {
	if data.TurnID == "" && !agentDeliveryEventIdentityPresent(data) {
		return nil
	}
	now := time.Now().UTC()
	effect := &models.AgentDeliveryEffect{
		EffectKey:   workflowEffectKey(data.TurnID),
		EffectType:  "workflow.on_turn_complete",
		State:       models.DeliveryEffectCompleted,
		TaskID:      data.TaskID,
		SessionID:   data.SessionID,
		TurnID:      data.TurnID,
		CreatedAt:   now,
		CompletedAt: &now,
	}
	if !agentDeliveryEventIdentityPresent(data) {
		return effect
	}
	effect.StreamID = data.DeliveryStreamID
	effect.IncarnationID = data.DeliveryIncarnationID
	effect.SubmissionID = data.DeliverySubmissionID
	if data.DeliveryHarnessGeneration > 0 && data.DeliveryHarnessGeneration <= 1<<63-1 {
		effect.HarnessGeneration = int64(data.DeliveryHarnessGeneration)
	}
	if data.DeliverySequence > 0 && data.DeliverySequence <= 1<<63-1 {
		effect.Sequence = int64(data.DeliverySequence)
	}
	return effect
}

func workflowEffectForTurn(turnID string) *models.AgentDeliveryEffect {
	return workflowEffectForEvent(watcher.AgentEventData{TurnID: turnID})
}

// workflowEffectAlreadyApplied is deliberately fail-closed once the durable
// effect store is available. A read failure cannot safely distinguish a first
// delivery from a replay, so the caller must not evaluate another workflow
// transition in that case.
func (s *Service) workflowEffectAlreadyApplied(ctx context.Context, effect *models.AgentDeliveryEffect) bool {
	if effect == nil {
		return false
	}
	if effect.EffectKey == "" {
		return true
	}
	reader, ok := s.repo.(agentDeliveryEffectReader)
	if !ok {
		return false
	}
	stored, err := reader.GetAgentDeliveryEffect(ctx, effect.EffectKey)
	if errors.Is(err, repository.ErrAgentDeliveryEffectNotFound) {
		return false
	}
	if err != nil {
		s.logger.Warn("failed to read durable workflow effect; suppressing replay",
			zap.String("effect_key", effect.EffectKey), zap.Error(err))
		return true
	}
	if stored == nil {
		return true
	}
	if !isBoundAgentDeliveryWorkflowEffect(effect) {
		return true
	}
	if !isBoundAgentDeliveryWorkflowEffect(stored) {
		if backfiller, ok := s.repo.(repository.AgentDeliveryWorkflowOutcomeRepository); ok {
			if bound, bindErr := backfiller.BackfillLegacyAgentDeliveryWorkflowEffect(ctx, effect); bindErr == nil && bound {
				return true
			} else if bindErr != nil {
				s.logger.Warn("failed to bind legacy workflow effect to its terminal event",
					zap.String("effect_key", effect.EffectKey), zap.Error(bindErr))
			}
		}
		s.logger.Warn("suppressing workflow replay with ambiguous terminal identity",
			zap.String("effect_key", effect.EffectKey))
		return true
	}
	if !sameBoundAgentDeliveryWorkflowEffect(stored, effect) {
		s.logger.Warn("suppressing workflow replay with mismatched terminal identity",
			zap.String("effect_key", effect.EffectKey),
			zap.String("stored_stream_id", stored.StreamID),
			zap.String("incoming_stream_id", effect.StreamID))
		return true
	}
	return true
}

func workflowEffectKey(turnID string) string {
	if turnID == "" {
		return ""
	}
	return "workflow.on_turn_complete:" + turnID
}

func agentDeliveryEventIdentityPresent(data watcher.AgentEventData) bool {
	return data.DeliveryStreamID != "" || data.DeliveryIncarnationID != "" ||
		data.DeliveryHarnessGeneration != 0 || data.DeliverySequence != 0 || data.DeliverySubmissionID != ""
}

func isBoundAgentDeliveryWorkflowEffect(effect *models.AgentDeliveryEffect) bool {
	return effect != nil && effect.EffectKey != "" && effect.TurnID != "" && effect.StreamID != "" &&
		effect.Sequence > 0 && effect.SessionID != "" && effect.IncarnationID != "" &&
		effect.HarnessGeneration > 0 && effect.SubmissionID != ""
}

func sameBoundAgentDeliveryWorkflowEffect(left, right *models.AgentDeliveryEffect) bool {
	return isBoundAgentDeliveryWorkflowEffect(left) && isBoundAgentDeliveryWorkflowEffect(right) &&
		left.EffectKey == right.EffectKey && left.EffectType == right.EffectType &&
		left.StreamID == right.StreamID && left.Sequence == right.Sequence &&
		left.SessionID == right.SessionID && left.IncarnationID == right.IncarnationID &&
		left.HarnessGeneration == right.HarnessGeneration && left.SubmissionID == right.SubmissionID &&
		left.TurnID == right.TurnID
}

func withWorkflowOutcome(ctx context.Context, outcome string) context.Context {
	effect := workflowEffectFromContext(ctx)
	if effect == nil {
		return ctx
	}
	completed := *effect
	now := time.Now().UTC()
	completed.State = models.DeliveryEffectCompleted
	completed.Outcome = outcome
	completed.CompletedAt = &now
	if completed.CreatedAt.IsZero() {
		completed.CreatedAt = now
	}
	return withWorkflowEffect(ctx, &completed)
}

func withWorkflowPolicy(ctx context.Context, task *models.Task, stepTime time.Time) context.Context {
	effect := workflowEffectFromContext(ctx)
	if effect == nil || !isBoundAgentDeliveryWorkflowEffect(effect) {
		return ctx
	}
	guarded := *effect
	guarded.TaskID = task.ID
	guarded.WorkflowID = task.WorkflowID
	guarded.WorkflowStepID = task.WorkflowStepID
	if task.WorkflowStepID != "" {
		guarded.WorkflowPolicyUpdatedAt = &stepTime
	} else {
		guarded.WorkflowPolicyUpdatedAt = nil
	}
	return withWorkflowEffect(ctx, &guarded)
}

func (s *Service) recordDeliveryWorkflowNoOp(
	ctx context.Context,
	task *models.Task,
	effect *models.AgentDeliveryEffect,
	outcome string,
) bool {
	if effect == nil || !agentDeliveryEffectCarriesDeliveryIdentity(effect) {
		return true
	}
	if !isBoundAgentDeliveryWorkflowEffect(effect) || task == nil || task.ID == "" || effect.TaskID != task.ID {
		s.logger.Warn("leaving delivery terminal pending because workflow identity is incomplete",
			zap.String("task_id", taskIDOrEmpty(task)), zap.String("effect_key", effect.EffectKey))
		return false
	}
	repo, ok := s.repo.(repository.AgentDeliveryWorkflowOutcomeRepository)
	if !ok {
		s.logger.Warn("leaving delivery terminal pending because guarded workflow outcome storage is unavailable",
			zap.String("task_id", task.ID), zap.String("effect_key", effect.EffectKey))
		return false
	}
	guarded := *effect
	guarded.State = models.DeliveryEffectCompleted
	guarded.Outcome = outcome
	now := time.Now().UTC()
	if guarded.CreatedAt.IsZero() {
		guarded.CreatedAt = now
	}
	guarded.CompletedAt = &now
	if !s.bindDeliveryWorkflowPolicy(ctx, task, &guarded) {
		return false
	}
	committed, err := repo.RecordAgentDeliveryWorkflowOutcome(ctx, task.ID, &guarded)
	if err != nil {
		s.logger.Warn("failed to persist guarded workflow no-op for delivery terminal",
			zap.String("task_id", task.ID), zap.String("effect_key", effect.EffectKey), zap.Error(err))
		return false
	}
	if !committed {
		s.logger.Debug("workflow source changed before delivery no-op receipt; terminal remains protected",
			zap.String("task_id", task.ID), zap.String("effect_key", effect.EffectKey))
	}
	return committed
}

func (s *Service) bindDeliveryWorkflowPolicy(ctx context.Context, task *models.Task, guarded *models.AgentDeliveryEffect) bool {
	if guarded.WorkflowID != task.WorkflowID || guarded.WorkflowStepID != task.WorkflowStepID {
		guarded.WorkflowID = task.WorkflowID
		guarded.WorkflowStepID = task.WorkflowStepID
		guarded.WorkflowPolicyUpdatedAt = nil
	}
	if task.WorkflowStepID == "" {
		guarded.WorkflowPolicyUpdatedAt = nil
		return true
	}
	if guarded.WorkflowPolicyUpdatedAt != nil {
		return true
	}
	if s.workflowStepGetter == nil {
		return false
	}
	step, err := s.workflowStepGetter.GetStep(ctx, task.WorkflowStepID)
	if err != nil || step == nil || step.WorkflowID != task.WorkflowID {
		s.logger.Warn("leaving delivery terminal pending because workflow policy could not be loaded",
			zap.String("task_id", task.ID), zap.String("step_id", task.WorkflowStepID), zap.Error(err))
		return false
	}
	updatedAt := step.UpdatedAt
	guarded.WorkflowPolicyUpdatedAt = &updatedAt
	return true
}

func (s *Service) recordNotRequiredDeliveryWorkflowOutcome(
	ctx context.Context,
	taskID string,
	effect *models.AgentDeliveryEffect,
) error {
	if effect == nil || !agentDeliveryEffectCarriesDeliveryIdentity(effect) {
		return nil
	}
	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil {
		return fmt.Errorf("load task for non-workflow delivery outcome: %w", err)
	}
	if !s.recordDeliveryWorkflowNoOp(ctx, task, effect, "not_required") {
		return errors.New("guarded non-workflow delivery outcome was not committed")
	}
	return nil
}

func (s *Service) settleFailedDeliveryWorkflowOutcome(ctx context.Context, data watcher.AgentEventData) {
	if data.DeliveryOutcome != string(models.DeliverySubmissionFailed) {
		return
	}
	effect := workflowEffectForEvent(data)
	if !agentDeliveryEffectCarriesDeliveryIdentity(effect) {
		return
	}
	if err := s.recordNotRequiredDeliveryWorkflowOutcome(ctx, data.TaskID, effect); err != nil {
		s.logger.Warn("failed to record failed delivery workflow no-op",
			zap.String("task_id", data.TaskID), zap.String("session_id", data.SessionID), zap.Error(err))
		return
	}
	if err := s.reconcileAgentDeliverySettlements(ctx, data.SessionID); err != nil {
		s.logger.Warn("failed to settle failed agent delivery terminal",
			zap.String("task_id", data.TaskID), zap.String("session_id", data.SessionID), zap.Error(err))
	}
}

func agentDeliveryEffectCarriesDeliveryIdentity(effect *models.AgentDeliveryEffect) bool {
	// SessionID also scopes legacy workflow receipts. Journal-specific fields
	// identify effects that require the complete durable delivery binding.
	return effect != nil && (effect.StreamID != "" || effect.Sequence != 0 ||
		effect.IncarnationID != "" || effect.HarnessGeneration != 0 || effect.SubmissionID != "")
}

func taskIDOrEmpty(task *models.Task) string {
	if task == nil {
		return ""
	}
	return task.ID
}
