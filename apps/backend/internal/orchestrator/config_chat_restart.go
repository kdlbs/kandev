package orchestrator

import (
	"context"
	"errors"
	"fmt"

	runtimeapi "github.com/kandev/kandev/internal/agent/runtime"
)

// RetireConfigChatSession holds conversation lifecycle exclusion until the
// authorized deletion callback commits. Runtime teardown and deletion may
// acquire the cancellation guard, so that guard is released during their I/O.
func (s *Service) RetireConfigChatSession(ctx context.Context, taskID, sessionID string, deleteFn func(context.Context) error) error {
	if taskID == "" || sessionID == "" || deleteFn == nil {
		return errors.New("configuration chat retirement requires a task, session and deletion callback")
	}
	if err := s.authorizeTaskSessionPair(ctx, taskID, sessionID); err != nil {
		return err
	}
	releaseLifecycle, acquired := s.tryAcquireSessionLifecycleLock(sessionID)
	if !acquired {
		return ErrSessionResetInProgress
	}
	defer releaseLifecycle()
	guard, err := s.lockCancelInFlightGuardWithContext(ctx, sessionID)
	if err != nil {
		return err
	}
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil || session == nil {
		guard.release()
		return fmt.Errorf("configuration chat session unavailable: %w", err)
	}
	if session.TaskID != taskID {
		guard.release()
		return ErrTaskSessionPairMismatch
	}
	if s.isSessionResetInProgress(sessionID) {
		guard.release()
		return ErrSessionResetInProgress
	}
	s.setSessionResetInProgress(sessionID, true)
	s.invalidateResumeAttempt(sessionID)
	guard.unlock()
	defer func() {
		guard.relock()
		s.setSessionResetInProgress(sessionID, false)
		guard.release()
	}()
	s.resetTransientRetryWithContext(ctx, sessionID, true)
	if err := s.StopSessionSynchronously(ctx, sessionID, "configuration chat restarted", true); err != nil && !errors.Is(err, runtimeapi.ErrNotFound) {
		return err
	}
	return deleteFn(ctx)
}
