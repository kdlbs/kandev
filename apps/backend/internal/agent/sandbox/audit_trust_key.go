package sandbox

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"strings"

	contract "github.com/kandev/kandev/internal/common/sandbox"
)

const auditTrustPublicKeyType = "PUBLIC KEY"
const auditTrustSourceOperator = "operator"

type auditTrustScope struct {
	OrgID          string `json:"org_id"`
	WorkspaceID    string `json:"workspace_id"`
	InstallationID string `json:"installation_id"`
	ProviderKey    string `json:"provider_key"`
}

func (s *Service) auditTrustScope(ctx context.Context, sessionID, attemptID string) (auditTrustScope, error) {
	org, err := s.authorizeReport(ctx, sessionID, attemptID)
	if err != nil {
		return auditTrustScope{}, err
	}
	attempt, err := s.ReadAttempt(ctx, org, attemptID)
	if err != nil {
		return auditTrustScope{}, err
	}
	if attempt.Snapshot.Provider.ProtocolVersion() != 2 {
		return auditTrustScope{}, ErrIneligible
	}
	return auditTrustScope{OrgID: org, WorkspaceID: attempt.Snapshot.WorkspaceID, InstallationID: attempt.Snapshot.Provider.InstallationID, ProviderKey: attempt.Snapshot.Provider.Key}, nil
}

func (scope auditTrustScope) revision() *AuditTrustRevision {
	return &AuditTrustRevision{SchemaVersion: 1, OrgID: scope.OrgID, WorkspaceID: scope.WorkspaceID, InstallationID: scope.InstallationID, ProviderKey: scope.ProviderKey, Revoked: true, Source: "unconfigured"}
}

func (scope auditTrustScope) digest() string {
	// Only fixed string fields enter this encoding.
	raw, _ := json.Marshal(scope)
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}

func canonicalAuditPublicKey(value string) (string, string, error) {
	if len(value) == 0 || len(value) > 16<<10 {
		return "", "", contract.ErrInvalidPolicy
	}
	if !strings.HasPrefix(strings.TrimSpace(value), "-----BEGIN PUBLIC KEY-----") {
		return "", "", contract.ErrInvalidPolicy
	}
	block, rest := pem.Decode([]byte(value))
	if block == nil || block.Type != auditTrustPublicKeyType || len(block.Headers) != 0 || len(bytes.TrimSpace(rest)) != 0 {
		return "", "", contract.ErrInvalidPolicy
	}
	key, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return "", "", contract.ErrInvalidPolicy
	}
	canonical, err := x509.MarshalPKIXPublicKey(key)
	if err != nil {
		return "", "", contract.ErrInvalidPolicy
	}
	digest := sha256.Sum256(canonical)
	return string(pem.EncodeToMemory(&pem.Block{Type: auditTrustPublicKeyType, Bytes: canonical})), hex.EncodeToString(digest[:]), nil
}

func auditTrustDigest(value AuditTrustRevision) (string, error) {
	value.Digest = ""
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:]), nil
}
