package websocket

import (
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// @covers AC-UI-SIDEBAR-PLUGIN-GROUPING-001.4
func TestSidebarPluginGroupingInvalidationKeepsWorkspaceStatePrivate(t *testing.T) {
	hub, eventBus, ctx := newCoordinatorBroadcastHub(t, true)
	t.Cleanup(eventBus.Close)
	broadcaster := RegisterSystemNotifications(ctx, eventBus, hub, testLogger())
	t.Cleanup(broadcaster.Close)
	owner := registerAccessClient(t, hub, "owner", authn.Identity{UserID: "user-a", Role: authn.RoleMember})
	foreign := registerAccessClient(t, hub, "foreign", authn.Identity{UserID: "user-b", Role: authn.RoleMember})
	data := map[string]any{"plugin_id": "acme", "workspace_id": "ws-a", "state": map[string]any{"secret": "private-value"}}
	if err := eventBus.Publish(ctx, events.PluginSidebarGroupingUpdated, bus.NewEvent(events.PluginSidebarGroupingUpdated, "test", data)); err != nil {
		t.Fatal(err)
	}
	message := readNotification(t, owner)
	if message.Action != ws.ActionPluginSidebarGroupingUpdated {
		t.Fatalf("wrong notification: %+v", message)
	}
	var payload map[string]any
	if err := json.Unmarshal(message.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload) != 2 || payload["plugin_id"] != "acme" || payload["workspace_id"] != "ws-a" {
		t.Fatalf("workspace state escaped invalidation: %#v", payload)
	}
	if actions := receivedActions(foreign); len(actions) != 0 {
		t.Fatalf("foreign actor learned workspace grouping activity: %v", actions)
	}
	data["catalog"] = true
	if err := eventBus.Publish(ctx, events.PluginSidebarGroupingUpdated, bus.NewEvent(events.PluginSidebarGroupingUpdated, "test", data)); err != nil {
		t.Fatal(err)
	}
	for _, client := range []*Client{owner, foreign} {
		payload = nil
		message = readNotification(t, client)
		if err := json.Unmarshal(message.Payload, &payload); err != nil {
			t.Fatal(err)
		}
		if len(payload) != 2 || payload["catalog"] != true || payload["plugin_id"] != "acme" {
			t.Fatalf("global catalog notification exposed workspace or state: %#v", payload)
		}
	}
}
