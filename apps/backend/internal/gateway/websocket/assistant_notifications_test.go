package websocket

import (
	"context"
	"encoding/json"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestAssistantAttentionNotificationOwnerOnly(t *testing.T) {
	h := newTestHub(t)
	owner := newTestClient("owner")
	foreign := newTestClient("foreign")
	registerTestClient(h, owner)
	registerTestClient(h, foreign)
	h.SubscribeToUser(owner, "one")
	h.SubscribeToUser(foreign, "two")
	eb := bus.NewMemoryEventBus(testLoggerForUserNotifications(t))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	RegisterUserNotifications(ctx, eb, h, testLoggerForUserNotifications(t))
	payload := map[string]any{"user_id": "one", "binding_id": "binding", "revision": "revision"}
	require.NoError(t, eb.Publish(ctx, events.AssistantUpdated, bus.NewEvent(events.AssistantUpdated, "test", payload)))
	select {
	case raw := <-owner.send:
		var msg ws.Message
		require.NoError(t, json.Unmarshal(raw, &msg))
		require.Equal(t, events.AssistantUpdated, msg.Action)
		require.JSONEq(t, `{"binding_id":"binding","revision":"revision"}`, string(msg.Payload))
	default:
		t.Fatal("owner did not receive invalidation")
	}
	require.False(t, clientReceived(foreign))
}
