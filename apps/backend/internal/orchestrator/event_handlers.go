// Package orchestrator provides event handler methods for the orchestrator service.
package orchestrator

import (
	"context"
	"errors"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
)

// Agent event type string constants.
const (
	agentEventComplete  = "complete"
	agentEventCompleted = "completed"
	agentEventError     = "error"
	agentEventToolCall  = "tool_call"
	agentEventFailed    = "failed"
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
	s.storeResumeToken(ctx, data.TaskID, data.SessionID, data.AgentExecutionID, data.ACPSessionID, "")
}

// storeResumeToken stores an agent's session ID as the resume token for session recovery.
// Called from handleACPSessionCreated, handleSessionStatusEvent, and handleCompleteStreamEvent.
//
// expectedExecID identifies the lifecycle execution that emitted the event. The repo
// performs a CAS update keyed on agent_execution_id: if the row has rotated to a
// different execution since this event was queued, the update is rejected with
// models.ErrExecutionRotated and the stale token is silently dropped. This prevents
// a previous execution's tail-end events (post model-switch / context-reset) from
// overwriting the live execution's resume_token with a defunct ACP session ID.
//
// Pass expectedExecID="" only when the caller genuinely doesn't care which execution
// the event came from (no callers should today; the parameter exists for forward
// compatibility with paths that don't have an execution context).
//
// The token is always stored when CAS succeeds. NativeSessionResume only gates ACP
// session/load vs session/new in session.go — agents without native resume (e.g.,
// Claude Code) use the token for their own --resume CLI flag instead.
func (s *Service) storeResumeToken(ctx context.Context, taskID, sessionID, expectedExecID, acpSessionID, lastMessageUUID string) {
	err := s.repo.UpdateResumeToken(ctx, sessionID, expectedExecID, acpSessionID, lastMessageUUID)
	switch {
	case err == nil:
		s.logger.Debug("stored resume token for session",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID),
			zap.String("expected_exec_id", expectedExecID),
			zap.String("resume_token", acpSessionID),
			zap.String("last_message_uuid", lastMessageUUID))
	case errors.Is(err, models.ErrExecutionRotated):
		// The row's agent_execution_id has rotated since the event was emitted —
		// the token belongs to a defunct execution and must be dropped. The new
		// execution will emit its own ACP session created event and a fresh
		// storeResumeToken will succeed.
		s.logger.Info("dropping resume token from rotated execution",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID),
			zap.String("expected_exec_id", expectedExecID))
	case errors.Is(err, models.ErrExecutorRunningNotFound):
		// No executors_running row for this session. With the lifecycle-manager-
		// owned persistence model this should never happen for a live execution
		// (the row is created in lockstep with the in-memory Add). Most likely
		// causes: a session was torn down between event emission and handling,
		// or a code path that emits ACP events without a registered execution.
		// Logged loud; nothing to retry.
		s.logger.Warn("no executors_running row when storing resume token",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID),
			zap.String("expected_exec_id", expectedExecID))
	default:
		s.logger.Warn("failed to persist resume token for session",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID),
			zap.Error(err))
	}
}
