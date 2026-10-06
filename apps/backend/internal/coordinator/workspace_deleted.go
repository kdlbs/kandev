package coordinator

import (
	"context"
	"fmt"
	"strings"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

// SubscribeWorkspaceDeleted subscribes svc to workspace.deleted events,
// deleting the workspace's coordinators, proposals and stall rows in one
// transaction (docs/specs/coordinator/system-design/
// coordinators.md#workspace-deletion). It publishes nothing.
func SubscribeWorkspaceDeleted(eventBus bus.EventBus, svc *Service, log *logger.Logger) (bus.Subscription, error) {
	h := &workspaceDeletedSubscriber{
		svc:    svc,
		logger: log.WithFields(zap.String("component", "coordinator-workspace-deleted-subscriber")),
	}
	return eventBus.Subscribe(events.WorkspaceDeleted, h.handle)
}

type workspaceDeletedSubscriber struct {
	svc    *Service
	logger *logger.Logger
}

func (h *workspaceDeletedSubscriber) handle(ctx context.Context, event *bus.Event) error {
	workspaceID := workspaceIDFromDeletedEvent(event)
	if workspaceID == "" {
		return nil
	}
	if err := h.svc.store.DeleteWorkspaceState(ctx, workspaceID); err != nil {
		return fmt.Errorf("delete coordinator state for workspace %q: %w", workspaceID, err)
	}
	return nil
}

// workspaceIDFromDeletedEvent reads the workspace id from the payload's "id"
// key (Adoption decision 7), mirroring internal/github's
// workspaceIDFromEvent.
func workspaceIDFromDeletedEvent(event *bus.Event) string {
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
