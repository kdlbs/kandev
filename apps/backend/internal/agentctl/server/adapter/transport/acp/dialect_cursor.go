package acp

import (
	"encoding/json"
	"strings"

	acp "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

const (
	cursorAgentID                      = "cursor-acp"
	cursorPromptStreamClosedRPCCode    = -32603
	cursorPromptStreamClosedRPCMessage = "Internal error"
	cursorPromptStreamClosedDetails    = "WritableIterable is closed"
	cursorRetriableStreamResetPrefix   = "Error: RetriableError:"
	cursorRetriableStreamResetMessage  = "Error: RetriableError: HTTP/2 stream closed with error code CANCEL (0x8)"
	cursorRetriableStreamResetMaxTail  = 256
)

func newCursorACPDialect() acpDialect {
	return acpDialect{
		mcpToolCall:         parseCursorMCPToolCall,
		cursorShellWatch:    parseCursorShellWatchFrame,
		continuationSupport: streams.ContinuationAgentManagedToolsV3,
	}
}

func cursorPromptStreamClosed(err error) bool {
	requestErr, ok := remotePromptRequestError(err)
	if !ok || requestErr.Code != cursorPromptStreamClosedRPCCode ||
		requestErr.Message != cursorPromptStreamClosedRPCMessage {
		return false
	}
	return cursorPromptStreamClosedData(requestErr.Data)
}

func cursorPromptStreamClosedData(data any) bool {
	object, ok := data.(map[string]any)
	if !ok || len(object) != 1 {
		return false
	}
	details, ok := object["details"].(string)
	return ok && details == cursorPromptStreamClosedDetails
}

type cursorShellWatchFrameKind string

const (
	cursorShellWatchRegistration cursorShellWatchFrameKind = "registration"
	cursorShellWatchRunning      cursorShellWatchFrameKind = "running"
	cursorShellWatchCompleted    cursorShellWatchFrameKind = "completed"
	cursorShellWatchFailed       cursorShellWatchFrameKind = "failed"
)

type cursorShellWatchFrame struct {
	toolCallID string
	title      string
	kind       cursorShellWatchFrameKind
}

func parseCursorShellWatchFrame(update acp.SessionUpdate) (cursorShellWatchFrame, bool) {
	switch {
	case update.ToolCall != nil:
		return parseCursorShellWatchRegistration(update.ToolCall)
	case update.ToolCallUpdate != nil:
		return parseCursorShellWatchUpdate(update.ToolCallUpdate)
	default:
		return cursorShellWatchFrame{}, false
	}
}

func parseCursorShellWatchRegistration(call *acp.SessionUpdateToolCall) (cursorShellWatchFrame, bool) {
	if call.SessionUpdate != acpSessionUpdateToolCall || string(call.Kind) != acpToolCallKindOther ||
		call.Status != acp.ToolCallStatusPending || !validCursorShellWatchCallID(string(call.ToolCallId)) {
		return cursorShellWatchFrame{}, false
	}
	title, ok := cursorShellWatchTitle(call.RawInput)
	if !ok {
		return cursorShellWatchFrame{}, false
	}
	return cursorShellWatchFrame{
		toolCallID: string(call.ToolCallId),
		title:      title,
		kind:       cursorShellWatchRegistration,
	}, true
}

func parseCursorShellWatchUpdate(update *acp.SessionToolCallUpdate) (cursorShellWatchFrame, bool) {
	if update.SessionUpdate != acpSessionUpdateToolCallUpdate || update.Status == nil ||
		!validCursorShellWatchCallID(string(update.ToolCallId)) {
		return cursorShellWatchFrame{}, false
	}
	var kind cursorShellWatchFrameKind
	switch *update.Status {
	case acp.ToolCallStatusInProgress:
		kind = cursorShellWatchRunning
	case acp.ToolCallStatusCompleted:
		kind = cursorShellWatchCompleted
	case acp.ToolCallStatusFailed:
		kind = cursorShellWatchFailed
	default:
		return cursorShellWatchFrame{}, false
	}
	return cursorShellWatchFrame{toolCallID: string(update.ToolCallId), kind: kind}, true
}

func validCursorShellWatchCallID(callID string) bool {
	return strings.HasPrefix(callID, "watch-") && strings.TrimSpace(strings.TrimPrefix(callID, "watch-")) != ""
}

func cursorShellWatchTitle(rawInput any) (string, bool) {
	input, ok := rawInput.(map[string]any)
	if !ok {
		return "", false
	}
	shells, ok := input["shells"].([]any)
	if !ok || len(shells) == 0 {
		return "", false
	}
	parts := make([]string, 0, len(shells))
	for _, rawShell := range shells {
		shell, ok := rawShell.(map[string]any)
		if !ok {
			return "", false
		}
		shellID, _ := shell["shellId"].(string)
		shellID = strings.TrimSpace(shellID)
		if shellID == "" {
			return "", false
		}
		reason, _ := shell["reason"].(string)
		reason = strings.TrimSpace(reason)
		reference, _ := shell["title"].(string)
		reference = strings.TrimSpace(reference)
		if reference == "" {
			reference = shellID
		}
		if reason == "" || reason == reference {
			parts = append(parts, reference)
		} else {
			parts = append(parts, reason+" — "+reference)
		}
	}
	return strings.Join(parts, "; "), true
}

func parseCursorMCPToolCall(_ map[string]any, rawInput any) (mcpToolCallFrame, bool) {
	input, ok := rawInput.(map[string]any)
	if !ok {
		return mcpToolCallFrame{}, false
	}
	provider, _ := input["providerIdentifier"].(string)
	tool, _ := input["toolName"].(string)
	arguments, ok := input["args"].(map[string]any)
	provider = strings.TrimSpace(provider)
	tool = strings.TrimSpace(tool)
	if provider == "" || tool == "" || !ok {
		return mcpToolCallFrame{}, false
	}
	return mcpToolCallFrame{name: provider + "/" + tool, arguments: arguments}, true
}

// isCursorRetriableStreamReset recognizes Cursor's bounded RetriableError
// control chunk. The prefix check keeps the common per-token path
// allocation-free; requiring a non-empty bounded suffix prevents a partial
// marker from becoming transport evidence.
func isCursorRetriableStreamReset(text string) bool {
	trimmed := strings.TrimSpace(text)
	if len(trimmed) < len(cursorRetriableStreamResetPrefix) ||
		!strings.EqualFold(trimmed[:len(cursorRetriableStreamResetPrefix)], cursorRetriableStreamResetPrefix) {
		return false
	}
	suffix := strings.TrimSpace(trimmed[len(cursorRetriableStreamResetPrefix):])
	if suffix == "" || len(suffix) > cursorRetriableStreamResetMaxTail || routingerr.IsCursorRetriableCancellation(suffix) {
		return false
	}
	return true
}

type cursorTaskMeta struct {
	ToolCallID  string
	AgentID     string
	Description string
	Model       string
	Prompt      string
}

func parseCursorTaskParams(params json.RawMessage) cursorTaskMeta {
	if len(params) == 0 {
		return cursorTaskMeta{}
	}
	var raw map[string]any
	if err := json.Unmarshal(params, &raw); err != nil {
		return cursorTaskMeta{}
	}
	return cursorTaskMeta{
		ToolCallID:  stringFromAnyMap(raw, "toolCallId"),
		AgentID:     stringFromAnyMap(raw, "agentId"),
		Description: stringFromAnyMap(raw, "description"),
		Model:       stringFromAnyMap(raw, "model"),
		Prompt:      stringFromAnyMap(raw, "prompt"),
	}
}

func mergeCursorTaskMeta(base, update cursorTaskMeta) cursorTaskMeta {
	if base.ToolCallID == "" {
		base.ToolCallID = update.ToolCallID
	}
	if base.AgentID == "" {
		base.AgentID = update.AgentID
	}
	if base.Description == "" {
		base.Description = update.Description
	}
	if base.Model == "" {
		base.Model = update.Model
	}
	if base.Prompt == "" {
		base.Prompt = update.Prompt
	}
	return base
}

func applyCursorTaskMeta(payload *streams.SubagentTaskPayload, meta cursorTaskMeta) {
	if payload == nil {
		return
	}
	fillIfEmpty(&payload.Description, meta.Description)
	fillIfEmpty(&payload.Prompt, meta.Prompt)
	fillIfEmpty(&payload.Model, meta.Model)
	fillIfEmpty(&payload.AgentID, meta.AgentID)
}

func stringFromAnyMap(raw map[string]any, key string) string {
	if raw == nil {
		return ""
	}
	value, _ := raw[key].(string)
	return value
}
