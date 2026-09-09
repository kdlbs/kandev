package scheduler

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

// TestQueueRunCtx_WaveCarryingRequest_DedupesPastIdempotencyWindow proves
// P1's own direct-insert path (QueueRunCtx -> queueRun -> ss.repo.CreateRun)
// classifies idx_run_wake_wave violations as a delivered duplicate, not an
// error — the runs/service classification from Task 02 does not cover this
// call site, since cascadeChildrenCompleted never goes through
// runs/service. Two producers deriving the same wave for the same parent
// must still collapse to one persisted run even once the ordinary
// idempotency-key window (24h) has passed, because .002.6 requires the
// wave-key guard to stay unbounded.
func TestQueueRunCtx_WaveCarryingRequest_DedupesPastIdempotencyWindow(t *testing.T) {
	repo := newReactivityTestRepo(t)
	ss := newChildrenCompletedTestScheduler(t, repo)
	createChildrenCompletedAgent(t, repo, "agent-1")
	ctx := context.Background()

	first := RunContext{
		Reason:         RunReasonTaskChildrenCompleted,
		TaskID:         "parent-1",
		IdempotencyKey: "task_children_completed:parent-1:agent-1:wave-a",
		WaveKey:        "task_children_completed:parent-1:deadbeef",
		WaveString:     "parent-1|child-1,child-2",
	}
	if err := ss.QueueRunCtx(ctx, "agent-1", first); err != nil {
		t.Fatalf("queue first: %v", err)
	}
	if got := runsCountForReason(t, ss, RunReasonTaskChildrenCompleted); got != 1 {
		t.Fatalf("after first queue: runs = %d, want 1", got)
	}

	ageRunsRequestedAt(t, ss, 25*time.Hour)

	// A distinct idempotency key (as if independently derived by a second
	// read) but the same wave — must still dedupe via the wave key.
	second := first
	second.IdempotencyKey = "task_children_completed:parent-1:agent-1:wave-a-retry"
	if err := ss.QueueRunCtx(ctx, "agent-1", second); err != nil {
		t.Fatalf("queue racing: %v", err)
	}
	if got := runsCountForReason(t, ss, RunReasonTaskChildrenCompleted); got != 1 {
		t.Fatalf("after wave-key collision: runs = %d, want 1 (wave key must dedupe)", got)
	}
}

// TestQueueRunCtx_WaveCarryingRequest_NotCoalesced is the office/scheduler
// twin of runs/service's TestQueueRun_WakeCarryingRequest_NotCoalesced:
// AC-OFFICE-WAKE-WAVE-IDENTITY-002.14 requires a wave-carrying request
// never be coalesced in, and this call site inserts directly through
// ss.repo.CoalesceRun rather than through runs/service.
func TestQueueRunCtx_WaveCarryingRequest_NotCoalesced(t *testing.T) {
	repo := newReactivityTestRepo(t)
	ss := newChildrenCompletedTestScheduler(t, repo)
	createChildrenCompletedAgent(t, repo, "agent-1")
	ctx := context.Background()

	first := RunContext{
		Reason:         RunReasonTaskChildrenCompleted,
		TaskID:         "parent-1",
		IdempotencyKey: "k1",
		WaveKey:        "wave-key-1",
		WaveString:     "parent-1|child-1",
	}
	second := RunContext{
		Reason:         RunReasonTaskChildrenCompleted,
		TaskID:         "parent-2",
		IdempotencyKey: "k2",
		WaveKey:        "wave-key-2",
		WaveString:     "parent-2|child-2",
	}
	if err := ss.QueueRunCtx(ctx, "agent-1", first); err != nil {
		t.Fatalf("queue first: %v", err)
	}
	if err := ss.QueueRunCtx(ctx, "agent-1", second); err != nil {
		t.Fatalf("queue second: %v", err)
	}

	if got := runsCountForReason(t, ss, RunReasonTaskChildrenCompleted); got != 2 {
		t.Fatalf("runs = %d, want 2 (wave-carrying requests must not coalesce)", got)
	}
}

// TestQueueRunCtx_NonWaveRequest_StillCoalesces is the regression guard for
// the coalescing-skip change above: two distinct requests with no wave key
// (distinct idempotency keys, so the idempotency check doesn't intercept
// either) must still coalesce into a single queued run exactly as before.
func TestQueueRunCtx_NonWaveRequest_StillCoalesces(t *testing.T) {
	repo := newReactivityTestRepo(t)
	ss := newChildrenCompletedTestScheduler(t, repo)
	createChildrenCompletedAgent(t, repo, "agent-1")
	ctx := context.Background()

	first := RunContext{Reason: RunReasonTaskBlockersResolved, TaskID: "task-1", IdempotencyKey: "k1"}
	second := RunContext{Reason: RunReasonTaskBlockersResolved, TaskID: "task-2", IdempotencyKey: "k2"}
	if err := ss.QueueRunCtx(ctx, "agent-1", first); err != nil {
		t.Fatalf("queue first: %v", err)
	}
	if err := ss.QueueRunCtx(ctx, "agent-1", second); err != nil {
		t.Fatalf("queue second: %v", err)
	}

	if got := runsCountForReason(t, ss, RunReasonTaskBlockersResolved); got != 1 {
		t.Fatalf("runs = %d, want 1 (non-wave requests must still coalesce)", got)
	}
}

// runPayloadForReason returns the JSON payload of the sole persisted run
// matching reason, decoded into a map for field assertions.
func runPayloadForReason(t *testing.T, ss *SchedulerService, reason string) map[string]any {
	t.Helper()
	var payload string
	row := ss.repo.ReaderDB().QueryRowx(
		ss.repo.ReaderDB().Rebind(`SELECT payload FROM runs WHERE reason = ?`), reason)
	if err := row.Scan(&payload); err != nil {
		t.Fatalf("scan payload: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(payload), &m); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	return m
}

// TestQueueRunCtx_ExtraPayloadTaskID_CannotRedirectRun is the regression
// guard for encodeRunContext's envelope re-assertion: a workflow-authored
// queue_run action's payload.task_id must never override the persisted
// run's task_id away from the cascade's own task.ParentID, since task_id
// is what SchedulerIntegration.extractTaskID reads to check out and
// budget the run.
func TestQueueRunCtx_ExtraPayloadTaskID_CannotRedirectRun(t *testing.T) {
	repo := newReactivityTestRepo(t)
	ss := newChildrenCompletedTestScheduler(t, repo)
	createChildrenCompletedAgent(t, repo, "agent-1")
	ctx := context.Background()

	c := RunContext{
		Reason:         RunReasonTaskChildrenCompleted,
		TaskID:         "parent-1",
		IdempotencyKey: "k1",
		ExtraPayload: map[string]any{
			"task_id": "foreign-task-99",
		},
	}
	if err := ss.QueueRunCtx(ctx, "agent-1", c); err != nil {
		t.Fatalf("queue: %v", err)
	}

	payload := runPayloadForReason(t, ss, RunReasonTaskChildrenCompleted)
	if got := payload["task_id"]; got != "parent-1" {
		t.Fatalf("payload task_id = %v, want %q (workflow-authored payload must not redirect the run)", got, "parent-1")
	}
}
