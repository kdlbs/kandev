package service

import (
	"context"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/task/models"
)

// ListTaskSessions returns all sessions for a task.
func (s *Service) ListTaskSessions(ctx context.Context, taskID string) ([]*models.TaskSession, error) {
	return s.sessions.ListTaskSessions(ctx, taskID)
}

// GetTaskSession returns a single session by ID.
func (s *Service) GetTaskSession(ctx context.Context, sessionID string) (*models.TaskSession, error) {
	return s.sessions.GetTaskSession(ctx, sessionID)
}

// GetPrimarySession returns the primary session for a task.
func (s *Service) GetPrimarySession(ctx context.Context, taskID string) (*models.TaskSession, error) {
	return s.sessions.GetPrimarySessionByTaskID(ctx, taskID)
}

// GetPrimarySessionIDsForTasks returns a map of task ID to primary session ID for the given task IDs.
// Tasks without a primary session are not included in the result.
func (s *Service) GetPrimarySessionIDsForTasks(ctx context.Context, taskIDs []string) (map[string]string, error) {
	return s.sessions.GetPrimarySessionIDsByTaskIDs(ctx, taskIDs)
}

// GetSessionCountsForTasks returns a map of task ID to session count for the given task IDs.
func (s *Service) GetSessionCountsForTasks(ctx context.Context, taskIDs []string) (map[string]int, error) {
	return s.sessions.GetSessionCountsByTaskIDs(ctx, taskIDs)
}

// GetPrimarySessionInfoForTasks returns a map of task ID to primary session info for the given task IDs.
func (s *Service) GetPrimarySessionInfoForTasks(ctx context.Context, taskIDs []string) (map[string]*models.TaskSession, error) {
	return s.sessions.GetPrimarySessionInfoByTaskIDs(ctx, taskIDs)
}

// SetPrimarySession sets a session as the primary session for its task.
// This will unset any existing primary session for the same task.
func (s *Service) SetPrimarySession(ctx context.Context, sessionID string) error {
	if err := s.sessions.SetSessionPrimary(ctx, sessionID); err != nil {
		s.logger.Error("failed to set primary session",
			zap.String("session_id", sessionID),
			zap.Error(err))
		return err
	}
	return nil
}

// MoveSessionToStep moves a session to a different workflow step.
func (s *Service) MoveSessionToStep(ctx context.Context, sessionID string, stepID string) error {
	if err := s.sessions.UpdateSessionWorkflowStep(ctx, sessionID, stepID); err != nil {
		s.logger.Error("failed to move session to step",
			zap.String("session_id", sessionID),
			zap.String("step_id", stepID),
			zap.Error(err))
		return err
	}
	return nil
}

// UpdateSessionReviewStatus updates the review status of a session.
func (s *Service) UpdateSessionReviewStatus(ctx context.Context, sessionID string, status string) error {
	if err := s.sessions.UpdateSessionReviewStatus(ctx, sessionID, status); err != nil {
		s.logger.Error("failed to update session review status",
			zap.String("session_id", sessionID),
			zap.String("status", status),
			zap.Error(err))
		return err
	}
	return nil
}
