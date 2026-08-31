package plugins

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// ApprovalDecision is the typed allow/deny result for the generic plugin
// authorization substrate. It is intentionally small and side-effect free:
// no Host operation runs here.
type ApprovalDecision struct {
	Allowed bool
	// Reason is stable and machine-readable when Allowed is false.
	Reason ApprovalDenyReason
	// AuditID is the immutable audit identity attached to the decision.
	AuditID string
	// Receipt contains the safe metadata a future exact Host adapter can use
	// when persisting an audit/read receipt.
	Receipt ApprovalReceipt
}

// ApprovalDenyReason is the stable typed denial vocabulary used by the
// generic plugin approval layer.
type ApprovalDenyReason string

const (
	ApprovalDenyMissingApproval       ApprovalDenyReason = "missing_capability_approval"
	ApprovalDenyStaleRevision         ApprovalDenyReason = "stale_capability_revision"
	ApprovalDenyRevokedApproval       ApprovalDenyReason = "capability_revoked"
	ApprovalDenyHumanReserved         ApprovalDenyReason = "human_reserved"
	ApprovalDenyForeignWorkspace      ApprovalDenyReason = "foreign_workspace"
	ApprovalDenyForeignInstallation   ApprovalDenyReason = "foreign_installation"
	ApprovalDenyUnsupportedCapability ApprovalDenyReason = "unsupported_capability"
	ApprovalDenyUndeclaredCapability  ApprovalDenyReason = "undeclared_capability"
	ApprovalDenyUnavailableCapability ApprovalDenyReason = "unavailable_capability"
	ApprovalDenyMalformedRequest      ApprovalDenyReason = "malformed_request"
)

// ApprovalReceipt is the bounded, deterministic metadata future Host adapters
// can attach to read/write receipts without carrying authority themselves.
type ApprovalReceipt struct {
	InstallationID string    `json:"installation_id"`
	WorkspaceID    string    `json:"workspace_id"`
	Revision       uint64    `json:"revision"`
	CapabilityID   string    `json:"capability_id"`
	RequestDigest  string    `json:"request_digest"`
	MethodDigest   string    `json:"method_digest"`
	AuditID        string    `json:"audit_id"`
	Result         string    `json:"result"`
	ObservedAt     time.Time `json:"observed_at"`
}

// CanonicalApprovalDigest returns a stable digest for receipt identity. The
// inputs are normalized to keep the output deterministic and to avoid
// accidentally treating whitespace as authority.
func CanonicalApprovalDigest(parts ...string) string {
	normalized := make([]string, len(parts))
	for i, part := range parts {
		normalized[i] = strings.TrimSpace(part)
	}
	sum := sha256.Sum256([]byte(strings.Join(normalized, "\x00")))
	return hex.EncodeToString(sum[:])
}

// CanonicalCapabilityList returns a copy of caps sorted and deduplicated.
func CanonicalCapabilityList(caps []string) ([]string, error) {
	out := make([]string, 0, len(caps))
	seen := make(map[string]struct{}, len(caps))
	for _, raw := range caps {
		capability := strings.TrimSpace(raw)
		if capability == "" {
			return nil, errors.New("plugins: empty capability")
		}
		if strings.ContainsAny(capability, "*?") {
			return nil, fmt.Errorf("plugins: wildcard capability %q is unsupported", capability)
		}
		if _, ok := seen[capability]; ok {
			continue
		}
		seen[capability] = struct{}{}
		out = append(out, capability)
	}
	sort.Strings(out)
	return out, nil
}
