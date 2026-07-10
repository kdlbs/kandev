package sentry

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
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

// Service orchestrates Sentry config storage, the cached client, and the
// browse operations used by the HTTP handlers.
type Service struct {
	store    *Store
	secrets  SecretStore
	log      *logger.Logger
	mu       sync.Mutex
	clientFn ClientFactory
	clients  map[string]Client
	// clientGen is incremented by invalidateClient each time a cached client
	// is discarded. clientFor captures it before I/O and only stores a newly
	// built client when the value is unchanged, preventing a stale client from
	// clobbering a concurrent invalidation.
	clientGen map[string]uint64
	probeHook func()
	// mockClient is non-nil only when Provide built the service with a MockClient.
	mockClient *MockClient
	// eventBus is wired by SetEventBus so the poller can publish
	// NewSentryIssueEvent. Optional: when nil, observed issues are not
	// surfaced to the orchestrator.
	eventBus bus.EventBus
	// taskDeleter is the cascade-delete entry point used by ResetIssueWatch.
	// Wired post-construction via SetTaskDeleter to avoid an import cycle
	// with the task service.
	taskDeleter watchreset.TaskDeleter
	// repoLookup validates an optional repository binding on create/update.
	// Wired post-construction via SetRepositoryLookup. When nil (e.g. unit
	// tests), the binding is accepted as-is and default-branch fill is skipped.
	repoLookup RepositoryLookup
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
type ClientFactory func(cfg *SentryConfig, secret string) Client

// DefaultClientFactory returns a real RESTClient.
func DefaultClientFactory(cfg *SentryConfig, secret string) Client {
	return NewRESTClient(cfg, secret)
}

// NewService wires the service. Pass nil for clientFn to use the default.
func NewService(store *Store, secrets SecretStore, clientFn ClientFactory, log *logger.Logger) *Service {
	if clientFn == nil {
		clientFn = DefaultClientFactory
	}
	return &Service{
		store:     store,
		secrets:   secrets,
		log:       log,
		clientFn:  clientFn,
		clients:   make(map[string]Client),
		clientGen: make(map[string]uint64),
	}
}

// Store exposes the underlying store so background workers can persist state.
func (s *Service) Store() *Store {
	return s.store
}

// GetConfig returns the singleton config enriched with a HasSecret flag.
func (s *Service) GetConfig(ctx context.Context) (*SentryConfig, error) {
	workspaceID, err := s.defaultWorkspaceID()
	if err != nil {
		return nil, err
	}
	return s.GetConfigForWorkspace(ctx, workspaceID)
}

// GetConfigForWorkspace returns a workspace config enriched with a HasSecret flag.
func (s *Service) GetConfigForWorkspace(ctx context.Context, workspaceID string) (*SentryConfig, error) {
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
		s.log.Warn("sentry: secret exists check failed", zap.Error(existsErr))
	}
	cfg.HasSecret = exists
	return cfg, nil
}

// ErrInvalidConfig is returned by SetConfig when the request fails validation.
var ErrInvalidConfig = errors.New("sentry: invalid configuration")

// SetConfig is upsert. An empty Secret on update keeps the existing token.
func (s *Service) SetConfig(ctx context.Context, req *SetConfigRequest) (*SentryConfig, error) {
	workspaceID, err := s.defaultWorkspaceID()
	if err != nil {
		return nil, err
	}
	return s.SetConfigForWorkspace(ctx, workspaceID, req)
}

// SetConfigForWorkspace is upsert for one workspace. An empty Secret on update
// keeps the existing token.
func (s *Service) SetConfigForWorkspace(ctx context.Context, workspaceID string, req *SetConfigRequest) (*SentryConfig, error) {
	workspaceID, err := s.normalizeWorkspaceID(workspaceID)
	if err != nil {
		return nil, err
	}
	if err := validateConfigRequest(req); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrInvalidConfig, err.Error())
	}
	cfg := &SentryConfig{AuthMethod: req.AuthMethod, URL: req.URL}
	if err := s.store.UpsertConfigForWorkspace(ctx, workspaceID, cfg); err != nil {
		return nil, fmt.Errorf("upsert sentry config: %w", err)
	}
	if req.Secret != "" && s.secrets != nil {
		if err := s.secrets.Set(ctx, SecretKeyForWorkspace(workspaceID), "Sentry auth token", req.Secret); err != nil {
			return nil, fmt.Errorf("store sentry secret: %w", err)
		}
	}
	s.invalidateClient(workspaceID)
	// Probe asynchronously so a slow Sentry doesn't stall the save response.
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
			s.log.Warn("sentry: secret delete failed", zap.Error(err))
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
		s.log.Warn("sentry: list config workspaces failed", zap.Error(err))
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
		s.log.Warn("sentry: resolve workspace for auth health failed", zap.Error(normalizeErr))
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
	// Detach the DB write from ctx so a probe that exhausted its deadline can
	// still record the failure.
	writeCtx, writeCancel := context.WithTimeout(context.Background(), authHealthWriteTimeout)
	defer writeCancel()
	if updateErr := s.store.UpdateAuthHealthForWorkspace(writeCtx, workspaceID, ok, errMsg, time.Now().UTC()); updateErr != nil {
		s.log.Warn("sentry: update auth health failed", zap.Error(updateErr))
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

// ListOrganizations returns the organizations the stored token can access.
func (s *Service) ListOrganizations(ctx context.Context) ([]SentryOrganization, error) {
	workspaceID, err := s.defaultWorkspaceID()
	if err != nil {
		return nil, err
	}
	client, err := s.clientFor(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	return client.ListOrganizations(ctx)
}

// ListProjects returns the projects the stored token can access.
func (s *Service) ListProjects(ctx context.Context) ([]SentryProject, error) {
	workspaceID, err := s.defaultWorkspaceID()
	if err != nil {
		return nil, err
	}
	client, err := s.clientFor(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	return client.ListProjects(ctx)
}

// SearchIssues runs a filtered search. The caller supplies the org/project to
// search — there is no install-wide default to fall back on.
func (s *Service) SearchIssues(ctx context.Context, filter SearchFilter, cursor string) (*SearchResult, error) {
	workspaceID, err := s.defaultWorkspaceID()
	if err != nil {
		return nil, err
	}
	return s.SearchIssuesForWorkspace(ctx, workspaceID, filter, cursor)
}

// SearchIssuesForWorkspace runs a filtered search against one workspace's client.
func (s *Service) SearchIssuesForWorkspace(ctx context.Context, workspaceID string, filter SearchFilter, cursor string) (*SearchResult, error) {
	workspaceID, err := s.normalizeWorkspaceID(workspaceID)
	if err != nil {
		return nil, err
	}
	client, err := s.clientFor(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	return client.SearchIssues(ctx, filter, cursor)
}

// GetIssue loads a single issue by short ID or numeric ID.
func (s *Service) GetIssue(ctx context.Context, idOrShortID string) (*SentryIssue, error) {
	workspaceID, err := s.defaultWorkspaceID()
	if err != nil {
		return nil, err
	}
	client, err := s.clientFor(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	return client.GetIssue(ctx, idOrShortID)
}

// clientFor returns the cached client, creating it if needed.
// It captures the generation counter before releasing the lock for I/O so
// that a concurrent invalidateClient call during the build window is not
// silently overwritten: if the counter changed the freshly built client is
// returned to the caller without being cached, and the next call rebuilds
// with the updated config.
func (s *Service) clientFor(ctx context.Context, workspaceID string) (Client, error) {
	workspaceID, err := s.normalizeWorkspaceID(workspaceID)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	if s.clients == nil {
		s.clients = make(map[string]Client)
	}
	if s.clientGen == nil {
		s.clientGen = make(map[string]uint64)
	}
	if s.clients[workspaceID] != nil {
		c := s.clients[workspaceID]
		s.mu.Unlock()
		return c, nil
	}
	gen := s.clientGen[workspaceID]
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
			return nil, fmt.Errorf("read sentry secret: %w", err)
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
	if s.clientGen == nil {
		s.clientGen = make(map[string]uint64)
	}
	if s.clients[workspaceID] != nil {
		// Another goroutine already cached a client; use that one.
		return s.clients[workspaceID], nil
	}
	if s.clientGen[workspaceID] != gen {
		// invalidateClient ran while we were doing I/O — the config/secret we
		// read is now stale. Return the client to the caller for this call only;
		// the next call will rebuild with fresh credentials.
		return client, nil
	}
	s.clients[workspaceID] = client
	return client, nil
}

// invalidateClient drops the cached client so the next request rebuilds it.
// The generation counter is incremented so a concurrent clientFor build does
// not restore a client built from the now-stale config.
func (s *Service) invalidateClient(workspaceID string) {
	s.mu.Lock()
	if s.clients != nil {
		delete(s.clients, workspaceID)
	}
	if s.clientGen == nil {
		s.clientGen = make(map[string]uint64)
	}
	s.clientGen[workspaceID]++
	s.mu.Unlock()
}

// resolveCredentials picks credentials for a test: inline if the request
// carries a secret, otherwise the stored secret.
func (s *Service) resolveCredentials(ctx context.Context, workspaceID string, req *SetConfigRequest) (*SentryConfig, string, error) {
	cfg := &SentryConfig{AuthMethod: req.AuthMethod, URL: normalizeSentryURL(req.URL)}
	if req.Secret != "" {
		if cfg.URL == "" {
			cfg.URL = DefaultSentryURL
		}
		return cfg, req.Secret, nil
	}
	if s.secrets == nil {
		return nil, "", errors.New("no secret store configured")
	}
	secret, err := s.revealSecret(ctx, workspaceID)
	if err != nil {
		s.log.Warn("sentry: secret reveal failed", zap.Error(err))
		return nil, "", fmt.Errorf("read sentry secret: %w", err)
	}
	if secret == "" {
		return nil, "", errors.New("no auth token stored — paste one to test")
	}
	stored, storeErr := s.store.GetConfigForWorkspace(ctx, workspaceID)
	if storeErr != nil {
		s.log.Warn("sentry: load stored config for credential resolution failed", zap.Error(storeErr))
	}
	if stored != nil {
		if cfg.AuthMethod == "" {
			cfg.AuthMethod = stored.AuthMethod
		}
		if cfg.URL == "" {
			cfg.URL = stored.URL
		}
	}
	if cfg.URL == "" {
		cfg.URL = DefaultSentryURL
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

// validateConfigRequest normalizes and validates a config request in place: it
// defaults a blank auth method and instance URL, rejects unknown auth methods,
// and rejects URLs the HTTP client could not use (see validateSentryURL).
func validateConfigRequest(req *SetConfigRequest) error {
	if req.AuthMethod == "" {
		req.AuthMethod = AuthMethodAuthToken
	}
	if req.AuthMethod != AuthMethodAuthToken {
		return fmt.Errorf("unknown auth method: %q", req.AuthMethod)
	}
	req.URL = normalizeSentryURL(req.URL)
	if req.URL == "" {
		req.URL = DefaultSentryURL
	}
	return validateSentryURL(req.URL)
}

// normalizeSentryURL trims whitespace and trailing slashes and prepends
// https:// when the user typed only a host (e.g. "sentry.acme.com"). Without a
// scheme the Go HTTP client fails with "unsupported protocol scheme". An empty
// input is returned as-is so callers can apply the sentry.io default.
func normalizeSentryURL(raw string) string {
	s := strings.TrimSpace(raw)
	s = strings.TrimRight(s, "/")
	if s == "" {
		return s
	}
	if !strings.Contains(s, "://") {
		s = "https://" + s
	}
	return s
}

// validateSentryURL rejects an instance URL the HTTP client could not use: it
// must parse, carry an http/https scheme, name a host, and be a bare host root.
// A path/query/fragment is rejected because apiPathPrefix ("/api/0") is later
// appended by string concatenation — e.g. "https://host?x=1" would otherwise
// become the malformed base "https://host?x=1/api/0".
func validateSentryURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid instance URL: %s", err.Error())
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("instance URL must use http or https: %q", raw)
	}
	if u.Host == "" {
		return fmt.Errorf("instance URL must include a host: %q", raw)
	}
	if u.Path != "" && u.Path != "/" {
		return fmt.Errorf("instance URL must be a host root without a path: %q", raw)
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("instance URL must not include a query or fragment: %q", raw)
	}
	return nil
}
