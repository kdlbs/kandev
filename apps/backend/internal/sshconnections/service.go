package sshconnections

import (
	"context"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/authz"
	"github.com/kandev/kandev/internal/secrets"
	"golang.org/x/crypto/ssh"
	"sync"
)

type Service struct {
	receiptsMu             sync.Mutex
	receipts               map[string]receipt
	enabled                bool
	store                  *Store
	vault                  *secrets.SSHIdentityVault
	migrationPlatformCheck func(context.Context, *ssh.Client) error
}

func NewService(enabled bool, store *Store, vault *secrets.SSHIdentityVault) *Service {
	return &Service{enabled: enabled, store: store, vault: vault}
}
func (s *Service) authorize(ctx context.Context) (string, error) {
	if !s.enabled {
		return "", ErrDisabled
	}
	identity, ok := authn.IdentityFromContext(ctx)
	if !ok {
		return "", ErrForbidden
	}
	subject := authz.Subject{UserID: identity.UserID, OrgID: identity.OrgID, OrgRole: authz.NormalizeOrgRole(string(identity.Role)), Unscoped: identity.Synthetic}
	if !authz.SubjectOrgScopes(subject).Has(authz.ScopeOrgConfigManage) {
		return "", ErrForbidden
	}
	if identity.Synthetic {
		return "single-user", nil
	}
	return identity.UserID, nil
}

type CreateIdentityRequest struct {
	RequestID  string `json:"request_id"`
	Label      string `json:"label"`
	Kind       string `json:"kind"`
	PrivateKey string `json:"private_key,omitempty"`
	Passphrase string `json:"passphrase,omitempty"`
	Reference  string `json:"reference,omitempty"`
}
