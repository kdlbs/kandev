package websocket

import (
	"context"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

func (b *SystemEventBroadcaster) subscribeSidebarGrouping(eventBus bus.EventBus) {
	sub, err := eventBus.Subscribe(events.PluginSidebarGroupingUpdated, func(_ context.Context, event *bus.Event) error {
		data, ok := event.Data.(map[string]any)
		if !ok {
			return nil
		}
		payload := map[string]any{}
		catalog, _ := data["catalog"].(bool)
		pluginID, _ := data["plugin_id"].(string)
		switch {
		case catalog:
			payload["catalog"] = true
			if pluginID != "" {
				payload["plugin_id"] = pluginID
			}
		case pluginID != "":
			payload["plugin_id"] = pluginID
		default:
			return nil
		}
		workspaceID := extractWorkspaceID(data)
		if !catalog && workspaceID != "" {
			payload["workspace_id"] = workspaceID
		}
		msg, err := ws.NewNotification(ws.ActionPluginSidebarGroupingUpdated, payload)
		if err != nil {
			return err
		}
		if !catalog && workspaceID != "" {
			b.hub.BroadcastToWorkspaceOrDrop(workspaceID, msg)
		} else {
			//ws:global install-wide provider/catalog identity only; never state values or workspace identifiers.
			b.hub.Broadcast(msg)
		}
		return nil
	})
	if err != nil {
		b.logger.Error("failed to subscribe to sidebar grouping invalidation", zap.Error(err))
		return
	}
	b.subscriptions = append(b.subscriptions, sub)
}
