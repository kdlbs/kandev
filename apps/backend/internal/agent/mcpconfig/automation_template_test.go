package mcpconfig

import (
	"encoding/json"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestAutomationMCPTemplateDecisionsAreDiagnosticOnly(t *testing.T) {
	result := &EffectiveMCPResolution{Servers: []ResolvedServer{{DefinitionID: "available", Name: "reports"}}}
	raw := []MCPResolutionDecision{{DefinitionID: "missing", ReasonCode: "definition_missing", Summary: "untrusted text"}, {DefinitionID: "available", ReasonCode: "definition_missing"}, {DefinitionID: "grant", ReasonCode: "connected"}}
	encoded, err := json.Marshal(raw)
	require.NoError(t, err)
	var stored any
	require.NoError(t, json.Unmarshal(encoded, &stored))
	AddTemplateUnavailableDecisions(result, map[string]interface{}{TemplateUnavailableMetadataKey: stored})
	require.Len(t, result.Servers, 1)
	require.Len(t, result.Decisions, 1)
	require.Equal(t, "missing", result.Decisions[0].DefinitionID)
	require.Contains(t, result.Decisions[0].Summary, "task creation")
	require.NotContains(t, result.Decisions[0].Summary, "untrusted")
}
