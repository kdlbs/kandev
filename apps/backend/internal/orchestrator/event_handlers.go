// Package orchestrator provides event handler methods for the orchestrator service.
package orchestrator

import (
	"context"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/lifecycle"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// Event handlers

func (s *Service) handleACPSessionCreated(ctx context.Context, data watcher.ACPSessionEventData) {
	if data.SessionID == "" || data.ACPSessionID == "" {
		return
	}

	session, err := s.repo.GetTaskSession(ctx, data.SessionID)
	if err != nil {
		s.logger.Warn("failed to load task session for ACP session update",
			zap.String("task_id", data.TaskID),
			zap.String("session_id", data.SessionID),
			zap.Error(err))
		return
	}

	resumable := true
	if session.ExecutorID != "" {
		if executor, err := s.repo.GetExecutor(ctx, session.ExecutorID); err == nil && executor != nil {
			resumable = executor.Resumable
		}
	}

	running := &models.ExecutorRunning{
		ID:               session.ID,
		SessionID:        session.ID,
		TaskID:           session.TaskID,
		ExecutorID:       session.ExecutorID,
		Status:           "ready",
		Resumable:        resumable,
		ResumeToken:      data.ACPSessionID,
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
			zap.String("task_id", data.TaskID),
			zap.String("session_id", data.SessionID),
			zap.Error(err))
		return
	}

	s.logger.Debug("stored resume token for task session",
		zap.String("task_id", data.TaskID),
		zap.String("session_id", data.SessionID),
		zap.String("resume_token", data.ACPSessionID))
}

// handleAgentCompleted handles agent completion events
func (s *Service) handleAgentCompleted(ctx context.Context, data watcher.AgentEventData) {
	s.logger.Info("handling agent completed",
		zap.String("task_id", data.TaskID),
		zap.String("agent_execution_id", data.AgentExecutionID))

	// Update scheduler and remove from queue
	s.scheduler.HandleTaskCompleted(data.TaskID, true)
	s.scheduler.RemoveTask(data.TaskID)

	// Move task to REVIEW state for user review
	// The user can then send a follow-up (moves back to IN_PROGRESS) or mark as COMPLETED
	if err := s.taskRepo.UpdateTaskState(ctx, data.TaskID, v1.TaskStateReview); err != nil {
		s.logger.Error("failed to update task state to REVIEW",
			zap.String("task_id", data.TaskID),
			zap.Error(err))
	} else {
		s.logger.Info("task moved to REVIEW state after agent completion",
			zap.String("task_id", data.TaskID))
	}
}

// handleAgentFailed handles agent failure events
func (s *Service) handleAgentFailed(ctx context.Context, data watcher.AgentEventData) {
	s.logger.Warn("handling agent failed",
		zap.String("task_id", data.TaskID),
		zap.String("agent_execution_id", data.AgentExecutionID),
		zap.String("error_message", data.ErrorMessage))

	// Trigger retry logic
	s.scheduler.HandleTaskCompleted(data.TaskID, false)
	if !s.scheduler.RetryTask(data.TaskID) {
		s.logger.Error("task failed and retry limit exceeded",
			zap.String("task_id", data.TaskID))

		// Move task to REVIEW state even on failure - user can decide to retry or close
		// This maintains the review cycle: user reviews the failure and decides next steps
		if err := s.taskRepo.UpdateTaskState(ctx, data.TaskID, v1.TaskStateReview); err != nil {
			s.logger.Error("failed to update task state to REVIEW after failure",
				zap.String("task_id", data.TaskID),
				zap.Error(err))
		} else {
			s.logger.Info("task moved to REVIEW state after failure (for user review)",
				zap.String("task_id", data.TaskID))
		}
	}
}

// handleAgentStreamEvent handles agent stream events (tool calls, message chunks, etc.)
func (s *Service) handleAgentStreamEvent(ctx context.Context, payload *lifecycle.AgentStreamEventPayload) {
	if payload == nil || payload.Data == nil {
		return
	}

	taskID := payload.TaskID
	sessionID := payload.SessionID
	eventType := payload.Data.Type

	s.logger.Debug("handling agent stream event",
		zap.String("task_id", taskID),
		zap.String("session_id", sessionID),
		zap.String("event_type", eventType))

	// Handle different event types
	switch eventType {
	case "message_streaming":
		s.handleMessageStreamingEvent(ctx, payload)

	case "tool_call":
		s.saveAgentTextIfPresent(ctx, payload)
		s.handleToolCallEvent(ctx, payload)

	case "tool_update":
		s.handleToolUpdateEvent(ctx, payload)

	case "complete":
		s.saveAgentTextIfPresent(ctx, payload)
		s.completeTurnForSession(ctx, sessionID)
		s.setSessionWaitingForInput(ctx, taskID, sessionID)

	case "error":
		// Handle error events
		// Note: Agent-specific error parsing is done in the adapter layer.
		// The adapter provides a user-friendly message in the Text field,
		// with raw details in the Data field for debugging.
		if sessionID != "" && s.messageCreator != nil {
			// Build error message - adapters should provide parsed message in Text field
			errorMsg := payload.Data.Error
			if errorMsg == "" {
				errorMsg = payload.Data.Text
			}
			if errorMsg == "" {
				errorMsg = "An error occurred while processing your request"
			}

			// Build metadata with all available error context
			metadata := map[string]interface{}{
				"provider":       "agent",
				"provider_agent": payload.AgentID,
			}
			if payload.Data.Data != nil {
				metadata["error_data"] = payload.Data.Data
			}

			if err := s.messageCreator.CreateSessionMessage(
				ctx,
				taskID,
				errorMsg,
				sessionID,
				string(v1.MessageTypeError),
				s.getActiveTurnID(sessionID),
				metadata,
				false,
			); err != nil {
				s.logger.Error("failed to create error message",
					zap.String("task_id", taskID),
					zap.Error(err))
			}
		}
		// Complete the turn since the agent errored
		s.completeTurnForSession(ctx, sessionID)

	case "session_status":
		// Handle session status events (resumed vs new session)
		if sessionID != "" && s.messageCreator != nil {
			var statusMsg string
			if payload.Data.SessionStatus == "resumed" {
				statusMsg = "Session resumed"
			} else {
				statusMsg = "New session started"
			}
			if err := s.messageCreator.CreateSessionMessage(
				ctx,
				taskID,
				statusMsg,
				sessionID,
				string(v1.MessageTypeStatus),
				s.getActiveTurnID(sessionID),
				nil,
				false,
			); err != nil {
				s.logger.Error("failed to create session status message",
					zap.String("task_id", taskID),
					zap.String("session_id", sessionID),
					zap.Error(err))
			}
		}
	}
}

// handleToolCallEvent handles tool_call events and creates messages
func (s *Service) handleToolCallEvent(ctx context.Context, payload *lifecycle.AgentStreamEventPayload) {
	if payload.SessionID == "" {
		s.logger.Warn("missing session_id for tool_call",
			zap.String("task_id", payload.TaskID),
			zap.String("tool_call_id", payload.Data.ToolCallID))
		return
	}

	if s.messageCreator != nil {
		if err := s.messageCreator.CreateToolCallMessage(
			ctx,
			payload.TaskID,
			payload.Data.ToolCallID,
			payload.Data.ToolTitle,
			payload.Data.ToolStatus,
			payload.SessionID,
			s.getActiveTurnID(payload.SessionID),
			payload.Data.ToolArgs,
		); err != nil {
			s.logger.Error("failed to create tool call message",
				zap.String("task_id", payload.TaskID),
				zap.String("tool_call_id", payload.Data.ToolCallID),
				zap.Error(err))
		} else {
			s.logger.Debug("created tool call message",
				zap.String("task_id", payload.TaskID),
				zap.String("tool_call_id", payload.Data.ToolCallID))
		}

		s.updateTaskSessionState(ctx, payload.TaskID, payload.SessionID, models.TaskSessionStateRunning, "", false)
	}
}

// saveAgentTextIfPresent saves any accumulated agent text as an agent message
func (s *Service) saveAgentTextIfPresent(ctx context.Context, payload *lifecycle.AgentStreamEventPayload) {
	if payload.Data.Text == "" || payload.SessionID == "" {
		return
	}

	if s.messageCreator != nil {
		if err := s.messageCreator.CreateAgentMessage(ctx, payload.TaskID, payload.Data.Text, payload.SessionID, s.getActiveTurnID(payload.SessionID)); err != nil {
			s.logger.Error("failed to create agent message",
				zap.String("task_id", payload.TaskID),
				zap.Error(err))
		} else {
			s.logger.Debug("created agent message",
				zap.String("task_id", payload.TaskID),
				zap.Int("message_length", len(payload.Data.Text)))
		}
	}
}

// handleMessageStreamingEvent handles streaming message events for real-time text updates.
// It creates a new message on first chunk (IsAppend=false) or appends to existing (IsAppend=true).
func (s *Service) handleMessageStreamingEvent(ctx context.Context, payload *lifecycle.AgentStreamEventPayload) {
	if payload.Data.Text == "" || payload.SessionID == "" {
		return
	}

	if s.messageCreator == nil {
		return
	}

	// MessageID is pre-generated by the manager to avoid race conditions
	messageID := payload.Data.MessageID
	if messageID == "" {
		s.logger.Warn("streaming message event missing message ID",
			zap.String("task_id", payload.TaskID),
			zap.String("session_id", payload.SessionID))
		return
	}

	if payload.Data.IsAppend {
		// Append to existing message
		if err := s.messageCreator.AppendAgentMessage(ctx, messageID, payload.Data.Text); err != nil {
			s.logger.Error("failed to append to streaming message",
				zap.String("task_id", payload.TaskID),
				zap.String("message_id", messageID),
				zap.Error(err))
		} else {
			s.logger.Debug("appended to streaming message",
				zap.String("task_id", payload.TaskID),
				zap.String("message_id", messageID),
				zap.Int("content_length", len(payload.Data.Text)))
		}
	} else {
		// Create new streaming message with the pre-generated ID
		if err := s.messageCreator.CreateAgentMessageStreaming(ctx, messageID, payload.TaskID, payload.Data.Text, payload.SessionID, s.getActiveTurnID(payload.SessionID)); err != nil {
			s.logger.Error("failed to create streaming message",
				zap.String("task_id", payload.TaskID),
				zap.String("message_id", messageID),
				zap.Error(err))
		} else {
			s.logger.Debug("created streaming message",
				zap.String("task_id", payload.TaskID),
				zap.String("session_id", payload.SessionID),
				zap.String("message_id", messageID),
				zap.Int("content_length", len(payload.Data.Text)))
		}
	}
}

// handleToolUpdateEvent handles tool_update events and updates messages
func (s *Service) handleToolUpdateEvent(ctx context.Context, payload *lifecycle.AgentStreamEventPayload) {
	if payload.SessionID == "" {
		s.logger.Warn("missing session_id for tool_update",
			zap.String("task_id", payload.TaskID),
			zap.String("tool_call_id", payload.Data.ToolCallID))
		return
	}

	// Only update message when tool call completes or errors
	switch payload.Data.ToolStatus {
	case "complete", "completed", "error", "failed":
		if s.messageCreator != nil {
			result := ""
			if payload.Data.ToolResult != nil {
				if str, ok := payload.Data.ToolResult.(string); ok {
					result = str
				}
			}
			if err := s.messageCreator.UpdateToolCallMessage(
				ctx,
				payload.TaskID,
				payload.Data.ToolCallID,
				payload.Data.ToolStatus,
				result,
				payload.SessionID,
			); err != nil {
				s.logger.Warn("failed to update tool call message",
					zap.String("task_id", payload.TaskID),
					zap.String("tool_call_id", payload.Data.ToolCallID),
					zap.Error(err))
			}

			s.updateTaskSessionState(ctx, payload.TaskID, payload.SessionID, models.TaskSessionStateRunning, "", false)
		}
	}
}

func (s *Service) updateTaskSessionState(ctx context.Context, taskID, sessionID string, nextState models.TaskSessionState, errorMessage string, allowWakeFromWaiting bool) {
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return
	}
	if session.State == models.TaskSessionStateWaitingForInput && nextState == models.TaskSessionStateRunning && !allowWakeFromWaiting {
		return
	}
	oldState := session.State
	switch session.State {
	case models.TaskSessionStateCompleted, models.TaskSessionStateFailed, models.TaskSessionStateCancelled:
		return
	}
	if session.State == nextState {
		return
	}
	if err := s.repo.UpdateTaskSessionState(ctx, sessionID, nextState, errorMessage); err != nil {
		s.logger.Error("failed to update task session state",
			zap.String("session_id", sessionID),
			zap.String("state", string(nextState)),
			zap.Error(err))
	}
	s.logger.Debug("task session state updated",
		zap.String("task_id", taskID),
		zap.String("session_id", sessionID),
		zap.String("old_state", string(oldState)),
		zap.String("new_state", string(nextState)))
	if s.eventBus != nil {
		_ = s.eventBus.Publish(ctx, events.TaskSessionStateChanged, bus.NewEvent(events.TaskSessionStateChanged, "task-session", map[string]interface{}{
			"task_id":          taskID,
			"session_id":       sessionID,
			"old_state":        string(oldState),
			"new_state":        string(nextState),
			"agent_profile_id": session.AgentProfileID,
		}))
	}
}

func (s *Service) setSessionWaitingForInput(ctx context.Context, taskID, sessionID string) {
	s.updateTaskSessionState(ctx, taskID, sessionID, models.TaskSessionStateWaitingForInput, "", false)

	if err := s.taskRepo.UpdateTaskState(ctx, taskID, v1.TaskStateReview); err != nil {
		s.logger.Error("failed to update task state to REVIEW",
			zap.String("task_id", taskID),
			zap.Error(err))
	} else {
		s.logger.Info("task moved to REVIEW state",
			zap.String("task_id", taskID))
	}
}

func (s *Service) setSessionRunning(ctx context.Context, taskID, sessionID string) {
	s.updateTaskSessionState(ctx, taskID, sessionID, models.TaskSessionStateRunning, "", true)

	if err := s.taskRepo.UpdateTaskState(ctx, taskID, v1.TaskStateInProgress); err != nil {
		s.logger.Error("failed to update task state to IN_PROGRESS",
			zap.String("task_id", taskID),
			zap.Error(err))
	} else {
		s.logger.Info("task moved to IN_PROGRESS state",
			zap.String("task_id", taskID))
	}
}

// handleGitStatusUpdated handles git status updates and persists them to agent session metadata
func (s *Service) handleGitStatusUpdated(ctx context.Context, data watcher.GitStatusData) {
	s.logger.Debug("handling git status update",
		zap.String("task_id", data.TaskID),
		zap.String("branch", data.Branch))

	if data.TaskSessionID == "" {
		s.logger.Debug("missing session_id for git status update",
			zap.String("task_id", data.TaskID))
		return
	}

	session, err := s.repo.GetTaskSession(ctx, data.TaskSessionID)
	if err != nil {
		s.logger.Debug("no task session for git status update",
			zap.String("session_id", data.TaskSessionID),
			zap.Error(err))
		return
	}

	// Update session metadata with git status
	if session.Metadata == nil {
		session.Metadata = make(map[string]interface{})
	}
	session.Metadata["git_status"] = map[string]interface{}{
		"branch":        data.Branch,
		"remote_branch": data.RemoteBranch,
		"modified":      data.Modified,
		"added":         data.Added,
		"deleted":       data.Deleted,
		"untracked":     data.Untracked,
		"renamed":       data.Renamed,
		"ahead":         data.Ahead,
		"behind":        data.Behind,
		"files":         data.Files,
		"timestamp":     data.Timestamp,
	}

	// Persist to database asynchronously
	go func() {
		if err := s.repo.UpdateTaskSession(context.Background(), session); err != nil {
			s.logger.Error("failed to update agent session with git status",
				zap.String("task_id", data.TaskID),
				zap.String("session_id", session.ID),
				zap.Error(err))
		} else {
			s.logger.Debug("persisted git status to agent session",
				zap.String("task_id", data.TaskID),
				zap.String("session_id", session.ID))
		}
	}()
}

// handleContextWindowUpdated handles context window updates and persists them to session metadata
func (s *Service) handleContextWindowUpdated(ctx context.Context, data watcher.ContextWindowData) {
	s.logger.Debug("handling context window update",
		zap.String("task_id", data.TaskID),
		zap.String("session_id", data.TaskSessionID),
		zap.Int64("size", data.ContextWindowSize),
		zap.Int64("used", data.ContextWindowUsed))

	if data.TaskSessionID == "" {
		s.logger.Debug("missing session_id for context window update",
			zap.String("task_id", data.TaskID))
		return
	}

	session, err := s.repo.GetTaskSession(ctx, data.TaskSessionID)
	if err != nil {
		s.logger.Debug("no task session for context window update",
			zap.String("session_id", data.TaskSessionID),
			zap.Error(err))
		return
	}

	// Update session metadata with context window info
	if session.Metadata == nil {
		session.Metadata = make(map[string]interface{})
	}
	contextWindowData := map[string]interface{}{
		"size":       data.ContextWindowSize,
		"used":       data.ContextWindowUsed,
		"remaining":  data.ContextWindowRemaining,
		"efficiency": data.ContextEfficiency,
	}
	session.Metadata["context_window"] = contextWindowData

	// Persist to database asynchronously
	go func() {
		if err := s.repo.UpdateTaskSession(context.Background(), session); err != nil {
			s.logger.Error("failed to update session with context window",
				zap.String("task_id", data.TaskID),
				zap.String("session_id", session.ID),
				zap.Error(err))
		} else {
			s.logger.Debug("persisted context window to session",
				zap.String("task_id", data.TaskID),
				zap.String("session_id", session.ID))
		}
	}()

	// Broadcast session state change with metadata so frontend can update
	// This uses the existing session.state_changed event with metadata included
	if s.eventBus != nil {
		_ = s.eventBus.Publish(ctx, events.TaskSessionStateChanged, bus.NewEvent(
			events.TaskSessionStateChanged,
			"orchestrator",
			map[string]interface{}{
				"task_id":          data.TaskID,
				"session_id":       session.ID,
				"new_state":        string(session.State),
				"agent_profile_id": session.AgentProfileID,
				"metadata": map[string]interface{}{
					"context_window": contextWindowData,
				},
			},
		))
	}
}

// handlePermissionRequest handles permission request events and saves as message
func (s *Service) handlePermissionRequest(ctx context.Context, data watcher.PermissionRequestData) {
	s.logger.Debug("handling permission request",
		zap.String("task_id", data.TaskID),
		zap.String("pending_id", data.PendingID),
		zap.String("title", data.Title))

	if data.TaskSessionID == "" {
		s.logger.Warn("missing session_id for permission_request",
			zap.String("task_id", data.TaskID),
			zap.String("pending_id", data.PendingID))
		return
	}

	s.setSessionWaitingForInput(ctx, data.TaskID, data.TaskSessionID)

	if s.messageCreator != nil {
		_, err := s.messageCreator.CreatePermissionRequestMessage(
			ctx,
			data.TaskID,
			data.TaskSessionID,
			data.PendingID,
			data.ToolCallID,
			data.Title,
			s.getActiveTurnID(data.TaskSessionID),
			data.Options,
			data.ActionType,
			data.ActionDetails,
		)
		if err != nil {
			s.logger.Error("failed to create permission request message",
				zap.String("task_id", data.TaskID),
				zap.String("pending_id", data.PendingID),
				zap.Error(err))
		} else {
			s.logger.Debug("created permission request message",
				zap.String("task_id", data.TaskID),
				zap.String("pending_id", data.PendingID))
		}
	}
}
