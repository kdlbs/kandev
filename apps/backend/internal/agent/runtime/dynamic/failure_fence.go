package dynamic

import "context"

// requireCurrentFailureRoute fences a failure to the generation and candidate
// the route holds now. A failure reported for a superseded generation or
// candidate belongs to an attempt the route already left: it must neither
// open the circuit of the candidate named by the caller nor advance the route.
// A session without a known route state keeps the caller's claim.
func (e *Engine) requireCurrentFailureRoute(
	ctx context.Context,
	sessionID string,
	generation int64,
	candidateID string,
) error {
	state, exists, err := e.stateForFailure(ctx, sessionID)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}
	// A waiting route has no selected candidate to compare.
	if state.Generation != generation ||
		(state.ExecutionProfileID != "" && state.ExecutionProfileID != candidateID) {
		return ErrStaleGeneration
	}
	return nil
}
