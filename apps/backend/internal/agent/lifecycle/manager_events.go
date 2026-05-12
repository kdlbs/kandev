package lifecycle

import (
	"context"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.uber.org/zap"

	agentctl "github.com/kandev/kandev/internal/agentctl/client"
	"github.com/kandev/kandev/internal/agentctl/tracing"
	"github.com/kandev/kandev/internal/events"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

const toolStatusComplete = "complete"

// handleMessageChunkEvent handles a "message_chunk" agent event, accumulating and flushing on newlines.
func (m *Manager) handleMessageChunkEvent(execution *AgentExecution, event agentctl.AgentEvent) {
	if event.Text == "" {
		return
	}
	execution.messageMu.Lock()
	execution.messageBuffer.WriteString(event.Text)
	bufferLenAfterWrite := execution.messageBuffer.Len()
	m.logger.Debug("message_chunk written to buffer",
		zap.String("execution_id", execution.ID),
		zap.String("operation_id", event.OperationID),
		zap.Int("text_length", len(event.Text)),
		zap.Int("buffer_length_after", bufferLenAfterWrite))

	bufContent := execution.messageBuffer.String()
	lastNewline := strings.LastIndex(bufContent, "\n")
	if lastNewline == -1 {
		execution.messageMu.Unlock()
		return
	}
	toFlush := bufContent[:lastNewline+1]
	remainder := bufContent[lastNewline+1:]
	execution.messageBuffer.Reset()
	execution.messageBuffer.WriteString(remainder)
	execution.messageMu.Unlock()

	if strings.TrimSpace(toFlush) != "" {
		m.publishStreamingMessage(execution, toFlush)
	}
}

// handleReasoningEvent handles a "reasoning" agent event, accumulating and flushing on newlines.
func (m *Manager) handleReasoningEvent(execution *AgentExecution, event agentctl.AgentEvent) {
	if event.ReasoningText == "" {
		return
	}
	execution.messageMu.Lock()
	execution.thinkingBuffer.WriteString(event.ReasoningText)

	bufContent := execution.thinkingBuffer.String()
	lastNewline := strings.LastIndex(bufContent, "\n")
	if lastNewline == -1 {
		execution.messageMu.Unlock()
		return
	}
	toFlush := bufContent[:lastNewline+1]
	remainder := bufContent[lastNewline+1:]
	execution.thinkingBuffer.Reset()
	execution.thinkingBuffer.WriteString(remainder)
	execution.messageMu.Unlock()

	if strings.TrimSpace(toFlush) != "" {
		m.publishStreamingThinking(execution, toFlush)
	}
}

// extractErrorMessage returns the best error message from an agent event.
// Priority: Error field > Text field > default message.
func extractErrorMessage(event *agentctl.AgentEvent) string {
	if event.Error != "" {
		return event.Error
	}
	if event.Text != "" {
		return event.Text
	}
	return "agent error completion"
}

// handleCompleteEventMarkState marks the execution state after a complete event:
// failed+removed on error, ready on success.
func (m *Manager) handleCompleteEventMarkState(execution *AgentExecution, event *agentctl.AgentEvent, isError bool) {
	if isError {
		errorMsg := extractErrorMessage(event)
		m.logger.Warn("error completion received, marking execution as failed",
			zap.String("execution_id", execution.ID),
			zap.String("task_id", execution.TaskID),
			zap.String("error", errorMsg),
			zap.String("event_error", event.Error),
			zap.String("event_text", event.Text),
			zap.Any("event_data", event.Data),
			zap.String("agent_command", execution.AgentCommand),
			zap.String("acp_session_id", execution.ACPSessionID))
		if err := m.MarkCompleted(execution.ID, 1, errorMsg); err != nil {
			m.logger.Error("failed to mark execution as failed after error completion",
				zap.String("execution_id", execution.ID),
				zap.Error(err))
		}
		return
	}
	// Empty wakeup turn fallback: if the wakeup-driven turn produced no
	// turn-content events (no message_chunk/tool_call/etc), recordActivity
	// won't have flipped Ready → Running. MarkReady would then early-return
	// on the Ready guard and silently drop AgentReady — same suppression
	// the wakeup fix is designed to prevent.
	//
	// Publishing AgentReady alone is not enough: the orchestrator's
	// handleAgentReady (`event_handlers_agent.go:205`) ignores AgentReady
	// when session.State is not Running/Starting, and after the previous
	// turn ended the session is WaitingForInput. We mirror what
	// recordActivity does for the non-empty case — flip Ready → Running
	// and publish AgentRunning first, so the orchestrator's session state
	// catches up — then let MarkReady do its normal Running → Ready
	// transition and publish AgentReady.
	if execution.Status == v1.AgentStatusReady {
		m.logger.Info("flipping Ready→Running for empty wakeup turn before publishing AgentReady",
			zap.String("execution_id", execution.ID),
			zap.String("session_id", execution.SessionID))
		if m.executionStore != nil {
			m.executionStore.UpdateStatus(execution.ID, v1.AgentStatusRunning)
		}
		m.eventPublisher.PublishAgentEvent(context.Background(), events.AgentRunning, execution)
	}
	if err := m.MarkReady(execution.ID); err != nil {
		m.logger.Error("failed to mark execution as ready after complete",
			zap.String("execution_id", execution.ID),
			zap.Error(err))
	}
}

// handleCompleteEventSignal sends the completion signal on the promptDoneCh channel.
func handleCompleteEventSignal(execution *AgentExecution, event *agentctl.AgentEvent, isError bool) {
	stopReason := "end_turn"
	errorMsg := ""
	if isError {
		stopReason = "error"
		errorMsg = extractErrorMessage(event)
	} else if event.Data != nil {
		// Read StopReason from the complete event (set by ACP adapter from PromptResponse)
		if sr, ok := event.Data["stop_reason"].(string); ok && sr != "" {
			stopReason = sr
		}
	}
	select {
	case execution.promptDoneCh <- PromptCompletionSignal{
		StopReason: stopReason,
		IsError:    isError,
		Error:      errorMsg,
	}:
	default:
		// Channel full or no one waiting — that's fine (e.g., initial prompt in goroutine)
	}
}

// handleCompleteEvent handles a "complete" agent event: flushes buffers, marks state, and signals SendPrompt.
func (m *Manager) handleCompleteEvent(execution *AgentExecution, event *agentctl.AgentEvent) {
	// Check buffer content BEFORE any processing
	execution.messageMu.Lock()
	bufferContentBeforeFlush := execution.messageBuffer.String()
	currentMsgID := execution.currentMessageID
	execution.messageMu.Unlock()

	bufferPreview := bufferContentBeforeFlush
	if len(bufferPreview) > 100 {
		bufferPreview = bufferPreview[:100] + "..."
	}

	// Check if this is an error completion (agent failed to process the prompt)
	isError := false
	if event.Data != nil {
		if v, ok := event.Data["is_error"].(bool); ok {
			isError = v
		}
	}

	// Determine stop reason for tracing
	stopReason := "end_turn"
	if isError {
		stopReason = "error"
	} else if event.Data != nil {
		if sr, ok := event.Data["stop_reason"].(string); ok && sr != "" {
			stopReason = sr
		}
	}

	// Create a turn_end span on the session trace
	_, turnSpan := tracing.TraceTurnEnd(execution.SessionTraceContext(), execution.ID, execution.SessionID)
	turnSpan.SetAttributes(
		attribute.String("stop_reason", stopReason),
		attribute.Bool("is_error", isError),
	)
	turnSpan.End()

	m.logger.Info("agent turn complete",
		zap.String("execution_id", execution.ID),
		zap.String("operation_id", event.OperationID),
		zap.String("session_id", event.SessionID),
		zap.String("current_msg_id", currentMsgID),
		zap.Int("buffer_length", len(bufferContentBeforeFlush)),
		zap.String("buffer_preview", bufferPreview),
		zap.Bool("is_error", isError))

	// Flush the message buffer to publish any remaining content as a streaming message.
	flushedText := m.flushMessageBuffer(execution)
	if flushedText != "" {
		event.Text = flushedText
		if m.historyManager != nil && execution.historyEnabled && execution.SessionID != "" {
			if err := m.historyManager.AppendAgentMessage(execution.SessionID, flushedText); err != nil {
				m.logger.Warn("failed to store final agent message to history", zap.Error(err))
			}
		}
	}

	m.logger.Info("complete event processed",
		zap.String("execution_id", execution.ID),
		zap.String("operation_id", event.OperationID))

	// Signal promptDoneCh BEFORE publishing agent.ready via MarkReady.
	// MarkReady publishes agent.ready synchronously, which triggers handleAgentReady
	// in the orchestrator. If handleAgentReady launches a follow-up prompt (queued
	// message), the new SendPrompt drains promptDoneCh. If we signal AFTER MarkReady,
	// the drain races with the first SendPrompt's receive and can steal the signal,
	// leaving the first SendPrompt hung and the second prompt's completion event
	// never reaching the event bus.
	handleCompleteEventSignal(execution, event, isError)
	m.handleCompleteEventMarkState(execution, event, isError)
}

// handleToolCallEvent processes the "tool_call" agent event: flushes the message buffer
// and stores the tool call in session history.
// Returns the (possibly updated) event.
func (m *Manager) handleToolCallEvent(execution *AgentExecution, event agentctl.AgentEvent) agentctl.AgentEvent {
	if flushedText := m.flushMessageBuffer(execution); flushedText != "" {
		event.Text = flushedText
		if m.historyManager != nil && execution.historyEnabled && execution.SessionID != "" {
			if err := m.historyManager.AppendAgentMessage(execution.SessionID, flushedText); err != nil {
				m.logger.Warn("failed to store agent message to history", zap.Error(err))
			}
		}
	}
	if m.historyManager != nil && execution.historyEnabled && execution.SessionID != "" {
		if err := m.historyManager.AppendToolCall(execution.SessionID, event); err != nil {
			m.logger.Warn("failed to store tool call to history", zap.Error(err))
		}
	}
	m.logger.Debug("tool call started",
		zap.String("execution_id", execution.ID),
		zap.String("tool_call_id", event.ToolCallID),
		zap.String("tool_name", event.ToolName))
	return event
}

// handleToolUpdateEvent stores completed tool results in session history.
func (m *Manager) handleToolUpdateEvent(execution *AgentExecution, event agentctl.AgentEvent) {
	if m.historyManager != nil && execution.historyEnabled && execution.SessionID != "" && event.ToolStatus == toolStatusComplete {
		if err := m.historyManager.AppendToolResult(execution.SessionID, event); err != nil {
			m.logger.Warn("failed to store tool result to history", zap.Error(err))
		}
	}
}

// handleErrorEvent processes the "error" agent event: flushes buffers, marks state as failed,
// and signals prompt completion. The raw error event is not published to the frontend stream;
// the agent failure path (handleAgentFailed) sets session FAILED with the error message.
func (m *Manager) handleErrorEvent(execution *AgentExecution, event agentctl.AgentEvent) {
	m.flushMessageBuffer(execution)
	m.logger.Error("agent error",
		zap.String("execution_id", execution.ID),
		zap.String("error", event.Error),
		zap.String("text", event.Text),
		zap.Any("data", event.Data))
	m.handleCompleteEventMarkState(execution, &event, true)
	handleCompleteEventSignal(execution, &event, true)
}

// handleContextWindowEvent processes the "context_window" agent event: logs and publishes it.
// Returns true because no further stream publishing is needed.
func (m *Manager) handleContextWindowEvent(execution *AgentExecution, event agentctl.AgentEvent) {
	m.logger.Debug("context window update received",
		zap.String("execution_id", execution.ID),
		zap.Int64("size", event.ContextWindowSize),
		zap.Int64("used", event.ContextWindowUsed),
		zap.Float64("efficiency", event.ContextEfficiency))
	m.eventPublisher.PublishContextWindow(
		execution,
		event.ContextWindowSize,
		event.ContextWindowUsed,
		event.ContextWindowRemaining,
		event.ContextEfficiency,
	)
}

// handleAvailableCommandsEvent processes the "available_commands" agent event.
func (m *Manager) handleAvailableCommandsEvent(execution *AgentExecution, event agentctl.AgentEvent) {
	if len(event.AvailableCommands) == 0 {
		return
	}
	execution.SetAvailableCommands(event.AvailableCommands)
	m.logger.Debug("stored available commands",
		zap.String("execution_id", execution.ID),
		zap.String("session_id", execution.SessionID),
		zap.Int("command_count", len(event.AvailableCommands)))
	m.eventPublisher.PublishAvailableCommands(execution, event.AvailableCommands)
}

// turnContentEventTypes is the set of agent event types that unambiguously
// signal a real turn is in progress — assistant text, reasoning, tool work,
// plan/permission updates. These are the only events that drive the
// Ready → Running flip for wakeup-driven turns; boot/metadata events
// (agent_capabilities, available_commands, session_mode/models/status,
// context_window, etc.) can arrive *after* MarkBootReady has put the
// execution into Ready (e.g. claude-agent-acp emits available_commands
// asynchronously ~50ms after session/new, well after dispatchInitialPrompt
// has fired MarkBootReady for a no-prompt task) and must NOT be treated
// as turn starts. Terminal events (complete/error) are excluded too —
// they own their own status transitions via handleCompleteEvent and a
// dedicated empty-turn fallback in handleCompleteEventMarkState.
var turnContentEventTypes = map[string]struct{}{
	"message_chunk":      {},
	"reasoning":          {},
	"tool_call":          {},
	"tool_update":        {},
	"plan":               {},
	"agent_plan":         {},
	"permission_request": {},
}

// recordActivity updates the last-activity timestamp and, on the very first
// event from an execution, publishes AgentRunning to transition STARTING → RUNNING.
//
// Wakeup-driven turns also flip Ready → Running here. The adapter's wakeup
// scheduler fires synthetic prompts directly via Adapter.Prompt, bypassing
// SessionManager.SendPrompt — so nothing on the lifecycle side flips the
// execution back to Running for those turns. Without that flip, MarkReady's
// duplicate-suppression guard (manager_interaction.go:896 — early-returns
// when execution.Status is already Ready) silently drops the wakeup turn's
// AgentReady event, the orchestrator never calls completeTurnForSession,
// and workflow on_turn_complete + queued-message dispatch silently break.
//
// The flip is gated on `turnContentEventTypes` so post-boot metadata events
// (available_commands_update arriving 50ms after MarkBootReady, etc.) don't
// accidentally re-arm a freshly-booted no-prompt session as Running.
func (m *Manager) recordActivity(execution *AgentExecution, event agentctl.AgentEvent) {
	execution.lastActivityAtMu.Lock()
	execution.lastActivityAt = time.Now()
	execution.lastActivityAtMu.Unlock()

	// Gate firstActivityOnce on `Status != Ready` so a delayed metadata
	// event arriving after MarkBootReady can't accidentally fire
	// AgentRunning. In practice the adapter always emits agent_capabilities
	// during Initialize (before MarkBootReady), so firstActivityOnce fires
	// while Status is still Running — this is defensive hardening.
	if execution.Status != v1.AgentStatusReady {
		execution.firstActivityOnce.Do(func() {
			m.eventPublisher.PublishAgentEvent(context.Background(), events.AgentRunning, execution)
		})
		return
	}
	if m.executionStore == nil {
		return
	}
	if _, ok := turnContentEventTypes[event.Type]; !ok {
		return
	}
	m.executionStore.UpdateStatus(execution.ID, v1.AgentStatusRunning)
	m.logger.Info("wakeup-driven turn detected; flipping execution back to Running",
		zap.String("execution_id", execution.ID),
		zap.String("session_id", execution.SessionID),
		zap.String("trigger_event_type", event.Type))
	m.eventPublisher.PublishAgentEvent(context.Background(), events.AgentRunning, execution)
}

// handleStreamDisconnect handles unexpected updates stream disconnections.
// It proactively updates execution status and publishes an error event so the
// orchestrator can transition the session state without waiting for a future prompt.
func (m *Manager) handleStreamDisconnect(execution *AgentExecution, err error) {
	m.logger.Warn("agent updates stream disconnected",
		zap.String("execution_id", execution.ID),
		zap.String("session_id", execution.SessionID),
		zap.Error(err))

	if m.executionStore != nil {
		m.executionStore.UpdateStatus(execution.ID, v1.AgentStatusFailed)
	}

	m.eventPublisher.PublishAgentctlEvent(
		context.Background(), events.AgentctlError, execution,
		"agent stream disconnected: "+err.Error(),
	)
}

// handleAgentEvent processes incoming agent events from the agent
func (m *Manager) handleAgentEvent(execution *AgentExecution, event agentctl.AgentEvent) {
	m.recordActivity(execution, event)

	m.logger.Debug("handleAgentEvent entry",
		zap.String("execution_id", execution.ID),
		zap.String("event_type", event.Type),
		zap.String("operation_id", event.OperationID),
		zap.Int("text_length", len(event.Text)))

	switch event.Type {
	case "message_chunk":
		m.handleMessageChunkEvent(execution, event)
		return

	case "reasoning":
		m.handleReasoningEvent(execution, event)
		return

	case "tool_call":
		event = m.handleToolCallEvent(execution, event)

	case "tool_update":
		m.handleToolUpdateEvent(execution, event)

	case "plan":
		m.logger.Debug("agent plan update",
			zap.String("execution_id", execution.ID))

	case "error":
		m.handleErrorEvent(execution, event)
		return

	case toolStatusComplete:
		m.handleCompleteEvent(execution, &event)

	case "permission_request":
		m.logger.Debug("permission request received",
			zap.String("execution_id", execution.ID),
			zap.String("pending_id", event.PendingID),
			zap.String("title", event.PermissionTitle))
		m.eventPublisher.PublishPermissionRequest(execution, event)
		return

	case "context_window":
		m.handleContextWindowEvent(execution, event)
		return

	case "available_commands":
		m.handleAvailableCommandsEvent(execution, event)
		return

	case "agent_capabilities":
		if len(event.AuthMethods) > 0 {
			execution.SetAuthMethods(event.AuthMethods)
		}

	case "session_mode":
		execution.SetModeState(&CachedModeState{
			CurrentModeID:  event.CurrentModeID,
			AvailableModes: event.AvailableModes,
		})
		// No return — must flow through to PublishAgentStreamEvent so the orchestrator
		// can filter and re-publish to the dedicated session mode subject.

	case "session_models":
		execution.SetModelState(&CachedModelState{
			CurrentModelID: event.CurrentModelID,
			Models:         event.SessionModels,
			ConfigOptions:  event.ConfigOptions,
		})
		// No return — must flow through to PublishAgentStreamEvent so the orchestrator
		// can persist the model and re-publish to the dedicated session models subject.
	}

	m.eventPublisher.PublishAgentStreamEvent(execution, event)
}

// handleGitStatusUpdate processes git status updates from the workspace tracker
func (m *Manager) handleGitStatusUpdate(execution *AgentExecution, update *agentctl.GitStatusUpdate) {
	// Publish git status update to event bus for WebSocket streaming and persistence
	m.eventPublisher.PublishGitStatus(execution, update)
}

// handleGitCommitCreated processes git commit events from the workspace tracker
func (m *Manager) handleGitCommitCreated(execution *AgentExecution, commit *agentctl.GitCommitNotification) {
	// Publish commit event to event bus for WebSocket streaming and orchestrator handling
	m.eventPublisher.PublishGitCommit(execution, commit)
}

// handleGitResetDetected processes git reset events from the workspace tracker
func (m *Manager) handleGitResetDetected(execution *AgentExecution, reset *agentctl.GitResetNotification) {
	// Publish reset event to event bus for orchestrator handling (commit sync)
	m.eventPublisher.PublishGitReset(execution, reset)
}

// handleBranchSwitch processes branch switch events from the workspace tracker
func (m *Manager) handleBranchSwitch(execution *AgentExecution, branchSwitch *agentctl.GitBranchSwitchNotification) {
	// Publish branch switch event to event bus for orchestrator handling (base commit update)
	m.eventPublisher.PublishBranchSwitch(execution, branchSwitch)
}

// handleFileChangeNotification processes file change notifications from the workspace tracker
func (m *Manager) handleFileChangeNotification(execution *AgentExecution, notification *agentctl.FileChangeNotification) {
	m.eventPublisher.PublishFileChange(execution, notification)
}

// handleShellOutput processes shell output from the workspace stream
func (m *Manager) handleShellOutput(execution *AgentExecution, data string) {
	m.eventPublisher.PublishShellOutput(execution, data)
}

// handleProcessOutput processes script process output from the workspace stream
func (m *Manager) handleProcessOutput(execution *AgentExecution, output *agentctl.ProcessOutput) {
	if output == nil {
		return
	}
	m.logger.Debug("lifecycle received process output",
		zap.String("session_id", output.SessionID),
		zap.String("process_id", output.ProcessID),
		zap.String("kind", string(output.Kind)),
		zap.String("stream", output.Stream),
		zap.Int("bytes", len(output.Data)),
	)
	m.eventPublisher.PublishProcessOutput(execution, output)
}

// handleProcessStatus processes script process status updates from the workspace stream
func (m *Manager) handleProcessStatus(execution *AgentExecution, status *agentctl.ProcessStatusUpdate) {
	if status == nil {
		return
	}
	m.logger.Debug("lifecycle received process status",
		zap.String("session_id", status.SessionID),
		zap.String("process_id", status.ProcessID),
		zap.String("status", string(status.Status)),
	)
	m.eventPublisher.PublishProcessStatus(execution, status)
}

// handleShellExit processes shell exit events from the workspace stream
func (m *Manager) handleShellExit(execution *AgentExecution, code int) {
	m.eventPublisher.PublishShellExit(execution, code)
}
