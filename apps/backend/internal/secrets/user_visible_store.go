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
	store  SecretStore
	scoped ScopedSecretStore
}

func NewUserVisibleStore(store SecretStore) SecretStore {
	if store == nil {
		return nil
	}
	scoped, _ := store.(ScopedSecretStore)
	return &UserVisibleStore{store: store, scoped: scoped}
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
	secret, err := s.store.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if normalizeStoredScope(secret.Scope) != ScopeGlobal {
		return nil, internalSecretNotFound(id)
	}
	return secret, nil
}

func (s *UserVisibleStore) Reveal(ctx context.Context, id string) (string, error) {
	if IsInternalID(id) {
		return "", internalSecretNotFound(id)
	}
	if s.scoped != nil {
		return s.scoped.RevealGlobal(ctx, id)
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
	if s.scoped != nil {
		return s.ListScoped(ctx, SecretListOptions{Scope: ScopeGlobal})
	}
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

func (s *UserVisibleStore) ListScoped(ctx context.Context, opts SecretListOptions) ([]*SecretListItem, error) {
	if s.scoped == nil {
		if opts.Scope == ScopeGlobal || opts.Scope == "" {
			return s.List(ctx)
		}
		return nil, fmt.Errorf("workspace-scoped secret storage is unavailable")
	}
	items, err := s.scoped.ListScoped(ctx, opts)
	if err != nil {
		return nil, err
	}
	return filterInternalSecretItems(items), nil
}

func (s *UserVisibleStore) GetForWorkspace(ctx context.Context, id, workspaceID string) (*Secret, error) {
	if IsInternalID(id) {
		return nil, internalSecretNotFound(id)
	}
	if s.scoped != nil {
		return s.scoped.GetForWorkspace(ctx, id, workspaceID)
	}
	secret, err := s.store.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if normalizeStoredScope(secret.Scope) == ScopeGlobal || secret.WorkspaceID == workspaceID {
		return secret, nil
	}
	return nil, internalSecretNotFound(id)
}

func (s *UserVisibleStore) RevealGlobal(ctx context.Context, id string) (string, error) {
	if IsInternalID(id) {
		return "", internalSecretNotFound(id)
	}
	if s.scoped != nil {
		return s.scoped.RevealGlobal(ctx, id)
	}
	return s.store.Reveal(ctx, id)
}

func (s *UserVisibleStore) RevealForWorkspace(ctx context.Context, id, workspaceID string) (string, error) {
	if IsInternalID(id) {
		return "", internalSecretNotFound(id)
	}
	if s.scoped != nil {
		return s.scoped.RevealForWorkspace(ctx, id, workspaceID)
	}
	if _, err := s.GetForWorkspace(ctx, id, workspaceID); err != nil {
		return "", err
	}
	return s.store.Reveal(ctx, id)
}

func (s *UserVisibleStore) DeleteWorkspaceSecrets(ctx context.Context, workspaceID string) error {
	if s.scoped == nil {
		return fmt.Errorf("workspace-scoped secret storage is unavailable")
	}
	return s.scoped.DeleteWorkspaceSecrets(ctx, workspaceID)
}

// The wrapped store is owned and closed by the repository container.
func (s *UserVisibleStore) Close() error { return nil }

func internalSecretNotFound(id string) error {
	return fmt.Errorf("%w: %s", ErrNotFound, id)
}

func filterInternalSecretItems(items []*SecretListItem) []*SecretListItem {
	visible := make([]*SecretListItem, 0, len(items))
	for _, item := range items {
		if item != nil && !IsInternalID(item.ID) {
			visible = append(visible, item)
		}
	}
	return visible
}
