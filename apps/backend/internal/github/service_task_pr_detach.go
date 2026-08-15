package github

import (
	"context"
	"errors"
	"strings"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"go.uber.org/zap"
)

var ErrTaskPRNotFound = errors.New("github task PR not found")

// TaskPRDeletedEvent identifies one task-PR association removed from active
// task surfaces. The remote GitHub pull request is intentionally untouched.
type TaskPRDeletedEvent struct {
	WorkspaceID   string `json:"workspace_id"`
	TaskID        string `json:"task_id"`
	AssociationID string `json:"association_id"`
}

// GetWorkspaceID lets the websocket broadcaster route the typed event to the
// owning workspace instead of treating it as an instance-wide notification.
func (e TaskPRDeletedEvent) GetWorkspaceID() string { return e.WorkspaceID }

// DetachTaskPR removes one association from active task views while retaining
// a durable tombstone that prevents background discovery from resurrecting it.
func (s *Service) DetachTaskPR(ctx context.Context, workspaceID, associationID string) (*TaskPR, error) {
	if strings.TrimSpace(workspaceID) == "" || strings.TrimSpace(associationID) == "" {
		return nil, ErrTaskPRNotFound
	}
	tp, err := s.store.GetTaskPRByID(ctx, associationID)
	if err != nil {
		return nil, err
	}
	if tp == nil {
		return nil, ErrTaskPRNotFound
	}
	resolvedWorkspaceID, err := s.authorizeTaskPRMutation(ctx, tp, workspaceID)
	if err != nil {
		return nil, err
	}
	if tp.DetachedAt != nil {
		return tp, nil
	}
	detached, transitioned, err := s.store.DetachTaskPR(ctx, associationID)
	if err != nil {
		return nil, err
	}
	if detached == nil {
		return nil, ErrTaskPRNotFound
	}
	if !transitioned {
		return detached, nil
	}
	if s.eventBus != nil {
		event := bus.NewEvent(events.GitHubTaskPRDeleted, "github", &TaskPRDeletedEvent{
			WorkspaceID:   resolvedWorkspaceID,
			TaskID:        detached.TaskID,
			AssociationID: detached.ID,
		})
		if err := s.eventBus.Publish(ctx, events.GitHubTaskPRDeleted, event); err != nil {
			s.logger.Debug("failed to publish task PR deleted event", zap.Error(err))
		}
	}
	return detached, nil
}
