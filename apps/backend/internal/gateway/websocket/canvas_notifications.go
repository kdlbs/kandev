package websocket

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

// CanvasEventBroadcaster forwards the content-free canvas lifecycle events to
// the owner of the canvas workspace. Application data belongs on the
// capability-bound SSE protocol, not on this shared WebSocket channel.
type CanvasEventBroadcaster struct {
	hub           *Hub
	subscriptions []bus.Subscription
	logger        *logger.Logger
	closeMu       sync.Mutex
	closed        bool
}

// RegisterCanvasNotifications wires the gated canvas lifecycle events to the
// existing workspace-scoped WebSocket hub. Callers must only invoke it when
// features.canvases is enabled.
func RegisterCanvasNotifications(ctx context.Context, eventBus bus.EventBus, hub *Hub, log *logger.Logger) *CanvasEventBroadcaster {
	b := &CanvasEventBroadcaster{
		hub:    hub,
		logger: log.WithFields(zap.String("component", "ws-canvas-broadcaster")),
	}
	if eventBus == nil || hub == nil {
		return b
	}

	for _, event := range []struct {
		subject string
		action  string
	}{
		{events.CanvasCreated, ws.ActionCanvasCreated},
		{events.CanvasReleaseActivated, ws.ActionCanvasReleaseActivated},
		{events.CanvasReleasePermissionRequired, ws.ActionCanvasReleasePermissionRequired},
		{events.CanvasPromoted, ws.ActionCanvasPromoted},
		{events.CanvasArchived, ws.ActionCanvasArchived},
		{events.CanvasRestored, ws.ActionCanvasRestored},
		{events.CanvasRemoved, ws.ActionCanvasRemoved},
	} {
		b.subscribe(eventBus, event.subject, event.action)
	}

	go func() {
		<-ctx.Done()
		b.Close()
	}()
	return b
}

// Close releases event-bus subscriptions. It is safe to call more than once.
func (b *CanvasEventBroadcaster) Close() {
	b.closeMu.Lock()
	if b.closed {
		b.closeMu.Unlock()
		return
	}
	b.closed = true
	subscriptions := b.subscriptions
	b.subscriptions = nil
	b.closeMu.Unlock()

	for _, sub := range subscriptions {
		if sub != nil && sub.IsValid() {
			_ = sub.Unsubscribe()
		}
	}
}

func (b *CanvasEventBroadcaster) subscribe(eventBus bus.EventBus, subject, action string) {
	sub, err := eventBus.Subscribe(subject, func(ctx context.Context, event *bus.Event) error {
		msg, err := ws.NewNotification(action, event.Data)
		if err != nil {
			b.logger.Error("failed to build canvas websocket notification", zap.String("action", action), zap.Error(err))
			return nil
		}
		workspaceID := canvasWorkspaceID(event.Data)
		// Canvas events are always workspace-scoped. Dropping an unattributed
		// event under auth is safer than allowing it to fan out globally.
		b.hub.BroadcastToWorkspaceOrDrop(workspaceID, msg)
		return nil
	})
	if err != nil {
		b.logger.Error("failed to subscribe to canvas events", zap.String("subject", subject), zap.Error(err))
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
	b.subscriptions = append(b.subscriptions, sub)
	b.closeMu.Unlock()
}

func canvasWorkspaceID(data interface{}) string {
	if workspaceID := extractWorkspaceID(data); workspaceID != "" {
		return workspaceID
	}
	// Memory-event callers often pass a typed LifecycleEvent while NATS
	// callers pass a JSON-shaped map. Normalize the former without making the
	// gateway depend on the canvas package's concrete event type.
	encoded, err := json.Marshal(data)
	if err != nil {
		return ""
	}
	var fields map[string]interface{}
	if err := json.Unmarshal(encoded, &fields); err != nil {
		return ""
	}
	return extractWorkspaceID(fields)
}
