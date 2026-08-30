package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/service"
)

// TestLoadContinuationSummary_AgentScope_RoundTripsThroughRealWriterAndReader
// pins WO-16: the reader must key off run.ContinuationScope, the value
// models.ContinuationScopeForRun computed once at run creation and that
// the writer reads back at completion — not the retired "heartbeat"
// literal, and not a fresh re-derivation. A taskless run with no
// routine_id in its context_snapshot is written under "agent:<id>"
// (ContinuationScopeForRun's fallback branch); the reader must find that
// same row when assembling the prompt for the next taskless wake.
func TestLoadContinuationSummary_AgentScope_RoundTripsThroughRealWriterAndReader(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()
	ensureTaskSessionMessagesTable(t, svc)

	createTestAgent(t, svc, "ws-1", "agent-scope-a")

	if err := svc.QueueRun(
		ctx, "agent-scope-a", service.RunReasonTaskAssigned, "{}", "continuation-agent-scope",
	); err != nil {
		t.Fatalf("queue run: %v", err)
	}
	run, err := svc.ClaimNextRun(ctx)
	if err != nil || run == nil {
		t.Fatalf("claim run: %v (run=%v)", err, run)
	}

	completed := bus.NewEvent(events.AgentCompleted, "test", map[string]string{
		"agent_id":         "claude-acp",
		"agent_profile_id": "agent-scope-a",
		"session_id":       "sess-scope-a",
	})
	if pErr := eb.Publish(ctx, events.AgentCompleted, completed); pErr != nil {
		t.Fatalf("publish completed: %v", pErr)
	}

	// Confirm the writer actually landed a row under "agent:agent-scope-a"
	// before exercising the reader, so a reader failure below is
	// unambiguously the reader's bug and not a writer regression.
	written, err := svc.GetContinuationSummaryForTest(ctx, "agent-scope-a", "agent:agent-scope-a")
	if err != nil || written == nil || written.Content == "" {
		t.Fatalf("writer did not produce a summary under agent:agent-scope-a: %v (row=%v)", err, written)
	}

	// ContinuationScope is set explicitly here, as it would be by a real
	// ClaimNextRun-returned run: CreateRun computes and persists it once,
	// at creation time, and the reader now reads that stored value
	// directly instead of re-deriving it from ContextSnapshot.
	nextRun := &models.Run{ContextSnapshot: "{}", ContinuationScope: "agent:agent-scope-a"}
	si := service.NewSchedulerIntegration(svc, time.Minute)
	got := si.LoadContinuationSummaryForTest(ctx, nextRun, "agent-scope-a", "")
	if got == "" {
		t.Fatalf(`loadContinuationSummary returned "" for the agent:<id> scope; ` +
			`reader must key off run.ContinuationScope, not the retired "heartbeat" literal`)
	}
	if got != written.Content {
		t.Errorf("loadContinuationSummary = %q, want writer's content %q", got, written.Content)
	}
}

// TestLoadContinuationSummary_RoutineScope_RoundTripsThroughRealWriterAndReader
// covers the writer's other branch: a taskless run dispatched from a
// routine carries routine_id in its context_snapshot, so
// ContinuationScopeForRun keys the upsert "routine:<id>" instead of
// "agent:<id>". This is the case a naive `"agent:"+agentID` reader fix
// would still leave broken — see WO-16 Decision 2. The row is seeded
// with continuation_scope already populated, simulating a run that went
// through the real CreateRun path (which decides the scope once, at
// creation, before this row could ever be coalesced into).
func TestLoadContinuationSummary_RoutineScope_RoundTripsThroughRealWriterAndReader(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()
	ensureTaskSessionMessagesTable(t, svc)

	createTestAgent(t, svc, "ws-1", "agent-scope-r")

	claimedAt := time.Now().UTC()
	svc.ExecSQL(t, `
		INSERT INTO runs (
			id, agent_profile_id, reason, payload, status, coalesced_count,
			context_snapshot, continuation_scope, requested_at, claimed_at
		) VALUES (
			'run-scope-r', 'agent-scope-r', 'routine_trigger', '{}',
			'claimed', 1, '{"routine_id":"rt-1"}', 'routine:rt-1', ?, ?
		)
	`, claimedAt, claimedAt)

	completed := bus.NewEvent(events.AgentCompleted, "test", map[string]string{
		"agent_id":         "claude-acp",
		"agent_profile_id": "agent-scope-r",
		"run_id":           "run-scope-r",
		"session_id":       "sess-scope-r",
	})
	if pErr := eb.Publish(ctx, events.AgentCompleted, completed); pErr != nil {
		t.Fatalf("publish completed: %v", pErr)
	}

	written, err := svc.GetContinuationSummaryForTest(ctx, "agent-scope-r", "routine:rt-1")
	if err != nil || written == nil || written.Content == "" {
		t.Fatalf("writer did not produce a summary under routine:rt-1: %v (row=%v)", err, written)
	}

	nextRun := &models.Run{ContextSnapshot: `{"routine_id":"rt-1"}`, ContinuationScope: "routine:rt-1"}
	si := service.NewSchedulerIntegration(svc, time.Minute)
	got := si.LoadContinuationSummaryForTest(ctx, nextRun, "agent-scope-r", "")
	if got == "" {
		t.Fatalf(`loadContinuationSummary returned "" for the routine:<id> scope; ` +
			`a reader hardcoded to "agent:"+agentID would fail exactly this case`)
	}
	if got != written.Content {
		t.Errorf("loadContinuationSummary = %q, want writer's content %q", got, written.Content)
	}
}

// TestLoadContinuationSummary_ScopeSurvivesCoalesceAfterClaim pins the PR
// #2971 review round-2 finding: reader and writer must agree on scope
// even when a routine wakeup coalesces into an already-claimed run
// between claim and completion. The reader holds the claimed run in memory,
// while the writer fetches it again after completion. Coalescing patches
// context_snapshot, so re-deriving the scope would let the two paths select
// different summary rows. CreateRun persists ContinuationScope before a row
// can be coalesced, so both paths use the same bucket.
//
// This run originates from a plain (non-routine) taskless wakeup, so its
// scope is decided as "agent:<id>" at creation, before a routine wakeup
// coalesces into it — exactly the ordering wakeup.Dispatcher.Dispatch
// produces when a routine fires for an agent that already has an
// in-flight run (PolicyCoalesceIfActive). On pre-fix code this test
// fails: the writer lands the summary under "routine:rt-2" instead, so
// the final read below (using the claim-time run, still scoped
// "agent:<id>") comes back empty.
func TestLoadContinuationSummary_ScopeSurvivesCoalesceAfterClaim(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()
	ensureTaskSessionMessagesTable(t, svc)

	createTestAgent(t, svc, "ws-1", "agent-scope-coalesce")

	if err := svc.QueueRun(
		ctx, "agent-scope-coalesce", service.RunReasonTaskAssigned, "{}", "continuation-scope-coalesce",
	); err != nil {
		t.Fatalf("queue run: %v", err)
	}
	// claimedRun is the scheduler's claim-time snapshot — the same
	// in-memory object assembleAgentPrompt's reader uses. It is captured
	// once, here, and never re-read from the DB below, mirroring
	// processRun's real control flow (prepareAndLaunch carries this same
	// object through to assembleAgentPrompt).
	claimedRun, err := svc.ClaimNextRun(ctx)
	if err != nil || claimedRun == nil {
		t.Fatalf("claim run: %v (run=%v)", err, claimedRun)
	}
	if claimedRun.ContinuationScope != "agent:agent-scope-coalesce" {
		t.Fatalf("claimed run continuation_scope = %q, want %q",
			claimedRun.ContinuationScope, "agent:agent-scope-coalesce")
	}

	// A routine wakeup fires for the same agent while this run is still
	// in flight and coalesces into it — the same call the wakeup
	// dispatcher makes when PolicyCoalesceIfActive finds an in-flight run.
	svc.CoalesceRoutineWakeupForTest(ctx, t, "agent-scope-coalesce", claimedRun.ID, "rt-2")

	completed := bus.NewEvent(events.AgentCompleted, "test", map[string]string{
		"agent_id":         "claude-acp",
		"agent_profile_id": "agent-scope-coalesce",
		"run_id":           claimedRun.ID,
		"session_id":       "sess-scope-coalesce",
	})
	if pErr := eb.Publish(ctx, events.AgentCompleted, completed); pErr != nil {
		t.Fatalf("publish completed: %v", pErr)
	}

	// The writer must have landed the summary under the run's persisted
	// creation-time scope, unaffected by the coalesce.
	written, err := svc.GetContinuationSummaryForTest(ctx, "agent-scope-coalesce", "agent:agent-scope-coalesce")
	if err != nil || written == nil || written.Content == "" {
		t.Fatalf(
			"writer did not produce a summary under agent:agent-scope-coalesce after a "+
				"post-claim routine coalesce: %v (row=%v)", err, written)
	}

	// The reader, using the exact claim-time run object (unaware of the
	// later coalesce), must find that same row.
	si := service.NewSchedulerIntegration(svc, time.Minute)
	got := si.LoadContinuationSummaryForTest(ctx, claimedRun, "agent-scope-coalesce", "")
	if got == "" {
		t.Fatal(
			"loadContinuationSummary returned \"\" after a routine wakeup coalesced into an " +
				"already-claimed run; reader and writer disagreed on scope")
	}
	if got != written.Content {
		t.Errorf("loadContinuationSummary = %q, want writer's content %q", got, written.Content)
	}
}
