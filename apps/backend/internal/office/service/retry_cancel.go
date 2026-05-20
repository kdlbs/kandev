package service

import (
	"context"

	"go.uber.org/zap"
)

// CancelPendingRetriesForTask cancels all queued retry runs for the given task.
func (s *Service) CancelPendingRetriesForTask(ctx context.Context, taskID string) error {
	pending, err := s.repo.ListPendingRunsForTask(ctx, taskID)
	if err != nil {
		return err
	}
	if len(pending) == 0 {
		return nil
	}

	ids := make([]string, len(pending))
	for i, w := range pending {
		ids[i] = w.ID
	}

	s.logger.Info("cancelling pending retries for reassigned task",
		zap.String("task_id", taskID),
		zap.Int("count", len(ids)))

	return s.repo.BulkCancelRuns(ctx, ids, "task_reassigned")
}
