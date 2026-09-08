package routines_test

import (
	"context"
	"expvar"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/office/models"
)

// loopCounterInt reads one labelled entry off a registered
// office_loop_* expvar map. Missing entries read as zero so a test can
// assert a fresh delta without pre-seeding the map.
func loopCounterInt(t *testing.T, mapName, key string) int64 {
	t.Helper()
	v := expvar.Get(mapName)
	if v == nil {
		t.Fatalf("expvar map %s not registered", mapName)
	}
	m, ok := v.(*expvar.Map)
	if !ok {
		t.Fatalf("%s is not a Map", mapName)
	}
	sub := m.Get(key)
	if sub == nil {
		return 0
	}
	iv, ok := sub.(*expvar.Int)
	if !ok {
		t.Fatalf("%s[%q] is not an Int", mapName, key)
	}
	return iv.Value()
}

// REQ-OFFICE-LOOP-LIVENESS-003 / AC-003.1: TickScheduledTriggers counts
// one cron loop tick evaluation regardless of whether any trigger is due.
func TestTickScheduledTriggers_IncrementsCronTickCounter(t *testing.T) {
	svc := newTestRoutineService(t)
	ctx := context.Background()

	before := loopCounterInt(t, "office_loop_cron_tick_total", "")
	if err := svc.TickScheduledTriggers(ctx, time.Now().UTC()); err != nil {
		t.Fatalf("tick: %v", err)
	}
	after := loopCounterInt(t, "office_loop_cron_tick_total", "")
	if after != before+1 {
		t.Fatalf("cron_tick delta = %d, want 1", after-before)
	}
}

// AC-003.1/AC-003.9: a due, successfully-claimed trigger increments
// office_loop_trigger_claimed_total labelled by the owning routine's
// workspace, at the site the claim persists.
func TestTickScheduledTriggers_IncrementsTriggerClaimedCounter(t *testing.T) {
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

	key := "workspace=" + routine.WorkspaceID
	before := loopCounterInt(t, "office_loop_trigger_claimed_total", key)

	if err := svc.TickScheduledTriggers(ctx, time.Now().UTC().Add(2*time.Minute)); err != nil {
		t.Fatalf("tick: %v", err)
	}

	after := loopCounterInt(t, "office_loop_trigger_claimed_total", key)
	if after != before+1 {
		t.Fatalf("trigger_claimed delta = %d, want 1", after-before)
	}
}

// AC-003.9: a claim that persists must count even when the routine
// lookup afterward fails, so the claim can't be attributed to a
// workspace. Without the fallback, a claim that already wrote to the
// database silently vanished from office_loop_trigger_claimed_total
// (Review round 1, R1-4) — the counter must reflect the persisted
// write, not the lookup that happens to follow it.
func TestTickScheduledTriggers_ClaimedCounterSurvivesRoutineLookupFailure(t *testing.T) {
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

	// Orphan the trigger: delete the routine it points at without
	// touching the trigger row, so ClaimTrigger still succeeds but the
	// subsequent GetRoutineFromConfig fails.
	if err := svc.DeleteRoutine(ctx, routine.ID); err != nil {
		t.Fatalf("delete routine: %v", err)
	}

	key := "workspace=_unattributed"
	before := loopCounterInt(t, "office_loop_trigger_claimed_total", key)

	if err := svc.TickScheduledTriggers(ctx, time.Now().UTC().Add(2*time.Minute)); err != nil {
		t.Fatalf("tick: %v", err)
	}

	after := loopCounterInt(t, "office_loop_trigger_claimed_total", key)
	if after != before+1 {
		t.Fatalf("trigger_claimed[_unattributed] delta = %d, want 1", after-before)
	}
}

// AC-003.2: a fire that runs to completion (lightweight, always_create)
// counts office_loop_routine_run_total with disposition=task_created.
func TestDispatchRoutineRun_CountsTaskCreatedDisposition(t *testing.T) {
	svc := newTestRoutineService(t)
	ctx := context.Background()

	routine := newLightweightTestRoutine(t, svc)
	enq := &fakeWakeupEnqueuer{}
	svc.SetWakeupEnqueuer(enq)

	key := "workspace=" + routine.WorkspaceID + ";source=manual;disposition=task_created"
	before := loopCounterInt(t, "office_loop_routine_run_total", key)

	if _, err := svc.FireManual(ctx, routine.ID, nil); err != nil {
		t.Fatalf("fire manual: %v", err)
	}

	after := loopCounterInt(t, "office_loop_routine_run_total", key)
	if after != before+1 {
		t.Fatalf("routine_run[task_created] delta = %d, want 1", after-before)
	}
}

// AC-003.2: a fire that the concurrency policy skips counts
// office_loop_routine_run_total with disposition=skipped, distinct from
// the launched fire that decided the skip.
func TestDispatchRoutineRun_CountsSkippedDisposition(t *testing.T) {
	svc := newTestRoutineService(t)
	ctx := context.Background()
	svc.SetWorkflowEnsurer(&fakeWorkflowEnsurer{})
	svc.SetTaskCreator(&fakeTaskCreator{})

	routine := createTestRoutine(t, svc, "Skip Counter", "skip_if_active")

	if _, err := svc.FireManual(ctx, routine.ID, nil); err != nil {
		t.Fatalf("first fire: %v", err)
	}

	key := "workspace=" + routine.WorkspaceID + ";source=manual;disposition=skipped"
	before := loopCounterInt(t, "office_loop_routine_run_total", key)

	if _, err := svc.FireManual(ctx, routine.ID, nil); err != nil {
		t.Fatalf("second fire: %v", err)
	}

	after := loopCounterInt(t, "office_loop_routine_run_total", key)
	if after != before+1 {
		t.Fatalf("routine_run[skipped] delta = %d, want 1", after-before)
	}
}

// AC-003.2: a fire the concurrency policy coalesces into an existing
// active run counts office_loop_routine_run_total with
// disposition=coalesced.
func TestDispatchRoutineRun_CountsCoalescedDisposition(t *testing.T) {
	svc := newTestRoutineService(t)
	ctx := context.Background()
	svc.SetWorkflowEnsurer(&fakeWorkflowEnsurer{})
	svc.SetTaskCreator(&fakeTaskCreator{})

	routine := createTestRoutine(t, svc, "Coalesce Counter", "coalesce_if_active")

	if _, err := svc.FireManual(ctx, routine.ID, nil); err != nil {
		t.Fatalf("first fire: %v", err)
	}

	key := "workspace=" + routine.WorkspaceID + ";source=manual;disposition=coalesced"
	before := loopCounterInt(t, "office_loop_routine_run_total", key)

	if _, err := svc.FireManual(ctx, routine.ID, nil); err != nil {
		t.Fatalf("second fire: %v", err)
	}

	after := loopCounterInt(t, "office_loop_routine_run_total", key)
	if after != before+1 {
		t.Fatalf("routine_run[coalesced] delta = %d, want 1", after-before)
	}
}

// AC-003.1: the lightweight path's successfully-created wakeup request
// counts office_loop_wakeup_created_total labelled by the row's own
// source column, reusing routines.WakeupRequest.Source rather than
// inventing a parallel vocabulary.
func TestMaterialiseLightweightRoutineRun_IncrementsWakeupCreatedCounter(t *testing.T) {
	svc := newTestRoutineService(t)
	ctx := context.Background()

	routine := newLightweightTestRoutine(t, svc)
	enq := &fakeWakeupEnqueuer{}
	svc.SetWakeupEnqueuer(enq)

	key := "workspace=" + routine.WorkspaceID + ";source=routine"
	before := loopCounterInt(t, "office_loop_wakeup_created_total", key)

	if _, err := svc.FireManual(ctx, routine.ID, nil); err != nil {
		t.Fatalf("fire manual: %v", err)
	}
	if len(enq.created) != 1 {
		t.Fatalf("expected 1 wakeup request created, got %d", len(enq.created))
	}

	after := loopCounterInt(t, "office_loop_wakeup_created_total", key)
	if after != before+1 {
		t.Fatalf("wakeup_created delta = %d, want 1", after-before)
	}
}
