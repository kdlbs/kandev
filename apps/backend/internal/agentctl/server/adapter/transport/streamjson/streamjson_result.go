package streamjson

import (
	"strings"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/pkg/claudecode"
	"go.uber.org/zap"
)

// updateContextWindowFromModelUsage updates context window from model_usage stats.
// Returns the updated contextUsed and contextSize.
func (a *Adapter) updateContextWindowFromModelUsage(msg *claudecode.CLIMessage) (contextUsed, contextSize int64) {
	a.mu.Lock()
	modelName := a.mainModelName
	if msg.ModelUsage != nil && modelName != "" {
		if modelStats, ok := msg.ModelUsage[modelName]; ok && modelStats.ContextWindow != nil {
			a.mainModelContextWindow = *modelStats.ContextWindow
			a.logger.Debug("updated context window from model_usage",
				zap.String("model", modelName),
				zap.Int64("context_window", a.mainModelContextWindow))
		}
	}
	contextSize = a.mainModelContextWindow
	contextUsed = a.contextTokensUsed
	a.mu.Unlock()
	return contextUsed, contextSize
}

// drainPendingToolCalls atomically removes and returns all pending tool call IDs.
func (a *Adapter) drainPendingToolCalls() []string {
	a.mu.Lock()
	pendingTools := make([]string, 0, len(a.pendingToolCalls))
	for toolID := range a.pendingToolCalls {
		pendingTools = append(pendingTools, toolID)
	}
	a.pendingToolCalls = make(map[string]*streams.NormalizedPayload) // Clear pending
	a.mu.Unlock()
	return pendingTools
}

// extractResultText extracts text from the result message (only if not already streamed).
func (a *Adapter) extractResultText(msg *claudecode.CLIMessage) string {
	if resultData := msg.GetResultData(); resultData != nil && resultData.Text != "" {
		return resultData.Text
	}
	if resultStr := msg.GetResultString(); resultStr != "" {
		return resultStr
	}
	return ""
}

// extractResultErrorMsg extracts the error message from a result.
func (a *Adapter) extractResultErrorMsg(msg *claudecode.CLIMessage) string {
	if !msg.IsError {
		return ""
	}
	if len(msg.Errors) > 0 {
		return strings.Join(msg.Errors, "; ")
	}
	return ""
}

// signalResultCompletion sends the result to the waiting prompt goroutine.
func (a *Adapter) signalResultCompletion(success bool, errMsg string) {
	a.mu.RLock()
	resultCh := a.resultCh
	a.mu.RUnlock()

	if resultCh == nil {
		return
	}
	select {
	case resultCh <- resultComplete{success: success, err: errMsg}:
		a.logger.Debug("signaled prompt completion")
	default:
		a.logger.Warn("result channel full, dropping signal")
	}
}

// handleResultMessage processes result (completion) messages.
func (a *Adapter) handleResultMessage(msg *claudecode.CLIMessage, sessionID, operationID string) {
	a.logger.Info("received result message",
		zap.Bool("is_error", msg.IsError),
		zap.Int("num_turns", msg.NumTurns))

	// Update session ID from result if provided
	if resultData := msg.GetResultData(); resultData != nil && resultData.SessionID != "" {
		a.mu.Lock()
		a.sessionID = resultData.SessionID
		sessionID = resultData.SessionID
		a.mu.Unlock()
	}

	// Auto-complete any pending tool calls that didn't receive explicit tool_result
	// This can happen if the tool result is not sent as a separate assistant message
	pendingTools := a.drainPendingToolCalls()
	for _, toolID := range pendingTools {
		a.logger.Debug("auto-completing pending tool call on result",
			zap.String("tool_call_id", toolID))
		a.sendUpdate(AgentEvent{
			Type:        streams.EventTypeToolUpdate,
			SessionID:   sessionID,
			OperationID: operationID,
			ToolCallID:  toolID,
			ToolStatus:  "complete",
		})
	}

	// Extract actual context window from model_usage if available
	contextUsed, contextSize := a.updateContextWindowFromModelUsage(msg)

	// Emit final accurate context window event
	if contextUsed > 0 {
		a.emitContextWindow(sessionID, operationID, contextUsed, contextSize)
	}

	// Check if text was already streamed this turn via assistant messages
	a.mu.RLock()
	textWasStreamed := a.streamingTextSentThisTurn
	a.mu.RUnlock()

	// Only send result.text if NO text was streamed this turn.
	// If text was already streamed via assistant messages, result.text is a duplicate.
	// This prevents the same content from appearing twice in the conversation.
	if !textWasStreamed {
		if resultText := a.extractResultText(msg); resultText != "" {
			a.logger.Debug("sending result text as message_chunk (no streaming text this turn)",
				zap.Int("text_length", len(resultText)))
			a.sendUpdate(AgentEvent{
				Type:        streams.EventTypeMessageChunk,
				SessionID:   sessionID,
				OperationID: operationID,
				Text:        resultText,
			})
		}
	} else {
		a.logger.Debug("skipping result text (streaming text already sent this turn)")
	}

	// Reset the streaming flag for next turn
	a.mu.Lock()
	a.streamingTextSentThisTurn = false
	a.mu.Unlock()

	// Build error message from errors array if available
	errorMsg := a.extractResultErrorMsg(msg)

	// Send completion event AFTER any result text chunk
	completeData := map[string]any{
		"cost_usd":      msg.CostUSD,
		"duration_ms":   msg.DurationMS,
		"num_turns":     msg.NumTurns,
		"input_tokens":  msg.TotalInputTokens,
		"output_tokens": msg.TotalOutputTokens,
		"is_error":      msg.IsError,
	}
	if len(msg.Errors) > 0 {
		completeData["errors"] = msg.Errors
	}
	a.sendUpdate(AgentEvent{
		Type:        streams.EventTypeComplete,
		SessionID:   sessionID,
		OperationID: operationID,
		Error:       errorMsg,
		Data:        completeData,
	})

	// Signal completion
	a.signalResultCompletion(!msg.IsError, errorMsg)

	// Send error event if failed
	if msg.IsError {
		a.sendResultError(msg, sessionID, operationID)
	}
}

// sendResultError emits an error event with the best available error message.
func (a *Adapter) sendResultError(msg *claudecode.CLIMessage, sessionID, operationID string) {
	errMsg := "prompt failed"
	// Use errors array first (most specific), then fallback to result string/data
	if len(msg.Errors) > 0 {
		errMsg = strings.Join(msg.Errors, "; ")
	} else if errStr := msg.GetResultString(); errStr != "" {
		errMsg = errStr
	} else if resultData := msg.GetResultData(); resultData != nil && resultData.Text != "" {
		errMsg = resultData.Text
	}
	a.sendUpdate(AgentEvent{
		Type:        streams.EventTypeError,
		SessionID:   sessionID,
		OperationID: operationID,
		Error:       errMsg,
	})
}
