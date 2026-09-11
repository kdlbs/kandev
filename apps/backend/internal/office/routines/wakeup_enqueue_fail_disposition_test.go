package routines_test

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/routines"
)

// failingCreateWakeupEnqueuer fails every CreateWakeupRequest call with a
// generic (non-ErrWakeupAlreadyRequested) error, but still lets Dispatch/
// FailWakeupRequest succeed trivially — they are never reached once
// CreateWakeupRequest itself fails.
type failingCreateWakeupEnqueuer struct{ fakeWakeupEnqueuer }

func (f *failingCreateWakeupEnqueuer) CreateWakeupRequest(context.Context, *routines.WakeupRequest) error {
	return errors.New("wakeup store unavailable")
}

// TestDispatchRoutineRun_FailedWakeupEnqueueDoesNotReportTaskCreated is the
// PR fixup round 2 regression test (CodeRabbit). materialiseLightweightRoutineRun
// finalizes a run as RoutineRunStatusFailed when CreateWakeupRequest errors,
// but returns a nil error once that finalizing UPDATE itself succeeds. The
// caller (dispatchRoutineRun) used to read that nil error as "materialise
// succeeded" and hard-code disposition = task_created regardless of what
// status was actually persisted — so a routine run that genuinely failed to
// enqueue its wakeup still recorded office_loop_routine_run_total as a
// success, corrupting the loop-health evidence this capability exists to
// produce.
func TestDispatchRoutineRun_FailedWakeupEnqueueDoesNotReportTaskCreated(t *testing.T) {
	svc := newTestRoutineService(t)
	ctx := context.Background()

	routine := newLightweightTestRoutine(t, svc)
	svc.SetWakeupEnqueuer(&failingCreateWakeupEnqueuer{})

	failedKey := "workspace=" + routine.WorkspaceID + ";source=manual;disposition=failed"
	taskCreatedKey := "workspace=" + routine.WorkspaceID + ";source=manual;disposition=task_created"
	failedBefore := loopCounterInt(t, "office_loop_routine_run_total", failedKey)
	taskCreatedBefore := loopCounterInt(t, "office_loop_routine_run_total", taskCreatedKey)

	run, err := svc.FireManual(ctx, routine.ID, nil)
	if err != nil {
		t.Fatalf("FireManual: %v", err)
	}
	if run.Status != models.RoutineRunStatusFailed {
		t.Fatalf("run.Status = %q, want %q — the wakeup enqueue failed and finalizeLightweightRun persisted 'failed'",
			run.Status, models.RoutineRunStatusFailed)
	}

	failedAfter := loopCounterInt(t, "office_loop_routine_run_total", failedKey)
	if failedAfter != failedBefore+1 {
		t.Fatalf("routine_run[failed] delta = %d, want 1 — a failed wakeup enqueue must not be "+
			"reported as disposition=task_created", failedAfter-failedBefore)
	}

	taskCreatedAfter := loopCounterInt(t, "office_loop_routine_run_total", taskCreatedKey)
	if taskCreatedAfter != taskCreatedBefore {
		t.Fatalf("routine_run[task_created] delta = %d, want 0 for a run whose wakeup enqueue failed",
			taskCreatedAfter-taskCreatedBefore)
	}
}
