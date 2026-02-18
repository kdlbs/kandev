package lifecycle

import (
	"strings"
	"time"

	"go.uber.org/zap"

	agentctl "github.com/kandev/kandev/internal/agentctl/client"
)

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

// handleCompleteEventMarkState marks the execution state after a complete event:
// failed+removed on error, ready on success.
func (m *Manager) handleCompleteEventMarkState(execution *AgentExecution, event *agentctl.AgentEvent, isError bool) {
	if isError {
		errorMsg := "agent error completion"
		if event.Error != "" {
			errorMsg = event.Error
		}
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
		m.RemoveExecution(execution.ID)
		return
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
		errorMsg = "agent error completion"
		if event.Error != "" {
			errorMsg = event.Error
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
		if m.historyManager != nil && execution.SessionID != "" {
			if err := m.historyManager.AppendAgentMessage(execution.SessionID, flushedText); err != nil {
				m.logger.Warn("failed to store final agent message to history", zap.Error(err))
			}
		}
	}

	m.logger.Info("complete event processed",
		zap.String("execution_id", execution.ID),
		zap.String("operation_id", event.OperationID))

	m.handleCompleteEventMarkState(execution, event, isError)
	handleCompleteEventSignal(execution, event, isError)
}

// handleToolCallEvent processes the "tool_call" agent event: flushes the message buffer
// and stores the tool call in session history.
// Returns the (possibly updated) event.
func (m *Manager) handleToolCallEvent(execution *AgentExecution, event agentctl.AgentEvent) agentctl.AgentEvent {
	if flushedText := m.flushMessageBuffer(execution); flushedText != "" {
		event.Text = flushedText
		if m.historyManager != nil && execution.SessionID != "" {
			if err := m.historyManager.AppendAgentMessage(execution.SessionID, flushedText); err != nil {
				m.logger.Warn("failed to store agent message to history", zap.Error(err))
			}
		}
	}
	if m.historyManager != nil && execution.SessionID != "" {
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

// handleAgentEvent processes incoming agent events from the agent
func (m *Manager) handleAgentEvent(execution *AgentExecution, event agentctl.AgentEvent) {
	// Update last activity timestamp for stall detection
	execution.lastActivityAtMu.Lock()
	execution.lastActivityAt = time.Now()
	execution.lastActivityAtMu.Unlock()

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
		if m.historyManager != nil && execution.SessionID != "" && event.ToolStatus == "complete" {
			if err := m.historyManager.AppendToolResult(execution.SessionID, event); err != nil {
				m.logger.Warn("failed to store tool result to history", zap.Error(err))
			}
		}

	case "plan":
		m.logger.Debug("agent plan update",
			zap.String("execution_id", execution.ID))

	case "error":
		m.flushMessageBuffer(execution)
		m.logger.Error("agent error",
			zap.String("execution_id", execution.ID),
			zap.String("error", event.Error),
			zap.String("text", event.Text),
			zap.Any("data", event.Data))

	case "complete":
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

