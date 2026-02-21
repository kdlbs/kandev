package secrets

import "context"

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
