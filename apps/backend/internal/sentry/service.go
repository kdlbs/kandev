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

// Service orchestrates Sentry instance storage, the per-instance cached
// clients, and the browse operations used by the HTTP handlers.
type Service struct {
	store    *Store
	secrets  SecretStore
	log      *logger.Logger
	mu       sync.Mutex
	clientFn ClientFactory
	// clients caches one built Client per instance id. invalidateInstance drops
	// an entry so the next request rebuilds it from fresh config + secret.
	clients map[string]Client
	// gens is a per-instance generation counter incremented by invalidateInstance
	// each time an instance's cached client is discarded. clientForInstance
	// captures it before I/O and only stores a newly built client when the value
	// is unchanged, preventing a stale client from clobbering a concurrent
	// invalidation. Survives in the map even while clients[id] is absent.
	gens      map[string]uint64
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
}

// SetTaskDeleter wires the cascade-delete dependency used by ResetIssueWatch.
// Optional — when unset, reset returns an error so the missing wiring is
// surfaced instead of silently no-op'ing.
func (s *Service) SetTaskDeleter(td watchreset.TaskDeleter) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.taskDeleter = td
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
		store:    store,
		secrets:  secrets,
		log:      log,
		clientFn: clientFn,
		clients:  make(map[string]Client),
		gens:     make(map[string]uint64),
	}
}

// Store exposes the underlying store so background workers can persist state.
func (s *Service) Store() *Store {
	return s.store
}

// ErrInstanceNotFound is returned by instance operations targeting an unknown id.
var ErrInstanceNotFound = errors.New("sentry: instance not found")

// ErrInstanceInUse is returned by DeleteInstance when issue watches still
// reference the instance. The dependent watch count is surfaced so the API
// layer can explain why the delete was blocked.
type ErrInstanceInUse struct {
	WatchCount int
}

func (e *ErrInstanceInUse) Error() string {
	return fmt.Sprintf("sentry: instance is referenced by %d issue watch(es)", e.WatchCount)
}

// ListInstances returns every configured Sentry instance, each enriched with a
// HasSecret flag.
func (s *Service) ListInstances(ctx context.Context) ([]*SentryConfig, error) {
	configs, err := s.store.ListConfigs(ctx)
	if err != nil {
		return nil, err
	}
	for _, cfg := range configs {
		s.enrichHasSecret(ctx, cfg)
	}
	return configs, nil
}

// GetInstance returns one instance config enriched with HasSecret, or nil when
// no instance has that id.
func (s *Service) GetInstance(ctx context.Context, id string) (*SentryConfig, error) {
	cfg, err := s.store.GetConfig(ctx, id)
	if err != nil || cfg == nil {
		return cfg, err
	}
	s.enrichHasSecret(ctx, cfg)
	return cfg, nil
}

func (s *Service) enrichHasSecret(ctx context.Context, cfg *SentryConfig) {
	if s.secrets == nil {
		return
	}
	exists, err := s.secrets.Exists(ctx, secretKeyFor(cfg.ID))
	if err != nil {
		s.log.Warn("sentry: secret exists check failed", zap.Error(err))
		return
	}
	cfg.HasSecret = exists
}

// ErrInvalidConfig is returned by CreateInstance/UpdateInstance when the request
// fails validation.
var ErrInvalidConfig = errors.New("sentry: invalid configuration")

// CreateInstance validates the request and persists a new Sentry instance.
func (s *Service) CreateInstance(ctx context.Context, req *SetConfigRequest) (*SentryConfig, error) {
	if err := validateConfigRequest(req); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrInvalidConfig, err.Error())
	}
	cfg := &SentryConfig{Name: instanceName(req.Name, req.URL), AuthMethod: req.AuthMethod, URL: req.URL}
	if err := s.store.CreateConfig(ctx, cfg); err != nil {
		return nil, fmt.Errorf("create sentry config: %w", err)
	}
	if req.Secret != "" && s.secrets != nil {
		if err := s.secrets.Set(ctx, secretKeyFor(cfg.ID), "Sentry auth token", req.Secret); err != nil {
			return nil, fmt.Errorf("store sentry secret: %w", err)
		}
	}
	s.invalidateInstance(cfg.ID)
	// Probe asynchronously so a slow Sentry doesn't stall the save response.
	go s.probeOne(context.Background(), cfg.ID)
	return s.GetInstance(ctx, cfg.ID)
}

// UpdateInstance applies a full update to an existing instance. An empty Secret
// keeps the stored token; a non-empty Secret replaces it.
func (s *Service) UpdateInstance(ctx context.Context, id string, req *SetConfigRequest) (*SentryConfig, error) {
	if err := validateConfigRequest(req); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrInvalidConfig, err.Error())
	}
	existing, err := s.store.GetConfig(ctx, id)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, ErrInstanceNotFound
	}
	existing.Name = instanceName(req.Name, req.URL)
	existing.AuthMethod = req.AuthMethod
	existing.URL = req.URL
	if err := s.store.UpdateConfig(ctx, existing); err != nil {
		return nil, fmt.Errorf("update sentry config: %w", err)
	}
	if req.Secret != "" && s.secrets != nil {
		if err := s.secrets.Set(ctx, secretKeyFor(id), "Sentry auth token", req.Secret); err != nil {
			return nil, fmt.Errorf("store sentry secret: %w", err)
		}
	}
	s.invalidateInstance(id)
	go s.probeOne(context.Background(), id)
	return s.GetInstance(ctx, id)
}

// DeleteInstance removes an instance config + its stored secret. It is blocked
// with ErrInstanceInUse when issue watches still reference the instance, so a
// delete can never silently orphan watches.
func (s *Service) DeleteInstance(ctx context.Context, id string) error {
	n, err := s.store.CountWatchesForInstance(ctx, id)
	if err != nil {
		return err
	}
	if n > 0 {
		return &ErrInstanceInUse{WatchCount: n}
	}
	if err := s.store.DeleteConfig(ctx, id); err != nil {
		return err
	}
	if s.secrets != nil {
		if err := s.secrets.Delete(ctx, secretKeyFor(id)); err != nil {
			s.log.Warn("sentry: secret delete failed", zap.String("instance_id", id), zap.Error(err))
		}
	}
	s.invalidateInstance(id)
	return nil
}

// instanceName returns a non-empty instance name: the trimmed user-supplied
// name when present, otherwise one derived from the instance URL.
func instanceName(name, instanceURL string) string {
	if n := strings.TrimSpace(name); n != "" {
		return n
	}
	return instanceNameFromURL(instanceURL)
}

// TestConnection validates credentials for instance id. When id is "" (testing
// an unsaved instance), the request must carry an inline secret.
func (s *Service) TestConnection(ctx context.Context, id string, req *SetConfigRequest) (*TestConnectionResult, error) {
	cfg, secret, err := s.resolveCredentials(ctx, id, req)
	if err != nil {
		return &TestConnectionResult{OK: false, Error: err.Error()}, nil
	}
	client := s.clientFn(cfg, secret)
	return client.TestAuth(ctx)
}

// probeAuthFor validates the stored credentials of one instance.
func (s *Service) probeAuthFor(ctx context.Context, id string) (*TestConnectionResult, error) {
	client, err := s.clientForInstance(ctx, id)
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
// pass (and each async create/update probe). Production code never sets this;
// tests use it to synchronise on probe completion without sleep-polling.
func (s *Service) SetProbeHook(fn func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.probeHook = fn
}

func (s *Service) fireProbeHook() {
	s.mu.Lock()
	hook := s.probeHook
	s.mu.Unlock()
	if hook != nil {
		hook()
	}
}

// RecordAuthHealth probes the stored credentials of every configured instance
// and writes each outcome onto its row.
func (s *Service) RecordAuthHealth(ctx context.Context) {
	configs, err := s.store.ListConfigs(ctx)
	if err != nil {
		s.log.Warn("sentry: list configs for auth health failed", zap.Error(err))
		return
	}
	for _, cfg := range configs {
		if ctx.Err() != nil {
			return
		}
		s.recordAuthHealthFor(ctx, cfg.ID)
	}
	s.fireProbeHook()
}

// probeOne probes a single instance then fires the probe hook (used by the
// async create/update path).
func (s *Service) probeOne(ctx context.Context, id string) {
	s.recordAuthHealthFor(ctx, id)
	s.fireProbeHook()
}

// recordAuthHealthFor probes one instance's credentials and stamps the outcome.
func (s *Service) recordAuthHealthFor(ctx context.Context, id string) {
	probeCtx, cancel := context.WithTimeout(ctx, authProbeTimeout)
	defer cancel()
	res, err := s.probeAuthFor(probeCtx, id)
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
	if updateErr := s.store.UpdateAuthHealth(writeCtx, id, ok, errMsg, time.Now().UTC()); updateErr != nil {
		s.log.Warn("sentry: update auth health failed", zap.String("instance_id", id), zap.Error(updateErr))
	}
}

// ListOrganizations returns the organizations the instance's token can access.
func (s *Service) ListOrganizations(ctx context.Context, instanceID string) ([]SentryOrganization, error) {
	client, err := s.clientForInstance(ctx, instanceID)
	if err != nil {
		return nil, err
	}
	return client.ListOrganizations(ctx)
}

// ListProjects returns the projects the instance's token can access.
func (s *Service) ListProjects(ctx context.Context, instanceID string) ([]SentryProject, error) {
	client, err := s.clientForInstance(ctx, instanceID)
	if err != nil {
		return nil, err
	}
	return client.ListProjects(ctx)
}

// SearchIssues runs a filtered search against one instance. The caller supplies
// the org/project to search — there is no install-wide default to fall back on.
func (s *Service) SearchIssues(ctx context.Context, instanceID string, filter SearchFilter, cursor string) (*SearchResult, error) {
	client, err := s.clientForInstance(ctx, instanceID)
	if err != nil {
		return nil, err
	}
	return client.SearchIssues(ctx, filter, cursor)
}

// GetIssue loads a single issue by short ID or numeric ID from one instance.
func (s *Service) GetIssue(ctx context.Context, instanceID, idOrShortID string) (*SentryIssue, error) {
	client, err := s.clientForInstance(ctx, instanceID)
	if err != nil {
		return nil, err
	}
	return client.GetIssue(ctx, idOrShortID)
}

// clientForInstance returns the cached client for an instance, creating it if
// needed. It captures the instance's generation counter before releasing the
// lock for I/O so a concurrent invalidateInstance during the build window is not
// silently overwritten: if the counter changed the freshly built client is
// returned to the caller without being cached, and the next call rebuilds with
// the updated config.
func (s *Service) clientForInstance(ctx context.Context, instanceID string) (Client, error) {
	if instanceID == "" {
		return nil, ErrNotConfigured
	}
	s.mu.Lock()
	if c := s.clients[instanceID]; c != nil {
		s.mu.Unlock()
		return c, nil
	}
	gen := s.gens[instanceID]
	s.mu.Unlock()

	cfg, err := s.store.GetConfig(ctx, instanceID)
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		return nil, ErrNotConfigured
	}
	secret := ""
	if s.secrets != nil {
		// Check existence first so an instance saved without a token reports
		// "not configured" (503) instead of leaking the store's not-found error
		// as a 500. A real Reveal error after Exists==true still propagates.
		exists, existsErr := s.secrets.Exists(ctx, secretKeyFor(instanceID))
		if existsErr != nil {
			return nil, fmt.Errorf("check sentry secret: %w", existsErr)
		}
		if !exists {
			return nil, ErrNotConfigured
		}
		secret, err = s.secrets.Reveal(ctx, secretKeyFor(instanceID))
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
	if c := s.clients[instanceID]; c != nil {
		// Another goroutine already cached a client; use that one.
		return c, nil
	}
	if s.gens[instanceID] != gen {
		// invalidateInstance ran while we were doing I/O — the config/secret we
		// read is now stale. Return the client to the caller for this call only;
		// the next call will rebuild with fresh credentials.
		return client, nil
	}
	s.clients[instanceID] = client
	return client, nil
}

// invalidateInstance drops the cached client for an instance so the next request
// rebuilds it. The generation counter is incremented so a concurrent
// clientForInstance build does not restore a client built from the now-stale
// config.
func (s *Service) invalidateInstance(instanceID string) {
	s.mu.Lock()
	delete(s.clients, instanceID)
	s.gens[instanceID]++
	s.mu.Unlock()
}

// resolveCredentials picks credentials for a test: inline if the request
// carries a secret, otherwise the stored secret for instance id.
func (s *Service) resolveCredentials(ctx context.Context, id string, req *SetConfigRequest) (*SentryConfig, string, error) {
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
	if id == "" {
		return nil, "", errors.New("no auth token stored — paste one to test")
	}
	secret, err := s.secrets.Reveal(ctx, secretKeyFor(id))
	if err != nil {
		s.log.Warn("sentry: secret reveal failed", zap.Error(err))
		return nil, "", fmt.Errorf("read sentry secret: %w", err)
	}
	if secret == "" {
		return nil, "", errors.New("no auth token stored — paste one to test")
	}
	stored, storeErr := s.store.GetConfig(ctx, id)
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
