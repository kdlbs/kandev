package routines_test

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/routines"
	"github.com/kandev/kandev/internal/office/shared"
)

// newLightweightTestRoutine builds an active, taskless routine assigned
// to agent-1. Shared setup for both halves of the WO-46 anti-drift lock
// below.
func newLightweightTestRoutine(t *testing.T, svc *routines.RoutineService) *models.Routine {
	t.Helper()
	routine := &models.Routine{
		WorkspaceID:            "ws-1",
		Name:                   "Lightweight",
		TaskTemplate:           "", // lightweight
		AssigneeAgentProfileID: "agent-1",
		Status:                 "active",
		ConcurrencyPolicy:      "always_create",
	}
	if err := svc.CreateRoutine(context.Background(), routine); err != nil {
		t.Fatalf("create routine: %v", err)
	}
	return routine
}

// TestDispatch_LightweightRoutine_ManualFireIsNotPeriodicTasklessWake is
// half of the WO-46 anti-drift lock (Review round 1, Blocker 2). A manual
// "Fire now" is event/user-triggered, not periodic — per
// docs/specs/office/scheduler.md ("Event-triggered wakeups always
// proceed - the skip applies only to periodic wakes") the idle-skip gate
// must never swallow it. Drives the reason through the real writer
// (materialiseLightweightRoutineRun via FireManual) so a future change
// that makes FireManual periodic-tagged again fails here first — this is
// the case Review round 1 found: the original version of this test
// asserted the opposite of what production behavior should be.
func TestDispatch_LightweightRoutine_ManualFireIsNotPeriodicTasklessWake(t *testing.T) {
	svc := newTestRoutineService(t)
	ctx := context.Background()

	routine := newLightweightTestRoutine(t, svc)

	enq := &fakeWakeupEnqueuer{}
	svc.SetWakeupEnqueuer(enq)

	if _, err := svc.FireManual(ctx, routine.ID, map[string]string{"name": "alpha"}); err != nil {
		t.Fatalf("fire manual: %v", err)
	}
	if len(enq.created) != 1 {
		t.Fatalf("expected 1 wakeup-request created, got %d", len(enq.created))
	}

	got := enq.created[0].Reason
	if shared.IsPeriodicTasklessWake(got) {
		t.Errorf("shared.IsPeriodicTasklessWake(%q) = true, want false — "+
			"a manual Fire-now would be silently swallowed by the idle-skip "+
			"gate for any SkipIdleRuns assignee with no actionable tasks", got)
	}
}

// TestDispatch_LightweightRoutine_CronFireIsPeriodicTasklessWake is the
// other half of the WO-46 anti-drift lock. A cron-driven fire is the
// only routine trigger that represents a periodic, unattended wake —
// the class the idle-skip gate exists to catch. Drives the reason
// through the real cron path (TickScheduledTriggers →
// materialiseLightweightRoutineRun) rather than asserting against a
// copied literal, so the writer and shared.IsPeriodicTasklessWake
// cannot silently diverge again the way RunReasonHeartbeat did.
func TestDispatch_LightweightRoutine_CronFireIsPeriodicTasklessWake(t *testing.T) {
	svc := newTestRoutineService(t)
	ctx := context.Background()

	routine := newLightweightTestRoutine(t, svc)
	if err := svc.CreateRoutineTrigger(ctx, &models.RoutineTrigger{
		RoutineID:      routine.ID,
		Kind:           "cron",
		CronExpression: "* * * * *",
		Timezone:       "UTC",
		Enabled:        true,
	}); err != nil {
		t.Fatalf("create trigger: %v", err)
	}

	enq := &fakeWakeupEnqueuer{}
	svc.SetWakeupEnqueuer(enq)

	// The trigger's next_run_at is the next full minute after creation;
	// ticking 2 minutes out guarantees it is due.
	if err := svc.TickScheduledTriggers(ctx, time.Now().UTC().Add(2*time.Minute)); err != nil {
		t.Fatalf("tick scheduled triggers: %v", err)
	}
	if len(enq.created) != 1 {
		t.Fatalf("expected 1 wakeup-request created, got %d", len(enq.created))
	}

	got := enq.created[0].Reason
	if !shared.IsPeriodicTasklessWake(got) {
		t.Errorf("shared.IsPeriodicTasklessWake(%q) = false, want true — "+
			"the idle-skip gate would silently stop recognizing production "+
			"cron-triggered routine-dispatch runs", got)
	}
}
