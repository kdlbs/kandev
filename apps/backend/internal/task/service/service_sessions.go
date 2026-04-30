package service

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/task/models"
)

// SessionReadyPollInterval is how often WaitForSessionReady polls session state.
const SessionReadyPollInterval = 1 * time.Second

// SessionReadyMaxWait is the maximum time WaitForSessionReady waits before timing out.
const SessionReadyMaxWait = 90 * time.Second

// WaitForSessionReady polls the session state until the agent is ready to accept
// prompts. Returns nil when the session reaches WAITING_FOR_INPUT, or an error if
// it transitions to FAILED/CANCELLED/COMPLETED, the context is cancelled, or the
// timeout is exceeded. Used after ResumeTaskSession to gate the prompt retry until
// the agent has actually finished booting.
func (s *Service) WaitForSessionReady(ctx context.Context, sessionID string) error {
	deadline := time.Now().Add(SessionReadyMaxWait)
	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("timeout waiting for session to become ready after resume")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(SessionReadyPollInterval):
		}
		session, err := s.sessions.GetTaskSession(ctx, sessionID)
		if err != nil {
			return fmt.Errorf("failed to check session state: %w", err)
		}
		switch session.State {
		case models.TaskSessionStateWaitingForInput:
			return nil
		case models.TaskSessionStateFailed:
			errMsg := session.ErrorMessage
			if errMsg == "" {
				errMsg = "session failed during resume"
			}
			return fmt.Errorf("session failed after resume: %s", errMsg)
		case models.TaskSessionStateCancelled, models.TaskSessionStateCompleted:
			return fmt.Errorf("session in unexpected state after resume: %s", session.State)
		default:
			// STARTING or RUNNING — keep polling
		}
	}
}

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
