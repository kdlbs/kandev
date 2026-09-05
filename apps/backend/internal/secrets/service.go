package secrets

import (
	"context"
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/common/logger"
)

// Service provides business logic and validation for secrets.
type Service struct {
	store               SecretStore
	logger              *logger.Logger
	workspaceAuthorizer func(context.Context, string) error
	workspaceExistence  func(context.Context, string) error
}

// NewService creates a new secrets service.
func NewService(store SecretStore, log *logger.Logger) *Service {
	return &Service{
		store:  store,
		logger: log,
	}
}

// SetWorkspaceAuthorizer wires the workspace reach check used by the
// workspace-scoped API. The callback is intentionally a function so the
// secrets package does not depend on the task service package.
func (s *Service) SetWorkspaceAuthorizer(authorizer func(context.Context, string) error) {
	s.workspaceAuthorizer = authorizer
}

// SetWorkspaceExistenceChecker wires a workspace-existence check that runs
// unconditionally for copy/move destinations, including auth-disabled
// contexts where the authorizer is a no-op. A nil checker fails closed:
// workspace destinations resolve as ErrWorkspaceAccessDenied.
func (s *Service) SetWorkspaceExistenceChecker(checker func(context.Context, string) error) {
	s.workspaceExistence = checker
}

// validateCreate validates a create request, trimming the name and checking
// length, scope, and workspace constraints.
func (s *Service) validateCreate(req *CreateSecretRequest) error {
	req.Name = strings.TrimSpace(req.Name)

	if req.Name == "" || len(req.Name) > 100 {
		return fmt.Errorf("name must be 1-100 bytes")
	}
	if req.Value == "" || len(req.Value) > 10000 {
		return fmt.Errorf("value must be 1-10000 characters")
	}
	if req.Scope == "" {
		req.Scope = ScopeGlobal
	}
	if err := validateSecretScope(req.Scope, req.WorkspaceID); err != nil {
		return fmt.Errorf("scope validation: %w", err)
	}
	return nil
}

// validateUpdate validates an update request, trimming the name and checking
// length constraints on the fields that are present.
func (s *Service) validateUpdate(req *UpdateSecretRequest) error {
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		req.Name = &name
		if name == "" || len(name) > 100 {
			return fmt.Errorf("name must be 1-100 bytes")
		}
	}
	if req.Value != nil && (len(*req.Value) == 0 || len(*req.Value) > 10000) {
		return fmt.Errorf("value must be 1-10000 characters")
	}
	return nil
}

// Create validates and stores a new secret.
func (s *Service) Create(ctx context.Context, req *CreateSecretRequest) (*SecretListItem, error) {
	if err := s.validateCreate(req); err != nil {
		return nil, fmt.Errorf("validation: %w", err)
	}
	if err := s.authorizeScope(ctx, req.Scope, req.WorkspaceID); err != nil {
		return nil, err
	}

	secret := &SecretWithValue{
		Secret: Secret{
			Name:        req.Name,
			Scope:       req.Scope,
			WorkspaceID: req.WorkspaceID,
		},
		Value: req.Value,
	}

	if err := s.store.Create(ctx, secret); err != nil {
		return nil, fmt.Errorf("create secret: %w", err)
	}

	return secretListItem(&secret.Secret), nil
}

// Get retrieves secret metadata.
func (s *Service) Get(ctx context.Context, id string) (*Secret, error) {
	secret, err := s.store.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if normalizeStoredScope(secret.Scope) != ScopeGlobal {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	return secret, nil
}

// GetForWorkspace retrieves a Global or same-workspace secret after checking
// the caller's workspace access.
func (s *Service) GetForWorkspace(ctx context.Context, id, workspaceID string) (*Secret, error) {
	if err := s.authorizeScope(ctx, ScopeWorkspace, workspaceID); err != nil {
		return nil, err
	}
	if scoped, ok := s.store.(ScopedSecretStore); ok {
		return scoped.GetForWorkspace(ctx, id, workspaceID)
	}
	secret, err := s.store.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if normalizeStoredScope(secret.Scope) == ScopeGlobal || secret.WorkspaceID == workspaceID {
		return secret, nil
	}
	return nil, fmt.Errorf("%w: %s", ErrNotFound, id)
}

// GetWorkspaceSecret retrieves a user-visible Workspace secret. Unlike
// GetForWorkspace, this deliberately excludes Global secrets because it backs
// workspace-scoped CRUD and reveal routes rather than repository selection.
func (s *Service) GetWorkspaceSecret(ctx context.Context, id, workspaceID string) (*Secret, error) {
	secret, err := s.GetForWorkspace(ctx, id, workspaceID)
	if err != nil {
		return nil, err
	}
	if normalizeStoredScope(secret.Scope) != ScopeWorkspace || secret.WorkspaceID != workspaceID {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	return secret, nil
}

// Reveal returns the decrypted secret value.
func (s *Service) Reveal(ctx context.Context, id string) (string, error) {
	if _, err := s.Get(ctx, id); err != nil {
		return "", err
	}
	if scoped, ok := s.store.(ScopedSecretStore); ok {
		return scoped.RevealGlobal(ctx, id)
	}
	return s.store.Reveal(ctx, id)
}

// RevealForWorkspace reveals a Global or same-workspace secret.
func (s *Service) RevealForWorkspace(ctx context.Context, id, workspaceID string) (string, error) {
	if _, err := s.GetForWorkspace(ctx, id, workspaceID); err != nil {
		return "", err
	}
	if scoped, ok := s.store.(ScopedSecretStore); ok {
		return scoped.RevealForWorkspace(ctx, id, workspaceID)
	}
	return s.store.Reveal(ctx, id)
}

// RevealWorkspaceSecret returns the decrypted value of a Workspace secret
// after checking the caller's workspace access.
func (s *Service) RevealWorkspaceSecret(ctx context.Context, id, workspaceID string) (string, error) {
	if _, err := s.GetWorkspaceSecret(ctx, id, workspaceID); err != nil {
		return "", err
	}
	if scoped, ok := s.store.(ScopedSecretStore); ok {
		return scoped.RevealForWorkspace(ctx, id, workspaceID)
	}
	return s.store.Reveal(ctx, id)
}

// Update validates and updates a secret.
func (s *Service) Update(ctx context.Context, id string, req *UpdateSecretRequest) (*SecretListItem, error) {
	if err := s.validateUpdate(req); err != nil {
		return nil, fmt.Errorf("validation: %w", err)
	}

	if _, err := s.Get(ctx, id); err != nil {
		return nil, err
	}
	if err := s.store.Update(ctx, id, req); err != nil {
		return nil, fmt.Errorf("update secret: %w", err)
	}

	secret, err := s.store.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	return secretListItem(secret), nil
}

// UpdateForWorkspace updates a Workspace secret after checking its scope and
// the caller's access. Global secrets remain valid through this endpoint too.
func (s *Service) UpdateForWorkspace(ctx context.Context, id, workspaceID string, req *UpdateSecretRequest) (*SecretListItem, error) {
	if err := s.validateUpdate(req); err != nil {
		return nil, fmt.Errorf("validation: %w", err)
	}
	if _, err := s.GetForWorkspace(ctx, id, workspaceID); err != nil {
		return nil, err
	}
	if err := s.store.Update(ctx, id, req); err != nil {
		return nil, fmt.Errorf("update secret: %w", err)
	}
	secret, err := s.store.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return secretListItem(secret), nil
}

// UpdateWorkspaceSecret validates and updates a Workspace secret after
// checking the caller's workspace access.
func (s *Service) UpdateWorkspaceSecret(ctx context.Context, id, workspaceID string, req *UpdateSecretRequest) (*SecretListItem, error) {
	if err := s.validateUpdate(req); err != nil {
		return nil, fmt.Errorf("validation: %w", err)
	}
	if _, err := s.GetWorkspaceSecret(ctx, id, workspaceID); err != nil {
		return nil, err
	}
	if err := s.store.Update(ctx, id, req); err != nil {
		return nil, fmt.Errorf("update secret: %w", err)
	}
	secret, err := s.store.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return secretListItem(secret), nil
}

// Delete removes a secret.
func (s *Service) Delete(ctx context.Context, id string) error {
	if _, err := s.Get(ctx, id); err != nil {
		return err
	}
	return s.store.Delete(ctx, id)
}

// DeleteForWorkspace deletes a Global or same-workspace secret.
func (s *Service) DeleteForWorkspace(ctx context.Context, id, workspaceID string) error {
	if _, err := s.GetForWorkspace(ctx, id, workspaceID); err != nil {
		return err
	}
	return s.store.Delete(ctx, id)
}

// DeleteWorkspaceSecret deletes a Workspace secret after checking the
// caller's workspace access.
func (s *Service) DeleteWorkspaceSecret(ctx context.Context, id, workspaceID string) error {
	if _, err := s.GetWorkspaceSecret(ctx, id, workspaceID); err != nil {
		return err
	}
	return s.store.Delete(ctx, id)
}

// List returns all secrets without values.
func (s *Service) List(ctx context.Context) ([]*SecretListItem, error) {
	return s.ListScoped(ctx, SecretListOptions{Scope: ScopeGlobal})
}

// ListScoped lists secrets in one scope. Workspace listings require access to
// that workspace before any metadata is returned.
func (s *Service) ListScoped(ctx context.Context, opts SecretListOptions) ([]*SecretListItem, error) {
	if opts.Scope == "" {
		opts.Scope = ScopeGlobal
	}
	if err := validateListOptions(opts); err != nil {
		return nil, err
	}
	if opts.Scope == ScopeWorkspace {
		if err := s.authorizeScope(ctx, ScopeWorkspace, opts.WorkspaceID); err != nil {
			return nil, err
		}
	}
	if scoped, ok := s.store.(ScopedSecretStore); ok {
		return scoped.ListScoped(ctx, opts)
	}
	if opts.Scope == ScopeGlobal {
		return s.store.List(ctx)
	}
	return nil, fmt.Errorf("workspace-scoped secret storage is unavailable")
}

// authorizeScope enforces workspace authorization for workspace-scoped
// requests; global scope requires no check.
func (s *Service) authorizeScope(ctx context.Context, scope SecretScope, workspaceID string) error {
	if scope != ScopeWorkspace {
		return nil
	}
	if s.workspaceAuthorizer == nil {
		return fmt.Errorf("workspace secret access is unavailable")
	}
	return s.workspaceAuthorizer(ctx, workspaceID)
}

// secretListItem converts a stored secret into a list item with a
// normalized scope.
func secretListItem(secret *Secret) *SecretListItem {
	return &SecretListItem{
		ID:          secret.ID,
		Name:        secret.Name,
		Scope:       normalizeStoredScope(secret.Scope),
		WorkspaceID: secret.WorkspaceID,
		HasValue:    true,
		CreatedAt:   secret.CreatedAt,
		UpdatedAt:   secret.UpdatedAt,
	}
}

// Copy copies a secret into another scope/workspace without removing the
// source. Errors are classified with the package sentinels: validation
// failures wrap ErrSecretValidation, missing/unauthorized sources and
// destinations surface ErrNotFound or ErrWorkspaceAccessDenied, and raw
// lookup/storage failures pass through unclassified (sanitized 500 upstream).
func (s *Service) Copy(ctx context.Context, sourceID, sourceWorkspaceID string, req *CopySecretRequest) (*SecretListItem, error) {
	return s.copyOrMove(ctx, sourceID, sourceWorkspaceID, req, false)
}

// Move copies a secret into another scope/workspace and removes the source in
// one atomic store operation.
func (s *Service) Move(ctx context.Context, sourceID, sourceWorkspaceID string, req *CopySecretRequest) (*SecretListItem, error) {
	return s.copyOrMove(ctx, sourceID, sourceWorkspaceID, req, true)
}

// copyOrMove is the shared implementation behind Copy and Move: it validates
// the request, resolves the source secret and target name, and performs the
// copy or move in one store operation.
func (s *Service) copyOrMove(ctx context.Context, sourceID, sourceWorkspaceID string, req *CopySecretRequest, move bool) (*SecretListItem, error) {
	if req == nil {
		return nil, fmt.Errorf("%w: request is required", ErrSecretValidation)
	}
	if err := validateSecretScope(req.Scope, req.WorkspaceID); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSecretValidation, err)
	}

	var source *Secret
	var err error
	if strings.TrimSpace(sourceWorkspaceID) == "" {
		source, err = s.Get(ctx, sourceID)
	} else {
		source, err = s.GetWorkspaceSecret(ctx, sourceID, sourceWorkspaceID)
	}
	if err != nil {
		return nil, err
	}

	// An explicit name (or an explicit null) is validated here; an omitted
	// name is resolved from the source row inside the store's transaction so
	// a concurrent rename can never split the copied name from its value.
	if req.nameNull || req.Name != nil {
		if _, err := resolveTransferName(req, ""); err != nil {
			return nil, err
		}
	}

	if normalizeStoredScope(req.Scope) == normalizeStoredScope(source.Scope) && req.WorkspaceID == source.WorkspaceID {
		return nil, fmt.Errorf("%w: source and destination scope are the same", ErrSecretValidation)
	}

	transferStore, ok := s.store.(SecretTransferStore)
	if !ok {
		return nil, fmt.Errorf("secret transfer storage is unavailable")
	}

	verifyDestination := s.destinationVerifier(ctx, req)

	var secret *Secret
	if move {
		secret, err = transferStore.MoveScoped(ctx, sourceID, source.WorkspaceID, req.Scope, req.WorkspaceID, req.Name, verifyDestination)
	} else {
		secret, err = transferStore.CopyScoped(ctx, sourceID, source.WorkspaceID, req.Scope, req.WorkspaceID, req.Name, verifyDestination)
	}
	if err != nil {
		return nil, err
	}
	return secretListItem(secret), nil
}

// resolveTransferName applies the name presence contract: an omitted name
// (and the absence of an explicit null) uses the source name; an explicit
// null, empty, whitespace-only, or over-length name is a validation error.
// The limit is 100 UTF-8 bytes, matching create/update validation.
func resolveTransferName(req *CopySecretRequest, sourceName string) (string, error) {
	if req.nameNull {
		return "", fmt.Errorf("%w: name must not be null", ErrSecretValidation)
	}
	if req.Name == nil {
		return sourceName, nil
	}
	name := strings.TrimSpace(*req.Name)
	if name == "" {
		return "", fmt.Errorf("%w: name must not be empty", ErrSecretValidation)
	}
	if len(name) > 100 {
		return "", fmt.Errorf("%w: name must be 1-100 bytes", ErrSecretValidation)
	}
	return name, nil
}

// destinationVerifier returns the in-transaction callback the store invokes
// while holding the destination lock: workspace existence (unconditional,
// fail-closed when unconfigured) then workspace authorization. Global targets
// need no verification.
func (s *Service) destinationVerifier(ctx context.Context, req *CopySecretRequest) func(context.Context) error {
	if normalizeStoredScope(req.Scope) != ScopeWorkspace {
		return nil
	}
	// The callback ignores the store-supplied context and uses the outer
	// request ctx: the auth callbacks run inside the store's transaction,
	// whose context is not the one the request handlers set up.
	return func(_ context.Context) error {
		if s.workspaceExistence == nil {
			return ErrWorkspaceAccessDenied
		}
		if err := s.workspaceExistence(ctx, req.WorkspaceID); err != nil {
			return err
		}
		return s.authorizeScope(ctx, ScopeWorkspace, req.WorkspaceID)
	}
}
