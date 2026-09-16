package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/service"
)

// transientMessage classifies as ClassTransient/AutoRetryable/FallbackAllowed
// (routingerr's provider-neutral network_unavailable rule) — the fixture
// used throughout this file for "a retryable blip".
const transientMessage = "connection reset by peer"

// TestHandleAgentFailure_TransientDoesNotCountTowardAutoPause is AC-4.2's
// stated observable: a classified-transient post-start failure on the
// legacy path must not count toward auto-pause on its first occurrence.
func TestHandleAgentFailure_TransientDoesNotCountTowardAutoPause(t *testing.T) {
	svc, _ := newTestServiceWithBus(t)
	ctx := context.Background()

	createTestAgent(t, svc, "ws-1", "agent-transient")
	taskID := "task-transient"
	insertSyntheticTask(t, svc, taskID, "ws-1", "agent-transient")
	run := queueAndReadRun(t, svc, "agent-transient", taskID)

	wrote, err := svc.HandleAgentFailure(ctx, run, transientMessage, nil)
	if err != nil {
		t.Fatalf("handle failure: %v", err)
	}
	if wrote {
		t.Fatal("wrote = true, want false (retry scheduled, run not terminal)")
	}

	agent, err := svc.GetAgentInstance(ctx, "agent-transient")
	if err != nil {
		t.Fatalf("get agent: %v", err)
	}
	if agent.ConsecutiveFailures != 0 {
		t.Fatalf("consecutive_failures = %d, want 0", agent.ConsecutiveFailures)
	}
	if agent.Status == models.AgentStatusPaused {
		t.Fatal("agent auto-paused on a single transient failure")
	}

	refreshed, err := svc.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if refreshed.Status != service.RunStatusQueued {
		t.Fatalf("run status = %q, want %q", refreshed.Status, service.RunStatusQueued)
	}
	if refreshed.ScheduledRetryAt == nil {
		t.Fatal("expected scheduled_retry_at to be set")
	}
	if refreshed.RetryCount != 1 {
		t.Fatalf("retry_count = %d, want 1", refreshed.RetryCount)
	}
}

// TestHandleAgentFailure_TransientRetryBounded pins the retry budget: the
// third consecutive classified-transient failure on the same run (which by
// then carries retry_count == officeLegacyTransientMaxRetries) falls
// through to today's terminal accounting instead of scheduling a third
// retry.
func TestHandleAgentFailure_TransientRetryBounded(t *testing.T) {
	svc, _ := newTestServiceWithBus(t)
	ctx := context.Background()

	createTestAgent(t, svc, "ws-1", "agent-transient-bounded")
	taskID := "task-transient-bounded"
	insertSyntheticTask(t, svc, taskID, "ws-1", "agent-transient-bounded")
	run := queueAndReadRun(t, svc, "agent-transient-bounded", taskID)

	for i := 0; i < 2; i++ {
		wrote, err := svc.HandleAgentFailure(ctx, run, transientMessage, nil)
		if err != nil {
			t.Fatalf("handle failure %d: %v", i, err)
		}
		if wrote {
			t.Fatalf("attempt %d: wrote = true, want false", i)
		}
		refreshed, err := svc.GetRun(ctx, run.ID)
		if err != nil {
			t.Fatalf("get run %d: %v", i, err)
		}
		if refreshed.RetryCount != i+1 {
			t.Fatalf("attempt %d: retry_count = %d, want %d", i, refreshed.RetryCount, i+1)
		}
		// Simulate the dispatcher re-claiming the requeued run for its
		// next launch attempt, the precondition every production caller
		// of HandleAgentFailure actually has.
		svc.ExecSQL(t, `UPDATE runs SET status = 'claimed', claimed_at = ? WHERE id = ?`,
			time.Now().UTC(), refreshed.ID)
		refreshed.Status = service.RunStatusClaimed
		run = refreshed
	}

	wrote, err := svc.HandleAgentFailure(ctx, run, transientMessage, nil)
	if err != nil {
		t.Fatalf("handle failure (bound): %v", err)
	}
	if !wrote {
		t.Fatal("wrote = false, want true once the retry budget is exhausted")
	}

	agent, err := svc.GetAgentInstance(ctx, "agent-transient-bounded")
	if err != nil {
		t.Fatalf("get agent: %v", err)
	}
	if agent.ConsecutiveFailures != 1 {
		t.Fatalf("consecutive_failures = %d, want 1", agent.ConsecutiveFailures)
	}
}

// TestHandleAgentFailure_NonTransientUnchanged pins that an unclassified
// message ("boom", the classifier-evidence fixture from triage) behaves
// exactly as it did before this change: no retry branch fires.
func TestHandleAgentFailure_NonTransientUnchanged(t *testing.T) {
	svc, _ := newTestServiceWithBus(t)
	ctx := context.Background()

	createTestAgent(t, svc, "ws-1", "agent-non-transient")
	taskID := "task-non-transient"
	insertSyntheticTask(t, svc, taskID, "ws-1", "agent-non-transient")
	run := queueAndReadRun(t, svc, "agent-non-transient", taskID)

	wrote, err := svc.HandleAgentFailure(ctx, run, "boom", nil)
	if err != nil {
		t.Fatalf("handle failure: %v", err)
	}
	if !wrote {
		t.Fatal("wrote = false, want true (unclassified failures stay terminal)")
	}

	agent, err := svc.GetAgentInstance(ctx, "agent-non-transient")
	if err != nil {
		t.Fatalf("get agent: %v", err)
	}
	if agent.ConsecutiveFailures != 1 {
		t.Fatalf("consecutive_failures = %d, want 1", agent.ConsecutiveFailures)
	}

	refreshed, err := svc.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if refreshed.Status != service.RunStatusFailed {
		t.Fatalf("run status = %q, want %q", refreshed.Status, service.RunStatusFailed)
	}
	if refreshed.ScheduledRetryAt != nil {
		t.Fatal("expected scheduled_retry_at to remain unset")
	}
}

// TestHandleAgentFailure_UnlaunchableMessageNotRetried pins triage finding
// 2: failUnlaunchableRun's wiring-fault message ("office service has no
// task starter configured" et al.) classifies unclassified from text
// alone, so the deliberate "do not retry a permanent wiring fault"
// decision (scheduler_integration.go) survives this change untouched.
func TestHandleAgentFailure_UnlaunchableMessageNotRetried(t *testing.T) {
	svc, _ := newTestServiceWithBus(t)
	ctx := context.Background()

	createTestAgent(t, svc, "ws-1", "agent-unlaunchable")
	taskID := "task-unlaunchable"
	insertSyntheticTask(t, svc, taskID, "ws-1", "agent-unlaunchable")
	run := queueAndReadRun(t, svc, "agent-unlaunchable", taskID)

	const msg = "scheduler cannot launch run: no task starter is configured"
	wrote, err := svc.HandleAgentFailure(ctx, run, msg, nil)
	if err != nil {
		t.Fatalf("handle failure: %v", err)
	}
	if !wrote {
		t.Fatal("wrote = false, want true (a wiring fault must not retry)")
	}

	refreshed, err := svc.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if refreshed.Status != service.RunStatusFailed {
		t.Fatalf("run status = %q, want %q", refreshed.Status, service.RunStatusFailed)
	}
}

// TestHandleAgentFailure_StaleRunNotRetried pins the 24h retryMaxAge
// abandon rule shared with the pre-launch tier: a transient failure on a
// run requested more than 24h ago falls through to today's accounting
// instead of scheduling a retry that would never plausibly help.
func TestHandleAgentFailure_StaleRunNotRetried(t *testing.T) {
	svc, _ := newTestServiceWithBus(t)
	ctx := context.Background()

	createTestAgent(t, svc, "ws-1", "agent-stale")
	taskID := "task-stale"
	insertSyntheticTask(t, svc, taskID, "ws-1", "agent-stale")
	run := queueAndReadRun(t, svc, "agent-stale", taskID)

	staleRequestedAt := time.Now().UTC().Add(-25 * time.Hour)
	svc.ExecSQL(t, `UPDATE runs SET requested_at = ? WHERE id = ?`, staleRequestedAt, run.ID)
	run.RequestedAt = staleRequestedAt

	wrote, err := svc.HandleAgentFailure(ctx, run, transientMessage, nil)
	if err != nil {
		t.Fatalf("handle failure: %v", err)
	}
	if !wrote {
		t.Fatal("wrote = false, want true (a stale run must not retry, it is already marked failed)")
	}

	agent, err := svc.GetAgentInstance(ctx, "agent-stale")
	if err != nil {
		t.Fatalf("get agent: %v", err)
	}
	if agent.ConsecutiveFailures != 1 {
		t.Fatalf("consecutive_failures = %d, want 1", agent.ConsecutiveFailures)
	}

	refreshed, err := svc.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if refreshed.Status != service.RunStatusFailed {
		t.Fatalf("run status = %q, want %q", refreshed.Status, service.RunStatusFailed)
	}
}

// TestHandleAgentFailure_ProviderErrorMessagePreferred pins the reason the
// providerError parameter is in scope at all: a bare agent stderr string
// like "Overloaded" classifies unclassified from text alone, but the
// structured ProviderError.Message carries the signal the classifier
// needs.
func TestHandleAgentFailure_ProviderErrorMessagePreferred(t *testing.T) {
	svc, _ := newTestServiceWithBus(t)
	ctx := context.Background()

	createTestAgent(t, svc, "ws-1", "agent-provider-error")
	taskID := "task-provider-error"
	insertSyntheticTask(t, svc, taskID, "ws-1", "agent-provider-error")
	run := queueAndReadRun(t, svc, "agent-provider-error", taskID)

	providerErr := &streams.ProviderError{
		Source:     "adapter",
		Message:    transientMessage,
		OccurredAt: time.Now().UTC(),
	}

	wrote, err := svc.HandleAgentFailure(ctx, run, "Overloaded", providerErr)
	if err != nil {
		t.Fatalf("handle failure: %v", err)
	}
	if wrote {
		t.Fatal("wrote = true, want false: providerError.Message should have classified transient")
	}

	refreshed, err := svc.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if refreshed.Status != service.RunStatusQueued {
		t.Fatalf("run status = %q, want %q", refreshed.Status, service.RunStatusQueued)
	}
	if refreshed.RetryCount != 1 {
		t.Fatalf("retry_count = %d, want 1", refreshed.RetryCount)
	}
}
