package service

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
)

// TestPublishWorkspaceEvent_EmitsUnitPlacement is the assignee gap one
// resource along: publishWorkspaceEvent hand-builds its payload map, so a
// field on the model is not automatically on the wire.
//
// It matters more here than for most fields. publishWorkspaceAccessChanged
// exists to tell open clients that who-can-reach-this changed, and placement
// IS reach now that the visibility flag is gone - a move between units is what
// grants and withdraws access. Without unit_id the event says "something about
// this workspace changed" and omits the only thing that did.
func TestPublishWorkspaceEvent_EmitsUnitPlacement(t *testing.T) {
	svc, eventBus, _ := createTestService(t)
	svc.publishWorkspaceEvent(context.Background(), events.WorkspaceUpdated, &models.Workspace{
		ID: "ws-placement", Name: "Platform", UnitID: "unit-runtime",
	})

	data := singlePublishedEventData(t, eventBus)
	if got, _ := data["unit_id"].(string); got != "unit-runtime" {
		t.Fatalf("unit_id payload = %#v, want unit-runtime", data["unit_id"])
	}
}

// TestPublishWorkspaceEvent_EmitsEmptyUnitWhenUnplaced keeps the key present
// when a workspace sits nowhere. The frontend pins the value it already holds
// when the key is absent, so an omitted key would leave every open client
// showing the unit the workspace was just moved out of.
func TestPublishWorkspaceEvent_EmitsEmptyUnitWhenUnplaced(t *testing.T) {
	svc, eventBus, _ := createTestService(t)
	svc.publishWorkspaceEvent(context.Background(), events.WorkspaceUpdated, &models.Workspace{
		ID: "ws-unplaced", Name: "Platform",
	})

	data := singlePublishedEventData(t, eventBus)
	got, ok := data["unit_id"]
	if !ok {
		t.Fatal("unit_id absent from the payload: a move would be invisible to open clients")
	}
	if got != "" {
		t.Fatalf("unit_id = %#v, want an explicit empty string", got)
	}
}
