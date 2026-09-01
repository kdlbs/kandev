package plugins

import (
	"fmt"
	"time"

	"github.com/kandev/kandev/internal/plugins/store"
)

func (s *Service) approvalLedger() *approvalLedger {
	if s.approvals != nil {
		return s.approvals
	}
	if s.pluginsDir == "" {
		return nil
	}
	s.approvals = newApprovalLedger(s.pluginsDir)
	return s.approvals
}

func (s *Service) approvalGrant(installationID, workspaceID string, revision uint64, manifestDigest string, capabilityIDs []string, actor, reason, auditID string) (CapabilityApproval, error) {
	ledger := s.approvalLedger()
	if ledger == nil {
		return CapabilityApproval{}, fmt.Errorf("plugins: approval ledger not configured")
	}
	canonical, err := CanonicalCapabilityList(capabilityIDs)
	if err != nil {
		return CapabilityApproval{}, err
	}
	return ledger.grant(installationID, workspaceID, revision, manifestDigest, canonical, actor, reason, auditID, time.Now().UTC())
}

func (s *Service) approvalRevoke(installationID, workspaceID, actor, reason, auditID string) (CapabilityApproval, error) {
	ledger := s.approvalLedger()
	if ledger == nil {
		return CapabilityApproval{}, fmt.Errorf("plugins: approval ledger not configured")
	}
	current, ok, err := ledger.get(installationID, workspaceID)
	if err != nil || !ok {
		if err != nil {
			return CapabilityApproval{}, err
		}
		return CapabilityApproval{}, fmt.Errorf("plugins: approval not found")
	}
	return ledger.revokeIfRevision(installationID, workspaceID, current.Revision, actor, reason, auditID, time.Now().UTC(), false)
}

func (s *Service) approvalTombstoneInstallation(installationID string) error {
	ledger := s.approvalLedger()
	if ledger == nil {
		return nil
	}
	return ledger.tombstoneInstallation(installationID, time.Now().UTC())
}

func (s *Service) approvalCurrent(installationID, workspaceID string) (CapabilityApproval, bool, error) {
	ledger := s.approvalLedger()
	if ledger == nil {
		return CapabilityApproval{}, false, fmt.Errorf("plugins: approval ledger not configured")
	}
	return ledger.get(installationID, workspaceID)
}

func (s *Service) approvalListByInstallation(installationID string) ([]CapabilityApproval, error) {
	ledger := s.approvalLedger()
	if ledger == nil {
		return nil, fmt.Errorf("plugins: approval ledger not configured")
	}
	return ledger.listByInstallation(installationID)
}

func (s *Service) authorizePluginCapability(installationID, workspaceID, capabilityID string, requestedRevision uint64, requestDigest, methodDigest string) ApprovalDecision {
	decision := ApprovalDecision{
		Receipt: ApprovalReceipt{
			InstallationID: installationID,
			WorkspaceID:    workspaceID,
			Revision:       requestedRevision,
			CapabilityID:   capabilityID,
			RequestDigest:  requestDigest,
			MethodDigest:   methodDigest,
			AuditID:        CanonicalApprovalDigest(installationID, workspaceID, capabilityID, requestDigest, methodDigest, fmt.Sprint(requestedRevision)),
			Result:         "denied",
			ObservedAt:     time.Now().UTC(),
		},
	}
	if isHumanReservedCapability(capabilityID) {
		decision.Reason = ApprovalDenyHumanReserved
		return decision
	}
	current, ok, err := s.approvalCurrent(installationID, workspaceID)
	if err != nil {
		decision.Reason = ApprovalDenyUnavailableCapability
		return decision
	}
	if !ok {
		decision.Reason = ApprovalDenyMissingApproval
		return decision
	}
	if current.InstallationID != installationID {
		decision.Reason = ApprovalDenyForeignInstallation
		return decision
	}
	if s.registry != nil {
		installed := s.installedRecordByInstallationID(installationID)
		if installed == nil {
			decision.Reason = ApprovalDenyForeignInstallation
			return decision
		}
		if current.ManifestDigest != ManifestCapabilityDigest(installed.Manifest) {
			decision.Reason = ApprovalDenyUnavailableCapability
			return decision
		}
		if !manifestDeclaresCapability(installed, capabilityID) {
			decision.Reason = ApprovalDenyUndeclaredCapability
			return decision
		}
	}
	if current.State != ApprovalStateActive {
		decision.Reason = ApprovalDenyRevokedApproval
		return decision
	}
	if current.Revision != requestedRevision {
		decision.Reason = ApprovalDenyStaleRevision
		return decision
	}
	for _, allowed := range current.CapabilityIDs {
		if allowed == capabilityID {
			decision.Allowed = true
			decision.Reason = ""
			decision.Receipt.Result = "allowed"
			decision.AuditID = decision.Receipt.AuditID
			return decision
		}
	}
	decision.Reason = ApprovalDenyUndeclaredCapability
	return decision
}

func (s *Service) installedRecordByInstallationID(installationID string) *store.Record {
	for _, record := range s.registry.List() {
		if record.InstallationID == installationID {
			return record
		}
	}
	return nil
}

func manifestDeclaresCapability(record *store.Record, capabilityID string) bool {
	for _, resource := range record.Capabilities.APIRead {
		if capabilityID == "api_read:"+resource {
			return true
		}
	}
	for _, resource := range record.Capabilities.APIWrite {
		if capabilityID == "api_write:"+resource {
			return true
		}
	}
	return false
}

func isHumanReservedCapability(capabilityID string) bool {
	switch capabilityID {
	case "merge", "deploy", "release", "rewrite_history", "cross_workspace", "secret_scope_expand":
		return true
	default:
		return false
	}
}
