package automation

import (
	"context"
	"encoding/json"
)

func (s *Service) CountMCPDefinitionSelections(ctx context.Context, workspaceID, definitionID string) (int, error) {
	counts, err := s.MCPDefinitionSelectionCounts(ctx, workspaceID)
	return counts[definitionID], err
}

// MCPDefinitionSelectionCounts reads reference usage without revalidating grants.
func (s *Service) MCPDefinitionSelectionCounts(ctx context.Context, workspaceID string) (map[string]int, error) {
	var rows []string
	if err := s.store.ro.SelectContext(ctx, &rows, s.store.ro.Rebind(`SELECT mcp_selection FROM automations WHERE workspace_id = ?`), workspaceID); err != nil {
		return nil, err
	}
	counts := make(map[string]int)
	for _, raw := range rows {
		var selection *MCPSelection
		if err := json.Unmarshal([]byte(raw), &selection); err != nil {
			return nil, err
		}
		if selection == nil {
			continue
		}
		seen := make(map[string]bool)
		for _, id := range selection.DefinitionIDs {
			if !seen[id] {
				counts[id]++
				seen[id] = true
			}
		}
	}
	return counts, nil
}
