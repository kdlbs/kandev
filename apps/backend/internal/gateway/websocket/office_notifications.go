package websocket

import (
	"context"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

// OfficeEventBroadcaster subscribes to office domain events on the event bus
// and broadcasts them as WS notifications to all connected clients.
// Clients filter by workspace_id in the payload.
type OfficeEventBroadcaster struct {
	hub           *Hub
	subscriptions []bus.Subscription
	logger        *logger.Logger
}

// RegisterOfficeNotifications creates an OfficeEventBroadcaster, subscribes to
// all office-relevant events, and returns it. The broadcaster is cleaned up when
// ctx is cancelled.
func RegisterOfficeNotifications(ctx context.Context, eventBus bus.EventBus, hub *Hub, log *logger.Logger) *OfficeEventBroadcaster {
	b := &OfficeEventBroadcaster{
		hub:    hub,
		logger: log.WithFields(zap.String("component", "ws-office-broadcaster")),
	}
	if eventBus == nil {
		return b
	}

	// Task lifecycle events
	b.subscribe(eventBus, events.TaskUpdated, ws.ActionOfficeTaskUpdated)
	b.subscribe(eventBus, events.TaskCreated, ws.ActionOfficeTaskCreated)
	b.subscribe(eventBus, events.TaskMoved, ws.ActionOfficeTaskMoved)
	b.subscribe(eventBus, events.OfficeTaskStatusChanged, ws.ActionOfficeTaskStatus)
	b.subscribe(eventBus, events.OfficeTaskUpdated, ws.ActionOfficeTaskUpdated)
	b.subscribe(eventBus, events.OfficeTaskDecisionRecorded, ws.ActionOfficeTaskDecision)
	b.subscribe(eventBus, events.OfficeTaskReviewRequested, ws.ActionOfficeTaskReview)

	// Comment events
	b.subscribe(eventBus, events.OfficeCommentCreated, ws.ActionOfficeCommentCreated)

	// Agent lifecycle events
	b.subscribe(eventBus, events.AgentCompleted, ws.ActionOfficeAgentCompleted)
	b.subscribe(eventBus, events.AgentFailed, ws.ActionOfficeAgentFailed)
	b.subscribe(eventBus, events.OfficeAgentUpdated, ws.ActionOfficeAgentUpdated)

	// Approval events
	b.subscribe(eventBus, events.OfficeApprovalCreated, ws.ActionOfficeApprovalCreated)
	b.subscribe(eventBus, events.OfficeApprovalResolved, ws.ActionOfficeApprovalResolved)

	// Cost events
	b.subscribe(eventBus, events.OfficeCostRecorded, ws.ActionOfficeCostRecorded)

	// Scheduler events
	b.subscribe(eventBus, events.OfficeRunQueued, ws.ActionOfficeRunQueued)
	b.subscribe(eventBus, events.OfficeRunProcessed, ws.ActionOfficeRunProcessed)
	b.subscribe(eventBus, events.OfficeRoutineTriggered, ws.ActionOfficeRoutineTriggered)

	// Provider-routing events
	b.subscribe(eventBus, events.OfficeProviderHealthChanged, ws.ActionOfficeProviderHealthChanged)
	b.subscribe(eventBus, events.OfficeRouteAttemptAppended, ws.ActionOfficeRouteAttemptAppended)
	b.subscribe(eventBus, events.OfficeRoutingSettingsUpdated, ws.ActionOfficeRoutingSettingsUpdated)

	go func() {
		<-ctx.Done()
		b.Close()
	}()

	return b
}

func (b *OfficeEventBroadcaster) Close() {
	for _, sub := range b.subscriptions {
		if sub != nil && sub.IsValid() {
			_ = sub.Unsubscribe()
		}
	}
	b.subscriptions = nil
}

func (b *OfficeEventBroadcaster) subscribe(eventBus bus.EventBus, subject, action string) {
	sub, err := eventBus.Subscribe(subject, func(_ context.Context, event *bus.Event) error {
		msg, err := ws.NewNotification(action, event.Data)
		if err != nil {
			b.logger.Error("failed to build office ws notification",
				zap.String("action", action), zap.Error(err))
			return nil
		}
		// Broadcast to all clients — they filter by workspace_id in the payload.
		b.hub.Broadcast(msg)
		return nil
	})
	if err != nil {
		b.logger.Error("failed to subscribe to office event",
			zap.String("subject", subject), zap.Error(err))
		return
	}
	b.subscriptions = append(b.subscriptions, sub)
}
