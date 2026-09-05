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
// the backend unable to ever adopt this server again. When the referenced
// secret is present but its current value cannot be read (a transient
// failure, not absence), this also falls back to creating a new one rather
// than overwriting it: see tryReuseCredentialSecret.
func storeControlServerCredential(ctx context.Context, store secrets.SecretStore, existingSecretID, token string) (string, error) {
	if store == nil {
		return "", errors.New("secret store is unavailable")
	}
	if token == "" {
		return "", errors.New("control server credential is required")
	}

	if existingSecretID != "" {
		reused, err := tryReuseCredentialSecret(ctx, store, existingSecretID, token)
		if err != nil {
			return "", err
		}
		if reused {
			return existingSecretID, nil
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

// tryReuseCredentialSecret overwrites existingSecretID in place with token
// and reports whether it did.
//
// It first reveals the secret's current value to prove the row is genuinely
// present and readable before overwriting it: Update alone cannot tell
// "absent" apart from "present but transiently unreadable" -- it blindly
// overwrites either way, since it never re-decrypts the prior value.
// Overwriting a slot whose value cannot currently be read would destroy
// whatever credential is there over what may be only a transient failure,
// which AC-EXECUTORS-CONTROL-OWNERSHIP-001.9 forbids ("shall not delete the
// stored credential" on a transient read error, "because the failure is on
// this side and destroying a secret over a transient read error is
// unrecoverable").
//
// A genuinely absent secret, or one that fails to reveal for any other
// reason, both report (false, nil) rather than an error: the caller falls
// back to creating a fresh secret instead of failing the whole operation, so
// a transient blip reading the OLD credential's row never blocks durably
// recording a new one (leaving one orphaned row behind is an acceptable cost
// next to destroying a credential that might still be needed). Only a
// failure of the Update call itself -- the row having just been proven
// present and readable, then failing to write -- is surfaced as an error.
func tryReuseCredentialSecret(ctx context.Context, store secrets.SecretStore, existingSecretID, token string) (bool, error) {
	if _, err := revealGlobalSecret(ctx, store, existingSecretID); err != nil {
		return false, nil
	}
	if err := store.Update(ctx, existingSecretID, &secrets.UpdateSecretRequest{Value: &token}); err != nil {
		if errors.Is(err, secrets.ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	return true, nil
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
