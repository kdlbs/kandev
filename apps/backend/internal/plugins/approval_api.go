package plugins

// ListCapabilityApprovals returns the current approval rows for one installed
// plugin identity.
func (s *Service) ListCapabilityApprovals(installationID string) ([]CapabilityApprovalDTO, error) {
	rows, err := s.approvalListByInstallation(installationID)
	if err != nil {
		return nil, err
	}
	out := make([]CapabilityApprovalDTO, 0, len(rows))
	for _, row := range rows {
		out = append(out, approvalDTOFromCurrent(row))
	}
	return out, nil
}

// GetCapabilityApproval returns the current approval row for an
// installation/workspace pair.
func (s *Service) GetCapabilityApproval(installationID, workspaceID string) (CapabilityApprovalDTO, bool, error) {
	row, ok, err := s.approvalCurrent(installationID, workspaceID)
	if err != nil || !ok {
		return CapabilityApprovalDTO{}, ok, err
	}
	return approvalDTOFromCurrent(row), true, nil
}

// AuthorizeCapability evaluates a single exact capability request against the
// current approval row and returns a typed allow/deny result.
func (s *Service) AuthorizeCapability(
	installationID, workspaceID, capabilityID string, requestedRevision uint64, requestDigest, methodDigest string,
) ApprovalDecision {
	return s.authorizePluginCapability(installationID, workspaceID, capabilityID, requestedRevision, requestDigest, methodDigest)
}
