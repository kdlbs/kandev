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
// pins WO-16: the reader must key off the exact scope summaryScopeForRun
// computes at write time, not the retired "heartbeat" literal. A taskless
// run with no routine_id in its context_snapshot is written under
// "agent:<id>" (summaryScopeForRun's fallback branch); the reader must find
// that same row when assembling the prompt for the next taskless wake.
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
		"agent_id":   "agent-scope-a",
		"session_id": "sess-scope-a",
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

	nextRun := &models.Run{ContextSnapshot: "{}"}
	si := service.NewSchedulerIntegration(svc, time.Minute)
	got := si.LoadContinuationSummaryForTest(ctx, nextRun, "agent-scope-a", "")
	if got == "" {
		t.Fatalf(`loadContinuationSummary returned "" for the agent:<id> scope; ` +
			`reader must key off summaryScopeForRun(run, agentID), not the retired "heartbeat" literal`)
	}
	if got != written.Content {
		t.Errorf("loadContinuationSummary = %q, want writer's content %q", got, written.Content)
	}
}

// TestLoadContinuationSummary_RoutineScope_RoundTripsThroughRealWriterAndReader
// covers the writer's other branch: a taskless run dispatched from a
// routine carries routine_id in its context_snapshot, so
// summaryScopeForRun keys the upsert "routine:<id>" instead of
// "agent:<id>". This is the case a naive `"agent:"+agentID` reader fix
// would still leave broken — see WO-16 Decision 2.
func TestLoadContinuationSummary_RoutineScope_RoundTripsThroughRealWriterAndReader(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()
	ensureTaskSessionMessagesTable(t, svc)

	createTestAgent(t, svc, "ws-1", "agent-scope-r")

	claimedAt := time.Now().UTC()
	svc.ExecSQL(t, `
		INSERT INTO runs (
			id, agent_profile_id, reason, payload, status, coalesced_count,
			context_snapshot, requested_at, claimed_at
		) VALUES (
			'run-scope-r', 'agent-scope-r', 'routine_trigger', '{}',
			'claimed', 1, '{"routine_id":"rt-1"}', ?, ?
		)
	`, claimedAt, claimedAt)

	completed := bus.NewEvent(events.AgentCompleted, "test", map[string]string{
		"agent_id":   "agent-scope-r",
		"session_id": "sess-scope-r",
	})
	if pErr := eb.Publish(ctx, events.AgentCompleted, completed); pErr != nil {
		t.Fatalf("publish completed: %v", pErr)
	}

	written, err := svc.GetContinuationSummaryForTest(ctx, "agent-scope-r", "routine:rt-1")
	if err != nil || written == nil || written.Content == "" {
		t.Fatalf("writer did not produce a summary under routine:rt-1: %v (row=%v)", err, written)
	}

	nextRun := &models.Run{ContextSnapshot: `{"routine_id":"rt-1"}`}
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
