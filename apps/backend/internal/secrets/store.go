package secrets

import (
	"context"
	"errors"
	"fmt"
)

// ErrNotFound is the sentinel returned (wrapped) by store implementations when
// a secret id is absent. Consumers that must tell "absent entry" apart from a
// genuine backend fault should match with errors.Is(err, secrets.ErrNotFound)
// rather than string-matching the error text.
var ErrNotFound = errors.New("secret not found")

// SecretStore abstracts secret storage. Implementations handle
// encryption/decryption internally.
type SecretStore interface {
	// Create stores a new secret (encrypts the value).
	Create(ctx context.Context, secret *SecretWithValue) error

	// Get retrieves secret metadata (without value).
	Get(ctx context.Context, id string) (*Secret, error)

	// Reveal retrieves the decrypted value of a secret.
	Reveal(ctx context.Context, id string) (string, error)

	// Update updates a secret's name and/or value.
	Update(ctx context.Context, id string, req *UpdateSecretRequest) error

	// Delete permanently removes a secret.
	Delete(ctx context.Context, id string) error

	// List returns all secrets without values.
	List(ctx context.Context) ([]*SecretListItem, error)

	// Close releases resources.
	Close() error
}

// ValidateGlobalReference verifies that a stored reference is usable by a
// shared agent or executor profile without decrypting its value.
func ValidateGlobalReference(ctx context.Context, store SecretStore, id string) error {
	if store == nil || id == "" {
		return fmt.Errorf("global secret reference is required")
	}
	secret, err := store.Get(ctx, id)
	if err != nil {
		return err
	}
	if normalizeStoredScope(secret.Scope) != ScopeGlobal {
		return fmt.Errorf("secret reference must use the global scope")
	}
	return nil
}

// ScopedSecretStore is the optional scope-aware extension used by user-facing
// settings, repository binding validation, and task-environment resolution.
// SecretStore deliberately remains small so existing integration adapters and
// test doubles do not need to know about workspace ownership.
type ScopedSecretStore interface {
	SecretStore
	ListScoped(ctx context.Context, opts SecretListOptions) ([]*SecretListItem, error)
	GetForWorkspace(ctx context.Context, id, workspaceID string) (*Secret, error)
	RevealGlobal(ctx context.Context, id string) (string, error)
	RevealForWorkspace(ctx context.Context, id, workspaceID string) (string, error)
	DeleteWorkspaceSecrets(ctx context.Context, workspaceID string) error
}
