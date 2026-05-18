package sqlite_test

import (
	"context"
	"testing"
)

func TestOnboarding_InitialStateEmpty(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	state, err := repo.GetOnboardingState(ctx, "ws-1")
	if err != nil {
		t.Fatalf("get onboarding state: %v", err)
	}
	if state != nil {
		t.Fatalf("expected nil state for unknown workspace, got %+v", state)
	}
}

func TestOnboarding_HasAnyCompleted_NoRows(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	has, err := repo.HasAnyCompletedOnboarding(ctx)
	if err != nil {
		t.Fatalf("has any: %v", err)
	}
	if has {
		t.Error("expected false when no rows exist")
	}
}

func TestOnboarding_MarkComplete(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	err := repo.MarkOnboardingComplete(ctx, "ws-1", "agent-1", "task-1")
	if err != nil {
		t.Fatalf("mark complete: %v", err)
	}

	state, err := repo.GetOnboardingState(ctx, "ws-1")
	if err != nil {
		t.Fatalf("get state: %v", err)
	}
	if state == nil {
		t.Fatal("expected non-nil state")
	}
	if !state.Completed {
		t.Error("expected completed to be true")
	}
	if state.CEOAgentID != "agent-1" {
		t.Errorf("ceo_agent_id = %q, want %q", state.CEOAgentID, "agent-1")
	}
	if state.FirstTaskID != "task-1" {
		t.Errorf("first_task_id = %q, want %q", state.FirstTaskID, "task-1")
	}
	if state.CompletedAt == nil {
		t.Error("expected completed_at to be set")
	}
}

func TestOnboarding_MarkComplete_HasAny(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	if err := repo.MarkOnboardingComplete(ctx, "ws-1", "agent-1", ""); err != nil {
		t.Fatalf("mark complete: %v", err)
	}

	has, err := repo.HasAnyCompletedOnboarding(ctx)
	if err != nil {
		t.Fatalf("has any: %v", err)
	}
	if !has {
		t.Error("expected true after marking complete")
	}
}

func TestOnboarding_MarkComplete_Idempotent(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	if err := repo.MarkOnboardingComplete(ctx, "ws-1", "agent-1", "task-1"); err != nil {
		t.Fatalf("first mark: %v", err)
	}
	if err := repo.MarkOnboardingComplete(ctx, "ws-1", "agent-2", "task-2"); err != nil {
		t.Fatalf("second mark: %v", err)
	}

	state, err := repo.GetOnboardingState(ctx, "ws-1")
	if err != nil {
		t.Fatalf("get state: %v", err)
	}
	if state.CEOAgentID != "agent-2" {
		t.Errorf("ceo_agent_id = %q, want %q (should be updated)", state.CEOAgentID, "agent-2")
	}
}
