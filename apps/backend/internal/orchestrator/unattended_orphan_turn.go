package orchestrator

import (
	"context"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/task/models"
)

// ErrOrphanTurnSessionBusy means the session is RUNNING or STARTING, so an
// unattended orphan turn is left as it is.
var ErrOrphanTurnSessionBusy = errors.New("session is running; orphan turn left open")

// CompleteUnattendedOrphanTurn completes turnID, the turn a rolled-back
// unattended wake send left active, and drops its in-memory active-turn entry.
// It changes nothing while the session is RUNNING or STARTING. A missing
// session counts as not busy. Every non-nil error leaves the turn as it was.
func (s *Service) CompleteUnattendedOrphanTurn(ctx context.Context, sessionID, turnID string) error {
	if sessionID == "" || turnID == "" {
		return errors.New("complete orphan turn: session id and turn id are required")
	}
	if s.repo == nil || s.turnService == nil {
		return errors.New("complete orphan turn: turn authority is not configured")
	}
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	switch {
	case errors.Is(err, models.ErrTaskSessionNotFound):
	case err != nil:
		return fmt.Errorf("complete orphan turn: load session: %w", err)
	case session.State == models.TaskSessionStateRunning || session.State == models.TaskSessionStateStarting:
		return ErrOrphanTurnSessionBusy
	}
	if err := s.completeExpectedTurn(ctx, sessionID, turnID); err != nil {
		return err
	}
	s.activeTurns.CompareAndDelete(sessionID, turnID)
	return nil
}
