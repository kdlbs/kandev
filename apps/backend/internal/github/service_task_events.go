package github

import (
	"context"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

// subscribeTaskEvents wires the github service to task archive/delete events so
// PR watches are pruned proactively. The ListActivePRWatches query already hides
// archived-task watches from the poller, but pruning keeps the table bounded.
func (s *Service) subscribeTaskEvents() {
	if s.eventBus == nil {
		return
	}
	if _, err := s.eventBus.Subscribe(events.TaskUpdated, s.handleTaskUpdated); err != nil {
		s.logger.Error("failed to subscribe to task.updated events", zap.Error(err))
	}
	if _, err := s.eventBus.Subscribe(events.TaskDeleted, s.handleTaskDeleted); err != nil {
		s.logger.Error("failed to subscribe to task.deleted events", zap.Error(err))
	}
}

// handleTaskUpdated deletes PR watches when a task is archived. Non-archive
// updates are ignored so we don't interfere with normal task edits.
func (s *Service) handleTaskUpdated(ctx context.Context, event *bus.Event) error {
	taskID, archived := taskIDAndArchivedFrom(event)
	if taskID == "" || !archived {
		return nil
	}
	s.pruneWatchesForTask(ctx, taskID, "archived")
	return nil
}

// handleTaskDeleted deletes PR watches when a task is hard-deleted.
func (s *Service) handleTaskDeleted(ctx context.Context, event *bus.Event) error {
	taskID, _ := taskIDAndArchivedFrom(event)
	if taskID == "" {
		return nil
	}
	s.pruneWatchesForTask(ctx, taskID, "deleted")
	return nil
}

func (s *Service) pruneWatchesForTask(ctx context.Context, taskID, reason string) {
	n, err := s.store.DeletePRWatchesByTaskID(ctx, taskID)
	if err != nil {
		s.logger.Error("failed to delete PR watches for task",
			zap.String("task_id", taskID),
			zap.String("reason", reason),
			zap.Error(err))
		return
	}
	if n > 0 {
		s.logger.Info("pruned PR watches after task change",
			zap.String("task_id", taskID),
			zap.String("reason", reason),
			zap.Int64("deleted", n))
	}
}

// taskIDAndArchivedFrom extracts task_id + archived flag from a task event
// payload (task-service publishes a map[string]interface{} — see
// internal/task/service/service_events.go publishTaskEvent).
func taskIDAndArchivedFrom(event *bus.Event) (taskID string, archived bool) {
	if event == nil {
		return "", false
	}
	data, ok := event.Data.(map[string]interface{})
	if !ok {
		return "", false
	}
	id, _ := data["task_id"].(string)
	_, archived = data["archived_at"]
	return id, archived
}
