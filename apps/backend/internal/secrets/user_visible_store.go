package secrets

import (
	"context"
	"fmt"
	"strings"
)

const internalGitHubSecretPrefix = "github:"

// IsInternalID reports whether a secret is owned by backend infrastructure
// and must never be listed, revealed, or selected as an agent credential.
func IsInternalID(id string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(id)), internalGitHubSecretPrefix)
}

// UserVisibleStore restricts a SecretStore to user-managed credentials. The
// underlying store remains available to integration services for their
// deterministic internal keys.
type UserVisibleStore struct {
	store SecretStore
}

func NewUserVisibleStore(store SecretStore) SecretStore {
	if store == nil {
		return nil
	}
	return &UserVisibleStore{store: store}
}

func (s *UserVisibleStore) Create(ctx context.Context, secret *SecretWithValue) error {
	if secret != nil && IsInternalID(secret.ID) {
		return internalSecretNotFound(secret.ID)
	}
	return s.store.Create(ctx, secret)
}

func (s *UserVisibleStore) Get(ctx context.Context, id string) (*Secret, error) {
	if IsInternalID(id) {
		return nil, internalSecretNotFound(id)
	}
	return s.store.Get(ctx, id)
}

func (s *UserVisibleStore) Reveal(ctx context.Context, id string) (string, error) {
	if IsInternalID(id) {
		return "", internalSecretNotFound(id)
	}
	return s.store.Reveal(ctx, id)
}

func (s *UserVisibleStore) Update(ctx context.Context, id string, req *UpdateSecretRequest) error {
	if IsInternalID(id) {
		return internalSecretNotFound(id)
	}
	return s.store.Update(ctx, id, req)
}

func (s *UserVisibleStore) Delete(ctx context.Context, id string) error {
	if IsInternalID(id) {
		return internalSecretNotFound(id)
	}
	return s.store.Delete(ctx, id)
}

func (s *UserVisibleStore) List(ctx context.Context) ([]*SecretListItem, error) {
	items, err := s.store.List(ctx)
	if err != nil {
		return nil, err
	}
	visible := make([]*SecretListItem, 0, len(items))
	for _, item := range items {
		if item != nil && !IsInternalID(item.ID) {
			visible = append(visible, item)
		}
	}
	return visible, nil
}

// The wrapped store is owned and closed by the repository container.
func (s *UserVisibleStore) Close() error { return nil }

func internalSecretNotFound(id string) error {
	return fmt.Errorf("%w: %s", ErrNotFound, id)
}
