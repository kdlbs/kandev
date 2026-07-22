package orchestrator

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/sessionstate"
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
	terminalCompleteStream := false

	if eventType == agentEventComplete {
		if marker, ok := s.terminalExecutionMarker(sessionID, payload.ExecutionID); ok {
			if !marker.allowCompleteStream {
				s.logger.Debug("ignoring complete stream event from terminal failed execution",
					zap.String("task_id", taskID),
					zap.String("session_id", sessionID),
					zap.String("agent_execution_id", payload.ExecutionID))
				return
			}
			terminalCompleteStream = true
		}
	} else if s.shouldDropCompletedExecutionStreamEvent(payload) {
		return
	}

	if !terminalCompleteStream {
		// Any live agent stream activity means the agent resumed after clarification.
		// Cancel primary-path clarification watchdogs for this session. Late terminal
		// completes are excluded because they belong to an already-finished execution.
		s.cancelClarificationWatchdogsForSession(sessionID, eventType)
	}

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

	case streams.EventTypeSessionInfo:
		s.handleSessionInfoEvent(ctx, payload)

	case streams.EventTypeForegroundIdle:
		if !s.foregroundIdleOwnsCurrentPrompt(payload) {
			return
		}
		s.yieldForegroundAndPublish(ctx, taskID, sessionID, foregroundYieldProviderIdle)

	case streams.EventTypeBackgroundComplete:
		if s.completeOneBackgroundTask(sessionID) {
			s.publishForegroundActivityChanged(ctx, taskID, sessionID)
		}

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

func (s *Service) foregroundIdleOwnsCurrentPrompt(payload *lifecycle.AgentStreamEventPayload) bool {
	// Generation zero is the compatibility path for legacy and
	// generation-unaware providers. Those events retain their historical ordered-
	// delivery semantics and cannot be protected from stale cross-prompt delivery;
	// generation-bearing providers fail closed below.
	if payload == nil || payload.Data == nil || payload.Data.PromptGeneration == 0 {
		return true
	}
	generationOwner, ok := s.agentManager.(interface {
		OwnsPromptGeneration(sessionID, executionID string, generation uint64) bool
	})
	if ok && generationOwner.OwnsPromptGeneration(
		payload.SessionID, payload.ExecutionID, payload.Data.PromptGeneration,
	) {
		return true
	}
	s.logger.Debug("ignoring foreground idle for superseded prompt generation",
		zap.String("task_id", payload.TaskID),
		zap.String("session_id", payload.SessionID),
		zap.String("agent_execution_id", payload.ExecutionID),
		zap.Uint64("event_prompt_generation", payload.Data.PromptGeneration))
	return false
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
	s.completeTurnForTaskSession(ctx, taskID, sessionID)
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
	if s.shouldDropCompletedExecutionStreamEvent(payload) {
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
		s.setSessionRunningForExecution(ctx, payload.TaskID, payload.SessionID, payload.ExecutionID)
	}

	// A top-level spawned background task (subagent / run-in-background shell)
	// holds the turn open while the foreground goes idle. Track it so
	// checkSessionPromptable can tell "foreground generating" from "waiting on
	// background". Child tool calls (ParentToolCallID set) are the subagent's
	// internal work, not a new background task, so they are ignored here. A
	// tool_call that already arrives terminal is not outstanding work — clearing
	// is driven by tool_update, so registering it would leak into the hold and
	// never clear.
	if payload.Data.ParentToolCallID == "" && !isTerminalToolStatus(payload.Data.ToolStatus) {
		if normalizedIsBackgroundTask(payload.Data.Normalized) {
			s.registerBackgroundTask(payload.SessionID, payload.Data.ToolCallID)
		} else if s.markForegroundGenerating(payload.SessionID) {
			s.publishForegroundActivityChanged(ctx, payload.TaskID, payload.SessionID)
		}
	}
}

// saveAgentTextIfPresent saves any accumulated agent text as an agent message
// and publishes an AgentTurnMessageSaved event so subscribers (e.g. the office
// comment bridge) can react without a direct dependency.
func (s *Service) saveAgentTextIfPresent(ctx context.Context, payload *lifecycle.AgentStreamEventPayload) {
	if payload.Data.Text == "" || payload.SessionID == "" {
		return
	}
	s.saveAgentTextForTurn(ctx, payload, s.getActiveTurnID(payload.SessionID))
}

func (s *Service) saveAgentTextForTurn(ctx context.Context, payload *lifecycle.AgentStreamEventPayload, turnID string) {
	if payload.Data.Text == "" || payload.SessionID == "" {
		return
	}
	if turnID == "" {
		s.logger.Debug("skipping agent text without a target turn",
			zap.String("task_id", payload.TaskID),
			zap.String("session_id", payload.SessionID),
			zap.String("agent_execution_id", payload.ExecutionID))
		return
	}

	if s.messageCreator != nil {
		if err := s.messageCreator.CreateAgentMessage(ctx, payload.TaskID, payload.Data.Text, payload.SessionID, turnID); err != nil {
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

// publishAgentTurnComplete publishes an event after an agent turn completes.
// The subscriber (office comment bridge) uses the task/session IDs to look up
// the agent's last message and auto-post it as a task comment.
func (s *Service) publishAgentTurnComplete(ctx context.Context, payload *lifecycle.AgentStreamEventPayload) {
	s.publishAgentTurnCompleteForTurn(ctx, payload, "")
}

func (s *Service) publishAgentTurnCompleteForTurn(ctx context.Context, payload *lifecycle.AgentStreamEventPayload, turnID string) {
	if s.eventBus == nil || payload.TaskID == "" || payload.SessionID == "" {
		return
	}

	// Include the text if available (non-streaming agents flush into Data.Text).
	// For streaming agents this will be empty — the subscriber falls back to
	// querying the last session message from the DB.
	data := map[string]string{
		"task_id":    payload.TaskID,
		"session_id": payload.SessionID,
		"agent_text": payload.Data.Text,
		"agent_id":   payload.AgentID,
		"turn_id":    turnID,
	}
	event := bus.NewEvent(events.AgentTurnMessageSaved, "orchestrator", data)
	if err := s.eventBus.Publish(ctx, events.AgentTurnMessageSaved, event); err != nil {
		s.logger.Warn("publish agent_turn_message_saved failed",
			zap.String("task_id", payload.TaskID),
			zap.Error(err))
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
	// Streamed foreground output means the agent is actively generating again,
	// even if a background task is still outstanding — narrows the busy signal.
	// Only genuine output flips the state: an empty/invalid frame is discarded by
	// handleStreamingEventKind, so it must not spuriously reclose the prompt gate.
	if payload.Data.Text != "" && s.markForegroundGenerating(payload.SessionID) {
		s.publishForegroundActivityChanged(ctx, payload.TaskID, payload.SessionID)
	}
	s.handleStreamingEventKind(ctx, payload, "message",
		s.messageCreator.AppendAgentMessage,
		s.messageCreator.CreateAgentMessageStreaming)
}

// handleThinkingStreamingEvent handles streaming thinking events for real-time reasoning updates.
// It creates a new thinking message on first chunk (IsAppend=false) or appends to existing (IsAppend=true).
func (s *Service) handleThinkingStreamingEvent(ctx context.Context, payload *lifecycle.AgentStreamEventPayload) {
	// Streamed foreground reasoning means the agent is actively generating again.
	// Only genuine output flips the state — an empty/invalid frame is discarded
	// downstream, so it must not spuriously reclose the prompt gate.
	if payload.Data.Text != "" && s.markForegroundGenerating(payload.SessionID) {
		s.publishForegroundActivityChanged(ctx, payload.TaskID, payload.SessionID)
	}
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
	if s.shouldDropCompletedExecutionStreamEvent(payload) {
		return
	}

	// Background-work bookkeeping for the finer-grained busy signal runs
	// regardless of message persistence — mirrors handleToolCallEvent — so a
	// run-in-background shell or Monitor watch still drives the prompt gate even
	// when no messageCreator is wired (tests, minimal configs).
	s.trackBackgroundToolUpdate(ctx, payload)

	if s.messageCreator == nil {
		return
	}

	// Determine message type from normalized payload for fallback creation
	msgType := toolKindToMessageType(payload.Data.Normalized)
	status := payload.Data.ToolStatus
	switch status {
	case "running", agentEventComplete, agentEventCompleted, "success", agentEventError, agentEventFailed, "cancelled", "pending", "in_progress":
	default:
		return
	}
	terminal := isTerminalToolStatus(status)
	turnID := ""
	if terminal {
		var err error
		turnID, err = s.peekActiveTurnID(ctx, payload.SessionID)
		if err != nil {
			s.logger.Warn("failed to look up active turn for terminal tool update",
				zap.String("task_id", payload.TaskID),
				zap.String("session_id", payload.SessionID),
				zap.String("tool_call_id", payload.Data.ToolCallID),
				zap.Error(err))
			// Fail closed: without a confirmed active turn, this must be an
			// update-only reconciliation and cannot wake a settled session.
			turnID = ""
		}
	} else {
		turnID = s.getActiveTurnID(payload.SessionID)
	}
	fallbackMsgType := msgType
	if terminal && turnID == "" {
		// A late terminal update can update its existing card, but must not
		// create a message (and implicitly a turn) after the turn settled.
		fallbackMsgType = ""
	}

	if err := s.messageCreator.UpdateToolCallMessage(
		ctx,
		payload.TaskID,
		payload.Data.ToolCallID,
		payload.Data.ParentToolCallID, // Pass parent for subagent nesting
		status,
		"", // result - no longer used, tool results in NormalizedPayload
		payload.SessionID,
		payload.Data.ToolTitle,  // Include title from update event
		turnID,                  // Turn ID for fallback creation
		fallbackMsgType,         // Empty for settled terminal reconciliations
		payload.Data.Normalized, // Pass normalized tool data for message metadata
	); err != nil {
		s.logger.Warn("failed to update tool call message",
			zap.String("task_id", payload.TaskID),
			zap.String("tool_call_id", payload.Data.ToolCallID),
			zap.Error(err))
	}

	// Terminal updates only wake an async turn that was established by prior
	// substantive output. A standalone terminal reconciliation belongs to the
	// already-settled turn that created the tool call.
	if terminal && status != "cancelled" && turnID != "" {
		s.setSessionRunningForExecution(ctx, payload.TaskID, payload.SessionID, payload.ExecutionID)
	}

	// Background-work bookkeeping for the finer-grained busy signal.
	s.trackBackgroundToolUpdate(ctx, payload)
}

// trackBackgroundToolUpdate maintains the fine-grained busy signal's background
// hold from a top-level tool_call_update: a terminal status clears the hold and
// the first recognizable non-terminal frame registers it. Child tool calls
// (ParentToolCallID set) are a subagent's own internal work, not a new
// background task, so they never touch the hold.
func (s *Service) trackBackgroundToolUpdate(ctx context.Context, payload *lifecycle.AgentStreamEventPayload) {
	if payload.Data.ParentToolCallID != "" {
		return
	}
	if isTerminalToolStatus(payload.Data.ToolStatus) {
		// A detached launch card is terminal as a tool invocation, but the
		// launched workload remains active until a provider background-complete
		// signal arrives. Monitor terminal payloads are no longer classified as
		// active, and synchronous subagents do not carry IsAsync.
		if normalizedIsDetachedLaunch(payload.Data.Normalized) {
			return
		}
		// A finished top-level background task no longer holds the turn open.
		// Once none remain, the foreground is no longer "waiting on background".
		// Cleared by tool-call ID membership rather than by re-classifying the
		// terminal payload: adapters that rebuild Normalized per update (or drop
		// the Background flag on the terminal frame) would otherwise never match,
		// leaving the session permanently "not generating" for the rest of the
		// turn. completeBackgroundTask is a no-op for IDs that were never
		// registered, so this cannot clear a still-outstanding background task.
		if s.completeBackgroundTask(payload.SessionID, payload.Data.ToolCallID) {
			s.publishForegroundActivityChanged(ctx, payload.TaskID, payload.SessionID)
		}
		return
	}
	if s.hasBackgroundTask(payload.SessionID, payload.Data.ToolCallID) {
		return
	}
	if !normalizedIsBackgroundTask(payload.Data.Normalized) {
		if s.markForegroundGenerating(payload.SessionID) {
			s.publishForegroundActivityChanged(ctx, payload.TaskID, payload.SessionID)
		}
		return
	}
	// Both of Claude's background shapes only become recognizable on a
	// tool_call_update — the run_in_background flag and command are streamed
	// after the initial (empty) tool_call, and the Monitor view is seeded on its
	// registration update — so a non-terminal update is the first frame where the
	// classifier can see them. Register only on that first recognition:
	// re-registering on later updates would re-set `yielded` and clobber a
	// foreground stream that meanwhile marked the turn generating again.
	s.registerBackgroundTask(payload.SessionID, payload.Data.ToolCallID)
}

// isTerminalToolStatus reports whether a tool_update status marks the tool call
// as finished (successfully, in error, or cancelled).
func isTerminalToolStatus(status string) bool {
	switch status {
	case agentEventComplete, agentEventCompleted, "success", agentEventError, agentEventFailed, "cancelled":
		return true
	default:
		return false
	}
}

func (s *Service) shouldDropCompletedExecutionStreamEvent(payload *lifecycle.AgentStreamEventPayload) bool {
	if payload == nil || payload.ExecutionID == "" || payload.SessionID == "" {
		return false
	}
	if !s.isExecutionCompleted(payload.SessionID, payload.ExecutionID) {
		return false
	}
	s.logger.Debug("ignoring stream event from completed execution",
		zap.String("task_id", payload.TaskID),
		zap.String("session_id", payload.SessionID),
		zap.String("agent_execution_id", payload.ExecutionID))
	return true
}

// updateTaskSessionState transitions a session to nextState with guard checks.
// When a preloadedSession is provided, its State is used for guard conditions (terminal-state
// check, same-state check). This is an optimistic fast-path: between load and check another
// goroutine may have changed the state in the DB. Production repositories use
// an expected-state compare-and-set so a delayed writer cannot revive a terminal session.
// Returns the session row after a successful write (refreshed from DB when possible); callers
// that need authoritative UpdatedAt should use the return value, not the preloaded input.
func (s *Service) updateTaskSessionState(ctx context.Context, taskID, sessionID string, nextState models.TaskSessionState, errorMessage string, allowWakeFromWaiting bool, preloadedSession ...*models.TaskSession) *models.TaskSession {
	updated, _ := s.updateTaskSessionStateWithHook(
		ctx, taskID, sessionID, nextState, errorMessage, allowWakeFromWaiting, nil, preloadedSession...,
	)
	return updated
}

// updateTaskSessionStateWithHook is updateTaskSessionState with an optional
// callback that runs only after the state CAS succeeds and before the state
// change is published. It lets a caller attach state-specific UI metadata
// without creating it when a concurrent terminal transition won the race.
func (s *Service) updateTaskSessionStateWithHook(
	ctx context.Context,
	taskID, sessionID string,
	nextState models.TaskSessionState,
	errorMessage string,
	allowWakeFromWaiting bool,
	onChanged func(),
	preloadedSession ...*models.TaskSession,
) (*models.TaskSession, bool) {
	var session *models.TaskSession
	if len(preloadedSession) > 0 && preloadedSession[0] != nil {
		session = preloadedSession[0]
	} else {
		var err error
		session, err = s.repo.GetTaskSession(ctx, sessionID)
		if err != nil {
			return nil, false
		}
	}
	if session.State == models.TaskSessionStateWaitingForInput && nextState == models.TaskSessionStateRunning && !allowWakeFromWaiting {
		return session, false
	}
	oldState := session.State
	switch session.State {
	case models.TaskSessionStateCompleted, models.TaskSessionStateFailed, models.TaskSessionStateCancelled:
		return session, false
	}
	if session.State == nextState {
		return session, false
	}
	session, authoritativeUpdatedAt, changed := s.persistTaskSessionState(
		ctx, sessionID, session, nextState, errorMessage,
	)
	if !changed {
		return session, false
	}
	if onChanged != nil {
		onChanged()
	}
	if authoritativeUpdatedAt == nil {
		s.logger.Warn("skipping session state_changed publish; could not read authoritative updated_at",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID),
			zap.String("new_state", string(nextState)))
	} else {
		s.logger.Debug("task session state updated",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID),
			zap.String("old_state", string(oldState)),
			zap.String("new_state", string(nextState)))
		s.publishTaskSessionStateChanged(ctx, taskID, sessionID, oldState, nextState, errorMessage, authoritativeUpdatedAt, session)
	}

	// Auto-promote another session to primary when the current primary enters a terminal state
	s.maybePromotePrimary(ctx, taskID, sessionID, nextState)
	return session, true
}

func (s *Service) persistTaskSessionState(
	ctx context.Context,
	sessionID string,
	session *models.TaskSession,
	nextState models.TaskSessionState,
	errorMessage string,
) (*models.TaskSession, *time.Time, bool) {
	if updater, ok := s.repo.(conditionalTaskSessionStateUpdater); ok {
		changed, updatedAt, err := updater.UpdateTaskSessionStateIfCurrent(
			ctx, sessionID, session.State, nextState, errorMessage,
		)
		if err != nil {
			s.logTaskSessionStateWriteError(sessionID, nextState, err)
			return session, nil, false
		}
		if !changed {
			return s.refreshTaskSessionOr(ctx, sessionID, session), nil, false
		}
		persisted := taskSessionAfterStateWrite(session, nextState, errorMessage, updatedAt)
		persisted = s.refreshTaskSessionOr(ctx, sessionID, persisted)
		t := updatedAt.UTC()
		return persisted, &t, true
	}

	if err := s.repo.UpdateTaskSessionState(ctx, sessionID, nextState, errorMessage); err != nil {
		s.logTaskSessionStateWriteError(sessionID, nextState, err)
		return session, nil, false
	}
	refreshed := s.refreshTaskSessionOr(ctx, sessionID, session)
	if refreshed.UpdatedAt.IsZero() {
		return refreshed, nil, true
	}
	t := refreshed.UpdatedAt.UTC()
	return refreshed, &t, true
}

func (s *Service) refreshTaskSessionOr(
	ctx context.Context,
	sessionID string,
	fallback *models.TaskSession,
) *models.TaskSession {
	refreshed, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil || refreshed == nil {
		return fallback
	}
	return refreshed
}

func (s *Service) logTaskSessionStateWriteError(
	sessionID string,
	nextState models.TaskSessionState,
	err error,
) {
	s.logger.Error("failed to update task session state",
		zap.String("session_id", sessionID),
		zap.String("state", string(nextState)),
		zap.Error(err))
}

// transitionTaskSessionState performs the strict state transition used by
// coordinator stop. It always reads current state, surfaces persistence/read
// failures, and publishes the accepted transition before returning.
func (s *Service) transitionTaskSessionState(
	ctx context.Context,
	taskID, sessionID string,
	nextState models.TaskSessionState,
	errorMessage string,
	onChanged func(),
) (bool, models.TaskSessionState, error) {
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return false, "", fmt.Errorf("get session before state transition: %w", err)
	}
	if session == nil {
		return false, "", fmt.Errorf("get session before state transition: session %q is nil", sessionID)
	}
	if isTerminalSessionState(session.State) || session.State == nextState {
		return false, session.State, nil
	}

	oldState := session.State
	changed, refreshed, authoritativeUpdatedAt, err := s.persistStrictTaskSessionState(
		ctx, sessionID, session, nextState, errorMessage,
	)
	if err != nil {
		return false, oldState, err
	}
	if !changed {
		return false, refreshed.State, nil
	}
	if onChanged != nil {
		onChanged()
	}
	s.publishTaskSessionStateChanged(
		ctx,
		taskID,
		sessionID,
		oldState,
		nextState,
		errorMessage,
		authoritativeUpdatedAt,
		refreshed,
	)
	s.maybePromotePrimary(ctx, taskID, sessionID, nextState)
	return true, nextState, nil
}

func (s *Service) persistStrictTaskSessionState(
	ctx context.Context,
	sessionID string,
	session *models.TaskSession,
	nextState models.TaskSessionState,
	errorMessage string,
) (bool, *models.TaskSession, *time.Time, error) {
	if canceller, ok := s.repo.(activeTaskSessionCanceller); ok && nextState == models.TaskSessionStateCancelled {
		return s.cancelActiveTaskSessionState(ctx, canceller, sessionID, session, errorMessage)
	}
	if updater, ok := s.repo.(conditionalTaskSessionStateUpdater); ok {
		return s.persistConditionalTaskSessionState(ctx, updater, sessionID, session, nextState, errorMessage)
	}
	if err := s.repo.UpdateTaskSessionState(ctx, sessionID, nextState, errorMessage); err != nil {
		return false, session, nil, err
	}
	refreshed, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return false, session, nil, fmt.Errorf("get session after state transition: %w", err)
	}
	if refreshed == nil {
		return false, session, nil, fmt.Errorf("get session after state transition: session %q is nil", sessionID)
	}
	if refreshed.State != nextState {
		return false, refreshed, nil, nil
	}
	var authoritativeUpdatedAt *time.Time
	if !refreshed.UpdatedAt.IsZero() {
		t := refreshed.UpdatedAt.UTC()
		authoritativeUpdatedAt = &t
	}
	return true, refreshed, authoritativeUpdatedAt, nil
}

func (s *Service) persistConditionalTaskSessionState(
	ctx context.Context,
	updater conditionalTaskSessionStateUpdater,
	sessionID string,
	session *models.TaskSession,
	nextState models.TaskSessionState,
	errorMessage string,
) (bool, *models.TaskSession, *time.Time, error) {
	changed, updatedAt, err := updater.UpdateTaskSessionStateIfCurrent(
		ctx,
		sessionID,
		session.State,
		nextState,
		errorMessage,
	)
	if err != nil {
		return false, session, nil, err
	}
	if !changed {
		refreshed, readErr := s.repo.GetTaskSession(ctx, sessionID)
		if readErr != nil {
			return false, session, nil, fmt.Errorf("get session after rejected state transition: %w", readErr)
		}
		if refreshed == nil {
			return false, session, nil, fmt.Errorf("get session after rejected state transition: session %q is nil", sessionID)
		}
		return false, refreshed, nil, nil
	}
	persisted := taskSessionAfterStateWrite(session, nextState, errorMessage, updatedAt)
	persisted = s.refreshTaskSessionOr(ctx, sessionID, persisted)
	t := updatedAt.UTC()
	return true, persisted, &t, nil
}

func (s *Service) cancelActiveTaskSessionState(
	ctx context.Context,
	canceller activeTaskSessionCanceller,
	sessionID string,
	session *models.TaskSession,
	errorMessage string,
) (bool, *models.TaskSession, *time.Time, error) {
	changed, updatedAt, err := canceller.CancelActiveTaskSession(ctx, sessionID, errorMessage)
	if err != nil {
		return false, session, nil, err
	}
	if !changed {
		refreshed, readErr := s.repo.GetTaskSession(ctx, sessionID)
		if readErr != nil {
			return false, session, nil, fmt.Errorf("get session after rejected cancellation: %w", readErr)
		}
		if refreshed == nil {
			return false, session, nil, fmt.Errorf("get session after rejected cancellation: session %q is nil", sessionID)
		}
		return false, refreshed, nil, nil
	}
	refreshed := taskSessionAfterStateWrite(session, models.TaskSessionStateCancelled, errorMessage, updatedAt)
	refreshed = s.refreshTaskSessionOr(ctx, sessionID, refreshed)
	t := updatedAt.UTC()
	return true, refreshed, &t, nil
}

type activeTaskSessionCanceller interface {
	CancelActiveTaskSession(ctx context.Context, sessionID, reason string) (bool, time.Time, error)
}

type conditionalTaskSessionStateUpdater interface {
	UpdateTaskSessionStateIfCurrent(
		ctx context.Context,
		sessionID string,
		expected, next models.TaskSessionState,
		errorMessage string,
	) (bool, time.Time, error)
}

func taskSessionAfterStateWrite(
	session *models.TaskSession,
	nextState models.TaskSessionState,
	errorMessage string,
	updatedAt time.Time,
) *models.TaskSession {
	updated := *session
	updated.State = nextState
	updated.ErrorMessage = errorMessage
	updated.UpdatedAt = updatedAt
	if isTerminalSessionState(nextState) {
		t := updatedAt
		updated.CompletedAt = &t
	} else {
		updated.CompletedAt = nil
	}
	return &updated
}

func (s *Service) publishTaskSessionStateChanged(
	ctx context.Context,
	taskID, sessionID string,
	oldState, nextState models.TaskSessionState,
	errorMessage string,
	stateUpdatedAt *time.Time,
	session *models.TaskSession,
) {
	if s.eventBus == nil || session == nil {
		return
	}
	agentProfileID := session.AgentProfileID
	if agentProfileID == "" {
		if task, terr := s.repo.GetTask(ctx, taskID); terr == nil && task != nil {
			agentProfileID = task.AssigneeAgentProfileID
		}
	}
	eventData := map[string]interface{}{
		metaKeyTaskID:            taskID,
		metaKeySessionID:         sessionID,
		"old_state":              string(oldState),
		metaKeyNewState:          string(nextState),
		"error_message":          errorMessage,
		metaKeyAgentProfileID:    agentProfileID,
		"agent_profile_snapshot": session.AgentProfileSnapshot,
		"is_passthrough":         session.IsPassthrough,
		// Carry the fine-grained busy substate on every coarse transition. A new
		// foreground turn resets it to generating; a turn ending may preserve
		// background while detached work remains live (ADR-0049).
		"foreground_activity": string(s.foregroundActivityValue(sessionID)),
	}
	if stateUpdatedAt != nil && !stateUpdatedAt.IsZero() {
		eventData[metaKeyUpdatedAt] = stateUpdatedAt.Format(time.RFC3339Nano)
	}
	if session.ReviewStatus != models.ReviewStatusNone {
		eventData["review_status"] = string(session.ReviewStatus)
	}
	// Always included (even when empty) so a rename-to-clear propagates;
	// the frontend only applies the key when present.
	eventData["name"] = session.Name
	if len(session.Metadata) > 0 {
		eventData["session_metadata"] = session.Metadata
	}
	if suppressed, ok := s.suppressToast.LoadAndDelete(sessionID); ok && suppressed.(bool) {
		eventData["suppress_toast"] = true
	}
	if session.TaskEnvironmentID != "" {
		eventData["task_environment_id"] = session.TaskEnvironmentID
	}
	_ = s.eventBus.Publish(ctx, events.TaskSessionStateChanged, bus.NewEvent(events.TaskSessionStateChanged, "task-session", eventData))
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

const completedExecutionRetention = 10 * time.Minute

type terminalExecutionMarker struct {
	expiresAt           time.Time
	allowCompleteStream bool
	turnID              string
}

func terminalExecutionKey(sessionID, executionID string) string {
	return sessionID + "\x00" + executionID
}

func (s *Service) markExecutionCompleted(sessionID, executionID string) {
	s.markTerminalExecution(sessionID, executionID, true)
}

func (s *Service) markExecutionFailed(sessionID, executionID string) {
	s.markTerminalExecution(sessionID, executionID, false)
}

func (s *Service) markTerminalExecution(sessionID, executionID string, allowCompleteStream bool) {
	if sessionID == "" || executionID == "" {
		return
	}
	key := terminalExecutionKey(sessionID, executionID)
	expiresAt := time.Now().Add(completedExecutionRetention)
	s.completedExecutions.Store(key, terminalExecutionMarker{
		expiresAt:           expiresAt,
		allowCompleteStream: allowCompleteStream,
		turnID:              s.currentTurnIDForSession(context.Background(), sessionID),
	})
	time.AfterFunc(completedExecutionRetention, func() {
		s.deleteCompletedExecutionIfExpired(key, expiresAt)
	})
}

func (s *Service) isExecutionCompleted(sessionID, executionID string) bool {
	_, ok := s.terminalExecutionMarker(sessionID, executionID)
	return ok
}

func (s *Service) terminalCompleteStreamMarker(sessionID, executionID string) (terminalExecutionMarker, bool) {
	marker, ok := s.terminalExecutionMarker(sessionID, executionID)
	return marker, ok && marker.allowCompleteStream
}

func (s *Service) terminalExecutionMarker(sessionID, executionID string) (terminalExecutionMarker, bool) {
	if sessionID == "" || executionID == "" {
		return terminalExecutionMarker{}, false
	}
	key := terminalExecutionKey(sessionID, executionID)
	value, ok := s.completedExecutions.Load(key)
	if !ok {
		return terminalExecutionMarker{}, false
	}
	marker, ok := value.(terminalExecutionMarker)
	if !ok || time.Now().After(marker.expiresAt) {
		s.deleteCompletedExecutionIfExpired(key, marker.expiresAt)
		return terminalExecutionMarker{}, false
	}
	return marker, true
}

func (s *Service) currentTurnIDForSession(ctx context.Context, sessionID string) string {
	if sessionID == "" {
		return ""
	}
	if turnIDVal, ok := s.activeTurns.Load(sessionID); ok {
		if turnID, ok := turnIDVal.(string); ok && turnID != "" {
			return turnID
		}
	}
	if s.turnService == nil {
		return ""
	}
	turn, err := s.turnService.GetActiveTurn(ctx, sessionID)
	if err != nil || turn == nil {
		return ""
	}
	return turn.ID
}

func (s *Service) deleteCompletedExecutionIfExpired(key string, expiresAt time.Time) {
	value, ok := s.completedExecutions.Load(key)
	if !ok {
		return
	}
	current, ok := value.(terminalExecutionMarker)
	if !ok || !current.expiresAt.After(expiresAt) {
		s.completedExecutions.Delete(key)
	}
}

func (s *Service) setSessionStarting(ctx context.Context, taskID string, session *models.TaskSession, promoteTask bool) error {
	if session == nil {
		return nil
	}

	var publishSession *models.TaskSession
	var stateUpdatedAt *time.Time
	var oldState models.TaskSessionState
	if err := func() error {
		s.taskRuntimeStateMu.Lock()
		defer s.taskRuntimeStateMu.Unlock()

		current, err := s.repo.GetTaskSession(ctx, session.ID)
		if err != nil {
			return err
		}
		allowedTerminalRecovery := !promoteTask &&
			session.State == models.TaskSessionStateStarting &&
			(current.State == models.TaskSessionStateFailed ||
				(current.State == models.TaskSessionStateCancelled &&
					models.IsArchiveCancelReason(current.ErrorMessage)))
		if isTerminalSessionState(current.State) && !allowedTerminalRecovery {
			return &executor.SessionStateSupersededError{SessionID: session.ID, State: current.State}
		}

		oldState = current.State
		if err := s.persistFullTaskSessionIfCurrent(ctx, session, current.State); err != nil {
			return err
		}

		if oldState != session.State {
			if refreshed, err := s.repo.GetTaskSession(ctx, session.ID); err == nil && refreshed != nil {
				if !refreshed.UpdatedAt.IsZero() {
					t := refreshed.UpdatedAt.UTC()
					stateUpdatedAt = &t
				}
				publishSession = refreshed
			}
		}

		return nil
	}(); err != nil {
		return err
	}
	if promoteTask {
		s.writeTaskInProgressForRuntime(ctx, taskID, session.ID)
	}

	if publishSession != nil {
		s.publishTaskSessionStateChanged(ctx, taskID, session.ID, oldState, session.State, session.ErrorMessage, stateUpdatedAt, publishSession)
	}
	return nil
}

func (s *Service) persistFullTaskSessionIfCurrent(
	ctx context.Context,
	session *models.TaskSession,
	expected models.TaskSessionState,
) error {
	changed, err := s.repo.UpdateTaskSessionIfCurrentState(ctx, session, expected)
	if err != nil {
		return err
	}
	if changed {
		return nil
	}
	latest, err := s.repo.GetTaskSession(ctx, session.ID)
	if err != nil {
		return err
	}
	if latest == nil {
		return fmt.Errorf("%w: agent session not found: %s", models.ErrTaskSessionNotFound, session.ID)
	}
	if isTerminalSessionState(latest.State) {
		return &executor.SessionStateSupersededError{SessionID: session.ID, State: latest.State}
	}
	return fmt.Errorf(
		"session %s state changed from %s to %s before full-row persistence",
		session.ID,
		expected,
		latest.State,
	)
}

func (s *Service) setSessionWaitingForInput(ctx context.Context, taskID, sessionID string, preloadedSession ...*models.TaskSession) {
	// Resolve session up front so we can skip the redundant task-state write
	// when the session was already WAITING_FOR_INPUT. Without this guard, every
	// caller (workflow on_turn_complete + handleCompleteStreamEvent + other
	// terminal paths) writes tasks.state=REVIEW on every turn even though the
	// state hasn't changed, producing duplicate "task moved to REVIEW" logs and
	// unnecessary DB churn.
	var session *models.TaskSession
	if len(preloadedSession) > 0 && preloadedSession[0] != nil {
		session = preloadedSession[0]
	} else {
		var err error
		session, err = s.repo.GetTaskSession(ctx, sessionID)
		if err != nil {
			// Fall back to legacy behavior — still attempt the task-state
			// write so a transient lookup failure doesn't drop a needed
			// REVIEW transition.
			s.updateTaskSessionState(ctx, taskID, sessionID, models.TaskSessionStateWaitingForInput, "", false)
			s.writeTaskReviewState(ctx, taskID, sessionID)
			return
		}
	}

	wasAlreadyWaiting := session.State == models.TaskSessionStateWaitingForInput
	if updatedSession := s.updateTaskSessionState(ctx, taskID, sessionID, models.TaskSessionStateWaitingForInput, "", false, session); updatedSession != nil {
		if len(preloadedSession) > 0 && preloadedSession[0] != nil && preloadedSession[0] != updatedSession {
			*preloadedSession[0] = *updatedSession
		}
	}

	if wasAlreadyWaiting {
		return
	}

	s.writeTaskReviewState(ctx, taskID, sessionID)
}

// taskArchived reports whether a task row has been archived. Runtime-state
// writes (IN_PROGRESS on session start, REVIEW on turn completion/cancel/
// startup reconciliation) must never resurrect an archived task's state —
// once archived_at is set the row is frozen from the kanban's perspective,
// so every write site here has to skip it instead of reviving stale runtime
// state that raced the archive.
func taskArchived(task *models.Task) bool {
	return task != nil && task.ArchivedAt != nil
}

func (s *Service) writeTaskReviewState(ctx context.Context, taskID, completedSessionID string) {
	// Task lookup errors fail closed so office/archived guards cannot be bypassed
	// by a transient repository failure.
	if dbTask, err := s.repo.GetTask(ctx, taskID); err != nil {
		s.logger.Warn("failed to load task before REVIEW state reconcile",
			zap.String("task_id", taskID),
			zap.Error(err))
		return
	} else if dbTask != nil && dbTask.IsFromOffice {
		s.logger.Debug("skipping REVIEW transition for office task",
			zap.String("task_id", taskID))
		return
	} else if taskArchived(dbTask) {
		s.logger.Debug("skipping REVIEW transition for archived task",
			zap.String("task_id", taskID))
		return
	}

	s.taskRuntimeStateMu.Lock()
	defer s.taskRuntimeStateMu.Unlock()

	if completedSessionID != "" {
		if session, err := s.repo.GetTaskSession(ctx, completedSessionID); err == nil && session != nil && isWorkingSessionState(session.State) {
			s.logger.Debug("skipping task REVIEW state because completed session is active again",
				zap.String("task_id", taskID),
				zap.String("session_id", completedSessionID),
				zap.String("session_state", string(session.State)))
			return
		}
	}

	if blockingSessionID, ok := s.otherWorkingSessionID(ctx, taskID, completedSessionID); !ok {
		return
	} else if blockingSessionID != "" {
		s.logger.Debug("skipping task REVIEW state while another session is working",
			zap.String("task_id", taskID),
			zap.String("completed_session_id", completedSessionID),
			zap.String("blocking_session_id", blockingSessionID))
		return
	}
	updated, err := s.taskRepo.UpdateTaskStateIfCurrentIn(
		ctx,
		taskID,
		v1.TaskStateReview,
		[]v1.TaskState{v1.TaskStateInProgress, v1.TaskStateScheduling},
	)
	if err != nil {
		s.logger.Error("failed to update task state to REVIEW",
			zap.String("task_id", taskID),
			zap.Error(err))
		return
	}
	if !updated {
		return
	}
	s.logger.Info("task moved to REVIEW state",
		zap.String("task_id", taskID))
}

func isWorkingSessionState(state models.TaskSessionState) bool {
	return sessionstate.IsWorking(state)
}

func (s *Service) otherWorkingSessionID(ctx context.Context, taskID, currentSessionID string) (string, bool) {
	sessions, err := s.repo.ListTaskSessions(ctx, taskID)
	if err != nil {
		s.logger.Warn("failed to list task sessions before REVIEW state reconcile",
			zap.String("task_id", taskID),
			zap.String("session_id", currentSessionID),
			zap.Error(err))
		return "", false
	}
	for _, session := range sessions {
		if session == nil {
			continue
		}
		if currentSessionID != "" && session.ID == currentSessionID {
			continue
		}
		if isWorkingSessionState(session.State) {
			return session.ID, true
		}
	}
	return "", true
}

// writeTaskReviewStateOnCancel clears the kanban "actively working" task
// states when the user cancels a turn mid-flight by landing the task in
// REVIEW — the same bucket a normal turn completion uses, so the sidebar
// shows the green check rather than the yellow "needs input" question icon.
// Office task status reflects workflow position, not runtime cancel, so those
// tasks are left alone. Only actively-working tasks are reconciled; tasks
// already past IN_PROGRESS / SCHEDULING keep their state.
func (s *Service) writeTaskReviewStateOnCancel(ctx context.Context, taskID, sessionID string) {
	dbTask, err := s.repo.GetTask(ctx, taskID)
	if err != nil || dbTask == nil {
		if err != nil {
			s.logger.Warn("failed to load task for cancel state reconcile",
				zap.String("task_id", taskID),
				zap.Error(err))
		}
		return
	}
	if dbTask.IsFromOffice {
		return
	}
	if taskArchived(dbTask) {
		s.logger.Debug("skipping REVIEW transition after cancel for archived task",
			zap.String("task_id", taskID))
		return
	}

	s.taskRuntimeStateMu.Lock()
	defer s.taskRuntimeStateMu.Unlock()

	if sessionID != "" {
		if session, err := s.repo.GetTaskSession(ctx, sessionID); err == nil && session != nil && isWorkingSessionState(session.State) {
			s.logger.Debug("skipping task REVIEW state after cancel because session is active again",
				zap.String("task_id", taskID),
				zap.String("session_id", sessionID),
				zap.String("session_state", string(session.State)))
			return
		}
	}

	if blockingSessionID, ok := s.otherWorkingSessionID(ctx, taskID, sessionID); !ok {
		return
	} else if blockingSessionID != "" {
		s.logger.Debug("skipping task REVIEW state after cancel while another session is working",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID),
			zap.String("blocking_session_id", blockingSessionID))
		return
	}

	updated, err := s.taskRepo.UpdateTaskStateIfCurrentIn(
		ctx,
		taskID,
		v1.TaskStateReview,
		[]v1.TaskState{v1.TaskStateInProgress, v1.TaskStateScheduling},
	)
	if err != nil {
		s.logger.Error("failed to update task state to REVIEW",
			zap.String("task_id", taskID),
			zap.Error(err))
		return
	}
	if !updated {
		return
	}
	s.logger.Info("task moved to REVIEW state after turn cancel",
		zap.String("task_id", taskID))
}

func (s *Service) setSessionRunning(ctx context.Context, taskID, sessionID string, preloadedSession ...*models.TaskSession) {
	s.setSessionRunningForExecution(ctx, taskID, sessionID, "", preloadedSession...)
}

func (s *Service) setSessionRunningForExecution(ctx context.Context, taskID, sessionID, executionID string, preloadedSession ...*models.TaskSession) {
	s.taskRuntimeStateMu.Lock()
	defer s.taskRuntimeStateMu.Unlock()

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
	if session.State == models.TaskSessionStateWaitingForInput && s.isExecutionCompleted(sessionID, executionID) {
		s.logger.Debug("ignoring stream event for completed execution",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID),
			zap.String("agent_execution_id", executionID))
		return
	}

	// Skip the redundant task-state write when the session is already RUNNING.
	// Tool calls fire many events per turn and each was triggering an
	// UpdateTaskState(IN_PROGRESS) write that produced no actual state change
	// (2,000+ redundant writes observed on long-running turns).
	wasAlreadyRunning := session.State == models.TaskSessionStateRunning

	if updatedSession := s.updateTaskSessionState(ctx, taskID, sessionID, models.TaskSessionStateRunning, "", true, session); updatedSession != nil {
		if len(preloadedSession) > 0 && preloadedSession[0] != nil && preloadedSession[0] != updatedSession {
			*preloadedSession[0] = *updatedSession
		}
	}

	if wasAlreadyRunning {
		return
	}

	if err := s.reconcileTaskStateForRuntimeLocked(
		ctx,
		taskID,
		sessionID,
		v1.TaskStateInProgress,
	); err != nil {
		s.logger.Error("failed to update task state to IN_PROGRESS",
			zap.String("task_id", taskID),
			zap.Error(err))
	}
}

func (s *Service) writeTaskInProgressForRuntime(ctx context.Context, taskID, sessionID string) {
	err := s.reconcileTaskStateForRuntime(ctx, taskID, sessionID, v1.TaskStateInProgress)
	if err != nil {
		s.logger.Error("failed to update task state to IN_PROGRESS",
			zap.String("task_id", taskID),
			zap.Error(err))
	}
}

func (s *Service) reconcileTaskStateForRuntime(
	ctx context.Context,
	taskID, sessionID string,
	state v1.TaskState,
) error {
	s.taskRuntimeStateMu.Lock()
	defer s.taskRuntimeStateMu.Unlock()
	return s.reconcileTaskStateForRuntimeLocked(ctx, taskID, sessionID, state)
}

func (s *Service) reconcileTaskStateForRuntimeLocked(
	ctx context.Context,
	taskID, sessionID string,
	state v1.TaskState,
) error {
	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil {
		return err
	}
	if taskArchived(task) {
		return nil
	}
	if state == v1.TaskStateInProgress && task != nil && task.IsFromOffice {
		return nil
	}
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return err
	}
	if !runtimeSessionOwnsTaskState(session, state) {
		return nil
	}
	updated, err := s.taskRepo.UpdateTaskStateIfSessionState(ctx, taskID, sessionID, session.State, state)
	if err != nil {
		return err
	}
	if updated {
		s.logger.Info("task state reconciled from active runtime",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID),
			zap.String("state", string(state)))
	}
	return nil
}

func runtimeSessionOwnsTaskState(session *models.TaskSession, state v1.TaskState) bool {
	if session == nil {
		return false
	}
	switch state {
	case v1.TaskStateInProgress:
		return sessionstate.IsWorking(session.State)
	case v1.TaskStateFailed:
		return session.State == models.TaskSessionStateFailed
	default:
		return false
	}
}

// handleCompleteStreamEvent handles the agentEventComplete stream event.
func (s *Service) handleCompleteStreamEvent(ctx context.Context, payload *lifecycle.AgentStreamEventPayload) {
	s.logger.Debug("handling complete stream event",
		zap.String("task_id", payload.TaskID),
		zap.String("session_id", payload.SessionID))
	terminalMarker, terminalCompleteStream := s.terminalCompleteStreamMarker(payload.SessionID, payload.ExecutionID)

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

	s.publishPromptUsage(ctx, payload, session)

	if terminalCompleteStream {
		s.saveAgentTextForTurn(ctx, payload, terminalMarker.turnID)
		s.publishAgentPlanForTurn(ctx, payload, terminalMarker.turnID, false)
		s.persistTurnPromptMetadataForTurn(ctx, payload, session, terminalMarker.turnID)
		if terminalMarker.turnID != "" {
			s.publishAgentTurnCompleteForTurn(ctx, payload, terminalMarker.turnID)
		}
		s.detachClarificationWaiters(ctx, payload.SessionID)
		s.logger.Debug("complete stream from terminal execution flushed final data; skipping active turn and runtime reconciliation",
			zap.String("task_id", payload.TaskID),
			zap.String("session_id", payload.SessionID),
			zap.String("agent_execution_id", payload.ExecutionID),
			zap.String("turn_id", terminalMarker.turnID))
		return
	}

	s.saveAgentTextIfPresent(ctx, payload)
	s.publishAgentPlanIfPresent(ctx, payload)
	s.persistTurnPromptMetadata(ctx, payload, session)
	s.completeTurnForTaskSession(ctx, payload.TaskID, payload.SessionID)

	// Publish agent turn message event so the office comment bridge can
	// auto-post the agent's response as a task comment. Published here
	// (not in saveAgentTextIfPresent) because for streaming agents the
	// text is drained by message_chunk events and Data.Text is empty at
	// complete time.
	s.publishAgentTurnComplete(ctx, payload)

	// Detach any pending clarifications so WaitForResponse unblocks while the
	// overlay stays interactive for a deferred answer via the event fallback path.
	s.detachClarificationWaiters(ctx, payload.SessionID)

	// Capture a fresh git status snapshot on every turn completion so the sidebar
	// diff badge stays current even when the agent remains running (the
	// agent_completed path only fires on process exit). This also makes the
	// badge resilient to backend restarts that kill the agent process before
	// it can publish a completion event.
	//
	// We must use captureGitStatusSnapshotFresh (not the cached version) because
	// the cached workspace tracker status may predate the agent's last commit.
	// After turn completion the poll mode can drop to slow (30s) if the user
	// navigates away, so the cached value could stay stale for a long time.
	//
	// This runs BEFORE the RUNNING-state guard so it fires regardless of which
	// event (READY vs COMPLETE) will drive the session state transition.
	//
	// Capture synchronously so the snapshot is persisted before the handler
	// returns. Running async risks the backend being killed (e.g. E2E restart)
	// before the snapshot is written. Retries handle transient git lock
	// contention between concurrent worktrees.
	if payload.SessionID != "" {
		s.captureGitStatusSnapshotWithRetry(ctx, payload.SessionID)
	}

	// Office sessions park at IDLE between scheduler runs; cancelled turns skip that path so the session stays promptable.
	stopReason := extractStopReason(payload)
	if session != nil && s.handleOfficeTurnComplete(ctx, payload.TaskID, payload.SessionID, session, stopReason) {
		return
	}
	if session != nil && s.handleAutomationTurnComplete(
		ctx,
		payload.TaskID,
		payload.SessionID,
		session,
		stopReason,
		extractCompleteIsError(payload),
		extractCompleteErrorMessage(payload),
	) {
		return
	}

	// READY events own workflow transitions and queued prompt execution.
	// If we're still RUNNING here, avoid racing READY by forcing WAITING/REVIEW.
	if session != nil && session.State == models.TaskSessionStateRunning {
		// Deferring the running→waiting transition to a READY event. If no READY
		// follows, the session stays RUNNING and the chat UI keeps showing the
		// agent as working even though the turn already completed. This is the
		// backend half of the frontend [session:state] trace — filter both by the
		// same task_id to see whether a clear ever lands.
		s.logger.Debug("complete-event deferring running->waiting to READY (turn done, state not yet cleared)",
			zap.String("task_id", payload.TaskID),
			zap.String("session_id", payload.SessionID))
		return
	}

	// Positive path: this complete event owns the running→WAITING_FOR_INPUT
	// transition (no READY race). Pairs with the frontend [session:state] line
	// under the same task_id.
	s.logger.Debug("complete-event clearing running->waiting (this event owns the transition)",
		zap.String("task_id", payload.TaskID),
		zap.String("session_id", payload.SessionID),
		zap.String("prev_state", sessionStateString(session)))
	s.setSessionWaitingForInput(ctx, payload.TaskID, payload.SessionID, session)
}

func (s *Service) detachClarificationWaiters(ctx context.Context, sessionID string) {
	if s.clarificationCanceller == nil || sessionID == "" {
		return
	}
	if n := s.clarificationCanceller.DetachSessionAndNotify(ctx, sessionID); n > 0 {
		s.logger.Info("detached pending clarifications on turn complete",
			zap.String("session_id", sessionID),
			zap.Int("count", n))
	}
}

// sessionStateString renders a session's state for logging, returning "" when
// the session is nil (e.g. a complete event with no session ID). Kept tiny so
// state-transition trace logs don't add branching to their hot-path callers.
func sessionStateString(session *models.TaskSession) string {
	if session == nil {
		return ""
	}
	return string(session.State)
}

// Mirrors the same read in lifecycle/manager_events.go.
func extractStopReason(payload *lifecycle.AgentStreamEventPayload) string {
	if payload == nil || payload.Data == nil {
		return ""
	}
	data, ok := payload.Data.Data.(map[string]interface{})
	if !ok {
		return ""
	}
	sr, _ := data["stop_reason"].(string)
	return sr
}

func extractCompleteIsError(payload *lifecycle.AgentStreamEventPayload) bool {
	if payload == nil || payload.Data == nil {
		return false
	}
	data, ok := payload.Data.Data.(map[string]interface{})
	if !ok {
		return false
	}
	isError, _ := data["is_error"].(bool)
	return isError
}

func extractCompleteErrorMessage(payload *lifecycle.AgentStreamEventPayload) string {
	if payload == nil || payload.Data == nil {
		return ""
	}
	if payload.Data.Error != "" {
		return payload.Data.Error
	}
	// Complete-event text is user-facing agent output; only structured error
	// fields should become AutomationRun.error_message.
	data, ok := payload.Data.Data.(map[string]interface{})
	if !ok {
		return ""
	}
	if message, ok := data["error"].(string); ok {
		return message
	}
	if message, ok := data["message"].(string); ok {
		return message
	}
	return ""
}

// Mirrors the "cancelled" literal in lifecycle/manager_events.go — not extracted to avoid cross-package coupling.
const stopReasonCancelled = "cancelled"

// Returns true when handled as office (state→IDLE + StopAgent); stopReason "cancelled" returns false to keep the session promptable.
func (s *Service) handleOfficeTurnComplete(
	ctx context.Context, taskID, sessionID string, session *models.TaskSession, stopReason string,
) bool {
	if session == nil || session.AgentProfileID == "" {
		return false
	}
	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil || task == nil || !task.IsFromOffice {
		return false
	}

	if stopReason == stopReasonCancelled {
		s.logger.Info("office turn cancelled by user — skipping IDLE flip, deferring to cancel handler",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID),
			zap.String("stop_reason", stopReason))
		return false
	}

	s.logger.Info("office turn complete — IDLE + tearing down execution",
		zap.String("task_id", taskID),
		zap.String("session_id", sessionID),
		zap.String("agent_profile_id", session.AgentProfileID))

	// State flip first so the workflow handler doesn't ping-pong on the
	// AgentStopped → handleAgentCompleted path.
	s.updateTaskSessionState(ctx, taskID, sessionID, models.TaskSessionStateIdle, "", false, session)

	if s.agentManager != nil && session.AgentExecutionID != "" {
		// Tears down the agent subprocess + executor backend + agentctl
		// connection. The session's acp_session_id stays on the row for the
		// next session/load on the next run.
		if err := s.agentManager.StopAgent(ctx, session.AgentExecutionID, false); err != nil {
			s.logger.Warn("failed to stop office agent on turn complete",
				zap.String("session_id", sessionID),
				zap.String("agent_execution_id", session.AgentExecutionID),
				zap.Error(err))
		}
	}
	return true
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
	s.publishAgentPlanForTurn(ctx, payload, "", true)
}

func (s *Service) publishAgentPlanForTurn(ctx context.Context, payload *lifecycle.AgentStreamEventPayload, turnID string, allowLazyTurn bool) {
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
	if turnID == "" && allowLazyTurn {
		turnID = s.getActiveTurnID(sessionID)
	}
	if turnID == "" {
		return
	}
	if err := s.messageCreator.CreateSessionMessage(
		ctx, payload.TaskID, planContent, sessionID,
		string(models.MessageTypeAgentPlan), turnID, nil, false,
	); err != nil {
		s.logger.Error("failed to create agent plan message",
			zap.String("task_id", payload.TaskID),
			zap.String("session_id", sessionID),
			zap.Error(err))
	}
}

// publishPromptUsage broadcasts prompt token usage to the WebSocket for the frontend.
// Model and agent type (CLI engine slug) come from payload first; when absent
// (which is the common case — CurrentModelID only travels on session_models
// frames) we fall back to the session's AgentProfileSnapshot, populated at
// session creation and refreshed by persistSessionModel on ACP model updates.
func (s *Service) publishPromptUsage(
	ctx context.Context,
	payload *lifecycle.AgentStreamEventPayload,
	session *models.TaskSession,
) {
	sessionID := payload.SessionID
	if sessionID == "" || s.eventBus == nil || payload.Data.Usage == nil {
		return
	}

	model, agentType := resolvePromptUsageLabels(payload, session)

	eventPayload := lifecycle.SessionPromptUsageEventPayload{
		TaskID:    payload.TaskID,
		SessionID: sessionID,
		AgentID:   payload.AgentID,
		AgentType: agentType,
		Model:     model,
		Usage:     payload.Data.Usage,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
	subject := events.BuildSessionPromptUsageSubject(sessionID)
	_ = s.eventBus.Publish(ctx, subject, bus.NewEvent(events.SessionPromptUsageUpdated, "orchestrator", eventPayload))
}

func resolvePromptUsageLabels(
	payload *lifecycle.AgentStreamEventPayload,
	session *models.TaskSession,
) (string, string) {
	model := ""
	if payload != nil && payload.Data != nil {
		model = payload.Data.CurrentModelID
	}
	agentType := ""
	if session != nil && session.AgentProfileSnapshot != nil {
		if model == "" {
			if m, ok := session.AgentProfileSnapshot["model"].(string); ok {
				model = m
			}
		}
		if t, ok := session.AgentProfileSnapshot["agent_name"].(string); ok {
			agentType = t
		}
	}
	return model, agentType
}

func (s *Service) persistTurnPromptMetadata(
	ctx context.Context,
	payload *lifecycle.AgentStreamEventPayload,
	session *models.TaskSession,
) {
	if payload == nil || payload.Data == nil || payload.SessionID == "" || payload.Data.Usage == nil || s.turnService == nil {
		return
	}
	turn, err := s.turnService.GetActiveTurn(ctx, payload.SessionID)
	if err != nil {
		s.logger.Warn("failed to get active turn for prompt usage metadata",
			zap.String("session_id", payload.SessionID),
			zap.Error(err))
		return
	}
	if turn == nil {
		return
	}
	s.persistPromptMetadataOnTurn(ctx, payload, session, turn)
}

func (s *Service) persistTurnPromptMetadataForTurn(
	ctx context.Context,
	payload *lifecycle.AgentStreamEventPayload,
	session *models.TaskSession,
	turnID string,
) {
	if payload == nil || payload.Data == nil || payload.SessionID == "" || payload.Data.Usage == nil || s.turnService == nil || turnID == "" {
		return
	}
	turn, err := s.turnService.GetTurn(ctx, turnID)
	if err != nil {
		s.logger.Warn("failed to get terminal turn for prompt usage metadata",
			zap.String("turn_id", turnID),
			zap.String("session_id", payload.SessionID),
			zap.Error(err))
		return
	}
	if turn == nil {
		return
	}
	s.persistPromptMetadataOnTurn(ctx, payload, session, turn)
}

func (s *Service) persistPromptMetadataOnTurn(
	ctx context.Context,
	payload *lifecycle.AgentStreamEventPayload,
	session *models.TaskSession,
	turn *models.Turn,
) {
	model, agentType := resolvePromptUsageLabels(payload, session)
	metadata := turn.Metadata
	if metadata == nil {
		metadata = make(map[string]interface{})
	}
	metadata["prompt_usage"] = promptUsageMetadata(payload.Data.Usage)
	if model != "" {
		metadata["model"] = model
	}
	if agentType != "" {
		metadata["agent_type"] = agentType
	}
	if payload.AgentID != "" {
		metadata["agent_id"] = payload.AgentID
	}
	turn.Metadata = metadata
	if err := s.turnService.UpdateTurn(ctx, turn); err != nil {
		s.logger.Warn("failed to persist prompt usage metadata on turn",
			zap.String("turn_id", turn.ID),
			zap.String("session_id", payload.SessionID),
			zap.Error(err))
	}
}

func promptUsageMetadata(usage *streams.PromptUsage) map[string]interface{} {
	if usage == nil {
		return nil
	}
	return map[string]interface{}{
		"input_tokens":                    usage.InputTokens,
		"output_tokens":                   usage.OutputTokens,
		"cached_read_tokens":              usage.CachedReadTokens,
		"cached_write_tokens":             usage.CachedWriteTokens,
		"thought_tokens":                  usage.ThoughtTokens,
		"total_tokens":                    usage.TotalTokens,
		"provider_reported_cost_subcents": usage.ProviderReportedCostSubcents,
		"estimated":                       usage.Estimated,
	}
}

func (s *Service) handleSessionInfoEvent(ctx context.Context, payload *lifecycle.AgentStreamEventPayload) {
	if payload == nil || payload.Data == nil || payload.SessionID == "" || s.repo == nil {
		return
	}
	info, err := s.mergedACPSessionInfo(ctx, payload.SessionID, payload.Data)
	if err != nil {
		s.logger.Warn("failed to read existing ACP session info",
			zap.String("session_id", payload.SessionID),
			zap.String("acp_session_id", payload.Data.ACPSessionID),
			zap.Error(err))
		return
	}
	if err := s.repo.SetSessionMetadataKey(ctx, payload.SessionID, "acp", info); err != nil {
		s.logger.Warn("failed to persist ACP session info",
			zap.String("session_id", payload.SessionID),
			zap.String("acp_session_id", payload.Data.ACPSessionID),
			zap.Error(err))
		return
	}
	if s.eventBus == nil {
		return
	}
	eventPayload := lifecycle.SessionInfoEventPayload{
		TaskID:           payload.TaskID,
		SessionID:        payload.SessionID,
		AgentID:          payload.AgentID,
		ACPSessionID:     stringFromMap(info, "session_id"),
		SessionTitle:     stringFromMap(info, "title"),
		SessionUpdatedAt: stringFromMap(info, "updated_at"),
		SessionMeta:      mapFromMap(info, "meta"),
		Timestamp:        time.Now().UTC().Format(time.RFC3339),
	}
	subject := events.BuildSessionInfoSubject(payload.SessionID)
	if err := s.eventBus.Publish(ctx, subject, bus.NewEvent(events.SessionInfoUpdated, "orchestrator", eventPayload)); err != nil {
		s.logger.Warn("failed to publish ACP session info",
			zap.String("session_id", payload.SessionID),
			zap.String("acp_session_id", eventPayload.ACPSessionID),
			zap.Error(err))
	}
}

func (s *Service) mergedACPSessionInfo(
	ctx context.Context,
	sessionID string,
	data *lifecycle.AgentStreamEventData,
) (map[string]interface{}, error) {
	info := map[string]interface{}{
		"session_id": "",
		"title":      "",
		"updated_at": "",
		"meta":       map[string]any{},
	}
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if session != nil {
		if existing, ok := session.Metadata["acp"].(map[string]interface{}); ok {
			for key, value := range existing {
				info[key] = value
			}
		}
	}
	if data.ACPSessionID != "" {
		info["session_id"] = data.ACPSessionID
	}
	if data.SessionTitle != "" {
		info["title"] = data.SessionTitle
	}
	if data.SessionUpdatedAt != "" {
		info["updated_at"] = data.SessionUpdatedAt
	}
	if data.SessionMeta != nil {
		info["meta"] = data.SessionMeta
	}
	return info, nil
}

func stringFromMap(values map[string]interface{}, key string) string {
	value, _ := values[key].(string)
	return value
}

func mapFromMap(values map[string]interface{}, key string) map[string]any {
	value, _ := values[key].(map[string]any)
	return value
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
	// Persist the agent-reported mode to session metadata so the user's chosen
	// permission mode survives a backend restart / SSR reload, mirroring how the
	// current model is persisted. Only non-empty modes are stored — an empty
	// CurrentModeID means the agent left a special mode, with nothing sticky to keep.
	if mode := payload.Data.CurrentModeID; mode != "" {
		s.persistSessionMode(ctx, sessionID, mode)
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

// persistSessionMode stores the agent-reported session permission mode in the
// session metadata using a targeted json_set, so other metadata keys (plan_mode,
// acp_session_id, …) are preserved. See issue #1183.
func (s *Service) persistSessionMode(ctx context.Context, sessionID, modeID string) {
	if s.repo == nil {
		return
	}
	if err := s.repo.SetSessionMetadataKey(ctx, sessionID, models.SessionMetaKeySessionMode, modeID); err != nil {
		s.logger.Warn("failed to persist session mode to metadata",
			zap.String("session_id", sessionID),
			zap.String("mode", modeID),
			zap.Error(err))
	}
	s.persistSessionRuntimeConfig(ctx, sessionID, "", modeID, nil)
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
// and persists the current model to the session snapshot so the model
// selector survives a page refresh without a flash.
func (s *Service) handleSessionModelsEvent(ctx context.Context, payload *lifecycle.AgentStreamEventPayload) {
	sessionID := payload.SessionID
	if sessionID == "" || s.eventBus == nil {
		return
	}

	// Store the write-once baseline before the mutable selector snapshot so a
	// concurrent task-detail boot cannot observe the new state without its
	// comparison values.
	configBaseline, err := s.sessionACPConfigBaselineForEvent(ctx, sessionID, payload.Data)
	if err != nil {
		s.logger.Warn("failed to persist ACP config baseline",
			zap.String("session_id", sessionID),
			zap.Error(err))
		return
	}
	s.persistSessionModelAndRuntimeConfig(
		ctx, sessionID, payload.Data.CurrentModelID, "", payload.Data.SessionModels, payload.Data.ConfigOptions,
	)

	eventPayload := lifecycle.SessionModelsEventPayload{
		TaskID:         payload.TaskID,
		SessionID:      sessionID,
		AgentID:        payload.AgentID,
		CurrentModelID: payload.Data.CurrentModelID,
		Models:         payload.Data.SessionModels,
		ConfigOptions:  payload.Data.ConfigOptions,
		ConfigBaseline: configBaseline,
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

func (s *Service) sessionACPConfigBaselineForEvent(
	ctx context.Context,
	sessionID string,
	data *lifecycle.AgentStreamEventData,
) (map[string]string, error) {
	baseline := s.loadSessionACPConfigBaseline(ctx, sessionID)
	if len(baseline) > 0 || data == nil || !configOptionsSettled(data.Data) {
		return baseline, nil
	}
	options := data.ConfigBaselineCandidate
	if len(options) == 0 {
		options = data.ConfigOptions
	}
	values := configOptionValues(options)
	if len(values) == 0 {
		return nil, nil
	}
	writeCtx := context.WithoutCancel(ctx)
	stored, err := s.repo.SetSessionMetadataKeyIfAbsent(
		writeCtx, sessionID, models.SessionMetaKeyACPConfigBaseline, values,
	)
	if err != nil {
		return nil, err
	}
	if stored {
		return values, nil
	}
	return s.loadSessionACPConfigBaseline(writeCtx, sessionID), nil
}

func configOptionValues(options []streams.ConfigOption) map[string]string {
	values := make(map[string]string, len(options))
	for _, option := range options {
		if option.ID != "" {
			values[option.ID] = option.CurrentValue
		}
	}
	if len(values) == 0 {
		return nil
	}
	return values
}

func configOptionsSettled(data any) bool {
	metadata, _ := data.(map[string]any)
	result, _ := metadata["config_options_settled"].(bool)
	return result
}

func (s *Service) loadSessionACPConfigBaseline(ctx context.Context, sessionID string) map[string]string {
	if s.repo == nil {
		return nil
	}
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil || session == nil {
		return nil
	}
	baseline, _ := models.LoadSessionACPConfigBaseline(session.Metadata)
	return baseline
}

// persistSessionModel writes the agent-reported current model to the session's
// AgentProfileSnapshot under the `model` key so SSR can render the model
// selector trigger with the right value on a page reload without a flash.
//
// We intentionally only persist the model (not the full set of dynamic config
// options) and intentionally do NOT replay this on backend-restart resume:
// agents that support session/load preserve the value themselves, and replay
// would issue redundant SetModel / SetConfigOption RPCs that cycle the session
// through STARTING / RUNNING and flicker the task into the sidebar's Running
// bucket (see session-resume-keeps-review-state.spec.ts and
// effectiveSessionMode's sibling note in lifecycle/manager_profile.go).
func (s *Service) persistSessionModel(ctx context.Context, sessionID, model string) {
	if model == "" {
		return
	}
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return
	}
	s.persistSessionModelOnSession(ctx, sessionID, session, model)
}

func (s *Service) persistSessionModelAndRuntimeConfig(
	ctx context.Context,
	sessionID, model, mode string,
	availableModels []streams.SessionModelInfo,
	options []streams.ConfigOption,
) {
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		s.logger.Warn("failed to load session for session model persistence",
			zap.String("session_id", sessionID),
			zap.Error(err))
		return
	}
	if session == nil {
		return
	}
	if model != "" {
		s.persistSessionModelOnSession(ctx, sessionID, session, model)
	}
	s.persistSessionRuntimeConfigOnSession(ctx, sessionID, session, model, mode, options)
	s.persistSessionModelsSnapshot(ctx, sessionID, model, availableModels, options)
}

func (s *Service) persistSessionModelsSnapshot(
	ctx context.Context,
	sessionID, currentModelID string,
	availableModels []streams.SessionModelInfo,
	options []streams.ConfigOption,
) {
	modelsForBoot := make([]streams.SessionModelInfo, 0, len(availableModels))
	for _, model := range availableModels {
		modelsForBoot = append(modelsForBoot, streams.SessionModelInfo{
			ModelID:         model.ModelID,
			Name:            model.Name,
			Description:     model.Description,
			UsageMultiplier: model.UsageMultiplier,
		})
	}
	snapshot := lifecycle.SessionModelsSnapshot{
		CurrentModelID: currentModelID,
		Models:         modelsForBoot,
		ConfigOptions:  options,
	}
	writeCtx := context.WithoutCancel(ctx)
	if err := s.repo.SetSessionMetadataKey(
		writeCtx, sessionID, models.SessionMetaKeyACPModelState, snapshot,
	); err != nil {
		s.logger.Warn("failed to persist ACP model selector state",
			zap.String("session_id", sessionID),
			zap.Error(err))
	}
}

func (s *Service) persistSessionRuntimeConfig(ctx context.Context, sessionID, model, mode string, options []streams.ConfigOption) {
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		s.logger.Warn("failed to load session for runtime config persistence",
			zap.String("session_id", sessionID),
			zap.Error(err))
		return
	}
	if session == nil {
		return
	}
	s.persistSessionRuntimeConfigOnSession(ctx, sessionID, session, model, mode, options)
}

func (s *Service) persistSessionRuntimeConfigOnSession(ctx context.Context, sessionID string, session *models.TaskSession, model, mode string, options []streams.ConfigOption) {
	cfg, _ := models.LoadSessionRuntimeConfig(session.Metadata)
	previousModel := cfg.Model
	applySessionRuntimeConfigUpdate(&cfg, model, mode, options)
	if cfg.IsZero() {
		return
	}
	writeCtx := context.WithoutCancel(ctx)
	if err := s.repo.SetSessionMetadataKey(writeCtx, sessionID, models.SessionMetaKeyRuntimeConfig, cfg); err != nil {
		s.logger.Warn("failed to persist session runtime config",
			zap.String("session_id", sessionID),
			zap.Error(err))
		return
	}
	if cfg.Model != "" {
		s.runtimeModelBySession.Store(sessionID, cfg.Model)
	}
	if cfg.Model != "" && cfg.Model != previousModel {
		if err := s.repo.SetSessionMetadataKey(writeCtx, sessionID, "context_window", nil); err != nil {
			s.logger.Warn("failed to clear stale context window after runtime model change",
				zap.String("session_id", sessionID),
				zap.String("previous_model", previousModel),
				zap.String("model", cfg.Model),
				zap.Error(err))
		}
	}
}

func (s *Service) persistSessionModelOnSession(ctx context.Context, sessionID string, session *models.TaskSession, model string) {
	if session.AgentProfileSnapshot == nil {
		session.AgentProfileSnapshot = make(map[string]interface{})
	}
	if existing, _ := session.AgentProfileSnapshot["model"].(string); existing == model {
		return
	}
	session.AgentProfileSnapshot["model"] = model
	if updater, ok := s.repo.(taskSessionAgentProfileSnapshotUpdater); ok {
		_ = updater.UpdateTaskSessionAgentProfileSnapshot(ctx, sessionID, session.AgentProfileSnapshot)
	} else {
		_ = s.repo.UpdateTaskSession(ctx, session)
	}
	// Invalidate the message creator's model cache so subsequent messages use the new model.
	if s.messageCreator != nil {
		s.messageCreator.InvalidateModelCache(sessionID)
	}
}

type taskSessionAgentProfileSnapshotUpdater interface {
	UpdateTaskSessionAgentProfileSnapshot(
		ctx context.Context,
		sessionID string,
		snapshot map[string]interface{},
	) error
}

func applySessionRuntimeConfigUpdate(cfg *models.SessionRuntimeConfig, model, mode string, options []streams.ConfigOption) {
	if model != "" {
		cfg.Model = model
	}
	if mode != "" {
		cfg.Mode = mode
	}
	for _, option := range options {
		if option.ID == "" || option.CurrentValue == "" {
			continue
		}
		if cfg.ConfigOptions == nil {
			cfg.ConfigOptions = make(map[string]string)
		}
		cfg.ConfigOptions[option.ID] = option.CurrentValue
		if option.ID == "model" || option.Category == "model" {
			cfg.Model = option.CurrentValue
		}
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
	if err := s.messageCreator.UpdatePermissionMessage(ctx, sessionID, payload.Data.PendingID, models.PermissionStatusExpired); err != nil {
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
