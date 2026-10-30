package websocket

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/events/bus"
)

func TestThemeNotificationReachesConnectedClients(t *testing.T) {
	log := testLogger()
	eventBus := bus.NewMemoryEventBus(log)
	hub := newTestHub(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	broadcaster := RegisterTaskNotifications(ctx, eventBus, hub, log)
	defer broadcaster.Close()
	if err := eventBus.Publish(context.Background(), "plugin.themes_changed", bus.NewEvent("plugin.themes_changed", "plugins", map[string]any{"boot_id": "boot", "revision": 2})); err != nil {
		t.Fatal(err)
	}
	select {
	case message := <-hub.broadcast:
		if message.Action != "plugin.themes_changed" {
			t.Fatalf("unexpected action %s", message.Action)
		}
	default:
		t.Fatal("theme invalidation was not delivered")
	}
}
