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
// Subscriptions are tracked so they can be released via unsubscribeTaskEvents.
func (s *Service) subscribeTaskEvents() {
	if s.eventBus == nil {
		return
	}
	if sub, err := s.eventBus.Subscribe(events.TaskUpdated, s.handleTaskUpdated); err != nil {
		s.logger.Error("failed to subscribe to task.updated events", zap.Error(err))
	} else {
		s.taskEventSubs = append(s.taskEventSubs, sub)
	}
	if sub, err := s.eventBus.Subscribe(events.TaskDeleted, s.handleTaskDeleted); err != nil {
		s.logger.Error("failed to subscribe to task.deleted events", zap.Error(err))
	} else {
		s.taskEventSubs = append(s.taskEventSubs, sub)
	}
}

// unsubscribeTaskEvents releases the subscriptions created in subscribeTaskEvents.
// Called from the provider cleanup so handlers don't accumulate if the service is
// torn down and re-created while the event bus persists.
func (s *Service) unsubscribeTaskEvents() {
	for _, sub := range s.taskEventSubs {
		if err := sub.Unsubscribe(); err != nil {
			s.logger.Error("failed to unsubscribe from task event", zap.Error(err))
		}
	}
	s.taskEventSubs = nil
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
// internal/task/service/service_events.go publishTaskEvent). Archived is
// detected from a non-empty string value so a future null/zero archived_at
// in the payload doesn't silently prune watches on non-archive updates.
func taskIDAndArchivedFrom(event *bus.Event) (taskID string, archived bool) {
	if event == nil {
		return "", false
	}
	data, ok := event.Data.(map[string]interface{})
	if !ok {
		return "", false
	}
	id, _ := data["task_id"].(string)
	archivedStr, _ := data["archived_at"].(string)
	return id, archivedStr != ""
}
