package orchestrator

import (
	"context"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
)

type localPassthroughResumeReadiness interface {
	IsLocalPassthroughReadyForResume(sessionID, executionID, attemptID string) bool
}

func (s *Service) reconcilePassthroughResumeReady(ctx context.Context, sessionID string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	attemptID := executor.ResumeAttemptIDFromContext(ctx)
	readiness, ok := s.agentManager.(localPassthroughResumeReadiness)
	if !ok || attemptID == "" {
		return false, nil
	}
	guard, err := s.lockCancelInFlightGuardAfterCancellation(ctx, sessionID)
	if err != nil {
		return false, err
	}
	defer guard.release()
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil || session == nil {
		return false, err
	}
	if session.State != models.TaskSessionStateStarting {
		return false, nil
	}
	executionID, err := s.agentManager.GetExecutionIDForSession(ctx, sessionID)
	if err != nil {
		return false, err
	}
	idle, err := s.passthroughResumeOwnsIdleStartup(ctx, sessionID, executionID, attemptID)
	if err != nil || !idle {
		return false, err
	}
	// A terminal reconnect can finish startup before the resume owner binds its
	// identity. Reconcile that idle startup without replaying a completion event.
	if !readiness.IsLocalPassthroughReadyForResume(sessionID, executionID, attemptID) {
		return false, nil
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	s.setSessionWaitingForInput(ctx, session.TaskID, sessionID, session)
	latest, err := s.repo.GetTaskSession(ctx, sessionID)
	return err == nil && latest != nil && latest.State == models.TaskSessionStateWaitingForInput, err
}

func (s *Service) passthroughResumeOwnsIdleStartup(ctx context.Context, sessionID, executionID, attemptID string) (bool, error) {
	if s.turnService == nil || !s.activeResumeAttemptOwnsStartup(sessionID, executionID, attemptID) {
		return false, nil
	}
	if _, active := s.activeTurns.Load(sessionID); active || s.reservedPromptTurnID(sessionID) != "" {
		return false, nil
	}
	turn, err := s.turnService.GetActiveTurn(ctx, sessionID)
	return err == nil && turn == nil, err
}
