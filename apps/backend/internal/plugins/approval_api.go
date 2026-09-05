package plugins

import "time"

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

// GrantCapabilityApproval records a workspace-scoped approval. revision is
// the exact next revision, and auditID is the stable idempotency identity.
func (s *Service) GrantCapabilityApproval(installationID, workspaceID string, revision uint64, manifestDigest string, capabilityIDs []string, actor, reason, auditID string) (CapabilityApprovalDTO, error) {
	row, err := s.approvalGrant(installationID, workspaceID, revision, manifestDigest, capabilityIDs, actor, reason, auditID)
	if err != nil {
		return CapabilityApprovalDTO{}, err
	}
	return approvalDTOFromCurrent(row), nil
}

// RevokeCapabilityApproval revokes only the supplied current revision. A
// stale caller receives ErrApprovalRevisionConflict and must read back state.
func (s *Service) RevokeCapabilityApproval(installationID, workspaceID string, expectedRevision uint64, actor, reason, auditID string) (CapabilityApprovalDTO, error) {
	ledger := s.approvalLedger()
	if ledger == nil {
		return CapabilityApprovalDTO{}, ErrApprovalRevisionConflict
	}
	row, err := ledger.revokeIfRevision(installationID, workspaceID, expectedRevision, actor, reason, auditID, time.Now().UTC(), false)
	if err != nil {
		return CapabilityApprovalDTO{}, err
	}
	return approvalDTOFromCurrent(row), nil
}
