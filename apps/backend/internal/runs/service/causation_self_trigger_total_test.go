package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/office/models"
	runsservice "github.com/kandev/kandev/internal/runs/service"
)

// TestQueueRun_RefusesBeyondSelfTriggerTotalAllowance pins
// AC-OFFICE-LAUNCH-SAFETY-004.8: a second, reason-independent allowance
// bounds the total self-caused wakes for one agent profile in the same
// rolling window, whatever reasons they name. Two different reasons each
// below the per-reason allowance can still together exceed the total.
func TestQueueRun_RefusesBeyondSelfTriggerTotalAllowance(t *testing.T) {
	svc, _ := newTestService(t)
	// Per-reason allowance high enough that neither reason alone trips it;
	// total allowance low enough that the combination does.
	svc.SetLaunchSafetyLimits(runsservice.DefaultMaxCausationDepth, 10, 2)
	ctx := context.Background()

	if _, err := svc.QueueRun(ctx, runsservice.QueueRunRequest{
		AgentProfileID: "agent-primary",
		Reason:         "reason_a",
		ActorKind:      models.ActorKindAgent,
		ActorID:        "agent-primary",
		IdempotencyKey: "task_comment:total-1",
	}); err != nil {
		t.Fatalf("queue first: %v", err)
	}
	if _, err := svc.QueueRun(ctx, runsservice.QueueRunRequest{
		AgentProfileID: "agent-primary",
		Reason:         "reason_b",
		ActorKind:      models.ActorKindAgent,
		ActorID:        "agent-primary",
		IdempotencyKey: "task_comment:total-2",
	}); err != nil {
		t.Fatalf("queue second: %v", err)
	}

	_, err := svc.QueueRun(ctx, runsservice.QueueRunRequest{
		AgentProfileID: "agent-primary",
		Reason:         "reason_c",
		ActorKind:      models.ActorKindAgent,
		ActorID:        "agent-primary",
		IdempotencyKey: "task_comment:total-3",
	})
	var refusal *runsservice.RefusalError
	if !errors.As(err, &refusal) {
		t.Fatalf("queue third (over total allowance): err = %v, want *RefusalError", err)
	}
	if refusal.Gate != runsservice.RefusalSelfTriggerTotal {
		t.Fatalf("refusal gate = %q, want %q", refusal.Gate, runsservice.RefusalSelfTriggerTotal)
	}
}

// TestQueueRun_SelfTriggerBothAllowancesExceeded_RefusedAsPerReason pins
// AC-OFFICE-LAUNCH-SAFETY-004.8's fixed-order clause: the two allowances
// are evaluated in a fixed order, per-reason first, so a wake that would
// exceed both is refused once and recorded as a per-reason refusal, not a
// total refusal.
func TestQueueRun_SelfTriggerBothAllowancesExceeded_RefusedAsPerReason(t *testing.T) {
	svc, _ := newTestService(t)
	svc.SetLaunchSafetyLimits(runsservice.DefaultMaxCausationDepth, 1, 1)
	ctx := context.Background()

	if _, err := svc.QueueRun(ctx, runsservice.QueueRunRequest{
		AgentProfileID: "agent-primary",
		Reason:         "same_reason",
		ActorKind:      models.ActorKindAgent,
		ActorID:        "agent-primary",
		IdempotencyKey: "task_comment:both-1",
	}); err != nil {
		t.Fatalf("queue first: %v", err)
	}

	_, err := svc.QueueRun(ctx, runsservice.QueueRunRequest{
		AgentProfileID: "agent-primary",
		Reason:         "same_reason",
		ActorKind:      models.ActorKindAgent,
		ActorID:        "agent-primary",
		IdempotencyKey: "task_comment:both-2",
	})
	var refusal *runsservice.RefusalError
	if !errors.As(err, &refusal) {
		t.Fatalf("queue second: err = %v, want *RefusalError", err)
	}
	if refusal.Gate != runsservice.RefusalSelfTrigger {
		t.Fatalf("refusal gate = %q, want %q (per-reason, fixed order)", refusal.Gate, runsservice.RefusalSelfTrigger)
	}
}

// TestQueueRun_SelfTriggerTotalBelowPerReason_HonoredAsConfigured pins
// AC-OFFICE-LAUNCH-SAFETY-004.8: a total configured below the per-reason
// value is valid and is honored as configured, making the per-reason
// allowance unreachable in practice for a single-reason caller.
func TestQueueRun_SelfTriggerTotalBelowPerReason_HonoredAsConfigured(t *testing.T) {
	svc, _ := newTestService(t)
	svc.SetLaunchSafetyLimits(runsservice.DefaultMaxCausationDepth, 5, 1)
	ctx := context.Background()

	if _, err := svc.QueueRun(ctx, runsservice.QueueRunRequest{
		AgentProfileID: "agent-primary",
		Reason:         "solo_reason",
		ActorKind:      models.ActorKindAgent,
		ActorID:        "agent-primary",
		IdempotencyKey: "task_comment:below-1",
	}); err != nil {
		t.Fatalf("queue first: %v", err)
	}

	_, err := svc.QueueRun(ctx, runsservice.QueueRunRequest{
		AgentProfileID: "agent-primary",
		Reason:         "solo_reason",
		ActorKind:      models.ActorKindAgent,
		ActorID:        "agent-primary",
		IdempotencyKey: "task_comment:below-2",
	})
	var refusal *runsservice.RefusalError
	if !errors.As(err, &refusal) {
		t.Fatalf("queue second (over the lower total): err = %v, want *RefusalError", err)
	}
	if refusal.Gate != runsservice.RefusalSelfTriggerTotal {
		t.Fatalf("refusal gate = %q, want %q (total is the binding limit here)", refusal.Gate, runsservice.RefusalSelfTriggerTotal)
	}
}

// TestQueueRun_SelfTriggerTotalAllowancePassResetsGateFailureState mirrors
// TestQueueRun_SelfTriggerAllowancePassResetsGateFailureState for the new
// self_trigger_total gate: a request within both allowances is a
// successful evaluation of the total gate too, and must reset a
// pre-existing failure streak for that (workspace, gate) pair.
func TestQueueRun_SelfTriggerTotalAllowancePassResetsGateFailureState(t *testing.T) {
	svc, _, repo := newTestServiceWithRepo(t)
	ctx := context.Background()

	if err := repo.RecordGateOutcome(ctx, testWorkspaceID, "self_trigger_total", false); err != nil {
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

	state, err := repo.GetGateFailureState(ctx, testWorkspaceID, "self_trigger_total")
	if err != nil {
		t.Fatalf("get gate failure state: %v", err)
	}
	if state.ConsecutiveFailures != 0 {
		t.Errorf("consecutive_failures = %d, want 0 (reset by the allowed self-trigger-total evaluation)", state.ConsecutiveFailures)
	}
}
