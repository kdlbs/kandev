package orchestrator

import (
	"context"

	"github.com/kandev/kandev/internal/task/models"
)

const autoResumeBlockedExecutorFailure = "executor_failure"

type executorFailureAdmissionStore interface {
	GetActiveExecutorFailure(context.Context, models.ExecutorObservationTarget) (*models.ExecutorFailureEpisode, error)
}

func (s *Service) executorFailureRecoveryBlockReason(ctx context.Context, session *models.TaskSession) string {
	store, ok := s.repo.(executorFailureAdmissionStore)
	if !ok {
		return ""
	}
	target := models.ExecutorObservationTarget{TaskID: session.TaskID, SessionID: session.ID}
	if session.TaskEnvironmentID != "" {
		env, err := s.repo.GetTaskEnvironment(ctx, session.TaskEnvironmentID)
		if err != nil || env == nil || env.TaskID == "" || env.OwnershipGeneration == 0 {
			return autoResumeBlockedOwnershipUnavailable
		}
		target.TaskID, target.EnvironmentID, target.OwnershipGeneration = env.TaskID, env.ID, env.OwnershipGeneration
	}
	episode, err := store.GetActiveExecutorFailure(ctx, target)
	if err != nil {
		return autoResumeBlockedOwnershipUnavailable
	}
	if episode != nil {
		return autoResumeBlockedExecutorFailure
	}
	return ""
}
