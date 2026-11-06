package automation

import (
	"context"
	"encoding/json"
	"slices"
)

func (s *Service) CountMCPDefinitionSelections(ctx context.Context, workspaceID, definitionID string) (int, error) {
	var rows []string
	if err := s.store.ro.SelectContext(ctx, &rows, s.store.ro.Rebind(`SELECT mcp_selection FROM automations WHERE workspace_id = ?`), workspaceID); err != nil {
		return 0, err
	}
	count := 0
	for _, raw := range rows {
		var selection *MCPSelection
		if err := json.Unmarshal([]byte(raw), &selection); err != nil {
			return 0, err
		}
		if err := selection.validate(); err != nil {
			return 0, err
		}
		if selection != nil && slices.Contains(selection.DefinitionIDs, definitionID) {
			count++
		}
	}
	return count, nil
}
