package sqlite_test

import (
	"context"
	"testing"
	"time"
)

func TestUpsertAgentRuntime_CreateAndRead(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	if err := repo.UpsertAgentRuntime(ctx, "agent-1", "idle", ""); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	state, err := repo.GetAgentRuntime(ctx, "agent-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if state == nil {
		t.Fatal("expected non-nil state")
	}
	if state.Status != "idle" {
		t.Errorf("status = %q, want idle", state.Status)
	}
	if state.PauseReason != "" {
		t.Errorf("pause_reason = %q, want empty", state.PauseReason)
	}
}

func TestUpsertAgentRuntime_Idempotent(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	if err := repo.UpsertAgentRuntime(ctx, "agent-1", "idle", ""); err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	if err := repo.UpsertAgentRuntime(ctx, "agent-1", "paused", "budget"); err != nil {
		t.Fatalf("second upsert: %v", err)
	}

	state, err := repo.GetAgentRuntime(ctx, "agent-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if state.Status != "paused" {
		t.Errorf("status = %q, want paused", state.Status)
	}
	if state.PauseReason != "budget" {
		t.Errorf("pause_reason = %q, want budget", state.PauseReason)
	}
}

func TestGetAgentRuntime_NotFound(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	state, err := repo.GetAgentRuntime(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if state != nil {
		t.Errorf("expected nil state for nonexistent agent, got %+v", state)
	}
}

func TestDeleteAgentRuntime(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	if err := repo.UpsertAgentRuntime(ctx, "agent-1", "idle", ""); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if err := repo.DeleteAgentRuntime(ctx, "agent-1"); err != nil {
		t.Fatalf("delete: %v", err)
	}

	state, err := repo.GetAgentRuntime(ctx, "agent-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if state != nil {
		t.Errorf("expected nil after delete, got %+v", state)
	}
}

func TestListAgentRuntimes(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	if err := repo.UpsertAgentRuntime(ctx, "a1", "idle", ""); err != nil {
		t.Fatalf("upsert a1: %v", err)
	}
	if err := repo.UpsertAgentRuntime(ctx, "a2", "working", ""); err != nil {
		t.Fatalf("upsert a2: %v", err)
	}

	runtimes, err := repo.ListAgentRuntimes(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(runtimes) != 2 {
		t.Fatalf("count = %d, want 2", len(runtimes))
	}
	if runtimes["a1"].Status != "idle" {
		t.Errorf("a1 status = %q, want idle", runtimes["a1"].Status)
	}
	if runtimes["a2"].Status != "working" {
		t.Errorf("a2 status = %q, want working", runtimes["a2"].Status)
	}
}

func TestUpdateRuntimeLastRunFinished(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	if err := repo.UpsertAgentRuntime(ctx, "agent-1", "paused", "budget"); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	now := time.Now().UTC()
	if err := repo.UpdateRuntimeLastRunFinished(ctx, "agent-1", now); err != nil {
		t.Fatalf("update: %v", err)
	}

	state, err := repo.GetAgentRuntime(ctx, "agent-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if state.LastRunFinishedAt == nil {
		t.Fatal("expected non-nil last_run_finished_at")
	}
	diff := state.LastRunFinishedAt.Sub(now)
	if diff < -time.Second || diff > time.Second {
		t.Errorf("last_run_finished_at off by %v", diff)
	}
	if state.Status != "paused" {
		t.Errorf("status = %q after UpdateRuntimeLastRunFinished, want paused", state.Status)
	}
	if state.PauseReason != "budget" {
		t.Errorf("pause_reason = %q after UpdateRuntimeLastRunFinished, want budget", state.PauseReason)
	}
}

// TestUpdateRuntimeLastRunFinished_CreatesRowWhenMissing is the regression
// test for the "43 of 44 agents" defect: agents that never went through
// onboarding's UpsertAgentRuntime call have no office_agent_runtime row, so
// a bare UPDATE affects 0 rows and the stamp is silently lost. The write
// must create the row instead of assuming one already exists.
func TestUpdateRuntimeLastRunFinished_CreatesRowWhenMissing(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	if state, err := repo.GetAgentRuntime(ctx, "agent-no-row"); err != nil {
		t.Fatalf("get before update: %v", err)
	} else if state != nil {
		t.Fatalf("expected no runtime row before update, got %+v", state)
	}

	now := time.Now().UTC()
	if err := repo.UpdateRuntimeLastRunFinished(ctx, "agent-no-row", now); err != nil {
		t.Fatalf("update: %v", err)
	}

	state, err := repo.GetAgentRuntime(ctx, "agent-no-row")
	if err != nil {
		t.Fatalf("get after update: %v", err)
	}
	if state == nil {
		t.Fatal("expected the runtime row to be created")
	}
	if state.LastRunFinishedAt == nil {
		t.Fatal("expected non-nil last_run_finished_at")
	}
	diff := state.LastRunFinishedAt.Sub(now)
	if diff < -time.Second || diff > time.Second {
		t.Errorf("last_run_finished_at off by %v", diff)
	}
	if state.Status != "idle" {
		t.Errorf("status = %q, want idle default", state.Status)
	}
}
