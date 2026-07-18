package service

import (
	"context"
	"errors"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"

	"go.uber.org/zap"
)

type taskDetachmentRepository interface {
	DetachTask(ctx context.Context, taskID string) (bool, error)
}

// DetachTask promotes a task to a root while preserving its existing shared
// workspace. Repeated calls for an already-root task are successful no-ops.
func (s *Service) DetachTask(ctx context.Context, id string) (*models.Task, error) {
	repo, ok := s.tasks.(taskDetachmentRepository)
	if !ok {
		return nil, errors.New("task repository does not support detachment")
	}
	if _, err := repo.DetachTask(ctx, id); err != nil {
		s.logger.Error("failed to detach task", zap.String("task_id", id), zap.Error(err))
		return nil, err
	}

	task, err := s.GetTask(ctx, id)
	if err != nil {
		return nil, err
	}

	var extra map[string]interface{}
	if task.ParentID == "" {
		extra = map[string]interface{}{"parent_id": nil}
	}
	s.publishTaskEventWithExtra(ctx, events.TaskUpdated, task, nil, extra)
	s.publishDetachOfficeEvent(ctx, task)
	return task, nil
}

func (s *Service) publishDetachOfficeEvent(ctx context.Context, task *models.Task) {
	if s.eventBus == nil {
		return
	}
	data := map[string]interface{}{
		"task_id":      task.ID,
		"workspace_id": task.WorkspaceID,
		"fields":       []string{"parent_id", "metadata"},
	}
	event := bus.NewEvent(events.OfficeTaskUpdated, "task-service", data)
	if err := s.eventBus.Publish(ctx, events.OfficeTaskUpdated, event); err != nil {
		s.logger.Error("failed to publish Office task detach event",
			zap.String("task_id", task.ID), zap.Error(err))
	}
}
