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

// TestQueueRunCtx_IdempotencyIndexRace_ClassifiedAsDedupe proves queueRun's
// direct-insert path treats a losing idx_run_idempotency violation the same
// as a losing idx_run_wake_wave violation: a no-op dedupe, not an error.
// Two concurrent cascade calls for the same parent compute the identical
// idempotency key deterministically from the child id set, so on Postgres
// the loser's CreateRun can report either unique index depending on
// constraint-check order — mirroring runs/service.insertRun's own two-way
// classification (idempotency_key OR wake_wave_key) keeps this call site
// from logging a spurious "enqueue run" error for what is actually the
// expected duplicate-wake outcome. The wave key is deliberately varied
// between the two requests so only the idempotency index, never the
// wake-wave index, can be the one that fires.
func TestQueueRunCtx_IdempotencyIndexRace_ClassifiedAsDedupe(t *testing.T) {
	repo := newReactivityTestRepo(t)
	ss := newChildrenCompletedTestScheduler(t, repo)
	createChildrenCompletedAgent(t, repo, "agent-1")
	ctx := context.Background()

	first := RunContext{
		Reason:         RunReasonTaskChildrenCompleted,
		TaskID:         "parent-1",
		IdempotencyKey: "task_children_completed:parent-1:agent-1:shared-key",
		WaveKey:        "task_children_completed:parent-1:wave-a",
		WaveString:     "parent-1|child-1,child-2",
	}
	if err := ss.QueueRunCtx(ctx, "agent-1", first); err != nil {
		t.Fatalf("queue first: %v", err)
	}
	if got := runsCountForReason(t, ss, RunReasonTaskChildrenCompleted); got != 1 {
		t.Fatalf("after first queue: runs = %d, want 1", got)
	}

	// Age the row past the windowed CheckIdempotencyKey lookup so the
	// second call's pre-check passes and it actually reaches CreateRun,
	// which is where the unbounded idx_run_idempotency constraint (not the
	// windowed check) is what has to classify the loss.
	ageRunsRequestedAt(t, ss, 25*time.Hour)

	second := first
	second.WaveKey = "task_children_completed:parent-1:wave-b"
	second.WaveString = "parent-1|child-1,child-2,child-3"
	if err := ss.QueueRunCtx(ctx, "agent-1", second); err != nil {
		t.Fatalf("queue racing (idempotency index): %v", err)
	}
	if got := runsCountForReason(t, ss, RunReasonTaskChildrenCompleted); got != 1 {
		t.Fatalf("after idempotency-index collision: runs = %d, want 1 (idempotency key must dedupe)", got)
	}
}

// TestQueueRunCtx_WaveCarryingRequest_NotCoalesced is the office/scheduler
// twin of runs/service's TestQueueRun_WakeCarryingRequest_NotCoalesced:
// AC-OFFICE-WAKE-WAVE-IDENTITY-002.14 requires a wave-carrying request
// never be coalesced in, and this call site inserts directly through
// ss.repo.CoalesceRun rather than through runs/service. Both requests here
// carry their own wave identity, so CoalesceRun's row-side guard (`AND
// wake_wave_key = ”`) already excludes the first row as a merge candidate
// on its own — this test does NOT isolate queueRun's own request-side
// guard (`if waveKey == ""`). See
// TestQueueRunCtx_WaveCarryingRequest_NotCoalescedIntoNonWaveRow for the
// asymmetric setup that isolates it.
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

// TestQueueRunCtx_WaveCarryingRequest_NotCoalescedIntoNonWaveRow isolates
// the REQUEST side of AC-OFFICE-WAKE-WAVE-IDENTITY-002.14 specifically: the
// existing QUEUED row for the same agent+reason carries NO wave identity,
// so CoalesceRun's own row-side guard (`AND wake_wave_key = ”`) would
// happily accept it as a merge candidate — only queueRun's own
// request-side check (`if waveKey == ""`) can prevent a merge here.
// TestQueueRunCtx_WaveCarryingRequest_NotCoalesced cannot prove this: both
// of its requests carry a wave key, so its outcome is fully explained by
// the row-side guard alone regardless of what the request-side check does.
func TestQueueRunCtx_WaveCarryingRequest_NotCoalescedIntoNonWaveRow(t *testing.T) {
	repo := newReactivityTestRepo(t)
	ss := newChildrenCompletedTestScheduler(t, repo)
	createChildrenCompletedAgent(t, repo, "agent-1")
	ctx := context.Background()

	first := RunContext{
		Reason:         RunReasonTaskChildrenCompleted,
		TaskID:         "parent-1",
		IdempotencyKey: "k1",
	}
	if err := ss.QueueRunCtx(ctx, "agent-1", first); err != nil {
		t.Fatalf("queue first: %v", err)
	}

	const waveKey = "task_children_completed:parent-2:bbbb"
	second := RunContext{
		Reason:         RunReasonTaskChildrenCompleted,
		TaskID:         "parent-2",
		IdempotencyKey: "k2",
		WaveKey:        waveKey,
		WaveString:     "parent-2|child-2",
	}
	if err := ss.QueueRunCtx(ctx, "agent-1", second); err != nil {
		t.Fatalf("queue second: %v", err)
	}

	if got := runsCountForReason(t, ss, RunReasonTaskChildrenCompleted); got != 2 {
		t.Fatalf("runs = %d, want 2 (a wave-carrying request must not coalesce into a non-wave-carrying row)", got)
	}

	var persistedWaveKey string
	row := ss.repo.ReaderDB().QueryRowx(
		ss.repo.ReaderDB().Rebind(`SELECT wake_wave_key FROM runs WHERE reason = ? AND wake_wave_key <> ''`),
		RunReasonTaskChildrenCompleted)
	if err := row.Scan(&persistedWaveKey); err != nil {
		t.Fatalf("scan persisted wave key: %v", err)
	}
	if persistedWaveKey != waveKey {
		t.Fatalf("persisted wake_wave_key = %q, want %q (second row's wave identity must survive uncoalesced)", persistedWaveKey, waveKey)
	}
}

// TestQueueRunCtx_NonWaveRequest_StillCoalesces is the regression guard for
// the coalescing-skip change above: two distinct requests with no wave key
// (distinct idempotency keys, so the idempotency check doesn't intercept
// either) must still coalesce into a single queued run exactly as before.
// Both requests target the same task: CoalesceRun now also scopes by
// payload.task_id (see runs/repository/sqlite's task-id-scoping fix), so a
// cross-task pair would correctly stay uncoalesced regardless of the
// wave-key gate this test isolates — using the same task keeps that
// unrelated guard out of the result.
func TestQueueRunCtx_NonWaveRequest_StillCoalesces(t *testing.T) {
	repo := newReactivityTestRepo(t)
	ss := newChildrenCompletedTestScheduler(t, repo)
	createChildrenCompletedAgent(t, repo, "agent-1")
	ctx := context.Background()

	first := RunContext{Reason: RunReasonTaskBlockersResolved, TaskID: "task-1", IdempotencyKey: "k1"}
	second := RunContext{Reason: RunReasonTaskBlockersResolved, TaskID: "task-1", IdempotencyKey: "k2"}
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

// TestQueueRunCtx_ExtraPayloadAgentProfileID_PassesThroughUnfiltered pins a
// documented non-guarantee: unlike task_id/workspace_id/child_task_id/
// workflow_step_id, encodeRunContext does not re-assert agent_profile_id
// from a typed field, because RunContext carries none to re-assert from.
// A workflow-authored payload.agent_profile_id therefore survives into the
// persisted payload as-is. This is harmless today — dispatch and routing
// read the run's AgentProfileID column, never this payload key — but a
// future consumer of payload["agent_profile_id"] must not assume the same
// override-protection the other four envelope fields get.
func TestQueueRunCtx_ExtraPayloadAgentProfileID_PassesThroughUnfiltered(t *testing.T) {
	repo := newReactivityTestRepo(t)
	ss := newChildrenCompletedTestScheduler(t, repo)
	createChildrenCompletedAgent(t, repo, "agent-1")
	ctx := context.Background()

	c := RunContext{
		Reason:         RunReasonTaskChildrenCompleted,
		TaskID:         "parent-1",
		IdempotencyKey: "k1",
		ExtraPayload: map[string]any{
			"agent_profile_id": "foreign-agent-99",
		},
	}
	if err := ss.QueueRunCtx(ctx, "agent-1", c); err != nil {
		t.Fatalf("queue: %v", err)
	}

	payload := runPayloadForReason(t, ss, RunReasonTaskChildrenCompleted)
	if got := payload["agent_profile_id"]; got != "foreign-agent-99" {
		t.Fatalf("payload agent_profile_id = %v, want %q (documented non-guarantee: this key is not re-asserted)", got, "foreign-agent-99")
	}
}
