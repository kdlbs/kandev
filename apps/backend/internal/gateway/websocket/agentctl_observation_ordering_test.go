package websocket

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

func TestAgentctlNotificationsPreserveObservationTime(t *testing.T) {
	log := testLogger()
	eventBus := &queuedTransportEventBus{MemoryEventBus: bus.NewMemoryEventBus(log)}
	hub := newTestHub(t)
	client := newTestClient("readiness-order")
	registerTestClient(hub, client)
	hub.SubscribeToSession(client, "session-1")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_ = RegisterTaskNotifications(ctx, eventBus, hub, log)
	starting := bus.NewEvent(events.AgentctlStarting, "test", map[string]interface{}{"session_id": "session-1"})
	starting.Timestamp = time.Date(2026, 10, 6, 2, 37, 39, 1, time.UTC)
	ready := bus.NewEvent(events.AgentctlReady, "test", map[string]interface{}{"session_id": "session-1"})
	ready.Timestamp = starting.Timestamp.Add(time.Second)
	require.NoError(t, eventBus.Publish(ctx, events.AgentctlStarting, starting))
	require.NoError(t, eventBus.Publish(ctx, events.AgentctlReady, ready))
	require.NoError(t, eventBus.deliverSubjectFirst(ctx, events.AgentctlReady))
	require.NoError(t, eventBus.deliverAll(ctx))
	for _, expected := range []*bus.Event{ready, starting} {
		select {
		case data := <-client.send:
			var message ws.Message
			require.NoError(t, json.Unmarshal(data, &message))
			require.True(t, message.Timestamp.Equal(expected.Timestamp), "delayed delivery must preserve observation time: %s != %s", message.Timestamp, expected.Timestamp)
		default:
			t.Fatal("missing readiness notification")
		}
	}
}
