package orchestrator

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/sessionstate"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

const sessionModelConfigKey = "model"

// usageEventIDNamespace seeds the deterministic UUID usageEventIDFor derives
// for a prompt-usage completion. Arbitrary but fixed — any stable value
// works since it only needs to be consistent across process restarts, never
// shared with another namespace.
var usageEventIDNamespace = uuid.MustParse("2f6a6f8c-6c1b-4b8a-9e3e-7a6d2c5b9f10")

// handleAgentStreamEvent handles agent stream events (tool calls, message chunks, etc.)
func (s *Service) handleAgentStreamEvent(ctx context.Context, payload *lifecycle.AgentStreamEventPayload) {
	if payload == nil || payload.Data == nil {
		return
	}
	if payload.SessionID != "" {
		// Serialize stream side effects with cancellation/interrupt decisions.
		// Checking the terminal-execution marker alone is insufficient: a stream
		// handler can pass that check, then block while persisting a message while
		// a coordinator stop marks the execution terminal. Holding the shared
		// per-session guard makes the check and its side effects one decision.
		lock, release := s.acquireCancelInFlightGuard(payload.SessionID)
		defer release()
		lock.Lock()
		defer lock.Unlock()
	}
	// Cancellation owns the yielded interval between the guarded preparation
	// and lifecycle wait. Terminal frames from that captured execution/prompt
	// remain admissible so the lifecycle manager can drain them; frames from a
	// successor or stale execution must not mutate the session while the owner
	// is reconciling the cancelled turn.
	eventExecutionID := payload.ExecutionID
	if eventExecutionID == "" {
		eventExecutionID = payload.AgentID
	}
	eventType := payload.Data.Type
	if !s.cancellationOwnsStreamEvent(
		payload.SessionID,
		eventExecutionID,
		payload.Data.PromptGeneration,
	) {
		s.cancelClarificationWatchdogsForSession(payload.SessionID, eventType, payload)
		s.logger.Debug("ignoring stream event for execution outside cancellation identity",
			zap.String("task_id", payload.TaskID),
			zap.String("session_id", payload.SessionID),
			zap.String("event_execution_id", eventExecutionID),
			zap.Uint64("event_prompt_generation", payload.Data.PromptGeneration))
		return
	}
	taskID := payload.TaskID
	sessionID := payload.SessionID
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
		// Keep the message side suppressed for completed executions, but do not
		// discard a late subagent frame before its durable context is recorded.
		// This guard runs before the event-type switch below, so handler-level
		// recording alone would not cover the production dispatch path.
		if eventType == agentEventToolCall || eventType == agentEventToolUpdate {
			s.recordSubagentContextFromFrame(ctx, payload, s.nonCreatingActiveTurnID(ctx, payload.SessionID))
		}
		return
	}
	switch eventType {
	case "message_streaming", "thinking_streaming", agentEventComplete:
		s.observeDynamicAttempt(
			payload.SessionID,
			payload.ExecutionID,
			strings.TrimSpace(payload.Data.Text) != "",
			false,
		)
	case agentEventToolCall, agentEventToolUpdate:
		s.observeDynamicAttempt(payload.SessionID, payload.ExecutionID, false, true)
	}
	if eventType == agentEventComplete {
		defer s.clearDynamicAttemptEvidence(payload.SessionID, payload.ExecutionID)
	}

	if !terminalCompleteStream {
		// Any live agent stream activity means the agent resumed after clarification.
		// Cancel primary-path clarification watchdogs for this session. Late terminal
		// completes are excluded because they belong to an already-finished execution.
		s.cancelClarificationWatchdogsForSession(sessionID, eventType, payload)
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

	case agentEventToolUpdate:
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

	case "session_model_fallback":
		s.handleSessionModelFallbackEvent(ctx, payload)

	case streams.EventTypeSessionModelSelectionWarning:
		s.handleSessionModelSelectionWarningEvent(ctx, payload)

	case streams.EventTypeMCPAttachment:
		s.handleSessionMCPAttachmentEvent(ctx, payload)

	case streams.EventTypeSessionInfo:
		s.handleSessionInfoEvent(ctx, payload)

	case streams.EventTypeForegroundIdle:
		if !s.foregroundIdleOwnsCurrentPrompt(payload) {
			return
		}
		s.yieldForegroundAndPublish(ctx, taskID, sessionID, foregroundYieldProviderIdle)

	case streams.EventTypeBackgroundComplete:
		value := s.backgroundCompletionActivityValue(ctx, sessionID)
		if publication, changed := s.completeBackgroundWorkSnapshot(
			sessionID, payload.ExecutionID, payload.Data.ToolCallID, value,
		); changed {
			s.publishForegroundActivitySnapshot(ctx, taskID, sessionID, publication)
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

func (s *Service) backgroundCompletionActivityValue(ctx context.Context, sessionID string) interface{} {
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err == nil && session != nil && session.State != models.TaskSessionStateRunning {
		// A settled foreground has no generating substate to fall back to after
		// its final detached child finishes. Explicit null is required because
		// partial client-store merges preserve an omitted/stale background value.
		return nil
	}
	return string(v1.ForegroundActivityGenerating)
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

// streamEventIsStalePrompt proves that a generation-bearing stream event came
// from a prompt that no longer owns the execution. Generation-zero providers
// do not expose enough identity to make that determination, so their terminal
// events use the normal settlement path.
func (s *Service) streamEventIsStalePrompt(payload *lifecycle.AgentStreamEventPayload) bool {
	if payload == nil || payload.Data == nil || payload.Data.PromptGeneration == 0 {
		return false
	}
	generationOwner, ok := s.agentManager.(interface {
		OwnsPromptGeneration(sessionID, executionID string, generation uint64) bool
	})
	if !ok {
		return false
	}
	executionID := payload.ExecutionID
	if executionID == "" {
		executionID = payload.AgentID
	}
	return !generationOwner.OwnsPromptGeneration(
		payload.SessionID, executionID, payload.Data.PromptGeneration,
	)
}

func (s *Service) completeTurnForStreamEvent(
	ctx context.Context,
	payload *lifecycle.AgentStreamEventPayload,
	capturedTurnIDs ...string,
) {
	if payload == nil {
		return
	}
	var capturedTurnID string
	if len(capturedTurnIDs) > 0 {
		capturedTurnID = capturedTurnIDs[0]
	}
	if capturedTurnID != "" {
		// A durable turn ID is authoritative. If the event is stale and an
		// accepted successor exists, preserve that successor while settling
		// the predecessor; otherwise close only the captured turn so a late
		// completion cannot sweep an unrelated active turn.
		stale := s.streamEventIsStalePrompt(payload)
		successorTurnID := s.acceptedDispatchSuccessorTurn(payload.SessionID)
		if successorTurnID != "" && successorTurnID != capturedTurnID {
			stale = true
		}
		if stale && s.acceptedDispatchInFlight(payload.SessionID) {
			s.completeTurnForTaskSessionWithSuccessorPolicy(ctx, payload.TaskID, payload.SessionID, true)
			return
		}
		if err := s.completeTurnForTaskSessionCheckedOwned(ctx, payload.TaskID, payload.SessionID, capturedTurnID); err != nil {
			s.logger.Warn("failed to complete stream event's captured turn",
				zap.String("session_id", payload.SessionID),
				zap.String("turn_id", capturedTurnID),
				zap.Error(err))
		}
		if successorTurnID == capturedTurnID {
			s.clearAcceptedQueuedDispatch(payload.SessionID)
		}
		return
	}
	s.completeTurnForTaskSessionWithSuccessorPolicy(
		ctx,
		payload.TaskID,
		payload.SessionID,
		s.streamEventIsStalePrompt(payload),
	)
}

// handleAgentErrorEvent handles agentEventError events by creating an error message and completing the turn.
func (s *Service) handleAgentErrorEvent(ctx context.Context, payload *lifecycle.AgentStreamEventPayload) {
	taskID := payload.TaskID
	sessionID := payload.SessionID
	if sessionID != "" {
		failure := watcher.AgentEventData{
			TaskID:           taskID,
			SessionID:        sessionID,
			AgentExecutionID: payload.ExecutionID,
			AgentID:          payload.AgentID,
			ErrorMessage:     payload.Data.Error,
			ProviderError:    payload.Data.ProviderError,
		}
		if failure.ErrorMessage == "" {
			failure.ErrorMessage = payload.Data.Text
		}
		failure = s.withDynamicAttemptEvidence(failure)
		if s.routeDynamicAgentFailure(ctx, failure, classifyKanbanFailure(failure)) {
			return
		}
	}
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
	s.completeTurnForStreamEvent(ctx, payload)
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
	turnID := s.currentTurnIDForSession(ctx, sessionID)
	if turnID == "" {
		return
	}
	if err := s.messageCreator.CreateSessionMessage(
		ctx, taskID, statusMsg, sessionID,
		string(v1.MessageTypeStatus), turnID, nil, false,
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
		// A recognized subagent_task frame with no session id is exactly the
		// AC-2 identity-skip case (skipped_no_identity++): this guard predates
		// the subagent-context feature and exists to protect message
		// creation, so it must not silently swallow that counter too.
		s.recordSubagentContextFromFrame(ctx, payload, "")
		return
	}
	if s.shouldDropCompletedExecutionStreamEvent(payload) {
		// A late-arriving frame for an already-completed execution is
		// correctly dropped for message purposes (see the guard's own
		// contract), but it can be the ONLY frame that ever recognizes and
		// settles this subagent — dropping it here too would permanently
		// omit the row's status/token/duration data (AC-1, AC-11).
		s.recordSubagentContextFromFrame(ctx, payload, s.nonCreatingActiveTurnID(ctx, payload.SessionID))
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
	// Recording a subagent-context observation must never itself start a
	// turn as a side effect (that would mutate durable state purely to label
	// a telemetry row) — use the non-creating lookup here even though
	// message creation above may have legitimately started one already.
	s.recordSubagentContextFromFrame(ctx, payload, s.nonCreatingActiveTurnID(ctx, payload.SessionID))

	ownership := toolOwnershipForeground
	if payload.Data.ParentToolCallID != "" {
		ownership = toolOwnershipChild
	} else if normalizedIsBackgroundTask(payload.Data.Normalized) {
		ownership = toolOwnershipBackground
	}
	s.recordToolOwnership(
		payload.SessionID,
		payload.Data.ToolCallID,
		payload.ExecutionID,
		ownership,
	)

	// A top-level spawned background task (subagent / run-in-background shell)
	// holds the turn open while the foreground goes idle. A tool_call that
	// already arrives terminal is not outstanding work — clearing is driven by
	// tool_update, so registering it would leak into the hold and never clear.
	if isTerminalToolStatus(payload.Data.ToolStatus) {
		s.clearToolOwnership(payload.SessionID, payload.Data.ToolCallID, payload.ExecutionID)
		return
	}
	switch ownership {
	case toolOwnershipBackground:
		// Register with the launching execution/work IDs up front: relying on the
		// later tool_update path to backfill them never happens, because
		// hasBackgroundTask short-circuits registration once the tool_call_id is
		// already tracked. Without the execution ID here,
		// retireExecutionActivitySnapshot can never match this entry on
		// execution teardown, orphaning it if the execution dies before a
		// terminal tool frame arrives.
		kind := backgroundWorkKind(payload.Data.Normalized)
		if s.registerBackgroundWorkKind(
			payload.SessionID,
			payload.Data.ToolCallID,
			payload.ExecutionID,
			backgroundWorkID(payload.Data.Normalized),
			kind,
		) && kind == streams.BackgroundWorkKindSubagent {
			s.publishForegroundActivityChanged(ctx, payload.TaskID, payload.SessionID)
		}
	case toolOwnershipForeground:
		if s.markForegroundGenerating(payload.SessionID, payload.ExecutionID) {
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
	// Keep the private ownership estimate current for accounting. Only genuine
	// output flips it; empty/invalid frames are discarded below.
	if payload.Data.Text != "" && s.markForegroundGenerating(payload.SessionID, payload.ExecutionID) {
		s.publishForegroundActivityChanged(ctx, payload.TaskID, payload.SessionID)
	}
	s.handleStreamingEventKind(ctx, payload, "message",
		s.messageCreator.AppendAgentMessage,
		s.messageCreator.CreateAgentMessageStreaming)
}

// handleThinkingStreamingEvent handles streaming thinking events for real-time reasoning updates.
// It creates a new thinking message on first chunk (IsAppend=false) or appends to existing (IsAppend=true).
func (s *Service) handleThinkingStreamingEvent(ctx context.Context, payload *lifecycle.AgentStreamEventPayload) {
	// Keep the private ownership estimate current for accounting. Empty/invalid
	// frames are discarded downstream.
	if payload.Data.Text != "" && s.markForegroundGenerating(payload.SessionID, payload.ExecutionID) {
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
		// See the matching comment in handleToolCallEvent: this guard
		// predates the subagent-context feature and must not silently
		// swallow the AC-2 skipped_no_identity counter.
		s.recordSubagentContextFromFrame(ctx, payload, "")
		return
	}
	if s.shouldDropCompletedExecutionStreamEvent(payload) {
		// See the matching guard in handleToolCallEvent: a dropped update can
		// be the subagent's final terminal frame, and must still settle the
		// durable row even though the message side is correctly ignored.
		s.recordSubagentContextFromFrame(ctx, payload, s.nonCreatingActiveTurnID(ctx, payload.SessionID))
		return
	}
	ownership := s.resolveToolUpdateOwnership(payload)
	if isTerminalToolStatus(payload.Data.ToolStatus) {
		defer s.clearToolOwnership(
			payload.SessionID,
			payload.Data.ToolCallID,
			payload.ExecutionID,
		)
	}
	// A terminal update from a foreground tool can be the last substantive frame
	// after the provider has already announced foreground-idle. Its output still
	// belongs to the current prompt and therefore temporarily restores foreground
	// precedence until turn completion. Ownership comes from the initial tool
	// call; missing parent metadata on an incremental update cannot promote a
	// child or unknown tool to foreground.
	if isTerminalToolStatus(payload.Data.ToolStatus) &&
		len(payload.Data.ToolCallContents) > 0 &&
		ownership == toolOwnershipForeground &&
		s.markForegroundGenerating(payload.SessionID, payload.ExecutionID) {
		s.publishForegroundActivityChanged(ctx, payload.TaskID, payload.SessionID)
	}

	// Background-work bookkeeping runs regardless of message persistence so
	// accounting remains available even when no messageCreator is wired.
	s.trackBackgroundToolUpdate(ctx, payload, ownership)

	s.persistToolUpdateMessage(ctx, payload)
}

// persistToolUpdateMessage handles the message-persistence half of a
// tool_update event: updating (or fallback-creating) the tool call message and
// waking the session for a terminal update that belongs to an active turn.
// Split out of handleToolUpdateEvent to keep that function within the
// package's function-length limits; no behavior change.
func (s *Service) persistToolUpdateMessage(ctx context.Context, payload *lifecycle.AgentStreamEventPayload) {
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
	} else if s.messageCreator != nil {
		// Only message creation needs a turn to attach a fallback-created
		// card to, which is why this branch alone may lazily start one.
		// Recording subagent context must never create a turn merely to
		// label a row — see nonCreatingActiveTurnID.
		turnID = s.getActiveTurnID(payload.SessionID)
	} else {
		turnID = s.nonCreatingActiveTurnID(ctx, payload.SessionID)
	}

	// Message persistence is optional (see SetMessageCreator); turn-id
	// resolution and subagent-context recording below must not depend on it.
	if s.messageCreator != nil {
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
	}
	s.recordSubagentContextFromFrame(ctx, payload, turnID)

	// Terminal updates only wake an async turn that was established by prior
	// substantive output. A standalone terminal reconciliation belongs to the
	// already-settled turn that created the tool call.
	if terminal && status != "cancelled" && turnID != "" {
		s.setSessionRunningForExecution(ctx, payload.TaskID, payload.SessionID, payload.ExecutionID)
	}
}

// trackBackgroundToolUpdate maintains best-effort background accounting from a
// top-level tool_call_update: a terminal status clears the hold and the first
// recognizable non-terminal frame registers it. Child tool calls
// (ParentToolCallID set) are a subagent's own internal work, not a new
// background task, so they never touch the hold.
func (s *Service) trackBackgroundToolUpdate(
	ctx context.Context,
	payload *lifecycle.AgentStreamEventPayload,
	ownership toolOwnership,
) {
	if ownership == toolOwnershipChild || ownership == toolOwnershipUnknown {
		return
	}
	if isTerminalToolStatus(payload.Data.ToolStatus) {
		// A detached launch card is terminal as a tool invocation, but the
		// launched workload remains active until a provider background-complete
		// signal arrives. Monitor terminal payloads are no longer classified as
		// active, and synchronous subagents do not carry IsAsync.
		if normalizedIsDetachedLaunch(payload.Data.Normalized) {
			kind := backgroundWorkKind(payload.Data.Normalized)
			if s.registerBackgroundWorkKind(
				payload.SessionID,
				payload.Data.ToolCallID,
				payload.ExecutionID,
				backgroundWorkID(payload.Data.Normalized),
				kind,
			) && kind == streams.BackgroundWorkKindSubagent {
				s.publishForegroundActivityChanged(ctx, payload.TaskID, payload.SessionID)
			}
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
		if s.completeBackgroundTaskForExecution(
			payload.SessionID, payload.Data.ToolCallID, payload.ExecutionID,
		) {
			s.publishForegroundActivityChanged(ctx, payload.TaskID, payload.SessionID)
		}
		return
	}
	if s.hasBackgroundTask(
		payload.SessionID,
		payload.Data.ToolCallID,
		payload.ExecutionID,
	) {
		return
	}
	if ownership == toolOwnershipForeground {
		if s.markForegroundGenerating(payload.SessionID, payload.ExecutionID) {
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
	kind := backgroundWorkKind(payload.Data.Normalized)
	if s.registerBackgroundWorkKind(
		payload.SessionID,
		payload.Data.ToolCallID,
		payload.ExecutionID,
		backgroundWorkID(payload.Data.Normalized),
		kind,
	) && kind == streams.BackgroundWorkKindSubagent {
		s.publishForegroundActivityChanged(ctx, payload.TaskID, payload.SessionID)
	}
}

// resolveToolUpdateOwnership preserves the ownership established by the
// initial tool_call. Some ACP providers temporarily omit parent metadata on
// incremental child updates; absence is not positive evidence of foreground
// work. A normalized background shape is explicit enough to reclassify an
// initially incomplete top-level call, while a genuinely unknown update
// preserves the current activity.
func (s *Service) resolveToolUpdateOwnership(payload *lifecycle.AgentStreamEventPayload) toolOwnership {
	if payload.Data.ParentToolCallID != "" {
		s.recordToolOwnership(
			payload.SessionID,
			payload.Data.ToolCallID,
			payload.ExecutionID,
			toolOwnershipChild,
		)
		return toolOwnershipChild
	}
	if normalizedIsBackgroundTask(payload.Data.Normalized) {
		s.recordToolOwnership(
			payload.SessionID,
			payload.Data.ToolCallID,
			payload.ExecutionID,
			toolOwnershipBackground,
		)
		return toolOwnershipBackground
	}
	return s.toolOwnership(payload.SessionID, payload.Data.ToolCallID, payload.ExecutionID)
}

func backgroundWorkID(payload *streams.NormalizedPayload) string {
	if payload == nil {
		return ""
	}
	if background := payload.BackgroundWork(); background != nil {
		return background.WorkID
	}
	if subagent := payload.SubagentTask(); subagent != nil {
		return subagent.AgentID
	}
	if monitor := payload.Monitor(); monitor != nil {
		return monitor.TaskID
	}
	return ""
}

func backgroundWorkKind(payload *streams.NormalizedPayload) streams.BackgroundWorkKind {
	if payload == nil || payload.BackgroundWork() == nil {
		return ""
	}
	return payload.BackgroundWork().Kind
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

// recordSubagentContextFromFrame persists a durable relational record of a
// subagent (Task tool) invocation when this frame's normalized payload is a
// recognized subagent_task. No-op when subagentContexts is unwired, when the
// frame isn't a subagent_task, or when the normalizer hasn't yet attached the
// typed payload (the initial tool_call for Claude/OpenCode carries none —
// recognition happens on a later tool_call_update, see AC-1a). turnID is
// whatever the call site already resolved; this helper never re-derives one.
//
// payload.SessionID is the Kandev task session id despite the
// agentSessionID-shaped naming downstream: messageCreatorAdapter passes the
// same value into CreateMessageRequest.TaskSessionID
// (internal/backendapp/adapters.go:879).
func (s *Service) recordSubagentContextFromFrame(ctx context.Context, payload *lifecycle.AgentStreamEventPayload, turnID string) {
	if s.subagentContexts == nil || payload == nil || payload.Data == nil {
		return
	}
	normalized := payload.Data.Normalized
	if normalized == nil || normalized.Kind() != streams.ToolKindSubagentTask {
		return
	}
	subagentTask := normalized.SubagentTask()
	if subagentTask == nil {
		return
	}
	s.subagentContexts.RecordSubagentContext(ctx, taskservice.RecordSubagentContextRequest{
		TaskSessionID:    payload.SessionID,
		TaskID:           payload.TaskID,
		TurnID:           turnID,
		ToolCallID:       payload.Data.ToolCallID,
		ParentToolCallID: payload.Data.ParentToolCallID,
		ExecutionID:      payload.ExecutionID,
		ToolStatus:       payload.Data.ToolStatus,
		Payload:          subagentTask,
		ObservedAt:       time.Now().UTC(),
	})
}

// nonCreatingActiveTurnID resolves sessionID's active turn without ever
// starting one — unlike getActiveTurnID, whose doc comment explains it
// lazily starts a turn "even in edge cases like resumed sessions". Every
// caller here uses the result only to label a subagent-context row, never to
// attach a message, so recording an observation must never itself mutate
// state by creating a durable turn as a side effect. A lookup error is
// treated as "no turn known", matching the fail-closed pattern already used
// for terminal tool updates below.
func (s *Service) nonCreatingActiveTurnID(ctx context.Context, sessionID string) string {
	turnID, err := s.peekActiveTurnID(ctx, sessionID)
	if err != nil {
		return ""
	}
	return turnID
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
	if isTerminalSessionState(nextState) {
		if err := s.expireTerminalClarificationWaiters(ctx, sessionID); err != nil {
			s.logger.Error("failed to expire clarification on terminal session; response claims remain quarantined",
				zap.String("task_id", taskID),
				zap.String("session_id", sessionID),
				zap.Error(err))
		}
	}
	// Work has resumed: a session entering STARTING/RUNNING clears the
	// startup interruption marker and republishes the task so open clients
	// drop the red interruption icon. No-op when the marker is absent.
	if nextState == models.TaskSessionStateStarting || nextState == models.TaskSessionStateRunning {
		s.clearTaskInterruptedMarker(ctx, taskID)
		s.clearTaskAutoStartFailedMarker(ctx, taskID)
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

	s.republishTaskActivityOnSettle(ctx, taskID, oldState, nextState)

	// Auto-promote another session to primary when the current primary enters a terminal state
	s.maybePromotePrimary(ctx, taskID, sessionID, nextState)
	return session, true
}

// republishTaskActivityOnSettle recomputes the task-level MOST-ACTIVE-WINS
// activity aggregate when a session leaves the generating-capable RUNNING state.
// On agent completion the turn-activity record is retired (detached) while the
// session is still RUNNING, then the session settles to WAITING_FOR_INPUT (or a
// terminal state) without a task-level republish. A detached record safely
// defaults to "generating" (turn_activity.go isForegroundTurnGenerating), so the
// last cached task aggregate stays "generating" and the sidebar/board spinner
// never clears. Republishing off the settled session list corrects the aggregate
// (the completed session is no longer RUNNING, so it drops out); the call is
// deduplicated by the task service and is a no-op when unchanged.
func (s *Service) republishTaskActivityOnSettle(
	ctx context.Context,
	taskID string,
	oldState, nextState models.TaskSessionState,
) {
	if oldState != models.TaskSessionStateRunning || nextState == models.TaskSessionStateRunning {
		return
	}
	s.publishTaskActivityIfChanged(ctx, taskID)
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
		// The hook may persist state-specific metadata after the state CAS. Read
		// the row again so the state event carries that metadata to projections.
		// Without this refresh, the event publishes the pre-hook snapshot and a
		// typed launch error can be durable but invisible in the task summary.
		refreshed = s.refreshTaskSessionOr(ctx, sessionID, refreshed)
	}
	if isTerminalSessionState(nextState) {
		if err := s.expireTerminalClarificationWaiters(ctx, sessionID); err != nil {
			s.logger.Error("failed to expire clarification on strict terminal transition; response claims remain quarantined",
				zap.String("task_id", taskID),
				zap.String("session_id", sessionID),
				zap.Error(err))
		}
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
	s.republishTaskActivityOnSettle(ctx, taskID, oldState, nextState)
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
	var foregroundActivity interface{}
	if nextState == models.TaskSessionStateRunning {
		foregroundActivity = string(s.ForegroundActivity(sessionID))
	} else if activity := s.ForegroundActivity(sessionID); activity == v1.ForegroundActivityBackground {
		// Only the enabled Claude experiment can return background here.
		// Preserve detached-work visibility as the coarse foreground settles.
		foregroundActivity = string(activity)
	}
	eventData := map[string]interface{}{
		metaKeyTaskID:               taskID,
		metaKeySessionID:            sessionID,
		"old_state":                 string(oldState),
		metaKeyNewState:             string(nextState),
		"error_message":             errorMessage,
		metaKeyAgentProfileID:       agentProfileID,
		"agent_profile_snapshot":    session.AgentProfileSnapshot,
		"execution_profile_id":      session.ExecutionProfileID,
		"route_generation":          session.RouteGeneration,
		"route_state":               session.RouteState,
		"route_reason":              session.RouteReason,
		"route_error_code":          session.RouteErrorCode,
		"route_error_class":         session.RouteErrorClass,
		"route_catalogue_version":   session.RouteCatalogueVersion,
		"route_retry_ordinal":       session.RouteRetryOrdinal,
		"route_deadline":            session.RouteDeadline,
		"route_pending_outcome":     session.RoutePendingOutcome,
		"downstream_acp_session_id": session.DownstreamACPSessionID,
		"is_passthrough":            session.IsPassthrough,
		"is_primary":                session.IsPrimary,
		// Carry activity only while the durable session is RUNNING. Every other
		// state gets an explicit null so partial client-store merges clear a
		// previously-live busy signal during settlement or teardown.
		"foreground_activity":   foregroundActivity,
		"active_subagent_count": s.ActiveSubagentCount(sessionID),
		"supports_steering":     s.SteerEligible(sessionID, nextState),
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
	candidate := terminalExecutionMarker{
		expiresAt:           expiresAt,
		allowCompleteStream: allowCompleteStream,
		turnID:              s.currentTurnIDForSession(context.Background(), sessionID),
	}
	for {
		value, loaded := s.completedExecutions.LoadOrStore(key, candidate)
		if !loaded {
			break
		}
		current, ok := value.(terminalExecutionMarker)
		if !ok || time.Now().After(current.expiresAt) {
			s.completedExecutions.CompareAndDelete(key, value)
			continue
		}
		merged := candidate
		if current.allowCompleteStream {
			// Terminal stream permission is monotonic for an execution:
			// StopExecution may emit agent.stopped after agent.completed but
			// before the successful execution's buffered complete stream.
			merged.allowCompleteStream = true
			merged.turnID = current.turnID
		}
		if s.completedExecutions.CompareAndSwap(key, value, merged) {
			break
		}
	}
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

type readyTurnMark struct {
	turnID    string
	expiresAt time.Time
}

func readyTurnKey(sessionID, executionID string, promptGeneration uint64) string {
	return fmt.Sprintf("%s\x00%s\x00%d", sessionID, executionID, promptGeneration)
}

// readyTurnZeroGenKey is deliberately generation-less: promptGeneration==0
// means the transport carries no generation tracking at all (see
// finishPromptCompletion / claimPromptCompletion's early return), so every
// completion on this (session, execution) shares the same identity and must
// be threaded through the FIFO queue in s.readyTurnMarksZeroGen instead of
// the single-slot s.readyTurnMarks map.
func readyTurnZeroGenKey(sessionID, executionID string) string {
	return fmt.Sprintf("%s\x00%s", sessionID, executionID)
}

// markReadyTurn records the turn ID handleAgentReady confirmed for
// (sessionID, executionID, promptGeneration), just before completeTurnForSession
// closes it and removes it from activeTurns. handleCompleteEventMarkState
// (lifecycle package) publishes agent.ready and closes the turn before the
// complete-stream frame for the same completion is published, on EVERY
// transport, including promptGeneration==0 (generation-less) completions —
// so this mark is required there too, not skippable. Because generation-less
// completions share one key per (session, execution) with no generation to
// disambiguate them, they queue FIFO in readyTurnMarksZeroGen instead of the
// single-slot readyTurnMarks map: ready and complete-stream for the same
// completion are published strictly in that order per execution, so pending
// marks and pending completions correlate 1:1 in arrival order.
//
// This ordering is airtight on the default in-memory event bus, where
// Publish delivers to a subject's subscriber synchronously before returning
// (see internal/events/bus/memory.go). It is best-effort, not guaranteed, on
// a NATS-backed bus (opt-in via cfg.NATS.URL): AgentReady and the
// agent.stream.* wildcard are different subjects, and this in-memory map is
// per-process. The durable TurnID on the completion payload closes that
// cross-subject/cross-instance gap; this mark remains a compatibility fallback
// for older producers and completion paths without a captured ID.
func (s *Service) markReadyTurn(sessionID, executionID string, promptGeneration uint64, turnID string) {
	if sessionID == "" || executionID == "" || turnID == "" {
		return
	}
	expiresAt := time.Now().Add(completedExecutionRetention)
	if promptGeneration == 0 {
		s.pushZeroGenReadyTurnMark(sessionID, executionID, readyTurnMark{turnID: turnID, expiresAt: expiresAt})
		return
	}
	key := readyTurnKey(sessionID, executionID, promptGeneration)
	s.readyTurnMarks.Store(key, readyTurnMark{turnID: turnID, expiresAt: expiresAt})
	time.AfterFunc(completedExecutionRetention, func() {
		s.deleteReadyTurnMarkIfExpired(key, expiresAt)
	})
}

// takeReadyTurnMark consumes (and removes) the turn ID markReadyTurn recorded
// for this completion, if any. A miss is expected whenever the completion
// never went through handleAgentReady's synchronous ready path (or the mark
// already expired); the caller falls back to a live lookup or a terminal-
// execution snapshot in that case.
func (s *Service) takeReadyTurnMark(sessionID, executionID string, promptGeneration uint64) (string, bool) {
	if sessionID == "" || executionID == "" {
		return "", false
	}
	if promptGeneration == 0 {
		return s.popZeroGenReadyTurnMark(sessionID, executionID)
	}
	key := readyTurnKey(sessionID, executionID, promptGeneration)
	value, ok := s.readyTurnMarks.LoadAndDelete(key)
	if !ok {
		return "", false
	}
	mark, ok := value.(readyTurnMark)
	if !ok || time.Now().After(mark.expiresAt) {
		return "", false
	}
	return mark.turnID, true
}

func (s *Service) deleteReadyTurnMarkIfExpired(key string, expiresAt time.Time) {
	value, ok := s.readyTurnMarks.Load(key)
	if !ok {
		return
	}
	current, ok := value.(readyTurnMark)
	if !ok || !current.expiresAt.After(expiresAt) {
		s.readyTurnMarks.Delete(key)
	}
}

// pushZeroGenReadyTurnMark appends a generation-less ready-turn mark to the
// FIFO queue for (sessionID, executionID). Guarded by
// readyTurnMarksZeroGenMu: sync.Map has no atomic append, and multiple
// pending marks for the same key are the expected case here (unlike the
// generation-keyed map, where each key holds at most one).
func (s *Service) pushZeroGenReadyTurnMark(sessionID, executionID string, mark readyTurnMark) {
	key := readyTurnZeroGenKey(sessionID, executionID)
	s.readyTurnMarksZeroGenMu.Lock()
	if s.readyTurnMarksZeroGen == nil {
		s.readyTurnMarksZeroGen = make(map[string][]readyTurnMark)
	}
	s.readyTurnMarksZeroGen[key] = append(s.readyTurnMarksZeroGen[key], mark)
	s.readyTurnMarksZeroGenMu.Unlock()
	time.AfterFunc(completedExecutionRetention, func() {
		s.pruneExpiredZeroGenReadyTurnMarks(key)
	})
}

// popZeroGenReadyTurnMark consumes the oldest live mark queued for
// (sessionID, executionID), discarding any expired entries ahead of it.
func (s *Service) popZeroGenReadyTurnMark(sessionID, executionID string) (string, bool) {
	key := readyTurnZeroGenKey(sessionID, executionID)
	s.readyTurnMarksZeroGenMu.Lock()
	defer s.readyTurnMarksZeroGenMu.Unlock()
	queue := s.readyTurnMarksZeroGen[key]
	now := time.Now()
	for len(queue) > 0 {
		mark := queue[0]
		queue = queue[1:]
		if now.After(mark.expiresAt) {
			continue
		}
		if len(queue) == 0 {
			delete(s.readyTurnMarksZeroGen, key)
		} else {
			s.readyTurnMarksZeroGen[key] = queue
		}
		return mark.turnID, true
	}
	delete(s.readyTurnMarksZeroGen, key)
	return "", false
}

// pruneExpiredZeroGenReadyTurnMarks drops expired entries from the front of
// (sessionID+executionID)'s queue so an unconsumed mark cannot grow the map
// unbounded, mirroring completedExecutions' expiry pattern.
func (s *Service) pruneExpiredZeroGenReadyTurnMarks(key string) {
	s.readyTurnMarksZeroGenMu.Lock()
	defer s.readyTurnMarksZeroGenMu.Unlock()
	queue := s.readyTurnMarksZeroGen[key]
	if len(queue) == 0 {
		return
	}
	now := time.Now()
	kept := queue[:0]
	for _, mark := range queue {
		if now.Before(mark.expiresAt) {
			kept = append(kept, mark)
		}
	}
	if len(kept) == 0 {
		delete(s.readyTurnMarksZeroGen, key)
		return
	}
	s.readyTurnMarksZeroGen[key] = kept
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

func allowsSessionStartingRecovery(
	nextState, expectedState, currentState models.TaskSessionState,
	promoteTask bool,
) bool {
	return !promoteTask &&
		nextState == models.TaskSessionStateStarting &&
		currentState == expectedState &&
		(expectedState == models.TaskSessionStateFailed ||
			expectedState == models.TaskSessionStateCancelled)
}

func (s *Service) setSessionStarting(
	ctx context.Context,
	taskID string,
	session *models.TaskSession,
	expectedState models.TaskSessionState,
	promoteTask bool,
) error {
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
		allowedTerminalRecovery := allowsSessionStartingRecovery(
			session.State, expectedState, current.State, promoteTask,
		)
		if isTerminalSessionState(current.State) && !allowedTerminalRecovery {
			return &executor.SessionStateSupersededError{SessionID: session.ID, State: current.State}
		}

		oldState = current.State
		if err := s.persistFullTaskSessionIfCurrent(ctx, session, expectedState); err != nil {
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

	// The launch path moves a session to STARTING without going through
	// updateTaskSessionStateWithHook, so clear the interruption marker here
	// too (no-op when absent).
	s.clearTaskInterruptedMarker(ctx, taskID)
	s.clearTaskAutoStartFailedMarker(ctx, taskID)

	if publishSession != nil {
		s.publishTaskSessionStateChanged(ctx, taskID, session.ID, oldState, session.State, session.ErrorMessage, stateUpdatedAt, publishSession)
	}
	return nil
}

// clearTaskInterruptedMarker removes the startup interruption marker from a
// task and republishes task.updated when it was actually present, so open
// clients drop the red interruption icon. Called from the session-start
// funnel when a session enters STARTING/RUNNING — work has resumed, so the
// task is no longer interrupted. No-op when the marker is absent.
func (s *Service) clearTaskInterruptedMarker(ctx context.Context, taskID string) {
	if taskID == "" {
		return
	}
	removed, err := s.repo.RemoveTaskMetadataKey(ctx, taskID, models.MetaKeyInterruptedAt)
	if err != nil {
		s.logger.Warn("failed to clear interrupted marker",
			zap.String("task_id", taskID),
			zap.Error(err))
		return
	}
	if !removed {
		return
	}
	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil || task == nil {
		s.logger.Warn("failed to load task for interrupted-clear publish",
			zap.String("task_id", taskID),
			zap.Error(err))
		return
	}
	s.publishTaskUpdated(ctx, task)
}

// clearTaskAutoStartFailedMarker removes the auto-start-failure marker from a
// task and republishes task.updated when it was actually present, so open
// clients drop the failure badge. Called from the same session-start funnel
// as clearTaskInterruptedMarker: a session entering STARTING/RUNNING means an
// agent did launch, so any earlier auto-start failure no longer applies.
// No-op when the marker is absent.
func (s *Service) clearTaskAutoStartFailedMarker(ctx context.Context, taskID string) {
	if taskID == "" {
		return
	}
	removed, err := s.repo.RemoveTaskMetadataKey(ctx, taskID, models.MetaKeyAutoStartFailed)
	if err != nil {
		s.logger.Warn("failed to clear auto-start-failed marker",
			zap.String("task_id", taskID),
			zap.Error(err))
		return
	}
	if !removed {
		return
	}
	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil || task == nil {
		s.logger.Warn("failed to load task for auto-start-failed-clear publish",
			zap.String("task_id", taskID),
			zap.Error(err))
		return
	}
	s.publishTaskUpdated(ctx, task)
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

// Child terminal receipts use the task's parent relationship and durable
// clarification projection to decide whether a session can collapse to
// COMPLETED. Root-task sibling sessions keep the original WAITING affordance.
// setSessionWaitingForInputIfRequested is the terminal-receipt variant
// of setSessionWaitingForInput. It only applies to subtasks (task.ParentID
// non-empty) because the symptom it guards — "child WAITING_FOR_INPUT
// after the agent's last turn had no active clarification" — only
// arises for child tasks whose task or workflow step is terminal, not to
// surface a UI prompt. Sibling sessions on a root task keep the original
// affordance so a finishing session on a multi-session task still flips to
// WAITING_FOR_INPUT.
//
// When a terminal task has no input request, collapse the session to
// COMPLETED so the child row does not leak in a stuck active state. A clean
// turn on a non-terminal child is not enough evidence for that collapse, so
// it remains WAITING_FOR_INPUT.
func (s *Service) setSessionWaitingForInputIfRequested(
	ctx context.Context,
	taskID, sessionID string,
	preloadedSession ...*models.TaskSession,
) {
	task, taskErr := s.repo.GetTask(ctx, taskID)
	if taskErr != nil || task == nil {
		// A task lookup failure is not evidence that a child reached a terminal
		// state. Preserve the promptable session instead of leaving it RUNNING.
		s.logger.Warn("failed to load task before terminal receipt; preserving WAITING state",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID),
			zap.Error(taskErr))
		s.setSessionWaitingForInput(ctx, taskID, sessionID, preloadedSession...)
		return
	}
	if task.ParentID == "" {
		s.setSessionWaitingForInput(ctx, taskID, sessionID, preloadedSession...)
		return
	}
	activeClarifications, err := s.repo.FindActiveClarificationMessagesBySessionID(ctx, sessionID)
	if err != nil {
		// A transient reader error is not evidence that the child completed.
		// Preserve the promptable state and avoid leaving the session RUNNING.
		s.logger.Warn("failed to read active clarifications; preserving WAITING state",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID),
			zap.Error(err))
		s.setSessionWaitingForInput(ctx, taskID, sessionID, preloadedSession...)
		return
	}
	taskTerminal := models.IsTerminalTaskState(task.State) || s.workflowStepIsTerminal(ctx, task.WorkflowStepID)
	if len(activeClarifications) == 0 && taskTerminal {
		s.logger.Debug("subtask terminal: skipping WAITING write; collapsing session to COMPLETED",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID))
		s.updateTaskSessionState(ctx, taskID, sessionID, models.TaskSessionStateCompleted, "", false)
		// The child has no further use for the provider-runtime reservation.
		// reclaimIdleSession is fail-closed (only proceeds when there is no
		// live agent and no active turn) and best-effort: a reclaim failure
		// logs and returns without affecting the terminal collapse.
		if err := s.reclaimIdleSession(ctx, sessionID); err != nil {
			s.logger.Warn("subtask terminal: reclaim failed; row preserved",
				zap.String("task_id", taskID),
				zap.String("session_id", sessionID),
				zap.Error(err))
		}
		return
	}
	if len(activeClarifications) == 0 {
		s.logger.Debug("subtask turn completed before terminal task state; preserving WAITING state",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID),
			zap.String("task_state", string(task.State)))
	}
	s.setSessionWaitingForInput(ctx, taskID, sessionID, preloadedSession...)
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

	if updatedSession, _ := s.updateTaskSessionStateWithHook(
		ctx,
		taskID,
		sessionID,
		models.TaskSessionStateRunning,
		"",
		true,
		func() {
			s.reconcileRunningTaskStateLocked(ctx, taskID, sessionID)
		},
		session,
	); updatedSession != nil {
		if len(preloadedSession) > 0 && preloadedSession[0] != nil && preloadedSession[0] != updatedSession {
			*preloadedSession[0] = *updatedSession
		}
	}

	if wasAlreadyRunning {
		return
	}
}

// reconcileRunningTaskStateLocked requires taskRuntimeStateMu to be held.
func (s *Service) reconcileRunningTaskStateLocked(ctx context.Context, taskID, sessionID string) {
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

	// The lifecycle completion payload carries the durable turn captured before
	// AgentReady can admit a successor prompt. Older producers do not include
	// it, so retain the generation-keyed (or generation-less FIFO) ready mark as
	// a compatibility fallback. A miss also means this completion never went
	// through handleAgentReady's ready path (e.g. an error/interrupt that
	// completed the execution directly), so fall back to whichever snapshot the
	// branch has: terminalMarker.turnID (captured by markTerminalExecution at
	// agent.completed) for terminal completions, or a live active-turn lookup
	// for non-terminal ones.
	completionTurnID := payload.Data.TurnID
	if completionTurnID == "" {
		var ok bool
		completionTurnID, ok = s.takeReadyTurnMark(payload.SessionID, payload.ExecutionID, payload.Data.PromptGeneration)
		if !ok {
			if terminalCompleteStream {
				completionTurnID = terminalMarker.turnID
			} else {
				completionTurnID = s.currentTurnIDForSession(ctx, payload.SessionID)
			}
		}
	}
	s.publishPromptUsage(ctx, payload, session, completionTurnID)

	if terminalCompleteStream {
		terminalTurnID := terminalMarker.turnID
		if payload.Data.TurnID != "" {
			terminalTurnID = payload.Data.TurnID
		}
		s.saveAgentTextForTurn(ctx, payload, terminalTurnID)
		s.publishAgentPlanForTurn(ctx, payload, terminalTurnID, false)
		s.persistTurnPromptMetadataForTurn(ctx, payload, session, terminalTurnID)
		if terminalTurnID != "" {
			s.publishAgentTurnCompleteForTurn(ctx, payload, terminalTurnID)
		}
		s.detachClarificationWaiters(ctx, payload.SessionID)
		s.logger.Debug("complete stream from terminal execution flushed final data; skipping active turn and runtime reconciliation",
			zap.String("task_id", payload.TaskID),
			zap.String("session_id", payload.SessionID),
			zap.String("agent_execution_id", payload.ExecutionID),
			zap.String("turn_id", terminalTurnID))
		return
	}

	if completionTurnID != "" {
		s.saveAgentTextForTurn(ctx, payload, completionTurnID)
		s.publishAgentPlanForTurn(ctx, payload, completionTurnID, false)
		s.persistTurnPromptMetadataForTurn(ctx, payload, session, completionTurnID)
	} else {
		s.saveAgentTextIfPresent(ctx, payload)
		s.publishAgentPlanIfPresent(ctx, payload)
		s.persistTurnPromptMetadata(ctx, payload, session)
	}
	s.completeTurnForStreamEvent(ctx, payload, completionTurnID)

	// Publish agent turn message event so the office comment bridge can
	// auto-post the agent's response as a task comment. Published here
	// (not in saveAgentTextIfPresent) because for streaming agents the
	// text is drained by message_chunk events and Data.Text is empty at
	// complete time.
	s.publishAgentTurnCompleteForTurn(ctx, payload, completionTurnID)

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
	if session != nil && s.handleAutomationTurnCompleteForTurn(
		ctx,
		payload.TaskID,
		payload.SessionID,
		session,
		completionTurnID,
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
	// Terminal-receipt path. Only flip the session to WAITING_FOR_INPUT
	// when the most recent agent-authored message actually asked the
	// user for input. Sibling sessions (root task, ParentID empty) keep
	// the original affordance so a finishing session on a multi-session
	// task still flips to WAITING — only subtasks (ParentID non-empty)
	// get the guard.
	s.setSessionWaitingForInputIfRequested(ctx, payload.TaskID, payload.SessionID, session)
}

func (s *Service) detachClarificationWaiters(ctx context.Context, sessionID string) {
	if s.clarificationCanceller == nil || sessionID == "" {
		return
	}
	n, err := s.clarificationCanceller.DetachSessionAndNotify(ctx, sessionID)
	if err != nil {
		s.logger.Warn("failed to detach pending clarifications on turn complete",
			zap.String("session_id", sessionID),
			zap.Error(err))
		return
	}
	if n > 0 {
		s.logger.Info("detached pending clarifications on turn complete",
			zap.String("session_id", sessionID),
			zap.Int("count", n))
	}
}

func (s *Service) expireClarificationWaiters(ctx context.Context, sessionID string) error {
	if s.clarificationCanceller == nil || sessionID == "" {
		return nil
	}
	n, err := s.clarificationCanceller.ExpireSessionAndNotify(ctx, sessionID)
	if n > 0 {
		s.logger.Info("expired pending clarifications on terminal session",
			zap.String("session_id", sessionID),
			zap.Int("count", n))
	}
	return err
}

const terminalClarificationExpiryTimeout = 10 * time.Second

func (s *Service) expireTerminalClarificationWaiters(ctx context.Context, sessionID string) error {
	expireCtx, cancel := context.WithTimeout(ctx, terminalClarificationExpiryTimeout)
	defer cancel()
	return s.expireClarificationWaiters(expireCtx, sessionID)
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

// usageEventIDFor derives the office cost subscriber's idempotency key from
// immutable upstream identity — session, execution, and prompt generation —
// instead of minting a fresh random value on every call. A minted-per-call
// key can only dedup literal redelivery of the identical *bus.Event object,
// which neither event bus provides (see internal/events/bus/{memory,nats}.go
// — the memory bus delivers once synchronously with no retry, and the NATS
// bus is plain core pub/sub, not JetStream). The real duplicate source is
// the SAME underlying completion frame reaching publishPromptUsage twice —
// e.g. a reconnecting WS client replaying a buffered stream event.
//
// promptGeneration==0 means this completion carries no generation tracking
// at all (see claimPromptCompletion's early return in the lifecycle
// package); deriving a key from (session, execution, 0) there would collide
// across genuinely distinct turns on a generation-less transport and
// silently under-count cost, which is worse than the duplicate-row bug this
// fixes. Fall back to a random id in that narrow case — unchanged from
// prior behavior there.
func usageEventIDFor(sessionID, executionID string, promptGeneration uint64) string {
	if sessionID == "" || executionID == "" || promptGeneration == 0 {
		return uuid.New().String()
	}
	name := fmt.Sprintf("%s\x00%s\x00%d", sessionID, executionID, promptGeneration)
	return uuid.NewSHA1(usageEventIDNamespace, []byte(name)).String()
}

// publishPromptUsage broadcasts prompt token usage to the WebSocket for the
// frontend and to the office cost subscriber. Model and agent type (CLI
// engine slug) come from payload first; when absent (which is the common
// case — CurrentModelID only travels on session_models frames) we fall back
// to the session's AgentProfileSnapshot, populated at session creation and
// refreshed by persistSessionModel on ACP model updates.
// AgentProfileID always comes from the persistent task session. It must not
// be resolved from the mutable workflow runner projection after publication.
//
// turnID is resolved by the caller (handleCompleteStreamEvent), not here:
// the terminal-execution snapshot and the live active-turn lookup are both
// call-site concerns. usageEventID is derived here, once, at the single
// publish site by usageEventIDFor — that is what makes it a stable
// idempotency key across a republished frame; a downstream consumer
// deriving its own would defeat the point.
func (s *Service) publishPromptUsage(
	ctx context.Context,
	payload *lifecycle.AgentStreamEventPayload,
	session *models.TaskSession,
	turnID string,
) {
	sessionID := payload.SessionID
	if sessionID == "" || s.eventBus == nil || payload.Data.Usage == nil {
		return
	}

	model, agentType := resolvePromptUsageLabels(payload, session)
	agentProfileID := ""
	if session != nil {
		agentProfileID = session.AgentProfileID
	}

	eventPayload := lifecycle.SessionPromptUsageEventPayload{
		TaskID:         payload.TaskID,
		SessionID:      sessionID,
		AgentID:        payload.AgentID,
		AgentProfileID: agentProfileID,
		AgentType:      agentType,
		Model:          model,
		Usage:          payload.Data.Usage,
		Timestamp:      time.Now().UTC().Format(time.RFC3339),
		TurnID:         turnID,
		UsageEventID: usageEventIDFor(
			sessionID, payload.ExecutionID, payload.Data.PromptGeneration,
		),
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
			if m, ok := session.AgentProfileSnapshot[sessionModelConfigKey].(string); ok {
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
	updates := map[string]interface{}{
		"prompt_usage": promptUsageMetadata(payload.Data.Usage),
	}
	if model != "" {
		updates[sessionModelConfigKey] = model
	}
	if agentType != "" {
		updates["agent_type"] = agentType
	}
	if payload.AgentID != "" {
		updates["agent_id"] = payload.AgentID
	}
	if err := s.turnService.PatchTurnMetadata(ctx, turn.TaskSessionID, turn.ID, updates); err != nil {
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
		"output_tokens_present":           usage.OutputTokensPresent,
		"cached_read_tokens":              usage.CachedReadTokens,
		"cached_write_tokens":             usage.CachedWriteTokens,
		"thought_tokens":                  usage.ThoughtTokens,
		"total_tokens":                    usage.TotalTokens,
		"provider_reported_cost_subcents": usage.ProviderReportedCostSubcents,
		"provider_reported_cost_present":  usage.ProviderReportedCostPresent,
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
	if sessionID == "" {
		return
	}
	// Record the negotiated prompt-queueing advertisement before the event-bus
	// guard. It gates prompt handoff and mid-turn steering, so it must land as
	// soon as the agent advertises it — recording is admission state, whereas the
	// bus below is only broadcast. Skipping it when no bus is configured would
	// silently make a capable agent ineligible.
	s.recordSessionPromptQueueing(sessionID, payload.Data.SupportsPromptQueueing)
	if s.eventBus == nil {
		return
	}
	eventPayload := lifecycle.AgentCapabilitiesEventPayload{
		TaskID:                  payload.TaskID,
		SessionID:               sessionID,
		AgentID:                 payload.AgentID,
		SupportsImage:           payload.Data.SupportsImage,
		SupportsAudio:           payload.Data.SupportsAudio,
		SupportsEmbeddedContext: payload.Data.SupportsEmbeddedContext,
		SupportsPromptQueueing:  payload.Data.SupportsPromptQueueing,
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
	if failures := workflowSessionConfigFailures(payload.Data.Data); len(failures) > 0 {
		stepID := ""
		if s.repo != nil && payload.TaskID != "" {
			if task, err := s.repo.GetTask(ctx, payload.TaskID); err == nil && task != nil {
				stepID = task.WorkflowStepID
			}
		}
		s.warnWorkflowSessionConfig(ctx, payload.TaskID, sessionID, stepID,
			fmt.Sprintf("Some session settings could not be applied at startup: %s.", strings.Join(failures, ", ")))
		return
	}
	if _, err := s.sessionOriginalEffectiveConfigurationForEvent(ctx, sessionID, payload.Data); err != nil {
		s.logger.Warn("failed to persist original effective session configuration",
			zap.String("session_id", sessionID),
			zap.Error(err))
	}
	settled := configOptionsSettled(payload.Data.Data)
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		s.logger.Warn("failed to load session before session model persistence",
			zap.String("session_id", sessionID),
			zap.Error(err))
		return
	}
	if shouldDeferUnsettledStartupModelsEvent(session, settled) {
		// The session state read is optimistic. A concurrent transition out of
		// STARTING can cause a conservative defer, and the next live model event
		// corrects the client state without introducing a lock-order dependency.
		s.logger.Debug("deferring unsettled startup session_models event",
			zap.String("session_id", sessionID),
			zap.String("current_model_id", payload.Data.CurrentModelID))
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
	s.persistSessionModelAndRuntimeConfigWithSettlement(
		ctx, sessionID, payload.Data.CurrentModelID, "", payload.Data.SessionModels, payload.Data.ConfigOptions, settled,
	)

	eventPayload := lifecycle.SessionModelsEventPayload{
		TaskID:               payload.TaskID,
		SessionID:            sessionID,
		AgentID:              payload.AgentID,
		CurrentModelID:       payload.Data.CurrentModelID,
		Models:               payload.Data.SessionModels,
		ConfigOptions:        payload.Data.ConfigOptions,
		ConfigOptionsSettled: settled,
		ConfigBaseline:       configBaseline,
		Timestamp:            time.Now().UTC().Format(time.RFC3339),
	}
	s.logger.Info("publishing session_models event to WS",
		zap.String("session_id", sessionID),
		zap.String("current_model_id", payload.Data.CurrentModelID),
		zap.Int("models_count", len(payload.Data.SessionModels)),
	)
	subject := events.BuildSessionModelsSubject(sessionID)
	_ = s.eventBus.Publish(ctx, subject, bus.NewEvent(events.SessionModelsUpdated, "orchestrator", eventPayload))
}

// handleSessionModelFallbackEvent broadcasts session_model_fallback events to
// the WebSocket so the UI can surface why the session is not on the
// configured start model (the profile's fallback was applied because the
// start model is unavailable).
func (s *Service) handleSessionModelFallbackEvent(ctx context.Context, payload *lifecycle.AgentStreamEventPayload) {
	sessionID := payload.SessionID
	// The fallback event fires during session init, before the execution's
	// task-session id is always linked. Resolve it from the task when the
	// payload carries no session id so the note is not dropped.
	if sessionID == "" && payload.TaskID != "" && s.repo != nil {
		if sess, err := s.repo.GetActiveTaskSessionByTaskID(ctx, payload.TaskID); err == nil && sess != nil {
			sessionID = sess.ID
		}
	}
	if sessionID == "" || s.eventBus == nil || payload.Data == nil {
		return
	}
	eventPayload := lifecycle.SessionModelFallbackEventPayload{
		TaskID:        payload.TaskID,
		SessionID:     sessionID,
		AgentID:       payload.AgentID,
		FallbackModel: payload.Data.FallbackModel,
		Timestamp:     time.Now().UTC().Format(time.RFC3339),
	}
	s.logger.Info("publishing session_model_fallback event to WS",
		zap.String("session_id", sessionID),
		zap.String("fallback_model", eventPayload.FallbackModel))
	subject := events.BuildSessionModelFallbackSubject(sessionID)
	_ = s.eventBus.Publish(ctx, subject, bus.NewEvent(events.SessionModelFallbackUpdated, "orchestrator", eventPayload))
}

// handleSessionModelSelectionWarningEvent persists one structured status
// message for an executor-authoritative model decision and publishes the same
// data to live WebSocket subscribers. Persistence is best-effort and never
// blocks the task launch.
func (s *Service) handleSessionModelSelectionWarningEvent(ctx context.Context, payload *lifecycle.AgentStreamEventPayload) {
	warning, sessionID, ok := s.modelSelectionWarningEvent(ctx, payload)
	if !ok {
		return
	}
	var releaseClaim func()
	if s.messageCreator != nil {
		var claimed bool
		releaseClaim, claimed = s.claimModelSelectionWarning(ctx, sessionID, warning.DecisionID)
		if !claimed {
			return
		}
	}
	if err := s.persistModelSelectionWarningMessage(ctx, payload.TaskID, sessionID, warning); err != nil {
		releaseClaim()
	}
	s.publishModelSelectionWarning(ctx, payload.TaskID, sessionID, warning)
}

func (s *Service) modelSelectionWarningEvent(
	ctx context.Context,
	payload *lifecycle.AgentStreamEventPayload,
) (streams.ModelSelectionWarning, string, bool) {
	if payload == nil || payload.Data == nil || payload.Data.ModelSelectionWarning == nil {
		return streams.ModelSelectionWarning{}, "", false
	}
	sessionID := payload.SessionID
	if sessionID == "" && payload.TaskID != "" && s.repo != nil {
		if sess, err := s.repo.GetActiveTaskSessionByTaskID(ctx, payload.TaskID); err == nil && sess != nil {
			sessionID = sess.ID
		}
	}
	if sessionID == "" {
		return streams.ModelSelectionWarning{}, "", false
	}
	return *payload.Data.ModelSelectionWarning, sessionID, true
}

func (s *Service) claimModelSelectionWarning(ctx context.Context, sessionID, decisionID string) (func(), bool) {
	if s.repo == nil || decisionID == "" {
		return func() {}, true
	}
	// A decision ID is created by lifecycle and is stable across event replay.
	// Use the structured metadata key as an atomic claim so two deliveries cannot
	// create duplicate status messages after a reconnect or restart.
	claimCtx := context.WithoutCancel(ctx)
	key := "model_selection_warning:" + decisionID
	if claimer, ok := s.repo.(failedSessionMetadataClaimer); ok {
		return s.claimModelSelectionWarningWithState(claimCtx, sessionID, key, claimer)
	}
	claimed, err := s.repo.SetSessionMetadataKeyIfAbsent(claimCtx, sessionID, key, true)
	if err != nil {
		s.logger.Warn("failed to claim model selection warning persistence",
			zap.String("session_id", sessionID), zap.Error(err))
		return func() {}, false
	}
	return func() {}, claimed
}

func (s *Service) claimModelSelectionWarningWithState(
	ctx context.Context,
	sessionID, key string,
	claimer failedSessionMetadataClaimer,
) (func(), bool) {
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil || session == nil {
		s.logger.Warn("failed to load session for model selection warning claim",
			zap.String("session_id", sessionID), zap.Error(err))
		return func() {}, false
	}
	claimed, err := claimer.SetSessionMetadataKeyIfAbsentIfState(ctx, sessionID, key, true, session.State)
	if err != nil {
		s.logger.Warn("failed to claim model selection warning persistence",
			zap.String("session_id", sessionID), zap.Error(err))
		return func() {}, false
	}
	if !claimed {
		return func() {}, false
	}
	return func() {
		s.releaseModelSelectionWarningClaim(ctx, sessionID, key, session.State)
	}, true
}

func (s *Service) releaseModelSelectionWarningClaim(
	ctx context.Context,
	sessionID, key string,
	expectedState models.TaskSessionState,
) {
	releaser, ok := s.repo.(failedSessionMetadataClaimReleaser)
	if !ok {
		s.logger.Warn("session repository cannot release model selection warning claim",
			zap.String("session_id", sessionID))
		return
	}
	if _, err := releaser.RemoveSessionMetadataKeyIfState(ctx, sessionID, key, expectedState); err != nil {
		s.logger.Warn("failed to release model selection warning claim",
			zap.String("session_id", sessionID), zap.Error(err))
	}
}

func modelSelectionWarningMetadata(warning streams.ModelSelectionWarning) map[string]interface{} {
	metadata := map[string]interface{}{
		"variant":             "warning",
		"kind":                warning.Kind,
		"reason":              warning.Reason,
		"requested_model":     warning.RequestedModel,
		"effective_model":     warning.EffectiveModel,
		"agent_id":            warning.AgentID,
		"executor_type":       warning.ExecutorType,
		"executor_profile_id": warning.ExecutorProfileID,
		"decision_id":         warning.DecisionID,
		"remediation":         []string{"executor_credentials", "copied_agent_configuration", "agent_version"},
	}
	if warning.FallbackModel != "" {
		metadata["fallback_model"] = warning.FallbackModel
	}
	return metadata
}

func (s *Service) persistModelSelectionWarningMessage(
	ctx context.Context,
	taskID, sessionID string,
	warning streams.ModelSelectionWarning,
) error {
	if s.messageCreator == nil {
		return nil
	}
	if err := s.messageCreator.CreateSessionMessage(
		ctx,
		taskID,
		"The executor could not use the saved model selection.",
		sessionID,
		string(v1.MessageTypeStatus),
		s.getActiveTurnID(sessionID),
		modelSelectionWarningMetadata(warning),
		false,
	); err != nil {
		s.logger.Warn("failed to persist model selection warning",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID), zap.Error(err))
		return err
	}
	return nil
}

func (s *Service) publishModelSelectionWarning(
	ctx context.Context,
	taskID, sessionID string,
	warning streams.ModelSelectionWarning,
) {
	if s.eventBus == nil {
		return
	}
	eventPayload := lifecycle.SessionModelSelectionWarningEventPayload{
		TaskID:    taskID,
		SessionID: sessionID,
		AgentID:   warning.AgentID,
		Warning:   warning,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
	subject := events.BuildSessionModelSelectionWarningSubject(sessionID)
	_ = s.eventBus.Publish(ctx, subject, bus.NewEvent(
		events.SessionModelSelectionWarningUpdated, "orchestrator", eventPayload,
	))
}

func workflowSessionConfigFailures(raw any) []string {
	data, ok := raw.(map[string]any)
	if !ok || data == nil {
		return nil
	}
	switch values := data["workflow_session_config_failures"].(type) {
	case []string:
		return values
	case []any:
		failures := make([]string, 0, len(values))
		for _, value := range values {
			if failure, ok := value.(string); ok && failure != "" {
				failures = append(failures, failure)
			}
		}
		return failures
	default:
		return nil
	}
}

func (s *Service) sessionOriginalEffectiveConfigurationForEvent(
	ctx context.Context,
	sessionID string,
	data *lifecycle.AgentStreamEventData,
) (*models.SessionOriginalEffectiveConfiguration, error) {
	if data == nil || !originalConfigSettled(data.Data) || s.repo == nil {
		return nil, nil
	}
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if session == nil || !models.IsOriginalTaskSession(session.Metadata) {
		return nil, nil
	}
	if existing, ok := models.LoadOriginalSessionEffectiveConfiguration(session.Metadata); ok {
		return &existing, nil
	}
	config := originalConfigurationFromEvent(data)
	if config.Model == "" && len(config.ConfigOptions) == 0 {
		return nil, nil
	}
	return s.persistOriginalSessionConfiguration(ctx, sessionID, session, config)
}

func originalConfigurationFromEvent(data *lifecycle.AgentStreamEventData) models.SessionOriginalEffectiveConfiguration {
	options := data.OriginalConfigCandidate
	if len(options) == 0 {
		options = data.ConfigOptions
	}
	return models.SessionOriginalEffectiveConfiguration{
		Model:         data.CurrentModelID,
		ConfigOptions: configOptionValuesWithoutModel(options),
	}
}

func (s *Service) persistOriginalSessionConfiguration(
	ctx context.Context,
	sessionID string,
	session *models.TaskSession,
	config models.SessionOriginalEffectiveConfiguration,
) (*models.SessionOriginalEffectiveConfiguration, error) {
	writeCtx := context.WithoutCancel(ctx)
	stored, err := s.repo.SetSessionMetadataKeyIfAbsent(
		writeCtx, sessionID, models.SessionMetaKeyOriginalEffectiveConfig, config,
	)
	if err != nil {
		return nil, err
	}
	if stored {
		return &config, nil
	}
	return s.originalSessionConfigurationAfterRace(writeCtx, sessionID, session, config)
}

func (s *Service) originalSessionConfigurationAfterRace(
	ctx context.Context,
	sessionID string,
	session *models.TaskSession,
	config models.SessionOriginalEffectiveConfiguration,
) (*models.SessionOriginalEffectiveConfiguration, error) {
	if existing, ok := models.LoadOriginalSessionEffectiveConfiguration(session.Metadata); ok {
		return &existing, nil
	}
	fresh, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if fresh == nil {
		return &config, nil
	}
	if existing, ok := models.LoadOriginalSessionEffectiveConfiguration(fresh.Metadata); ok {
		return &existing, nil
	}
	return &config, nil
}

// handleSessionMCPAttachmentEvent reduces safe attachment observations into
// bounded session metadata and broadcasts the resulting report. A failed
// diagnostic write is intentionally isolated from task and agent state.
func (s *Service) handleSessionMCPAttachmentEvent(ctx context.Context, payload *lifecycle.AgentStreamEventPayload) {
	if payload == nil || payload.Data == nil || payload.SessionID == "" || s.repo == nil {
		return
	}
	session, err := s.repo.GetTaskSession(ctx, payload.SessionID)
	if err != nil || session == nil {
		if err != nil {
			s.logger.Warn("failed to load session for MCP attachment evidence", zap.String("session_id", payload.SessionID), zap.Error(err))
		}
		return
	}
	if staleMCPAttachmentAttempt(payload) {
		s.logger.Warn("dropping stale MCP attachment attempt in orchestrator",
			zap.String("attempt_execution_id", payload.Data.MCPAttachmentAttempt.ExecutionID),
			zap.String("event_execution_id", payload.ExecutionID))
		return
	}
	history, _ := lifecycle.LoadMCPAttachmentHistory(session.Metadata[models.SessionMetaKeyMCPAttachmentState])
	changed := reduceMCPAttachmentHistory(&history, payload.Data)
	if !changed || !history.Valid() {
		return
	}
	writeCtx := context.WithoutCancel(ctx)
	if err := s.repo.SetSessionMetadataKey(writeCtx, payload.SessionID, models.SessionMetaKeyMCPAttachmentState, history); err != nil {
		s.logger.Warn("failed to persist MCP attachment status", zap.String("session_id", payload.SessionID), zap.Error(err))
		return
	}
	eventPayload := lifecycle.SessionMCPStatusEventPayload{
		TaskID: payload.TaskID, SessionID: payload.SessionID, History: history,
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
	}
	if s.eventBus != nil {
		_ = s.eventBus.Publish(writeCtx, events.BuildSessionMCPStatusSubject(payload.SessionID), bus.NewEvent(events.SessionMCPStatusUpdated, "orchestrator", eventPayload))
	}
}

func staleMCPAttachmentAttempt(payload *lifecycle.AgentStreamEventPayload) bool {
	attempt := payload.Data.MCPAttachmentAttempt
	return attempt != nil && attempt.ExecutionID != "" && attempt.ExecutionID != payload.ExecutionID
}

func reduceMCPAttachmentHistory(history *streams.MCPAttachmentHistory, data *lifecycle.AgentStreamEventData) bool {
	changed := false
	if attempt := data.MCPAttachmentAttempt; attempt != nil {
		history.StartAttempt(*attempt)
		changed = true
	}
	if evidence := data.MCPAttachment; evidence != nil {
		changed = history.Apply(*evidence) || changed
	}
	return changed
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

func configOptionValuesWithoutModel(options []streams.ConfigOption) map[string]string {
	values := configOptionValues(options)
	for key, option := range optionsByID(options) {
		// The immutable restore snapshot contains only selectable ACP options.
		// Toggle/boolean values are provider state, not model configuration, and
		// may be invalid when replayed after a model switch.
		if option.Type != "select" || key == sessionModelConfigKey || option.Category == sessionModelConfigKey {
			delete(values, key)
		}
	}
	if len(values) == 0 {
		return nil
	}
	return values
}

func optionsByID(options []streams.ConfigOption) map[string]streams.ConfigOption {
	result := make(map[string]streams.ConfigOption, len(options))
	for _, option := range options {
		if option.ID != "" {
			result[option.ID] = option
		}
	}
	return result
}

func configOptionsSettled(data any) bool {
	metadata, _ := data.(map[string]any)
	result, _ := metadata["config_options_settled"].(bool)
	return result
}

func shouldDeferUnsettledStartupModelsEvent(session *models.TaskSession, settled bool) bool {
	return !settled && session != nil && session.State == models.TaskSessionStateStarting
}

func originalConfigSettled(data any) bool {
	metadata, _ := data.(map[string]any)
	result, _ := metadata["original_config_settled"].(bool)
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
	s.persistSessionModelAndRuntimeConfigWithSettlement(
		ctx, sessionID, model, mode, availableModels, options, false,
	)
}

func (s *Service) persistSessionModelAndRuntimeConfigWithSettlement(
	ctx context.Context,
	sessionID, model, mode string,
	availableModels []streams.SessionModelInfo,
	options []streams.ConfigOption,
	configOptionsSettled bool,
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
	s.persistSessionModelsSnapshot(ctx, sessionID, session, model, availableModels, options, configOptionsSettled)
}

func (s *Service) persistSessionModelsSnapshot(
	ctx context.Context,
	sessionID string,
	session *models.TaskSession,
	currentModelID string,
	availableModels []streams.SessionModelInfo,
	options []streams.ConfigOption,
	configOptionsSettled bool,
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
		CurrentModelID:       currentModelID,
		Models:               modelsForBoot,
		ConfigOptions:        options,
		ConfigOptionsSettled: configOptionsSettled,
	}
	if previous, ok := lifecycle.LoadSessionModelsSnapshot(session.Metadata[models.SessionMetaKeyACPModelState]); ok {
		snapshot.ConfigOptionsSettled = snapshot.ConfigOptionsSettled || previous.ConfigOptionsSettled
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
		if err := s.clearContextWindowForReset(writeCtx, sessionID); err != nil {
			s.logger.Warn("failed to clear stale context window after runtime model change",
				zap.String("session_id", sessionID),
				zap.String("previous_model", previousModel),
				zap.String(sessionModelConfigKey, cfg.Model),
				zap.Error(err))
		}
	}
}

func (s *Service) persistSessionModelOnSession(ctx context.Context, sessionID string, session *models.TaskSession, model string) {
	if session.AgentProfileSnapshot == nil {
		session.AgentProfileSnapshot = make(map[string]interface{})
	}
	if existing, _ := session.AgentProfileSnapshot[sessionModelConfigKey].(string); existing == model {
		return
	}
	session.AgentProfileSnapshot[sessionModelConfigKey] = model
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
		if option.ID == sessionModelConfigKey || option.Category == sessionModelConfigKey {
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
//
// The update is qualified by RequestID as well as PendingID: a provider may
// reuse pending_id for a later, unrelated request once the original is
// resolved, and this event can arrive after that happens (agentctl's
// ctx.Done() cancellation path races the handler goroutine's own teardown).
// Matching RequestID too keeps a delayed cancellation from expiring the new
// request's message.
func (s *Service) handlePermissionCancelledEvent(ctx context.Context, payload *lifecycle.AgentStreamEventPayload) {
	sessionID := payload.SessionID
	if sessionID == "" || payload.Data.PendingID == "" || s.messageCreator == nil {
		return
	}
	if err := s.messageCreator.UpdatePermissionMessage(ctx, payload.TaskID, sessionID, payload.Data.RequestID, payload.Data.PendingID, models.PermissionStatusExpired); err != nil {
		s.logger.Warn("failed to mark permission as expired",
			zap.String("session_id", sessionID),
			zap.String("request_id", payload.Data.RequestID),
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
