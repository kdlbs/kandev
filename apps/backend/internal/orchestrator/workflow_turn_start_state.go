package orchestrator

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/task/models"
	"go.uber.org/zap"
)

func (s *Service) prepareWorkflowTurnStartSessionState(
	ctx context.Context,
	taskID, sessionID string,
) error {
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("load turn-start recipient session: %w", err)
	}
	if session == nil {
		return fmt.Errorf("load turn-start recipient session: session %q is nil", sessionID)
	}
	if session.TaskID != taskID {
		return fmt.Errorf("turn-start recipient session %q does not belong to task %q", sessionID, taskID)
	}

	switch session.State {
	case models.TaskSessionStateStarting,
		models.TaskSessionStateRunning,
		models.TaskSessionStateWaitingForInput,
		models.TaskSessionStateFailed,
		models.TaskSessionStateCancelled,
		models.TaskSessionStateCompleted:
		return nil
	case models.TaskSessionStateCreated, models.TaskSessionStateIdle:
		expectedState := session.State
		_, _, err := s.transitionTaskSessionState(
			ctx,
			taskID,
			sessionID,
			&expectedState,
			models.TaskSessionStateWaitingForInput,
			"",
			func() { s.writeTaskReviewState(ctx, taskID, sessionID) },
		)
		if err != nil {
			return fmt.Errorf("prepare turn-start recipient session: %w", err)
		}
		return nil
	default:
		return fmt.Errorf("turn-start recipient session %q has unsupported state %q", sessionID, session.State)
	}
}

func (s *Service) reportWorkflowTurnStartPreparationError(
	ctx context.Context,
	taskID, sessionID string,
	err error,
) {
	if err == nil {
		return
	}
	recordWorkflowTransitionError(ctx, err)
	s.logger.Warn("failed to prepare session after workflow turn-start",
		zap.String("task_id", taskID),
		zap.String("session_id", sessionID),
		zap.Error(err))
}
