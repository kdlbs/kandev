package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/constants"
)

// A seam-1 launch is admitted before its session exists, so its reservation starts
// launch-scoped and is moved onto the session id once the session is created. The
// population must not dip during the move.
func TestRebindMovesALaunchScopedReservationOntoItsSession(t *testing.T) {
	lister := &fakeAdmittedLister{}
	c := newTestController(t, 4, lister)

	d := c.admit(context.Background(), admissionRequest{taskID: "t1", origin: launchOriginAutomatic, seam: "seam1"})
	if !d.admitted || d.reservationKey == "" {
		t.Fatalf("admit = %+v, want admitted with a launch-scoped key", d)
	}
	before := mustPopulation(t, c)

	if !c.rebind(d.reservationKey, "s1") {
		t.Fatal("rebind reported no reservation to move")
	}
	if got := mustPopulation(t, c); got != before {
		t.Fatalf("population after rebind = %d, want %d unchanged", got, before)
	}

	// The reservation now answers to the session id, so the session's own row
	// landing does not double-count it.
	lister.set("s1")
	if got := mustPopulation(t, c); got != 1 {
		t.Fatalf("population once the row lands = %d, want 1", got)
	}
	c.release("s1")
	if got := mustPopulation(t, c); got != 1 {
		t.Fatalf("population after releasing the rebound key = %d, want 1 (the row remains)", got)
	}
}

func TestRebindReportsAnUnknownLaunchKey(t *testing.T) {
	c := newTestController(t, 4, &fakeAdmittedLister{})
	if c.rebind("launch-does-not-exist", "s1") {
		t.Fatal("rebind reported success for a key it does not hold")
	}
}

// startCreatedSession can redirect to a different session after it has gated,
// locked and reserved the original. The reservation must follow, or the acceptance
// edge fires with an id that matches nothing and the original leaks until the
// backstop.
func TestRekeyMovesTheReservationToTheReplacementSession(t *testing.T) {
	lister := &fakeAdmittedLister{}
	c := newTestController(t, 4, lister)
	c.admit(context.Background(), admissionRequest{taskID: "t1", sessionID: "original", origin: launchOriginAutomatic, seam: "seam2"})

	if !c.rekey(context.Background(), "original", "replacement") {
		t.Fatal("rekey reported no reservation to move")
	}
	if got := mustPopulation(t, c); got != 1 {
		t.Fatalf("population after rekey = %d, want 1 (still one launch)", got)
	}

	c.release("original")
	if got := mustPopulation(t, c); got != 1 {
		t.Fatalf("releasing the original key changed the population to %d, want 1", got)
	}
	c.release("replacement")
	if got := mustPopulation(t, c); got != 0 {
		t.Fatalf("population after releasing the replacement = %d, want 0", got)
	}
}

// Where the replacement is already counted, the original is released and no second
// unit is consumed.
func TestRekeyConsumesNoSecondUnitWhenTheReplacementIsAlreadyCounted(t *testing.T) {
	lister := &fakeAdmittedLister{}
	lister.set("replacement")
	c := newTestController(t, 4, lister)
	c.admit(context.Background(), admissionRequest{taskID: "t1", sessionID: "original", origin: launchOriginAutomatic, seam: "seam2"})
	if got := mustPopulation(t, c); got != 2 {
		t.Fatalf("precondition: population = %d, want 2", got)
	}

	if !c.rekey(context.Background(), "original", "replacement") {
		t.Fatal("rekey reported no reservation to move")
	}
	if got := mustPopulation(t, c); got != 1 {
		t.Fatalf("population after rekey onto a counted session = %d, want 1", got)
	}
}

// Where the replacement session already holds its own unrelated reservation
// (not a counted row), rekey must not report ownership transferring onto it:
// the caller would then release someone else's reservation instead of its
// own already-deleted, already-no-op key.
func TestRekeyDoesNotTransferOwnershipOnAReservationCollision(t *testing.T) {
	lister := &fakeAdmittedLister{}
	c := newTestController(t, 4, lister)
	c.admit(context.Background(), admissionRequest{taskID: "t1", sessionID: "original", origin: launchOriginAutomatic, seam: "seam2"})
	c.admit(context.Background(), admissionRequest{taskID: "t2", sessionID: "replacement", origin: launchOriginAutomatic, seam: "seam2"})
	if got := mustPopulation(t, c); got != 2 {
		t.Fatalf("precondition: population = %d, want 2", got)
	}

	if c.rekey(context.Background(), "original", "replacement") {
		t.Fatal("rekey reported ownership transferring onto a reservation collision, want false")
	}
	if got := mustPopulation(t, c); got != 1 {
		t.Fatalf("population after the collision = %d, want 1 (original released, replacement untouched)", got)
	}

	// A caller told "false" leaves its key at "original" (already deleted by
	// rekey), so its own cleanup is a no-op rather than deleting the
	// replacement's still-live reservation.
	c.release("original")
	if got := mustPopulation(t, c); got != 1 {
		t.Fatalf("population after releasing the stale original key = %d, want 1 (no-op)", got)
	}
	c.release("replacement")
	if got := mustPopulation(t, c); got != 0 {
		t.Fatalf("population after releasing the replacement's own reservation = %d, want 0", got)
	}
}

// The automatic dynamic-route path enters while its session is still RUNNING. The
// hand-off converts that membership into a reservation without consulting the
// ceiling, so the population is flat across the CREATED write instead of dipping.
func TestHandOffConvertsCountedMembershipWithoutChangingThePopulation(t *testing.T) {
	lister := &fakeAdmittedLister{}
	lister.set("s1", "s2")
	c := newTestController(t, 2, lister)

	d := c.handOffOrAdmit(context.Background(), admissionRequest{taskID: "t1", sessionID: "s1", origin: launchOriginAutomatic, seam: "seam5"})
	if !d.admitted || !d.handedOff {
		t.Fatalf("handOffOrAdmit = %+v, want admitted as a hand-off", d)
	}
	if d.reservationKey != "s1" {
		t.Fatalf("reservationKey = %q, want %q", d.reservationKey, "s1")
	}
	if got := mustPopulation(t, c); got != 2 {
		t.Fatalf("population after the hand-off = %d, want 2 unchanged", got)
	}

	// The relaunch moves the session out of the counted rows; the reservation is
	// what keeps its slot held.
	lister.set("s2")
	if got := mustPopulation(t, c); got != 2 {
		t.Fatalf("population across the CREATED write = %d, want 2 (the reservation holds the slot)", got)
	}
}

// A hand-off is never refused, even at a full ceiling, because it adds nothing.
func TestHandOffIsNeverRefusedAtTheCeiling(t *testing.T) {
	lister := &fakeAdmittedLister{}
	lister.set("s1")
	c := newTestController(t, 1, lister)

	d := c.handOffOrAdmit(context.Background(), admissionRequest{taskID: "t1", sessionID: "s1", origin: launchOriginAutomatic, seam: "seam5"})
	if !d.admitted || !d.handedOff {
		t.Fatalf("handOffOrAdmit at a full ceiling = %+v, want admitted as a hand-off", d)
	}
}

// A relaunch that aborts before dispatching a replacement releases the handed-off
// slot: a hand-off transfers a slot, it does not pin one.
func TestHandOffIsReleasedLikeAnyOtherReservation(t *testing.T) {
	lister := &fakeAdmittedLister{}
	lister.set("s1")
	c := newTestController(t, 2, lister)
	c.handOffOrAdmit(context.Background(), admissionRequest{taskID: "t1", sessionID: "s1", origin: launchOriginAutomatic, seam: "seam5"})

	lister.set()
	c.release("s1")
	if got := mustPopulation(t, c); got != 0 {
		t.Fatalf("population after releasing an aborted hand-off = %d, want 0", got)
	}
}

// The manual and recovery callers of seam 5 arrive after the route has been parked,
// so their session is not counted and there is no slot to hand off. That is an
// ordinary admission request and it is refusable.
func TestHandOffFallsBackToOrdinaryAdmissionForAnUncountedSession(t *testing.T) {
	lister := &fakeAdmittedLister{}
	lister.set("other-1", "other-2")
	c := newTestController(t, 2, lister)

	d := c.handOffOrAdmit(context.Background(), admissionRequest{taskID: "t1", sessionID: "parked", origin: launchOriginAutomatic, seam: "seam5"})
	if d.handedOff {
		t.Fatalf("handOffOrAdmit = %+v, want an ordinary request for an uncounted session", d)
	}
	if d.admitted {
		t.Fatalf("handOffOrAdmit = %+v, want refused at the ceiling", d)
	}
	if d.reasonCode != ceilingReasonRefused {
		t.Fatalf("reasonCode = %q, want %q", d.reasonCode, ceilingReasonRefused)
	}
}

// A launch that neither reaches a counted state nor reports failure is released by
// the backstop, sized to the whole admission-to-STARTING window.
func TestExpireStaleReservationsReleasesOnlyReservationsPastTheLaunchBudget(t *testing.T) {
	lister := &fakeAdmittedLister{}
	c := newTestController(t, 10, lister)

	base := time.Now()
	c.now = func() time.Time { return base }
	c.admit(context.Background(), admissionRequest{taskID: "t1", sessionID: "stale", origin: launchOriginAutomatic, seam: "seam2"})

	budget := constants.AgentLaunchTimeout + 5*time.Minute
	c.now = func() time.Time { return base.Add(budget - time.Second) }
	c.admit(context.Background(), admissionRequest{taskID: "t2", sessionID: "fresh", origin: launchOriginAutomatic, seam: "seam2"})
	if released := c.expireStaleReservations(); released != 0 {
		t.Fatalf("expireStaleReservations released %d reservations one second inside the budget, want 0", released)
	}

	c.now = func() time.Time { return base.Add(budget + time.Second) }
	if released := c.expireStaleReservations(); released != 1 {
		t.Fatalf("expireStaleReservations released %d, want 1 (only the stale one)", released)
	}
	if got := mustPopulation(t, c); got != 1 {
		t.Fatalf("population after expiry = %d, want 1 (the fresh reservation survived)", got)
	}
}
