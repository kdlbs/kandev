package plugins

import (
	"context"
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

// subscribeWorkspaceDeletedEvents removes plugin-reported change-request rows
// when their workspace is deleted. The table has no database foreign key or
// cascade, so this handler owns the cleanup.
func (s *Service) subscribeWorkspaceDeletedEvents() {
	if s == nil || s.eventBus == nil {
		return
	}
	s.mu.Lock()
	if s.workspaceDeletedSubscription != nil {
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()
	subscription, err := s.eventBus.Subscribe(events.WorkspaceDeleted, s.handleWorkspaceDeleted)
	if err != nil {
		if s.log != nil {
			s.log.Warn("failed to subscribe plugin change requests to workspace deletion")
		}
		return
	}
	s.mu.Lock()
	if s.workspaceDeletedSubscription == nil {
		s.workspaceDeletedSubscription = subscription
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()
	_ = subscription.Unsubscribe()
}

func (s *Service) closeWorkspaceDeletedEvents() {
	if s == nil {
		return
	}
	s.mu.Lock()
	subscription := s.workspaceDeletedSubscription
	s.workspaceDeletedSubscription = nil
	s.mu.Unlock()
	if subscription != nil {
		_ = subscription.Unsubscribe()
	}
}

func (s *Service) handleWorkspaceDeleted(ctx context.Context, event *bus.Event) error {
	workspaceID := pluginEventResourceID(event)
	if workspaceID == "" {
		return nil
	}
	ledger := s.changeRequestStoreDep()
	if ledger == nil {
		return nil
	}
	if err := ledger.DeleteByWorkspace(ctx, workspaceID); err != nil {
		return fmt.Errorf("delete plugin task change requests for workspace %q: %w", workspaceID, err)
	}
	return nil
}

func pluginEventResourceID(event *bus.Event) string {
	if event == nil {
		return ""
	}
	data, ok := event.Data.(map[string]interface{})
	if !ok {
		return ""
	}
	value, _ := data["id"].(string)
	return strings.TrimSpace(value)
}
