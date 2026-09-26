package orchestrator

import (
	"context"
	"errors"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/task/models"
)

// sessionKeyedCeilingReservation tracks an admission reservation taken for a
// session that already exists, shared by seam 2 (AC-4d/AC-4e) and seam 4
// (AC-4 family) — unlike seam 1's launch-scoped reservation, this one is keyed
// by the session id from the moment it is taken, because both seams are
// called with a session id already in hand.
type sessionKeyedCeilingReservation struct {
	controller *sessionCeilingController
	key        string
	consumed   bool

	// manualOverride, population, populationKnown and ceiling are the
	// admission decision's own reading, carried forward so AC-14/AC-53's
	// audit write and card warning can be performed once this reservation is
	// consumed by a session that already exists.
	manualOverride  bool
	population      int
	populationKnown bool
	ceiling         int
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

// admitOrDeferSessionKeyedLaunch is the shared admission/defer sequence for
// every seam whose session already exists at admission time (seams 2 and 4):
// consult the ceiling for the caller's session id, and on refusal persist a
// ceiling_deferred record of the given kind from the caller's own payload.
// failureContext names the launch kind in the error log ("the resume could
// not be admitted or recorded" vs. "the launch could not be admitted or
// recorded") so the two seams keep their own wording.
func (s *Service) admitOrDeferSessionKeyedLaunch(
	ctx context.Context, taskID, sessionID string, origin launchOrigin, seam string,
	kind models.CeilingLaunchKind, payload map[string]interface{}, failureContext string,
) (*sessionKeyedCeilingReservation, bool, error) {
	decision := s.sessionCeiling.admit(ctx, admissionRequest{
		taskID: taskID, sessionID: sessionID, origin: origin, seam: seam,
	})
	if decision.admitted {
		return &sessionKeyedCeilingReservation{
			controller: s.sessionCeiling, key: decision.reservationKey,
			manualOverride: decision.manualOverride, population: decision.population,
			populationKnown: decision.populationKnown, ceiling: decision.ceiling,
		}, false, nil
	}
	payload = s.enrichCeilingLaunchPayload(ctx, taskID, sessionID, payload)

	if err := s.deferCeilingRefusal(ctx, taskID, sessionID, kind, payload, decision.reasonCode,
		decision.population, decision.populationKnown, decision.ceiling); err != nil {
		if errors.Is(err, ErrCeilingLaunchConflict) && isSessionOpenRecoveryContext(ctx) {
			s.logger.Zap().Debug("session-open recovery left the existing ceiling queue unchanged after capacity refusal",
				zap.String("task_id", taskID), zap.String("session_id", sessionID), zap.Error(err))
		} else {
			s.logger.Zap().Error("could not persist a ceiling deferral; "+failureContext,
				zap.String("task_id", taskID), zap.String("session_id", sessionID),
				zap.String(ceilingFieldReasonCode, ceilingReasonDeferWriteFailed), zap.Error(err))
		}
		return nil, false, err
	}
	return nil, true, nil
}
