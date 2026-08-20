package acp

import (
	"path"
	"strings"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

const (
	codexAgentID                   = "codex-acp"
	codexCollaborationSpawnAgent   = "spawnAgent"
	codexSubagentStarted           = "started"
	codexSystemErrorType           = "systemError"
	codexModelCapacityErrorMessage = "Selected model is at capacity. Please try a different model."
)

type codexSubagentSignal uint8

const (
	codexSubagentSignalNone codexSubagentSignal = iota
	codexSubagentSignalCollaboration
	codexSubagentSignalActivity
)

// newCodexACPDialect translates codex-acp's implementation metadata into the
// provider-neutral subagent payload. codex-acp reports collaboration control
// operations through the same envelope, so only spawnAgent and the matching
// "started" activity are creation signals.
func newCodexACPDialect() acpDialect {
	return acpDialect{
		subagentFrame:        parseCodexSubagentFrame,
		normalizePromptUsage: normalizeCodexPromptUsage,
		mcpToolCall:          parseCodexMCPToolCall,
		mcpToolResult:        normalizeCodexMCPToolResult,
	}
}

// normalizeCodexPromptUsage marks codex-acp's typed usage frame as
// estimated. codex-acp 1.4.0 does emit a typed usage block on the prompt
// response, but its three response-construction sites (normal end_turn,
// cancelled, terminal failure) all hardcode it to
// sessionState.lastTokenUsage — the LAST model request of the turn, not a
// per-turn total (sessionState.totalTokenUsage is tracked internally but
// never crosses the ACP boundary). A turn making N requests reports only
// request N's counts: verified against codex's own rollout log on both a
// 22-request turn (recorded 410 output tokens vs a true 8813) and a
// 4-request turn (recorded 9 vs a true 219). Estimated is the existing
// signal for "not an authoritative per-turn frame" (streams.PromptUsage's
// doc comment), so this keeps the row honest until codex-acp emits a
// genuine per-turn total upstream.
func normalizeCodexPromptUsage(
	usage *streams.PromptUsage,
	_ map[string]any,
) *streams.PromptUsage {
	if usage != nil {
		usage.Estimated = true
	}
	return usage
}

// parseCodexMCPToolCall recognizes Codex's observed MCP-over-ACP envelope.
// Codex reports these as the broad ACP "execute" kind and puts the actual MCP
// identity and arguments in rawInput. Require the explicit implementation
// marker plus the complete envelope so ordinary execute tools remain shells.
func parseCodexMCPToolCall(meta map[string]any, rawInput any) (mcpToolCallFrame, bool) {
	isMCP, _ := meta["is_mcp_tool_call"].(bool)
	if !isMCP {
		return mcpToolCallFrame{}, false
	}
	input, ok := rawInput.(map[string]any)
	if !ok {
		return mcpToolCallFrame{}, false
	}
	server, _ := input["server"].(string)
	tool, _ := input["tool"].(string)
	arguments, ok := input["arguments"].(map[string]any)
	server = strings.TrimSpace(server)
	tool = strings.TrimSpace(tool)
	if server == "" || tool == "" || !ok {
		return mcpToolCallFrame{}, false
	}
	return mcpToolCallFrame{name: server + "/" + tool, arguments: arguments}, true
}

// normalizeCodexMCPToolResult removes Codex's transport-only
// {error, result} wrapper while retaining the standard MCP CallToolResult.
func normalizeCodexMCPToolResult(rawOutput any) (any, bool) {
	output, ok := rawOutput.(map[string]any)
	if !ok {
		return rawOutput, rawOutput != nil
	}
	result, exists := output["result"]
	if !exists {
		return rawOutput, true
	}
	return result, true
}

// codexSystemErrorMeta reports the explicit thread-status marker emitted by
// codex-acp when a prompt failed at the provider boundary. The marker is
// implementation metadata, so keep its shape local to the Codex dialect and
// never expose the raw metadata to routing or the UI.
func codexSystemErrorMeta(meta map[string]any) bool {
	status, ok := nestedMap(meta, "threadStatus")
	if !ok {
		codex, codexOK := nestedMap(meta, "codex")
		if !codexOK {
			return false
		}
		status, ok = nestedMap(codex, "threadStatus")
	}
	if !ok {
		return false
	}
	typeValue, _ := status["type"].(string)
	return typeValue == codexSystemErrorType
}

func codexModelCapacityMessage(text string) bool {
	normalized := strings.ToLower(strings.Join(strings.Fields(text), " "))
	return strings.Contains(normalized, "selected model is at capacity") ||
		strings.Contains(normalized, "model is at capacity")
}

func parseCodexSubagentFrame(meta map[string]any, _ string, rawInput any) (subagentFrame, bool) {
	codex, ok := nestedMap(meta, "codex")
	if !ok {
		return subagentFrame{}, false
	}
	if collaboration, ok := nestedMap(codex, "collaboration"); ok && codexCollaborationSignal(collaboration) {
		return parseCodexCollaboration(collaboration, rawInput)
	}
	if activity, ok := nestedMap(codex, "subagent"); ok && codexActivitySignal(activity) {
		return parseCodexSubagentActivity(activity, rawInput)
	}
	return subagentFrame{}, false
}

func codexSubagentSignalFromMeta(meta map[string]any) codexSubagentSignal {
	codex, ok := nestedMap(meta, "codex")
	if !ok {
		return codexSubagentSignalNone
	}
	if collaboration, ok := nestedMap(codex, "collaboration"); ok && codexCollaborationSignal(collaboration) {
		return codexSubagentSignalCollaboration
	}
	if activity, ok := nestedMap(codex, "subagent"); ok && codexActivitySignal(activity) {
		return codexSubagentSignalActivity
	}
	return codexSubagentSignalNone
}

func codexSenderThreadID(meta map[string]any) string {
	codex, ok := nestedMap(meta, "codex")
	if !ok {
		return ""
	}
	collaboration, ok := nestedMap(codex, "collaboration")
	if !ok {
		return ""
	}
	senderThreadID, _ := collaboration["senderThreadId"].(string)
	return senderThreadID
}

func codexCollaborationSignal(collaboration map[string]any) bool {
	tool, _ := collaboration["tool"].(string)
	return tool == codexCollaborationSpawnAgent
}

func codexActivitySignal(activity map[string]any) bool {
	activityKind, _ := activity["activity"].(string)
	return activityKind == codexSubagentStarted || codexActivityTerminal(activityKind)
}

func codexActivityTerminal(activity string) bool {
	switch activity {
	case toolStatusCompleted, toolStatusComplete, toolStatusInterrupted, toolStatusCancelled,
		toolStatusErrored, toolStatusError, toolStatusShutdown, toolStatusNotFound:
		return true
	default:
		return false
	}
}

func parseCodexCollaboration(collaboration map[string]any, rawInput any) (subagentFrame, bool) {
	if !codexCollaborationSignal(collaboration) {
		return subagentFrame{}, false
	}

	input, _ := rawInput.(map[string]any)
	receivers := stringSlice(collaboration["receiverThreadIds"])
	if len(receivers) == 0 {
		receivers = stringSlice(input["receiverThreadIds"])
	}
	childID := firstString(receivers)
	prompt, _ := input["prompt"].(string)
	description, status := codexAgentState(input, childID)
	if description == "" {
		description = prompt
	}
	if status == "" {
		status, _ = input["status"].(string)
	}
	model, _ := input["model"].(string)

	return subagentFrame{
		description: description,
		prompt:      prompt,
		result: SubagentTaskResult{
			Status:         status,
			Model:          model,
			ChildSessionID: childID,
		},
	}, true
}

func parseCodexSubagentActivity(activity map[string]any, rawInput any) (subagentFrame, bool) {
	activityKind, _ := activity["activity"].(string)
	if !codexActivitySignal(activity) {
		return subagentFrame{}, false
	}
	input, _ := rawInput.(map[string]any)
	childID, _ := activity["threadId"].(string)
	if childID == "" {
		childID, _ = input["agentThreadId"].(string)
	}
	agentPath, _ := activity["path"].(string)
	if agentPath == "" {
		agentPath, _ = input["agentPath"].(string)
	}
	name := codexSubagentName(agentPath)

	return subagentFrame{
		description:  name,
		subagentType: name,
		result: SubagentTaskResult{
			Status:         activityKind,
			ChildSessionID: childID,
		},
	}, true
}

func codexAgentState(input map[string]any, childID string) (message, status string) {
	if input == nil || childID == "" {
		return "", ""
	}
	states, ok := input["agentsStates"].(map[string]any)
	if !ok {
		return "", ""
	}
	state, ok := states[childID].(map[string]any)
	if !ok {
		return "", ""
	}
	message, _ = state["message"].(string)
	status, _ = state["status"].(string)
	return message, status
}

func codexSubagentName(agentPath string) string {
	cleaned := strings.TrimSpace(agentPath)
	if cleaned == "" {
		return ""
	}
	name := path.Base(cleaned)
	if name == "." || name == "/" {
		return ""
	}
	return name
}

func nestedMap(parent map[string]any, key string) (map[string]any, bool) {
	if parent == nil {
		return nil, false
	}
	value, ok := parent[key].(map[string]any)
	return value, ok
}

func stringSlice(value any) []string {
	switch values := value.(type) {
	case []string:
		return values
	case []any:
		result := make([]string, 0, len(values))
		for _, value := range values {
			if item, ok := value.(string); ok && item != "" {
				result = append(result, item)
			}
		}
		return result
	default:
		return nil
	}
}

func firstString(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}
