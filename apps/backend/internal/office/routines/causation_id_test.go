package routines_test

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/office/models"
)

// AC-OFFICE-LOOP-LIVENESS-002.1: dispatchRoutineRun mints one causation
// id per fire — non-empty and distinct across separate fires.
func TestDispatchRoutineRun_MintsOneCausationIDPerFire(t *testing.T) {
	svc := newTestRoutineService(t)
	ctx := context.Background()
	routine := createTestRoutine(t, svc, "Causation Mint", "always_create")

	first, err := svc.FireManual(ctx, routine.ID, nil)
	if err != nil {
		t.Fatalf("first fire: %v", err)
	}
	if first.CausationID == "" {
		t.Fatal("expected non-empty causation id on first fire")
	}

	second, err := svc.FireManual(ctx, routine.ID, nil)
	if err != nil {
		t.Fatalf("second fire: %v", err)
	}
	if second.CausationID == "" {
		t.Fatal("expected non-empty causation id on second fire")
	}
	if second.CausationID == first.CausationID {
		t.Fatalf("expected distinct causation ids across fires, both were %q", first.CausationID)
	}
}

// AC-OFFICE-LOOP-LIVENESS-002.2: the lightweight (taskless) path copies
// the fire's minted causation id onto the WakeupRequest it builds.
func TestDispatchRoutineRun_LightweightPathCopiesCausationIDOntoWakeupRequest(t *testing.T) {
	svc := newTestRoutineService(t)
	ctx := context.Background()

	routine := &models.Routine{
		WorkspaceID:            "ws-1",
		Name:                   "Lightweight Causation",
		TaskTemplate:           "", // lightweight
		AssigneeAgentProfileID: "agent-1",
		Status:                 "active",
		ConcurrencyPolicy:      "always_create",
	}
	if err := svc.CreateRoutine(ctx, routine); err != nil {
		t.Fatalf("create routine: %v", err)
	}

	enq := &fakeWakeupEnqueuer{}
	svc.SetWakeupEnqueuer(enq)

	run, err := svc.FireManual(ctx, routine.ID, map[string]string{"name": "alpha"})
	if err != nil {
		t.Fatalf("fire manual: %v", err)
	}
	if run.CausationID == "" {
		t.Fatal("expected non-empty causation id on the routine run")
	}
	if len(enq.created) != 1 {
		t.Fatalf("expected 1 wakeup request created, got %d", len(enq.created))
	}
	if enq.created[0].CausationID != run.CausationID {
		t.Fatalf("wakeup request causation id = %q, want %q (copied from the fire)",
			enq.created[0].CausationID, run.CausationID)
	}
}
