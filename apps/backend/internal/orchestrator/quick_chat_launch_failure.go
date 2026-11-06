package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/common/constants"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	"go.uber.org/zap"

	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func (s *Service) beginQuickChatStartFailureOwnership(
	ctx context.Context,
	req *LaunchSessionRequest,
) (func(), func(error) error, error) {
	release := s.acquireSessionLifecycleLock(req.SessionID)
	session, err := s.repo.GetTaskSession(ctx, req.SessionID)
	if err != nil {
		release()
		return nil, nil, fmt.Errorf("load Quick Chat session before deferred startup: %w", err)
	}
	if session == nil || session.TaskID != req.TaskID {
		release()
		return nil, nil, errors.New("quick chat deferred start session does not belong to task")
	}
	if session.State == models.TaskSessionStateStarting || session.State == models.TaskSessionStateRunning {
		release()
		return nil, nil, executor.ErrExecutionAlreadyRunning
	}
	req.QuickChatStartAttemptID = uuid.NewString()
	req.QuickChatStartAgentAttemptID = models.StringFromAny(
		session.Metadata[models.SessionMetaKeyAgentStartAttemptID],
	)
	if err := s.claimQuickChatStartAttempt(ctx, session, req.QuickChatStartAttemptID); err != nil {
		release()
		return nil, nil, err
	}
	return release, func(launchErr error) error {
		return s.handleQuickChatStartCreatedFailure(
			ctx,
			req.TaskID,
			req.SessionID,
			req.QuickChatStartAttemptID,
			req.QuickChatStartAgentAttemptID,
			launchErr,
		)
	}, nil
}

// Newly allocated Quick Chats retain admission failures through the same typed
// failure transition as executor startup. Existing sessions and capacity
// deferrals keep their current owner and lifecycle.
func (s *Service) handleQuickChatStartFailure(ctx context.Context, task *v1.Task, sessionID string, created bool, launchErr error) error {
	if !task.IsEphemeral || !created || sessionID == "" || errors.Is(launchErr, ErrCeilingLaunchDeferred) {
		return launchErr
	}
	s.clearTransientRetryState(sessionID)
	return s.executor.RecordEarlyLaunchFailure(ctx, task.ID, sessionID, launchErr)
}

func (s *Service) handleQuickChatStartCreatedFailure(
	ctx context.Context,
	taskID, sessionID, attemptID, agentAttemptID string,
	launchErr error,
) error {
	if !shouldPersistQuickChatStartFailure(launchErr, taskID, sessionID, attemptID) ||
		s.repo == nil || s.taskRepo == nil {
		return launchErr
	}
	failureCtx, cancel := context.WithTimeout(
		context.WithoutCancel(ctx), constants.TaskDeleteTimeout,
	)
	defer cancel()
	return s.persistQuickChatStartFailure(
		failureCtx, taskID, sessionID, attemptID, agentAttemptID, launchErr,
	)
}
func shouldPersistQuickChatStartFailure(
	launchErr error,
	taskID, sessionID, attemptID string,
) bool {
	return launchErr != nil && taskID != "" && sessionID != "" && attemptID != "" &&
		!errors.Is(launchErr, executor.ErrExecutionAlreadyRunning) &&
		!errors.Is(launchErr, ErrCeilingLaunchDeferred)
}

func (s *Service) persistQuickChatStartFailure(
	ctx context.Context,
	taskID, sessionID, attemptID, agentAttemptID string,
	launchErr error,
) error {
	task, err := s.taskRepo.GetTask(ctx, taskID)
	if err != nil || task == nil {
		if err != nil {
			s.logger.Warn("failed to load Quick Chat after deferred startup failure",
				zap.String("task_id", taskID), zap.Error(err))
		}
		return launchErr
	}
	if !task.IsEphemeral {
		return launchErr
	}
	_, ok := s.quickChatStartFailureSession(ctx, taskID, sessionID, attemptID, agentAttemptID)
	if !ok {
		return launchErr
	}
	changed, oldState, updatedAt, safeErr := s.executor.RecordEarlyQuickChatStartFailure(
		ctx, taskID, sessionID, attemptID, agentAttemptID, launchErr,
	)
	if changed {
		s.publishQuickChatStartFailure(ctx, taskID, sessionID, oldState, updatedAt, safeErr)
	}
	return safeErr
}

func (s *Service) quickChatStartFailureSession(
	ctx context.Context,
	taskID, sessionID, attemptID, agentAttemptID string,
) (*models.TaskSession, bool) {
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil || session == nil || session.TaskID != taskID {
		if err != nil {
			s.logger.Warn("failed to load Quick Chat session after deferred startup failure",
				zap.String("task_id", taskID), zap.String("session_id", sessionID), zap.Error(err))
		}
		return nil, false
	}
	if models.StringFromAny(session.Metadata[models.SessionMetaKeyQuickChatStartAttemptID]) != attemptID ||
		models.StringFromAny(session.Metadata[models.SessionMetaKeyAgentStartAttemptID]) != agentAttemptID {
		return nil, false
	}
	switch session.State {
	case models.TaskSessionStateCreated, models.TaskSessionStateStarting, models.TaskSessionStateWaitingForInput:
		return session, true
	default:
		return nil, false
	}
}

func (s *Service) publishQuickChatStartFailure(
	ctx context.Context,
	taskID, sessionID string,
	oldState models.TaskSessionState,
	updatedAt time.Time,
	safeErr error,
) {
	s.clearTransientRetryState(sessionID)
	refreshed, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		s.logger.Warn("failed to reload Quick Chat session after deferred startup failure",
			zap.String("task_id", taskID), zap.String("session_id", sessionID), zap.Error(err))
		return
	}
	if refreshed == nil {
		return
	}
	s.publishAcceptedTaskSessionState(
		ctx, taskID, sessionID, oldState, models.TaskSessionStateFailed,
		safeErr.Error(), &updatedAt, refreshed,
	)
}

type quickChatStartAttemptMetadataCAS interface {
	SetSessionMetadataKeyIfJSONValue(context.Context, string, string, interface{}, interface{}) (bool, error)
}

type quickChatStartAttemptMetadataIfAbsent interface {
	SetSessionMetadataKeyIfAbsent(context.Context, string, string, interface{}) (bool, error)
}

func (s *Service) claimQuickChatStartAttempt(
	ctx context.Context,
	session *models.TaskSession,
	attemptID string,
) error {
	key := models.SessionMetaKeyQuickChatStartAttemptID
	previous, exists := session.Metadata[key]
	if !exists {
		supported, err := s.claimQuickChatStartAttemptIfAbsent(ctx, session.ID, key, attemptID)
		if err != nil {
			return err
		}
		if supported {
			return nil
		}
		return s.repo.SetSessionMetadataKey(ctx, session.ID, key, attemptID)
	}
	if setter, ok := s.repo.(quickChatStartAttemptMetadataCAS); ok {
		claimed, err := setter.SetSessionMetadataKeyIfJSONValue(
			ctx, session.ID, key, previous, attemptID,
		)
		if err != nil {
			return fmt.Errorf("claim Quick Chat deferred start attempt: %w", err)
		}
		if !claimed {
			return executor.ErrExecutionAlreadyRunning
		}
		return nil
	}
	return s.repo.SetSessionMetadataKey(ctx, session.ID, key, attemptID)
}

func (s *Service) claimQuickChatStartAttemptIfAbsent(
	ctx context.Context,
	sessionID, key, attemptID string,
) (bool, error) {
	setter, ok := s.repo.(quickChatStartAttemptMetadataIfAbsent)
	if !ok {
		return false, nil
	}
	claimed, err := setter.SetSessionMetadataKeyIfAbsent(ctx, sessionID, key, attemptID)
	if err != nil {
		return true, fmt.Errorf("claim Quick Chat deferred start attempt: %w", err)
	}
	if !claimed {
		return true, executor.ErrExecutionAlreadyRunning
	}
	return true, nil
}
