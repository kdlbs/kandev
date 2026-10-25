package plugins

import (
	"context"
	"fmt"
	"time"
)

// SetCapabilityApprovalWorkspaceAuthorizer wires the workspace manage check
// used by the human approval HTTP surface. Production supplies the task
// service's canonical workspace authorization method during route setup.
func (s *Service) SetCapabilityApprovalWorkspaceAuthorizer(authorizer func(context.Context, string) error) {
	s.capabilityApprovalWorkspaceAuthorizer = authorizer
}

// SetHumanInteractionResponseAuthorizer wires the native session-control
// scope used to mint a one-use receipt for a human response.
func (s *Service) SetHumanInteractionResponseAuthorizer(authorizer func(context.Context, string) error) {
	s.humanInteractionResponseAuthorizer = authorizer
}

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

// ListCapabilityApprovalEvents returns the immutable audit events for one
// installation/workspace pair in append order.
func (s *Service) ListCapabilityApprovalEvents(installationID, workspaceID string) ([]CapabilityApprovalEventDTO, error) {
	ledger := s.approvalLedger()
	if ledger == nil {
		return nil, ErrApprovalLedgerUnavailable
	}
	events, err := ledger.eventsByWorkspace(installationID, workspaceID)
	if err != nil {
		return nil, err
	}
	out := make([]CapabilityApprovalEventDTO, 0, len(events))
	for _, event := range events {
		out = append(out, approvalEventDTOFromCurrent(event))
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
	s.approvalEffectMu.Lock()
	ledger := s.approvalLedger()
	if ledger == nil {
		s.approvalEffectMu.Unlock()
		return CapabilityApprovalDTO{}, ErrApprovalLedgerUnavailable
	}
	previous, found, err := ledger.get(installationID, workspaceID)
	if err != nil {
		s.approvalEffectMu.Unlock()
		return CapabilityApprovalDTO{}, err
	}
	if !found {
		s.approvalEffectMu.Unlock()
		return CapabilityApprovalDTO{}, ErrApprovalNotFound
	}
	row, err := ledger.revokeIfRevision(installationID, workspaceID, expectedRevision, actor, reason, auditID, time.Now().UTC(), false)
	changed := err == nil && (row.Revision != previous.Revision || row.State != previous.State)
	s.approvalEffectMu.Unlock()
	if err != nil {
		return CapabilityApprovalDTO{}, err
	}
	if changed {
		if err := s.invalidateManagedConversationPolicy(installationID, workspaceID); err != nil {
			return approvalDTOFromCurrent(row), fmt.Errorf("plugins: approval revoked but managed conversation cancellation failed: %w", err)
		}
	}
	return approvalDTOFromCurrent(row), nil
}
