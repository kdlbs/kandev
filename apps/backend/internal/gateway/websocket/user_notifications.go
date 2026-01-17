package websocket

import (
	"context"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

type UserEventBroadcaster struct {
	hub           *Hub
	subscriptions []bus.Subscription
	logger        *logger.Logger
}

func RegisterUserNotifications(ctx context.Context, eventBus bus.EventBus, hub *Hub, log *logger.Logger) *UserEventBroadcaster {
	b := &UserEventBroadcaster{
		hub:    hub,
		logger: log.WithFields(zap.String("component", "ws-user-broadcaster")),
	}
	if eventBus == nil {
		return b
	}

	b.subscribe(eventBus, events.UserSettingsUpdated, ws.ActionUserSettingsUpdated)

	go func() {
		<-ctx.Done()
		b.Close()
	}()

	return b
}

func (b *UserEventBroadcaster) Close() {
	for _, sub := range b.subscriptions {
		if sub != nil && sub.IsValid() {
			_ = sub.Unsubscribe()
		}
	}
	b.subscriptions = nil
}

func (b *UserEventBroadcaster) subscribe(eventBus bus.EventBus, subject, action string) {
	sub, err := eventBus.Subscribe(subject, func(ctx context.Context, event *bus.Event) error {
		// Try to extract user_id from event data (works for both map and struct types)
		var userID string
		if data, ok := event.Data.(map[string]interface{}); ok {
			userID, _ = data["user_id"].(string)
		} else if data, ok := event.Data.(interface{ GetUserID() string }); ok {
			userID = data.GetUserID()
		}
		if userID == "" {
			return nil
		}
		msg, err := ws.NewNotification(action, event.Data)
		if err != nil {
			b.logger.Error("failed to build websocket notification", zap.String("action", action), zap.Error(err))
			return nil
		}
		b.hub.BroadcastToUser(userID, msg)
		return nil
	})
	if err != nil {
		b.logger.Error("failed to subscribe to events", zap.String("subject", subject), zap.Error(err))
		return
	}
	b.subscriptions = append(b.subscriptions, sub)
}
