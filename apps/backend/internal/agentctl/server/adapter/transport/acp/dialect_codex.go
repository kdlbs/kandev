package acp

import (
	"path"
	"strings"
)

const (
	codexAgentID                 = "codex-acp"
	codexCollaborationSpawnAgent = "spawnAgent"
	codexSubagentStarted         = "started"
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
	return acpDialect{subagentFrame: parseCodexSubagentFrame}
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
	return activityKind == codexSubagentStarted
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
