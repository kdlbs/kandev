package orchestrator

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
)

// seam5Payload builds a minimal AC-42f payload for a fake relaunch, keyed by
// the given session id, so these unit tests can exercise admitOrDeferSeam5
// without going through the full relaunchDynamicTaskAfterFailure body.
func seam5Payload(sessionID string) map[string]interface{} {
	return seam5DynamicRelaunchPayload(watcher.AgentEventData{
		TaskID:           "irrelevant-for-payload",
		SessionID:        sessionID,
		AgentExecutionID: "exec-1",
		AgentProfileID:   "agent-1",
	}, "profile-1")
}

// TestAdmitOrDeferSeam5AdmitsUnderCeiling covers the ordinary case: with no
// lister wired (as in every other seam's unit test), the session is never
// reported as already counted, so handOffOrAdmit falls through to an
// ordinary admission and behaves exactly like admit() while under the
// ceiling.
func TestAdmitOrDeferSeam5AdmitsUnderCeiling(t *testing.T) {
	svc, repo := newServiceWithCeiling(t, 5)
	ctx := context.Background()
	if err := repo.CreateTask(ctx, &models.Task{ID: "seam5-admit", Title: "T"}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	reservation, deferred, err := svc.admitOrDeferSeam5(ctx, "seam5-admit", launchOriginAutomatic, seam5Payload("seam5-admit-session"))
	if err != nil {
		t.Fatalf("admitOrDeferSeam5: %v", err)
	}
	if deferred {
		t.Fatal("relaunch was deferred while under the ceiling")
	}
	if reservation == nil || reservation.key != "seam5-admit-session" {
		t.Fatalf("reservation not keyed by the session being relaunched: %+v", reservation)
	}

	record := deferredLaunchOf(t, svc, "seam5-admit")
	if record != nil {
		t.Fatalf("a deferred_launch record was written for an admitted relaunch: %+v", record)
	}
}

// TestAdmitOrDeferSeam5DefersAutomaticOverCeiling covers AC-38a/AC-42f: with
// the predecessor session not counted (the two LaunchDynamicRouteAction
// callers reach this after markDynamicRouteActionRequired has already
// parked the route), an automatic relaunch at the ceiling is an ordinary
// refusal and its "dynamic_relaunch" payload is recorded, not dropped.
func TestAdmitOrDeferSeam5DefersAutomaticOverCeiling(t *testing.T) {
	svc, repo := newServiceWithCeiling(t, 1)
	ctx := context.Background()
	for _, id := range []string{"seam5-first", "seam5-second"} {
		if err := repo.CreateTask(ctx, &models.Task{ID: id, Title: "T"}); err != nil {
			t.Fatalf("CreateTask(%s): %v", id, err)
		}
	}

	if _, deferred, err := svc.admitOrDeferSeam5(ctx, "seam5-first", launchOriginAutomatic, seam5Payload("seam5-first-session")); err != nil || deferred {
		t.Fatalf("first relaunch was not admitted: deferred=%v err=%v", deferred, err)
	}

	reservation, deferred, err := svc.admitOrDeferSeam5(ctx, "seam5-second", launchOriginAutomatic, seam5Payload("seam5-second-session"))
	if err != nil {
		t.Fatalf("admitOrDeferSeam5: %v", err)
	}
	if !deferred {
		t.Fatal("second automatic relaunch over the ceiling was admitted, want deferred")
	}
	if reservation != nil {
		t.Fatal("a deferred relaunch must not hold a reservation")
	}

	record := deferredLaunchOf(t, svc, "seam5-second")
	if record == nil || record[models.CeilingDeferredKey] != true {
		t.Fatalf("the refused relaunch's payload was not recorded: %+v", record)
	}
	if record[models.CeilingLaunchKindKey] != string(models.CeilingLaunchDynamicRelaunch) {
		t.Fatalf("ceiling_launch_kind = %v, want %q", record[models.CeilingLaunchKindKey], models.CeilingLaunchDynamicRelaunch)
	}
	nested, _ := record[models.CeilingLaunchPayloadKey].(map[string]interface{})
	if nested["agent_execution_id"] != "exec-1" {
		t.Fatalf("the recorded payload does not match the refused relaunch: %+v", record)
	}
}

// TestAdmitOrDeferSeam5AdmitsManualOverCeiling covers AC-14/AC-38a: a manual
// relaunch (the UI's manual retry/try-next action) is always admitted, never
// deferred, even at the ceiling.
func TestAdmitOrDeferSeam5AdmitsManualOverCeiling(t *testing.T) {
	svc, repo := newServiceWithCeiling(t, 1)
	ctx := context.Background()
	for _, id := range []string{"seam5-manual-first", "seam5-manual-second"} {
		if err := repo.CreateTask(ctx, &models.Task{ID: id, Title: "T"}); err != nil {
			t.Fatalf("CreateTask(%s): %v", id, err)
		}
	}
	if _, deferred, err := svc.admitOrDeferSeam5(ctx, "seam5-manual-first", launchOriginAutomatic, seam5Payload("seam5-manual-first-session")); err != nil || deferred {
		t.Fatalf("first relaunch was not admitted: deferred=%v err=%v", deferred, err)
	}

	reservation, deferred, err := svc.admitOrDeferSeam5(ctx, "seam5-manual-second", launchOriginManual, seam5Payload("seam5-manual-second-session"))
	if err != nil {
		t.Fatalf("admitOrDeferSeam5: %v", err)
	}
	if deferred {
		t.Fatal("a manual relaunch was deferred; AC-14 requires it always be admitted")
	}
	if reservation == nil {
		t.Fatal("a manual override still consumes a reservation")
	}
	if record := deferredLaunchOf(t, svc, "seam5-manual-second"); record != nil {
		t.Fatalf("a manual override must never write a ceiling_deferred record: %+v", record)
	}
}

// TestAdmitOrDeferSeam5ReportsWriteFailure covers AC-45: a refusal that
// cannot be persisted must not be swallowed.
func TestAdmitOrDeferSeam5ReportsWriteFailure(t *testing.T) {
	svc, _ := newServiceWithCeiling(t, 1)
	ctx := context.Background()
	svc.sessionCeiling.admit(ctx, admissionRequest{taskID: "filler", sessionID: "filler-session", origin: launchOriginAutomatic, seam: "relaunchDynamicTaskAfterFailure"})

	_, _, err := svc.admitOrDeferSeam5(ctx, "missing-task", launchOriginAutomatic, seam5Payload("missing-session"))
	if err == nil {
		t.Fatal("expected an error when the refusal could not be persisted")
	}
}

// TestAdmitOrDeferSeam5UnsetOriginDefaultsAutomatic pins AC-13b: an
// unset/invalid origin must classify as automatic, not fall through as an
// unconditional manual admit.
func TestAdmitOrDeferSeam5UnsetOriginDefaultsAutomatic(t *testing.T) {
	svc, repo := newServiceWithCeiling(t, 1)
	ctx := context.Background()
	for _, id := range []string{"seam5-unset-first", "seam5-unset-second"} {
		if err := repo.CreateTask(ctx, &models.Task{ID: id, Title: "T"}); err != nil {
			t.Fatalf("CreateTask(%s): %v", id, err)
		}
	}
	if _, deferred, err := svc.admitOrDeferSeam5(ctx, "seam5-unset-first", launchOriginAutomatic, seam5Payload("seam5-unset-first-session")); err != nil || deferred {
		t.Fatalf("first relaunch was not admitted: deferred=%v err=%v", deferred, err)
	}

	// launchOrigin("") is neither "manual" nor "automatic".
	_, deferred, err := svc.admitOrDeferSeam5(ctx, "seam5-unset-second", launchOrigin(""), seam5Payload("seam5-unset-second-session"))
	if err != nil {
		t.Fatalf("admitOrDeferSeam5: %v", err)
	}
	if !deferred {
		t.Fatal("an unset origin over the ceiling must default to automatic and be deferred, not admitted as manual")
	}
}
