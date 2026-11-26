package orchestrator

import (
	"context"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/task/models"
)

type interruptedTaskWarningSnapshot struct {
	taskID        string
	sessionID     string
	incarnationID string
	token         string
	legacyValue   string
	executionID   string
	attemptID     string
	turnID        string
	captured      bool
}

func (s *Service) captureInterruptedTaskWarning(
	ctx context.Context,
	taskID string,
	session *models.TaskSession,
	executionID, attemptID, turnID string,
) interruptedTaskWarningSnapshot {
	snapshot := interruptedTaskWarningSnapshot{
		taskID: taskID, executionID: executionID, attemptID: attemptID, turnID: turnID,
	}
	if !validInterruptedTaskWarningTarget(s, taskID, session) {
		return snapshot
	}
	snapshot.sessionID = session.ID
	snapshot.incarnationID = session.QueueIncarnationID
	snapshot.token, snapshot.legacyValue = s.readInterruptedTaskWarningValues(ctx, taskID, session)
	snapshot.captured = snapshot.token != "" || snapshot.legacyValue != ""
	return snapshot
}

func validInterruptedTaskWarningTarget(s *Service, taskID string, session *models.TaskSession) bool {
	return s != nil && s.repo != nil && taskID != "" && session != nil && session.ID != "" && session.TaskID == taskID
}

func (s *Service) readInterruptedTaskWarningValues(
	ctx context.Context,
	taskID string,
	session *models.TaskSession,
) (token, legacy string) {
	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil {
		s.logger.Warn("failed to capture task interruption warning",
			zap.String("task_id", taskID), zap.Error(err))
		return "", ""
	}
	if task == nil || task.ArchivedAt != nil {
		return "", ""
	}
	entries, _, err := models.LoadInterruptedSessionMarkers(task.Metadata)
	if err != nil {
		s.logger.Warn("failed to decode task interruption warnings",
			zap.String("task_id", taskID), zap.Error(err))
		return "", ""
	}
	if marker, ok := entries[session.ID]; ok &&
		marker.IncarnationID == session.QueueIncarnationID && marker.Token != "" {
		token = marker.Token
	}
	legacy = models.StringFromAny(task.Metadata[models.MetaKeyLegacyInterruptedAt])
	if legacy == "" && len(entries) == 0 {
		legacy = models.StringFromAny(task.Metadata[models.MetaKeyInterruptedAt])
	}
	return token, legacy
}

func (s *Service) resolveInterruptedTaskWarningAfterPromptAcceptance(
	ctx context.Context,
	snapshot interruptedTaskWarningSnapshot,
	executionID, attemptID, turnID string,
) {
	if !snapshot.captured || snapshot.taskID == "" || snapshot.sessionID == "" {
		return
	}
	if snapshot.executionID != "" && snapshot.executionID != executionID {
		return
	}
	if snapshot.attemptID != "" && snapshot.attemptID != attemptID {
		return
	}
	if snapshot.turnID != "" && snapshot.turnID != turnID {
		return
	}
	resolver, ok := s.repo.(interface {
		ResolveTaskInterruptedSession(
			context.Context, string, string, string, string, string,
		) (bool, error)
	})
	if !ok {
		s.logger.Warn("task repository does not support interruption resolution",
			zap.String("task_id", snapshot.taskID))
		return
	}
	resolveCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), promptFailureCleanupTimeout)
	defer cancel()
	changed, err := resolver.ResolveTaskInterruptedSession(
		resolveCtx,
		snapshot.taskID,
		snapshot.sessionID,
		snapshot.incarnationID,
		snapshot.token,
		snapshot.legacyValue,
	)
	if err != nil {
		s.logger.Warn("failed to resolve accepted task interruption",
			zap.String("task_id", snapshot.taskID),
			zap.String("session_id", snapshot.sessionID),
			zap.Error(err))
		return
	}
	if !changed {
		return
	}
	task, err := s.repo.GetTask(resolveCtx, snapshot.taskID)
	if err != nil || task == nil {
		s.logger.Warn("failed to load task after interruption resolution",
			zap.String("task_id", snapshot.taskID), zap.Error(err))
		return
	}
	s.publishTaskUpdated(resolveCtx, task)
}
