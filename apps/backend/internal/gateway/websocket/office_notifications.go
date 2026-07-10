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
	// task.moved → office.task.moved is a WORKSPACE-scoped notification (clients
	// filter by workspace_id). The underlying task.moved event carries a
	// session_id for the orchestrator's on_exit/on_enter wiring, but no office
	// consumer reads it. Forwarding it verbatim makes the FE WS-account stamp
	// the envelope as session-routed, so the bridge audit then expects a
	// per-session cache mutation that an office handler legitimately never makes
	// on a non-office page. Drop the session-scoped fields from the re-broadcast.
	b.subscribeWithout(eventBus, events.TaskMoved, ws.ActionOfficeTaskMoved, "session_id")
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
	b.subscribeWithout(eventBus, subject, action)
}

// stripPayloadKeys returns the event payload with the named keys removed. When
// no keys are requested (or the payload isn't a map) the original value is
// returned unchanged. Always clones before deleting so the source event payload
// — shared with every other subscriber — is never mutated.
func stripPayloadKeys(data interface{}, dropKeys []string) interface{} {
	if len(dropKeys) == 0 {
		return data
	}
	m, ok := data.(map[string]interface{})
	if !ok {
		return data
	}
	cp := make(map[string]interface{}, len(m))
	for k, v := range m {
		cp[k] = v
	}
	for _, key := range dropKeys {
		delete(cp, key)
	}
	return cp
}

// subscribeWithout is like subscribe but strips the named payload keys from the
// re-broadcast notification. Office notifications are workspace-scoped (clients
// filter by workspace_id); session-scoped fields leaked from the source event
// would otherwise mis-classify the envelope as session-routed for accounting.
func (b *OfficeEventBroadcaster) subscribeWithout(eventBus bus.EventBus, subject, action string, dropKeys ...string) {
	sub, err := eventBus.Subscribe(subject, func(_ context.Context, event *bus.Event) error {
		data := stripPayloadKeys(event.Data, dropKeys)
		msg, err := ws.NewNotification(action, data)
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
