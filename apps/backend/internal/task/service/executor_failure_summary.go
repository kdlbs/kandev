package service

import (
	"context"
	"github.com/kandev/kandev/internal/task/models"
)

func (s *Service) executorFailureSummary(ctx context.Context, taskID string) (*models.ExecutorFailureEpisode, bool, error) {
	reader, ok := s.tasks.(interface {
		GetExecutorFailure(context.Context, string) (*models.ExecutorFailureEpisode, error)
	})
	if !ok {
		return nil, false, nil
	}
	failure, err := reader.GetExecutorFailure(ctx, taskID)
	return failure, true, err
}
