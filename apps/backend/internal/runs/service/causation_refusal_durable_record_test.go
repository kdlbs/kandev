package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/office/models"
	runsservice "github.com/kandev/kandev/internal/runs/service"
)

// TestQueueRun_DepthRefusalPersistsDurableCausationRefusal pins
// AC-OFFICE-LAUNCH-SAFETY-003.5: a causation-depth refusal must leave a
// durable, operator-visible record naming the causation identifier, the
// refusing depth, and the agent that requested it — not just the zap log
// entry and expvar counter TestQueueRun_DepthRefusalLogsTheRefusingDepth
// already covers.
func TestQueueRun_DepthRefusalPersistsDurableCausationRefusal(t *testing.T) {
	svc, _, repo := newTestServiceWithRepo(t)
	svc.SetLaunchSafetyLimits(1, runsservice.DefaultSelfTriggerAllowance, runsservice.DefaultSelfTriggerTotalAllowance)
	ctx := context.Background()

	if _, err := svc.QueueRun(ctx, runsservice.QueueRunRequest{
		AgentProfileID: "agent-primary",
		Reason:         "root_reason",
		ActorKind:      models.ActorKindSystem,
	}); err != nil {
		t.Fatalf("queue root: %v", err)
	}
	root := getRun(t, repo, "agent-primary", "root_reason")

	if _, err := svc.QueueRun(ctx, runsservice.QueueRunRequest{
		AgentProfileID: "agent-primary",
		Reason:         "child1_reason",
		ActorKind:      models.ActorKindAgent,
		ActorID:        "agent-primary",
		CausingRunID:   root.ID,
	}); err != nil {
		t.Fatalf("queue child1: %v", err)
	}
	child1 := getRun(t, repo, "agent-primary", "child1_reason")

	_, err := svc.QueueRun(ctx, runsservice.QueueRunRequest{
		AgentProfileID: "agent-primary",
		Reason:         "child2_reason",
		ActorKind:      models.ActorKindAgent,
		ActorID:        "agent-primary",
		CausingRunID:   child1.ID,
	})
	var refusal *runsservice.RefusalError
	if !errors.As(err, &refusal) {
		t.Fatalf("queue child2: err = %v, want *RefusalError", err)
	}

	entries, err := repo.ListCausationRefusals(ctx, testWorkspaceID)
	if err != nil {
		t.Fatalf("list causation refusals: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("len(entries) = %d, want 1", len(entries))
	}
	entry := entries[0]
	if entry.Gate != string(runsservice.RefusalCausationDepth) {
		t.Errorf("gate = %q, want %q", entry.Gate, runsservice.RefusalCausationDepth)
	}
	if entry.AgentProfileID != "agent-primary" {
		t.Errorf("agent_profile_id = %q, want %q", entry.AgentProfileID, "agent-primary")
	}
	if entry.CausationID != root.CausationID {
		t.Errorf("causation_id = %q, want %q", entry.CausationID, root.CausationID)
	}
	if entry.CausationDepth != 2 {
		t.Errorf("causation_depth = %d, want 2 (the refusing depth)", entry.CausationDepth)
	}
}

// TestQueueRun_SelfTriggerRefusalPersistsDurableCausationRefusal pins
// AC-OFFICE-LAUNCH-SAFETY-004.3's "record the refusal as in
// AC-OFFICE-LAUNCH-SAFETY-003.5" clause for the per-reason self-trigger
// gate.
func TestQueueRun_SelfTriggerRefusalPersistsDurableCausationRefusal(t *testing.T) {
	svc, _, repo := newTestServiceWithRepo(t)
	svc.SetLaunchSafetyLimits(runsservice.DefaultMaxCausationDepth, 1, 10)
	ctx := context.Background()

	if _, err := svc.QueueRun(ctx, runsservice.QueueRunRequest{
		AgentProfileID: "agent-primary",
		Reason:         "same_reason",
		ActorKind:      models.ActorKindAgent,
		ActorID:        "agent-primary",
		IdempotencyKey: "task_comment:durable-1",
	}); err != nil {
		t.Fatalf("queue first: %v", err)
	}

	_, err := svc.QueueRun(ctx, runsservice.QueueRunRequest{
		AgentProfileID: "agent-primary",
		Reason:         "same_reason",
		ActorKind:      models.ActorKindAgent,
		ActorID:        "agent-primary",
		IdempotencyKey: "task_comment:durable-2",
	})
	var refusal *runsservice.RefusalError
	if !errors.As(err, &refusal) {
		t.Fatalf("queue second: err = %v, want *RefusalError", err)
	}

	entries, err := repo.ListCausationRefusals(ctx, testWorkspaceID)
	if err != nil {
		t.Fatalf("list causation refusals: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("len(entries) = %d, want 1", len(entries))
	}
	entry := entries[0]
	if entry.Gate != string(runsservice.RefusalSelfTrigger) {
		t.Errorf("gate = %q, want %q", entry.Gate, runsservice.RefusalSelfTrigger)
	}
	if entry.AgentProfileID != "agent-primary" {
		t.Errorf("agent_profile_id = %q, want %q", entry.AgentProfileID, "agent-primary")
	}
	if entry.Reason != "same_reason" {
		t.Errorf("reason = %q, want %q", entry.Reason, "same_reason")
	}
}

// TestQueueRun_SelfTriggerTotalRefusalPersistsDurableCausationRefusal pins
// AC-OFFICE-LAUNCH-SAFETY-004.8's "record the refusal as in
// AC-OFFICE-LAUNCH-SAFETY-003.5" clause for the reason-independent total
// self-trigger gate.
func TestQueueRun_SelfTriggerTotalRefusalPersistsDurableCausationRefusal(t *testing.T) {
	svc, _, repo := newTestServiceWithRepo(t)
	svc.SetLaunchSafetyLimits(runsservice.DefaultMaxCausationDepth, 10, 2)
	ctx := context.Background()

	if _, err := svc.QueueRun(ctx, runsservice.QueueRunRequest{
		AgentProfileID: "agent-primary",
		Reason:         "reason_a",
		ActorKind:      models.ActorKindAgent,
		ActorID:        "agent-primary",
		IdempotencyKey: "task_comment:durable-total-1",
	}); err != nil {
		t.Fatalf("queue first: %v", err)
	}
	if _, err := svc.QueueRun(ctx, runsservice.QueueRunRequest{
		AgentProfileID: "agent-primary",
		Reason:         "reason_b",
		ActorKind:      models.ActorKindAgent,
		ActorID:        "agent-primary",
		IdempotencyKey: "task_comment:durable-total-2",
	}); err != nil {
		t.Fatalf("queue second: %v", err)
	}

	_, err := svc.QueueRun(ctx, runsservice.QueueRunRequest{
		AgentProfileID: "agent-primary",
		Reason:         "reason_c",
		ActorKind:      models.ActorKindAgent,
		ActorID:        "agent-primary",
		IdempotencyKey: "task_comment:durable-total-3",
	})
	var refusal *runsservice.RefusalError
	if !errors.As(err, &refusal) {
		t.Fatalf("queue third: err = %v, want *RefusalError", err)
	}

	entries, err := repo.ListCausationRefusals(ctx, testWorkspaceID)
	if err != nil {
		t.Fatalf("list causation refusals: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("len(entries) = %d, want 1", len(entries))
	}
	entry := entries[0]
	if entry.Gate != string(runsservice.RefusalSelfTriggerTotal) {
		t.Errorf("gate = %q, want %q", entry.Gate, runsservice.RefusalSelfTriggerTotal)
	}
	if entry.AgentProfileID != "agent-primary" {
		t.Errorf("agent_profile_id = %q, want %q", entry.AgentProfileID, "agent-primary")
	}
	if entry.Reason != "reason_c" {
		t.Errorf("reason = %q, want %q", entry.Reason, "reason_c")
	}
}
