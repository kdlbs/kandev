package service

import (
	"context"
	"time"

	"github.com/kandev/kandev/internal/orchestrate/models"

	"go.uber.org/zap"
)

// RecordCostEvent calculates cost from token usage and stores a cost event.
// If the model is not in the pricing table, cost_cents is 0.
func (s *Service) RecordCostEvent(
	ctx context.Context,
	sessionID, taskID, agentInstanceID, projectID string,
	model, provider string,
	tokensIn, tokensCachedIn, tokensOut int,
) (*models.CostEvent, error) {
	costCents := 0
	pricing, found := GetModelPricing(model)
	if found {
		costCents = CalculateCostCents(tokensIn, tokensCachedIn, tokensOut, pricing)
	}

	event := &models.CostEvent{
		SessionID:       sessionID,
		TaskID:          taskID,
		AgentInstanceID: agentInstanceID,
		ProjectID:       projectID,
		Model:           model,
		Provider:        provider,
		TokensIn:        tokensIn,
		TokensCachedIn:  tokensCachedIn,
		TokensOut:       tokensOut,
		CostCents:       costCents,
		OccurredAt:      time.Now().UTC(),
	}

	if err := s.repo.CreateCostEvent(ctx, event); err != nil {
		return nil, err
	}

	s.logger.Info("cost event recorded",
		zap.String("session_id", sessionID),
		zap.String("model", model),
		zap.Int("cost_cents", costCents),
		zap.Bool("pricing_found", found))

	return event, nil
}

// GetCostSummary returns total spend for a workspace.
func (s *Service) GetCostSummary(ctx context.Context, wsID string) (int, error) {
	events, err := s.repo.ListCostEvents(ctx, wsID)
	if err != nil {
		return 0, err
	}
	total := 0
	for _, e := range events {
		total += e.CostCents
	}
	return total, nil
}
