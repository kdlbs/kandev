package secrets

import "context"

// SecretStore abstracts secret storage. Implementations handle
// encryption/decryption internally.
type SecretStore interface {
	// Create stores a new secret (encrypts the value).
	Create(ctx context.Context, secret *SecretWithValue) error

	// Get retrieves secret metadata (without value).
	Get(ctx context.Context, id string) (*Secret, error)

	// GetByEnvKey retrieves secret metadata by env key name.
	GetByEnvKey(ctx context.Context, envKey string) (*Secret, error)

	// Reveal retrieves the decrypted value of a secret.
	Reveal(ctx context.Context, id string) (string, error)

	// RevealByEnvKey retrieves the decrypted value by env key name.
	RevealByEnvKey(ctx context.Context, envKey string) (string, error)

	// Update updates a secret's value and/or metadata.
	Update(ctx context.Context, id string, req *UpdateSecretRequest) error

	// Delete permanently removes a secret.
	Delete(ctx context.Context, id string) error

	// List returns all secrets without values.
	List(ctx context.Context) ([]*SecretListItem, error)

	// ListByCategory returns secrets filtered by category.
	ListByCategory(ctx context.Context, category SecretCategory) ([]*SecretListItem, error)

	// Close releases resources.
	Close() error
}
