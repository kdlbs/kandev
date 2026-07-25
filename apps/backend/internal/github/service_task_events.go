package github

import (
	"context"
	"fmt"
	"strings"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

// subscribeTaskEvents wires task/workspace lifecycle cleanup. PR watches are
// pruned proactively, while task, session, and workspace credential leases are
// revoked as soon as their owning runtime scope terminates. Subscriptions are
// tracked so they can be released via unsubscribeTaskEvents.
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
	if sub, err := s.eventBus.Subscribe(events.WorkspaceDeleted, s.handleWorkspaceDeleted); err != nil {
		s.logger.Error("failed to subscribe to workspace.deleted events", zap.Error(err))
	} else {
		s.taskEventSubs = append(s.taskEventSubs, sub)
	}
	if sub, err := s.eventBus.Subscribe(events.TaskSessionStateChanged, s.handleTaskSessionStateChanged); err != nil {
		s.logger.Error("failed to subscribe to task session state events", zap.Error(err))
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
	s.revokeCredentialTask(taskID)
	s.pruneWatchesForTask(ctx, taskID, "archived")
	return nil
}

// handleTaskDeleted deletes PR watches when a task is hard-deleted.
func (s *Service) handleTaskDeleted(ctx context.Context, event *bus.Event) error {
	taskID, _ := taskIDAndArchivedFrom(event)
	if taskID == "" {
		return nil
	}
	s.revokeCredentialTask(taskID)
	s.pruneWatchesForTask(ctx, taskID, "deleted")
	return nil
}

func (s *Service) handleTaskSessionStateChanged(_ context.Context, event *bus.Event) error {
	sessionID, terminal := terminalSessionIDFrom(event)
	if sessionID == "" || !terminal {
		return nil
	}
	s.mu.Lock()
	broker := s.credentialBroker
	s.mu.Unlock()
	if broker != nil {
		broker.RevokeSession(sessionID)
	}
	return nil
}

type connectionSecretIDLister interface {
	ListIDs(context.Context) ([]string, error)
}

// handleWorkspaceDeleted removes encrypted GitHub credentials after the
// workspace-owned metadata rows have been deleted by the database cascade.
// Secret IDs are namespaced, so future multi-user connections are covered
// without reading deleted user-connection rows.
func (s *Service) handleWorkspaceDeleted(ctx context.Context, event *bus.Event) error {
	workspaceID := workspaceIDFromEvent(event)
	if workspaceID == "" {
		return nil
	}
	s.mu.Lock()
	broker := s.credentialBroker
	s.mu.Unlock()
	if broker != nil {
		broker.RevokeWorkspace(workspaceID)
	}
	if s.connectionSecrets == nil {
		return nil
	}
	keys := []string{WorkspacePATSecretKey(workspaceID)}
	if lister, ok := s.connectionSecrets.(connectionSecretIDLister); ok {
		ids, err := lister.ListIDs(ctx)
		if err != nil {
			return fmt.Errorf("list GitHub connection secrets: %w", err)
		}
		userPrefix := "github:user:" + workspaceID + ":"
		for _, id := range ids {
			if strings.HasPrefix(id, userPrefix) {
				keys = append(keys, id)
			}
		}
	} else {
		keys = append(keys,
			UserAccessTokenSecretKey(workspaceID, DefaultUserID),
			UserRefreshTokenSecretKey(workspaceID, DefaultUserID),
		)
	}
	for _, key := range keys {
		if err := deleteOptionalSecret(ctx, s.connectionSecrets, key); err != nil {
			return fmt.Errorf("delete GitHub connection secret %q: %w", key, err)
		}
	}
	s.invalidateWorkspaceCredential(workspaceID)
	return nil
}

func (s *Service) revokeCredentialTask(taskID string) {
	s.mu.Lock()
	broker := s.credentialBroker
	s.mu.Unlock()
	if broker != nil {
		broker.RevokeTask(taskID)
	}
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

func workspaceIDFromEvent(event *bus.Event) string {
	if event == nil {
		return ""
	}
	data, ok := event.Data.(map[string]interface{})
	if !ok {
		return ""
	}
	workspaceID, _ := data["id"].(string)
	return strings.TrimSpace(workspaceID)
}

func terminalSessionIDFrom(event *bus.Event) (string, bool) {
	if event == nil {
		return "", false
	}
	data, ok := event.Data.(map[string]interface{})
	if !ok {
		return "", false
	}
	sessionID, _ := data["session_id"].(string)
	state, _ := data["new_state"].(string)
	switch taskmodels.TaskSessionState(strings.ToUpper(strings.TrimSpace(state))) {
	case taskmodels.TaskSessionStateCompleted,
		taskmodels.TaskSessionStateFailed,
		taskmodels.TaskSessionStateCancelled:
		return strings.TrimSpace(sessionID), true
	default:
		return strings.TrimSpace(sessionID), false
	}
}
