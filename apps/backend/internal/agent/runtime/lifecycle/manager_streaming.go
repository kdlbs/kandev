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

func (m *Manager) streamCoalescer(execution *AgentExecution) *streamCoalescer {
	execution.streamMu.Lock()
	defer execution.streamMu.Unlock()
	if execution.stream == nil {
		execution.stream = newStreamCoalescer(defaultStreamCoalesceWindow, func(chunk coalescedStreamChunk) {
			m.publishStreamingContentNowWithAttempt(
				execution,
				chunk.eventType,
				chunk.messageID,
				chunk.content,
				chunk.isAppend,
				chunk.attemptID,
			)
		})
	}
	return execution.stream
}

func (m *Manager) flushStreamCoalescer(execution *AgentExecution) {
	execution.streamMu.Lock()
	stream := execution.stream
	execution.streamMu.Unlock()
	if stream != nil {
		stream.flushBoundary()
	}
}

func (m *Manager) closeStreamCoalescer(execution *AgentExecution) {
	execution.streamMu.Lock()
	stream := execution.stream
	execution.stream = nil
	execution.streamMu.Unlock()
	if stream != nil {
		stream.close()
		stats := stream.stats()
		if stats.received > 0 {
			m.logger.Info("agent stream coalescer closed",
				zap.String("execution_id", execution.ID),
				zap.String("session_id", execution.SessionID),
				zap.Int("stream_chunks_received", stats.received),
				zap.Int("stream_chunks_coalesced", stats.coalesced),
				zap.Int("stream_segments_flushed", stats.flushed))
		}
	}
}

func (m *Manager) enqueueStreamingContent(
	execution *AgentExecution,
	eventType string,
	messageID string,
	content string,
	isAppend bool,
) {
	m.enqueueStreamingContentWithAttempt(
		execution, eventType, messageID, content, isAppend, execution.currentStartupAttemptID(),
	)
}

func (m *Manager) enqueueStreamingContentWithAttempt(
	execution *AgentExecution,
	eventType string,
	messageID string,
	content string,
	isAppend bool,
	attemptID string,
) {
	if content == "" {
		return
	}
	m.streamCoalescer(execution).add(coalescedStreamChunk{
		eventType: eventType,
		messageID: messageID,
		attemptID: attemptID,
		content:   content,
		isAppend:  isAppend,
	})
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
	m.publishProtocolMessageWithAttempt(execution, protocolMessageID, content, execution.currentStartupAttemptID())
}

func (m *Manager) publishProtocolMessageWithAttempt(
	execution *AgentExecution,
	protocolMessageID string,
	content string,
	attemptID string,
) {
	execution.messageMu.Lock()
	messageID, isAppend := protocolRecordID(&execution.protocolMessageIDs, protocolMessageID)
	execution.messageMu.Unlock()

	m.publishStreamingContentWithAttempt(execution, "message_streaming", messageID, content, isAppend, attemptID)
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

func (m *Manager) resetStreamingStateWithHistory(execution *AgentExecution) {
	resetStreamingStateWithHistory(execution, m.historyManager, m.logger)
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
	content := execution.detachAssistantHistoryLocked()
	execution.messageMu.Unlock()

	persistAssistantHistory(content, execution, historyManager, log)
}

// resetStreamingStateWithHistory atomically detaches the prior assistant
// transcript and resets every streaming buffer/correlation under messageMu.
// Persistence happens afterward so filesystem I/O never blocks incoming chunks.
func resetStreamingStateWithHistory(
	execution *AgentExecution,
	historyManager *SessionHistoryManager,
	log *logger.Logger,
) {
	execution.streamMu.Lock()
	stream := execution.stream
	execution.streamMu.Unlock()
	if stream != nil {
		stream.flushBoundary()
	}
	execution.messageMu.Lock()
	content := execution.detachAssistantHistoryLocked()
	execution.resetStreamingStateLocked()
	execution.messageMu.Unlock()

	persistAssistantHistory(content, execution, historyManager, log)
}

func (e *AgentExecution) detachAssistantHistoryLocked() string {
	content := e.assistantHistoryBuffer.String()
	e.assistantHistoryBuffer.Reset()
	return content
}

func persistAssistantHistory(
	content string,
	execution *AgentExecution,
	historyManager *SessionHistoryManager,
	log *logger.Logger,
) {
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
	m.flushPendingLegacyMessageWithAttempt(execution, execution.currentStartupAttemptID())
}

func (m *Manager) flushPendingLegacyMessageWithAttempt(execution *AgentExecution, attemptID string) {
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
		// The final direct append must follow any coalesced legacy chunk that
		// was already emitted for this record.
		m.flushStreamCoalescer(execution)
		m.publishStreamingMessageFinalWithAttempt(execution, messageID, trimmed, attemptID)
		return
	}
	m.publishStreamingMessageWithAttempt(execution, trimmed, attemptID)
	execution.messageMu.Lock()
	execution.currentMessageID = ""
	execution.messageMu.Unlock()
}

// flushPendingLegacyThinking is the reasoning-stream counterpart of
// flushPendingLegacyMessage.
func (m *Manager) flushPendingLegacyThinking(execution *AgentExecution) {
	m.flushPendingLegacyThinkingWithAttempt(execution, execution.currentStartupAttemptID())
}

func (m *Manager) flushPendingLegacyThinkingWithAttempt(execution *AgentExecution, attemptID string) {
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
		// The final direct append must follow any coalesced legacy chunk that
		// was already emitted for this record.
		m.flushStreamCoalescer(execution)
		m.publishStreamingThinkingFinalWithAttempt(execution, messageID, trimmed, attemptID)
		return
	}
	m.publishStreamingThinkingWithAttempt(execution, trimmed, attemptID)
	execution.messageMu.Lock()
	execution.currentThinkingID = ""
	execution.messageMu.Unlock()
}

func (m *Manager) publishProtocolThinking(
	execution *AgentExecution,
	protocolMessageID string,
	content string,
) {
	m.publishProtocolThinkingWithAttempt(execution, protocolMessageID, content, execution.currentStartupAttemptID())
}

func (m *Manager) publishProtocolThinkingWithAttempt(
	execution *AgentExecution,
	protocolMessageID string,
	content string,
	attemptID string,
) {
	execution.messageMu.Lock()
	messageID, isAppend := protocolRecordID(&execution.protocolThinkingIDs, protocolMessageID)
	execution.messageMu.Unlock()

	m.enqueueStreamingContentWithAttempt(execution, thinkingStreamingEventType, messageID, content, isAppend, attemptID)
}

func (m *Manager) publishStreamingContent(
	execution *AgentExecution,
	eventType string,
	messageID string,
	content string,
	isAppend bool,
) {
	m.publishStreamingContentWithAttempt(execution, eventType, messageID, content, isAppend, execution.currentStartupAttemptID())
}

func (m *Manager) publishStreamingContentWithAttempt(
	execution *AgentExecution,
	eventType string,
	messageID string,
	content string,
	isAppend bool,
	attemptID string,
) {
	m.enqueueStreamingContentWithAttempt(execution, eventType, messageID, content, isAppend, attemptID)
}

func (m *Manager) publishStreamingContentNow(
	execution *AgentExecution,
	eventType string,
	messageID string,
	content string,
	isAppend bool,
) {
	m.publishStreamingContentNowWithAttempt(
		execution, eventType, messageID, content, isAppend, execution.currentStartupAttemptID(),
	)
}

func (m *Manager) publishStreamingContentNowWithAttempt(
	execution *AgentExecution,
	eventType string,
	messageID string,
	content string,
	isAppend bool,
	attemptID string,
) {
	if attemptID == "" {
		attemptID = execution.currentStartupAttemptID()
	}
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
		Type:           "agent/event",
		Timestamp:      time.Now().UTC().Format(time.RFC3339Nano),
		AgentID:        execution.ID,
		ExecutionID:    execution.ID,
		AttemptID:      attemptID,
		AgentProfileID: execution.officeProfileID(),
		TaskID:         execution.TaskID,
		SessionID:      execution.SessionID,
		Data:           &event,
	}
	m.eventPublisher.PublishAgentStreamEventPayload(payload)
}

// publishStreamingMessage publishes a streaming message event for real-time text updates.
// It creates a new message on first call (currentMessageID empty) or appends to existing.
// The message ID is generated and set synchronously to avoid race conditions.
func (m *Manager) publishStreamingMessage(execution *AgentExecution, content string) {
	m.publishStreamingMessageWithAttempt(execution, content, execution.currentStartupAttemptID())
}

func (m *Manager) publishStreamingMessageWithAttempt(execution *AgentExecution, content, attemptID string) {
	execution.messageMu.Lock()
	isAppend := execution.currentMessageID != ""
	messageID := execution.currentMessageID

	// If this is the first chunk of a new message segment, generate the ID now
	if !isAppend {
		messageID = uuid.New().String()
		execution.currentMessageID = messageID
	}
	execution.messageMu.Unlock()

	m.logger.Debug("publishing streaming message",
		zap.String("execution_id", execution.ID),
		zap.String("message_id", messageID),
		zap.Bool("is_append", isAppend),
		zap.Int("content_length", len(content)))

	m.enqueueStreamingContentWithAttempt(execution, "message_streaming", messageID, content, isAppend, attemptID)
}

// flushMessageBuffer extracts any accumulated message from the buffer and returns it.
// This is called when a tool use starts or on complete to get the agent's response.
// It also clears the currentMessageID to start fresh for the next message segment.
// Additionally flushes any accumulated thinking content.
func (m *Manager) flushMessageBuffer(execution *AgentExecution) string {
	return m.flushMessageBufferWithAttempt(execution, execution.currentStartupAttemptID())
}

func (m *Manager) flushMessageBufferWithAttempt(execution *AgentExecution, attemptID string) string {
	m.flushStreamCoalescer(execution)
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
			m.publishStreamingThinkingFinalWithAttempt(execution, currentThinkingID, trimmedThinking, attemptID)
		} else {
			// No streaming thinking message exists yet - create one with all the content
			// This happens when thinking content has no newlines (never triggered streaming)
			m.publishStreamingThinkingWithAttempt(execution, trimmedThinking, attemptID)
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
			m.publishStreamingMessageFinalWithAttempt(execution, currentMsgID, trimmedMessage, attemptID)
		} else {
			// No streaming message exists yet - create one with all the content
			// This happens when message content has no newlines (never triggered streaming)
			m.publishStreamingMessageWithAttempt(execution, trimmedMessage, attemptID)
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
	m.publishStreamingMessageFinalWithAttempt(execution, messageID, content, execution.currentStartupAttemptID())
}

func (m *Manager) publishStreamingMessageFinalWithAttempt(execution *AgentExecution, messageID, content, attemptID string) {
	m.logger.Debug("publishing final streaming message chunk",
		zap.String("execution_id", execution.ID),
		zap.String("message_id", messageID),
		zap.Int("content_length", len(content)))

	m.publishStreamingContentNowWithAttempt(execution, "message_streaming", messageID, content, true, attemptID)
}

// publishStreamingThinking publishes a streaming thinking event for real-time thinking updates.
// It creates a new thinking message on first call (currentThinkingID empty) or appends to existing.
// The message ID is generated and set synchronously to avoid race conditions.
func (m *Manager) publishStreamingThinking(execution *AgentExecution, content string) {
	m.publishStreamingThinkingWithAttempt(execution, content, execution.currentStartupAttemptID())
}

func (m *Manager) publishStreamingThinkingWithAttempt(execution *AgentExecution, content, attemptID string) {
	execution.messageMu.Lock()
	isAppend := execution.currentThinkingID != ""
	thinkingID := execution.currentThinkingID

	// If this is the first chunk of a new thinking segment, generate the ID now
	if !isAppend {
		thinkingID = uuid.New().String()
		execution.currentThinkingID = thinkingID
	}
	execution.messageMu.Unlock()

	m.enqueueStreamingContentWithAttempt(execution, thinkingStreamingEventType, thinkingID, content, isAppend, attemptID)
}

// publishStreamingThinkingFinal publishes the final chunk of a streaming thinking message.
// This is called during flush to append any remaining buffered thinking content.
func (m *Manager) publishStreamingThinkingFinal(execution *AgentExecution, thinkingID, content string) {
	m.publishStreamingThinkingFinalWithAttempt(execution, thinkingID, content, execution.currentStartupAttemptID())
}

func (m *Manager) publishStreamingThinkingFinalWithAttempt(execution *AgentExecution, thinkingID, content, attemptID string) {
	m.logger.Debug("publishing final streaming thinking chunk",
		zap.String("execution_id", execution.ID),
		zap.String("thinking_id", thinkingID),
		zap.Int("content_length", len(content)))

	m.publishStreamingContentNowWithAttempt(execution, thinkingStreamingEventType, thinkingID, content, true, attemptID)
}

// updateExecutionError updates an execution with an error
func (m *Manager) updateExecutionError(executionID, errorMsg string) {
	m.executionStore.UpdateError(executionID, errorMsg)
}
