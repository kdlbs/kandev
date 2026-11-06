package acp

import (
	"strings"

	acpsdk "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

const capacityContinuationToolLimit = 256

type capacityToolEvidence struct {
	status acpsdk.ToolCallStatus
	kind   acpsdk.ToolKind
	failed bool
	// mcp marks an MCP-over-ACP call. Codex reports these as execute-kind
	// tools, but they are not shell commands and never qualify for the
	// known-command-failure exception.
	mcp bool
	// terminal carries the V2 command-outcome evidence captured before any
	// synthetic prompt cleanup can make an unresolved tool look terminal.
	terminal capacityTerminalEvidence
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

// capacityTerminalEvidence is the bounded terminal proof for one V2 command.
// invalid permanently blocks this tool's evidence once any partial,
// contradictory, signalled, or cancelled frame is observed.
type capacityTerminalEvidence struct {
	seen        bool
	exitCode    int
	rawSeen     bool
	rawExitCode int
	invalid     bool
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
	knownCommandSupport := snapshot.SupportsKnownFailedShellTools()
	for _, tool := range tools {
		if tool.detached && !tool.detachedResolved {
			// A completed launch tool is not completed workload evidence.
			snapshot.UnaccountedBackground = true
		}
		if tool.terminal.invalid {
			snapshot.UnknownOutcomes = true
			continue
		}
		if knownCommandSupport && tool.knownFailedCommand() {
			snapshot.KnownFailedShellTools++
			continue
		}
		if tool.failed || tool.status == acpsdk.ToolCallStatusFailed {
			snapshot.FailedTools = true
			continue
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

// knownFailedCommand reports V2 evidence for a command that ended at a known
// nonzero terminal exit. The real failed status stays in tool history.
func (t capacityToolEvidence) knownFailedCommand() bool {
	return t.kind == acpsdk.ToolKindExecute && !t.mcp && !t.terminal.invalid &&
		t.terminal.seen && t.terminal.exitCode != 0 &&
		(t.status == acpsdk.ToolCallStatusCompleted || t.status == acpsdk.ToolCallStatusFailed)
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
	support := a.dialect.capacityContinuationSupport
	detachedShapes := support == streams.CapacityContinuationOpenCodeLiveSessionV1
	knownCommandShapes := capacitySupportAcceptsKnownCommands(support)
	update := n.Update
	if update.ToolCall != nil {
		turn.observeCapacityToolCall(update.ToolCall, detachedShapes, knownCommandShapes)
	}
	if update.ToolCallUpdate != nil {
		turn.observeCapacityToolUpdate(update.ToolCallUpdate, detachedShapes, knownCommandShapes)
	}
}

func capacitySupportAcceptsKnownCommands(support streams.CapacityContinuationSupport) bool {
	return support == streams.CapacityContinuationCodexLiveSessionV2 ||
		support == streams.CapacityContinuationMockLiveSessionV2
}

func (t *promptTurnState) observeCapacityToolCall(
	call *acpsdk.SessionUpdateToolCall,
	detachedShapes bool,
	knownCommandShapes bool,
) {
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
	if codexSubagentSignalFromFrame(call.Meta, call.Title, call.RawInput) != codexSubagentSignalNone || parentToolUseID(call.Meta) != "" {
		t.capacityBackground = true
	}
	if !validCapacityToolStatus(call.Status) {
		t.capacityUnknown = true
	}
	evidence := capacityToolEvidence{
		status: call.Status, kind: call.Kind,
		mcp: capacityMCPToolCall(call.Meta, call.RawInput),
	}
	if knownCommandShapes {
		t.observeCapacityTerminalIdentity(&evidence.terminal, id, call.Meta)
	}
	if detachedShapes {
		t.observeCapacityDetachedLaunch(&evidence, call.Kind, call.RawInput)
	}
	// Keep the ID so duplicate calls and later updates cannot become fresh evidence.
	t.capacityTools[id] = evidence
}

// capacityMCPToolCall recognizes Codex's MCP-over-ACP envelope in either the
// explicit marker or the complete structured request.
func capacityMCPToolCall(meta map[string]any, rawInput any) bool {
	if marker, ok := meta["is_mcp_tool_call"].(bool); ok && marker {
		return true
	}
	_, ok := parseCodexMCPToolCall(meta, rawInput)
	return ok
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

func (t *promptTurnState) observeCapacityToolUpdate(
	update *acpsdk.SessionToolCallUpdate,
	detachedShapes bool,
	knownCommandShapes bool,
) {
	id := string(update.ToolCallId)
	tool, exists := t.capacityTools[id]
	if id == "" || !exists {
		t.capacityUnknown = true
		return
	}
	title := ""
	if update.Title != nil {
		title = *update.Title
	}
	if codexSubagentSignalFromFrame(update.Meta, title, update.RawInput) != codexSubagentSignalNone || parentToolUseID(update.Meta) != "" {
		t.capacityBackground = true
	}
	if capacityMCPToolCall(update.Meta, update.RawInput) {
		tool.mcp = true
	}
	t.applyCapacityToolKind(&tool, update.Kind)
	if knownCommandShapes {
		t.observeCapacityTerminalEvidence(&tool, id, update)
	}
	if detachedShapes {
		t.observeCapacityDetachment(&tool, update)
	}
	t.applyCapacityToolStatus(&tool, update)
	if tool.kind == acpsdk.ToolKindExecute && capacityToolHasFailedExit(update) {
		tool.failed = true
	}
	t.capacityTools[id] = tool
}

// observeCapacityTerminalEvidence captures the V2 proof from the same ordered
// notification that carries the tool update. Any partial, malformed,
// signalled, cancelled, or conflicting evidence permanently blocks the tool.
func (t *promptTurnState) observeCapacityTerminalEvidence(
	tool *capacityToolEvidence,
	toolID string,
	update *acpsdk.SessionToolCallUpdate,
) {
	if update.Meta != nil {
		t.observeCapacityTerminalIdentity(&tool.terminal, toolID, update.Meta)
		if exit, ok := update.Meta["terminal_exit"]; ok {
			t.observeCapacityTerminalExit(&tool.terminal, toolID, exit)
		}
	}
	if code, present, valid := capacityRawExitEvidence(update.RawOutput); present {
		switch {
		case !valid:
			tool.terminal.invalid = true
		case tool.terminal.rawSeen && tool.terminal.rawExitCode != code:
			tool.terminal.invalid = true
		case !tool.terminal.rawSeen:
			tool.terminal.rawSeen = true
			tool.terminal.rawExitCode = code
		}
	}
	// Terminal and raw exit evidence may arrive on the same frame or on
	// separate updates in either order. Once both are present they must
	// agree or the tool stays permanently blocked.
	if tool.terminal.seen && tool.terminal.rawSeen &&
		tool.terminal.exitCode != tool.terminal.rawExitCode {
		tool.terminal.invalid = true
	}
}

func (t *promptTurnState) observeCapacityTerminalIdentity(
	terminal *capacityTerminalEvidence,
	toolID string,
	meta map[string]any,
) {
	for _, key := range []string{"terminal_info", "terminal_output", "terminal_exit"} {
		value, present := meta[key]
		if !present {
			continue
		}
		fields, ok := value.(map[string]any)
		if !ok {
			terminal.invalid = true
			continue
		}
		terminalID, present := fields["terminal_id"]
		if !present {
			continue
		}
		id, ok := terminalID.(string)
		if !ok || id == "" || id != toolID {
			terminal.invalid = true
		}
	}
}

func (t *promptTurnState) observeCapacityTerminalExit(
	terminal *capacityTerminalEvidence,
	toolID string,
	value any,
) {
	fields, ok := value.(map[string]any)
	if !ok {
		terminal.invalid = true
		return
	}
	terminalID, ok := fields["terminal_id"].(string)
	if !ok || terminalID == "" || terminalID != toolID {
		terminal.invalid = true
		return
	}
	if capacityTerminalSignal(fields) || capacityTerminalCancelled(fields) {
		terminal.invalid = true
		return
	}
	code, ok := capacityTerminalExitCode(fields)
	if !ok {
		terminal.invalid = true
		return
	}
	if terminal.seen && terminal.exitCode != code {
		terminal.invalid = true
		return
	}
	terminal.seen = true
	terminal.exitCode = code
}

func capacityTerminalSignal(fields map[string]any) bool {
	signal, present := fields["signal"]
	return present && signal != nil
}

func capacityTerminalCancelled(fields map[string]any) bool {
	for _, key := range []string{"cancelled", "canceled"} {
		value, present := fields[key]
		if !present || value == nil {
			continue
		}
		if cancelled, ok := value.(bool); !ok || cancelled {
			return true
		}
	}
	return false
}

func capacityTerminalExitCode(fields map[string]any) (int, bool) {
	code := 0
	found := false
	for _, key := range []string{"exit_code", "code"} {
		value, present := fields[key]
		if !present {
			continue
		}
		parsed, ok := authoritativeTerminalExitCode(value)
		if !ok || (found && parsed != code) {
			return 0, false
		}
		code, found = parsed, true
	}
	return code, found
}

// authoritativeTerminalExitCode rejects string-shaped exit values: V2 requires
// an integer terminal exit from the provider's terminal extension.
func authoritativeTerminalExitCode(value any) (int, bool) {
	if _, isString := value.(string); isString {
		return 0, false
	}
	return parseExitCode(value)
}

// capacityRawExitEvidence reports a raw exit value separately from whether the
// field was present and whether it parsed. A present malformed value must block
// the tool instead of being treated as absent evidence.
func capacityRawExitEvidence(rawOutput any) (int, bool, bool) {
	value := unwrapShellRawOutput(rawOutput)
	switch result := value.(type) {
	case map[string]any:
		return capacityRawMapExitEvidence(result)
	case string:
		return capacityRawTextExitEvidence(result)
	default:
		return 0, false, true
	}
}

func capacityRawMapExitEvidence(result map[string]any) (int, bool, bool) {
	if raw, present := result["exit_code"]; present {
		code, ok := parseExitCode(raw)
		return code, true, ok
	}
	if metadata, ok := result["metadata"].(map[string]any); ok {
		if raw, present := metadata["exit"]; present {
			code, ok := parseExitCode(raw)
			return code, true, ok
		}
	}
	if _, explicit := result["stdout"]; explicit {
		return 0, false, true
	}
	if _, explicit := result["stderr"]; explicit {
		return 0, false, true
	}
	if formatted, ok := result["formatted_output"].(string); ok {
		return capacityRawTextExitEvidence(formatted)
	}
	if output, ok := result["output"].(string); ok {
		return capacityRawTextExitEvidence(output)
	}
	return 0, false, true
}

func capacityRawTextExitEvidence(text string) (int, bool, bool) {
	if !strings.Contains(text, "<return-code>") {
		return 0, false, true
	}
	codeText, present := embeddedShellField(text, "return-code")
	if !present {
		return 0, false, true
	}
	code, ok := parseExitCode(codeText)
	return code, true, ok
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
