package github

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

const workspacePATSecretName = "GitHub workspace PAT"

// ErrWorkspaceConnectionStale prevents a validated request from replacing a
// newer workspace connection that committed while validation was in flight.
var ErrWorkspaceConnectionStale = errors.New("GitHub workspace connection changed")

type SetWorkspaceConnectionRequest struct {
	Source ConnectionSource `json:"source"`
	Token  string           `json:"token,omitempty"`
	Host   string           `json:"host,omitempty"`
	Login  string           `json:"login,omitempty"`
}

// GetWorkspaceConnectionHealth exposes aggregate persisted connection state
// for system health without falling back to startup or ambient credentials.
func (s *Service) GetWorkspaceConnectionHealth(ctx context.Context) (WorkspaceConnectionHealth, error) {
	if s == nil || s.store == nil {
		return WorkspaceConnectionHealth{}, ErrGitHubNotConfigured
	}
	return s.store.GetWorkspaceConnectionHealth(ctx)
}

func (s *Service) SetWorkspaceConnection(
	ctx context.Context,
	workspaceID string,
	req SetWorkspaceConnectionRequest,
) (*WorkspaceConnection, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return nil, ErrGitHubWorkspaceRequired
	}
	if s.store == nil || s.connectionSecrets == nil {
		return nil, ErrGitHubNotConfigured
	}
	observed, err := s.store.GetWorkspaceConnection(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("load workspace connection: %w", err)
	}
	expectedGeneration := workspaceConnectionGeneration(observed)

	switch req.Source {
	case ConnectionSourcePAT:
		token := strings.TrimSpace(req.Token)
		if token == "" {
			return nil, fmt.Errorf("GitHub token is required")
		}
		login, err := s.validateWorkspaceToken(ctx, token)
		if err != nil {
			return nil, err
		}
		return s.commitWorkspacePAT(ctx, workspaceID, login, token, expectedGeneration)
	case ConnectionSourceGHCLI:
		return s.validateAndCommitWorkspaceGHCLI(
			ctx, workspaceID, req.Host, req.Login, expectedGeneration,
		)
	case ConnectionSourceLegacyShared:
		return nil, fmt.Errorf("legacy shared GitHub auth is migration-only")
	case ConnectionSourceGitHubAppInstallation:
		return nil, fmt.Errorf("GitHub App installation must use the verified installation flow")
	default:
		return nil, fmt.Errorf("unsupported GitHub connection source %q", req.Source)
	}
}

func (s *Service) newWorkspaceConnection(
	workspaceID string,
	source ConnectionSource,
	existing *WorkspaceConnection,
) *WorkspaceConnection {
	now := time.Now().UTC()
	connection := &WorkspaceConnection{
		WorkspaceID: workspaceID, Source: source, GitHubHost: defaultGitHubHost,
		Status: ConnectionStatusActive, CredentialGeneration: nextCredentialGeneration(existing),
		CreatedAt: now, UpdatedAt: now,
	}
	if existing != nil && !existing.CreatedAt.IsZero() {
		connection.CreatedAt = existing.CreatedAt
	}
	return connection
}

func nextCredentialGeneration(existing *WorkspaceConnection) int64 {
	if existing == nil || existing.CredentialGeneration < 1 {
		return 1
	}
	return existing.CredentialGeneration + 1
}

func workspaceConnectionGeneration(connection *WorkspaceConnection) int64 {
	if connection == nil {
		return 0
	}
	return connection.CredentialGeneration
}

func (s *Service) commitWorkspacePAT(
	ctx context.Context,
	workspaceID, login, token string,
	expectedGeneration int64,
) (*WorkspaceConnection, error) {
	lock := s.workspaceConnectionMutationLock(workspaceID)
	lock.Lock()
	defer lock.Unlock()
	existing, err := s.store.GetWorkspaceConnection(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("load workspace connection: %w", err)
	}
	if workspaceConnectionGeneration(existing) != expectedGeneration {
		return nil, ErrWorkspaceConnectionStale
	}
	connection := s.newWorkspaceConnection(workspaceID, ConnectionSourcePAT, existing)
	connection.Login = login
	secretKey := WorkspacePATSecretKey(connection.WorkspaceID)
	previous, hadPrevious, err := revealOptionalSecret(ctx, s.connectionSecrets, secretKey)
	if err != nil {
		return nil, fmt.Errorf("load previous workspace PAT: %w", err)
	}
	if err := s.connectionSecrets.Set(ctx, secretKey, workspacePATSecretName, token); err != nil {
		return nil, fmt.Errorf("store workspace PAT: %w", err)
	}
	if err := s.applyAutomationTransition(ctx, existing, connection, func() error {
		return s.store.UpsertWorkspaceConnection(ctx, connection)
	}); err != nil {
		s.compensatePATWrite(ctx, secretKey, previous, hadPrevious)
		return nil, fmt.Errorf("replace workspace GitHub connection: %w", err)
	}
	if existing != nil && existing.InstallationID != nil {
		s.InvalidateAppInstallationCredentials(existing.AppRegistrationID, *existing.InstallationID)
	}
	s.invalidateWorkspaceCredential(connection.WorkspaceID)
	return connection, nil
}

func (s *Service) validateAndCommitWorkspaceGHCLI(
	ctx context.Context,
	workspaceID, host, login string,
	expectedGeneration int64,
) (*WorkspaceConnection, error) {
	host = strings.TrimSpace(host)
	if host == "" {
		host = defaultGitHubHost
	}
	login = strings.TrimSpace(login)
	if login == "" {
		return nil, fmt.Errorf("GitHub login is required")
	}
	tokenResolver := ResolveGHAccountToken
	if s.resolver != nil && s.resolver.ghToken != nil {
		tokenResolver = s.resolver.ghToken
	}
	token, err := tokenResolver(ctx, host, login)
	if err != nil {
		return nil, err
	}
	validatedLogin, err := s.validateWorkspaceToken(ctx, token)
	if err != nil {
		return nil, err
	}
	if !strings.EqualFold(login, validatedLogin) {
		return nil, fmt.Errorf("selected gh login %q resolved as %q", login, validatedLogin)
	}
	return s.commitWorkspaceGHCLI(ctx, workspaceID, host, validatedLogin, expectedGeneration)
}

func (s *Service) commitWorkspaceGHCLI(
	ctx context.Context,
	workspaceID, host, login string,
	expectedGeneration int64,
) (*WorkspaceConnection, error) {
	lock := s.workspaceConnectionMutationLock(workspaceID)
	lock.Lock()
	defer lock.Unlock()
	existing, err := s.store.GetWorkspaceConnection(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("load workspace connection: %w", err)
	}
	if workspaceConnectionGeneration(existing) != expectedGeneration {
		return nil, ErrWorkspaceConnectionStale
	}
	connection := s.newWorkspaceConnection(workspaceID, ConnectionSourceGHCLI, existing)
	connection.GitHubHost = host
	connection.Login = login
	if err := s.applyAutomationTransition(ctx, existing, connection, func() error {
		return s.store.UpsertWorkspaceConnection(ctx, connection)
	}); err != nil {
		return nil, fmt.Errorf("replace workspace GitHub connection: %w", err)
	}
	if existing != nil && existing.Source == ConnectionSourcePAT {
		if err := deleteOptionalSecret(ctx, s.connectionSecrets, WorkspacePATSecretKey(workspaceID)); err != nil {
			return nil, errors.Join(err, restoreWorkspaceConnection(ctx, s.store, existing, workspaceID))
		}
	}
	if existing != nil && existing.InstallationID != nil {
		s.InvalidateAppInstallationCredentials(existing.AppRegistrationID, *existing.InstallationID)
	}
	s.invalidateWorkspaceCredential(connection.WorkspaceID)
	return connection, nil
}

func (s *Service) restorePATAfterFailedTransition(
	ctx context.Context,
	key, previous string,
	hadPrevious bool,
) error {
	if hadPrevious {
		return s.connectionSecrets.Set(ctx, key, workspacePATSecretName, previous)
	}
	return deleteOptionalSecret(ctx, s.connectionSecrets, key)
}

func (s *Service) revokePersonalForAutomationTransition(
	ctx context.Context,
	existing, replacement *WorkspaceConnection,
) error {
	if existing == nil || existing.Source != ConnectionSourceGitHubAppInstallation {
		return nil
	}
	if replacement != nil && replacement.Source == ConnectionSourceGitHubAppInstallation &&
		existing.AppRegistrationID == replacement.AppRegistrationID &&
		equalInstallationID(existing.InstallationID, replacement.InstallationID) {
		return nil
	}
	if s.personalConnections == nil {
		return errors.New("personal GitHub connection repository is not configured")
	}
	return s.personalConnections.RevokeWorkspacePersonalConnections(ctx, existing.WorkspaceID)
}

func (s *Service) applyAutomationTransition(
	ctx context.Context,
	existing, replacement *WorkspaceConnection,
	mutation func() error,
) error {
	if existing == nil || existing.Source != ConnectionSourceGitHubAppInstallation ||
		(replacement != nil && replacement.Source == ConnectionSourceGitHubAppInstallation &&
			existing.AppRegistrationID == replacement.AppRegistrationID &&
			equalInstallationID(existing.InstallationID, replacement.InstallationID)) {
		return mutation()
	}
	if s.personalConnections == nil {
		return errors.New("personal GitHub connection repository is not configured")
	}
	return s.personalConnections.TransitionWorkspacePersonalConnections(ctx, existing.WorkspaceID, mutation)
}

func equalInstallationID(left, right *int64) bool {
	return left != nil && right != nil && *left == *right
}

func (s *Service) validateWorkspaceToken(ctx context.Context, token string) (string, error) {
	factory := s.tokenClientFactory
	if factory == nil {
		factory = func(value string) Client { return NewPATClient(value) }
	}
	login, err := factory(token).GetAuthenticatedUser(ctx)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrInvalidToken, err)
	}
	login = strings.TrimSpace(login)
	if login == "" {
		return "", fmt.Errorf("%w: GitHub returned an empty login", ErrInvalidToken)
	}
	return login, nil
}

func revealOptionalSecret(ctx context.Context, secrets ConnectionSecretStore, key string) (string, bool, error) {
	exists, err := secrets.Exists(ctx, key)
	if err != nil || !exists {
		return "", false, err
	}
	value, err := secrets.Reveal(ctx, key)
	return value, true, err
}

func (s *Service) compensatePATWrite(ctx context.Context, key, previous string, hadPrevious bool) {
	if hadPrevious {
		_ = s.connectionSecrets.Set(ctx, key, workspacePATSecretName, previous)
		return
	}
	_ = s.connectionSecrets.Delete(ctx, key)
}

func (s *Service) DeleteWorkspaceConnection(ctx context.Context, workspaceID string) error {
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return ErrGitHubWorkspaceRequired
	}
	if s.store == nil {
		return ErrGitHubNotConfigured
	}
	lock := s.workspaceConnectionMutationLock(workspaceID)
	lock.Lock()
	defer lock.Unlock()
	connection, err := s.store.GetWorkspaceConnection(ctx, workspaceID)
	if err != nil {
		return err
	}
	if connection != nil && connection.Source == ConnectionSourcePAT && s.connectionSecrets != nil {
		err = s.deleteWorkspacePATConnection(ctx, workspaceID)
	} else {
		err = s.deleteWorkspaceConnectionMetadata(ctx, workspaceID, connection)
	}
	if err != nil {
		return err
	}
	if connection != nil && connection.InstallationID != nil {
		s.InvalidateAppInstallationCredentials(connection.AppRegistrationID, *connection.InstallationID)
	}
	s.invalidateWorkspaceCredential(workspaceID)
	return nil
}

func (s *Service) deleteWorkspacePATConnection(ctx context.Context, workspaceID string) error {
	secretKey := WorkspacePATSecretKey(workspaceID)
	previous, hadPrevious, err := revealOptionalSecret(ctx, s.connectionSecrets, secretKey)
	if err != nil {
		return fmt.Errorf("load workspace PAT: %w", err)
	}
	if err := deleteOptionalSecret(ctx, s.connectionSecrets, secretKey); err != nil {
		return fmt.Errorf("delete workspace PAT: %w", err)
	}
	if err := s.store.DeleteWorkspaceConnection(ctx, workspaceID); err != nil {
		return errors.Join(err, restoreOptionalSecret(
			ctx, s.connectionSecrets, secretKey, workspacePATSecretName, previous, hadPrevious,
		))
	}
	return nil
}

func (s *Service) deleteWorkspaceConnectionMetadata(
	ctx context.Context, workspaceID string, connection *WorkspaceConnection,
) error {
	return s.applyAutomationTransition(ctx, connection, nil, func() error {
		return s.store.DeleteWorkspaceConnection(ctx, workspaceID)
	})
}

func (s *Service) workspaceConnectionMutationLock(workspaceID string) *sync.Mutex {
	var hash uint32
	for index := range len(workspaceID) {
		hash = hash*33 + uint32(workspaceID[index])
	}
	return &s.connectionMutationLocks[hash%uint32(len(s.connectionMutationLocks))]
}

func deleteOptionalSecret(ctx context.Context, secrets ConnectionSecretStore, key string) error {
	if secrets == nil {
		return nil
	}
	exists, err := secrets.Exists(ctx, key)
	if err != nil || !exists {
		return err
	}
	return secrets.Delete(ctx, key)
}

func (s *Service) invalidateWorkspaceCredential(workspaceID string) {
	if s.resolver != nil {
		s.resolver.InvalidateWorkspace(workspaceID)
	}
	// Existing caches are still global during the compatibility phase. Clear
	// them on replacement until Task 05 prefixes every key by principal.
	s.clearAuthCaches()
}

func (s *Service) clearAuthCaches() {
	for _, cache := range []*ttlCache{
		s.searchCache, s.prStatusCache, s.prFeedbackCache, s.mergeMethodsCache,
		s.accessibleReposCache, s.repoErrorCache,
	} {
		if cache != nil {
			cache.clear()
		}
	}
	if s.protectionCache != nil {
		s.protectionCache.clear()
	}
}
