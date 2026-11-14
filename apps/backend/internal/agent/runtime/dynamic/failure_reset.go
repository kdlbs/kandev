package dynamic

import "context"

// ResetCircuitFailures clears the consecutive failure count of the shared
// circuit of the candidate whose turn completed without a provider error. The
// completion is fenced like a failure: one that names a superseded generation
// or candidate returns ErrStaleGeneration and clears nothing. The boolean
// reports whether a count was cleared.
func (e *Engine) ResetCircuitFailures(
	ctx context.Context,
	sessionID string,
	profile Profile,
	expectedGeneration int64,
	candidateID string,
) (bool, error) {
	if e == nil || e.circuits == nil || candidateID == "" {
		return false, nil
	}
	candidate, ok := candidateByID(profile, candidateID)
	if !ok || candidate.BindingKey == "" {
		return false, nil
	}
	if err := e.requireCurrentFailureRoute(ctx, sessionID, expectedGeneration, candidateID); err != nil {
		return false, err
	}
	return e.circuits.ResetFailures(candidate.BindingKey), nil
}
