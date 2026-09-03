package websocket

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// pluginUserStateTestEvent mirrors internal/plugins' pluginUserStateUpdatedEvent
// (a plain struct with an exported UserID, json:"user_id") without a
// plugins package import (avoiding a gateway/websocket -> plugins dependency
// this package doesn't otherwise have).
type pluginUserStateTestEvent struct {
	UserID   string `json:"user_id"`
	PluginID string `json:"pluginId"`
	Key      string `json:"key"`
}

func testLoggerForUserNotifications(t *testing.T) *logger.Logger {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	return log
}

// TestRegisterUserNotifications_PluginUserStateReachesOnlyWriter pins AC24:
// a plugin.user-state.updated bus event reaches only the writing user's WS
// connections; a second, differently-identified user's client receives
// nothing.
func TestRegisterUserNotifications_PluginUserStateReachesOnlyWriter(t *testing.T) {
	h := newTestHub(t)
	writer := newTestClient("writer-conn")
	other := newTestClient("other-conn")
	registerTestClient(h, writer)
	registerTestClient(h, other)
	h.SubscribeToUser(writer, "user_1")
	h.SubscribeToUser(other, "user_2")

	eventBus := bus.NewMemoryEventBus(testLoggerForUserNotifications(t))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	RegisterUserNotifications(ctx, eventBus, h, testLoggerForUserNotifications(t))

	payload := pluginUserStateTestEvent{UserID: "user_1", PluginID: "kandev-plugin-notes", Key: "note"}
	if err := eventBus.Publish(ctx, events.PluginUserStateUpdated,
		bus.NewEvent(events.PluginUserStateUpdated, "test", payload)); err != nil {
		t.Fatalf("publish: %v", err)
	}

	if !clientReceived(writer) {
		t.Fatal("expected the writing user's client to receive the notification")
	}
	if clientReceived(other) {
		t.Fatal("expected a different user's client to receive nothing")
	}
}

// TestRegisterUserNotifications_PluginUserStatePayloadShape pins that the WS
// notification payload carries exactly the plugin/scope/key fields (AC24) —
// no user_id leaks into the message the client receives.
func TestRegisterUserNotifications_PluginUserStatePayloadShape(t *testing.T) {
	h := newTestHub(t)
	writer := newTestClient("writer-conn")
	registerTestClient(h, writer)
	h.SubscribeToUser(writer, "user_1")

	eventBus := bus.NewMemoryEventBus(testLoggerForUserNotifications(t))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	RegisterUserNotifications(ctx, eventBus, h, testLoggerForUserNotifications(t))

	payload := pluginUserStateTestEvent{UserID: "user_1", PluginID: "kandev-plugin-notes", Key: "note"}
	if err := eventBus.Publish(ctx, events.PluginUserStateUpdated,
		bus.NewEvent(events.PluginUserStateUpdated, "test", payload)); err != nil {
		t.Fatalf("publish: %v", err)
	}

	select {
	case raw := <-writer.send:
		var msg ws.Message
		if err := json.Unmarshal(raw, &msg); err != nil {
			t.Fatalf("unmarshal envelope: %v", err)
		}
		if msg.Action != "plugin.user-state.updated" {
			t.Fatalf("Action = %q, want %q", msg.Action, "plugin.user-state.updated")
		}
		var got map[string]any
		if err := json.Unmarshal(msg.Payload, &got); err != nil {
			t.Fatalf("unmarshal payload: %v", err)
		}
		if got["pluginId"] != "kandev-plugin-notes" || got["key"] != "note" {
			t.Fatalf("payload = %+v, missing expected fields", got)
		}
		if _, leaked := got["user_id"]; leaked {
			t.Fatalf("payload leaked user_id: %+v", got)
		}
	default:
		t.Fatal("expected a message on writer.send")
	}
}

// TestRegisterUserNotifications_PluginUserStateRoutesUnderNATSTransport is
// the regression test for the NATS routing gap found in review: a
// NATS-backed EventBus marshals every event before publishing and
// JSON-decodes it into an untyped Data field on arrival
// (internal/events/bus/nats.go's createMsgHandler does
// json.Unmarshal(msg.Data, &event)), so a subscriber always sees
// event.Data as a map[string]interface{} — never the original Go struct.
// This reproduces exactly that shape (marshal the real event struct, then
// unmarshal into a bus.Event the same way nats.go does) without needing a
// live NATS server, and proves routing plus the no-leak-to-the-browser
// contract both still hold once event.Data arrives as a map instead of a
// struct.
func TestRegisterUserNotifications_PluginUserStateRoutesUnderNATSTransport(t *testing.T) {
	h := newTestHub(t)
	writer := newTestClient("writer-conn")
	registerTestClient(h, writer)
	h.SubscribeToUser(writer, "user_1")

	eventBus := bus.NewMemoryEventBus(testLoggerForUserNotifications(t))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	RegisterUserNotifications(ctx, eventBus, h, testLoggerForUserNotifications(t))

	published := bus.NewEvent(events.PluginUserStateUpdated, "test",
		pluginUserStateTestEvent{UserID: "user_1", PluginID: "kandev-plugin-notes", Key: "note"})
	wire, err := json.Marshal(published)
	if err != nil {
		t.Fatalf("marshal (simulating what a NATS publisher sends over the wire): %v", err)
	}
	var arrived bus.Event
	if err := json.Unmarshal(wire, &arrived); err != nil {
		t.Fatalf("unmarshal (simulating nats.go's createMsgHandler on receipt): %v", err)
	}
	if _, ok := arrived.Data.(map[string]interface{}); !ok {
		t.Fatalf("arrived.Data = %T, want map[string]interface{} (the whole point of this test)", arrived.Data)
	}

	if err := eventBus.Publish(ctx, events.PluginUserStateUpdated, &arrived); err != nil {
		t.Fatalf("publish: %v", err)
	}

	select {
	case raw := <-writer.send:
		var msg ws.Message
		if err := json.Unmarshal(raw, &msg); err != nil {
			t.Fatalf("unmarshal envelope: %v", err)
		}
		var got map[string]any
		if err := json.Unmarshal(msg.Payload, &got); err != nil {
			t.Fatalf("unmarshal payload: %v", err)
		}
		if got["pluginId"] != "kandev-plugin-notes" || got["key"] != "note" {
			t.Fatalf("payload = %+v, missing expected fields", got)
		}
		if _, leaked := got["user_id"]; leaked {
			t.Fatalf("payload leaked user_id: %+v", got)
		}
	default:
		t.Fatal("expected the writing user's client to receive the notification even when event.Data arrives as a map (NATS transport)")
	}
}

func TestRegisterUserNotifications_AgentProfileRecentUseReachesOnlyOwner(t *testing.T) {
	h := newTestHub(t)
	owner := newTestClient("owner-conn")
	other := newTestClient("other-conn")
	registerTestClient(h, owner)
	registerTestClient(h, other)
	h.SubscribeToUser(owner, "user_1")
	h.SubscribeToUser(other, "user_2")

	eventBus := bus.NewMemoryEventBus(testLoggerForUserNotifications(t))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	RegisterUserNotifications(ctx, eventBus, h, testLoggerForUserNotifications(t))
	if err := eventBus.Publish(ctx, events.UserAgentProfileRecentUseUpdated, bus.NewEvent(
		events.UserAgentProfileRecentUseUpdated,
		"test",
		map[string]any{
			"user_id":     "user_1",
			"context":     "quick_chat",
			"profile_ids": []string{"profile-a"},
			"revision":    int64(1),
			"updated_at":  "2026-08-27T00:00:00Z",
		},
	)); err != nil {
		t.Fatalf("publish: %v", err)
	}

	select {
	case raw := <-owner.send:
		var message ws.Message
		if err := json.Unmarshal(raw, &message); err != nil {
			t.Fatalf("unmarshal envelope: %v", err)
		}
		if message.Action != ws.ActionUserAgentProfileRecentUseUpdated {
			t.Fatalf("action = %q, want %q", message.Action, ws.ActionUserAgentProfileRecentUseUpdated)
		}
		var payload map[string]any
		if err := json.Unmarshal(message.Payload, &payload); err != nil {
			t.Fatalf("unmarshal payload: %v", err)
		}
		if _, leaked := payload["user_id"]; leaked {
			t.Fatalf("payload leaked user_id: %+v", payload)
		}
	default:
		t.Fatal("expected owner notification")
	}
	if clientReceived(other) {
		t.Fatal("expected other user's client to receive nothing")
	}
}
