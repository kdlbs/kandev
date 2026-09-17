package backendapp

import (
	"context"
	"errors"
	"time"

	"github.com/kandev/kandev/internal/orchestration/models"
	"github.com/kandev/kandev/internal/secrets"
)

// Metadata-only, explicitly referenced lookups. Never reveal a value, scan a
// vault, or fall back to a different secret/account.
type assistantCredentialReader struct{ store secrets.SecretStore }

func (r assistantCredentialReader) CredentialHealth(ctx context.Context, workspace string, d models.CredentialDescriptor) models.CredentialValidation {
	result := models.CredentialValidation{Status: credentialUnavailable}
	if d.Resolver != "kandev" {
		return result
	}
	scoped, ok := r.store.(secrets.ScopedSecretStore)
	if !ok {
		return result
	}
	metadata, err := scoped.GetForWorkspace(ctx, d.Reference, workspace)
	checked := time.Now().UTC()
	result.ValidatedAt = &checked
	if errors.Is(err, secrets.ErrNotFound) || errors.Is(err, secrets.ErrWorkspaceAccessDenied) {
		result.Status = "missing"
	} else if err == nil && metadata != nil {
		result.Status = "ready"
		result.ConfigurationGeneration = metadata.UpdatedAt.UTC().Format(time.RFC3339Nano)
	}
	return result
}
