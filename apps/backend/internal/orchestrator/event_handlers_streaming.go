package orchestrator

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/lifecycle"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
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

	// Any agent stream activity means the agent resumed after clarification.
	// Cancel primary-path clarification watchdogs for this session.
	s.cancelClarificationWatchdogsForSession(sessionID, eventType)

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

	case agentEventToolCall:
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

	case "agent_capabilities":
		s.handleAgentCapabilitiesEvent(ctx, payload)

	case "session_models":
		s.handleSessionModelsEvent(ctx, payload)

	case "plan":
		s.handleSessionTodosEvent(ctx, payload)

	case "agent_plan":
		s.handleAgentPlanEvent(ctx, payload)

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
		s.storeResumeToken(ctx, taskID, sessionID, payload.ExecutionID, payload.Data.ACPSessionID, "")
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

		// Allow tool calls to wake session from WAITING_FOR_INPUT.
		// Use setSessionRunning (not updateTaskSessionState) so the task is
		// flipped to IN_PROGRESS in lockstep — otherwise an out-of-turn tool
		// event (e.g. a Monitor watcher firing after on_turn_complete moved
		// the task to REVIEW) leaves session=RUNNING with task=REVIEW.
		s.setSessionRunning(ctx, payload.TaskID, payload.SessionID)
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

	// Handle all status updates (running, complete, error, cancelled, pending, in_progress)
	switch payload.Data.ToolStatus {
	case "running", agentEventComplete, agentEventCompleted, "success", agentEventError, agentEventFailed, "cancelled", "pending", "in_progress":
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

		// Update session state for completion events.
		// Use setSessionRunning so the task flips to IN_PROGRESS alongside —
		// see comment in handleToolCallEvent for the REVIEW/RUNNING split bug.
		if payload.Data.ToolStatus == agentEventComplete || payload.Data.ToolStatus == agentEventCompleted ||
			payload.Data.ToolStatus == "success" || payload.Data.ToolStatus == agentEventError || payload.Data.ToolStatus == agentEventFailed {
			s.setSessionRunning(ctx, payload.TaskID, payload.SessionID)
		}
	}
}

// updateTaskSessionState transitions a session to nextState with guard checks.
// When a preloadedSession is provided, its State is used for guard conditions (terminal-state
// check, same-state check). This is an optimistic fast-path: between load and check another
// goroutine may have changed the state in the DB. The guards are best-effort to avoid
// unnecessary writes; the DB update via UpdateTaskSessionState is the atomic source of truth.
func (s *Service) updateTaskSessionState(ctx context.Context, taskID, sessionID string, nextState models.TaskSessionState, errorMessage string, allowWakeFromWaiting bool, preloadedSession ...*models.TaskSession) {
	var session *models.TaskSession
	if len(preloadedSession) > 0 && preloadedSession[0] != nil {
		session = preloadedSession[0]
	} else {
		var err error
		session, err = s.repo.GetTaskSession(ctx, sessionID)
		if err != nil {
			return
		}
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
		// Include review_status if present to ensure frontend state consistency
		if session.ReviewStatus != nil {
			eventData["review_status"] = *session.ReviewStatus
		}
		// Include session metadata (e.g. plan_mode set by workflow events).
		// Key is "session_metadata" to avoid conflict with message-level "metadata".
		if len(session.Metadata) > 0 {
			eventData["session_metadata"] = session.Metadata
		}
		// Propagate toast suppression flag set by failure handlers (e.g. missing branch).
		if suppressed, ok := s.suppressToast.LoadAndDelete(sessionID); ok && suppressed.(bool) {
			eventData["suppress_toast"] = true
		}
		if session.TaskEnvironmentID != "" {
			eventData["task_environment_id"] = session.TaskEnvironmentID
		}
		_ = s.eventBus.Publish(ctx, events.TaskSessionStateChanged, bus.NewEvent(events.TaskSessionStateChanged, "task-session", eventData))
	}

	// Auto-promote another session to primary when the current primary enters a terminal state
	s.maybePromotePrimary(ctx, taskID, sessionID, nextState)
}

// maybePromotePrimary promotes the next best active session to primary when the
// current primary session enters a terminal state (COMPLETED, FAILED, CANCELLED).
func (s *Service) maybePromotePrimary(ctx context.Context, taskID, sessionID string, newState models.TaskSessionState) {
	if !isTerminalSessionState(newState) {
		return
	}

	// Check whether the stopped session is actually the primary
	sessions, err := s.repo.ListTaskSessions(ctx, taskID)
	if err != nil {
		return
	}
	var stoppedIsPrimary bool
	for _, sess := range sessions {
		if sess.ID == sessionID && sess.IsPrimary {
			stoppedIsPrimary = true
			break
		}
	}
	if !stoppedIsPrimary {
		return
	}

	// Pick the best candidate: prefer RUNNING, then STARTING, then WAITING_FOR_INPUT
	var candidate string
	for _, sess := range sessions {
		if sess.ID == sessionID {
			continue
		}
		if sess.State == models.TaskSessionStateRunning {
			candidate = sess.ID
			break
		}
		if candidate == "" && isActiveSessionState(sess.State) {
			candidate = sess.ID
		}
	}
	if candidate != "" {
		if err := s.SetPrimarySession(ctx, candidate); err != nil {
			s.logger.Warn("failed to auto-promote primary session",
				zap.String("task_id", taskID),
				zap.String("candidate", candidate),
				zap.Error(err))
		} else {
			s.logger.Info("auto-promoted primary session",
				zap.String("task_id", taskID),
				zap.String("old_primary", sessionID),
				zap.String("new_primary", candidate))
		}
	}
}

func isTerminalSessionState(s models.TaskSessionState) bool {
	return s == models.TaskSessionStateCompleted ||
		s == models.TaskSessionStateFailed ||
		s == models.TaskSessionStateCancelled
}

func (s *Service) setSessionWaitingForInput(ctx context.Context, taskID, sessionID string, preloadedSession ...*models.TaskSession) {
	s.updateTaskSessionState(ctx, taskID, sessionID, models.TaskSessionStateWaitingForInput, "", false, preloadedSession...)

	if err := s.taskRepo.UpdateTaskState(ctx, taskID, v1.TaskStateReview); err != nil {
		s.logger.Error("failed to update task state to REVIEW",
			zap.String("task_id", taskID),
			zap.Error(err))
	} else {
		s.logger.Info("task moved to REVIEW state",
			zap.String("task_id", taskID))
	}
}

func (s *Service) setSessionRunning(ctx context.Context, taskID, sessionID string, preloadedSession ...*models.TaskSession) {
	// Resolve session up front so we can guard the task write against terminal
	// states. updateTaskSessionState silently no-ops for terminal sessions, so
	// without this guard a buffered tool event arriving after a CANCELLED /
	// FAILED / COMPLETED session would still clobber tasks.state to IN_PROGRESS.
	var session *models.TaskSession
	if len(preloadedSession) > 0 && preloadedSession[0] != nil {
		session = preloadedSession[0]
	} else {
		var err error
		session, err = s.repo.GetTaskSession(ctx, sessionID)
		if err != nil {
			return
		}
	}
	if isTerminalSessionState(session.State) {
		return
	}

	s.updateTaskSessionState(ctx, taskID, sessionID, models.TaskSessionStateRunning, "", true, session)

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
	s.logger.Debug("handling complete stream event",
		zap.String("task_id", payload.TaskID),
		zap.String("session_id", payload.SessionID))

	// Load session once up front — used by storeResumeToken, state check, and setSessionWaitingForInput.
	var session *models.TaskSession
	if payload.SessionID != "" {
		var err error
		session, err = s.repo.GetTaskSession(ctx, payload.SessionID)
		if err != nil {
			s.logger.Warn("skipping complete-event processing; session lookup failed",
				zap.String("task_id", payload.TaskID),
				zap.String("session_id", payload.SessionID),
				zap.Error(err))
			return
		}
	}

	// Update resume token with latest ACP session ID and message UUID on every turn.
	if payload.SessionID != "" && payload.Data.ACPSessionID != "" {
		var lastMsgUUID string
		if data, ok := payload.Data.Data.(map[string]interface{}); ok {
			if uuid, ok := data["last_message_uuid"].(string); ok {
				lastMsgUUID = uuid
			}
		}
		s.storeResumeToken(ctx, payload.TaskID, payload.SessionID, payload.ExecutionID, payload.Data.ACPSessionID, lastMsgUUID)
	}

	s.saveAgentTextIfPresent(ctx, payload)
	s.publishAgentPlanIfPresent(ctx, payload)
	s.publishPromptUsage(ctx, payload)
	s.completeTurnForSession(ctx, payload.SessionID)

	// Cancel any pending clarifications for this session so WaitForResponse
	// unblocks and later user responses go through the event fallback path.
	if s.clarificationCanceller != nil && payload.SessionID != "" {
		if n := s.clarificationCanceller.CancelSessionAndNotify(ctx, payload.SessionID); n > 0 {
			s.logger.Info("cancelled pending clarifications on turn complete",
				zap.String("session_id", payload.SessionID),
				zap.Int("count", n))
		}
	}

	// READY events own workflow transitions and queued prompt execution.
	// If we're still RUNNING here, avoid racing READY by forcing WAITING/REVIEW.
	if session != nil && session.State == models.TaskSessionStateRunning {
		s.logger.Debug("skipping complete-event terminal state update while session is running",
			zap.String("task_id", payload.TaskID),
			zap.String("session_id", payload.SessionID))
		return
	}

	s.setSessionWaitingForInput(ctx, payload.TaskID, payload.SessionID, session)
}

// handleAgentPlanEvent handles agent_plan events from tool calls (e.g. ExitPlanMode)
// and creates a dedicated agent_plan message in the session.
func (s *Service) handleAgentPlanEvent(ctx context.Context, payload *lifecycle.AgentStreamEventPayload) {
	if payload.SessionID == "" || payload.Data.PlanContent == "" || s.messageCreator == nil {
		return
	}
	sessionID := payload.SessionID
	if err := s.messageCreator.CreateSessionMessage(
		ctx, payload.TaskID, payload.Data.PlanContent, sessionID,
		string(models.MessageTypeAgentPlan), s.getActiveTurnID(sessionID), nil, false,
	); err != nil {
		s.logger.Error("failed to create agent plan message",
			zap.String("task_id", payload.TaskID),
			zap.String("session_id", sessionID),
			zap.Error(err))
	}
}

// publishAgentPlanIfPresent extracts plan_content from a complete event and creates
// a dedicated agent_plan message in the session.
func (s *Service) publishAgentPlanIfPresent(ctx context.Context, payload *lifecycle.AgentStreamEventPayload) {
	if payload.SessionID == "" || payload.Data.Data == nil || s.messageCreator == nil {
		return
	}
	dataMap, ok := payload.Data.Data.(map[string]interface{})
	if !ok {
		return
	}
	planContent, ok := dataMap["plan_content"].(string)
	if !ok || planContent == "" {
		return
	}

	sessionID := payload.SessionID
	if err := s.messageCreator.CreateSessionMessage(
		ctx, payload.TaskID, planContent, sessionID,
		string(models.MessageTypeAgentPlan), s.getActiveTurnID(sessionID), nil, false,
	); err != nil {
		s.logger.Error("failed to create agent plan message",
			zap.String("task_id", payload.TaskID),
			zap.String("session_id", sessionID),
			zap.Error(err))
	}
}

// publishPromptUsage broadcasts prompt token usage to the WebSocket for the frontend.
func (s *Service) publishPromptUsage(ctx context.Context, payload *lifecycle.AgentStreamEventPayload) {
	sessionID := payload.SessionID
	if sessionID == "" || s.eventBus == nil || payload.Data.Usage == nil {
		return
	}
	eventPayload := lifecycle.SessionPromptUsageEventPayload{
		TaskID:    payload.TaskID,
		SessionID: sessionID,
		AgentID:   payload.AgentID,
		Usage:     payload.Data.Usage,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
	subject := events.BuildSessionPromptUsageSubject(sessionID)
	_ = s.eventBus.Publish(ctx, subject, bus.NewEvent(events.SessionPromptUsageUpdated, "orchestrator", eventPayload))
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
// An empty CurrentModeID means the agent has exited its special mode (e.g. plan mode ended).
func (s *Service) handleSessionModeEvent(ctx context.Context, payload *lifecycle.AgentStreamEventPayload) {
	sessionID := payload.SessionID
	if sessionID == "" || s.eventBus == nil {
		return
	}
	eventPayload := lifecycle.SessionModeEventPayload{
		TaskID:         payload.TaskID,
		SessionID:      sessionID,
		AgentID:        payload.AgentID,
		CurrentModeID:  payload.Data.CurrentModeID,
		AvailableModes: payload.Data.AvailableModes,
		Timestamp:      time.Now().UTC().Format(time.RFC3339),
	}
	subject := events.BuildSessionModeSubject(sessionID)
	_ = s.eventBus.Publish(ctx, subject, bus.NewEvent(events.SessionModeChanged, "orchestrator", eventPayload))
}

// handleAgentCapabilitiesEvent broadcasts agent_capabilities events to the WebSocket.
func (s *Service) handleAgentCapabilitiesEvent(ctx context.Context, payload *lifecycle.AgentStreamEventPayload) {
	sessionID := payload.SessionID
	if sessionID == "" || s.eventBus == nil {
		return
	}
	eventPayload := lifecycle.AgentCapabilitiesEventPayload{
		TaskID:                  payload.TaskID,
		SessionID:               sessionID,
		AgentID:                 payload.AgentID,
		SupportsImage:           payload.Data.SupportsImage,
		SupportsAudio:           payload.Data.SupportsAudio,
		SupportsEmbeddedContext: payload.Data.SupportsEmbeddedContext,
		AuthMethods:             payload.Data.AuthMethods,
		Timestamp:               time.Now().UTC().Format(time.RFC3339),
	}
	subject := events.BuildAgentCapabilitiesSubject(sessionID)
	_ = s.eventBus.Publish(ctx, subject, bus.NewEvent(events.AgentCapabilitiesUpdated, "orchestrator", eventPayload))
}

// handleSessionModelsEvent broadcasts session_models events to the WebSocket
// and persists the current model to the session snapshot so it survives page refresh.
func (s *Service) handleSessionModelsEvent(ctx context.Context, payload *lifecycle.AgentStreamEventPayload) {
	sessionID := payload.SessionID
	if sessionID == "" || s.eventBus == nil {
		return
	}

	// Persist the ACP-reported current model to the session snapshot so it's
	// available after page refresh (SSR reads from the DB, not runtime state).
	if currentModel := payload.Data.CurrentModelID; currentModel != "" {
		s.persistSessionModel(ctx, sessionID, currentModel)
	}

	eventPayload := lifecycle.SessionModelsEventPayload{
		TaskID:         payload.TaskID,
		SessionID:      sessionID,
		AgentID:        payload.AgentID,
		CurrentModelID: payload.Data.CurrentModelID,
		Models:         payload.Data.SessionModels,
		ConfigOptions:  payload.Data.ConfigOptions,
		Timestamp:      time.Now().UTC().Format(time.RFC3339),
	}
	s.logger.Info("publishing session_models event to WS",
		zap.String("session_id", sessionID),
		zap.String("current_model_id", payload.Data.CurrentModelID),
		zap.Int("models_count", len(payload.Data.SessionModels)),
	)
	subject := events.BuildSessionModelsSubject(sessionID)
	_ = s.eventBus.Publish(ctx, subject, bus.NewEvent(events.SessionModelsUpdated, "orchestrator", eventPayload))
}

// persistSessionModel updates the session's AgentProfileSnapshot with the current model.
func (s *Service) persistSessionModel(ctx context.Context, sessionID, model string) {
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return
	}
	if session.AgentProfileSnapshot == nil {
		session.AgentProfileSnapshot = make(map[string]interface{})
	}
	existing, _ := session.AgentProfileSnapshot["model"].(string)
	if existing == model {
		return
	}
	session.AgentProfileSnapshot["model"] = model
	_ = s.repo.UpdateTaskSession(ctx, session)
	// Invalidate the message creator's model cache so subsequent messages use the new model.
	if s.messageCreator != nil {
		s.messageCreator.InvalidateModelCache(sessionID)
	}
}

// handleSessionTodosEvent broadcasts plan/todo entries to the WebSocket and persists
// them as a chat message so they survive page refresh and appear in the chat timeline.
func (s *Service) handleSessionTodosEvent(ctx context.Context, payload *lifecycle.AgentStreamEventPayload) {
	sessionID := payload.SessionID
	if sessionID == "" || s.eventBus == nil {
		return
	}
	entries := payload.Data.PlanEntries

	// Broadcast real-time update via event bus (updates the store's sessionTodos)
	eventPayload := lifecycle.SessionTodosEventPayload{
		TaskID:    payload.TaskID,
		SessionID: sessionID,
		AgentID:   payload.AgentID,
		Entries:   entries,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
	subject := events.BuildSessionTodosSubject(sessionID)
	_ = s.eventBus.Publish(ctx, subject, bus.NewEvent(events.SessionTodosUpdated, "orchestrator", eventPayload))

	// Persist as a chat message so todos appear in the timeline and survive refresh
	s.persistTodoMessage(ctx, payload.TaskID, sessionID, entries)
}

// persistTodoMessage creates a "todo" message with the todo entries as metadata.
// Empty entries are persisted too — they represent the agent clearing all todos.
func (s *Service) persistTodoMessage(ctx context.Context, taskID, sessionID string, entries []streams.PlanEntry) {
	if s.messageCreator == nil {
		return
	}
	todos := make([]map[string]interface{}, len(entries))
	for i, e := range entries {
		todos[i] = map[string]interface{}{
			"text":   e.Description,
			"status": e.Status,
			"done":   e.Status == agentEventCompleted,
		}
	}
	metadata := map[string]interface{}{"todos": todos}
	if err := s.messageCreator.CreateSessionMessage(
		ctx, taskID, "Updated Todos", sessionID,
		string(models.MessageTypeTodo), s.getActiveTurnID(sessionID), metadata, false,
	); err != nil {
		s.logger.Warn("failed to create todo message",
			zap.String("session_id", sessionID),
			zap.Error(err))
	}
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
