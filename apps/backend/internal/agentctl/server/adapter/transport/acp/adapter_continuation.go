package acp

import (
	acpsdk "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

const continuationToolLimit = 256

func (a *Adapter) continuationSafetySnapshot(turn *promptTurnState) *streams.ContinuationSafetySnapshot {
	a.mu.RLock()
	enabled := a.cfg.ProviderInterruptionContinuation && a.dialect.continuationSupport != "" &&
		(a.capabilities.LoadSession || a.capabilities.SessionCapabilities.Resume != nil) && a.sessionID != ""
	support := a.dialect.continuationSupport
	a.mu.RUnlock()
	if !enabled || turn == nil || turn.promptGeneration == 0 {
		return nil
	}
	turn.evidenceMu.Lock()
	defer turn.evidenceMu.Unlock()
	snapshot := &streams.ContinuationSafetySnapshot{
		Support:          support,
		PromptGeneration: turn.promptGeneration,
		Known:            true,
		Unsafe:           turn.continuationUnsafe,
		Pending:          turn.continuationPermissions != 0,
	}
	for _, completed := range turn.continuationTools {
		if completed {
			if support == streams.ContinuationNativeSavedHistoryV1 {
				snapshot.CompletedReads++
			} else {
				snapshot.CompletedTools++
			}
		} else {
			snapshot.Pending = true
		}
	}
	for id := range turn.continuationPermissionTools {
		if !turn.continuationTools[id] {
			snapshot.Pending = true
		}
	}
	return snapshot
}

func (a *Adapter) observeContinuationSafety(n acpsdk.SessionNotification, generation uint64) {
	a.mu.RLock()
	enabled := a.cfg.ProviderInterruptionContinuation && a.dialect.continuationSupport != "" &&
		string(n.SessionId) == a.sessionID && !a.isLoadingSession
	support := a.dialect.continuationSupport
	a.mu.RUnlock()
	turn := a.currentPromptTurn()
	if !enabled || turn == nil || generation == 0 || turn.promptGeneration != generation {
		return
	}
	turn.evidenceMu.Lock()
	defer turn.evidenceMu.Unlock()
	if turn.continuationUnsafe {
		return
	}
	u := n.Update
	turn.observeContinuationCall(u.ToolCall, support)
	turn.observeContinuationUpdate(u.ToolCallUpdate, support)
}

func (t *promptTurnState) observeContinuationCall(call *acpsdk.SessionUpdateToolCall, support streams.ContinuationSupport) {
	if call == nil {
		return
	}
	id := string(call.ToolCallId)
	_, duplicate := t.continuationTools[id]
	if id == "" || duplicate || len(t.continuationTools) >= continuationToolLimit || !continuationStatusValid(call.Status) {
		t.continuationUnsafe = true
		return
	}
	if continuationHasUnownedWork(call.Meta, call.Title, call.RawInput, call.RawOutput) {
		t.continuationUnsafe = true
		return
	}
	if support == streams.ContinuationNativeSavedHistoryV1 {
		if call.Kind != acpsdk.ToolKindRead || !continuationReadPayload(call.Meta, call.RawInput) {
			t.continuationUnsafe = true
			return
		}
	}
	if t.continuationTools == nil {
		t.continuationTools = make(map[string]bool)
	}
	t.continuationTools[id] = call.Status == acpsdk.ToolCallStatusCompleted
}

func (t *promptTurnState) observeContinuationUpdate(update *acpsdk.SessionToolCallUpdate, support streams.ContinuationSupport) {
	if update == nil {
		return
	}
	id := string(update.ToolCallId)
	completed, exists := t.continuationTools[id]
	if !exists {
		t.continuationUnsafe = true
		return
	}
	title := ""
	if update.Title != nil {
		title = *update.Title
	}
	if continuationHasUnownedWork(update.Meta, title, update.RawInput, update.RawOutput) {
		t.continuationUnsafe = true
		return
	}
	if support == streams.ContinuationNativeSavedHistoryV1 {
		if !continuationReadPayload(update.Meta, update.RawInput) ||
			(update.Kind != nil && *update.Kind != acpsdk.ToolKindRead) {
			t.continuationUnsafe = true
			return
		}
	}
	if update.Status != nil {
		if !continuationStatusValid(*update.Status) || (completed && *update.Status != acpsdk.ToolCallStatusCompleted) {
			t.continuationUnsafe = true
			return
		}
		t.continuationTools[id] = *update.Status == acpsdk.ToolCallStatusCompleted
	}
}

func continuationStatusValid(status acpsdk.ToolCallStatus) bool {
	return status == acpsdk.ToolCallStatusPending || status == acpsdk.ToolCallStatusInProgress || status == acpsdk.ToolCallStatusCompleted
}

func continuationHasUnownedWork(meta map[string]any, title string, input, output any) bool {
	if isSubagentOrBackgroundMeta(meta) || parentToolUseID(meta) != "" || isSubagentSignal(meta, title, input) {
		return true
	}
	if rawInput, ok := input.(map[string]any); ok && isBackgroundExecInput(rawInput) {
		return true
	}
	if rawOutput, ok := output.(map[string]any); ok {
		var result SubagentTaskResult
		return cursorSubagentResult(rawOutput, &result) && result.IsAsync
	}
	return false
}

func isSubagentOrBackgroundMeta(meta map[string]any) bool {
	if meta == nil {
		return false
	}
	if isBg, ok := meta["isBackground"].(bool); ok && isBg {
		return true
	}
	if parentID, ok := meta["parentToolUseId"].(string); ok && parentID != "" {
		return true
	}
	if parentID, ok := meta["parentToolCallId"].(string); ok && parentID != "" {
		return true
	}
	return false
}

// Only metadata fields verified on the Cursor read wire are accepted.
func continuationReadPayload(meta map[string]any, input any) bool {
	for key := range meta {
		if key != "durationMs" {
			return false
		}
	}
	if input == nil {
		return true
	}
	raw, ok := input.(map[string]any)
	if !ok {
		return false
	}
	for key := range raw {
		if key != "path" && key != "offset" && key != "limit" {
			return false
		}
	}
	return true
}

func (a *Adapter) poisonContinuationSafety() {
	a.mu.RLock()
	enabled := a.cfg.ProviderInterruptionContinuation
	a.mu.RUnlock()
	if !enabled {
		return
	}
	turn := a.currentPromptTurn()
	if turn == nil {
		return
	}
	turn.evidenceMu.Lock()
	turn.continuationUnsafe = true
	turn.evidenceMu.Unlock()
}
