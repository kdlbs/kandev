package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/service"
)

// TestHandleAgentCompleted_FinishRunFailureSkipsCompletionSideEffects is the
// PR fixup round 2 regression test (CodeRabbit). handleAgentCompleted used
// to record the "complete" run event and the output summary BEFORE checking
// FinishRun's result at all — so a run whose terminal write never actually
// landed (a genuine persistence failure here, or a lost race to a
// concurrent writer in production) still got its evidence recorded as if
// this call had won. Both outcomes reach the same guard now: the
// completion side effects only run once FinishRun confirms wrote=true.
func TestHandleAgentCompleted_FinishRunFailureSkipsCompletionSideEffects(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()
	ensureTaskSessionMessagesTable(t, svc)

	createTestAgent(t, svc, "ws-1", "worker-race")
	taskID := createOfficeTask(t, svc, "ws-1", "worker-race")

	if err := svc.QueueRun(
		ctx, "worker-race", service.RunReasonTaskAssigned,
		`{"task_id":"`+taskID+`"}`, "run-output-race",
	); err != nil {
		t.Fatalf("queue run: %v", err)
	}
	run, err := svc.ClaimNextRun(ctx)
	if err != nil || run == nil {
		t.Fatalf("claim run: %v (run=%v)", err, run)
	}
	claimedAt := requireClaimedAt(t, run)

	svc.ExecSQL(t, `
		INSERT INTO task_session_messages (id, task_session_id, type, author_type, content, created_at)
		VALUES ('m-race-1', 'sess-race', 'message', 'agent', 'should never be recorded', ?)
	`, claimedAt.Add(5*time.Second))

	// Force FinishRun's UPDATE to fail with a genuine error, matching
	// TestSchedulerTick_AgentCompletedKeepsCheckoutWhenFinishRunFails'
	// fault injection: a targeted fault (drop the column FinishRun sets)
	// rather than a global read-only pragma.
	svc.ExecSQL(t, "ALTER TABLE runs DROP COLUMN finished_at")

	completed := bus.NewEvent(events.AgentCompleted, "test", map[string]string{
		"task_id":          taskID,
		"session_id":       "sess-race",
		"agent_profile_id": "worker-race",
	})
	if pErr := eb.Publish(ctx, events.AgentCompleted, completed); pErr != nil {
		t.Fatalf("publish completed: %v", pErr)
	}

	got := findRunByID(t, svc, "ws-1", run.ID)
	if got.OutputSummary != "" {
		t.Errorf("output_summary = %q, want empty — FinishRun never landed, so recordRunOutputSummary "+
			"must not have run", got.OutputSummary)
	}
	if got.Status != service.RunStatusClaimed {
		t.Errorf("status = %q, want claimed (FinishRun's write failed)", got.Status)
	}

	runEvents, err := svc.ListRunEventsForTest(ctx, run.ID)
	if err != nil {
		t.Fatalf("list run events: %v", err)
	}
	for _, e := range runEvents {
		if e.EventType == "complete" {
			t.Fatalf("expected no 'complete' run event when FinishRun's write failed; got %+v", e)
		}
	}
}

// TestHandleTasklessAgentCompleted_FinishRunFailureSkipsCompletionSideEffects
// is the taskless-path counterpart: refreshContinuationSummary and
// recordRunOutputSummary must not persist evidence for a completion whose
// terminal write never landed either.
func TestHandleTasklessAgentCompleted_FinishRunFailureSkipsCompletionSideEffects(t *testing.T) {
	svc := newTestService(t)
	svc.SetSyncHandlers(true)
	ctx := context.Background()
	eb := bus.NewMemoryEventBus(logger.Default())
	if err := svc.RegisterEventSubscribers(eb); err != nil {
		t.Fatalf("register subscribers: %v", err)
	}

	agent := &models.AgentInstance{
		ID:                 "coordinator-taskless-race-effects",
		WorkspaceID:        "ws-1",
		Name:               "coordinator-taskless-race-effects",
		Role:               models.AgentRoleCEO,
		Status:             models.AgentStatusIdle,
		ExecutorPreference: `{"type":"worktree"}`,
	}
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	svc.ExecSQL(t, `
		INSERT INTO runs (
			id, agent_profile_id, reason, payload, status, coalesced_count,
			context_snapshot, requested_at, claimed_at
		) VALUES (
			'run-taskless-race-1', ?, 'routine_trigger', '{}',
			'claimed', 1, '{}', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
		)
	`, agent.ID)

	svc.ExecSQL(t, "ALTER TABLE runs DROP COLUMN finished_at")

	completed := bus.NewEvent(events.AgentCompleted, "test", map[string]string{
		"agent_id":         agent.ID,
		"agent_profile_id": agent.ID,
		"session_id":       "sess-taskless-race",
	})
	if err := eb.Publish(ctx, events.AgentCompleted, completed); err != nil {
		t.Fatalf("publish completed: %v", err)
	}

	got := findRunByID(t, svc, "ws-1", "run-taskless-race-1")
	if got.Status != service.RunStatusClaimed {
		t.Errorf("status = %q, want claimed (FinishRun's write failed)", got.Status)
	}

	runEvents, err := svc.ListRunEventsForTest(ctx, "run-taskless-race-1")
	if err != nil {
		t.Fatalf("list run events: %v", err)
	}
	for _, e := range runEvents {
		if e.EventType == "complete" {
			t.Fatalf("expected no 'complete' run event when FinishRun's write failed; got %+v", e)
		}
	}

	summary, err := svc.GetContinuationSummaryForTest(ctx, agent.ID, "")
	if err == nil && summary != nil {
		t.Fatalf("expected no continuation summary written when FinishRun's write failed; got %+v", summary)
	}
}
