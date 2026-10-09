package websocket

import (
	"context"
	"sync"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

// CoordinatorEventBroadcaster forwards events.CoordinatorUpdated to the
// owning workspace's subscribers (coordinators.md#flag-and-wiring, Build
// decision 13). Publishing sites land with later work packages; this
// forwarder only relays whatever is published.
type CoordinatorEventBroadcaster struct {
	hub          *Hub
	subscription bus.Subscription
	logger       *logger.Logger
	closeMu      sync.Mutex
	closed       bool
}

// RegisterCoordinatorNotifications wires events.CoordinatorUpdated to the
// existing workspace-scoped WebSocket hub. Callers must only invoke it when
// features.coordinator is enabled.
func RegisterCoordinatorNotifications(ctx context.Context, eventBus bus.EventBus, hub *Hub, log *logger.Logger) *CoordinatorEventBroadcaster {
	b := &CoordinatorEventBroadcaster{
		hub:    hub,
		logger: log.WithFields(zap.String("component", "ws-coordinator-broadcaster")),
	}
	if eventBus == nil || hub == nil {
		return b
	}

	b.subscribe(eventBus)

	go func() {
		<-ctx.Done()
		b.Close()
	}()
	return b
}

// Close releases the event-bus subscription. It is safe to call more than
// once.
func (b *CoordinatorEventBroadcaster) Close() {
	b.closeMu.Lock()
	if b.closed {
		b.closeMu.Unlock()
		return
	}
	b.closed = true
	sub := b.subscription
	b.subscription = nil
	b.closeMu.Unlock()

	if sub != nil && sub.IsValid() {
		_ = sub.Unsubscribe()
	}
}

func (b *CoordinatorEventBroadcaster) subscribe(eventBus bus.EventBus) {
	sub, err := eventBus.Subscribe(events.CoordinatorUpdated, func(ctx context.Context, event *bus.Event) error {
		msg, err := ws.NewNotification(ws.ActionCoordinatorUpdated, event.Data)
		if err != nil {
			b.logger.Error("failed to build coordinator websocket notification", zap.Error(err))
			return nil
		}
		workspaceID := extractWorkspaceID(event.Data)
		// coordinator.updated always carries workspace_id (Build decision 13).
		// Dropping an unattributed event under auth is safer than fanning it
		// out globally.
		b.hub.BroadcastToWorkspaceOrDrop(workspaceID, msg)
		return nil
	})
	if err != nil {
		b.logger.Error("failed to subscribe to coordinator events", zap.String("subject", events.CoordinatorUpdated), zap.Error(err))
		return
	}
	b.closeMu.Lock()
	if b.closed {
		b.closeMu.Unlock()
		if sub.IsValid() {
			_ = sub.Unsubscribe()
		}
		return
	}
	b.subscription = sub
	b.closeMu.Unlock()
}
