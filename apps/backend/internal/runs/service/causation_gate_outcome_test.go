package service_test

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/office/models"
	runsservice "github.com/kandev/kandev/internal/runs/service"
)

// TestQueueRun_UnreadableCausingRunRecordsFailedGateOutcome pins
// AC-OFFICE-BACKPRESSURE-003.3/003.8: RefusalCausingRunUnreadable is
// exactly the "input cannot be read" case, so a refusal on that gate must
// increment the durable per-(workspace, gate) failure count, not just the
// refusal counter AC-OFFICE-RUN-CAUSATION-001.21 already required.
func TestQueueRun_UnreadableCausingRunRecordsFailedGateOutcome(t *testing.T) {
	svc, _, repo := newTestServiceWithRepo(t)
	ctx := context.Background()

	if _, err := svc.QueueRun(ctx, runsservice.QueueRunRequest{
		AgentProfileID: "agent-primary",
		Reason:         "task_assigned",
		CausingRunID:   "no-such-run",
	}); err == nil {
		t.Fatal("queue: want a refusal error")
	}

	state, err := repo.GetGateFailureState(ctx, testWorkspaceID, "causing_run_unreadable")
	if err != nil {
		t.Fatalf("get gate failure state: %v", err)
	}
	if state.ConsecutiveFailures != 1 {
		t.Errorf("consecutive_failures = %d, want 1", state.ConsecutiveFailures)
	}
}

// TestQueueRun_ReadableCausingRunResetsGateFailureState pins the success
// half of the same gate: a causing run that *does* read successfully is a
// successful evaluation (AC-OFFICE-BACKPRESSURE-003.10) and must reset a
// pre-existing failure streak, exercised through the real QueueRun path
// rather than the standalone RecordGateOutcome unit tests.
func TestQueueRun_ReadableCausingRunResetsGateFailureState(t *testing.T) {
	svc, _, repo := newTestServiceWithRepo(t)
	ctx := context.Background()

	if err := repo.RecordGateOutcome(ctx, testWorkspaceID, "causing_run_unreadable", false); err != nil {
		t.Fatalf("seed failure: %v", err)
	}
	if err := repo.RecordGateOutcome(ctx, testWorkspaceID, "causing_run_unreadable", false); err != nil {
		t.Fatalf("seed failure: %v", err)
	}

	if _, err := svc.QueueRun(ctx, runsservice.QueueRunRequest{
		AgentProfileID: "agent-primary",
		Reason:         "root",
		ActorKind:      models.ActorKindSystem,
	}); err != nil {
		t.Fatalf("queue root: %v", err)
	}
	root := getRun(t, repo, "agent-primary", "root")

	if _, err := svc.QueueRun(ctx, runsservice.QueueRunRequest{
		AgentProfileID: "agent-primary",
		Reason:         "follow_on",
		ActorKind:      models.ActorKindSystem,
		CausingRunID:   root.ID,
	}); err != nil {
		t.Fatalf("queue follow-on: %v", err)
	}

	state, err := repo.GetGateFailureState(ctx, testWorkspaceID, "causing_run_unreadable")
	if err != nil {
		t.Fatalf("get gate failure state: %v", err)
	}
	if state.ConsecutiveFailures != 0 {
		t.Errorf("consecutive_failures = %d, want 0 (reset by the readable causing run)", state.ConsecutiveFailures)
	}
}

// TestQueueRun_SelfTriggerAllowancePassResetsGateFailureState pins the
// self_trigger gate's success path through the real enqueue flow: a
// request within the allowance is a successful evaluation of the gate
// (AC-OFFICE-BACKPRESSURE-003.10) and must reset a pre-existing failure
// streak for that (workspace, gate) pair.
func TestQueueRun_SelfTriggerAllowancePassResetsGateFailureState(t *testing.T) {
	svc, _, repo := newTestServiceWithRepo(t)
	ctx := context.Background()

	if err := repo.RecordGateOutcome(ctx, testWorkspaceID, "self_trigger", false); err != nil {
		t.Fatalf("seed failure: %v", err)
	}

	if _, err := svc.QueueRun(ctx, runsservice.QueueRunRequest{
		AgentProfileID: "agent-primary",
		Reason:         "self_trigger_reason",
		ActorKind:      models.ActorKindAgent,
		ActorID:        "agent-primary",
	}); err != nil {
		t.Fatalf("queue: %v", err)
	}

	state, err := repo.GetGateFailureState(ctx, testWorkspaceID, "self_trigger")
	if err != nil {
		t.Fatalf("get gate failure state: %v", err)
	}
	if state.ConsecutiveFailures != 0 {
		t.Errorf("consecutive_failures = %d, want 0 (reset by the allowed self-trigger evaluation)", state.ConsecutiveFailures)
	}
}
