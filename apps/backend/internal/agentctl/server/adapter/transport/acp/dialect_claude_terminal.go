package acp

import (
	"strings"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

// Verified Claude bridge evidence for a detached shell's own terminal frame.
// The bridge retains a display terminal after a Bash command detaches and
// reports its exit in a separate tool_call_update. Older bridges attach the
// same metadata name to the launch result, so only the exact inspected tuple
// may interpret a null exit as job termination.
const (
	verifiedClaudeTerminalBridgeName    = "@agentclientprotocol/claude-agent-acp"
	verifiedClaudeTerminalBridgeVersion = "0.97.0"
)

// connectedAgentIdentity returns the immutable initialize identity of the
// connected bridge. Before initialize completes both values are empty, which
// never qualifies as verified evidence.
func (a *Adapter) connectedAgentIdentity() (string, string) {
	if a.agentInfo == nil {
		return "", ""
	}
	return a.agentInfo.Name, a.agentInfo.Version
}

// claudeBackgroundTerminalExit reports whether update is the verified terminal
// frame for the tracked detached shell identified by toolCallID. A null exit
// code proves the command ended without a reportable outcome, whether it exited
// normally (null signal) or was terminated by a signal. A numeric code keeps the
// existing terminal-code path, and malformed evidence establishes nothing.
func claudeBackgroundTerminalExit(
	agentID, agentName, agentVersion string,
	meta map[string]any,
	toolCallID string,
	payload *streams.NormalizedPayload,
) bool {
	if !verifiedClaudeTerminalBridge(agentID, agentName, agentVersion) {
		return false
	}
	if !claudeActiveDetachedShell(payload) {
		return false
	}
	terminal, ok := claudeTerminalExitEnvelope(meta, toolCallID)
	if !ok {
		return false
	}
	return claudeTerminalOutcomeUnavailable(terminal)
}

func verifiedClaudeTerminalBridge(agentID, agentName, agentVersion string) bool {
	return agentID == claudeAgentID &&
		agentName == verifiedClaudeTerminalBridgeName &&
		agentVersion == verifiedClaudeTerminalBridgeVersion
}

func claudeActiveDetachedShell(payload *streams.NormalizedPayload) bool {
	work := payload.BackgroundWork()
	return work != nil &&
		work.Kind == streams.BackgroundWorkKindShell &&
		work.Detached &&
		!work.Ended
}

func claudeTerminalExitEnvelope(meta map[string]any, toolCallID string) (map[string]any, bool) {
	terminal, ok := meta["terminal_exit"].(map[string]any)
	if !ok {
		return nil, false
	}
	terminalID, ok := terminal["terminal_id"].(string)
	if !ok || strings.TrimSpace(terminalID) == "" || terminalID != toolCallID {
		return nil, false
	}
	return terminal, true
}

func claudeTerminalOutcomeUnavailable(terminal map[string]any) bool {
	exitCode, hasExitCode := terminal["exit_code"]
	if !hasExitCode || exitCode != nil {
		return false
	}
	signal, hasSignal := terminal["signal"]
	if !hasSignal {
		return false
	}
	if signal == nil {
		return true
	}
	// A signal-terminated process has no exit code but still proves the
	// command ended. Only a non-empty signal string is valid evidence.
	signalName, ok := signal.(string)
	return ok && strings.TrimSpace(signalName) != ""
}

// preservesVerifiedClaudeBackgroundShell reports whether this adapter can
// still deliver a tracked shell's own terminal evidence after the foreground
// prompt ends. Only the exact verified bridge qualifies; legacy and unknown
// implementations keep their prompt-end cleanup fallback.
func (a *Adapter) preservesVerifiedClaudeBackgroundShell(payload *streams.NormalizedPayload) bool {
	name, version := a.connectedAgentIdentity()
	if !verifiedClaudeTerminalBridge(a.agentID, name, version) {
		return false
	}
	work := payload.BackgroundWork()
	return work != nil &&
		work.Kind == streams.BackgroundWorkKindShell &&
		work.Detached &&
		!work.Ended &&
		payload.IsVerifiedBackgroundWork() &&
		payload.IsActiveBackgroundWork()
}

// claudeBackgroundTerminalObservation builds the typed terminal observation
// for a command whose bridge-reported exit carries no outcome. It preserves the
// launch workload identity and reports no exit code: null is not zero.
func claudeBackgroundTerminalObservation(
	payload *streams.NormalizedPayload,
	sourceCallID string,
	finishedAt time.Time,
) *streams.WorkloadRunObservation {
	work := payload.BackgroundWork()
	shell := payload.ShellExec()
	if work == nil || shell == nil {
		return nil
	}
	workID := work.WorkID
	if workID == "" {
		workID = sourceCallID
	}
	title := shell.Description
	if title == "" {
		title = shell.Command
	}
	observation := &streams.WorkloadRunObservation{
		WorkID:       workID,
		RunID:        work.RunID,
		Kind:         streams.WorkloadKindShell,
		Title:        title,
		State:        streams.RunStateEnded,
		SourceCallID: sourceCallID,
		FinishedAt:   &finishedAt,
	}
	if work.Capabilities != nil {
		observation.Capabilities = *work.Capabilities
	}
	return observation
}
