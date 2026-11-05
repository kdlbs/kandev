package mcpconfig

import "context"

// ProfileImportStatus exposes readiness only, never legacy failure text or configuration.
func (s *SelectionService) ProfileImportStatus(ctx context.Context, workspaceID, profileID string) string {
	const unknown = "unknown"
	if s.validateContext(ctx, SelectionScopeProfile, workspaceID, profileID) != nil {
		return unknown
	}
	reader, ok := s.repo.(LegacyImportStateReader)
	if !ok {
		return unknown
	}
	state, err := reader.GetMCPImportState(ctx, workspaceID, profileID)
	if err != nil {
		return unknown
	}
	switch state.Status {
	case LegacyImportStatusComplete, LegacyImportStatusPending:
		return state.Status
	default:
		return unknown
	}
}
