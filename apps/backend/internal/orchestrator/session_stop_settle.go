package orchestrator

import (
	"context"
	"errors"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
)

// settleExecutionLessSessionStop handles a stop for a session with no live
// execution. A settled or terminal session (or a missing row) is settled by
// closing its open turn, keeping the session row and state. Any other state may
// still be registering its execution, so it is not settled and the caller keeps
// the original not-found error. A turn lookup or settlement failure is returned.
//
// Lifecycle and prompt admission remain excluded from the runtime recheck
// through turn closure, including admission that would reuse the same turn.
func (s *Service) settleExecutionLessSessionStop(ctx context.Context, sessionID string) (bool, error) {
	releaseLifecycle, acquired := s.tryAcquireSessionLifecycleLock(sessionID)
	if !acquired {
		return false, nil
	}
	defer releaseLifecycle()
	if err := ctx.Err(); err != nil {
		return false, err
	}
	guard, releaseGuard := s.acquireCancelInFlightGuard(sessionID)
	defer releaseGuard()
	// Existing launch callers can own this guard before acquiring lifecycle.
	// Settlement must yield rather than wait while holding lifecycle itself.
	if !guard.TryLock() {
		return false, ctx.Err()
	}
	defer guard.Unlock()
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if s.isCancelInFlight(sessionID) {
		return false, nil
	}
	if _, active := s.resumeAttemptStore().current(sessionID); active {
		return false, nil
	}
	// The initial failed stop released the executor's lock. A launch may have
	// registered a runtime before settlement acquired admission ownership.
	executionID, err := s.agentManager.GetExecutionIDForSession(ctx, sessionID)
	if executionID != "" || err == nil {
		return false, nil
	}
	if !executor.IsNoExecutionForSessionError(err) {
		return false, err
	}
	return s.settleExecutionLessSessionTurn(ctx, sessionID)
}

// settleExecutionLessSessionTurn requires lifecycle and prompt admission ownership.
func (s *Service) settleExecutionLessSessionTurn(ctx context.Context, sessionID string) (bool, error) {
	var turn *models.Turn
	if s.turnService != nil {
		active, err := s.turnService.GetActiveTurn(ctx, sessionID)
		switch {
		case err == nil:
			turn = active
		case !isNoActiveTurnError(err):
			return false, err
		}
	}
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if errors.Is(err, models.ErrTaskSessionNotFound) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	if session == nil {
		return true, nil
	}
	switch session.State {
	case models.TaskSessionStateWaitingForInput,
		models.TaskSessionStateCompleted,
		models.TaskSessionStateFailed,
		models.TaskSessionStateCancelled:
	default:
		return false, nil
	}
	if turn == nil {
		return true, nil
	}
	if err := s.completeTurnForTaskSessionCheckedOwned(ctx, session.TaskID, sessionID, turn.ID); err != nil {
		return false, err
	}
	return true, nil
}
