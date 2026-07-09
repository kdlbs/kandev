package linear

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/watchreset"
)

// SecretStore is the subset of the secrets store the service needs.
type SecretStore interface {
	Reveal(ctx context.Context, id string) (string, error)
	Set(ctx context.Context, id, name, value string) error
	Delete(ctx context.Context, id string) error
	Exists(ctx context.Context, id string) (bool, error)
}

// RepositoryLookup is the subset of the task service used to validate a watch's
// optional repository binding (workspace ownership + default-branch fill).
// Wired post-construction via SetRepositoryLookup to avoid an import cycle with
// the task service. ok is false when the repository does not exist or has been
// soft-deleted.
type RepositoryLookup interface {
	GetRepository(ctx context.Context, id string) (workspaceID, defaultBranch string, ok bool)
}

// Service orchestrates Linear config storage, the cached client, and the
// fetch/transition operations used by the WebSocket + HTTP handlers.
type Service struct {
	store     *Store
	secrets   SecretStore
	log       *logger.Logger
	mu        sync.Mutex
	clientFn  ClientFactory
	clients   map[string]Client
	probeHook func()
	eventBus  bus.EventBus
	// taskDeleter is the cascade-delete entry point used by ResetIssueWatch.
	// Wired post-construction via SetTaskDeleter to avoid an import cycle
	// with the task service.
	taskDeleter watchreset.TaskDeleter
	// repoLookup validates an optional repository binding on create/update.
	// Wired post-construction via SetRepositoryLookup. When nil (e.g. unit
	// tests), the binding is accepted as-is and default-branch fill is skipped.
	repoLookup RepositoryLookup
	// mockClient is non-nil only when Provide built the service with a MockClient
	// (KANDEV_MOCK_LINEAR=true). Exposed via MockClient() so the e2e control
	// routes can drive the same instance the clientFn returns.
	mockClient *MockClient
}

// SetTaskDeleter wires the cascade-delete dependency used by ResetIssueWatch.
// Optional — when unset, reset returns an error so the missing wiring is
// surfaced instead of silently no-op'ing.
func (s *Service) SetTaskDeleter(td watchreset.TaskDeleter) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.taskDeleter = td
}

// SetRepositoryLookup wires the repository validator used by CreateIssueWatch /
// UpdateIssueWatch. Optional — when unset, a repository binding is persisted
// as-is without workspace/default-branch resolution.
func (s *Service) SetRepositoryLookup(rl RepositoryLookup) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.repoLookup = rl
}

func (s *Service) getRepositoryLookup() RepositoryLookup {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.repoLookup
}

// MockClient returns the shared mock client when the service was built in mock
// mode, or nil for production builds.
func (s *Service) MockClient() *MockClient {
	return s.mockClient
}

// ClientFactory builds a Client for the given config + secret. Overridable so
// tests can inject fakes without touching HTTP.
type ClientFactory func(cfg *LinearConfig, secret string) Client

// DefaultClientFactory returns a real GraphQLClient.
func DefaultClientFactory(cfg *LinearConfig, secret string) Client {
	return NewGraphQLClient(cfg, secret)
}

// NewService wires the service. Pass nil for clientFn to use the default.
func NewService(store *Store, secrets SecretStore, clientFn ClientFactory, log *logger.Logger) *Service {
	if clientFn == nil {
		clientFn = DefaultClientFactory
	}
	return &Service{
		store:    store,
		secrets:  secrets,
		log:      log,
		clientFn: clientFn,
		clients:  make(map[string]Client),
	}
}

// GetConfig returns the default workspace config enriched with a HasSecret flag.
func (s *Service) GetConfig(ctx context.Context) (*LinearConfig, error) {
	workspaceID, err := s.defaultWorkspaceID()
	if err != nil {
		return nil, err
	}
	return s.GetConfigForWorkspace(ctx, workspaceID)
}

// GetConfigForWorkspace returns a workspace config enriched with a HasSecret flag.
func (s *Service) GetConfigForWorkspace(ctx context.Context, workspaceID string) (*LinearConfig, error) {
	workspaceID, err := s.normalizeWorkspaceID(workspaceID)
	if err != nil {
		return nil, err
	}
	cfg, err := s.store.GetConfigForWorkspace(ctx, workspaceID)
	if err != nil || cfg == nil {
		return cfg, err
	}
	if s.secrets == nil {
		return cfg, nil
	}
	exists, existsErr := s.secretExists(ctx, workspaceID)
	if existsErr != nil {
		s.log.Warn("linear: secret exists check failed", zap.Error(existsErr))
	}
	cfg.HasSecret = exists
	return cfg, nil
}

// ErrInvalidConfig is returned by SetConfig when the request fails validation.
var ErrInvalidConfig = errors.New("linear: invalid configuration")

// SetConfig is upsert. An empty Secret on update keeps the existing token.
func (s *Service) SetConfig(ctx context.Context, req *SetConfigRequest) (*LinearConfig, error) {
	workspaceID, err := s.defaultWorkspaceID()
	if err != nil {
		return nil, err
	}
	return s.SetConfigForWorkspace(ctx, workspaceID, req)
}

// SetConfigForWorkspace is upsert for one workspace. An empty Secret on update
// keeps the existing token.
func (s *Service) SetConfigForWorkspace(ctx context.Context, workspaceID string, req *SetConfigRequest) (*LinearConfig, error) {
	workspaceID, err := s.normalizeWorkspaceID(workspaceID)
	if err != nil {
		return nil, err
	}
	if err := validateConfigRequest(req); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrInvalidConfig, err.Error())
	}
	cfg := &LinearConfig{
		AuthMethod:     req.AuthMethod,
		DefaultTeamKey: req.DefaultTeamKey,
	}
	if err := s.store.UpsertConfigForWorkspace(ctx, workspaceID, cfg); err != nil {
		return nil, fmt.Errorf("upsert linear config: %w", err)
	}
	if req.Secret != "" && s.secrets != nil {
		if err := s.secrets.Set(ctx, SecretKeyForWorkspace(workspaceID), "Linear API key", req.Secret); err != nil {
			return nil, fmt.Errorf("store linear secret: %w", err)
		}
	}
	s.invalidateClient(workspaceID)
	// Probe asynchronously so a slow Linear doesn't stall the save response.
	go func() {
		s.RecordAuthHealthForWorkspace(context.Background(), workspaceID)
	}()
	return s.GetConfigForWorkspace(ctx, workspaceID)
}

// DeleteConfig removes both the config row and the stored secret.
func (s *Service) DeleteConfig(ctx context.Context) error {
	workspaceID, err := s.defaultWorkspaceID()
	if err != nil {
		return err
	}
	return s.DeleteConfigForWorkspace(ctx, workspaceID)
}

// DeleteConfigForWorkspace removes both the config row and the stored secret.
func (s *Service) DeleteConfigForWorkspace(ctx context.Context, workspaceID string) error {
	workspaceID, err := s.normalizeWorkspaceID(workspaceID)
	if err != nil {
		return err
	}
	if err := s.store.DeleteConfigForWorkspace(ctx, workspaceID); err != nil {
		return err
	}
	if s.secrets != nil {
		if err := s.secrets.Delete(ctx, SecretKeyForWorkspace(workspaceID)); err != nil {
			s.log.Warn("linear: secret delete failed", zap.Error(err))
		}
	}
	s.invalidateClient(workspaceID)
	return nil
}

// TestConnection validates credentials either from a fresh SetConfigRequest
// (before persisting) or from the stored config (after saving).
func (s *Service) TestConnection(ctx context.Context, req *SetConfigRequest) (*TestConnectionResult, error) {
	workspaceID, err := s.defaultWorkspaceID()
	if err != nil {
		return &TestConnectionResult{OK: false, Error: err.Error()}, nil
	}
	return s.TestConnectionForWorkspace(ctx, workspaceID, req)
}

// TestConnectionForWorkspace validates credentials for one workspace.
func (s *Service) TestConnectionForWorkspace(ctx context.Context, workspaceID string, req *SetConfigRequest) (*TestConnectionResult, error) {
	workspaceID, err := s.normalizeWorkspaceID(workspaceID)
	if err != nil {
		return &TestConnectionResult{OK: false, Error: err.Error()}, nil
	}
	cfg, secret, err := s.resolveCredentials(ctx, workspaceID, req)
	if err != nil {
		return &TestConnectionResult{OK: false, Error: err.Error()}, nil
	}
	client := s.clientFn(cfg, secret)
	return client.TestAuth(ctx)
}

// ProbeAuth validates the stored credentials.
func (s *Service) ProbeAuth(ctx context.Context) (*TestConnectionResult, error) {
	workspaceID, err := s.defaultWorkspaceID()
	if err != nil {
		return &TestConnectionResult{OK: false, Error: err.Error()}, nil
	}
	return s.ProbeAuthForWorkspace(ctx, workspaceID)
}

// ProbeAuthForWorkspace validates the stored credentials for one workspace.
func (s *Service) ProbeAuthForWorkspace(ctx context.Context, workspaceID string) (*TestConnectionResult, error) {
	workspaceID, err := s.normalizeWorkspaceID(workspaceID)
	if err != nil {
		return &TestConnectionResult{OK: false, Error: err.Error()}, nil
	}
	client, err := s.clientFor(ctx, workspaceID)
	if err != nil {
		return &TestConnectionResult{OK: false, Error: err.Error()}, nil
	}
	return client.TestAuth(ctx)
}

// Store exposes the underlying store so background workers can persist state.
func (s *Service) Store() *Store {
	return s.store
}

// authProbeTimeout caps a single auth-health probe.
const authProbeTimeout = 15 * time.Second

// authHealthWriteTimeout bounds the DB write that persists the probe outcome.
const authHealthWriteTimeout = 5 * time.Second

// SetProbeHook installs a callback fired at the end of each RecordAuthHealth
// call. Production code never sets this; tests use it to synchronise on probe
// completion without sleep-polling.
func (s *Service) SetProbeHook(fn func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.probeHook = fn
}

// RecordAuthHealth probes credentials and writes the outcome onto the row.
func (s *Service) RecordAuthHealth(ctx context.Context) {
	workspaceIDs, err := s.store.ListConfigWorkspaceIDs(ctx)
	if err != nil {
		s.log.Warn("linear: list config workspaces failed", zap.Error(err))
		return
	}
	if len(workspaceIDs) == 0 {
		s.fireProbeHook()
		return
	}
	for _, workspaceID := range workspaceIDs {
		s.RecordAuthHealthForWorkspace(ctx, workspaceID)
	}
}

// RecordAuthHealthForWorkspace probes credentials and writes the outcome onto
// one workspace row.
func (s *Service) RecordAuthHealthForWorkspace(ctx context.Context, workspaceID string) {
	workspaceID, normalizeErr := s.normalizeWorkspaceID(workspaceID)
	if normalizeErr != nil {
		s.log.Warn("linear: resolve workspace for auth health failed", zap.Error(normalizeErr))
		return
	}
	probeCtx, cancel := context.WithTimeout(ctx, authProbeTimeout)
	defer cancel()
	res, err := s.ProbeAuthForWorkspace(probeCtx, workspaceID)
	ok := err == nil && res != nil && res.OK
	errMsg := ""
	switch {
	case err != nil:
		errMsg = err.Error()
	case res != nil && !res.OK:
		errMsg = res.Error
	}
	orgSlug := ""
	if res != nil && ok {
		orgSlug = res.OrgSlug
	}
	// Detach the DB write from ctx so a probe that exhausted its deadline can
	// still record the failure.
	writeCtx, writeCancel := context.WithTimeout(context.Background(), authHealthWriteTimeout)
	defer writeCancel()
	if updateErr := s.store.UpdateAuthHealthForWorkspace(writeCtx, workspaceID, ok, errMsg, orgSlug, time.Now().UTC()); updateErr != nil {
		s.log.Warn("linear: update auth health failed", zap.Error(updateErr))
	}
	s.fireProbeHook()
}

func (s *Service) fireProbeHook() {
	s.mu.Lock()
	hook := s.probeHook
	s.mu.Unlock()
	if hook != nil {
		hook()
	}
}

// GetIssue loads a Linear issue by identifier (e.g. "ENG-123").
func (s *Service) GetIssue(ctx context.Context, identifier string) (*LinearIssue, error) {
	workspaceID, err := s.defaultWorkspaceID()
	if err != nil {
		return nil, err
	}
	client, err := s.clientFor(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	return client.GetIssue(ctx, identifier)
}

// SetIssueState moves an issue into the requested workflow state.
func (s *Service) SetIssueState(ctx context.Context, issueID, stateID string) error {
	workspaceID, err := s.defaultWorkspaceID()
	if err != nil {
		return err
	}
	client, err := s.clientFor(ctx, workspaceID)
	if err != nil {
		return err
	}
	return client.SetIssueState(ctx, issueID, stateID)
}

// ListTeams populates the team selector on the settings page.
func (s *Service) ListTeams(ctx context.Context) ([]LinearTeam, error) {
	workspaceID, err := s.defaultWorkspaceID()
	if err != nil {
		return nil, err
	}
	client, err := s.clientFor(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	return client.ListTeams(ctx)
}

// ListStates returns the workflow states for a team identified by its key.
func (s *Service) ListStates(ctx context.Context, teamKey string) ([]LinearWorkflowState, error) {
	workspaceID, err := s.defaultWorkspaceID()
	if err != nil {
		return nil, err
	}
	client, err := s.clientFor(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	return client.ListStates(ctx, teamKey)
}

// ListLabels returns the issue labels for a team identified by its key.
func (s *Service) ListLabels(ctx context.Context, teamKey string) ([]LinearLabel, error) {
	workspaceID, err := s.defaultWorkspaceID()
	if err != nil {
		return nil, err
	}
	client, err := s.clientFor(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	return client.ListLabels(ctx, teamKey)
}

// ListUsers returns the members of a team identified by its key. Used to
// populate creator / assignee selectors on the watcher filter UI.
func (s *Service) ListUsers(ctx context.Context, teamKey string) ([]LinearUser, error) {
	workspaceID, err := s.defaultWorkspaceID()
	if err != nil {
		return nil, err
	}
	client, err := s.clientFor(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	return client.ListUsers(ctx, teamKey)
}

// SearchIssues runs a filtered search.
func (s *Service) SearchIssues(ctx context.Context, filter SearchFilter, pageToken string, maxResults int) (*SearchResult, error) {
	workspaceID, err := s.defaultWorkspaceID()
	if err != nil {
		return nil, err
	}
	return s.SearchIssuesForWorkspace(ctx, workspaceID, filter, pageToken, maxResults)
}

// SearchIssuesForWorkspace runs a filtered search against one workspace's client.
func (s *Service) SearchIssuesForWorkspace(ctx context.Context, workspaceID string, filter SearchFilter, pageToken string, maxResults int) (*SearchResult, error) {
	workspaceID, err := s.normalizeWorkspaceID(workspaceID)
	if err != nil {
		return nil, err
	}
	client, err := s.clientFor(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	return client.SearchIssues(ctx, filter, pageToken, maxResults)
}

// clientFor returns the cached client, creating it if needed.
func (s *Service) clientFor(ctx context.Context, workspaceID string) (Client, error) {
	workspaceID, err := s.normalizeWorkspaceID(workspaceID)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	if s.clients == nil {
		s.clients = make(map[string]Client)
	}
	if s.clients[workspaceID] != nil {
		c := s.clients[workspaceID]
		s.mu.Unlock()
		return c, nil
	}
	s.mu.Unlock()

	cfg, err := s.store.GetConfigForWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		return nil, ErrNotConfigured
	}
	secret := ""
	if s.secrets != nil {
		secret, err = s.revealSecret(ctx, workspaceID)
		if err != nil {
			return nil, fmt.Errorf("read linear secret: %w", err)
		}
		if secret == "" {
			return nil, ErrNotConfigured
		}
	}
	client := s.clientFn(cfg, secret)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.clients == nil {
		s.clients = make(map[string]Client)
	}
	if s.clients[workspaceID] != nil {
		return s.clients[workspaceID], nil
	}
	s.clients[workspaceID] = client
	return client, nil
}

// invalidateClient drops the cached client so the next request rebuilds it.
func (s *Service) invalidateClient(workspaceID string) {
	s.mu.Lock()
	if s.clients != nil {
		delete(s.clients, workspaceID)
	}
	s.mu.Unlock()
}

// resolveCredentials picks credentials for a test: inline if the request
// carries a secret, otherwise the stored secret.
func (s *Service) resolveCredentials(ctx context.Context, workspaceID string, req *SetConfigRequest) (*LinearConfig, string, error) {
	cfg := &LinearConfig{
		AuthMethod: req.AuthMethod,
	}
	if req.Secret != "" {
		return cfg, req.Secret, nil
	}
	if s.secrets == nil {
		return nil, "", errors.New("no secret store configured")
	}
	secret, err := s.revealSecret(ctx, workspaceID)
	if err != nil {
		s.log.Warn("linear: secret reveal failed", zap.Error(err))
		return nil, "", fmt.Errorf("read linear secret: %w", err)
	}
	if secret == "" {
		return nil, "", errors.New("no api key stored — paste one to test")
	}
	stored, storeErr := s.store.GetConfigForWorkspace(ctx, workspaceID)
	if storeErr != nil {
		// Soft-fail: a transient DB error here only loses the saved-config
		// fallback values; the inline credentials still work for the test.
		s.log.Warn("linear: load stored config for credential resolution failed", zap.Error(storeErr))
	}
	if stored != nil && cfg.AuthMethod == "" {
		cfg.AuthMethod = stored.AuthMethod
	}
	return cfg, secret, nil
}

func (s *Service) defaultWorkspaceID() (string, error) {
	return s.store.defaultWorkspaceID()
}

func (s *Service) normalizeWorkspaceID(workspaceID string) (string, error) {
	if workspaceID != "" {
		return workspaceID, nil
	}
	return s.defaultWorkspaceID()
}

func (s *Service) secretExists(ctx context.Context, workspaceID string) (bool, error) {
	if s.secrets == nil {
		return false, nil
	}
	exists, err := s.secrets.Exists(ctx, SecretKeyForWorkspace(workspaceID))
	if err != nil || exists {
		return exists, err
	}
	return s.secrets.Exists(ctx, SecretKey)
}

func (s *Service) revealSecret(ctx context.Context, workspaceID string) (string, error) {
	if s.secrets == nil {
		return "", nil
	}
	secret, err := s.secrets.Reveal(ctx, SecretKeyForWorkspace(workspaceID))
	if err == nil && secret != "" {
		return secret, nil
	}
	legacy, legacyErr := s.secrets.Reveal(ctx, SecretKey)
	if legacyErr == nil && legacy != "" {
		return legacy, nil
	}
	if err != nil {
		return "", err
	}
	return "", legacyErr
}

func validateConfigRequest(req *SetConfigRequest) error {
	if req.AuthMethod == "" {
		req.AuthMethod = AuthMethodAPIKey
	}
	if req.AuthMethod != AuthMethodAPIKey {
		return fmt.Errorf("unknown auth method: %q", req.AuthMethod)
	}
	return nil
}
