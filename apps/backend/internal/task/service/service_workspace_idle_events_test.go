package service

import (
	"testing"

	"github.com/kandev/kandev/internal/events"
)

func assertWorkspaceIdlePolicyEvent(t *testing.T, eventBus *MockEventBus, eventType, workspaceID string, enabled bool, timeout int) {
	t.Helper()
	published := eventBus.GetPublishedEvents()
	if len(published) == 0 {
		t.Fatal("workspace event was not published")
	}
	event := published[len(published)-1]
	if event.Type != eventType {
		t.Fatalf("event type = %q, want %q", event.Type, eventType)
	}
	data, ok := event.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("workspace event data has type %T", event.Data)
	}
	if data["id"] != workspaceID || data["acp_idle_suspension_enabled"] != enabled || data["acp_idle_timeout_minutes"] != timeout {
		t.Fatalf("workspace event policy = %#v, want workspace %q enabled %t timeout %d", data, workspaceID, enabled, timeout)
	}
}

func TestWorkspaceLifecycleEventsCarrySavedIdlePolicy(t *testing.T) {
	svc, eventBus, _ := createTestService(t)
	ctx := t.Context()
	workspace, err := svc.CreateWorkspace(ctx, &CreateWorkspaceRequest{Name: "Idle policy events"})
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	assertWorkspaceIdlePolicyEvent(t, eventBus, events.WorkspaceCreated, workspace.ID, false, 120)

	enabled, timeout := true, 45
	_, err = svc.UpdateWorkspace(ctx, workspace.ID, &UpdateWorkspaceRequest{
		ACPIdleSuspensionEnabled: &enabled,
		ACPIdleTimeoutMinutes:    &timeout,
	})
	if err != nil {
		t.Fatalf("UpdateWorkspace policy: %v", err)
	}
	assertWorkspaceIdlePolicyEvent(t, eventBus, events.WorkspaceUpdated, workspace.ID, true, 45)

	name := "Renamed"
	if _, err := svc.UpdateWorkspace(ctx, workspace.ID, &UpdateWorkspaceRequest{Name: &name}); err != nil {
		t.Fatalf("UpdateWorkspace name: %v", err)
	}
	assertWorkspaceIdlePolicyEvent(t, eventBus, events.WorkspaceUpdated, workspace.ID, true, 45)

	enabled = false
	if _, err := svc.UpdateWorkspace(ctx, workspace.ID, &UpdateWorkspaceRequest{ACPIdleSuspensionEnabled: &enabled}); err != nil {
		t.Fatalf("UpdateWorkspace disable policy: %v", err)
	}
	assertWorkspaceIdlePolicyEvent(t, eventBus, events.WorkspaceUpdated, workspace.ID, false, 45)
}
