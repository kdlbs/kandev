package lifecycle

import (
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
)

const thinkingStreamingEventType = "thinking_streaming"

func (e *AgentExecution) clearProtocolMessageCorrelationLocked() {
	e.protocolMessageIDs = nil
	e.protocolThinkingIDs = nil
}

func (e *AgentExecution) resetStreamingStateLocked() {
	e.messageBuffer.Reset()
	e.thinkingBuffer.Reset()
	e.assistantHistoryBuffer.Reset()
	e.currentMessageID = ""
	e.currentThinkingID = ""
	e.clearProtocolMessageCorrelationLocked()
}

func protocolRecordID(ids *map[string]string, protocolMessageID string) (string, bool) {
	if *ids == nil {
		*ids = make(map[string]string)
	}
	if messageID, ok := (*ids)[protocolMessageID]; ok {
		return messageID, true
	}
	messageID := uuid.New().String()
	(*ids)[protocolMessageID] = messageID
	return messageID, false
}

func (m *Manager) publishProtocolMessage(
	execution *AgentExecution,
	protocolMessageID string,
	content string,
) {
	execution.messageMu.Lock()
	messageID, isAppend := protocolRecordID(&execution.protocolMessageIDs, protocolMessageID)
	execution.messageMu.Unlock()

	m.publishStreamingContent(execution, "message_streaming", messageID, content, isAppend)
}

func (m *Manager) appendAssistantHistoryChunk(execution *AgentExecution, content string) {
	execution.messageMu.Lock()
	execution.assistantHistoryBuffer.WriteString(content)
	execution.messageMu.Unlock()
}

// flushAssistantHistory persists the assistant text observed since the prior
// history boundary. It is independent from visible-stream flushing so a
// subagent tool can retain UI nesting while history still follows wire order.
func (m *Manager) flushAssistantHistory(execution *AgentExecution) {
	flushAssistantHistory(execution, m.historyManager, m.logger)
}

// flushAssistantHistory drains the pending assistant segment exactly once.
// Draining also deliberately discards the segment when history recording is
// unavailable or disabled, so stale text cannot leak into a later prompt.
func flushAssistantHistory(
	execution *AgentExecution,
	historyManager *SessionHistoryManager,
	log *logger.Logger,
) {
	execution.messageMu.Lock()
	content := execution.assistantHistoryBuffer.String()
	execution.assistantHistoryBuffer.Reset()
	execution.messageMu.Unlock()

	if content == "" || historyManager == nil || !execution.historyEnabled || execution.SessionID == "" {
		return
	}
	if err := historyManager.AppendAgentMessage(execution.SessionID, content); err != nil {
		log.Warn("failed to store agent message to history", zap.Error(err))
	}
}

// flushPendingLegacyMessage closes only the ID-less assistant stream. It is
// used when an explicit protocol message begins so buffered legacy content is
// published first and a later ID-less chunk cannot append across that boundary.
func (m *Manager) flushPendingLegacyMessage(execution *AgentExecution) {
	execution.messageMu.Lock()
	content := execution.messageBuffer.String()
	execution.messageBuffer.Reset()
	messageID := execution.currentMessageID
	execution.currentMessageID = ""
	execution.messageMu.Unlock()

	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return
	}
	if messageID != "" {
		m.publishStreamingMessageFinal(execution, messageID, trimmed)
		return
	}
	m.publishStreamingMessage(execution, trimmed)
	execution.messageMu.Lock()
	execution.currentMessageID = ""
	execution.messageMu.Unlock()
}

// flushPendingLegacyThinking is the reasoning-stream counterpart of
// flushPendingLegacyMessage.
func (m *Manager) flushPendingLegacyThinking(execution *AgentExecution) {
	execution.messageMu.Lock()
	content := execution.thinkingBuffer.String()
	execution.thinkingBuffer.Reset()
	messageID := execution.currentThinkingID
	execution.currentThinkingID = ""
	execution.messageMu.Unlock()

	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return
	}
	if messageID != "" {
		m.publishStreamingThinkingFinal(execution, messageID, trimmed)
		return
	}
	m.publishStreamingThinking(execution, trimmed)
	execution.messageMu.Lock()
	execution.currentThinkingID = ""
	execution.messageMu.Unlock()
}

func (m *Manager) publishProtocolThinking(
	execution *AgentExecution,
	protocolMessageID string,
	content string,
) {
	execution.messageMu.Lock()
	messageID, isAppend := protocolRecordID(&execution.protocolThinkingIDs, protocolMessageID)
	execution.messageMu.Unlock()

	m.publishStreamingContent(execution, thinkingStreamingEventType, messageID, content, isAppend)
}

func (m *Manager) publishStreamingContent(
	execution *AgentExecution,
	eventType string,
	messageID string,
	content string,
	isAppend bool,
) {
	event := AgentStreamEventData{
		Type:      eventType,
		Text:      content,
		MessageID: messageID,
		IsAppend:  isAppend,
	}
	if eventType == thinkingStreamingEventType {
		event.MessageType = "thinking"
	}

	payload := &AgentStreamEventPayload{
		Type:        "agent/event",
		Timestamp:   time.Now().UTC().Format(time.RFC3339Nano),
		AgentID:     execution.ID,
		ExecutionID: execution.ID,
		TaskID:      execution.TaskID,
		SessionID:   execution.SessionID,
		Data:        &event,
	}
	m.eventPublisher.PublishAgentStreamEventPayload(payload)
}

// publishStreamingMessage publishes a streaming message event for real-time text updates.
// It creates a new message on first call (currentMessageID empty) or appends to existing.
// The message ID is generated and set synchronously to avoid race conditions.
func (m *Manager) publishStreamingMessage(execution *AgentExecution, content string) {
	execution.messageMu.Lock()
	isAppend := execution.currentMessageID != ""
	messageID := execution.currentMessageID

	// If this is the first chunk of a new message segment, generate the ID now
	if !isAppend {
		messageID = uuid.New().String()
		execution.currentMessageID = messageID
	}
	execution.messageMu.Unlock()

	event := AgentStreamEventData{
		Type:      "message_streaming",
		Text:      content,
		MessageID: messageID,
		IsAppend:  isAppend,
	}

	// Create payload manually to include streaming-specific fields
	payload := &AgentStreamEventPayload{
		Type:        "agent/event",
		Timestamp:   time.Now().UTC().Format(time.RFC3339Nano),
		AgentID:     execution.ID,
		ExecutionID: execution.ID,
		TaskID:      execution.TaskID,
		SessionID:   execution.SessionID,
		Data:        &event,
	}

	m.logger.Debug("publishing streaming message",
		zap.String("execution_id", execution.ID),
		zap.String("message_id", messageID),
		zap.Bool("is_append", isAppend),
		zap.Int("content_length", len(content)))

	// Publish the streaming event - orchestrator will handle create/append logic
	m.eventPublisher.PublishAgentStreamEventPayload(payload)
}

// flushMessageBuffer extracts any accumulated message from the buffer and returns it.
// This is called when a tool use starts or on complete to get the agent's response.
// It also clears the currentMessageID to start fresh for the next message segment.
// Additionally flushes any accumulated thinking content.
func (m *Manager) flushMessageBuffer(execution *AgentExecution) string {
	execution.messageMu.Lock()
	agentMessage := execution.messageBuffer.String()
	thinkingContent := execution.thinkingBuffer.String()
	execution.messageBuffer.Reset()
	execution.thinkingBuffer.Reset()
	// Clear the streaming message IDs so next segment starts fresh
	currentMsgID := execution.currentMessageID
	currentThinkingID := execution.currentThinkingID
	execution.currentMessageID = ""
	execution.currentThinkingID = ""
	execution.messageMu.Unlock()

	// If we have remaining thinking content, publish it
	trimmedThinking := strings.TrimSpace(thinkingContent)
	if trimmedThinking != "" {
		if currentThinkingID != "" {
			// Append to existing streaming thinking message
			m.publishStreamingThinkingFinal(execution, currentThinkingID, trimmedThinking)
		} else {
			// No streaming thinking message exists yet - create one with all the content
			// This happens when thinking content has no newlines (never triggered streaming)
			m.publishStreamingThinking(execution, trimmedThinking)
		}
		// Clear the thinking ID that publishStreamingThinking may have set as a side effect.
		// After a flush (tool call or complete), the next thinking segment must start a new message.
		execution.messageMu.Lock()
		execution.currentThinkingID = ""
		execution.messageMu.Unlock()
	}

	// If we have remaining message content, publish it
	trimmedMessage := strings.TrimSpace(agentMessage)
	if trimmedMessage != "" {
		if currentMsgID != "" {
			// Publish final append to the streaming message
			m.publishStreamingMessageFinal(execution, currentMsgID, trimmedMessage)
		} else {
			// No streaming message exists yet - create one with all the content
			// This happens when message content has no newlines (never triggered streaming)
			m.publishStreamingMessage(execution, trimmedMessage)
		}
		// Clear the message ID that publishStreamingMessage may have set as a side effect.
		// After a flush (tool call or complete), the next text segment must start a new message.
		execution.messageMu.Lock()
		execution.currentMessageID = ""
		execution.messageMu.Unlock()
		// Return empty since we've already handled it via streaming
		return ""
	}

	return ""
}

// publishStreamingMessageFinal publishes the final chunk of a streaming message.
// This is called during flush to append any remaining buffered content.
func (m *Manager) publishStreamingMessageFinal(execution *AgentExecution, messageID, content string) {
	event := AgentStreamEventData{
		Type:      "message_streaming",
		Text:      content,
		MessageID: messageID,
		IsAppend:  true,
	}

	payload := &AgentStreamEventPayload{
		Type:        "agent/event",
		Timestamp:   time.Now().UTC().Format(time.RFC3339Nano),
		AgentID:     execution.ID,
		ExecutionID: execution.ID,
		TaskID:      execution.TaskID,
		SessionID:   execution.SessionID,
		Data:        &event,
	}

	m.logger.Debug("publishing final streaming message chunk",
		zap.String("execution_id", execution.ID),
		zap.String("message_id", messageID),
		zap.Int("content_length", len(content)))

	m.eventPublisher.PublishAgentStreamEventPayload(payload)
}

// publishStreamingThinking publishes a streaming thinking event for real-time thinking updates.
// It creates a new thinking message on first call (currentThinkingID empty) or appends to existing.
// The message ID is generated and set synchronously to avoid race conditions.
func (m *Manager) publishStreamingThinking(execution *AgentExecution, content string) {
	execution.messageMu.Lock()
	isAppend := execution.currentThinkingID != ""
	thinkingID := execution.currentThinkingID

	// If this is the first chunk of a new thinking segment, generate the ID now
	if !isAppend {
		thinkingID = uuid.New().String()
		execution.currentThinkingID = thinkingID
	}
	execution.messageMu.Unlock()

	event := AgentStreamEventData{
		Type:        thinkingStreamingEventType,
		Text:        content,
		MessageID:   thinkingID,
		IsAppend:    isAppend,
		MessageType: "thinking",
	}

	// Create payload manually to include streaming-specific fields
	payload := &AgentStreamEventPayload{
		Type:        "agent/event",
		Timestamp:   time.Now().UTC().Format(time.RFC3339Nano),
		AgentID:     execution.ID,
		ExecutionID: execution.ID,
		TaskID:      execution.TaskID,
		SessionID:   execution.SessionID,
		Data:        &event,
	}

	// Publish the streaming event - orchestrator will handle create/append logic
	m.eventPublisher.PublishAgentStreamEventPayload(payload)
}

// publishStreamingThinkingFinal publishes the final chunk of a streaming thinking message.
// This is called during flush to append any remaining buffered thinking content.
func (m *Manager) publishStreamingThinkingFinal(execution *AgentExecution, thinkingID, content string) {
	event := AgentStreamEventData{
		Type:        thinkingStreamingEventType,
		Text:        content,
		MessageID:   thinkingID,
		IsAppend:    true,
		MessageType: "thinking",
	}

	payload := &AgentStreamEventPayload{
		Type:        "agent/event",
		Timestamp:   time.Now().UTC().Format(time.RFC3339Nano),
		AgentID:     execution.ID,
		ExecutionID: execution.ID,
		TaskID:      execution.TaskID,
		SessionID:   execution.SessionID,
		Data:        &event,
	}

	m.logger.Debug("publishing final streaming thinking chunk",
		zap.String("execution_id", execution.ID),
		zap.String("thinking_id", thinkingID),
		zap.Int("content_length", len(content)))

	m.eventPublisher.PublishAgentStreamEventPayload(payload)
}

// updateExecutionError updates an execution with an error
func (m *Manager) updateExecutionError(executionID, errorMsg string) {
	m.executionStore.UpdateError(executionID, errorMsg)
}
