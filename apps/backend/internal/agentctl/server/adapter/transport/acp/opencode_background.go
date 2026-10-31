package acp

import (
	"strings"

	"github.com/kandev/kandev/internal/agentctl/server/adapter/transport/acp/backgroundlaunch"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

const (
	// openCodeAgentID is the registry identifier for the OpenCode ACP agent.
	openCodeAgentID = "opencode-acp"

	// openCodeShellRunningStatus is the native shell status OpenCode reports
	// for a detached job that is still executing. Only this status confirms a
	// launch; any other value is missing or contradictory evidence.
	openCodeShellRunningStatus = "running"

	// openCodeBackgroundInputKey is the boolean a detached OpenCode shell tool
	// call carries in its input. It is the presentation flag, never proof by
	// itself that a job started.
	openCodeBackgroundInputKey = "background"
)

func init() {
	backgroundlaunch.Register(openCodeBackgroundLaunchRecognizer{})
}

// openCodeBackgroundLaunch is the structured detached-shell identity OpenCode
// reports in a launch result. ShellID is opaque, scoped to the owning session,
// and doubles as the shared workload and run identity.
type openCodeBackgroundLaunch struct {
	ShellID string
}

// recognizeOpenCodeBackgroundLaunch extracts the structured launch identity
// from one OpenCode execute/shell tool result. The human-readable background
// banner is never parsed, and content that only mentions an ID is not
// ownership evidence: the identity must come from the result metadata.
func recognizeOpenCodeBackgroundLaunch(result any) (openCodeBackgroundLaunch, bool) {
	metadata, ok := openCodeShellMetadata(result)
	if !ok {
		return openCodeBackgroundLaunch{}, false
	}
	status, ok := openCodeShellStatus(metadata)
	if !ok || status != openCodeShellRunningStatus {
		return openCodeBackgroundLaunch{}, false
	}
	shellID, ok := openCodeShellID(metadata)
	if !ok {
		return openCodeBackgroundLaunch{}, false
	}
	return openCodeBackgroundLaunch{ShellID: shellID}, true
}

// openCodeShellMetadata reads the result's metadata object, tolerating the
// {rawOutput: {...}} envelope some ACP hops add.
func openCodeShellMetadata(result any) (map[string]any, bool) {
	resultMap, ok := result.(map[string]any)
	if !ok {
		return nil, false
	}
	if raw, ok := resultMap["rawOutput"].(map[string]any); ok {
		resultMap = raw
	}
	metadata, ok := resultMap["metadata"].(map[string]any)
	if !ok {
		return nil, false
	}
	return metadata, true
}

// openCodeShellStatus reads metadata.status. A non-string or empty status is
// malformed evidence, not an assumption to fill in.
func openCodeShellStatus(metadata map[string]any) (string, bool) {
	value, ok := metadata["status"].(string)
	if !ok {
		return "", false
	}
	status := strings.ToLower(strings.TrimSpace(value))
	if status == "" {
		return "", false
	}
	return status, true
}

// openCodeShellID resolves the native shell identity. Both spellings are
// accepted only because the wire spelling is not frozen; two non-empty values
// that disagree are conflicting evidence and are rejected.
func openCodeShellID(metadata map[string]any) (string, bool) {
	upper, upperOK := metadataString(metadata, "shellID")
	lower, lowerOK := metadataString(metadata, "shellId")
	if upperOK && lowerOK && upper != lower {
		return "", false
	}
	if upperOK {
		return upper, true
	}
	if lowerOK {
		return lower, true
	}
	return "", false
}

func metadataString(metadata map[string]any, key string) (string, bool) {
	value, ok := metadata[key].(string)
	if !ok {
		return "", false
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return "", false
	}
	return value, true
}

// confirmsOpenCodeBackgroundLaunch correlates the input's background flag with
// a result that identifies the same call's running shell. Either observation
// without the other is not a confirmed launch.
func confirmsOpenCodeBackgroundLaunch(
	payload *streams.NormalizedPayload,
	result any,
) (openCodeBackgroundLaunch, bool) {
	if payload == nil || payload.ShellExec() == nil || !payload.ShellExec().Background {
		return openCodeBackgroundLaunch{}, false
	}
	return recognizeOpenCodeBackgroundLaunch(result)
}

// openCodeBackgroundLaunchRecognizer registers OpenCode's launch identity with
// the shared backgroundlaunch seam. Input alone never confirms a launch: the
// first phase reports false and the result phase supplies the shell identity.
type openCodeBackgroundLaunchRecognizer struct{}

func (openCodeBackgroundLaunchRecognizer) AgentID() string { return openCodeAgentID }

func (openCodeBackgroundLaunchRecognizer) RecognizesDetachedLaunch(*streams.NormalizedPayload) bool {
	return false
}

func (openCodeBackgroundLaunchRecognizer) RecognizesConfirmedLaunch(
	obs backgroundlaunch.ResultObservation,
) (string, bool) {
	launch, ok := confirmsOpenCodeBackgroundLaunch(obs.Payload, obs.Result)
	if !ok {
		return "", false
	}
	return launch.ShellID, true
}

// confirmBackgroundLaunch stamps confirmed detached-work attestation from a
// tool result. OpenCode's launch identity is registered through
// backgroundlaunch, so this method has no provider branch of its own.
func (n *Normalizer) confirmBackgroundLaunch(payload *streams.NormalizedPayload, result any) {
	if n == nil || !n.backgroundWorkEnabled {
		return
	}
	if payload == nil || payload.Kind() != streams.ToolKindShellExec {
		return
	}
	workID, ok := backgroundlaunch.ConfirmDetachedLaunch(n.agentID, payload, result)
	if !ok {
		return
	}
	if payload.BackgroundWork() != nil && !payload.BackgroundWork().Unverified {
		return
	}
	payload.SetUnverifiedBackgroundWorkIdentity(
		streams.BackgroundWorkKindShell,
		workID,
		true,
		false,
	)
	// OpenCode's shell identity is both the workload and its only execution
	// identity; unlike Codex, there is no separate process key. The declared
	// capability is observation-only because this version cannot stream the
	// job's output or control the process.
	payload.SetBackgroundWorkRunID(workID)
	payload.SetBackgroundWorkCapabilities(openCodeBackgroundCapabilities())
}

// openCodeBackgroundCapabilities is the truthful declaration for a legacy
// OpenCode launch: inspectable identity, no output stream, no controls.
func openCodeBackgroundCapabilities() streams.WorkloadCapabilities {
	return streams.InspectionOnlyWorkloadCapabilities()
}

// settleUnverifiedOpenCodeBackgroundWork closes the honest inspection window
// for launches whose provider ends event delivery with the foreground prompt.
// Such a job is retained as inspectable history, becomes explicitly unknown,
// and stops fencing later prompts: no terminal frame can arrive on this
// connection. Negotiated continuity (a verified workload) is left untouched.
func (a *Adapter) settleUnverifiedOpenCodeBackgroundWork(sessionID string) {
	if a.agentID != openCodeAgentID || !a.backgroundWorkEnabled {
		return
	}

	a.mu.Lock()
	unverified := make(map[string]*streams.NormalizedPayload)
	for toolCallID, payload := range a.activeToolCalls {
		if payload == nil || payload.Kind() != streams.ToolKindShellExec {
			continue
		}
		work := payload.BackgroundWork()
		if work == nil || work.Kind != streams.BackgroundWorkKindShell ||
			!work.Detached || work.Ended || !work.Unverified {
			continue
		}
		unverified[toolCallID] = payload
		delete(a.activeToolCalls, toolCallID)
		a.forgetPromptHandoffToolLocked(toolCallID)
	}
	a.mu.Unlock()

	for toolCallID, payload := range unverified {
		a.sendUpdate(AgentEvent{
			Type:           streams.EventTypeBackgroundWorkUpdated,
			SessionID:      sessionID,
			BackgroundWork: unverifiedOpenCodeWorkloadObservation(payload, toolCallID),
		})
	}
}

// unverifiedOpenCodeWorkloadObservation projects a retained launch as unknown
// activity. It carries the original command-derived title and identity only:
// no exit status, output, or capability is fabricated for it.
func unverifiedOpenCodeWorkloadObservation(
	payload *streams.NormalizedPayload,
	toolCallID string,
) *streams.WorkloadRunObservation {
	shell := payload.ShellExec()
	work := payload.BackgroundWork()
	if shell == nil || work == nil {
		return nil
	}
	title := shell.Description
	if title == "" {
		title = shell.Command
	}
	return &streams.WorkloadRunObservation{
		WorkID: work.WorkID,
		RunID:  work.RunID,
		Kind:   streams.WorkloadKindShell,
		Title:  title,
		State:  streams.RunStateUnknown,
		// The settlement update carries the original launch call so the durable
		// row keeps its conversation association. Origin turn is preserved by
		// the recorder because this update has none.
		SourceCallID: toolCallID,
		Capabilities: openCodeBackgroundCapabilities(),
	}
}
