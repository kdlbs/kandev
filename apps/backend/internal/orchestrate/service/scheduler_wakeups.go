package service

import (
	"context"
	"database/sql"
	"errors"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/orchestrate/models"
)

// ClaimNextWakeup atomically claims the next eligible wakeup from the queue.
// Returns nil, nil if no wakeup is available.
func (s *Service) ClaimNextWakeup(ctx context.Context) (*models.WakeupRequest, error) {
	req, err := s.repo.ClaimNextEligibleWakeup(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	s.logger.Info("wakeup claimed",
		zap.String("id", req.ID),
		zap.String("agent", req.AgentInstanceID),
		zap.String("reason", req.Reason))
	return req, nil
}

// FinishWakeup marks a claimed wakeup as finished.
func (s *Service) FinishWakeup(ctx context.Context, id string) error {
	return s.repo.FinishWakeupRequest(ctx, id, WakeupStatusFinished)
}

// FailWakeup marks a claimed wakeup as failed.
func (s *Service) FailWakeup(ctx context.Context, id string) error {
	return s.repo.FinishWakeupRequest(ctx, id, WakeupStatusFailed)
}

// ProcessWakeupGuard checks if the agent is still eligible to be woken.
// Returns true if the wakeup should proceed, false if it should be skipped.
func (s *Service) ProcessWakeupGuard(_ context.Context, wakeup *models.WakeupRequest) (bool, error) {
	agent, err := s.GetAgentFromConfig(context.Background(), wakeup.AgentInstanceID)
	if err != nil {
		return false, err
	}
	switch agent.Status {
	case models.AgentStatusPaused, models.AgentStatusStopped, models.AgentStatusPendingApproval:
		s.logger.Info("wakeup skipped (agent not active)",
			zap.String("wakeup_id", wakeup.ID),
			zap.String("agent_status", string(agent.Status)))
		return false, nil
	}
	return true, nil
}
