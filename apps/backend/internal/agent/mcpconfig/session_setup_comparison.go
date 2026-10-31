package mcpconfig

import "context"

func (s *SelectionService) SetSessionResolutionContext(provider func(context.Context, string, string) (ResolutionContext, error)) {
	s.resolutionContext = provider
}

func (s *SelectionService) sessionSetupComparison(ctx context.Context, workspaceID, sessionID string, state SessionMCPSelectionState) *MCPSetupComparison {
	unavailable := CompareMCPSetups(state, nil)
	if s.resolutionContext == nil {
		unavailable.Reason = "context_unavailable"
		return unavailable
	}
	rc, err := s.resolutionContext(ctx, workspaceID, sessionID)
	if err != nil || rc.WorkspaceID != workspaceID || rc.SessionID != sessionID {
		unavailable.Reason = "context_unavailable"
		return unavailable
	}
	reader, ok := s.repo.(ResolutionInputsRepository)
	if !ok {
		unavailable.Reason = comparisonReasonDesiredUnavailable
		return unavailable
	}
	inputs, err := reader.ReadMCPResolutionInputs(ctx, rc)
	if err != nil {
		unavailable.Reason = comparisonReasonDesiredUnavailable
		return unavailable
	}
	latest, err := s.stateRepo.GetMCPSelectionState(ctx, sessionID)
	if err != nil || inputs.DesiredRevision != state.DesiredRevision || latest.DesiredRevision != state.DesiredRevision || latest.AppliedRevision != state.AppliedRevision || latest.AttachmentAttemptID != state.AttachmentAttemptID {
		unavailable.Reason = "snapshot_changed"
		return unavailable
	}
	comparison := CompareMCPSetups(state, setupFromInputs(inputs))
	comparison.SelectionIDs = inputs.SessionSelectionIDs
	comparison.ExcludedDefinitionIDs = inputs.ExcludedDefinitionIDs
	return boundedMCPComparison(comparison)
}
