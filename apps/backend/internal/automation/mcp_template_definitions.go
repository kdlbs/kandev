package automation

import (
	"context"
	"errors"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
)

// automationMCPTemplateDefinitions separates attachable references from missing or disabled
// definition evidence.
func (s *Service) automationMCPTemplateDefinitions(ctx context.Context, a *Automation) ([]string, []mcpconfig.MCPResolutionDecision, error) {
	ids := make([]string, 0, len(a.MCPSelection.DefinitionIDs))
	var decisions []mcpconfig.MCPResolutionDecision
	if len(a.MCPSelection.DefinitionIDs) > 0 && s.mcpDefinitions == nil {
		return nil, nil, errors.New("MCP catalog unavailable")
	}
	for _, id := range a.MCPSelection.DefinitionIDs {
		definition, err := s.mcpDefinitions.GetMCPServerDefinition(ctx, a.WorkspaceID, id)
		if err != nil && !errors.Is(err, mcpconfig.ErrMCPServerDefinitionNotFound) {
			return nil, nil, err
		}
		reason := ""
		if definition == nil || definition.WorkspaceID != a.WorkspaceID {
			reason = "definition_missing"
		} else if !definition.Enabled {
			reason = "definition_disabled"
		}
		if reason == "" {
			ids = append(ids, id)
			continue
		}
		decisions = append(decisions, mcpconfig.MCPResolutionDecision{DefinitionID: id, ReasonCode: reason, Origins: []mcpconfig.SelectionOrigin{{Scope: mcpconfig.SelectionScope("automation"), WorkspaceID: a.WorkspaceID, OwnerID: a.ID}}})
	}
	return ids, decisions, nil
}
