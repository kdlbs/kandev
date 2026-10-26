package github

import (
	"context"
	"errors"
	"fmt"
	"github.com/kandev/kandev/internal/common/githubhost"
	"go.uber.org/zap"
	"strings"
	"sync"
	"time"

	"github.com/kandev/kandev/internal/auth/authn"
)

const workspacePATSecretName = "GitHub workspace PAT"

// ErrWorkspaceConnectionStale prevents a validated request from replacing a
// newer workspace connection that committed while validation was in flight.
var ErrWorkspaceConnectionStale = errors.New("GitHub workspace connection changed")

// ErrGHCLIOperatorRequired prevents authenticated members from binding a
// deployment host gh CLI account to a workspace.
var ErrGHCLIOperatorRequired = errors.New("GitHub CLI operator access is required")

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

// WorkspaceConnectionFingerprint returns an opaque, non-secret string that
// changes whenever the workspace's automation connection is replaced,
// reconnected, or has its managed credential rotated or removed. Callers
// (Workflow Sync and the PR poller's auth circuit breakers) use this to detect "the credential
// changed" and reset an open backoff circuit promptly instead of waiting out
// the schedule. Returns "" when no connection exists — an empty fingerprint
// must never be treated as "changed" by callers, since there was nothing to
// compare against.
func (s *Service) WorkspaceConnectionFingerprint(ctx context.Context, workspaceID string) (string, error) {
	if s == nil || s.store == nil {
		return "", nil
	}
	connection, err := s.store.GetWorkspaceConnection(ctx, workspaceID)
	if err != nil {
		return "", err
	}
	if connection == nil {
		return "", nil
	}
	fingerprint := fmt.Sprintf("%s:%d", connection.Status, connection.CredentialGeneration)
	if connection.Source == ConnectionSourceGitHubAppInstallation {
		runtime := s.AppRegistrationRuntimeSnapshot(connection.AppRegistrationID)
		fingerprint += fmt.Sprintf(":app:%s:%d:%t", connection.AppRegistrationID, runtime.Generation, runtime.Ready)
	}
	if connection.Source == ConnectionSourceLegacyShared {
		revision, err := s.legacyTokenRevision(ctx)
		if err != nil {
			return "", err
		}
		fingerprint += ":legacy:" + revision
	}
	return fingerprint, nil
}

// legacyTokenRevision uses metadata committed with the encrypted secret. It
// survives restart without reading or persisting the credential value.
func (s *Service) legacyTokenRevision(ctx context.Context) (string, error) {
	s.legacyCredentialMu.Lock()
	defer s.legacyCredentialMu.Unlock()
	revision, err := s.readLegacyTokenRevision(ctx)
	if err != nil {
		return "", err
	}
	if revision != s.legacyCredentialRevision {
		s.mu.Lock()
		s.clearLegacyCredentialCachesLocked()
		s.mu.Unlock()
		s.legacyCredentialRevision = revision
	}
	return revision, nil
}

func (s *Service) readLegacyTokenRevision(ctx context.Context) (string, error) {
	if s.secrets == nil {
		return "absent", nil
	}
	items, err := s.secrets.List(ctx)
	if err != nil {
		return "", fmt.Errorf("read legacy GitHub credential revision: %w", err)
	}
	for _, item := range items {
		if item.HasValue && (item.Name == "GITHUB_TOKEN" || item.Name == "github_token") {
			return fmt.Sprintf("%d:%s:%s", len(item.ID), item.ID, item.UpdatedAt.UTC().Format(time.RFC3339Nano)), nil
		}
	}
	return "absent", nil
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
	if err := s.authorizeWorkspaceAccess(ctx, workspaceID); err != nil {
		return nil, err
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
		host, err := connectionHost(req.Host)
		if err != nil {
			return nil, err
		}
		login, err := s.validateWorkspaceTokenForHost(ctx, token, host)
		if err != nil {
			return nil, err
		}
		return s.commitWorkspacePAT(ctx, workspaceID, login, token, expectedGeneration, host.String())
	case ConnectionSourceGHCLI:
		if err := requireGHCLIOperator(ctx); err != nil {
			return nil, err
		}
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

func requireGHCLIOperator(ctx context.Context) error {
	identity, ok := authn.IdentityFromContext(ctx)
	if !ok {
		// Identity-free callers are internal lifecycle operations. HTTP requests
		// always carry either a real identity or the auth-disabled synthetic admin.
		return nil
	}
	if identity.IsAdmin() {
		return nil
	}
	return ErrGHCLIOperatorRequired
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
	hosts ...string,
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
	if len(hosts) > 0 {
		connection.GitHubHost = hosts[0]
	}
	secretKey := WorkspacePATSecretKeyForHost(connection.WorkspaceID, connection.GitHubHost)
	previous, hadPrevious, err := revealOptionalSecret(ctx, s.connectionSecrets, secretKey)
	if err != nil {
		return nil, fmt.Errorf("load previous workspace PAT: %w", err)
	}
	if err := s.connectionSecrets.Set(ctx, secretKey, workspacePATSecretName, token); err != nil {
		return nil, fmt.Errorf("store workspace PAT: %w", err)
	}
	if err := s.applyAutomationTransition(ctx, existing, connection, func() error {
		return s.store.ReplaceWorkspaceHostConnection(ctx, existing, connection)
	}); err != nil {
		s.compensatePATWrite(ctx, secretKey, previous, hadPrevious)
		return nil, fmt.Errorf("replace workspace GitHub connection: %w", err)
	}
	if existing != nil && existing.InstallationID != nil {
		s.InvalidateAppInstallationCredentials(existing.AppRegistrationID, *existing.InstallationID)
	}
	if existing != nil && existing.Source == ConnectionSourcePAT && WorkspacePATSecretKeyForHost(existing.WorkspaceID, existing.GitHubHost) != WorkspacePATSecretKeyForHost(connection.WorkspaceID, connection.GitHubHost) {
		if err := deleteOptionalSecret(ctx, s.connectionSecrets, WorkspacePATSecretKeyForHost(existing.WorkspaceID, existing.GitHubHost)); err != nil {
			s.warnObsoletePATCleanup(existing)
		}
	}
	s.invalidateWorkspaceCredential(connection.WorkspaceID)
	return connection, nil
}

func (s *Service) validateAndCommitWorkspaceGHCLI(
	ctx context.Context,
	workspaceID, host, login string,
	expectedGeneration int64,
) (*WorkspaceConnection, error) {
	parsedHost, err := connectionHost(host)
	if err != nil {
		return nil, err
	}
	host = parsedHost.String()
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
	validatedLogin, err := s.validateWorkspaceTokenForHost(ctx, token, parsedHost)
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
		return s.store.ReplaceWorkspaceHostConnection(ctx, existing, connection)
	}); err != nil {
		return nil, fmt.Errorf("replace workspace GitHub connection: %w", err)
	}
	if existing != nil && existing.Source == ConnectionSourcePAT {
		if err := deleteOptionalSecret(ctx, s.connectionSecrets, WorkspacePATSecretKeyForHost(workspaceID, existing.GitHubHost)); err != nil {
			if defaultAuthFlowHost(existing.GitHubHost) == defaultAuthFlowHost(connection.GitHubHost) {
				return nil, errors.Join(err, restoreWorkspaceConnection(ctx, s.store, existing, workspaceID))
			}
			s.warnObsoletePATCleanup(existing)
		}
	}
	if existing != nil && existing.InstallationID != nil {
		s.InvalidateAppInstallationCredentials(existing.AppRegistrationID, *existing.InstallationID)
	}
	s.invalidateWorkspaceCredential(connection.WorkspaceID)
	return connection, nil
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
	if err := s.authorizeWorkspaceAccess(ctx, workspaceID); err != nil {
		return err
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
		err = s.deleteWorkspacePATConnection(ctx, workspaceID, connection.GitHubHost)
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

func (s *Service) deleteWorkspacePATConnection(ctx context.Context, workspaceID, hostname string) error {
	secretKey := WorkspacePATSecretKeyForHost(workspaceID, hostname)
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
	if s.prDiscoveryHealth != nil {
		s.prDiscoveryHealth.clearWorkspace(workspaceID)
		s.invalidateAllPRDiscoveryAttempts(workspaceID)
	}
	// Existing caches are still global during the compatibility phase. Clear
	// them on replacement until Task 05 prefixes every key by principal.
	s.clearAuthCaches()
}

func (s *Service) clearAuthCaches() {
	for _, cache := range []*ttlCache{
		s.searchCache, s.prStatusCache, s.prFeedbackCache, s.mergeMethodsCache,
		s.accessibleReposCache, s.repoErrorCache, s.workflowRunsCache, s.workflowJobsCache,
	} {
		if cache != nil {
			cache.clear()
		}
	}
	if s.protectionCache != nil {
		s.protectionCache.clear()
	}
}

// CopyWorkspaceConnectionToWorkspace duplicates the source workspace's GitHub
// connection onto the destination workspace, including the PAT secret for
// `pat` sources. No-op when the source workspace has no connection or the two
// workspaces are the same. Used by the improve-kandev bootstrap when it
// creates the dedicated workspace so improve tasks inherit the user's GitHub
// access without re-authenticating.
func (s *Service) CopyWorkspaceConnectionToWorkspace(ctx context.Context, srcWorkspaceID, dstWorkspaceID string) error {
	if srcWorkspaceID == "" || dstWorkspaceID == "" || srcWorkspaceID == dstWorkspaceID {
		return nil
	}
	src, err := s.store.GetWorkspaceConnection(ctx, srcWorkspaceID)
	if err != nil {
		return fmt.Errorf("load source workspace connection: %w", err)
	}
	if src == nil {
		return nil
	}
	connection := &WorkspaceConnection{
		WorkspaceID:              dstWorkspaceID,
		Source:                   src.Source,
		GitHubHost:               src.GitHubHost,
		Login:                    src.Login,
		InstallationID:           src.InstallationID,
		InstallationAccountLogin: src.InstallationAccountLogin,
		InstallationAccountType:  src.InstallationAccountType,
		AppRegistrationID:        src.AppRegistrationID,
		Status:                   src.Status,
		CredentialGeneration:     nextCredentialGeneration(nil),
		CreatedAt:                time.Now().UTC(),
		UpdatedAt:                time.Now().UTC(),
	}
	if err := s.store.UpsertWorkspaceConnection(ctx, connection); err != nil {
		return fmt.Errorf("store destination workspace connection: %w", err)
	}
	if src.Source == ConnectionSourcePAT {
		token, err := s.connectionSecrets.Reveal(ctx, WorkspacePATSecretKeyForHost(srcWorkspaceID, src.GitHubHost))
		if err != nil {
			return fmt.Errorf("reveal source workspace PAT: %w", err)
		}
		if err := s.connectionSecrets.Set(ctx, WorkspacePATSecretKeyForHost(dstWorkspaceID, src.GitHubHost), workspacePATSecretName, token); err != nil {
			return fmt.Errorf("store destination workspace PAT: %w", err)
		}
	}
	s.invalidateWorkspaceCredential(dstWorkspaceID)
	return nil
}

func connectionHost(value string) (githubhost.Host, error) {
	if value == "" {
		return githubhost.Public(), nil
	}
	return githubhost.Parse(value)
}

func (s *Service) validateWorkspaceTokenForHost(ctx context.Context, token string, host githubhost.Host) (string, error) {
	if host.IsPublic() && s.hostTokenClientFactory == nil {
		return s.validateWorkspaceToken(ctx, token)
	}
	factory := s.hostTokenClientFactory
	if factory == nil {
		factory = func(value string, selected githubhost.Host) Client { return NewPATClientForHost(value, selected) }
	}
	login, err := factory(token, host).GetAuthenticatedUser(ctx)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrInvalidToken, err)
	}
	if strings.TrimSpace(login) == "" {
		return "", ErrInvalidToken
	}
	return strings.TrimSpace(login), nil
}

func (s *Service) warnObsoletePATCleanup(connection *WorkspaceConnection) {
	if s.logger != nil {
		s.logger.Warn("failed to remove obsolete GitHub PAT", zap.String("workspace_id", connection.WorkspaceID), zap.String("github_host", defaultAuthFlowHost(connection.GitHubHost)))
	}
}
