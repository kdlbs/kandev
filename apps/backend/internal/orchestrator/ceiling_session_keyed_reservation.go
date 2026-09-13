package orchestrator

import "context"

// sessionKeyedCeilingReservation tracks an admission reservation taken for a
// session that already exists, shared by seam 2 (AC-4d/AC-4e) and seam 4
// (AC-4 family) — unlike seam 1's launch-scoped reservation, this one is keyed
// by the session id from the moment it is taken, because both seams are
// called with a session id already in hand.
type sessionKeyedCeilingReservation struct {
	controller *sessionCeilingController
	key        string
	consumed   bool
}

// rekeyToSession moves the reservation from the session it was gated under
// onto the replacement session that actually launches, as one operation so the
// population never momentarily drops. Only seam 2's on_turn_start redirect
// needs this; seam 4 never changes session id mid-resume and simply never
// calls it.
func (r *sessionKeyedCeilingReservation) rekeyToSession(ctx context.Context, sessionID string) {
	if r == nil || r.controller == nil || r.consumed || sessionID == "" || sessionID == r.key {
		return
	}
	if r.controller.rekey(ctx, r.key, sessionID) {
		r.key = sessionID
	}
}

// consume marks the reservation as belonging to a session that is now actually
// running, so releaseIfNotConsumed becomes a no-op.
func (r *sessionKeyedCeilingReservation) consume() {
	if r == nil {
		return
	}
	r.consumed = true
}

// releaseIfNotConsumed is the deferred cleanup for every return path between
// admission and the launch actually succeeding.
func (r *sessionKeyedCeilingReservation) releaseIfNotConsumed() {
	if r == nil || r.controller == nil || r.consumed {
		return
	}
	r.controller.release(r.key)
}
