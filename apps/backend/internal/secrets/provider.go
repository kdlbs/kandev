package secrets

import (
	"context"

	"github.com/kandev/kandev/internal/agent/credentials"
)

// SecretStoreProvider bridges SecretStore into the credential provider chain.
// It implements credentials.CredentialProvider.
type SecretStoreProvider struct {
	store SecretStore
}

var _ credentials.CredentialProvider = (*SecretStoreProvider)(nil)

// NewSecretStoreProvider creates a credential provider backed by the secret store.
func NewSecretStoreProvider(store SecretStore) *SecretStoreProvider {
	return &SecretStoreProvider{store: store}
}

// Name returns the provider name.
func (p *SecretStoreProvider) Name() string {
	return "secret_store"
}

// GetCredential retrieves a credential by env key from the encrypted store.
func (p *SecretStoreProvider) GetCredential(ctx context.Context, key string) (*credentials.Credential, error) {
	value, err := p.store.RevealByEnvKey(ctx, key)
	if err != nil {
		return nil, err
	}
	return &credentials.Credential{
		Key:    key,
		Value:  value,
		Source: "secret_store",
	}, nil
}

// ListAvailable returns all env keys that have stored secrets.
func (p *SecretStoreProvider) ListAvailable(ctx context.Context) ([]string, error) {
	items, err := p.store.List(ctx)
	if err != nil {
		return nil, err
	}
	keys := make([]string, len(items))
	for i, item := range items {
		keys[i] = item.EnvKey
	}
	return keys, nil
}
