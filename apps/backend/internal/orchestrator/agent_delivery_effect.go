package orchestrator

import (
	"context"
	"errors"
	"time"

	"go.uber.org/zap"

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
		targetStepID string,
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

func workflowEffectForTurn(turnID string) *models.AgentDeliveryEffect {
	if turnID == "" {
		return nil
	}
	now := time.Now().UTC()
	return &models.AgentDeliveryEffect{
		EffectKey:   "workflow.on_turn_complete:" + turnID,
		EffectType:  "workflow.on_turn_complete",
		State:       models.DeliveryEffectCompleted,
		CreatedAt:   now,
		CompletedAt: &now,
	}
}

// workflowEffectAlreadyApplied is deliberately fail-closed once the durable
// effect store is available. A read failure cannot safely distinguish a first
// delivery from a replay, so the caller must not evaluate another workflow
// transition in that case.
func (s *Service) workflowEffectAlreadyApplied(ctx context.Context, effect *models.AgentDeliveryEffect) bool {
	if effect == nil {
		return false
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
	return stored != nil && stored.State == models.DeliveryEffectCompleted
}
