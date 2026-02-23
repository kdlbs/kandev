package orchestrator

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/lifecycle"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

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

	case "thinking_streaming":
		s.handleThinkingStreamingEvent(ctx, payload)

	case "tool_call":
		s.saveAgentTextIfPresent(ctx, payload)
		s.handleToolCallEvent(ctx, payload)

	case "tool_update":
		s.handleToolUpdateEvent(ctx, payload)

	case agentEventComplete:
		s.handleCompleteStreamEvent(ctx, payload)

	case agentEventError:
		s.handleAgentErrorEvent(ctx, payload)

	case "session_status":
		s.handleSessionStatusEvent(ctx, payload)

	case "available_commands":
		s.handleAvailableCommandsEvent(ctx, payload)

	case "session_mode":
		s.handleSessionModeEvent(ctx, payload)

	case "permission_cancelled":
		s.handlePermissionCancelledEvent(ctx, payload)

	case "log":
		s.handleAgentLogEvent(ctx, payload)
	}
}

// handleAgentErrorEvent handles agentEventError events by creating an error message and completing the turn.
func (s *Service) handleAgentErrorEvent(ctx context.Context, payload *lifecycle.AgentStreamEventPayload) {
	taskID := payload.TaskID
	sessionID := payload.SessionID
	if sessionID != "" && s.messageCreator != nil {
		errorMsg := payload.Data.Error
		if errorMsg == "" {
			errorMsg = payload.Data.Text
		}
		if errorMsg == "" {
			errorMsg = "An error occurred while processing your request"
		}
		metadata := map[string]interface{}{
			"provider":       "agent",
			"provider_agent": payload.AgentID,
		}
		if payload.Data.Data != nil {
			metadata["error_data"] = payload.Data.Data
		}
		if err := s.messageCreator.CreateSessionMessage(
			ctx, taskID, errorMsg, sessionID,
			string(v1.MessageTypeError), s.getActiveTurnID(sessionID), metadata, false,
		); err != nil {
			s.logger.Error("failed to create error message",
				zap.String("task_id", taskID),
				zap.Error(err))
		}
	}
	s.completeTurnForSession(ctx, sessionID)
}

// handleSessionStatusEvent handles session_status events by storing resume token and creating a status message.
func (s *Service) handleSessionStatusEvent(ctx context.Context, payload *lifecycle.AgentStreamEventPayload) {
	taskID := payload.TaskID
	sessionID := payload.SessionID
	if sessionID != "" && payload.Data.ACPSessionID != "" {
		s.storeResumeToken(ctx, taskID, sessionID, payload.Data.ACPSessionID, "")
	}
	if sessionID == "" || s.messageCreator == nil {
		return
	}
	statusMsg := "New session started"
	if payload.Data.SessionStatus == "resumed" {
		statusMsg = "Session resumed"
	}
	if err := s.messageCreator.CreateSessionMessage(
		ctx, taskID, statusMsg, sessionID,
		string(v1.MessageTypeStatus), s.getActiveTurnID(sessionID), nil, false,
	); err != nil {
		s.logger.Error("failed to create session status message",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID),
			zap.Error(err))
	}
}

// handleAgentLogEvent handles log events by storing agent log messages to the database.
func (s *Service) handleAgentLogEvent(ctx context.Context, payload *lifecycle.AgentStreamEventPayload) {
	taskID := payload.TaskID
	sessionID := payload.SessionID
	if sessionID == "" || s.messageCreator == nil {
		return
	}
	dataMap, _ := payload.Data.Data.(map[string]interface{})
	logMsg := payload.Data.Text
	if logMsg == "" && dataMap != nil {
		if msg, ok := dataMap["message"].(string); ok {
			logMsg = msg
		}
	}
	if logMsg == "" {
		return
	}
	metadata := map[string]interface{}{
		"provider":       "agent",
		"provider_agent": payload.AgentID,
	}
	if dataMap != nil {
		if level, ok := dataMap["level"].(string); ok {
			metadata["level"] = level
		}
		for k, v := range dataMap {
			if k != "message" && k != "level" {
				metadata[k] = v
			}
		}
	}
	if err := s.messageCreator.CreateSessionMessage(
		ctx, taskID, logMsg, sessionID,
		string(v1.MessageTypeLog), s.getActiveTurnID(sessionID), metadata, false,
	); err != nil {
		s.logger.Error("failed to create log message",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID),
			zap.Error(err))
	} else {
		level := "unknown"
		if l, ok := metadata["level"].(string); ok {
			level = l
		}
		s.logger.Debug("created log message",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID),
			zap.String("level", level))
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
			payload.Data.ParentToolCallID, // Pass parent for subagent nesting
			payload.Data.ToolTitle,
			payload.Data.ToolStatus,
			payload.SessionID,
			s.getActiveTurnID(payload.SessionID),
			payload.Data.Normalized, // Pass normalized tool data for message metadata
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

		// Allow tool calls to wake session from WAITING_FOR_INPUT
		// This ensures that when user responds to a clarification and the agent continues,
		// the session properly transitions to RUNNING
		s.updateTaskSessionState(ctx, payload.TaskID, payload.SessionID, models.TaskSessionStateRunning, "", true)
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

// handleStreamingEventKind is the shared implementation for streaming message and thinking events.
// appendFn appends content to an existing message; createFn creates a new streaming message.
func (s *Service) handleStreamingEventKind(
	ctx context.Context,
	payload *lifecycle.AgentStreamEventPayload,
	kind string,
	appendFn func(context.Context, string, string) error,
	createFn func(context.Context, string, string, string, string, string) error,
) {
	if payload.Data.Text == "" || payload.SessionID == "" {
		return
	}
	if s.messageCreator == nil {
		return
	}
	messageID := payload.Data.MessageID
	if messageID == "" {
		s.logger.Warn("streaming "+kind+" event missing message ID",
			zap.String("task_id", payload.TaskID),
			zap.String("session_id", payload.SessionID))
		return
	}
	if payload.Data.IsAppend {
		s.appendStreamingChunk(ctx, kind, messageID, payload.TaskID, payload.Data.Text, appendFn)
		return
	}
	turnID := s.getActiveTurnID(payload.SessionID)
	s.createStreamingChunk(ctx, kind, messageID, payload.TaskID, payload.Data.Text, payload.SessionID, turnID, createFn)
}

// handleMessageStreamingEvent handles streaming message events for real-time text updates.
// It creates a new message on first chunk (IsAppend=false) or appends to existing (IsAppend=true).
func (s *Service) handleMessageStreamingEvent(ctx context.Context, payload *lifecycle.AgentStreamEventPayload) {
	s.handleStreamingEventKind(ctx, payload, "message",
		s.messageCreator.AppendAgentMessage,
		s.messageCreator.CreateAgentMessageStreaming)
}

// handleThinkingStreamingEvent handles streaming thinking events for real-time reasoning updates.
// It creates a new thinking message on first chunk (IsAppend=false) or appends to existing (IsAppend=true).
func (s *Service) handleThinkingStreamingEvent(ctx context.Context, payload *lifecycle.AgentStreamEventPayload) {
	s.handleStreamingEventKind(ctx, payload, "thinking message",
		s.messageCreator.AppendThinkingMessage,
		s.messageCreator.CreateThinkingMessageStreaming)
}

// handleToolUpdateEvent handles tool_update events and updates messages
func (s *Service) handleToolUpdateEvent(ctx context.Context, payload *lifecycle.AgentStreamEventPayload) {
	if payload.SessionID == "" {
		s.logger.Warn("missing session_id for tool_update",
			zap.String("task_id", payload.TaskID),
			zap.String("tool_call_id", payload.Data.ToolCallID))
		return
	}

	if s.messageCreator == nil {
		return
	}

	// Determine message type from normalized payload for fallback creation
	msgType := toolKindToMessageType(payload.Data.Normalized)

	// Handle all status updates (running, complete, error)
	switch payload.Data.ToolStatus {
	case "running", agentEventComplete, "completed", "success", agentEventError, "failed":
		if err := s.messageCreator.UpdateToolCallMessage(
			ctx,
			payload.TaskID,
			payload.Data.ToolCallID,
			payload.Data.ParentToolCallID, // Pass parent for subagent nesting
			payload.Data.ToolStatus,
			"", // result - no longer used, tool results in NormalizedPayload
			payload.SessionID,
			payload.Data.ToolTitle,               // Include title from update event
			s.getActiveTurnID(payload.SessionID), // Turn ID for fallback creation
			msgType,                              // Message type for fallback creation
			payload.Data.Normalized,              // Pass normalized tool data for message metadata
		); err != nil {
			s.logger.Warn("failed to update tool call message",
				zap.String("task_id", payload.TaskID),
				zap.String("tool_call_id", payload.Data.ToolCallID),
				zap.Error(err))
		}

		// Update session state for completion events
		// Allow tool completions to wake session from WAITING_FOR_INPUT
		if payload.Data.ToolStatus == agentEventComplete || payload.Data.ToolStatus == "completed" ||
			payload.Data.ToolStatus == "success" || payload.Data.ToolStatus == agentEventError || payload.Data.ToolStatus == "failed" {
			s.updateTaskSessionState(ctx, payload.TaskID, payload.SessionID, models.TaskSessionStateRunning, "", true)
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
		eventData := map[string]interface{}{
			"task_id":                taskID,
			"session_id":             sessionID,
			"old_state":              string(oldState),
			"new_state":              string(nextState),
			"error_message":          errorMessage,
			"agent_profile_id":       session.AgentProfileID,
			"agent_profile_snapshot": session.AgentProfileSnapshot,
			"is_passthrough":         session.IsPassthrough,
		}
		// Include review_status and workflow_step_id if present to ensure frontend state consistency
		if session.ReviewStatus != nil {
			eventData["review_status"] = *session.ReviewStatus
		}
		if session.WorkflowStepID != nil {
			eventData["workflow_step_id"] = *session.WorkflowStepID
		}
		_ = s.eventBus.Publish(ctx, events.TaskSessionStateChanged, bus.NewEvent(events.TaskSessionStateChanged, "task-session", eventData))
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

// handleCompleteStreamEvent handles the agentEventComplete stream event.
func (s *Service) handleCompleteStreamEvent(ctx context.Context, payload *lifecycle.AgentStreamEventPayload) {
	s.logger.Debug("orchestrator received complete event",
		zap.String("task_id", payload.TaskID),
		zap.String("session_id", payload.SessionID),
		zap.Int("text_length", len(payload.Data.Text)),
		zap.Bool("has_text", payload.Data.Text != ""))

	// Update resume token with latest ACP session ID and message UUID on every turn.
	// This ensures the token stays current after session compaction.
	if payload.SessionID != "" && payload.Data.ACPSessionID != "" {
		var lastMsgUUID string
		if data, ok := payload.Data.Data.(map[string]interface{}); ok {
			if uuid, ok := data["last_message_uuid"].(string); ok {
				lastMsgUUID = uuid
			}
		}
		s.storeResumeToken(ctx, payload.TaskID, payload.SessionID, payload.Data.ACPSessionID, lastMsgUUID)
	}

	s.saveAgentTextIfPresent(ctx, payload)
	s.completeTurnForSession(ctx, payload.SessionID)
	s.setSessionWaitingForInput(ctx, payload.TaskID, payload.SessionID)
}

// handleAvailableCommandsEvent broadcasts available_commands events to the WebSocket for the frontend.
func (s *Service) handleAvailableCommandsEvent(ctx context.Context, payload *lifecycle.AgentStreamEventPayload) {
	sessionID := payload.SessionID
	if sessionID == "" || s.eventBus == nil || len(payload.Data.AvailableCommands) == 0 {
		return
	}
	eventPayload := lifecycle.AvailableCommandsEventPayload{
		TaskID:            payload.TaskID,
		SessionID:         sessionID,
		AgentID:           payload.AgentID,
		AvailableCommands: payload.Data.AvailableCommands,
	}
	subject := events.BuildAvailableCommandsSubject(sessionID)
	_ = s.eventBus.Publish(ctx, subject, bus.NewEvent(events.AvailableCommandsUpdated, "orchestrator", eventPayload))
}

// handleSessionModeEvent broadcasts session_mode events to the WebSocket for the frontend.
func (s *Service) handleSessionModeEvent(ctx context.Context, payload *lifecycle.AgentStreamEventPayload) {
	sessionID := payload.SessionID
	if sessionID == "" || s.eventBus == nil || payload.Data.CurrentModeID == "" {
		return
	}
	eventPayload := lifecycle.SessionModeEventPayload{
		TaskID:        payload.TaskID,
		SessionID:     sessionID,
		AgentID:       payload.AgentID,
		CurrentModeID: payload.Data.CurrentModeID,
		Timestamp:     time.Now().UTC().Format(time.RFC3339),
	}
	subject := events.BuildSessionModeSubject(sessionID)
	_ = s.eventBus.Publish(ctx, subject, bus.NewEvent(events.SessionModeChanged, "orchestrator", eventPayload))
}

// handlePermissionCancelledEvent marks the pending permission message as expired.
func (s *Service) handlePermissionCancelledEvent(ctx context.Context, payload *lifecycle.AgentStreamEventPayload) {
	sessionID := payload.SessionID
	if sessionID == "" || payload.Data.PendingID == "" || s.messageCreator == nil {
		return
	}
	if err := s.messageCreator.UpdatePermissionMessage(ctx, sessionID, payload.Data.PendingID, "expired"); err != nil {
		s.logger.Warn("failed to mark permission as expired",
			zap.String("session_id", sessionID),
			zap.String("pending_id", payload.Data.PendingID),
			zap.Error(err))
	}
}

// appendStreamingChunk appends a text chunk to an existing streaming message.
func (s *Service) appendStreamingChunk(ctx context.Context, kind, messageID, taskID, text string, appendFn func(context.Context, string, string) error) {
	if err := appendFn(ctx, messageID, text); err != nil {
		s.logger.Error("failed to append to streaming "+kind,
			zap.String("task_id", taskID),
			zap.String("message_id", messageID),
			zap.Error(err))
		return
	}
	s.logger.Debug("appended to streaming "+kind,
		zap.String("task_id", taskID),
		zap.String("message_id", messageID),
		zap.Int("content_length", len(text)))
}

// createStreamingChunk creates a new streaming message for the first chunk.
func (s *Service) createStreamingChunk(ctx context.Context, kind, messageID, taskID, text, sessionID, turnID string, createFn func(context.Context, string, string, string, string, string) error) {
	if err := createFn(ctx, messageID, taskID, text, sessionID, turnID); err != nil {
		s.logger.Error("failed to create streaming "+kind,
			zap.String("task_id", taskID),
			zap.String("message_id", messageID),
			zap.Error(err))
		return
	}
	s.logger.Debug("created streaming "+kind,
		zap.String("task_id", taskID),
		zap.String("session_id", sessionID),
		zap.String("message_id", messageID),
		zap.Int("content_length", len(text)))
}
