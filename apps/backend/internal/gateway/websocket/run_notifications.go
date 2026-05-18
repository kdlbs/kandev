package websocket

import (
	"context"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

// RunEventBroadcaster subscribes to office_run_events bus notifications
// using a per-subject wildcard, then fans the event out to every WS
// client subscribed to that specific run id via run.subscribe.
//
// Why a wildcard rather than per-run subscriptions? The set of "live"
// runs changes frequently and we don't want to add/remove bus
// subscriptions per WS subscriber — one wildcard subscription with a
// run-id-keyed broadcast on top of it is simpler and matches how the
// other gateway broadcasters operate.
type RunEventBroadcaster struct {
	hub          *Hub
	subscription bus.Subscription
	logger       *logger.Logger
}

// RegisterRunNotifications creates a RunEventBroadcaster and wires it
// to the event bus. The broadcaster is cleaned up when ctx is cancelled.
func RegisterRunNotifications(
	ctx context.Context, eventBus bus.EventBus, hub *Hub, log *logger.Logger,
) *RunEventBroadcaster {
	b := &RunEventBroadcaster{
		hub:    hub,
		logger: log.WithFields(zap.String("component", "ws-run-broadcaster")),
	}
	if eventBus == nil {
		return b
	}

	sub, err := eventBus.Subscribe(events.BuildOfficeRunEventWildcardSubject(), b.handle)
	if err != nil {
		b.logger.Error("failed to subscribe to run-event wildcard", zap.Error(err))
		return b
	}
	b.subscription = sub

	go func() {
		<-ctx.Done()
		b.Close()
	}()

	return b
}

// Close unsubscribes the wildcard subscription.
func (b *RunEventBroadcaster) Close() {
	if b.subscription == nil {
		return
	}
	if b.subscription.IsValid() {
		_ = b.subscription.Unsubscribe()
	}
	b.subscription = nil
}

// handle pulls run_id out of the event payload and broadcasts the
// run.event.appended action to clients subscribed to that run.
func (b *RunEventBroadcaster) handle(_ context.Context, event *bus.Event) error {
	payload, ok := event.Data.(map[string]interface{})
	if !ok {
		return nil
	}
	runID, _ := payload["run_id"].(string)
	if runID == "" {
		return nil
	}
	msg, err := ws.NewNotification(ws.ActionRunEventAppended, payload)
	if err != nil {
		b.logger.Error("failed to build run-event notification", zap.Error(err))
		return nil
	}
	b.hub.BroadcastToRun(runID, msg)
	return nil
}
