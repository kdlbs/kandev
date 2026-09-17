package backendapp

import (
	"context"

	"github.com/kandev/kandev/internal/orchestration/models"
	"github.com/kandev/kandev/internal/secrets"
)

// Metadata-only, explicitly referenced lookups. Never reveal a value, scan a
// vault, or fall back to a different secret/account.
type assistantCredentialReader struct{ store secrets.SecretStore }

func (r assistantCredentialReader) CredentialHealth(ctx context.Context, workspace string, d models.CredentialDescriptor) string {
	if d.Resolver != "kandev" {
		return credentialUnavailable
	}
	scoped, ok := r.store.(secrets.ScopedSecretStore)
	if !ok {
		return credentialUnavailable
	}
	if _, err := scoped.GetForWorkspace(ctx, d.Reference, workspace); err != nil {
		return credentialUnavailable
	}
	return "ready"
}
