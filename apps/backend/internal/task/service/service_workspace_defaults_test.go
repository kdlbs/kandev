package service

import (
	"context"
	"errors"
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

func TestWorkspaceIdlePolicyDefaultsAndPartialUpdates(t *testing.T) {
	svc, _, _ := createTestService(t)
	ctx := context.Background()
	first, err := svc.CreateWorkspace(ctx, &CreateWorkspaceRequest{Name: "First"})
	if err != nil {
		t.Fatalf("CreateWorkspace first: %v", err)
	}
	second, err := svc.CreateWorkspace(ctx, &CreateWorkspaceRequest{Name: "Second"})
	if err != nil {
		t.Fatalf("CreateWorkspace second: %v", err)
	}
	if first.ACPIdleSuspensionEnabled || first.ACPIdleTimeoutMinutes != 120 {
		t.Fatalf("first workspace default = enabled:%v timeout:%d", first.ACPIdleSuspensionEnabled, first.ACPIdleTimeoutMinutes)
	}

	enabled := true
	timeout := 45
	updated, err := svc.UpdateWorkspace(ctx, first.ID, &UpdateWorkspaceRequest{
		ACPIdleSuspensionEnabled: &enabled,
		ACPIdleTimeoutMinutes:    &timeout,
	})
	if err != nil {
		t.Fatalf("UpdateWorkspace policy: %v", err)
	}
	if !updated.ACPIdleSuspensionEnabled || updated.ACPIdleTimeoutMinutes != timeout {
		t.Fatalf("updated policy = enabled:%v timeout:%d", updated.ACPIdleSuspensionEnabled, updated.ACPIdleTimeoutMinutes)
	}
	name := "Renamed"
	updated, err = svc.UpdateWorkspace(ctx, first.ID, &UpdateWorkspaceRequest{Name: &name})
	if err != nil {
		t.Fatalf("UpdateWorkspace partial name: %v", err)
	}
	if !updated.ACPIdleSuspensionEnabled || updated.ACPIdleTimeoutMinutes != timeout {
		t.Fatalf("partial update reset policy = enabled:%v timeout:%d", updated.ACPIdleSuspensionEnabled, updated.ACPIdleTimeoutMinutes)
	}
	unchanged, err := svc.GetWorkspace(ctx, second.ID)
	if err != nil {
		t.Fatalf("GetWorkspace second: %v", err)
	}
	if unchanged.ACPIdleSuspensionEnabled || unchanged.ACPIdleTimeoutMinutes != 120 {
		t.Fatalf("workspace-scoped policy changed sibling: enabled:%v timeout:%d", unchanged.ACPIdleSuspensionEnabled, unchanged.ACPIdleTimeoutMinutes)
	}
	invalid := 0
	if _, err := svc.UpdateWorkspace(ctx, first.ID, &UpdateWorkspaceRequest{ACPIdleTimeoutMinutes: &invalid}); !errors.Is(err, ErrWorkspaceIdleTimeoutInvalid) {
		t.Fatalf("non-positive timeout update = %v, want ErrWorkspaceIdleTimeoutInvalid", err)
	}
}
