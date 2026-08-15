package gitlab

import (
	"context"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/events/bus"
)

func (s *Service) handleTaskDeleted(ctx context.Context, event *bus.Event) error {
	if event == nil {
		return nil
	}
	payload, ok := event.Data.(map[string]interface{})
	if !ok {
		return nil
	}
	taskID, _ := payload["task_id"].(string)
	if taskID == "" {
		return nil
	}
	store := s.requireStore()
	if store == nil {
		return nil
	}
	n, err := store.DeleteTaskOwnedStateByTaskID(ctx, taskID)
	if err != nil {
		s.logger.Error("failed to delete task-owned GitLab state", zap.String("task_id", taskID), zap.Error(err))
	} else if n > 0 {
		s.logger.Info("pruned task-owned GitLab state after task deletion", zap.String("task_id", taskID), zap.Int64("deleted", n))
	}
	return nil
}
