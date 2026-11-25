package plugins

import (
	"context"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"go.uber.org/zap"
)

const (
	pluginIDPayloadKey        = "plugin_id"
	pluginStateWorkspaceScope = "workspace"
)

func (h *pluginHost) notifySidebarGroupingState(ctx context.Context, scope, scopeID string) {
	if !h.hasSidebarGroupings || h.bus == nil {
		return
	}
	payload := map[string]any{pluginIDPayloadKey: h.pluginID}
	if scope == pluginStateWorkspaceScope {
		payload["workspace_id"] = scopeID
	}
	event := bus.NewEvent(events.PluginSidebarGroupingUpdated, "plugin:"+h.pluginID, payload)
	if err := h.bus.Publish(ctx, events.PluginSidebarGroupingUpdated, event); err != nil && h.log != nil {
		h.log.Warn("sidebar grouping invalidation failed", zap.Error(err))
	}
}

func (s *Service) notifySidebarGroupingCatalog(pluginID string) {
	if pluginID != "" {
		s.mu.Lock()
		s.advanceSidebarGroupingCatalogLocked(pluginID)
		s.mu.Unlock()
	}
	s.publishSidebarGroupingCatalog(pluginID)
}

func (s *Service) advanceSidebarGroupingCatalogLocked(pluginID string) {
	if s.sidebarGroupingCatalogGenerations == nil {
		s.sidebarGroupingCatalogGenerations = make(map[string]uint64)
	}
	s.sidebarGroupingCatalogGenerations[pluginID]++
}

func (s *Service) publishSidebarGroupingCatalog(pluginID string) {
	payload := map[string]any{"catalog": true}
	if pluginID != "" {
		payload[pluginIDPayloadKey] = pluginID
	}
	if s.eventBus == nil {
		return
	}
	event := bus.NewEvent(events.PluginSidebarGroupingUpdated, "plugins", payload)
	if err := s.eventBus.Publish(context.Background(), events.PluginSidebarGroupingUpdated, event); err != nil {
		s.log.Warn("sidebar grouping catalog invalidation failed", zap.Error(err))
	}
}
