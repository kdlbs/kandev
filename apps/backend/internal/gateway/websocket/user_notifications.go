package websocket

import (
	"context"
	"encoding/json"

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
	b.subscribe(eventBus, events.UserAgentProfileRecentUseUpdated, ws.ActionUserAgentProfileRecentUseUpdated)
	b.subscribe(eventBus, events.PluginUserStateUpdated, ws.ActionPluginUserStateUpdated)

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
		userID, payload := routingUserIDAndPayload(event.Data)
		if userID == "" {
			return nil
		}
		msg, err := ws.NewNotification(action, payload)
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

// routingUserIDAndPayload resolves the user to route event data to, and the
// payload to send them with "user_id" stripped.
//
// event.Data arrives in one of two shapes depending on transport:
//   - MemoryEventBus delivers the original Go value in-process, so a
//     struct's exported fields are already there.
//   - a NATS-backed EventBus marshals every event before publishing and
//     JSON-decodes it into an untyped Data field on arrival
//     (internal/events/bus/nats.go), so a subscriber always sees a
//     map[string]interface{} — an unexported struct field is invisible to
//     that marshal and is silently gone by the time it gets here.
//
// Both shapes are normalized to a map (round-tripping a struct through JSON
// first) so every event type is read and routed the same way regardless of
// transport, and one strip covers both: "user_id" is a routing-only concern
// the client already knows (it's their own id) and must never appear in the
// message they actually receive.
func routingUserIDAndPayload(data interface{}) (userID string, payload map[string]interface{}) {
	raw, ok := data.(map[string]interface{})
	if !ok {
		encoded, err := json.Marshal(data)
		if err != nil {
			return "", nil
		}
		if err := json.Unmarshal(encoded, &raw); err != nil {
			return "", nil
		}
	}
	userID, _ = raw["user_id"].(string)
	if userID == "" {
		return "", nil
	}
	payload = make(map[string]interface{}, len(raw))
	for k, v := range raw {
		if k != "user_id" {
			payload[k] = v
		}
	}
	return userID, payload
}
