package lifecycle

import (
	"context"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/secrets"
)

// controlServerCredentialSecretName is the fixed secret name for the
// installation's single standalone control-server ownership credential. It
// is never looked up by name -- only by the ID recorded in
// models.ControlServerRecord.CredentialSecretID -- so a fixed, non-unique
// name is safe.
const controlServerCredentialSecretName = "agent-survival-control-server-credential"

// storeControlServerCredential durably stores the control server's ownership
// credential and returns the secret ID that
// models.ControlServerRecord.CredentialSecretID should reference. Storing
// the token itself in the record would put a bearer credential for a
// service that executes commands in a user's worktree into the database in
// clear (design 01, "Ownership identity and credential") -- the record
// holds only this reference.
//
// existingSecretID, when non-empty, updates that secret in place so an
// ordinary credential rotation reuses the same reference and the record's
// CredentialSecretID never has to change. When the referenced secret is
// absent (e.g. deleted out of band), this falls back to creating a new one
// rather than failing: refusing to store a rotated credential would leave
// the backend unable to ever adopt this server again.
func storeControlServerCredential(ctx context.Context, store secrets.SecretStore, existingSecretID, token string) (string, error) {
	if store == nil {
		return "", errors.New("secret store is unavailable")
	}
	if token == "" {
		return "", errors.New("control server credential is required")
	}

	if existingSecretID != "" {
		err := store.Update(ctx, existingSecretID, &secrets.UpdateSecretRequest{Value: &token})
		if err == nil {
			return existingSecretID, nil
		}
		if !errors.Is(err, secrets.ErrNotFound) {
			return "", err
		}
	}

	secret := &secrets.SecretWithValue{
		Secret: secrets.Secret{Name: controlServerCredentialSecretName},
		Value:  token,
	}
	if err := store.Create(ctx, secret); err != nil {
		return "", err
	}
	return secret.ID, nil
}

// revealControlServerCredential retrieves the decrypted ownership credential
// referenced by a control-server record. An empty reference is rejected
// before any store contact, distinct from a not-found lookup, so a caller
// can tell "record has no credential" apart from "stored reference is
// stale" -- design 02's failure table treats these differently
// (credential-unavailable vs authentication failure).
func revealControlServerCredential(ctx context.Context, store secrets.SecretStore, secretID string) (string, error) {
	if secretID == "" {
		return "", errors.New("control server credential reference is empty")
	}
	value, err := revealGlobalSecret(ctx, store, secretID)
	if err != nil {
		return "", fmt.Errorf("reveal control server credential: %w", err)
	}
	return value, nil
}
