package acp

import (
	acpsdk "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

const capacityContinuationToolLimit = 256

type capacityToolEvidence struct {
	status acpsdk.ToolCallStatus
	kind   acpsdk.ToolKind
	failed bool
	// detached marks an execute call that asked for a detached job. The
	// workload stays unresolved until its own terminal evidence arrives,
	// independent of the background-inspection presentation feature.
	detached bool
	// detachedWorkloadID is the native shell identity a running launch reported.
	detachedWorkloadID string
	// detachedResolved is set only by terminal workload evidence that does not
	// contradict that identity.
	detachedResolved bool
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
		if tool.failed {
			snapshot.FailedTools = true
			continue
		}
		if tool.detached && !tool.detachedResolved {
			// A completed launch tool is not completed workload evidence.
			snapshot.UnaccountedBackground = true
		}
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
	detachedShapes := a.dialect.capacityContinuationSupport == streams.CapacityContinuationOpenCodeLiveSessionV1
	update := n.Update
	if update.ToolCall != nil {
		turn.observeCapacityToolCall(update.ToolCall, detachedShapes)
	}
	if update.ToolCallUpdate != nil {
		turn.observeCapacityToolUpdate(update.ToolCallUpdate, detachedShapes)
	}
}

func (t *promptTurnState) observeCapacityToolCall(call *acpsdk.SessionUpdateToolCall, detachedShapes bool) {
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
	evidence := capacityToolEvidence{status: call.Status, kind: call.Kind}
	if detachedShapes {
		t.observeCapacityDetachedLaunch(&evidence, call.Kind, call.RawInput)
	}
	// Keep the ID so duplicate calls and later updates cannot become fresh evidence.
	t.capacityTools[id] = evidence
}

// observeCapacityDetachedLaunch records a detached-job request from either
// frame. A flag on a non-execute call is contradictory evidence and fails
// closed; a confirmed launch stays unresolved until terminal workload evidence.
func (t *promptTurnState) observeCapacityDetachedLaunch(
	evidence *capacityToolEvidence,
	kind acpsdk.ToolKind,
	rawInput any,
) {
	input, ok := rawInput.(map[string]any)
	if !ok || !isBackgroundExecInput(input) {
		return
	}
	if kind != acpsdk.ToolKindExecute {
		t.capacityBackground = true
		return
	}
	evidence.detached = true
}

func (t *promptTurnState) observeCapacityToolUpdate(update *acpsdk.SessionToolCallUpdate, detachedShapes bool) {
	id := string(update.ToolCallId)
	tool, exists := t.capacityTools[id]
	if id == "" || !exists {
		t.capacityUnknown = true
		return
	}
	if codexSubagentSignalFromMeta(update.Meta) != codexSubagentSignalNone || parentToolUseID(update.Meta) != "" {
		t.capacityBackground = true
	}
	t.applyCapacityToolKind(&tool, update.Kind)
	if detachedShapes {
		t.observeCapacityDetachment(&tool, update)
	}
	t.applyCapacityToolStatus(&tool, update)
	if tool.kind == acpsdk.ToolKindExecute && capacityToolHasFailedExit(update) {
		tool.failed = true
	}
	t.capacityTools[id] = tool
}

func (t *promptTurnState) applyCapacityToolKind(tool *capacityToolEvidence, kind *acpsdk.ToolKind) {
	if kind == nil {
		return
	}
	if tool.kind != "" && tool.kind != *kind {
		t.capacityUnknown = true
		return
	}
	if tool.kind == "" {
		tool.kind = *kind
	}
}

func (t *promptTurnState) applyCapacityToolStatus(tool *capacityToolEvidence, update *acpsdk.SessionToolCallUpdate) {
	if update.Status == nil {
		return
	}
	if !validCapacityToolStatus(*update.Status) || !validCapacityStatusTransition(tool.status, *update.Status) {
		t.capacityUnknown = true
	}
	if validCapacityToolStatus(*update.Status) {
		tool.status = *update.Status
	}
}

func (t *promptTurnState) observeCapacityDetachment(tool *capacityToolEvidence, update *acpsdk.SessionToolCallUpdate) {
	if !tool.detached {
		t.observeCapacityDetachedLaunch(tool, tool.kind, update.RawInput)
	}
	if tool.detached {
		t.observeCapacityDetachedWorkload(tool, update)
	}
}

// observeCapacityDetachedWorkload settles a detached launch only on terminal
// evidence for the same workload. A running shell keeps it unresolved, and a
// contradictory identity fails closed instead of resolving the job.
func (t *promptTurnState) observeCapacityDetachedWorkload(
	tool *capacityToolEvidence,
	update *acpsdk.SessionToolCallUpdate,
) {
	if launch, ok := recognizeOpenCodeBackgroundLaunch(update.RawOutput); ok {
		switch {
		case tool.detachedWorkloadID == "":
			tool.detachedWorkloadID = launch.ShellID
		case tool.detachedWorkloadID != launch.ShellID:
			t.capacityBackground = true
		}
		return
	}
	status, shellID, conflict, hasStatus := capacityShellMetadata(update.RawOutput)
	if conflict {
		t.capacityBackground = true
		return
	}
	if hasStatus {
		if tool.detachedWorkloadID != "" && shellID != "" && shellID != tool.detachedWorkloadID {
			t.capacityBackground = true
			return
		}
		if status == openCodeShellRunningStatus {
			return
		}
	}
	// Without a launch identity there is no way to prove which workload
	// finished, so an exit code alone never resolves the detached call.
	if tool.detachedWorkloadID == "" || !capacityObservedShellExit(update) {
		return
	}
	tool.detachedResolved = true
}

// capacityShellMetadata exposes a shell status/identity pair even when the
// status is not "running", so a terminal result can still be correlated.
// Conflict reports two present, disagreeing identity spellings, which is
// contradictory evidence even when the status is missing.
func capacityShellMetadata(result any) (status, shellID string, conflict, ok bool) {
	metadata, ok := openCodeShellMetadata(result)
	if !ok {
		return "", "", false, false
	}
	shellID, idOK := openCodeShellID(metadata)
	if !idOK {
		if _, upperOK := metadataString(metadata, "shellID"); upperOK {
			if _, lowerOK := metadataString(metadata, "shellId"); lowerOK {
				conflict = true
			}
		}
	}
	status, ok = openCodeShellStatus(metadata)
	if !ok {
		return "", "", conflict, false
	}
	return status, shellID, conflict, true
}

func capacityObservedShellExit(update *acpsdk.SessionToolCallUpdate) bool {
	if _, ok := terminalExitCode(update.Meta); ok {
		return true
	}
	return normalizeFinalShellResult(update.RawOutput).exitCode != nil
}

func capacityToolHasFailedExit(update *acpsdk.SessionToolCallUpdate) bool {
	if update == nil || update.Status == nil || *update.Status != acpsdk.ToolCallStatusCompleted {
		return false
	}
	if code, ok := terminalExitCode(update.Meta); ok {
		return code != 0
	}
	result := normalizeFinalShellResult(update.RawOutput)
	return result.exitCode != nil && *result.exitCode != 0
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
