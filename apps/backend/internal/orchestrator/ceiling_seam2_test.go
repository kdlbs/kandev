package orchestrator

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

// TestAdmitOrDeferSeam2AdmitsUnderCeiling covers the ordinary case: population
// below the ceiling, admitted, no record written, reservation keyed by the
// session that already exists.
func TestAdmitOrDeferSeam2AdmitsUnderCeiling(t *testing.T) {
	svc, repo := newServiceWithCeiling(t, 5)
	ctx := context.Background()
	if err := repo.CreateTask(ctx, &models.Task{ID: "seam2-admit", Title: "T"}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	reservation, deferred, err := svc.admitOrDeferSeam2(ctx, "seam2-admit", "seam2-admit-session", launchOriginAutomatic, map[string]interface{}{"prompt": "p"})
	if err != nil {
		t.Fatalf("admitOrDeferSeam2: %v", err)
	}
	if deferred {
		t.Fatal("launch was deferred while under the ceiling")
	}
	if reservation == nil || reservation.key != "seam2-admit-session" {
		t.Fatalf("reservation not keyed by the existing session id: %+v", reservation)
	}

	record := deferredLaunchOf(t, svc, "seam2-admit")
	if record != nil {
		t.Fatalf("a deferred_launch record was written for an admitted launch: %+v", record)
	}
}

// TestAdmitOrDeferSeam2DefersAutomaticOverCeiling covers AC-11/AC-42d: an
// automatic launch at the ceiling is refused and its start_created payload is
// recorded, not dropped.
func TestAdmitOrDeferSeam2DefersAutomaticOverCeiling(t *testing.T) {
	svc, repo := newServiceWithCeiling(t, 1)
	ctx := context.Background()
	for _, id := range []string{"seam2-first", "seam2-second"} {
		if err := repo.CreateTask(ctx, &models.Task{ID: id, Title: "T"}); err != nil {
			t.Fatalf("CreateTask(%s): %v", id, err)
		}
	}

	if _, deferred, err := svc.admitOrDeferSeam2(ctx, "seam2-first", "seam2-first-session", launchOriginAutomatic, map[string]interface{}{"prompt": "first"}); err != nil || deferred {
		t.Fatalf("first launch was not admitted: deferred=%v err=%v", deferred, err)
	}

	reservation, deferred, err := svc.admitOrDeferSeam2(ctx, "seam2-second", "seam2-second-session", launchOriginAutomatic, map[string]interface{}{"prompt": "second"})
	if err != nil {
		t.Fatalf("admitOrDeferSeam2: %v", err)
	}
	if !deferred {
		t.Fatal("second automatic launch over the ceiling was admitted, want deferred")
	}
	if reservation != nil {
		t.Fatal("a deferred launch must not hold a reservation")
	}

	record := deferredLaunchOf(t, svc, "seam2-second")
	if record == nil || record[models.CeilingDeferredKey] != true {
		t.Fatalf("the refused launch's payload was not recorded: %+v", record)
	}
	if record[models.CeilingLaunchKindKey] != string(models.CeilingLaunchStartCreated) {
		t.Fatalf("ceiling_launch_kind = %v, want %q", record[models.CeilingLaunchKindKey], models.CeilingLaunchStartCreated)
	}
	nested, _ := record[models.CeilingLaunchPayloadKey].(map[string]interface{})
	if nested["prompt"] != "second" {
		t.Fatalf("the recorded payload does not match the refused launch: %+v", record)
	}
}

// TestAdmitOrDeferSeam2AdmitsManualOverCeiling covers AC-14: a manual launch is
// always admitted, never deferred, even at the ceiling.
func TestAdmitOrDeferSeam2AdmitsManualOverCeiling(t *testing.T) {
	svc, repo := newServiceWithCeiling(t, 1)
	ctx := context.Background()
	for _, id := range []string{"seam2-manual-first", "seam2-manual-second"} {
		if err := repo.CreateTask(ctx, &models.Task{ID: id, Title: "T"}); err != nil {
			t.Fatalf("CreateTask(%s): %v", id, err)
		}
	}
	if _, deferred, err := svc.admitOrDeferSeam2(ctx, "seam2-manual-first", "seam2-manual-first-session", launchOriginAutomatic, map[string]interface{}{"prompt": "first"}); err != nil || deferred {
		t.Fatalf("first launch was not admitted: deferred=%v err=%v", deferred, err)
	}

	reservation, deferred, err := svc.admitOrDeferSeam2(ctx, "seam2-manual-second", "seam2-manual-second-session", launchOriginManual, map[string]interface{}{"prompt": "second"})
	if err != nil {
		t.Fatalf("admitOrDeferSeam2: %v", err)
	}
	if deferred {
		t.Fatal("a manual launch was deferred; AC-14 requires it always be admitted")
	}
	if reservation == nil {
		t.Fatal("a manual override still consumes a reservation")
	}
	if record := deferredLaunchOf(t, svc, "seam2-manual-second"); record != nil {
		t.Fatalf("a manual override must never write a ceiling_deferred record: %+v", record)
	}
}

// TestSeam2ReservationRekeyMovesThePopulationUnit pins AC-4e: the redirect to
// an already-active session must move the reservation, not release-then-admit
// a second unit for the same launch.
func TestSeam2ReservationRekeyMovesThePopulationUnit(t *testing.T) {
	ctx := context.Background()
	controller := newSessionCeilingController(1, nil, nil)
	decision := controller.admit(ctx, admissionRequest{taskID: "t", sessionID: "gated-session", origin: launchOriginAutomatic, seam: "startCreatedSession"})
	if !decision.admitted {
		t.Fatal("expected admission")
	}
	reservation := &sessionKeyedCeilingReservation{controller: controller, key: decision.reservationKey}

	reservation.rekeyToSession(ctx, "redirect-session")
	if reservation.key != "redirect-session" {
		t.Fatalf("reservation key = %q, want %q", reservation.key, "redirect-session")
	}

	population, err := controller.population(ctx)
	if err != nil {
		t.Fatalf("population: %v", err)
	}
	if population != 1 {
		t.Fatalf("population = %d, want 1 (rekey must not consume a second unit)", population)
	}

	// A second launch should now be refused: the ceiling's only slot is still
	// held, under the redirected key.
	second := controller.admit(ctx, admissionRequest{taskID: "t2", sessionID: "other-session", origin: launchOriginAutomatic, seam: "startCreatedSession"})
	if second.admitted {
		t.Fatal("the ceiling's slot should still be held after rekey")
	}

	reservation.consume()
	reservation.releaseIfNotConsumed()
	population, err = controller.population(ctx)
	if err != nil {
		t.Fatalf("population: %v", err)
	}
	if population != 1 {
		t.Fatalf("population = %d, want 1 (consumed reservation must not be released)", population)
	}
}

// TestAdmitOrDeferSeam2ReportsWriteFailure covers AC-45: a refusal that cannot
// be persisted must not be swallowed.
func TestAdmitOrDeferSeam2ReportsWriteFailure(t *testing.T) {
	svc, _ := newServiceWithCeiling(t, 1)
	ctx := context.Background()
	svc.sessionCeiling.admit(ctx, admissionRequest{taskID: "filler", sessionID: "filler-session", origin: launchOriginAutomatic, seam: "startCreatedSession"})

	_, _, err := svc.admitOrDeferSeam2(ctx, "missing-task", "missing-session", launchOriginAutomatic, map[string]interface{}{"prompt": "p"})
	if err == nil {
		t.Fatal("expected an error when the refusal could not be persisted")
	}
}
