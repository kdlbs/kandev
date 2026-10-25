package orchestrator

import (
	"context"
	"errors"
)

// releaseStartEnvironmentClaim releases the creating task environment that a
// session created by this start still owns when the start returns early. A
// ceiling deferral keeps the claim for the deferred launch, and a session the
// start reused keeps whatever it owned before.
func (s *Service) releaseStartEnvironmentClaim(ctx context.Context, taskID, sessionID string, created bool, startErr error) {
	if !created || sessionID == "" || s.executor == nil || errors.Is(startErr, ErrCeilingLaunchDeferred) {
		return
	}
	s.executor.ReleaseTaskEnvironmentMaterialization(context.WithoutCancel(ctx), taskID, sessionID)
}
