package codexappserver

import (
	"encoding/json"
	"fmt"
	"math"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	protocol "github.com/kandev/kandev/pkg/codexappserver"
)

const (
	nativeUsageSchemaVersion      = 1
	nativeUsageSourceTurnFallback = "turn_fallback"
)

func (a *Adapter) handleTokenUsageNotification(params map[string]any) {
	var notification protocol.ThreadTokenUsageUpdatedNotification
	encoded, err := json.Marshal(params)
	if err != nil || json.Unmarshal(encoded, &notification) != nil || notification.ThreadID == "" || notification.TurnID == "" {
		return
	}
	threadID := notification.ThreadID
	key := nativeTurnKey(threadID, notification.TurnID)
	a.mu.Lock()
	a.latestTokenTotals[threadID] = notification.TokenUsage.Total
	if notification.TokenUsage.ModelContextWindow != nil && *notification.TokenUsage.ModelContextWindow > 0 {
		a.latestContextWindows[threadID] = *notification.TokenUsage.ModelContextWindow
	}
	shouldFallback := a.completedProviderTurns[key] && !a.turnResponseObserved[key] && !a.turnFallbackSelected[key]
	current := notification.TokenUsage.Total
	baseline, hasBaseline := a.turnTokenBaselines[key], a.turnHasTokenBaseline[key]
	a.mu.Unlock()
	if shouldFallback && hasBaseline {
		a.emitFallbackObservation(threadID, notification.TurnID, baseline, current)
	}
}

func (a *Adapter) beginProviderTurn(threadID, turnID string) {
	if threadID == "" || turnID == "" {
		return
	}
	key := nativeTurnKey(threadID, turnID)
	a.mu.Lock()
	if _, exists := a.turnModels[key]; exists {
		a.mu.Unlock()
		return
	}
	model := a.modelID
	generation := a.activeGeneration
	if child, ok := a.children[threadID]; ok {
		model = child.model
		generation = child.generation
	}
	a.turnModels[key] = model
	a.turnGenerations[key] = generation
	if total, ok := a.latestTokenTotals[threadID]; ok {
		a.turnTokenBaselines[key] = total
		a.turnHasTokenBaseline[key] = true
	} else {
		a.turnHasTokenBaseline[key] = false
	}
	a.mu.Unlock()
}

func (a *Adapter) handleRawResponseCompleted(params map[string]any) {
	var notification protocol.RawResponseCompleted
	encoded, err := json.Marshal(params)
	if err != nil || json.Unmarshal(encoded, &notification) != nil || notification.ThreadID == "" || notification.TurnID == "" || notification.ResponseID == "" || notification.Usage == nil {
		return
	}
	rootThreadID, parentToolCallID, isChild := a.eventScope(notification.ThreadID)
	if notification.ThreadID != rootThreadID && !isChild {
		// A provider thread without a known parent must not be attributed to the
		// root conversation by guessing from its arrival time.
		return
	}
	key := nativeTurnKey(notification.ThreadID, notification.TurnID)
	a.mu.Lock()
	if a.turnFallbackSelected[key] {
		a.mu.Unlock()
		return
	}
	a.turnResponseObserved[key] = true
	model := a.turnModels[key]
	generation := a.turnGenerations[key]
	a.mu.Unlock()

	usage, observation, ok := normalizeRawResponseUsage(*notification.Usage, notification.ThreadID, notification.TurnID, notification.ResponseID, isChild, model)
	if !ok {
		return
	}
	a.emit(streams.AgentEvent{
		Type:             streams.EventTypeUsageObservation,
		SessionID:        rootThreadID,
		OperationID:      notification.TurnID,
		PromptGeneration: generation,
		ParentToolCallID: parentToolCallID,
		Usage:            usage,
		UsageObservation: observation,
	})
}

func (a *Adapter) finalizeProviderTurn(threadID, turnID string) {
	if threadID == "" || turnID == "" {
		return
	}
	key := nativeTurnKey(threadID, turnID)
	a.mu.Lock()
	a.completedProviderTurns[key] = true
	if a.turnResponseObserved[key] || a.turnFallbackSelected[key] {
		a.mu.Unlock()
		return
	}
	baseline, hasBaseline := a.turnTokenBaselines[key], a.turnHasTokenBaseline[key]
	current, hasCurrent := a.latestTokenTotals[threadID]
	a.mu.Unlock()
	if hasBaseline && hasCurrent {
		a.emitFallbackObservation(threadID, turnID, baseline, current)
	}
}

func (a *Adapter) emitFallbackObservation(threadID, turnID string, baseline, current protocol.TokenUsageBreakdown) {
	delta, ok := subtractTokenUsage(current, baseline)
	if !ok {
		return
	}
	rootThreadID, parentToolCallID, isChild := a.eventScope(threadID)
	if threadID != rootThreadID && !isChild {
		return
	}
	a.mu.Lock()
	key := nativeTurnKey(threadID, turnID)
	if a.turnResponseObserved[key] || a.turnFallbackSelected[key] {
		a.mu.Unlock()
		return
	}
	a.turnFallbackSelected[key] = true
	model := a.turnModels[key]
	generation := a.turnGenerations[key]
	a.mu.Unlock()
	usage, observation, ok := normalizeRawResponseUsage(delta, threadID, turnID, "", isChild, model)
	if !ok {
		return
	}
	usage.Estimated = true
	observation.Source = nativeUsageSourceTurnFallback
	observation.Completeness = "estimated"
	a.emit(streams.AgentEvent{
		Type:             streams.EventTypeUsageObservation,
		SessionID:        rootThreadID,
		OperationID:      turnID,
		PromptGeneration: generation,
		ParentToolCallID: parentToolCallID,
		Usage:            usage,
		UsageObservation: observation,
	})
}

func normalizeRawResponseUsage(raw protocol.TokenUsageBreakdown, threadID, turnID, responseID string, child bool, model string) (*streams.PromptUsage, *streams.NativeUsageObservation, bool) {
	if raw.InputTokens < 0 || raw.CachedInputTokens < 0 || raw.CacheWriteInputTokens < 0 || raw.OutputTokens < 0 || raw.ReasoningOutputTokens < 0 || raw.TotalTokens < 0 || raw.CachedInputTokens > raw.InputTokens || raw.ReasoningOutputTokens > raw.OutputTokens {
		return nil, nil, false
	}
	input := raw.InputTokens - raw.CachedInputTokens
	total, ok := sumNativeTokens(input, raw.CachedInputTokens, raw.OutputTokens)
	if !ok {
		return nil, nil, false
	}
	reasoning := raw.ReasoningOutputTokens
	cacheWrite := raw.CacheWriteInputTokens
	reportedTotal := raw.TotalTokens
	scope := "direct"
	if child {
		scope = "child"
	}
	usage := &streams.PromptUsage{
		InputTokens:         input,
		CachedReadTokens:    raw.CachedInputTokens,
		OutputTokens:        raw.OutputTokens,
		OutputTokensPresent: true,
		TotalTokens:         total,
		PriceSuppressed:     raw.CacheWriteInputTokens > 0,
	}
	observation := &streams.NativeUsageObservation{
		SchemaVersion:            nativeUsageSchemaVersion,
		Source:                   "response",
		ProviderThreadID:         threadID,
		ProviderTurnID:           turnID,
		ProviderResponseID:       responseID,
		Scope:                    scope,
		Completeness:             "exact",
		Model:                    model,
		ReasoningOutputTokens:    &reasoning,
		ReportedCacheWriteTokens: &cacheWrite,
		ReportedTotalTokens:      &reportedTotal,
		PriceSuppressed:          cacheWrite > 0,
	}
	return usage, observation, true
}

func subtractTokenUsage(current, baseline protocol.TokenUsageBreakdown) (protocol.TokenUsageBreakdown, bool) {
	values := []struct {
		current int64
		base    int64
	}{
		{current.InputTokens, baseline.InputTokens},
		{current.CachedInputTokens, baseline.CachedInputTokens},
		{current.CacheWriteInputTokens, baseline.CacheWriteInputTokens},
		{current.OutputTokens, baseline.OutputTokens},
		{current.ReasoningOutputTokens, baseline.ReasoningOutputTokens},
		{current.TotalTokens, baseline.TotalTokens},
	}
	deltas := make([]int64, len(values))
	for i, value := range values {
		if value.current < value.base {
			return protocol.TokenUsageBreakdown{}, false
		}
		deltas[i] = value.current - value.base
	}
	return protocol.TokenUsageBreakdown{
		InputTokens:           deltas[0],
		CachedInputTokens:     deltas[1],
		CacheWriteInputTokens: deltas[2],
		OutputTokens:          deltas[3],
		ReasoningOutputTokens: deltas[4],
		TotalTokens:           deltas[5],
	}, true
}

func sumNativeTokens(values ...int64) (int64, bool) {
	var total int64
	for _, value := range values {
		if value < 0 || total > math.MaxInt64-value {
			return 0, false
		}
		total += value
	}
	return total, true
}

func nativeTurnKey(threadID, turnID string) string {
	return fmt.Sprintf("%s\x00%s", threadID, turnID)
}
