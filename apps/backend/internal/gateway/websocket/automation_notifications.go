package websocket

import (
	"context"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

// automationRunCreatedFields are the only event fields forwarded to clients.
var automationRunCreatedFields = []string{"workspace_id", "automation_id", "run_id", "status"}

func (b *SystemEventBroadcaster) subscribeAutomationRunCreated(eventBus bus.EventBus) {
	sub, err := eventBus.Subscribe(events.AutomationRunCreated, func(_ context.Context, event *bus.Event) error {
		data, ok := event.Data.(map[string]any)
		if !ok {
			return nil
		}
		workspaceID := extractWorkspaceID(data)
		if workspaceID == "" {
			return nil
		}
		payload := make(map[string]any, len(automationRunCreatedFields))
		for _, field := range automationRunCreatedFields {
			if value, ok := data[field].(string); ok {
				payload[field] = value
			}
		}
		msg, err := ws.NewNotification(ws.ActionAutomationRunCreated, payload)
		if err != nil {
			return err
		}
		b.hub.BroadcastToWorkspaceOrDrop(workspaceID, msg)
		return nil
	})
	if err != nil {
		b.logger.Error("failed to subscribe to automation run notifications", zap.Error(err))
		return
	}
	b.subscriptions = append(b.subscriptions, sub)
}
