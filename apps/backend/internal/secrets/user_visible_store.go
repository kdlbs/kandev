package secrets

import (
	"context"
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"
)

const (
	internalGitHubSecretPrefix  = "github:"
	internalRuntimeSecretPrefix = "runtime:"
	legacyAgentctlAuthPrefix    = "agentctl-auth-"
	legacyAgentctlNoncePrefix   = "agentctl-bootstrap-"
)

// IsInternalID reports whether a secret is owned by backend infrastructure
// and must never be listed, revealed, or selected as an agent credential.
func IsInternalID(id string) bool {
	normalized := strings.ToLower(strings.TrimSpace(id))
	return strings.HasPrefix(normalized, internalGitHubSecretPrefix) ||
		strings.HasPrefix(normalized, internalRuntimeSecretPrefix)
}

func isInternalSecret(id, name string) bool {
	if IsInternalID(id) {
		return true
	}
	normalizedName := strings.ToLower(strings.TrimSpace(name))
	return strings.HasPrefix(normalizedName, legacyAgentctlAuthPrefix) ||
		strings.HasPrefix(normalizedName, legacyAgentctlNoncePrefix)
}

// UserVisibleStore restricts a SecretStore to user-managed credentials. The
// underlying store remains available to integration services for their
// deterministic internal keys.
type UserVisibleStore struct {
	store SecretStore
	// scoped carries both the scope-aware read/lifecycle surface and the
	// transfer surface the wrapper exposes to the service.
	scoped userVisibleScopedStore
}

// userVisibleScopedStore is the store surface UserVisibleStore forwards:
// scope-aware reads/lifecycle plus atomic scope transfers.
type userVisibleScopedStore interface {
	ScopedSecretStore
	SecretTransferStore
}

// NewUserVisibleStore wraps a SecretStore, returning a store that exposes
// only user-managed credentials; it returns nil when store is nil.
func NewUserVisibleStore(store SecretStore) SecretStore {
	if store == nil {
		return nil
	}
	scoped, _ := store.(userVisibleScopedStore)
	return &UserVisibleStore{store: store, scoped: scoped}
}

// Create creates a secret owned by the current user.
func (s *UserVisibleStore) Create(ctx context.Context, secret *SecretWithValue) error {
	if secret != nil && isInternalSecret(secret.ID, secret.Name) {
		return internalSecretNotFound(secret.ID)
	}
	return s.store.Create(ctx, secret)
}

// Get returns the global user-visible secret with the given id, treating
// internal ids and non-global scopes as not found.
func (s *UserVisibleStore) Get(ctx context.Context, id string) (*Secret, error) {
	if IsInternalID(id) {
		return nil, internalSecretNotFound(id)
	}
	secret, err := s.store.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if isInternalSecret(secret.ID, secret.Name) {
		return nil, internalSecretNotFound(id)
	}
	if normalizeStoredScope(secret.Scope) != ScopeGlobal {
		return nil, internalSecretNotFound(id)
	}
	return secret, nil
}

// Reveal returns the plaintext value of the user-visible secret with the given id.
func (s *UserVisibleStore) Reveal(ctx context.Context, id string) (string, error) {
	return s.RevealGlobal(ctx, id)
}

// Update updates the user-visible secret with the given id.
func (s *UserVisibleStore) Update(ctx context.Context, id string, req *UpdateSecretRequest) error {
	if IsInternalID(id) {
		return internalSecretNotFound(id)
	}
	if req != nil && req.Name != nil && isInternalSecret("", *req.Name) {
		return internalSecretNotFound(id)
	}
	if _, err := s.Get(ctx, id); err != nil {
		return err
	}
	return s.store.Update(ctx, id, req)
}

// Delete deletes the user-visible secret with the given id.
func (s *UserVisibleStore) Delete(ctx context.Context, id string) error {
	if IsInternalID(id) {
		return internalSecretNotFound(id)
	}
	if _, err := s.Get(ctx, id); err != nil {
		return err
	}
	return s.store.Delete(ctx, id)
}

// List returns all user-visible (non-internal) secrets.
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
		if item != nil && !isInternalSecret(item.ID, item.Name) {
			visible = append(visible, item)
		}
	}
	return visible, nil
}

// ListScoped returns user-visible secrets matching the given scope options.
func (s *UserVisibleStore) ListScoped(ctx context.Context, opts SecretListOptions) ([]*SecretListItem, error) {
	if opts.Scope == "" {
		opts.Scope = ScopeGlobal
	}
	if s.scoped == nil {
		if opts.Scope == ScopeGlobal {
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

// GetForWorkspace returns the user-visible secret with the given id scoped to the workspace.
func (s *UserVisibleStore) GetForWorkspace(ctx context.Context, id, workspaceID string) (*Secret, error) {
	if IsInternalID(id) {
		return nil, internalSecretNotFound(id)
	}
	if s.scoped == nil {
		return nil, fmt.Errorf("workspace-scoped secret storage is unavailable")
	}
	secret, err := s.scoped.GetForWorkspace(ctx, id, workspaceID)
	if err != nil {
		return nil, err
	}
	if isInternalSecret(secret.ID, secret.Name) {
		return nil, internalSecretNotFound(id)
	}
	return secret, nil
}

// RevealGlobal returns the plaintext value of the global user-visible secret with the given id.
func (s *UserVisibleStore) RevealGlobal(ctx context.Context, id string) (string, error) {
	if _, err := s.Get(ctx, id); err != nil {
		return "", err
	}
	if s.scoped != nil {
		return s.scoped.RevealGlobal(ctx, id)
	}
	return s.store.Reveal(ctx, id)
}

// RevealForWorkspace returns the plaintext value of the user-visible secret with the given id scoped to the workspace.
func (s *UserVisibleStore) RevealForWorkspace(ctx context.Context, id, workspaceID string) (string, error) {
	if _, err := s.GetForWorkspace(ctx, id, workspaceID); err != nil {
		return "", err
	}
	if s.scoped != nil {
		return s.scoped.RevealForWorkspace(ctx, id, workspaceID)
	}
	return s.store.Reveal(ctx, id)
}

// DeleteWorkspaceSecrets deletes all user-visible secrets scoped to the given workspace.
func (s *UserVisibleStore) DeleteWorkspaceSecrets(ctx context.Context, workspaceID string) error {
	if s.scoped == nil {
		return fmt.Errorf("workspace-scoped secret storage is unavailable")
	}
	return s.scoped.DeleteWorkspaceSecrets(ctx, workspaceID)
}

// CopyScoped copies the user-visible secret to the target scope after the destination verifier approves.
func (s *UserVisibleStore) CopyScoped(ctx context.Context, sourceID, sourceWorkspaceID string, targetScope SecretScope, targetWorkspaceID string, requestedName *string, verifyDestination func(context.Context) error) (*Secret, error) {
	if IsInternalID(sourceID) {
		return nil, internalSecretNotFound(sourceID)
	}
	if s.scoped == nil {
		return nil, fmt.Errorf("workspace-scoped secret storage is unavailable")
	}
	return s.scoped.CopyScoped(ctx, sourceID, sourceWorkspaceID, targetScope, targetWorkspaceID, requestedName, verifyDestination)
}

// MoveScoped moves the user-visible secret to the target scope after the destination verifier approves.
func (s *UserVisibleStore) MoveScoped(ctx context.Context, sourceID, sourceWorkspaceID string, targetScope SecretScope, targetWorkspaceID string, requestedName *string, verifyDestination func(context.Context) error) (*Secret, error) {
	if IsInternalID(sourceID) {
		return nil, internalSecretNotFound(sourceID)
	}
	if s.scoped == nil {
		return nil, fmt.Errorf("workspace-scoped secret storage is unavailable")
	}
	return s.scoped.MoveScoped(ctx, sourceID, sourceWorkspaceID, targetScope, targetWorkspaceID, requestedName, verifyDestination)
}

// DeleteWorkspaceSecretsTx deletes workspace-scoped user-visible secrets inside the given transaction.
func (s *UserVisibleStore) DeleteWorkspaceSecretsTx(ctx context.Context, tx *sqlx.Tx, workspaceID string) error {
	if s.scoped == nil {
		return fmt.Errorf("workspace-scoped secret storage is unavailable")
	}
	transactional, ok := s.scoped.(WorkspaceSecretTransactionalDeleter)
	if !ok {
		return fmt.Errorf("transactional workspace-scoped secret storage is unavailable")
	}
	return transactional.DeleteWorkspaceSecretsTx(ctx, tx, workspaceID)
}

// The wrapped store is owned and closed by the repository container.
func (s *UserVisibleStore) Close() error { return nil }

// internalSecretNotFound returns the store's sentinel not-found error annotated with the given secret id.
func internalSecretNotFound(id string) error {
	return fmt.Errorf("%w: %s", ErrNotFound, id)
}

// filterInternalSecretItems returns only the user-managed items, dropping internal ones.
func filterInternalSecretItems(items []*SecretListItem) []*SecretListItem {
	visible := make([]*SecretListItem, 0, len(items))
	for _, item := range items {
		if item != nil && !isInternalSecret(item.ID, item.Name) {
			visible = append(visible, item)
		}
	}
	return visible
}
