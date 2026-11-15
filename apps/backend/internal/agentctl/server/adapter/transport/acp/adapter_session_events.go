package acp

import (
	"encoding/json"
	"math"
	"strings"
	"unicode/utf8"

	acp "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

const (
	maxCompactionSummaryBytes = 64 << 10
	compactionStatusFailed    = "failed"
)

func (a *Adapter) convertCompactionUpdate(sessionID string, update *acp.SessionCompactionUpdate) *AgentEvent {
	if update == nil || string(update.CompactionId) == "" {
		return nil
	}

	toolCallID := compactionToolCallID(sessionID, string(update.CompactionId))
	a.mu.Lock()
	if a.contextCompactions == nil {
		a.contextCompactions = make(map[string]*streams.NormalizedPayload)
	}
	payload := a.contextCompactions[toolCallID]
	eventType := streams.EventTypeToolUpdate
	if payload == nil {
		payload = streams.NewContextCompaction(streams.ContextCompactionPayload{
			ID:     string(update.CompactionId),
			Status: normalizedCompactionStatus(string(update.Status)),
		})
		a.contextCompactions[toolCallID] = payload
		eventType = streams.EventTypeToolCall
	}
	compaction := payload.ContextCompaction()
	compaction.Status = normalizedCompactionStatus(string(update.Status))
	applyCompactionError(compaction, update.Error)
	applyCompactionSummary(compaction, update.Summary)
	applyCompactionMeta(compaction, update.Meta)
	status := compactionToolStatus(compaction.Status)
	result := payload.Snapshot()
	a.mu.Unlock()

	return &AgentEvent{
		Type:              eventType,
		SessionID:         sessionID,
		ToolCallID:        toolCallID,
		ToolName:          "context_compaction",
		ToolTitle:         "Context compaction",
		ToolStatus:        status,
		NormalizedPayload: result,
	}
}

func (a *Adapter) convertCompactionSummaryChunk(
	sessionID string,
	chunk *acp.SessionUpdateCompactionSummaryChunk,
) *AgentEvent {
	if chunk == nil || string(chunk.CompactionId) == "" || chunk.Content.Text == nil {
		return nil
	}
	toolCallID := compactionToolCallID(sessionID, string(chunk.CompactionId))
	a.mu.Lock()
	payload := a.contextCompactions[toolCallID]
	if payload == nil {
		a.mu.Unlock()
		return nil
	}
	compaction := payload.ContextCompaction()
	compaction.Summary = appendCompactionSummary(compaction.Summary, chunk.Content.Text.Text)
	result := payload.Snapshot()
	status := compactionToolStatus(compaction.Status)
	a.mu.Unlock()
	return &AgentEvent{
		Type:              streams.EventTypeToolUpdate,
		SessionID:         sessionID,
		ToolCallID:        toolCallID,
		ToolName:          "context_compaction",
		ToolTitle:         "Context compaction",
		ToolStatus:        status,
		NormalizedPayload: result,
	}
}

func compactionToolCallID(sessionID, compactionID string) string {
	return "compaction:" + sessionID + ":" + compactionID
}

func normalizedCompactionStatus(status string) string {
	switch status {
	case toolStatusCompleted, compactionStatusFailed, toolStatusCancelled:
		return status
	case toolStatusInProgress:
		return toolStatusInProgress
	default:
		return toolStatusInProgress
	}
}

func compactionToolStatus(status string) string {
	switch status {
	case toolStatusCompleted:
		return toolStatusComplete
	case compactionStatusFailed:
		return toolStatusError
	case toolStatusCancelled:
		return toolStatusCancelled
	default:
		return toolStatusInProgress
	}
}

func applyCompactionError(payload *streams.ContextCompactionPayload, value acp.Optional[string]) {
	if !value.IsSet() {
		return
	}
	text, ok := value.Get()
	if !ok {
		payload.Error = ""
		return
	}
	payload.Error = boundedCompactionText(text)
}

func applyCompactionSummary(payload *streams.ContextCompactionPayload, value acp.Optional[[]acp.ContentBlock]) {
	if !value.IsSet() {
		return
	}
	blocks, ok := value.Get()
	if !ok || len(blocks) == 0 {
		payload.Summary = ""
		return
	}
	var summary strings.Builder
	for _, block := range blocks {
		if block.Text == nil {
			continue
		}
		if summary.Len() > 0 {
			summary.WriteByte('\n')
		}
		remaining := maxCompactionSummaryBytes - summary.Len()
		if remaining <= 0 {
			break
		}
		summary.WriteString(truncateCompactionText(block.Text.Text, remaining))
		if summary.Len() == maxCompactionSummaryBytes {
			break
		}
	}
	payload.Summary = boundedCompactionText(summary.String())
}

func applyCompactionMeta(payload *streams.ContextCompactionPayload, value acp.Optional[map[string]any]) {
	if !value.IsSet() {
		return
	}
	meta, ok := value.Get()
	if !ok {
		clearCompactionMeta(payload)
		return
	}
	contextValue, exists := meta["contextCompaction"]
	if !exists {
		return
	}
	if contextValue == nil {
		clearCompactionMeta(payload)
		return
	}
	contextMeta, ok := contextValue.(map[string]any)
	if !ok {
		return
	}
	if raw, exists := contextMeta["trigger"]; exists {
		if raw == nil {
			payload.Trigger = ""
		} else if trigger, ok := raw.(string); ok {
			payload.Trigger = boundedCompactionText(trigger)
		}
	}
	applyCompactionCount(contextMeta, &payload.PreTokens, "pre_tokens", "preTokens")
	applyCompactionCount(contextMeta, &payload.PostTokens, "post_tokens", "postTokens")
	applyCompactionCount(contextMeta, &payload.DurationMs, "duration_ms", "durationMs")
}

func clearCompactionMeta(payload *streams.ContextCompactionPayload) {
	payload.Trigger = ""
	payload.PreTokens = nil
	payload.PostTokens = nil
	payload.DurationMs = nil
}

func applyCompactionCount(meta map[string]any, target **int64, keys ...string) {
	for _, key := range keys {
		value, exists := meta[key]
		if !exists {
			continue
		}
		if value == nil {
			*target = nil
			return
		}
		count, ok := compactionInt64(value)
		if ok {
			*target = &count
		}
		return
	}
}

func compactionInt64(value any) (int64, bool) {
	switch number := value.(type) {
	case int:
		return checkedCompactionInt64(int64(number))
	case int32:
		return checkedCompactionInt64(int64(number))
	case int64:
		return checkedCompactionInt64(number)
	case uint:
		return checkedCompactionUint64(uint64(number))
	case uint64:
		return checkedCompactionUint64(number)
	case float64:
		return checkedCompactionFloat64(number)
	case json.Number:
		parsed, err := number.Int64()
		if err != nil {
			return 0, false
		}
		return checkedCompactionInt64(parsed)
	default:
		return 0, false
	}
}

func checkedCompactionInt64(value int64) (int64, bool) {
	return value, value >= 0
}

func checkedCompactionUint64(value uint64) (int64, bool) {
	if value > math.MaxInt64 {
		return 0, false
	}
	return int64(value), true
}

func checkedCompactionFloat64(value float64) (int64, bool) {
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || math.Trunc(value) != value || value >= float64(math.MaxInt64) {
		return 0, false
	}
	return int64(value), true
}

func appendCompactionSummary(existing, chunk string) string {
	if len(existing) >= maxCompactionSummaryBytes {
		return boundedCompactionText(existing)
	}
	remaining := maxCompactionSummaryBytes - len(existing)
	chunk = truncateCompactionText(chunk, remaining)
	return existing + chunk
}

func boundedCompactionText(value string) string {
	if len(value) <= maxCompactionSummaryBytes {
		return value
	}
	return truncateCompactionText(value, maxCompactionSummaryBytes)
}

func truncateCompactionText(value string, maxBytes int) string {
	if len(value) <= maxBytes {
		return value
	}
	end := maxBytes
	for end > 0 && !utf8.RuneStart(value[end]) {
		end--
	}
	return value[:end]
}
