package websocket

import (
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// @covers AC-OFFICE-AUTOMATION-RUNS-001.23
func TestAutomationRunCreatedReachesOnlyTheWorkspace(t *testing.T) {
	hub, eventBus, ctx := newCoordinatorBroadcastHub(t, true)
	t.Cleanup(eventBus.Close)
	broadcaster := RegisterSystemNotifications(ctx, eventBus, hub, testLogger())
	t.Cleanup(broadcaster.Close)
	owner := registerAccessClient(t, hub, "owner", authn.Identity{UserID: "user-a", Role: authn.RoleMember})
	foreign := registerAccessClient(t, hub, "foreign", authn.Identity{UserID: "user-b", Role: authn.RoleMember})
	data := map[string]any{
		"workspace_id": "ws-a", "automation_id": "auto-1", "run_id": "run-1", "status": "triggered",
		"prompt": "must not leak",
	}
	if err := eventBus.Publish(ctx, events.AutomationRunCreated, bus.NewEvent(events.AutomationRunCreated, "test", data)); err != nil {
		t.Fatal(err)
	}
	message := readNotification(t, owner)
	if message.Action != ws.ActionAutomationRunCreated {
		t.Fatalf("wrong notification: %+v", message)
	}
	var payload map[string]any
	if err := json.Unmarshal(message.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"workspace_id": "ws-a", "automation_id": "auto-1", "run_id": "run-1", "status": "triggered"}
	if len(payload) != len(want) {
		t.Fatalf("payload = %#v, want %#v", payload, want)
	}
	for key, value := range want {
		if payload[key] != value {
			t.Fatalf("payload = %#v, want %#v", payload, want)
		}
	}
	if actions := receivedActions(foreign); len(actions) != 0 {
		t.Fatalf("foreign actor learned about the run: %v", actions)
	}
}
