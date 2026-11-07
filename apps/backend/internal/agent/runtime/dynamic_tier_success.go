package runtime

import "context"

// OrderedTiersEnabled keeps disabled completion paths free of tier reads.
func (r *ProfileExecutionResolver) OrderedTiersEnabled() bool {
	return r != nil && r.Enabled() && r.engine.OrderedTiersEnabled()
}

// ClearSelectionChain accepts only a caller-verified current success boundary.
// It does not participate in failure-policy counter or startup resets.
func (r *ProfileExecutionResolver) ClearSelectionChain(ctx context.Context, sessionID string, generation int64, candidateID string) error {
	if r == nil || r.engine == nil {
		return nil
	}
	return r.engine.ClearSelectionChain(ctx, sessionID, generation, candidateID)
}
