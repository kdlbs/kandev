package dynamic

import "context"

// PreviewReviewerCandidate reads candidate availability without claiming a
// route or consuming a circuit probe. The final run still owns selection.
func (e *Engine) PreviewReviewerCandidate(ctx context.Context, profile Profile) (string, error) {
	if e.OrderedTiersEnabled() {
		preview, err := e.PreviewTiersContext(ctx, profile)
		return preview.CandidateID, err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	now := e.now()
	for _, candidate := range profile.Candidates {
		if candidate.Enabled && e.circuits.selectionAvailability(candidate, now) == "" {
			return candidate.ID, nil
		}
	}
	return "", nil
}
