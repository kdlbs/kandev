package lifecycle

import (
	"context"
	"errors"

	"github.com/kandev/kandev/internal/agent/agents"
)

// Callers check their context before fallback. A probe's own deadline may
// expire while launch admission remains live; cancellation never permits it.
func validateNativeOpenCodeProbeFallback(probeErr error, selectedVersion string) error {
	if agents.IsUnsupportedOpenCodeMajorError(probeErr) || errors.Is(probeErr, context.Canceled) {
		return probeErr
	}
	if _, err := agents.OpenCodeACPArgsForVersion(selectedVersion); err != nil {
		return errors.Join(probeErr, err)
	}
	return nil
}
