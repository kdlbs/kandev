package routines_test

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/shared"
)

// TestDispatch_LightweightRoutine_ReasonIsPeriodicTasklessWake is the WO-46
// anti-drift lock. It drives the reason through the real writer
// (RoutineService.materialiseLightweightRoutineRun, via FireManual) rather
// than asserting against a copied literal, so the writer and the
// office/service idle-skip gate's shared.IsPeriodicTasklessWake predicate
// cannot silently diverge again the way RunReasonHeartbeat did.
func TestDispatch_LightweightRoutine_ReasonIsPeriodicTasklessWake(t *testing.T) {
	svc := newTestRoutineService(t)
	ctx := context.Background()

	routine := &models.Routine{
		WorkspaceID:            "ws-1",
		Name:                   "Lightweight",
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

	if _, err := svc.FireManual(ctx, routine.ID, map[string]string{"name": "alpha"}); err != nil {
		t.Fatalf("fire manual: %v", err)
	}
	if len(enq.created) != 1 {
		t.Fatalf("expected 1 wakeup-request created, got %d", len(enq.created))
	}

	got := enq.created[0].Reason
	if !shared.IsPeriodicTasklessWake(got) {
		t.Errorf("shared.IsPeriodicTasklessWake(%q) = false, want true — "+
			"the idle-skip gate would silently stop recognizing production "+
			"routine-dispatch runs", got)
	}
}
