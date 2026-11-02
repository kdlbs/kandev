package dynamic

import (
	"context"

	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
)

// RecordFailureCircuit charges a qualifying classified failure to the shared
// resource circuit of the candidate that produced it, without routing.
//
// Callers use it for a failure that may not advance the route, such as one
// reported after the attempt already produced output or effects. The route
// generation, the selected candidate, and the recovery surface stay exactly as
// they are; only later selections skip the open circuit.
//
// The failure is fenced like ApplyFailureContext: a failure that names a
// superseded generation or candidate returns ErrStaleGeneration and opens
// nothing. An owner-release code is not recorded here; the caller holds that
// profile through HoldAccountBlock. The boolean reports whether a circuit was
// opened.
func (e *Engine) RecordFailureCircuit(
	ctx context.Context,
	sessionID string,
	profile Profile,
	expectedGeneration int64,
	candidateID string,
	failure *routingerr.Error,
) (bool, error) {
	if e == nil || e.circuits == nil || failure == nil || candidateID == "" {
		return false, nil
	}
	if routingerr.HoldsUntilRelease(failure.Code) || !qualifiesForCircuit(failure.Code) {
		return false, nil
	}
	candidate, ok := candidateByID(profile, candidateID)
	if !ok || candidate.BindingKey == "" {
		return false, nil
	}
	if err := e.requireCurrentFailureRoute(ctx, sessionID, expectedGeneration, candidateID); err != nil {
		return false, err
	}
	e.openCircuitForFailure(profile, candidateID, failure)
	e.releaseProbeForFailure(sessionID, expectedGeneration, candidateID)
	return true, nil
}
