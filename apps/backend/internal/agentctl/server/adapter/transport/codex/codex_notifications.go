package codex

import (
	"encoding/json"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/pkg/codex"
	"go.uber.org/zap"
)

// handleAgentMessageDelta handles item/agentMessage/delta notifications.
func (a *Adapter) handleAgentMessageDelta(params json.RawMessage, threadID, turnID string) {
	var p codex.AgentMessageDeltaParams
	if err := json.Unmarshal(params, &p); err != nil {
		a.logger.Warn("failed to parse agent message delta", zap.Error(err))
		return
	}
	a.mu.Lock()
	a.messageBuffer += p.Delta
	a.mu.Unlock()
	a.sendUpdate(AgentEvent{
		Type:        streams.EventTypeMessageChunk,
		SessionID:   threadID,
		OperationID: turnID,
		Text:        p.Delta,
	})
}

// handleReasoningDelta handles reasoning text and summary delta notifications.
func (a *Adapter) handleReasoningDelta(params json.RawMessage, threadID, turnID, logLabel string) {
	var p codex.ReasoningDeltaParams
	if err := json.Unmarshal(params, &p); err != nil {
		a.logger.Warn("failed to parse "+logLabel, zap.Error(err))
		return
	}
	a.mu.Lock()
	// Add separator when switching to a new reasoning item
	if p.ItemID != a.currentReasoningItemID && a.reasoningBuffer != "" {
		a.reasoningBuffer += "\n\n"
	}
	a.currentReasoningItemID = p.ItemID
	a.reasoningBuffer += p.Delta
	a.mu.Unlock()
	a.sendUpdate(AgentEvent{
		Type:          streams.EventTypeReasoning,
		SessionID:     threadID,
		OperationID:   turnID,
		ReasoningText: p.Delta,
	})
}

// handleTurnCompleted handles turn/completed notifications.
func (a *Adapter) handleTurnCompleted(params json.RawMessage, threadID string) {
	var p codex.TurnCompletedParams
	if err := json.Unmarshal(params, &p); err != nil {
		a.logger.Warn("failed to parse turn completed", zap.Error(err))
		return
	}

	// Signal turn completion to the waiting Prompt() call
	a.mu.RLock()
	completeCh := a.turnCompleteCh
	a.mu.RUnlock()

	if completeCh != nil {
		select {
		case completeCh <- turnCompleteResult{success: p.Success, err: p.Error}:
			a.logger.Debug("signaled turn completion", zap.String("turn_id", p.TurnID), zap.Bool("success", p.Success))
		default:
			a.logger.Warn("turn complete channel full, dropping signal")
		}
	}

	// Send error event if the turn failed WITH an explicit error message.
	// Note: We don't send error events here based on stderr alone, because
	// NotifyError will handle error notifications (prevents duplicate messages).
	if !p.Success {
		a.logger.Debug("turn completed with failure",
			zap.String("thread_id", threadID),
			zap.String("turn_id", p.TurnID),
			zap.Bool("success", p.Success),
			zap.String("error", p.Error))

		// Only send error event if there's an explicit error message
		// (NotifyError handles the case when error details come separately)
		if p.Error != "" {
			a.sendUpdate(AgentEvent{
				Type:        streams.EventTypeError,
				SessionID:   threadID,
				OperationID: p.TurnID,
				Error:       p.Error,
			})
		}
	}
}

// handleTurnDiffUpdated handles turn/diffUpdated notifications.
func (a *Adapter) handleTurnDiffUpdated(params json.RawMessage, threadID string) {
	var p codex.TurnDiffUpdatedParams
	if err := json.Unmarshal(params, &p); err != nil {
		a.logger.Warn("failed to parse turn diff updated", zap.Error(err))
		return
	}
	a.sendUpdate(AgentEvent{
		Type:        streams.EventTypeMessageChunk,
		SessionID:   threadID,
		OperationID: p.TurnID,
		Diff:        p.Diff,
	})
}

// handleTurnPlanUpdated handles turn/planUpdated notifications.
func (a *Adapter) handleTurnPlanUpdated(params json.RawMessage, threadID string) {
	var p codex.TurnPlanUpdatedParams
	if err := json.Unmarshal(params, &p); err != nil {
		a.logger.Warn("failed to parse turn plan updated", zap.Error(err))
		return
	}
	entries := make([]PlanEntry, len(p.Plan))
	for i, e := range p.Plan {
		entries[i] = PlanEntry{
			Description: e.Description,
			Status:      e.Status,
		}
	}
	a.sendUpdate(AgentEvent{
		Type:        streams.EventTypePlan,
		SessionID:   threadID,
		OperationID: p.TurnID,
		PlanEntries: entries,
	})
}

// handleErrorNotification handles error notifications from the agent.
func (a *Adapter) handleErrorNotification(params json.RawMessage, threadID string) {
	var p codex.ErrorParams
	if err := json.Unmarshal(params, &p); err != nil {
		a.logger.Warn("failed to parse error notification", zap.Error(err))
		return
	}

	// Get recent stderr for error context
	var stderrLines []string
	a.mu.RLock()
	if a.stderrProvider != nil {
		stderrLines = a.stderrProvider.GetRecentStderr()
	}
	a.mu.RUnlock()

	// Try to parse stderr into structured error info
	// This handles Codex-specific error formats (e.g., rate limits)
	parsedError := ParseCodexStderrLines(stderrLines)

	var parsedMsg string
	if parsedError != nil {
		parsedMsg = parsedError.Message
	}

	a.logger.Debug("received error notification from agent",
		zap.String("thread_id", threadID),
		zap.Int("code", p.Code),
		zap.String("message", p.Message),
		zap.String("parsed_message", parsedMsg),
		zap.Any("data", p.Data),
		zap.Int("stderr_lines", len(stderrLines)))

	// Build error data with all available context
	errorData := map[string]any{
		"code":   p.Code,
		"data":   p.Data,
		"stderr": stderrLines,
	}

	// Include parsed error details if available
	if parsedError != nil {
		parsed := map[string]any{
			"http_error": parsedError.HTTPError,
		}
		// Include the full raw JSON (captures all fields from any error type)
		if parsedError.RawJSON != nil {
			parsed["error_json"] = parsedError.RawJSON
		}
		errorData["parsed"] = parsed
	}

	a.sendUpdate(AgentEvent{
		Type:      streams.EventTypeError,
		SessionID: threadID,
		Error:     p.Message,
		Text:      parsedMsg, // User-friendly parsed message
		Data:      errorData,
	})
}

// handleCmdExecOutputDelta handles item/cmdExec/outputDelta notifications.
func (a *Adapter) handleCmdExecOutputDelta(params json.RawMessage, threadID, turnID string) {
	var p codex.CommandOutputDeltaParams
	if err := json.Unmarshal(params, &p); err != nil {
		a.logger.Warn("failed to parse command output delta", zap.Error(err))
		return
	}
	a.sendUpdate(AgentEvent{
		Type:        streams.EventTypeToolUpdate,
		SessionID:   threadID,
		OperationID: turnID,
		ToolCallID:  p.ItemID,
	})
}

// handleTokenUsageUpdated handles thread/tokenUsage/updated notifications.
func (a *Adapter) handleTokenUsageUpdated(params json.RawMessage, threadID, turnID string) {
	var p codex.ThreadTokenUsageUpdatedParams
	if err := json.Unmarshal(params, &p); err != nil {
		a.logger.Warn("failed to parse thread token usage updated notification", zap.Error(err))
		return
	}
	// Extract context window information from the token usage update
	if p.TokenUsage == nil || p.TokenUsage.ModelContextWindow <= 0 {
		return
	}
	contextWindowSize := p.TokenUsage.ModelContextWindow
	contextWindowUsed := int64(p.TokenUsage.Last.TotalTokens)

	remaining := contextWindowSize - contextWindowUsed
	if remaining < 0 {
		remaining = 0
	}
	efficiency := float64(contextWindowUsed) / float64(contextWindowSize) * 100

	a.logger.Debug("emitting context window event",
		zap.Int64("size", contextWindowSize),
		zap.Int64("used", contextWindowUsed),
		zap.Int64("remaining", remaining),
		zap.Float64("efficiency", efficiency))

	a.sendUpdate(AgentEvent{
		Type:                   streams.EventTypeContextWindow,
		SessionID:              threadID,
		OperationID:            turnID,
		ContextWindowSize:      contextWindowSize,
		ContextWindowUsed:      contextWindowUsed,
		ContextWindowRemaining: remaining,
		ContextEfficiency:      efficiency,
	})
}

// handleContextCompacted handles context/compacted notifications.
func (a *Adapter) handleContextCompacted(params json.RawMessage) {
	var p codex.ContextCompactedParams
	if err := json.Unmarshal(params, &p); err != nil {
		a.logger.Warn("failed to parse context compacted notification", zap.Error(err))
		return
	}
	a.logger.Info("context compacted",
		zap.String("thread_id", p.ThreadID),
		zap.String("turn_id", p.TurnID))
	// We could emit an event here if we want to notify the frontend about compaction
}
