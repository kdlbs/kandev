package websocket

import (
	"context"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

// SystemEventBroadcaster forwards system-maintenance job events
// (vacuum, factory reset, snapshot create/restore, disk walk) to every
// connected WebSocket client. These events are install-wide and not scoped
// to a user/session/task, so they use Hub.Broadcast.
type SystemEventBroadcaster struct {
	hub           *Hub
	subscriptions []bus.Subscription
	logger        *logger.Logger
}

// RegisterSystemNotifications wires the system event broadcaster onto the
// shared event bus + Hub. Unsubscribes when ctx is cancelled.
func RegisterSystemNotifications(ctx context.Context, eventBus bus.EventBus, hub *Hub, log *logger.Logger) *SystemEventBroadcaster {
	b := &SystemEventBroadcaster{
		hub:    hub,
		logger: log.WithFields(zap.String("component", "ws-system-broadcaster")),
	}
	if eventBus == nil {
		return b
	}

	b.subscribe(eventBus, events.SystemJobUpdate, ws.ActionSystemJobUpdate)

	go func() {
		<-ctx.Done()
		b.Close()
	}()

	return b
}

// Close unsubscribes from all event-bus subscriptions.
func (b *SystemEventBroadcaster) Close() {
	for _, sub := range b.subscriptions {
		if sub != nil && sub.IsValid() {
			_ = sub.Unsubscribe()
		}
	}
	b.subscriptions = nil
}

func (b *SystemEventBroadcaster) subscribe(eventBus bus.EventBus, subject, action string) {
	sub, err := eventBus.Subscribe(subject, func(_ context.Context, event *bus.Event) error {
		msg, err := ws.NewNotification(action, event.Data)
		if err != nil {
			b.logger.Error("failed to build websocket notification", zap.String("action", action), zap.Error(err))
			return nil
		}
		b.hub.Broadcast(msg)
		return nil
	})
	if err != nil {
		b.logger.Error("failed to subscribe to events", zap.String("subject", subject), zap.Error(err))
		return
	}
	b.subscriptions = append(b.subscriptions, sub)
}
