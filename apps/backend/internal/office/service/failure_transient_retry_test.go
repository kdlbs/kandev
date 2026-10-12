package service_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/service"
)

// transientMessage classifies as ClassTransient/AutoRetryable/FallbackAllowed
// (routingerr's provider-neutral network_unavailable rule) — the fixture
// used throughout this file for "a retryable blip".
const transientMessage = "connection reset by peer"

func safeTransientEvidence(run *models.Run) service.AgentFailureEvidence {
	return service.AgentFailureEvidence{
		RunID:            run.ID,
		SessionID:        "session-test",
		AgentExecutionID: "execution-test",
		PromptGeneration: 1,
		EvidenceKnown:    true,
	}
}

// assertScheduledRetryNear pins the exact AC-OFFICE-RUNTIME-001.11 backoff
// value ("5s then 10s"), not just that a retry was scheduled at all
// (Review round 3, R3-3: collapsing officeLegacyTransientBackoff to
// {1ms, 1ms} passed every prior assertion in this file).
func assertScheduledRetryNear(t *testing.T, got *time.Time, want time.Duration) {
	t.Helper()
	if got == nil {
		t.Fatal("expected scheduled_retry_at to be set")
	}
	const tolerance = 2 * time.Second
	if delta := time.Until(*got); delta < want-tolerance || delta > want+tolerance {
		t.Fatalf("scheduled_retry_at = now+%v, want approximately now+%v", delta, want)
	}
}

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

	wrote, err := svc.HandleAgentFailure(ctx, run, transientMessage, "", nil, safeTransientEvidence(run))
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
	assertScheduledRetryNear(t, refreshed.ScheduledRetryAt, 5*time.Second)
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

	// AC-OFFICE-RUNTIME-001.11's stated backoff: 5s on the first retry, 10s
	// on the second.
	wantDelay := []time.Duration{5 * time.Second, 10 * time.Second}

	for i := 0; i < 2; i++ {
		wrote, err := svc.HandleAgentFailure(ctx, run, transientMessage, "", nil, safeTransientEvidence(run))
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
		assertScheduledRetryNear(t, refreshed.ScheduledRetryAt, wantDelay[i])
		// Simulate the dispatcher re-claiming the requeued run for its
		// next launch attempt, the precondition every production caller
		// of HandleAgentFailure actually has.
		svc.ExecSQL(t, `UPDATE runs SET status = 'claimed', claimed_at = ? WHERE id = ?`,
			time.Now().UTC(), refreshed.ID)
		refreshed.Status = service.RunStatusClaimed
		run = refreshed
	}

	wrote, err := svc.HandleAgentFailure(ctx, run, transientMessage, "", nil, safeTransientEvidence(run))
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

	wrote, err := svc.HandleAgentFailure(ctx, run, "boom", "", nil, safeTransientEvidence(run))
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
	wrote, err := svc.HandleAgentFailure(ctx, run, msg, "", nil, safeTransientEvidence(run))
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

	wrote, err := svc.HandleAgentFailure(ctx, run, transientMessage, "", nil, safeTransientEvidence(run))
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

// TestHandleAgentFailure_ClaimStaleRunNotRetried pins review round 2
// finding R2-1: scheduling a retry always leaves retry_count > 0, and
// evaluateRunStaleness cancels any claimed run with retry_count > 0 once
// requested_at is older than the 2h staleRunThreshold — well before the
// 24h retryMaxAge this tier otherwise shares with the pre-launch retry
// tier. Without this gate, a transient failure on a run in that window
// would schedule a retry that the claim path immediately cancels,
// destroying the failure with no consecutive_failures increment and no
// inbox row. A run in the 2h-24h band must instead fall through to
// today's terminal accounting, exactly like a run past 24h already does.
func TestHandleAgentFailure_ClaimStaleRunNotRetried(t *testing.T) {
	svc, _ := newTestServiceWithBus(t)
	ctx := context.Background()

	createTestAgent(t, svc, "ws-1", "agent-claim-stale")
	taskID := "task-claim-stale"
	insertSyntheticTask(t, svc, taskID, "ws-1", "agent-claim-stale")
	run := queueAndReadRun(t, svc, "agent-claim-stale", taskID)

	// Inside the 24h retryMaxAge but past the 2h staleRunThreshold: a
	// long-running Office agent session is routine, not an edge case.
	requestedAt := time.Now().UTC().Add(-3 * time.Hour)
	svc.ExecSQL(t, `UPDATE runs SET requested_at = ? WHERE id = ?`, requestedAt, run.ID)
	run.RequestedAt = requestedAt

	wrote, err := svc.HandleAgentFailure(ctx, run, transientMessage, "", nil, safeTransientEvidence(run))
	if err != nil {
		t.Fatalf("handle failure: %v", err)
	}
	if !wrote {
		t.Fatal("wrote = false, want true (a claim-stale run must not retry, it is already marked failed)")
	}

	agent, err := svc.GetAgentInstance(ctx, "agent-claim-stale")
	if err != nil {
		t.Fatalf("get agent: %v", err)
	}
	if agent.ConsecutiveFailures != 1 {
		t.Fatalf("consecutive_failures = %d, want 1 (the failure must count, not vanish)", agent.ConsecutiveFailures)
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

// TestHandleAgentFailure_ScheduledArrivalPastStaleThresholdNotRetried pins
// review round 5 finding R5-1: the 2h staleRunThreshold gate must account
// for the backoff it is about to schedule, not just the run's current age.
// A run requested (2h - 3s) ago is still inside the threshold right now,
// but scheduling the first-attempt 5s retry would land its claimable time
// 2s past the threshold, where evaluateRunStaleness would cancel it with no
// consecutive_failures increment and no inbox row — the exact silent-loss
// outcome R2-1's gate exists to prevent, just displaced by the width of the
// backoff this same function schedules. The gate must refuse before that
// happens, falling through to today's terminal accounting instead.
func TestHandleAgentFailure_ScheduledArrivalPastStaleThresholdNotRetried(t *testing.T) {
	svc, _ := newTestServiceWithBus(t)
	ctx := context.Background()

	createTestAgent(t, svc, "ws-1", "agent-arrival-stale-first")
	taskID := "task-arrival-stale-first"
	insertSyntheticTask(t, svc, taskID, "ws-1", "agent-arrival-stale-first")
	run := queueAndReadRun(t, svc, "agent-arrival-stale-first", taskID)

	// retry_count == 0: the pending backoff is 5s. 2h-3s+5s = 2h+2s, past
	// the threshold.
	requestedAt := time.Now().UTC().Add(-(2*time.Hour - 3*time.Second))
	svc.ExecSQL(t, `UPDATE runs SET requested_at = ? WHERE id = ?`, requestedAt, run.ID)
	run.RequestedAt = requestedAt

	wrote, err := svc.HandleAgentFailure(ctx, run, transientMessage, "", nil, safeTransientEvidence(run))
	if err != nil {
		t.Fatalf("handle failure: %v", err)
	}
	if !wrote {
		t.Fatal("wrote = false, want true (the scheduled arrival is past the stale threshold; the failure must count, not vanish)")
	}

	agent, err := svc.GetAgentInstance(ctx, "agent-arrival-stale-first")
	if err != nil {
		t.Fatalf("get agent: %v", err)
	}
	if agent.ConsecutiveFailures != 1 {
		t.Fatalf("consecutive_failures = %d, want 1 (the failure must count, not vanish)", agent.ConsecutiveFailures)
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

// TestHandleAgentFailure_ScheduledArrivalPastStaleThresholdNotRetriedSecondAttempt
// pins the same R5-1 gap at the second backoff step (10s, retry_count ==
// 1), which a fix using a constant or first-step width would still miss: a
// run requested (2h - 8s) ago is inside the threshold under EITHER the old
// gate (time.Since alone, ~2h-8s < 2h) or a fixed-5s-width gate (2h-8s+5s <
// 2h), and only fails when the gate reads the actual per-attempt backoff
// (officeLegacyTransientBackoff[run.RetryCount] == 10s: 2h-8s+10s = 2h+2s).
func TestHandleAgentFailure_ScheduledArrivalPastStaleThresholdNotRetriedSecondAttempt(t *testing.T) {
	svc, _ := newTestServiceWithBus(t)
	ctx := context.Background()

	createTestAgent(t, svc, "ws-1", "agent-arrival-stale-second")
	taskID := "task-arrival-stale-second"
	insertSyntheticTask(t, svc, taskID, "ws-1", "agent-arrival-stale-second")
	run := queueAndReadRun(t, svc, "agent-arrival-stale-second", taskID)

	requestedAt := time.Now().UTC().Add(-(2*time.Hour - 8*time.Second))
	svc.ExecSQL(t, `UPDATE runs SET requested_at = ?, retry_count = 1 WHERE id = ?`,
		requestedAt, run.ID)
	run.RequestedAt = requestedAt
	run.RetryCount = 1

	wrote, err := svc.HandleAgentFailure(ctx, run, transientMessage, "", nil, safeTransientEvidence(run))
	if err != nil {
		t.Fatalf("handle failure: %v", err)
	}
	if !wrote {
		t.Fatal("wrote = false, want true (the scheduled arrival is past the stale threshold; the failure must count, not vanish)")
	}

	agent, err := svc.GetAgentInstance(ctx, "agent-arrival-stale-second")
	if err != nil {
		t.Fatalf("get agent: %v", err)
	}
	if agent.ConsecutiveFailures != 1 {
		t.Fatalf("consecutive_failures = %d, want 1 (the failure must count, not vanish)", agent.ConsecutiveFailures)
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

	wrote, err := svc.HandleAgentFailure(ctx, run, "Overloaded", "", providerErr, safeTransientEvidence(run))
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

// TestHandleAgentFailure_TransientRetryClearsDeadSession pins review round
// 1 finding R1-1: a run that already launched (and so already carries the
// session_id of the attempt that just failed) must have that session_id
// cleared on requeue. Otherwise the relaunch resolves the dead session in
// context_builder, mints its runtime JWT and prompt against it, and every
// runtime action the new attempt takes gets attributed to a terminated
// session until persistLaunchedSession finally overwrites the row.
func TestHandleAgentFailure_TransientRetryClearsDeadSession(t *testing.T) {
	svc, _ := newTestServiceWithBus(t)
	ctx := context.Background()

	createTestAgent(t, svc, "ws-1", "agent-dead-session")
	taskID := "task-dead-session"
	insertSyntheticTask(t, svc, taskID, "ws-1", "agent-dead-session")
	run := queueAndReadRun(t, svc, "agent-dead-session", taskID)

	const deadSessionID = "session-from-failed-attempt"
	svc.ExecSQL(t, `UPDATE runs SET session_id = ? WHERE id = ?`, deadSessionID, run.ID)
	run.SessionID = deadSessionID

	wrote, err := svc.HandleAgentFailure(ctx, run, transientMessage, "", nil, service.AgentFailureEvidence{
		RunID:            run.ID,
		SessionID:        deadSessionID,
		AgentExecutionID: "execution-dead-session",
		PromptGeneration: 1,
		EvidenceKnown:    true,
	})
	if err != nil {
		t.Fatalf("handle failure: %v", err)
	}
	if wrote {
		t.Fatal("wrote = true, want false (retry scheduled, run not terminal)")
	}

	refreshed, err := svc.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if refreshed.SessionID != "" {
		t.Fatalf("session_id = %q, want empty: a relaunch must not resolve the dead session from the failed attempt", refreshed.SessionID)
	}
}

// TestHandleAgentFailure_ProviderIDRequiredForTransientClassification pins
// review round 3 finding R3-1: routingerr's provider rules are matched
// before the provider-neutral rules, so classifying a raw provider rate
// limit string without the agent id misses "claude.stderr.rate.v1" and
// falls back to an unretryable agent_runtime_error — the canonical
// transient failure AC-4.2 exists to cover, silently un-covered. Threading
// the agent id ("claude-acp", which has provider rules) must retry; the
// identical message with no agent id must not.
func TestHandleAgentFailure_ProviderIDRequiredForTransientClassification(t *testing.T) {
	const rateLimitMessage = "rate limit exceeded"

	svc, _ := newTestServiceWithBus(t)
	ctx := context.Background()

	createTestAgent(t, svc, "ws-1", "agent-rate-limit-id")
	taskID := "task-rate-limit-id"
	insertSyntheticTask(t, svc, taskID, "ws-1", "agent-rate-limit-id")
	run := queueAndReadRun(t, svc, "agent-rate-limit-id", taskID)

	wrote, err := svc.HandleAgentFailure(ctx, run, rateLimitMessage, "claude-acp", nil, safeTransientEvidence(run))
	if err != nil {
		t.Fatalf("handle failure (with agent id): %v", err)
	}
	if wrote {
		t.Fatal("wrote = true, want false: claude-acp's provider rule should have classified transient")
	}
	refreshed, err := svc.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if refreshed.Status != service.RunStatusQueued {
		t.Fatalf("run status = %q, want %q", refreshed.Status, service.RunStatusQueued)
	}

	svc.ExecSQL(t, `UPDATE runs SET status = 'claimed', claimed_at = ? WHERE id = ?`,
		time.Now().UTC(), refreshed.ID)
	refreshed.Status = service.RunStatusClaimed

	createTestAgent(t, svc, "ws-1", "agent-rate-limit-no-id")
	taskID2 := "task-rate-limit-no-id"
	insertSyntheticTask(t, svc, taskID2, "ws-1", "agent-rate-limit-no-id")
	run2 := queueAndReadRun(t, svc, "agent-rate-limit-no-id", taskID2)

	wrote2, err := svc.HandleAgentFailure(ctx, run2, rateLimitMessage, "", nil, safeTransientEvidence(run2))
	if err != nil {
		t.Fatalf("handle failure (no agent id): %v", err)
	}
	if !wrote2 {
		t.Fatal("wrote = false, want true: without a provider id the same message classifies unclassified, not transient")
	}
	refreshed2, err := svc.GetRun(ctx, run2.ID)
	if err != nil {
		t.Fatalf("get run 2: %v", err)
	}
	if refreshed2.Status != service.RunStatusFailed {
		t.Fatalf("run status = %q, want %q", refreshed2.Status, service.RunStatusFailed)
	}
}

// TestHandleAgentFailure_ProviderErrorIDSubstitutedWhenAgentIDHasNoRules pins
// review round 4 finding R4-1: the substitution branch in
// tryLegacyTransientRetry (providerError.ProviderID stands in for agentID
// only when agentID itself has no provider rules) had no test exercising it
// — every existing case either supplied an agentID with rules (claude-acp)
// or left providerError.ProviderID empty. "too many requests" matches only
// codex-acp's rate rule, not any provider-neutral rule, so a retry here is
// evidence only of the substitution, not of some other classification path.
func TestHandleAgentFailure_ProviderErrorIDSubstitutedWhenAgentIDHasNoRules(t *testing.T) {
	svc, _ := newTestServiceWithBus(t)
	ctx := context.Background()

	createTestAgent(t, svc, "ws-1", "agent-opencode-go")
	taskID := "task-provider-id-substituted"
	insertSyntheticTask(t, svc, taskID, "ws-1", "agent-opencode-go")
	run := queueAndReadRun(t, svc, "agent-opencode-go", taskID)

	providerErr := &streams.ProviderError{
		Source:     "adapter",
		ProviderID: "codex-acp",
		Message:    "too many requests",
		OccurredAt: time.Now().UTC(),
	}

	// "opencode-go" is a real model-provider id (routingerr/rules.go's
	// HasProviderRules doc comment) with no rules of its own, so this only
	// retries if providerErr.ProviderID is substituted in.
	wrote, err := svc.HandleAgentFailure(ctx, run, "too many requests", "opencode-go", providerErr, safeTransientEvidence(run))
	if err != nil {
		t.Fatalf("handle failure: %v", err)
	}
	if wrote {
		t.Fatal("wrote = true, want false: codex-acp's provider rule should have classified transient via providerError.ProviderID")
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

// TestHandleAgentFailure_ScheduleRetryErrorFallsThroughToTerminal pins
// review round 3 finding R3-3: when the retry write itself errors,
// tryLegacyTransientRetry must return false so the caller falls through to
// terminal accounting, rather than the failure vanishing between
// MarkRunFailed and the retry write. A trigger targets exactly the column
// ScheduleRetry sets and MarkRunFailed does not, so MarkRunFailed's own
// write still succeeds.
func TestHandleAgentFailure_ScheduleRetryErrorFallsThroughToTerminal(t *testing.T) {
	svc, _ := newTestServiceWithBus(t)
	ctx := context.Background()

	createTestAgent(t, svc, "ws-1", "agent-schedule-retry-error")
	taskID := "task-schedule-retry-error"
	insertSyntheticTask(t, svc, taskID, "ws-1", "agent-schedule-retry-error")
	run := queueAndReadRun(t, svc, "agent-schedule-retry-error", taskID)

	svc.ExecSQL(t, `
		CREATE TRIGGER block_schedule_retry_test
		BEFORE UPDATE OF retry_count ON runs
		BEGIN
			SELECT RAISE(FAIL, 'retry_count update blocked for test');
		END`)

	wrote, err := svc.HandleAgentFailure(ctx, run, transientMessage, "", nil, safeTransientEvidence(run))
	if err != nil {
		t.Fatalf("handle failure: %v", err)
	}
	if !wrote {
		t.Fatal("wrote = false, want true: a ScheduleRetry error must fall through to terminal accounting")
	}

	agent, err := svc.GetAgentInstance(ctx, "agent-schedule-retry-error")
	if err != nil {
		t.Fatalf("get agent: %v", err)
	}
	if agent.ConsecutiveFailures != 1 {
		t.Fatalf("consecutive_failures = %d, want 1 (the failure must count, not vanish)", agent.ConsecutiveFailures)
	}

	refreshed, err := svc.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if refreshed.Status != service.RunStatusFailed {
		t.Fatalf("run status = %q, want %q", refreshed.Status, service.RunStatusFailed)
	}
}

// TestAgentFailedEvent_ThreadsAgentIDThroughToTransientRetry drives the
// real production call site end-to-end: publishing an AgentFailed bus
// event (not calling HandleAgentFailure directly, as every other test in
// this file does) with AgentLifecycleData.AgentID set to a provider with
// rules. R3-1 added a second identity threaded from the decoded event
// through to the classifier; this is the only test that would catch it
// being mis-threaded or dropped between the subscriber and the retry.
func TestAgentFailedEvent_ThreadsAgentIDThroughToTransientRetry(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()

	createTestAgent(t, svc, "ws-1", "agent-e2e-ratelimit")
	taskID := "task-agent-failed-e2e"
	insertSyntheticTask(t, svc, taskID, "ws-1", "agent-e2e-ratelimit")
	run := queueAndReadRun(t, svc, "agent-e2e-ratelimit", taskID)

	event := bus.NewEvent(events.AgentFailed, "orchestrator", service.AgentLifecycleData{
		TaskID:           taskID,
		RunID:            run.ID,
		AgentID:          "claude-acp",
		AgentProfileID:   "agent-e2e-ratelimit",
		SessionID:        "session-e2e-ratelimit",
		AgentExecutionID: "execution-e2e-ratelimit",
		PromptGeneration: 1,
		EvidenceKnown:    true,
		ErrorMessage:     "rate limit exceeded",
		ProviderError: &streams.ProviderError{
			Source:     "adapter",
			Message:    "rate limit exceeded",
			OccurredAt: time.Now().UTC(),
		},
	})
	if err := eb.Publish(ctx, events.AgentFailed, event); err != nil {
		t.Fatalf("publish: %v", err)
	}

	refreshed, err := svc.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if refreshed.Status != service.RunStatusQueued {
		t.Fatalf("run status = %q, want %q: a rate limit from an agent id with provider rules must retry",
			refreshed.Status, service.RunStatusQueued)
	}
	if refreshed.RetryCount != 1 {
		t.Fatalf("retry_count = %d, want 1", refreshed.RetryCount)
	}
}

func TestAgentFailedEvent_UnknownOrEffectfulEvidenceFallsThroughToTerminal(t *testing.T) {
	cases := []struct {
		name           string
		evidenceKnown  bool
		outputObserved bool
		effectObserved bool
	}{
		{name: "unknown", evidenceKnown: false},
		{name: "output", evidenceKnown: true, outputObserved: true},
		{name: "effect", evidenceKnown: true, effectObserved: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, eb := newTestServiceWithBus(t)
			ctx := context.Background()
			agentID := "agent-evidence-" + tc.name
			taskID := "task-evidence-" + tc.name
			createTestAgent(t, svc, "ws-1", agentID)
			insertSyntheticTask(t, svc, taskID, "ws-1", agentID)
			run := queueAndReadRun(t, svc, agentID, taskID)

			event := bus.NewEvent(events.AgentFailed, "orchestrator", service.AgentLifecycleData{
				TaskID:           taskID,
				RunID:            run.ID,
				AgentID:          "claude-acp",
				AgentProfileID:   agentID,
				AgentExecutionID: "execution-" + tc.name,
				PromptGeneration: 1,
				EvidenceKnown:    tc.evidenceKnown,
				OutputObserved:   tc.outputObserved,
				EffectObserved:   tc.effectObserved,
				ErrorMessage:     "rate limit exceeded",
			})
			if err := eb.Publish(ctx, events.AgentFailed, event); err != nil {
				t.Fatalf("publish: %v", err)
			}

			refreshed, err := svc.GetRun(ctx, run.ID)
			if err != nil {
				t.Fatalf("get run: %v", err)
			}
			if refreshed.Status != service.RunStatusFailed {
				t.Fatalf("run status = %q, want %q", refreshed.Status, service.RunStatusFailed)
			}
			if refreshed.RetryCount != 0 {
				t.Fatalf("retry_count = %d, want 0", refreshed.RetryCount)
			}
		})
	}
}

func TestAgentFailedEvent_DiagnosticMustMatchTerminalFailure(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()
	createTestAgent(t, svc, "ws-1", "agent-diagnostic-mismatch")
	taskID := "task-diagnostic-mismatch"
	insertSyntheticTask(t, svc, taskID, "ws-1", "agent-diagnostic-mismatch")
	run := queueAndReadRun(t, svc, "agent-diagnostic-mismatch", taskID)

	event := bus.NewEvent(events.AgentFailed, "orchestrator", service.AgentLifecycleData{
		TaskID:                      taskID,
		RunID:                       run.ID,
		AgentID:                     "claude-acp",
		AgentProfileID:              "agent-diagnostic-mismatch",
		SessionID:                   "session-diagnostic-mismatch",
		AgentExecutionID:            "execution-diagnostic-mismatch",
		PromptGeneration:            1,
		EvidenceKnown:               true,
		ProviderDiagnosticCandidate: true,
		ProviderDiagnosticText:      "connection reset by peer",
		ErrorMessage:                "rate limit exceeded",
	})
	if err := eb.Publish(ctx, events.AgentFailed, event); err != nil {
		t.Fatalf("publish: %v", err)
	}

	refreshed, err := svc.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if refreshed.Status != service.RunStatusFailed {
		t.Fatalf("run status = %q, want %q", refreshed.Status, service.RunStatusFailed)
	}
	if refreshed.RetryCount != 0 {
		t.Fatalf("retry_count = %d, want 0", refreshed.RetryCount)
	}
}

func TestAgentFailedEvent_RejectsStaleSessionEvidence(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()
	createTestAgent(t, svc, "ws-1", "agent-stale-session")
	taskID := "task-stale-session"
	insertSyntheticTask(t, svc, taskID, "ws-1", "agent-stale-session")
	run := queueAndReadRun(t, svc, "agent-stale-session", taskID)
	svc.ExecSQL(t, `UPDATE runs SET session_id = ? WHERE id = ?`, "current-session", run.ID)

	event := bus.NewEvent(events.AgentFailed, "orchestrator", service.AgentLifecycleData{
		TaskID:           taskID,
		RunID:            run.ID,
		AgentID:          "claude-acp",
		AgentProfileID:   "agent-stale-session",
		SessionID:        "predecessor-session",
		AgentExecutionID: "execution-stale-session",
		PromptGeneration: 1,
		EvidenceKnown:    true,
		ErrorMessage:     "rate limit exceeded",
	})
	if err := eb.Publish(ctx, events.AgentFailed, event); err != nil {
		t.Fatalf("publish: %v", err)
	}

	refreshed, err := svc.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if refreshed.Status != service.RunStatusClaimed {
		t.Fatalf("run status = %q, want %q: stale event must not resolve the current claim", refreshed.Status, service.RunStatusClaimed)
	}
}

// TestAgentFailedEvent_WithoutRunIDStaysTerminal covers the compatibility
// lookup for old events. The fallback can still find the claimed run by task
// and agent, but that event must not authorize an automatic retry because it
// cannot prove which run produced the failure.
func TestAgentFailedEvent_WithoutRunIDStaysTerminal(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()
	createTestAgent(t, svc, "ws-1", "agent-missing-run-id")
	taskID := "task-missing-run-id"
	insertSyntheticTask(t, svc, taskID, "ws-1", "agent-missing-run-id")
	run := queueAndReadRun(t, svc, "agent-missing-run-id", taskID)

	event := bus.NewEvent(events.AgentFailed, "orchestrator", service.AgentLifecycleData{
		TaskID:           taskID,
		AgentID:          "claude-acp",
		AgentProfileID:   "agent-missing-run-id",
		SessionID:        "session-missing-run-id",
		AgentExecutionID: "execution-missing-run-id",
		PromptGeneration: 1,
		EvidenceKnown:    true,
		ErrorMessage:     "rate limit exceeded",
	})
	if err := eb.Publish(ctx, events.AgentFailed, event); err != nil {
		t.Fatalf("publish: %v", err)
	}

	refreshed, err := svc.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if refreshed.Status != service.RunStatusFailed {
		t.Fatalf("run status = %q, want %q: a legacy event without run identity must stay terminal", refreshed.Status, service.RunStatusFailed)
	}
	if refreshed.RetryCount != 0 {
		t.Fatalf("retry_count = %d, want 0", refreshed.RetryCount)
	}
}

// TestHandleAgentFailure_TransientRetryNeverMarksRunFailed pins Review
// round 6's R6-1 fix: a classified-transient failure must requeue
// straight off the row's 'claimed' status without ever passing through
// status='failed' on the way. Before the reorder, HandleAgentFailure
// wrote 'failed' first and requeued second, leaving a
// claimed -> failed -> queued window: every policy cancel (task-tree
// cancel, workspace pause, participant eviction) guards its write to
// status IN ('queued','claimed'), so a cancel landing in that window
// would silently no-op and this handler would then resurrect a run the
// canceller believed it had stopped. A trigger that fails the write the
// instant status is set to 'failed' turns that window into a test
// failure instead of a race that only shows up in production.
func TestHandleAgentFailure_TransientRetryNeverMarksRunFailed(t *testing.T) {
	svc, _ := newTestServiceWithBus(t)
	ctx := context.Background()

	createTestAgent(t, svc, "ws-1", "agent-never-failed")
	taskID := "task-never-failed"
	insertSyntheticTask(t, svc, taskID, "ws-1", "agent-never-failed")
	run := queueAndReadRun(t, svc, "agent-never-failed", taskID)

	svc.ExecSQL(t, fmt.Sprintf(`
		CREATE TRIGGER guard_no_failed_on_retry
		BEFORE UPDATE OF status ON runs
		WHEN NEW.status = 'failed' AND OLD.id = '%s'
		BEGIN
			SELECT RAISE(FAIL, 'run must not be marked failed on the retry path');
		END;
	`, run.ID))

	wrote, err := svc.HandleAgentFailure(ctx, run, transientMessage, "", nil, safeTransientEvidence(run))
	if err != nil {
		t.Fatalf("handle failure: %v (the run was written to status=failed before requeuing)", err)
	}
	if wrote {
		t.Fatal("wrote = true, want false (retry scheduled, run not terminal)")
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

// TestHandleAgentFailure_TransientRetryLostRaceFallsThroughWithoutTerminalAccounting
// covers the other half of R6-1's fix: when the guarded requeue loses the
// race because a concurrent writer already moved the run off 'claimed'
// (simulating a task-tree cancel, workspace pause, or participant
// eviction that landed first), the handler must not resurrect the run to
// 'queued' and must not run terminal accounting (no consecutive-failure
// increment, no auto-pause) for a run it no longer owns — it must fall
// through to MarkRunFailed's identical 'claimed' guard, which also no-ops
// and leaves the winning writer's terminal state untouched.
func TestHandleAgentFailure_TransientRetryLostRaceFallsThroughWithoutTerminalAccounting(t *testing.T) {
	svc, _ := newTestServiceWithBus(t)
	ctx := context.Background()

	createTestAgent(t, svc, "ws-1", "agent-lost-race")
	taskID := "task-lost-race"
	insertSyntheticTask(t, svc, taskID, "ws-1", "agent-lost-race")
	run := queueAndReadRun(t, svc, "agent-lost-race", taskID)

	// Simulate a concurrent cancel winning the race and moving the run off
	// 'claimed' before this handler's guarded requeue attempt runs.
	svc.ExecSQL(t, `UPDATE runs SET status = 'cancelled', finished_at = ? WHERE id = ?`,
		time.Now().UTC(), run.ID)

	wrote, err := svc.HandleAgentFailure(ctx, run, transientMessage, "", nil, safeTransientEvidence(run))
	if err != nil {
		t.Fatalf("handle failure: %v", err)
	}
	if wrote {
		t.Fatal("wrote = true, want false: a lost race must not run terminal accounting")
	}

	agent, err := svc.GetAgentInstance(ctx, "agent-lost-race")
	if err != nil {
		t.Fatalf("get agent: %v", err)
	}
	if agent.ConsecutiveFailures != 0 {
		t.Fatalf("consecutive_failures = %d, want 0: the cancel already resolved this run", agent.ConsecutiveFailures)
	}

	refreshed, err := svc.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if refreshed.Status != service.RunStatusCancelled {
		t.Fatalf("run status = %q, want %q (the winning writer's terminal state must survive)",
			refreshed.Status, service.RunStatusCancelled)
	}
}
