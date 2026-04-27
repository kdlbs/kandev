package service_test

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/orchestrate/service"
)

func TestGetOnboardingState_InitiallyNotCompleted(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	state, err := svc.GetOnboardingState(ctx)
	if err != nil {
		t.Fatalf("get onboarding state: %v", err)
	}
	if state.Completed {
		t.Error("expected completed=false initially")
	}
}

func TestCompleteOnboarding_CreatesEntities(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	result, err := svc.CompleteOnboarding(ctx, service.OnboardingCompleteRequest{
		WorkspaceName:      "default",
		TaskPrefix:         "TST",
		AgentName:          "CEO",
		AgentProfileID:     "",
		ExecutorPreference: "local_pc",
	})
	if err != nil {
		t.Fatalf("complete onboarding: %v", err)
	}
	if result.WorkspaceID == "" {
		t.Error("expected non-empty workspace ID")
	}
	if result.AgentID == "" {
		t.Error("expected non-empty agent ID")
	}
	if result.ProjectID == "" {
		t.Error("expected non-empty project ID")
	}

	// Verify onboarding is now completed.
	state, err := svc.GetOnboardingState(ctx)
	if err != nil {
		t.Fatalf("get state after complete: %v", err)
	}
	if !state.Completed {
		t.Error("expected completed=true after CompleteOnboarding")
	}
	if state.WorkspaceID != result.WorkspaceID {
		t.Errorf("state.WorkspaceID = %q, want %q", state.WorkspaceID, result.WorkspaceID)
	}
	if state.CEOAgentID != result.AgentID {
		t.Errorf("state.CEOAgentID = %q, want %q", state.CEOAgentID, result.AgentID)
	}
}

func TestCompleteOnboarding_RequiresWorkspaceName(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	// The service should fail creating the workspace.
	// We rely on the handler's validation for the workspaceName empty check,
	// but the service will also fail because the workspace won't resolve.
	_, err := svc.CompleteOnboarding(ctx, service.OnboardingCompleteRequest{
		WorkspaceName: "",
		AgentName:     "CEO",
	})
	if err == nil {
		t.Error("expected error for empty workspace name")
	}
}
