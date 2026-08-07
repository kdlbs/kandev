package service

import (
	"context"
	"testing"
)

type recordingWorkspaceDefaultsInitializer struct {
	eventBus *MockEventBus
	calls    []string
	seen     []int
}

func (r *recordingWorkspaceDefaultsInitializer) InitializeWorkspaceDefaults(_ context.Context, workspaceID string) error {
	r.calls = append(r.calls, workspaceID)
	r.seen = append(r.seen, len(r.eventBus.GetPublishedEvents()))
	return nil
}

func TestService_CreateWorkspaceInitializesDefaultsBeforePublication(t *testing.T) {
	svc, eventBus, _ := createTestService(t)
	initializer := &recordingWorkspaceDefaultsInitializer{eventBus: eventBus}
	svc.SetWorkspaceDefaultsInitializer(initializer)

	workspace, err := svc.CreateWorkspace(context.Background(), &CreateWorkspaceRequest{Name: "Defaults"})
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	if initializer.calls == nil || len(initializer.calls) != 1 || initializer.calls[0] != workspace.ID {
		t.Fatalf("initializer calls = %#v, want workspace %q", initializer.calls, workspace.ID)
	}
	if len(initializer.seen) != 1 || initializer.seen[0] != 0 {
		t.Fatalf("events visible during initialization = %#v, want none", initializer.seen)
	}
	if events := eventBus.GetPublishedEvents(); len(events) == 0 {
		t.Fatal("workspace.created was not published after initialization")
	}
}
