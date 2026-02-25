// Package orchestrator provides event handler methods for the orchestrator service.
package orchestrator

import (
	"context"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
)

// Agent event type string constants.
const (
	agentEventComplete = "complete"
	agentEventError    = "error"
)

// toolKindToMessageType maps the normalized tool kind to a frontend message type.
func toolKindToMessageType(normalized *streams.NormalizedPayload) string {
	if normalized == nil {
		return "tool_call"
	}
	return normalized.Kind().ToMessageType()
}

// Event handlers

func (s *Service) handleTaskDeleted(ctx context.Context, data watcher.TaskEventData) {
	s.scheduler.RemoveTask(data.TaskID)
}

func (s *Service) handleACPSessionCreated(ctx context.Context, data watcher.ACPSessionEventData) {
	if data.SessionID == "" || data.ACPSessionID == "" {
		return
	}
	s.storeResumeToken(ctx, data.TaskID, data.SessionID, data.ACPSessionID, "")
}

// storeResumeToken stores an agent's session ID as the resume token for session recovery.
// This is called from handleACPSessionCreated, handleSessionStatusEvent, and handleCompleteStreamEvent.
// The optional lastMessageUUID is persisted alongside the token for --resume-session-at support.
// The resume token is only stored for agents that support native session resume (ACP session/load).
func (s *Service) storeResumeToken(ctx context.Context, taskID, sessionID, acpSessionID, lastMessageUUID string, preloadedSession ...*models.TaskSession) {
	var session *models.TaskSession
	if len(preloadedSession) > 0 && preloadedSession[0] != nil {
		session = preloadedSession[0]
	} else {
		var err error
		session, err = s.repo.GetTaskSession(ctx, sessionID)
		if err != nil {
			s.logger.Warn("failed to load task session for resume token storage",
				zap.String("task_id", taskID),
				zap.String("session_id", sessionID),
				zap.Error(err))
			return
		}
	}

	// Clear the resume token for agents that don't support native session resume (ACP session/load).
	// The ExecutorRunning record is still created (for worktree info etc.) but without a token,
	// so the frontend won't attempt to auto-resume with a stale ACP session ID.
	if session.AgentProfileID != "" {
		profileInfo, profileErr := s.agentManager.ResolveAgentProfile(ctx, session.AgentProfileID)
		if profileErr == nil && profileInfo != nil && !profileInfo.NativeSessionResume {
			acpSessionID = ""
			lastMessageUUID = ""
		}
	}

	resumable := true
	runtimeName := ""
	if session.ExecutorID != "" {
		if executor, err := s.repo.GetExecutor(ctx, session.ExecutorID); err == nil && executor != nil {
			resumable = executor.Resumable
			runtimeName = string(executor.Type)
		}
	}

	running := &models.ExecutorRunning{
		ID:               session.ID,
		SessionID:        session.ID,
		TaskID:           session.TaskID,
		ExecutorID:       session.ExecutorID,
		Runtime:          runtimeName,
		Status:           "ready",
		Resumable:        resumable,
		ResumeToken:      acpSessionID,
		LastMessageUUID:  lastMessageUUID,
		AgentExecutionID: session.AgentExecutionID,
		ContainerID:      session.ContainerID,
	}
	if len(session.Worktrees) > 0 {
		running.WorktreeID = session.Worktrees[0].WorktreeID
		running.WorktreePath = session.Worktrees[0].WorktreePath
		running.WorktreeBranch = session.Worktrees[0].WorktreeBranch
	}

	if err := s.repo.UpsertExecutorRunning(ctx, running); err != nil {
		s.logger.Warn("failed to persist resume token for session",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID),
			zap.Error(err))
		return
	}

	s.logger.Debug("stored resume token for session",
		zap.String("task_id", taskID),
		zap.String("session_id", sessionID),
		zap.String("resume_token", acpSessionID),
		zap.String("last_message_uuid", lastMessageUUID))
}
