package service_test

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/office/service"
)

// createStepDecisionsTable stubs the workflow_step_decisions table (owned
// by internal/workflow/repository in production) so tests in this package
// can exercise the office service's read of it without pulling in the
// workflow package's full schema.
func createStepDecisionsTable(t *testing.T, svc *service.Service) {
	t.Helper()
	svc.ExecSQL(t, `CREATE TABLE IF NOT EXISTS workflow_step_decisions (
		id TEXT PRIMARY KEY,
		task_id TEXT NOT NULL,
		step_id TEXT NOT NULL,
		decider_id TEXT NOT NULL DEFAULT '',
		decision TEXT NOT NULL,
		superseded_at DATETIME
	)`)
}

// TestHandleAgentCompleted_WarnsWhenReviewDecisionMissing pins the fix for
// the "reviewer rejects in a comment but the task strands in Review
// forever" defect: a review-stage run that completes without an active
// workflow_step_decisions row for the reviewer must emit a warn-level
// "decision.missing" run event, so the stall is visible instead of silent.
func TestHandleAgentCompleted_WarnsWhenReviewDecisionMissing(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()
	createStepDecisionsTable(t, svc)

	createTestAgent(t, svc, "ws-1", "reviewer-1")
	taskID := createOfficeTask(t, svc, "ws-1", "reviewer-1")

	payload := `{"task_id":"` + taskID + `","stage_type":"review","workflow_step_id":"step-1"}`
	if err := svc.QueueRun(ctx, "reviewer-1", service.RunReasonTaskAssigned, payload, "review-no-decision"); err != nil {
		t.Fatalf("queue run: %v", err)
	}
	run, err := svc.ClaimNextRun(ctx)
	if err != nil || run == nil {
		t.Fatalf("claim run: %v (run=%v)", err, run)
	}

	completed := bus.NewEvent(events.AgentCompleted, "test", map[string]string{
		"task_id":          taskID,
		"session_id":       "sess-1",
		"agent_profile_id": "reviewer-1",
	})
	if pErr := eb.Publish(ctx, events.AgentCompleted, completed); pErr != nil {
		t.Fatalf("publish completed: %v", pErr)
	}

	rowEvents, err := listRunEvents(t, svc, run.ID)
	if err != nil {
		t.Fatalf("list run events: %v", err)
	}
	found := false
	for _, e := range rowEvents {
		if string(e.EventType) == "decision.missing" {
			found = true
			if string(e.Level) != "warn" {
				t.Errorf("decision.missing level = %q, want warn", e.Level)
			}
		}
	}
	if !found {
		t.Fatalf("events = %+v, want a decision.missing warn event", rowEvents)
	}
}

// TestHandleAgentCompleted_WarnsOnLegacyReviewStartedReason pins Review
// round 1 finding 1: a run woken with the legacy "review_started" reason
// (the exact reason on the incident's run f05a6f89) carries no stage_type
// in its payload — production payload shape is {task_id, workflow_step_id,
// agent_profile_id}, mirroring what runPayload persists. The detector must
// derive stage_type from run.Reason the same way buildPromptContext already
// does for the prompt, or a decisionless review_started/approval_started
// turn goes undetected.
func TestHandleAgentCompleted_WarnsOnLegacyReviewStartedReason(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()
	createStepDecisionsTable(t, svc)

	createTestAgent(t, svc, "ws-1", "reviewer-1")
	taskID := createOfficeTask(t, svc, "ws-1", "reviewer-1")

	payload := `{"task_id":"` + taskID + `","workflow_step_id":"step-1","agent_profile_id":"reviewer-1"}`
	if err := svc.QueueRun(ctx, "reviewer-1", "review_started", payload, "review-started-no-decision"); err != nil {
		t.Fatalf("queue run: %v", err)
	}
	run, err := svc.ClaimNextRun(ctx)
	if err != nil || run == nil {
		t.Fatalf("claim run: %v (run=%v)", err, run)
	}

	completed := bus.NewEvent(events.AgentCompleted, "test", map[string]string{
		"task_id":          taskID,
		"session_id":       "sess-1",
		"agent_profile_id": "reviewer-1",
	})
	if pErr := eb.Publish(ctx, events.AgentCompleted, completed); pErr != nil {
		t.Fatalf("publish completed: %v", pErr)
	}

	rowEvents, err := listRunEvents(t, svc, run.ID)
	if err != nil {
		t.Fatalf("list run events: %v", err)
	}
	found := false
	for _, e := range rowEvents {
		if string(e.EventType) == "decision.missing" {
			found = true
		}
	}
	if !found {
		t.Fatalf("events = %+v, want a decision.missing warn event for legacy review_started reason", rowEvents)
	}
}

// TestHandleAgentCompleted_NoWarnWhenReviewDecisionRecorded is the
// counterpart green path: when the reviewer's decision was recorded via
// record_step_decision_kandev before the run finished, no decision.missing
// event should be emitted.
func TestHandleAgentCompleted_NoWarnWhenReviewDecisionRecorded(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()
	createStepDecisionsTable(t, svc)

	createTestAgent(t, svc, "ws-1", "reviewer-1")
	taskID := createOfficeTask(t, svc, "ws-1", "reviewer-1")

	payload := `{"task_id":"` + taskID + `","stage_type":"review","workflow_step_id":"step-1"}`
	if err := svc.QueueRun(ctx, "reviewer-1", service.RunReasonTaskAssigned, payload, "review-with-decision"); err != nil {
		t.Fatalf("queue run: %v", err)
	}
	run, err := svc.ClaimNextRun(ctx)
	if err != nil || run == nil {
		t.Fatalf("claim run: %v (run=%v)", err, run)
	}

	svc.ExecSQL(t,
		`INSERT INTO workflow_step_decisions (id, task_id, step_id, decider_id, decision) VALUES (?, ?, ?, ?, ?)`,
		"decision-1", taskID, "step-1", "reviewer-1", "rejected",
	)

	completed := bus.NewEvent(events.AgentCompleted, "test", map[string]string{
		"task_id":          taskID,
		"session_id":       "sess-1",
		"agent_profile_id": "reviewer-1",
	})
	if pErr := eb.Publish(ctx, events.AgentCompleted, completed); pErr != nil {
		t.Fatalf("publish completed: %v", pErr)
	}

	rowEvents, err := listRunEvents(t, svc, run.ID)
	if err != nil {
		t.Fatalf("list run events: %v", err)
	}
	for _, e := range rowEvents {
		if string(e.EventType) == "decision.missing" {
			t.Fatalf("unexpected decision.missing event: %+v", e)
		}
	}
}

// TestHandleAgentCompleted_NoWarnForWorkStage pins that the check is
// scoped to review/approval stages only: a work-stage run finishing
// without any workflow_step_decisions row is expected (builders don't
// record decisions) and must not emit decision.missing.
func TestHandleAgentCompleted_NoWarnForWorkStage(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()
	createStepDecisionsTable(t, svc)

	createTestAgent(t, svc, "ws-1", "builder-1")
	taskID := createOfficeTask(t, svc, "ws-1", "builder-1")

	payload := `{"task_id":"` + taskID + `","stage_type":"work","workflow_step_id":"step-1"}`
	if err := svc.QueueRun(ctx, "builder-1", service.RunReasonTaskAssigned, payload, "work-stage"); err != nil {
		t.Fatalf("queue run: %v", err)
	}
	run, err := svc.ClaimNextRun(ctx)
	if err != nil || run == nil {
		t.Fatalf("claim run: %v (run=%v)", err, run)
	}

	completed := bus.NewEvent(events.AgentCompleted, "test", map[string]string{
		"task_id":          taskID,
		"session_id":       "sess-1",
		"agent_profile_id": "builder-1",
	})
	if pErr := eb.Publish(ctx, events.AgentCompleted, completed); pErr != nil {
		t.Fatalf("publish completed: %v", pErr)
	}

	rowEvents, err := listRunEvents(t, svc, run.ID)
	if err != nil {
		t.Fatalf("list run events: %v", err)
	}
	for _, e := range rowEvents {
		if string(e.EventType) == "decision.missing" {
			t.Fatalf("unexpected decision.missing event for work stage: %+v", e)
		}
	}
}
