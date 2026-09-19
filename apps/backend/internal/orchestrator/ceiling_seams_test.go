package orchestrator

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
)

func newServiceWithCeiling(t *testing.T, ceiling int) (*Service, *tasksqlite.Repository) {
	t.Helper()
	svc, repo := newServiceWithRealRepo(t)
	svc.sessionCeiling = newSessionCeilingController(ceiling, nil, nil)
	return svc, repo
}

// TestAdmitOrDeferSeam1AdmitsUnderCeiling covers the ordinary case: population
// below the ceiling, admitted, no record written.
func TestAdmitOrDeferSeam1AdmitsUnderCeiling(t *testing.T) {
	svc, repo := newServiceWithCeiling(t, 5)
	ctx := context.Background()
	if err := repo.CreateTask(ctx, &models.Task{ID: "seam1-admit", Title: "T"}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	reservation, deferred, err := svc.admitOrDeferSeam1(ctx, "seam1-admit", launchOriginAutomatic, map[string]interface{}{"prompt": "p"})
	if err != nil {
		t.Fatalf("admitOrDeferSeam1: %v", err)
	}
	if deferred {
		t.Fatal("launch was deferred while under the ceiling")
	}
	if reservation == nil || reservation.key == "" {
		t.Fatal("no reservation was returned for an admitted launch")
	}

	record := deferredLaunchOf(t, svc, "seam1-admit")
	if record != nil {
		t.Fatalf("a deferred_launch record was written for an admitted launch: %+v", record)
	}
}

// TestAdmitOrDeferSeam1DefersAutomaticOverCeiling covers AC-11: an automatic
// launch at the ceiling is refused and its payload is recorded, not dropped.
func TestAdmitOrDeferSeam1DefersAutomaticOverCeiling(t *testing.T) {
	svc, repo := newServiceWithCeiling(t, 1)
	ctx := context.Background()
	for _, id := range []string{"seam1-first", "seam1-second"} {
		if err := repo.CreateTask(ctx, &models.Task{ID: id, Title: "T"}); err != nil {
			t.Fatalf("CreateTask(%s): %v", id, err)
		}
	}

	if _, deferred, err := svc.admitOrDeferSeam1(ctx, "seam1-first", launchOriginAutomatic, map[string]interface{}{"prompt": "first"}); err != nil || deferred {
		t.Fatalf("first launch was not admitted: deferred=%v err=%v", deferred, err)
	}

	reservation, deferred, err := svc.admitOrDeferSeam1(ctx, "seam1-second", launchOriginAutomatic, map[string]interface{}{"prompt": "second"})
	if err != nil {
		t.Fatalf("admitOrDeferSeam1: %v", err)
	}
	if !deferred {
		t.Fatal("second automatic launch over the ceiling was admitted, want deferred")
	}
	if reservation != nil {
		t.Fatal("a deferred launch must not hold a reservation")
	}

	record := deferredLaunchOf(t, svc, "seam1-second")
	if record == nil || record[models.CeilingDeferredKey] != true {
		t.Fatalf("the refused launch's payload was not recorded: %+v", record)
	}
	nested, _ := record[models.CeilingLaunchPayloadKey].(map[string]interface{})
	if nested["prompt"] != "second" {
		t.Fatalf("the recorded payload does not match the refused launch: %+v", record)
	}
}

// TestAdmitOrDeferSeam1AdmitsManualOverCeiling covers AC-14: a manual launch is
// always admitted, never deferred, even at the ceiling.
func TestAdmitOrDeferSeam1AdmitsManualOverCeiling(t *testing.T) {
	svc, repo := newServiceWithCeiling(t, 1)
	ctx := context.Background()
	for _, id := range []string{"seam1-manual-first", "seam1-manual-second"} {
		if err := repo.CreateTask(ctx, &models.Task{ID: id, Title: "T"}); err != nil {
			t.Fatalf("CreateTask(%s): %v", id, err)
		}
	}
	if _, deferred, err := svc.admitOrDeferSeam1(ctx, "seam1-manual-first", launchOriginAutomatic, map[string]interface{}{"prompt": "first"}); err != nil || deferred {
		t.Fatalf("first launch was not admitted: deferred=%v err=%v", deferred, err)
	}

	reservation, deferred, err := svc.admitOrDeferSeam1(ctx, "seam1-manual-second", launchOriginManual, map[string]interface{}{"prompt": "second"})
	if err != nil {
		t.Fatalf("admitOrDeferSeam1: %v", err)
	}
	if deferred {
		t.Fatal("a manual launch was deferred; AC-14 requires it always be admitted")
	}
	if reservation == nil {
		t.Fatal("a manual override still consumes a reservation")
	}
	if record := deferredLaunchOf(t, svc, "seam1-manual-second"); record != nil {
		t.Fatalf("a manual override must never write a ceiling_deferred record: %+v", record)
	}
}

// TestAdmitOrDeferSeam1ReportsWriteFailure covers AC-45: a refusal that cannot
// be persisted must not be swallowed.
func TestAdmitOrDeferSeam1ReportsWriteFailure(t *testing.T) {
	svc, _ := newServiceWithCeiling(t, 1)
	ctx := context.Background()
	// Fill the one slot with a launch-scoped reservation so the next automatic
	// request is refused.
	svc.sessionCeiling.admit(ctx, admissionRequest{taskID: "filler", origin: launchOriginAutomatic, seam: "startTask"})

	// "missing-task" was never created, so the CAS read fails and the refusal
	// cannot be persisted.
	_, _, err := svc.admitOrDeferSeam1(ctx, "missing-task", launchOriginAutomatic, map[string]interface{}{"prompt": "p"})
	if err == nil {
		t.Fatal("expected an error when the refusal could not be persisted")
	}
}

// TestSeam1ReservationReleasesAfterRebindIfNeverConsumed pins the corrected
// rebind/release contract: rebinding moves the reservation onto the session's
// id, but does not by itself protect it from release. A launch that fails
// after the session was created (so rebindToSession already ran) but before
// it ever reached STARTING must still free the slot via the deferred
// releaseIfNotConsumed — otherwise the reservation would leak under the
// session id until the stale-reservation sweep eventually reclaims it.
func TestSeam1ReservationReleasesAfterRebindIfNeverConsumed(t *testing.T) {
	controller := newSessionCeilingController(5, nil, nil)
	decision := controller.admit(context.Background(), admissionRequest{taskID: "t", origin: launchOriginAutomatic, seam: "startTask"})
	if !decision.admitted {
		t.Fatal("expected admission")
	}
	reservation := &seam1Reservation{controller: controller, key: decision.reservationKey}
	reservation.rebindToSession("session-1")
	// Simulate a launch failure between rebind and STARTING: the reservation
	// was never consumed, so the deferred cleanup must still release it.
	reservation.releaseIfNotConsumed()

	population, err := controller.population(context.Background())
	if err != nil {
		t.Fatalf("population: %v", err)
	}
	if population != 0 {
		t.Fatalf("population = %d, want 0 (a rebound-but-unconsumed reservation must still release)", population)
	}
}

// TestSeam1ReservationConsumeSurvivesRebind covers the success path: once the
// launch actually succeeds and the reservation is consumed, the deferred
// releaseIfNotConsumed becomes a no-op and the population keeps counting the
// session under its rebound key.
func TestSeam1ReservationConsumeSurvivesRebind(t *testing.T) {
	controller := newSessionCeilingController(5, nil, nil)
	decision := controller.admit(context.Background(), admissionRequest{taskID: "t", origin: launchOriginAutomatic, seam: "startTask"})
	if !decision.admitted {
		t.Fatal("expected admission")
	}
	reservation := &seam1Reservation{controller: controller, key: decision.reservationKey}
	reservation.rebindToSession("session-1")
	reservation.consume()
	reservation.releaseIfNotConsumed()

	population, err := controller.population(context.Background())
	if err != nil {
		t.Fatalf("population: %v", err)
	}
	if population != 1 {
		t.Fatalf("population = %d, want 1 (a consumed reservation must survive the deferred release)", population)
	}
}

// TestSeam1ReservationRebindCollisionReleasesTheLoserNotTheWinner pins RV3-A:
// two independently-admitted seam 1 reservations can converge on the same
// session id (Office identity-owned sessions: EnsureSessionForAgentWithCreation
// converges concurrent callers for the same task/agent onto one session row).
// The second rebind to arrive must not silently overwrite the first
// reservation's map slot — its own later cleanup must be a no-op, not a
// deletion of the still-in-flight winner's reservation.
func TestSeam1ReservationRebindCollisionReleasesTheLoserNotTheWinner(t *testing.T) {
	controller := newSessionCeilingController(2, nil, nil)
	decisionA := controller.admit(context.Background(), admissionRequest{taskID: "task-a", origin: launchOriginAutomatic, seam: "startTask"})
	decisionB := controller.admit(context.Background(), admissionRequest{taskID: "task-b", origin: launchOriginAutomatic, seam: "startTask"})
	if !decisionA.admitted || !decisionB.admitted {
		t.Fatal("expected both launches to be admitted under a ceiling of 2")
	}

	winner := &seam1Reservation{controller: controller, key: decisionA.reservationKey}
	loser := &seam1Reservation{controller: controller, key: decisionB.reservationKey}

	// Both launches converge on the same session, exactly as two concurrent
	// startTask calls do via EnsureSessionForAgentWithCreation.
	winner.rebindToSession("session-shared")
	loser.rebindToSession("session-shared")

	// The loser's launch fails before the winner's ever resolves.
	loser.releaseIfNotConsumed()

	population, err := controller.population(context.Background())
	if err != nil {
		t.Fatalf("population: %v", err)
	}
	if population != 1 {
		t.Fatalf("population = %d, want 1 (the winner's still in-flight reservation must survive "+
			"the loser's collision cleanup)", population)
	}

	// The winner's own eventual failure must still free its slot: the fix must
	// not leave it stranded forever.
	winner.releaseIfNotConsumed()
	population, err = controller.population(context.Background())
	if err != nil {
		t.Fatalf("population: %v", err)
	}
	if population != 0 {
		t.Fatalf("population = %d, want 0 (the winner's own release must still free its slot)", population)
	}
}
