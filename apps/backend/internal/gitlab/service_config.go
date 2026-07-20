package gitlab

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"go.uber.org/zap"
)

// WorkspaceClientFactory constructs a client from one workspace's resolved
// config and credential. Tests replace it to avoid external GitLab calls.
type WorkspaceClientFactory func(context.Context, *GitLabConfig, string) (Client, error)

var ErrWorkspaceRequired = errors.New("workspace_id required")
var ErrInvalidConfig = errors.New("gitlab: invalid configuration")
var ErrNotConfigured = errors.New("gitlab: workspace not configured")
var ErrWorkspaceHostMismatch = errors.New("gitlab: workspace host mismatch")

const (
	envGitLabHost       = "GITLAB_HOST"
	envKandevGitLabHost = "KANDEV_GITLAB_HOST"
)

// ResolveGitLabExecutionCredentials returns credentials for exactly one
// configured workspace. It is consumed by the orchestrator executor and never
// falls back to another workspace's connection.
func (s *Service) ResolveGitLabExecutionCredentials(ctx context.Context, workspaceID string) (string, string, error) {
	cfg, err := s.GetConfigForWorkspace(ctx, workspaceID)
	if err != nil {
		return "", "", err
	}
	if cfg == nil {
		return "", "", ErrNotConfigured
	}
	if err := s.validateEnvironmentTokenHost(cfg.AuthMethod, cfg.Host); err != nil {
		return "", "", err
	}
	if cfg.AuthMethod == AuthMethodGLab {
		s.mu.RLock()
		resolve := s.glabTokenFn
		s.mu.RUnlock()
		if resolve == nil {
			return "", "", fmt.Errorf("%w: glab credential resolver unavailable", ErrInvalidConfig)
		}
		token, resolveErr := resolve(ctx, stripScheme(cfg.Host))
		if resolveErr != nil || strings.TrimSpace(token) == "" {
			if resolveErr == nil {
				resolveErr = errors.New("empty token")
			}
			return "", "", fmt.Errorf("resolve glab token for %s: %w", cfg.Host, resolveErr)
		}
		return cfg.Host, strings.TrimSpace(token), nil
	}
	token, err := s.resolveWorkspaceToken(ctx, workspaceID, cfg.AuthMethod, "")
	if err != nil {
		return "", "", err
	}
	return cfg.Host, token, nil
}

// GetConfigForWorkspace returns public connection metadata for one workspace.
func (s *Service) GetConfigForWorkspace(ctx context.Context, workspaceID string) (*GitLabConfig, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return nil, ErrWorkspaceRequired
	}
	s.mu.RLock()
	store := s.store
	secrets := s.workspaceSecrets
	s.mu.RUnlock()
	if store == nil {
		return nil, errors.New("gitlab store not configured")
	}
	cfg, err := store.GetConfigForWorkspace(ctx, workspaceID)
	if err != nil || cfg == nil || secrets == nil {
		return cfg, err
	}
	cfg.HasSecret, err = secrets.Exists(ctx, SecretKeyForWorkspace(workspaceID))
	return cfg, err
}

// SetConfigForWorkspace validates credentials before replacing the saved
// connection, so a failed probe leaves the previous row and secret untouched.
func (s *Service) SetConfigForWorkspace(ctx context.Context, workspaceID string, req *SetConfigRequest) (*GitLabConfig, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return nil, ErrWorkspaceRequired
	}
	cfg, token, err := s.resolveConfigRequest(ctx, workspaceID, req)
	if err != nil {
		return nil, err
	}
	client, err := s.buildWorkspaceClient(ctx, cfg, token)
	if err != nil {
		return nil, err
	}
	username, err := client.GetAuthenticatedUser(ctx)
	if err != nil || username == "" {
		if err == nil {
			err = errors.New("GitLab returned an empty username")
		}
		return nil, fmt.Errorf("test GitLab connection: %w", err)
	}
	cfg.Username = username
	if err := s.persistWorkspaceConfig(ctx, workspaceID, cfg, token); err != nil {
		return nil, err
	}
	return s.GetConfigForWorkspace(ctx, workspaceID)
}

// TestConfigForWorkspace probes supplied or stored settings without persisting.
func (s *Service) TestConfigForWorkspace(ctx context.Context, workspaceID string, req *SetConfigRequest) *TestConnectionResult {
	cfg, token, err := s.resolveConfigRequest(ctx, strings.TrimSpace(workspaceID), req)
	if err != nil {
		return &TestConnectionResult{Error: publicConfigError(err)}
	}
	client, err := s.buildWorkspaceClient(ctx, cfg, token)
	if err != nil {
		return &TestConnectionResult{Error: "GitLab connection test failed"}
	}
	username, err := client.GetAuthenticatedUser(ctx)
	if err != nil {
		return &TestConnectionResult{Error: "GitLab connection test failed"}
	}
	return &TestConnectionResult{OK: username != "", Username: username}
}

func publicConfigError(err error) string {
	switch {
	case errors.Is(err, ErrWorkspaceRequired):
		return "workspace_id required"
	case errors.Is(err, ErrInvalidConfig):
		return err.Error()
	default:
		return "GitLab connection test failed"
	}
}

func (s *Service) resolveConfigRequest(ctx context.Context, workspaceID string, req *SetConfigRequest) (*GitLabConfig, string, error) {
	if workspaceID == "" {
		return nil, "", ErrWorkspaceRequired
	}
	if req == nil {
		return nil, "", fmt.Errorf("%w: request required", ErrInvalidConfig)
	}
	host := strings.TrimSpace(req.Host)
	authMethod := strings.TrimSpace(req.AuthMethod)
	host, authMethod, err := s.configRequestDefaults(ctx, workspaceID, host, authMethod)
	if err != nil {
		return nil, "", err
	}
	normalizedHost, err := normalizeHostOrigin(host)
	if err != nil {
		return nil, "", fmt.Errorf("%w: %s", ErrInvalidConfig, err)
	}
	if !validWorkspaceAuthMethod(authMethod) {
		return nil, "", fmt.Errorf("%w: unsupported auth_method %q", ErrInvalidConfig, authMethod)
	}
	if err := s.validateEnvironmentTokenHost(authMethod, normalizedHost); err != nil {
		return nil, "", err
	}
	token, err := s.resolveWorkspaceToken(ctx, workspaceID, authMethod, strings.TrimSpace(req.Token))
	if err != nil {
		return nil, "", err
	}
	return &GitLabConfig{WorkspaceID: workspaceID, Host: normalizedHost, AuthMethod: authMethod}, token, nil
}

func (s *Service) configRequestDefaults(ctx context.Context, workspaceID, host, authMethod string) (string, string, error) {
	if host != "" && authMethod != "" {
		return host, authMethod, nil
	}
	stored, err := s.GetConfigForWorkspace(ctx, workspaceID)
	if err != nil {
		return "", "", err
	}
	if stored == nil {
		return host, authMethod, nil
	}
	if host == "" {
		host = stored.Host
	}
	if authMethod == "" {
		authMethod = stored.AuthMethod
	}
	return host, authMethod, nil
}

func normalizeHostOrigin(host string) (string, error) {
	parsed, err := validateHost(strings.TrimSpace(host))
	if err != nil {
		return "", err
	}
	if parsed.Path != "" {
		return "", errors.New("host must be an HTTP(S) origin without a path")
	}
	return parsed.String(), nil
}

func validWorkspaceAuthMethod(method string) bool {
	switch method {
	case AuthMethodPAT, AuthMethodGLab, AuthMethodEnvironment:
		return true
	default:
		return false
	}
}

func trustedEnvironmentTokenHost(startupHost string) string {
	host := strings.TrimSpace(os.Getenv(envKandevGitLabHost))
	if host == "" {
		host = strings.TrimSpace(os.Getenv(envGitLabHost))
	}
	if host == "" {
		host = startupHost
	}
	normalized, err := normalizeHostOrigin(host)
	if err != nil {
		return ""
	}
	return normalized
}

func (s *Service) validateEnvironmentTokenHost(authMethod, host string) error {
	if authMethod != AuthMethodEnvironment {
		return nil
	}
	s.mu.RLock()
	trustedHost := s.environmentTokenHost
	s.mu.RUnlock()
	if trustedHost == "" || !strings.EqualFold(host, trustedHost) {
		return fmt.Errorf("%w: environment credential host must match the trusted startup GitLab origin", ErrInvalidConfig)
	}
	return nil
}

func (s *Service) resolveWorkspaceToken(ctx context.Context, workspaceID, authMethod, supplied string) (string, error) {
	if authMethod == AuthMethodEnvironment {
		token := strings.TrimSpace(os.Getenv(secretNameToken))
		if token == "" {
			return "", fmt.Errorf("%w: GITLAB_TOKEN is empty", ErrInvalidConfig)
		}
		return token, nil
	}
	if authMethod != AuthMethodPAT {
		return "", nil
	}
	if supplied != "" {
		return supplied, nil
	}
	s.mu.RLock()
	secrets := s.workspaceSecrets
	s.mu.RUnlock()
	if secrets == nil {
		return "", fmt.Errorf("%w: token required", ErrInvalidConfig)
	}
	token, err := secrets.Reveal(ctx, SecretKeyForWorkspace(workspaceID))
	if err != nil || strings.TrimSpace(token) == "" {
		return "", fmt.Errorf("%w: token required", ErrInvalidConfig)
	}
	return strings.TrimSpace(token), nil
}

func (s *Service) persistWorkspaceConfig(ctx context.Context, workspaceID string, cfg *GitLabConfig, token string) error {
	s.configMutationMu.Lock()
	defer s.configMutationMu.Unlock()

	s.mu.RLock()
	store := s.store
	secrets := s.workspaceSecrets
	s.mu.RUnlock()
	return s.persistWorkspaceConfigLocked(ctx, store, secrets, workspaceID, cfg, token)
}

func (s *Service) persistWorkspaceConfigLocked(ctx context.Context, store *Store, secrets WorkspaceSecretStore, workspaceID string, cfg *GitLabConfig, token string) error {
	if store == nil {
		return errors.New("gitlab store not configured")
	}
	snapshot, err := snapshotWorkspaceConnection(ctx, store, secrets, workspaceID)
	if err != nil {
		return fmt.Errorf("snapshot GitLab config: %w", err)
	}
	if err := persistWorkspaceSecret(ctx, secrets, workspaceID, cfg.AuthMethod, token, snapshot.secret); err != nil {
		return rollbackWorkspaceConnection(ctx, store, secrets, workspaceID, snapshot, fmt.Errorf("store GitLab token: %w", err))
	}
	if err := store.SaveConfigForWorkspace(ctx, workspaceID, cfg); err != nil {
		return rollbackWorkspaceConnection(ctx, store, secrets, workspaceID, snapshot, fmt.Errorf("store GitLab config: %w", err))
	}
	s.invalidateWorkspaceClient(workspaceID)
	return nil
}

type workspaceSecretSnapshot struct {
	exists bool
	value  string
}

type workspaceConnectionSnapshot struct {
	config *GitLabConfig
	secret workspaceSecretSnapshot
}

func snapshotWorkspaceConnection(ctx context.Context, store *Store, secrets WorkspaceSecretStore, workspaceID string) (workspaceConnectionSnapshot, error) {
	cfg, err := store.GetConfigForWorkspace(ctx, workspaceID)
	if err != nil {
		return workspaceConnectionSnapshot{}, err
	}
	secret, err := snapshotWorkspaceSecret(ctx, secrets, SecretKeyForWorkspace(workspaceID))
	if err != nil {
		return workspaceConnectionSnapshot{}, err
	}
	return workspaceConnectionSnapshot{config: cfg, secret: secret}, nil
}

func snapshotWorkspaceSecret(ctx context.Context, secrets WorkspaceSecretStore, key string) (workspaceSecretSnapshot, error) {
	if secrets == nil {
		return workspaceSecretSnapshot{}, nil
	}
	exists, err := secrets.Exists(ctx, key)
	if err != nil || !exists {
		return workspaceSecretSnapshot{}, err
	}
	value, err := secrets.Reveal(ctx, key)
	if err != nil {
		return workspaceSecretSnapshot{}, err
	}
	return workspaceSecretSnapshot{exists: true, value: value}, nil
}

func persistWorkspaceSecret(ctx context.Context, secrets WorkspaceSecretStore, workspaceID, authMethod, token string, previous workspaceSecretSnapshot) error {
	if authMethod == AuthMethodPAT {
		if secrets == nil {
			return errors.New("gitlab secret store not configured")
		}
		return secrets.Set(ctx, SecretKeyForWorkspace(workspaceID), "GitLab token", token)
	}
	if secrets != nil && previous.exists {
		return secrets.Delete(ctx, SecretKeyForWorkspace(workspaceID))
	}
	return nil
}

func rollbackWorkspaceConnection(ctx context.Context, store *Store, secrets WorkspaceSecretStore, workspaceID string, snapshot workspaceConnectionSnapshot, cause error) error {
	rollbackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()

	var rollbackErrs []error
	current, err := store.GetConfigForWorkspace(rollbackCtx, workspaceID)
	if err != nil {
		rollbackErrs = append(rollbackErrs, fmt.Errorf("read GitLab config during rollback: %w", err))
	} else if !sameGitLabConfig(current, snapshot.config) {
		if err := store.RestoreConfigForWorkspace(rollbackCtx, workspaceID, snapshot.config); err != nil {
			rollbackErrs = append(rollbackErrs, fmt.Errorf("restore GitLab config: %w", err))
		}
	}
	if err := restoreWorkspaceSecret(rollbackCtx, secrets, SecretKeyForWorkspace(workspaceID), snapshot.secret); err != nil {
		rollbackErrs = append(rollbackErrs, fmt.Errorf("restore GitLab token: %w", err))
	}
	if len(rollbackErrs) == 0 {
		return cause
	}
	return errors.Join(append([]error{cause}, rollbackErrs...)...)
}

func restoreWorkspaceSecret(ctx context.Context, secrets WorkspaceSecretStore, key string, snapshot workspaceSecretSnapshot) error {
	if secrets == nil {
		return nil
	}
	if snapshot.exists {
		return secrets.Set(ctx, key, "GitLab token", snapshot.value)
	}
	exists, err := secrets.Exists(ctx, key)
	if err != nil || !exists {
		return err
	}
	return secrets.Delete(ctx, key)
}

func sameGitLabConfig(left, right *GitLabConfig) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return left.WorkspaceID == right.WorkspaceID &&
		left.Host == right.Host &&
		left.AuthMethod == right.AuthMethod &&
		left.Username == right.Username &&
		left.LastOK == right.LastOK &&
		left.LastError == right.LastError &&
		left.Revision == right.Revision &&
		sameTimePointer(left.LastCheckedAt, right.LastCheckedAt) &&
		left.CreatedAt.Equal(right.CreatedAt) &&
		left.UpdatedAt.Equal(right.UpdatedAt)
}

func sameTimePointer(left, right *time.Time) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return left.Equal(*right)
}

func (s *Service) buildWorkspaceClient(ctx context.Context, cfg *GitLabConfig, token string) (Client, error) {
	s.mu.RLock()
	factory := s.workspaceClientFn
	log := s.logger
	s.mu.RUnlock()
	if factory != nil {
		return factory(ctx, cfg, token)
	}
	if os.Getenv("KANDEV_MOCK_GITLAB") == "true" {
		return NewMockClient(cfg.Host), nil
	}
	switch cfg.AuthMethod {
	case AuthMethodPAT, AuthMethodEnvironment:
		return NewPATClient(cfg.Host, token), nil
	case AuthMethodGLab:
		client, err := NewGLabClient(ctx, cfg.Host)
		if err != nil && log != nil {
			log.Debug("glab CLI unavailable for workspace connection")
		}
		return client, err
	default:
		return nil, fmt.Errorf("%w: unsupported auth_method %q", ErrInvalidConfig, cfg.AuthMethod)
	}
}

// ClientForWorkspace resolves and caches only the requested workspace client.
func (s *Service) ClientForWorkspace(ctx context.Context, workspaceID string) (Client, error) {
	return s.ClientForWorkspaceHost(ctx, workspaceID, "")
}

func (s *Service) ClientForWorkspaceHost(ctx context.Context, workspaceID, expectedHost string) (Client, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return nil, ErrWorkspaceRequired
	}
	expectedHost = strings.TrimSpace(expectedHost)
	if expectedHost != "" {
		normalized, err := normalizeHostOrigin(expectedHost)
		if err != nil {
			return nil, ErrWorkspaceHostMismatch
		}
		expectedHost = normalized
	}
	for {
		client, retry, err := s.resolveWorkspaceClientRevision(ctx, workspaceID, expectedHost)
		if err != nil || !retry {
			return client, err
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
	}
}

// RunWithWorkspaceClient resolves the configured host, credential, and client
// from one mutation-locked snapshot. Client construction, and the upstream
// action itself, run without holding the process-wide config mutation lock.
func (s *Service) RunWithWorkspaceClient(
	ctx context.Context,
	workspaceID, expectedHost string,
	action func(Client) error,
) error {
	client, err := s.ClientForWorkspaceHost(ctx, workspaceID, expectedHost)
	if err != nil {
		return err
	}
	return action(client)
}

func (s *Service) resolveWorkspaceClientRevision(
	ctx context.Context, workspaceID, expectedHost string,
) (Client, bool, error) {
	if expectedHost == "" {
		s.mu.RLock()
		cached := s.workspaceClients[workspaceID]
		revision := s.workspaceClientRevs[workspaceID]
		s.mu.RUnlock()
		if cached != nil && revision == 0 {
			return cached, false, nil
		}
	}
	snapshot, err := s.snapshotWorkspaceClientRevision(ctx, workspaceID)
	if err != nil {
		return nil, false, err
	}
	if snapshot.config == nil {
		return nil, false, ErrNotConfigured
	}
	if expectedHost != "" && !strings.EqualFold(snapshot.config.Host, expectedHost) {
		return nil, false, ErrWorkspaceHostMismatch
	}
	s.mu.RLock()
	cached := s.workspaceClients[workspaceID]
	cachedRevision := s.workspaceClientRevs[workspaceID]
	s.mu.RUnlock()
	if cached != nil && cachedRevision == snapshot.config.Revision &&
		strings.EqualFold(cached.Host(), snapshot.config.Host) {
		return cached, false, nil
	}
	token, err := s.tokenForWorkspaceSnapshot(snapshot)
	if err != nil {
		return nil, false, err
	}
	client, err := s.buildWorkspaceClient(ctx, snapshot.config, token)
	if err != nil {
		return nil, false, err
	}
	published, current, err := s.publishWorkspaceClientRevision(ctx, workspaceID, snapshot.config, client)
	if err != nil {
		return nil, false, err
	}
	return published, !current, nil
}

func (s *Service) snapshotWorkspaceClientRevision(
	ctx context.Context, workspaceID string,
) (workspaceConnectionSnapshot, error) {
	s.configMutationMu.Lock()
	defer s.configMutationMu.Unlock()
	s.mu.RLock()
	store := s.store
	secrets := s.workspaceSecrets
	s.mu.RUnlock()
	if store == nil {
		return workspaceConnectionSnapshot{}, errors.New("gitlab store not configured")
	}
	return snapshotWorkspaceConnection(ctx, store, secrets, workspaceID)
}

func (s *Service) publishWorkspaceClientRevision(
	ctx context.Context,
	workspaceID string,
	snapshot *GitLabConfig,
	client Client,
) (Client, bool, error) {
	s.configMutationMu.Lock()
	defer s.configMutationMu.Unlock()
	s.mu.RLock()
	store := s.store
	s.mu.RUnlock()
	if store == nil {
		return nil, false, errors.New("gitlab store not configured")
	}
	current, err := store.GetConfigForWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, false, err
	}
	if current == nil || current.Revision != snapshot.Revision ||
		!strings.EqualFold(current.Host, snapshot.Host) {
		return nil, false, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.workspaceClients == nil {
		s.workspaceClients = make(map[string]Client)
	}
	if s.workspaceClientRevs == nil {
		s.workspaceClientRevs = make(map[string]int64)
	}
	if existing := s.workspaceClients[workspaceID]; existing != nil &&
		s.workspaceClientRevs[workspaceID] == snapshot.Revision {
		return existing, true, nil
	}
	s.workspaceClients[workspaceID] = client
	s.workspaceClientRevs[workspaceID] = snapshot.Revision
	return client, true, nil
}

// GetStatusForWorkspace probes and reports only the selected connection.
func (s *Service) GetStatusForWorkspace(ctx context.Context, workspaceID string) (*Status, error) {
	cfg, err := s.GetConfigForWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		return &Status{Host: DefaultHost, AuthMethod: AuthMethodNone, RequiredScopes: []string{"api", "read_user"}}, nil
	}
	status := &Status{
		Username:        cfg.Username,
		AuthMethod:      cfg.AuthMethod,
		Host:            cfg.Host,
		TokenConfigured: cfg.HasSecret,
		RequiredScopes:  []string{"api", "read_user"},
	}
	client, clientErr := s.ClientForWorkspace(ctx, workspaceID)
	if clientErr != nil {
		status.ConnectionError = connectionUnavailable
		return status, nil
	}
	status.Authenticated, clientErr = client.IsAuthenticated(ctx)
	if clientErr != nil {
		status.ConnectionError = connectionUnavailable
	}
	if status.Authenticated {
		status.Username, _ = client.GetAuthenticatedUser(ctx)
	}
	if glab, ok := client.(*GLabClient); ok {
		status.GLabVersion = glab.Version()
	}
	return status, nil
}

func (s *Service) invalidateWorkspaceClient(workspaceID string) {
	s.mu.Lock()
	delete(s.workspaceClients, workspaceID)
	delete(s.workspaceClientRevs, workspaceID)
	s.mu.Unlock()
}

func (s *Service) clientForTask(ctx context.Context, taskID string) (Client, error) {
	s.mu.RLock()
	store := s.store
	legacyOnly := s.workspaceSecrets == nil
	s.mu.RUnlock()
	if store == nil {
		return s.Client(), nil
	}
	if legacyOnly {
		return s.Client(), nil
	}
	workspaceID, err := store.WorkspaceIDForTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	return s.ClientForWorkspace(ctx, workspaceID)
}

// DeleteConfigForWorkspace deletes one workspace's metadata and PAT.
func (s *Service) DeleteConfigForWorkspace(ctx context.Context, workspaceID string) error {
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return ErrWorkspaceRequired
	}
	s.configMutationMu.Lock()
	defer s.configMutationMu.Unlock()

	s.mu.RLock()
	store := s.store
	secrets := s.workspaceSecrets
	s.mu.RUnlock()
	if store == nil {
		return errors.New("gitlab store not configured")
	}
	snapshot, err := snapshotWorkspaceConnection(ctx, store, secrets, workspaceID)
	if err != nil {
		return fmt.Errorf("snapshot GitLab config: %w", err)
	}
	if secrets != nil && snapshot.secret.exists {
		if err := secrets.Delete(ctx, SecretKeyForWorkspace(workspaceID)); err != nil {
			return rollbackWorkspaceConnection(ctx, store, secrets, workspaceID, snapshot, fmt.Errorf("delete GitLab token: %w", err))
		}
	}
	if err := store.DeleteConfigForWorkspace(ctx, workspaceID); err != nil {
		return rollbackWorkspaceConnection(ctx, store, secrets, workspaceID, snapshot, fmt.Errorf("delete GitLab config: %w", err))
	}
	s.invalidateWorkspaceClient(workspaceID)
	return nil
}

// RecordWorkspaceAuthHealth probes every configured workspace independently
// and persists only sanitized status. One broken connection never prevents
// later workspace rows from being checked.
func (s *Service) RecordWorkspaceAuthHealth(ctx context.Context) {
	s.mu.RLock()
	store := s.store
	s.mu.RUnlock()
	if store == nil {
		return
	}
	workspaceIDs, err := store.ListConfigWorkspaceIDs(ctx)
	if err != nil {
		s.logger.Warn("gitlab health: list configured workspaces", zap.Error(err))
		return
	}
	for _, workspaceID := range workspaceIDs {
		if ctx.Err() != nil {
			return
		}
		s.recordWorkspaceAuthHealth(ctx, store, workspaceID)
	}
}

func (s *Service) recordWorkspaceAuthHealth(ctx context.Context, store *Store, workspaceID string) {
	checkedAt := time.Now().UTC()
	client, revision, err := s.clientForWorkspaceRevision(ctx, store, workspaceID)
	if err != nil {
		_ = s.updateConfigHealth(ctx, store, workspaceID, "", false, "connection unavailable", checkedAt, revision)
		return
	}
	username, err := client.GetAuthenticatedUser(ctx)
	if err != nil || strings.TrimSpace(username) == "" {
		_ = s.updateConfigHealth(ctx, store, workspaceID, "", false, "authentication failed", checkedAt, revision)
		return
	}
	if err := s.updateConfigHealth(ctx, store, workspaceID, username, true, "", checkedAt, revision); err != nil {
		s.logger.Warn("gitlab health: persist result", zap.String("workspace_id", workspaceID), zap.Error(err))
	}
}

func (s *Service) clientForWorkspaceRevision(ctx context.Context, store *Store, workspaceID string) (Client, int64, error) {
	s.configMutationMu.Lock()
	s.mu.RLock()
	secrets := s.workspaceSecrets
	s.mu.RUnlock()
	snapshot, err := snapshotWorkspaceConnection(ctx, store, secrets, workspaceID)
	s.configMutationMu.Unlock()
	if err != nil {
		return nil, 0, err
	}
	if snapshot.config == nil {
		return nil, 0, ErrNotConfigured
	}
	token, err := s.tokenForWorkspaceSnapshot(snapshot)
	if err != nil {
		return nil, snapshot.config.Revision, err
	}
	client, err := s.buildWorkspaceClient(ctx, snapshot.config, token)
	return client, snapshot.config.Revision, err
}

func (s *Service) tokenForWorkspaceSnapshot(snapshot workspaceConnectionSnapshot) (string, error) {
	switch snapshot.config.AuthMethod {
	case AuthMethodPAT:
		if !snapshot.secret.exists || strings.TrimSpace(snapshot.secret.value) == "" {
			return "", fmt.Errorf("%w: token required", ErrInvalidConfig)
		}
		return strings.TrimSpace(snapshot.secret.value), nil
	case AuthMethodEnvironment:
		if err := s.validateEnvironmentTokenHost(snapshot.config.AuthMethod, snapshot.config.Host); err != nil {
			return "", err
		}
		token := strings.TrimSpace(os.Getenv(secretNameToken))
		if token == "" {
			return "", fmt.Errorf("%w: GITLAB_TOKEN is empty", ErrInvalidConfig)
		}
		return token, nil
	default:
		return "", nil
	}
}

func (s *Service) updateConfigHealth(ctx context.Context, store *Store, workspaceID, username string, ok bool, errMsg string, checkedAt time.Time, revision int64) error {
	if revision == 0 {
		return nil
	}
	s.configMutationMu.Lock()
	defer s.configMutationMu.Unlock()
	_, err := store.UpdateConfigHealthForRevision(ctx, workspaceID, username, ok, errMsg, checkedAt, revision)
	return err
}
