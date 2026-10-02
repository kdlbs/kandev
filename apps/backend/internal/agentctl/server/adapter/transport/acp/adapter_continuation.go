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
	a.mu.RUnlock()
	if !enabled || turn == nil || turn.promptGeneration == 0 {
		return nil
	}
	turn.evidenceMu.Lock()
	defer turn.evidenceMu.Unlock()
	snapshot := &streams.ContinuationSafetySnapshot{Support: a.dialect.continuationSupport, PromptGeneration: turn.promptGeneration, Known: true, Unsafe: turn.continuationUnsafe}
	for _, completed := range turn.continuationTools {
		if completed {
			snapshot.CompletedReads++
		} else {
			snapshot.Pending = true
		}
	}
	return snapshot
}

func (a *Adapter) observeContinuationSafety(n acpsdk.SessionNotification, generation uint64) {
	a.mu.RLock()
	enabled := a.cfg.ProviderInterruptionContinuation && a.dialect.continuationSupport != "" &&
		string(n.SessionId) == a.sessionID && !a.isLoadingSession
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
	turn.observeContinuationCall(u.ToolCall)
	turn.observeContinuationUpdate(u.ToolCallUpdate)
}

func (t *promptTurnState) observeContinuationCall(call *acpsdk.SessionUpdateToolCall) {
	if call == nil {
		return
	}
	id := string(call.ToolCallId)
	_, duplicate := t.continuationTools[id]
	if id == "" || duplicate || len(t.continuationTools) >= continuationToolLimit || call.Kind != acpsdk.ToolKindRead ||
		!continuationReadPayload(call.Meta, call.RawInput) || !continuationReadStatus(call.Status) {
		t.continuationUnsafe = true
		return
	}
	if t.continuationTools == nil {
		t.continuationTools = make(map[string]bool)
	}
	t.continuationTools[id] = call.Status == acpsdk.ToolCallStatusCompleted
}

func (t *promptTurnState) observeContinuationUpdate(update *acpsdk.SessionToolCallUpdate) {
	if update == nil {
		return
	}
	id := string(update.ToolCallId)
	completed, exists := t.continuationTools[id]
	if !exists || !continuationReadPayload(update.Meta, update.RawInput) ||
		(update.Kind != nil && *update.Kind != acpsdk.ToolKindRead) ||
		(update.Status != nil && (!continuationReadStatus(*update.Status) || completed && *update.Status != acpsdk.ToolCallStatusCompleted)) {
		t.continuationUnsafe = true
		return
	}
	if update.Status != nil {
		t.continuationTools[id] = *update.Status == acpsdk.ToolCallStatusCompleted
	}
}

func continuationReadStatus(status acpsdk.ToolCallStatus) bool {
	return status == acpsdk.ToolCallStatusPending || status == acpsdk.ToolCallStatusInProgress || status == acpsdk.ToolCallStatusCompleted
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
	if !a.cfg.ProviderInterruptionContinuation {
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
