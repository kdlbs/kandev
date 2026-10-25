package orchestrator

import (
	"context"
	"fmt"
)

type contextualPassthroughRunningPublisher interface {
	MarkPassthroughRunningWithContext(context.Context, string) error
}

// guardedPassthroughRunning scopes ownership to the synchronous publication,
// after admission has reacquired the guard. Preparation keeps its original context.
func (s *Service) guardedPassthroughRunning(ctx context.Context, sessionID string, guard *lockedCancelInFlightGuard) func() error {
	if guard == nil {
		return nil
	}
	return func() error {
		if !guard.locked {
			return fmt.Errorf("passthrough running publication requires the session admission guard")
		}
		publisher, ok := s.agentManager.(contextualPassthroughRunningPublisher)
		if !ok {
			return fmt.Errorf("passthrough running publisher does not preserve admission ownership")
		}
		return publisher.MarkPassthroughRunningWithContext(
			withCancelInFlightGuardHeldForSession(ctx, sessionID), sessionID,
		)
	}
}
