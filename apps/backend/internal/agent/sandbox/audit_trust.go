package sandbox

import (
	"context"
	"math"
	"time"

	"github.com/google/uuid"
	contract "github.com/kandev/kandev/internal/common/sandbox"
)

type AuditTrustRequest struct {
	ExpectedVersion int64  `json:"expected_version"`
	PublicKeyPEM    string `json:"public_key_pem"`
	Revoke          bool   `json:"revoke"`
}

// AuditTrustRevision records operator trust. Embedded artifact keys cannot create it.
type AuditTrustRevision struct {
	SchemaVersion  int       `json:"schema_version"`
	ID             string    `json:"id"`
	Version        int64     `json:"version"`
	OrgID          string    `json:"org_id"`
	WorkspaceID    string    `json:"workspace_id"`
	InstallationID string    `json:"installation_id"`
	ProviderKey    string    `json:"provider_key"`
	Fingerprint    string    `json:"fingerprint"`
	PublicKeyPEM   string    `json:"public_key_pem"`
	Revoked        bool      `json:"revoked"`
	OperatorID     string    `json:"operator_id"`
	Source         string    `json:"source"`
	CreatedAt      time.Time `json:"created_at"`
	Digest         string    `json:"digest"`
}

func (s *Service) GetAuditTrust(ctx context.Context, sessionID, attemptID string) (*AuditTrustRevision, error) {
	scope, err := s.auditTrustScope(ctx, sessionID, attemptID)
	if err != nil {
		return nil, err
	}
	return s.readAuditTrust(ctx, scope)
}

func (s *Service) SetAuditTrust(ctx context.Context, sessionID, attemptID string, request AuditTrustRequest) (*AuditTrustRevision, error) {
	operator, err := s.operator(ctx)
	if err != nil {
		return nil, err
	}
	scope, err := s.auditTrustScope(ctx, sessionID, attemptID)
	if err != nil {
		return nil, err
	}
	if request.ExpectedVersion < 0 || request.ExpectedVersion == math.MaxInt64 || request.Revoke && request.PublicKeyPEM != "" {
		return nil, contract.ErrInvalidPolicy
	}
	revision := scope.revision()
	revision.ID, revision.Version = uuid.NewString(), request.ExpectedVersion+1
	revision.OperatorID, revision.Source, revision.CreatedAt = operator.UserID, auditTrustSourceOperator, time.Now().UTC()
	revision.Revoked = request.Revoke
	if !request.Revoke {
		revision.PublicKeyPEM, revision.Fingerprint, err = canonicalAuditPublicKey(request.PublicKeyPEM)
		if err != nil {
			return nil, err
		}
	}
	revision.Digest, err = auditTrustDigest(*revision)
	if err != nil {
		return nil, err
	}
	if err = s.insertAuditTrust(ctx, scope, revision, request.ExpectedVersion); err != nil {
		return nil, err
	}
	return revision, nil
}
