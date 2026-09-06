package plugins

import (
	"fmt"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/plugins/store"
)

// approvalLedger returns the ledger wired by SetPluginsDir. Callers handle a
// nil result (e.g. a Service constructed directly in tests without a plugins
// directory).
func (s *Service) approvalLedger() *approvalLedger {
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
	return ledger.revokeIfRevision(installationID, workspaceID, current.Revision, actor, reason, auditID, time.Now().UTC(), true)
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
	if reason, ok := malformedAuthorizationRequestReason(installationID, workspaceID, capabilityID, requestDigest, methodDigest); !ok {
		decision.Reason = reason
		return decision
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
	if current.State != ApprovalStateActive || current.TombstonedAt != nil {
		decision.Reason = ApprovalDenyRevokedApproval
		return decision
	}
	if current.Revision != requestedRevision {
		decision.Reason = ApprovalDenyStaleRevision
		return decision
	}
	if reason, ok := s.manifestIntersectionDenyReason(installationID, capabilityID, current); !ok {
		decision.Reason = reason
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

// malformedAuthorizationRequestReason validates the structural shape of an
// authorization request before any capability/approval semantics are
// considered. ok is false when the request must be denied; reason is only
// meaningful when ok is false.
func malformedAuthorizationRequestReason(installationID, workspaceID, capabilityID, requestDigest, methodDigest string) (reason ApprovalDenyReason, ok bool) {
	if strings.TrimSpace(installationID) == "" || strings.TrimSpace(workspaceID) == "" ||
		strings.TrimSpace(requestDigest) == "" || strings.TrimSpace(methodDigest) == "" {
		return ApprovalDenyMalformedRequest, false
	}
	if isUnsupportedCapabilityID(capabilityID) {
		return ApprovalDenyUnsupportedCapability, false
	}
	return "", true
}

// manifestIntersectionDenyReason enforces that the current installed
// manifest still declares capabilityID under the same digest the approval
// was granted against. ok is false when the request must be denied; reason
// is only meaningful when ok is false. A nil registry (e.g. a Service
// constructed directly in tests) skips this check.
func (s *Service) manifestIntersectionDenyReason(installationID, capabilityID string, current CapabilityApproval) (reason ApprovalDenyReason, ok bool) {
	if s.registry == nil {
		return "", true
	}
	installed := s.installedRecordByInstallationID(installationID)
	if installed == nil {
		return ApprovalDenyForeignInstallation, false
	}
	if current.ManifestDigest != ManifestCapabilityDigest(installed.Manifest) {
		return ApprovalDenyUnavailableCapability, false
	}
	if !manifestDeclaresCapability(installed, capabilityID) {
		return ApprovalDenyUndeclaredCapability, false
	}
	return "", true
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

// isUnsupportedCapabilityID reports whether capabilityID cannot be an exact
// admitted capability class: empty, containing leading/trailing whitespace,
// or a wildcard/broad alias. This mirrors the canonicalization rejected at
// grant time (CanonicalCapabilityList) so an authorization request cannot
// bypass the same rule by presenting a broad identity directly.
func isUnsupportedCapabilityID(capabilityID string) bool {
	if capabilityID == "" || strings.TrimSpace(capabilityID) != capabilityID {
		return true
	}
	return strings.ContainsAny(capabilityID, "*?")
}

func isHumanReservedCapability(capabilityID string) bool {
	switch capabilityID {
	case "merge", "deploy", "release", "rewrite_history", "cross_workspace", "secret_scope_expand":
		return true
	default:
		return false
	}
}
