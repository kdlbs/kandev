package mcpconfig

import "encoding/json"

const (
	reasonDefinitionMissing  = "definition_missing"
	reasonDefinitionDisabled = "definition_disabled"
)

const TemplateUnavailableMetadataKey = "mcp_template_unavailable"

// AddTemplateUnavailableDecisions carries creation-time diagnostics without granting servers.
func AddTemplateUnavailableDecisions(result *EffectiveMCPResolution, metadata map[string]interface{}) {
	if result == nil || metadata[TemplateUnavailableMetadataKey] == nil {
		return
	}
	encoded, err := json.Marshal(metadata[TemplateUnavailableMetadataKey])
	if err != nil {
		return
	}
	var decisions []MCPResolutionDecision
	if json.Unmarshal(encoded, &decisions) != nil {
		return
	}
	seen := make(map[string]bool)
	for _, server := range result.Servers {
		seen[server.DefinitionID] = true
	}
	for _, decision := range result.Decisions {
		seen[decision.DefinitionID] = true
	}
	for _, decision := range decisions {
		if decision.DefinitionID == "" || seen[decision.DefinitionID] {
			continue
		}
		switch decision.ReasonCode {
		case reasonDefinitionMissing, reasonDefinitionDisabled:
		default:
			continue
		}
		decision.Summary = "MCP definition was unavailable at task creation"
		decision.RuntimeName = ""
		decision.DefinitionRevision = 0
		result.Decisions = append(result.Decisions, decision)
		seen[decision.DefinitionID] = true
	}
}
