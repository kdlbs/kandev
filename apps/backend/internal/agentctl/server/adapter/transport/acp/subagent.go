package acp

import (
	"strings"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

// Subagent (Task) tool detection constants. These are the wire-level tool
// identifiers three real agents use to spawn a child agent ("subagent"):
//   - Claude-acp tags it via `_meta.claudeCode.toolName == "Agent"`.
//   - OpenCode reports tool title == "task" (case-insensitive).
//   - Cursor reports `rawInput._toolName == "task"` (title "Task: Subagent task").
const (
	subagentClaudeToolName = "Agent"
	subagentTaskName       = "task"
)

// rawInput field keys carrying subagent invocation details. These arrive on a
// later tool_call_update for Claude/OpenCode; the initial tool_call is empty.
const (
	subagentKeyDescription = "description"
	subagentKeyPrompt      = "prompt"
	subagentKeySubagentTyp = "subagent_type"
	subagentKeyToolName    = "_toolName"
)

// SubagentTaskResult holds the result metadata extracted from a completion
// tool_call_update. Each agent provides a different subset; absent fields stay
// zero-valued.
type SubagentTaskResult struct {
	Status         string
	AgentID        string
	SubagentType   string
	Model          string
	ChildSessionID string
	DurationMs     int64
	TotalTokens    int64
	ToolUseCount   int
}

// recognizeSubagent reports whether a tool call spawns a subagent (Task) and
// pulls description/prompt/subagent_type out of rawInput when present. The
// detection mirrors monitor.go: defensive over untyped maps, no logging.
//
// A tool call is a subagent if ANY of:
//   - Claude:   meta.claudeCode.toolName == "Agent"
//   - OpenCode: title == "task" (case-insensitive)
//   - Cursor:   rawInput._toolName == "task"
//
// The Claude "Monitor"/"ScheduleWakeup" tool names are deliberately NOT matched
// here so their dedicated handling stays intact.
func recognizeSubagent(meta map[string]any, title string, rawInput any) (description, prompt, subagentType string, ok bool) {
	if !isSubagentSignal(meta, title, rawInput) {
		return "", "", "", false
	}
	description, prompt, subagentType = subagentInputFields(rawInput)
	return description, prompt, subagentType, true
}

// isSubagentSignal implements the three detection rules.
func isSubagentSignal(meta map[string]any, title string, rawInput any) bool {
	if isClaudeAgentMeta(meta) {
		return true
	}
	if strings.EqualFold(strings.TrimSpace(title), subagentTaskName) {
		return true
	}
	if input, ok := rawInput.(map[string]any); ok {
		if name, _ := input[subagentKeyToolName].(string); strings.EqualFold(name, subagentTaskName) {
			return true
		}
	}
	return false
}

// isClaudeAgentMeta returns true when `_meta.claudeCode.toolName == "Agent"`.
func isClaudeAgentMeta(meta map[string]any) bool {
	if meta == nil {
		return false
	}
	cc, ok := meta["claudeCode"].(map[string]any)
	if !ok {
		return false
	}
	name, _ := cc["toolName"].(string)
	return name == subagentClaudeToolName
}

// subagentInputFields pulls description/prompt/subagent_type from a tool call's
// rawInput. Any may be empty (the initial tool_call carries none of them).
func subagentInputFields(rawInput any) (description, prompt, subagentType string) {
	input, ok := rawInput.(map[string]any)
	if !ok {
		return "", "", ""
	}
	description, _ = input[subagentKeyDescription].(string)
	prompt, _ = input[subagentKeyPrompt].(string)
	subagentType, _ = input[subagentKeySubagentTyp].(string)
	return description, prompt, subagentType
}

// extractSubagentResult reads the per-agent result shapes off a completion
// tool_call_update. Returns ok=false when neither meta nor rawOutput carries
// any recognizable result fields.
//
// Shapes:
//   - Claude:   meta.claudeCode.toolResponse = {agentId, agentType, status,
//     totalDurationMs, totalTokens, totalToolUseCount}
//   - OpenCode: rawOutput.metadata = {sessionId, parentSessionId,
//     model:{providerID, modelID}}
//   - Cursor:   rawOutput = {durationMs, isBackground}
func extractSubagentResult(meta map[string]any, rawOutput any) (res SubagentTaskResult, ok bool) {
	if claudeSubagentResponse(meta, &res) {
		ok = true
	}
	if out, isMap := rawOutput.(map[string]any); isMap {
		if openCodeSubagentMetadata(out, &res) {
			ok = true
		}
		if cursorSubagentResult(out, &res) {
			ok = true
		}
	}
	return res, ok
}

// claudeSubagentResponse reads `_meta.claudeCode.toolResponse` into res.
func claudeSubagentResponse(meta map[string]any, res *SubagentTaskResult) bool {
	if meta == nil {
		return false
	}
	cc, ok := meta["claudeCode"].(map[string]any)
	if !ok {
		return false
	}
	resp, ok := cc["toolResponse"].(map[string]any)
	if !ok {
		return false
	}
	res.AgentID, _ = resp["agentId"].(string)
	res.SubagentType, _ = resp["agentType"].(string)
	res.Status, _ = resp["status"].(string)
	res.DurationMs = asInt64(resp["totalDurationMs"])
	res.TotalTokens = asInt64(resp["totalTokens"])
	res.ToolUseCount = int(asInt64(resp["totalToolUseCount"]))
	return true
}

// openCodeSubagentMetadata reads OpenCode's `rawOutput.metadata` into res and
// composes Model as "providerID/modelID".
func openCodeSubagentMetadata(out map[string]any, res *SubagentTaskResult) bool {
	md, ok := out["metadata"].(map[string]any)
	if !ok {
		return false
	}
	found := false
	if sid, _ := md["sessionId"].(string); sid != "" {
		res.ChildSessionID = sid
		found = true
	}
	if model, ok := md["model"].(map[string]any); ok {
		provider, _ := model["providerID"].(string)
		modelID, _ := model["modelID"].(string)
		if provider != "" || modelID != "" {
			res.Model = provider + "/" + modelID
			found = true
		}
	}
	return found
}

// cursorSubagentResult reads Cursor's flat `rawOutput.durationMs` into res.
// Returns false when no Cursor-shaped field is present so it doesn't claim
// unrelated rawOutput maps.
func cursorSubagentResult(out map[string]any, res *SubagentTaskResult) bool {
	dur, present := out["durationMs"]
	if !present {
		return false
	}
	res.DurationMs = asInt64(dur)
	return true
}

// asInt64 coerces JSON-decoded numbers (float64 most commonly) to int64.
func asInt64(v any) int64 {
	switch n := v.(type) {
	case float64:
		return int64(n)
	case int64:
		return n
	case int:
		return int64(n)
	}
	return 0
}

// applySubagentResult writes the extracted result fields onto a subagent
// payload, only filling fields the result actually provides (so we never
// blank out values already learned from rawInput, e.g. SubagentType).
func applySubagentResult(p *streams.SubagentTaskPayload, res SubagentTaskResult) {
	if p == nil {
		return
	}
	if res.Status != "" {
		p.Status = res.Status
	}
	if res.AgentID != "" {
		p.AgentID = res.AgentID
	}
	if res.SubagentType != "" && p.SubagentType == "" {
		p.SubagentType = res.SubagentType
	}
	if res.Model != "" {
		p.Model = res.Model
	}
	if res.ChildSessionID != "" {
		p.ChildSessionID = res.ChildSessionID
	}
	if res.DurationMs != 0 {
		p.DurationMs = res.DurationMs
	}
	if res.TotalTokens != 0 {
		p.TotalTokens = res.TotalTokens
	}
	if res.ToolUseCount != 0 {
		p.ToolUseCount = res.ToolUseCount
	}
}
