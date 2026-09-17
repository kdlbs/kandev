package service_test

// Covers AC-OFFICE-RUN-CAUSATION-001.18/.24: a QueueRunRequest carrying
// CarrierCreatingRunID/CarrierCausationID/CarrierCausationDepth (in place
// of a live CausingRunID) inherits causation exactly as if the carrier's
// creating run were the causing run, without requiring a read of that
// run. This file exercises applyCausationLineage's carrier branch
// directly through QueueRun; office/service/run_causation_carrier_test.go
// covers the end-to-end read off a real task's metadata.

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/office/models"
	runsservice "github.com/kandev/kandev/internal/runs/service"
)

func boolPtr(b bool) *bool { return &b }

// TestQueueRun_CarrierCreatingRunIDInheritsLineage pins
// AC-OFFICE-RUN-CAUSATION-001.18: a non-empty CarrierCreatingRunID stands
// in for a live causing run, adopting its causation id as parent's
// causation id, its own id as parent run id, and depth + 1.
func TestQueueRun_CarrierCreatingRunIDInheritsLineage(t *testing.T) {
	svc, _, repo := newTestServiceWithRepo(t)
	ctx := context.Background()

	if _, err := svc.QueueRun(ctx, runsservice.QueueRunRequest{
		AgentProfileID: "agent-primary",
		Reason:         "creating_run_reason",
		ActorKind:      models.ActorKindAgent,
		ActorID:        "agent-primary",
	}); err != nil {
		t.Fatalf("queue creating run: %v", err)
	}
	creating := getRun(t, repo, "agent-primary", "creating_run_reason")

	if _, err := svc.QueueRun(ctx, runsservice.QueueRunRequest{
		AgentProfileID:        "agent-primary",
		Reason:                "carrier_followup",
		ActorKind:             models.ActorKindAgent,
		ActorID:               "some-agent",
		CarrierCreatingRunID:  creating.ID,
		CarrierCausationID:    creating.ChainCausationID,
		CarrierCausationDepth: creating.CausationDepth,
		CarrierHumanRooted:    boolPtr(creating.HumanRooted),
	}); err != nil {
		t.Fatalf("queue carrier follow-up: %v", err)
	}
	followUp := getRun(t, repo, "agent-primary", "carrier_followup")
	if followUp.ChainCausationID != creating.ChainCausationID {
		t.Errorf("causation_id = %q, want %q (carried)", followUp.ChainCausationID, creating.ChainCausationID)
	}
	if followUp.ParentRunID != creating.ID {
		t.Errorf("parent_run_id = %q, want %q (carrier's creating run)", followUp.ParentRunID, creating.ID)
	}
	if followUp.CausationDepth != creating.CausationDepth+1 {
		t.Errorf("causation_depth = %d, want %d", followUp.CausationDepth, creating.CausationDepth+1)
	}
}

// TestQueueRun_EmptyCarrierCreatingRunIDRoots pins
// AC-OFFICE-RUN-CAUSATION-001.24: an empty CarrierCreatingRunID roots the
// run regardless of what CarrierCausationID/CarrierCausationDepth say —
// the creating run identifier alone decides.
func TestQueueRun_EmptyCarrierCreatingRunIDRoots(t *testing.T) {
	svc, _, repo := newTestServiceWithRepo(t)
	ctx := context.Background()

	if _, err := svc.QueueRun(ctx, runsservice.QueueRunRequest{
		AgentProfileID:        "agent-primary",
		Reason:                "routine_fire_reason",
		ActorKind:             models.ActorKindSystem,
		CarrierCreatingRunID:  "",
		CarrierCausationID:    "should-be-ignored",
		CarrierCausationDepth: 7,
	}); err != nil {
		t.Fatalf("queue: %v", err)
	}
	run := getRun(t, repo, "agent-primary", "routine_fire_reason")
	if run.ChainCausationID != run.ID {
		t.Errorf("causation_id = %q, want own id %q (root)", run.ChainCausationID, run.ID)
	}
	if run.CausationDepth != 0 {
		t.Errorf("causation_depth = %d, want 0 (root)", run.CausationDepth)
	}
	if run.ParentRunID != "" {
		t.Errorf("parent_run_id = %q, want empty (root)", run.ParentRunID)
	}
}

// TestQueueRun_HumanActorDiscardsCarrierLineage pins
// AC-OFFICE-RUN-CAUSATION-001.9: a human actor always roots a new chain,
// even when the request carries a non-empty CarrierCreatingRunID.
func TestQueueRun_HumanActorDiscardsCarrierLineage(t *testing.T) {
	svc, _, repo := newTestServiceWithRepo(t)
	ctx := context.Background()

	if _, err := svc.QueueRun(ctx, runsservice.QueueRunRequest{
		AgentProfileID: "agent-primary",
		Reason:         "carrier_source",
		ActorKind:      models.ActorKindAgent,
		ActorID:        "agent-primary",
	}); err != nil {
		t.Fatalf("queue carrier source: %v", err)
	}
	source := getRun(t, repo, "agent-primary", "carrier_source")

	if _, err := svc.QueueRun(ctx, runsservice.QueueRunRequest{
		AgentProfileID:        "agent-primary",
		Reason:                "human_via_carrier",
		ActorKind:             models.ActorKindUser,
		ActorID:               "user-1",
		CarrierCreatingRunID:  source.ID,
		CarrierCausationID:    source.ChainCausationID,
		CarrierCausationDepth: source.CausationDepth,
		CarrierHumanRooted:    boolPtr(false),
	}); err != nil {
		t.Fatalf("queue human via carrier: %v", err)
	}
	run := getRun(t, repo, "agent-primary", "human_via_carrier")
	if run.ChainCausationID == source.ChainCausationID {
		t.Errorf("human actor adopted carrier's chain, want a new root")
	}
	if run.CausationDepth != 0 {
		t.Errorf("causation_depth = %d, want 0 (human roots)", run.CausationDepth)
	}
	if !run.HumanRooted {
		t.Error("human_rooted = false, want true (actor is user, overrides carrier)")
	}
}

// TestQueueRun_CarrierHumanRootedInheritsAcrossBoundary pins
// AC-OFFICE-RUN-CAUSATION-001.13: CarrierHumanRooted propagates the
// human-rooted flag across a task boundary without a live CausingRunID.
func TestQueueRun_CarrierHumanRootedInheritsAcrossBoundary(t *testing.T) {
	svc, _, repo := newTestServiceWithRepo(t)
	ctx := context.Background()

	if _, err := svc.QueueRun(ctx, runsservice.QueueRunRequest{
		AgentProfileID:     "agent-primary",
		Reason:             "carrier_human_rooted",
		ActorKind:          models.ActorKindAgent,
		ActorID:            "agent-primary",
		CarrierHumanRooted: boolPtr(true),
	}); err != nil {
		t.Fatalf("queue: %v", err)
	}
	run := getRun(t, repo, "agent-primary", "carrier_human_rooted")
	if !run.HumanRooted {
		t.Error("human_rooted = false, want true (carried across the task boundary)")
	}
}
