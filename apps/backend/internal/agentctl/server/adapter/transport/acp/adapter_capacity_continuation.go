package acp

import (
	acpsdk "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

const capacityContinuationToolLimit = 256

type capacityToolEvidence struct {
	status acpsdk.ToolCallStatus
	kind   acpsdk.ToolKind
}

func (a *Adapter) capacityContinuationSnapshot(
	turn *promptTurnState,
	notificationsDrained bool,
) *streams.CapacityContinuationSnapshot {
	a.mu.RLock()
	support := a.dialect.capacityContinuationSupport
	sessionID := a.sessionID
	active := sessionID != ""
	unaccountedBackground := a.sessionHasUnresolvedBackgroundWorkLocked(sessionID)
	a.mu.RUnlock()
	if !active || !notificationsDrained || turn == nil || turn.promptGeneration == 0 || support == "" {
		return nil
	}
	turn.evidenceMu.Lock()
	defer turn.evidenceMu.Unlock()
	snapshot := &streams.CapacityContinuationSnapshot{
		Support: support, PromptGeneration: turn.promptGeneration,
		EvidenceComplete: true, UnknownOutcomes: turn.capacityUnknown,
		PermissionPending:     turn.capacityPermissions > 0,
		UnaccountedBackground: turn.capacityBackground || unaccountedBackground,
	}
	addCapacityToolOutcomes(snapshot, turn.capacityTools)
	return snapshot
}

func (a *Adapter) sessionHasUnresolvedBackgroundWorkLocked(sessionID string) bool {
	for _, payload := range a.activeToolCalls {
		if isUnresolvedBackgroundPayload(payload) {
			return true
		}
	}
	return len(a.activeMonitors[sessionID]) > 0
}

func isUnresolvedBackgroundPayload(payload *streams.NormalizedPayload) bool {
	return payload != nil && (payload.IsActiveBackgroundWork() || payload.Kind() == streams.ToolKindSubagentTask)
}

func addCapacityToolOutcomes(snapshot *streams.CapacityContinuationSnapshot, tools map[string]capacityToolEvidence) {
	for _, tool := range tools {
		switch tool.status {
		case acpsdk.ToolCallStatusPending, acpsdk.ToolCallStatusInProgress:
			snapshot.PendingTools = true
		case acpsdk.ToolCallStatusCompleted:
			snapshot.CompletedTools++
		case acpsdk.ToolCallStatusFailed:
			snapshot.FailedTools = true
		default:
			snapshot.UnknownOutcomes = true
		}
	}
}

func (a *Adapter) observeCapacityContinuation(n acpsdk.SessionNotification, generation uint64) {
	a.mu.RLock()
	enabled := a.dialect.capacityContinuationSupport != "" && string(n.SessionId) == a.sessionID && !a.isLoadingSession
	a.mu.RUnlock()
	turn := a.currentPromptTurn()
	if !enabled || turn == nil || generation == 0 || turn.promptGeneration != generation {
		return
	}
	turn.evidenceMu.Lock()
	defer turn.evidenceMu.Unlock()
	update := n.Update
	if update.ToolCall != nil {
		turn.observeCapacityToolCall(update.ToolCall)
	}
	if update.ToolCallUpdate != nil {
		turn.observeCapacityToolUpdate(update.ToolCallUpdate)
	}
}

func (t *promptTurnState) observeCapacityToolCall(call *acpsdk.SessionUpdateToolCall) {
	if t.capacityTools == nil {
		t.capacityTools = make(map[string]capacityToolEvidence)
	}
	id := string(call.ToolCallId)
	if id == "" {
		t.capacityUnknown = true
		return
	}
	if _, exists := t.capacityTools[id]; exists {
		t.capacityUnknown = true
		return
	}
	if len(t.capacityTools) >= capacityContinuationToolLimit {
		t.capacityUnknown = true
		return
	}
	if codexSubagentSignalFromMeta(call.Meta) != codexSubagentSignalNone || parentToolUseID(call.Meta) != "" {
		t.capacityBackground = true
	}
	if !validCapacityToolStatus(call.Status) {
		t.capacityUnknown = true
	}
	t.capacityTools[id] = capacityToolEvidence{status: call.Status, kind: call.Kind}
}

func (t *promptTurnState) observeCapacityToolUpdate(update *acpsdk.SessionToolCallUpdate) {
	id := string(update.ToolCallId)
	tool, exists := t.capacityTools[id]
	if id == "" || !exists {
		t.capacityUnknown = true
		return
	}
	if codexSubagentSignalFromMeta(update.Meta) != codexSubagentSignalNone || parentToolUseID(update.Meta) != "" {
		t.capacityBackground = true
	}
	if update.Kind != nil {
		if tool.kind != "" && tool.kind != *update.Kind {
			t.capacityUnknown = true
		} else if tool.kind == "" {
			tool.kind = *update.Kind
		}
	}
	if update.Status != nil {
		if !validCapacityToolStatus(*update.Status) || !validCapacityStatusTransition(tool.status, *update.Status) {
			t.capacityUnknown = true
		}
		if validCapacityToolStatus(*update.Status) {
			tool.status = *update.Status
		}
	}
	t.capacityTools[id] = tool
}

func validCapacityToolStatus(status acpsdk.ToolCallStatus) bool {
	switch status {
	case acpsdk.ToolCallStatusPending, acpsdk.ToolCallStatusInProgress,
		acpsdk.ToolCallStatusCompleted, acpsdk.ToolCallStatusFailed:
		return true
	default:
		return false
	}
}

func validCapacityStatusTransition(current, next acpsdk.ToolCallStatus) bool {
	switch current {
	case acpsdk.ToolCallStatusPending:
		return next == acpsdk.ToolCallStatusPending || next == acpsdk.ToolCallStatusInProgress ||
			next == acpsdk.ToolCallStatusCompleted || next == acpsdk.ToolCallStatusFailed
	case acpsdk.ToolCallStatusInProgress:
		return next == acpsdk.ToolCallStatusInProgress || next == acpsdk.ToolCallStatusCompleted || next == acpsdk.ToolCallStatusFailed
	case acpsdk.ToolCallStatusCompleted:
		return next == acpsdk.ToolCallStatusCompleted
	case acpsdk.ToolCallStatusFailed:
		return next == acpsdk.ToolCallStatusFailed
	default:
		return false
	}
}

func (a *Adapter) capacityPermissionStarted(turn *promptTurnState, toolCallID string) {
	if turn == nil || a.dialect.capacityContinuationSupport == "" {
		return
	}
	turn.evidenceMu.Lock()
	turn.capacityPermissions++
	tool, exists := turn.capacityTools[toolCallID]
	if toolCallID == "" || !exists || tool.status != acpsdk.ToolCallStatusPending && tool.status != acpsdk.ToolCallStatusInProgress {
		turn.capacityUnknown = true
	}
	turn.evidenceMu.Unlock()
}

func (a *Adapter) capacityPermissionFinished(turn *promptTurnState) {
	if turn == nil || a.dialect.capacityContinuationSupport == "" {
		return
	}
	turn.evidenceMu.Lock()
	if turn.capacityPermissions == 0 {
		turn.capacityUnknown = true
	} else {
		turn.capacityPermissions--
	}
	turn.evidenceMu.Unlock()
}
