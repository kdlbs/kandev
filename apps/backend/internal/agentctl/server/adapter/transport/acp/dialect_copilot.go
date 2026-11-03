package acp

import (
	"regexp"
	"strings"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

const copilotAgentID = "copilot-acp"

var copilotBackgroundAgentID = regexp.MustCompile(
	`(?i)agent started in background with agent_id:\s*([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})`,
)

func newCopilotACPDialect() acpDialect {
	return acpDialect{
		subagentFrame:   parseCopilotSubagentFrame,
		subagentResult:  copilotSubagentResult,
		subagentControl: copilotSubagentControl,
	}
}

func parseCopilotSubagentFrame(_ map[string]any, _ string, rawInput any) (subagentFrame, bool) {
	input, ok := rawInput.(map[string]any)
	if !ok {
		return subagentFrame{}, false
	}
	agentType, _ := input["agent_type"].(string)
	prompt, _ := input["prompt"].(string)
	if strings.TrimSpace(agentType) == "" || strings.TrimSpace(prompt) == "" {
		return subagentFrame{}, false
	}
	name, _ := input["name"].(string)
	description, _ := input["description"].(string)
	mode, _ := input["mode"].(string)
	result := SubagentTaskResult{IsAsync: mode == "background"}
	if result.IsAsync {
		result.Status = subagentAsyncLaunchedStatus
	}
	return subagentFrame{
		provider:     "copilot",
		name:         strings.TrimSpace(name),
		description:  strings.TrimSpace(description),
		prompt:       prompt,
		subagentType: strings.TrimSpace(agentType),
		result:       result,
	}, true
}

func copilotSubagentResult(rawOutput any) (SubagentTaskResult, bool) {
	output, ok := rawOutput.(map[string]any)
	if !ok {
		return SubagentTaskResult{}, false
	}
	content, ok := output["content"].(string)
	if !ok || strings.TrimSpace(content) == "" {
		return SubagentTaskResult{}, false
	}
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(content)), "agent started in background") {
		result := SubagentTaskResult{IsAsync: true, Status: subagentAsyncLaunchedStatus}
		if match := copilotBackgroundAgentID.FindStringSubmatch(content); len(match) == 2 {
			result.AgentID = match[1]
		}
		return result, true
	}
	return SubagentTaskResult{Status: toolStatusComplete, ResultText: content}, true
}

func copilotSubagentControl(toolName string, _ map[string]any, _ any) streams.SubagentControl {
	switch toolName {
	case "read_agent":
		return streams.SubagentControlRead
	case "write_agent":
		return streams.SubagentControlMessage
	case "list_agents":
		return streams.SubagentControlList
	default:
		return ""
	}
}
