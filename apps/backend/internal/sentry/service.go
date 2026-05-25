package sentry

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
)

// SecretStore is the subset of the secrets store the service needs.
type SecretStore interface {
	Reveal(ctx context.Context, id string) (string, error)
	Set(ctx context.Context, id, name, value string) error
	Delete(ctx context.Context, id string) error
	Exists(ctx context.Context, id string) (bool, error)
}

// Service orchestrates Sentry config storage, the cached client, and the
// browse operations used by the HTTP handlers.
type Service struct {
	store     *Store
	secrets   SecretStore
	log       *logger.Logger
	mu        sync.Mutex
	clientFn  ClientFactory
	client    Client
	probeHook func()
	// mockClient is non-nil only when Provide built the service with a MockClient.
	mockClient *MockClient
	// eventBus is wired by SetEventBus so the poller can publish
	// NewSentryIssueEvent. Optional: when nil, observed issues are not
	// surfaced to the orchestrator.
	eventBus bus.EventBus
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
	}
}

// Store exposes the underlying store so background workers can persist state.
func (s *Service) Store() *Store {
	return s.store
}

// GetConfig returns the singleton config enriched with a HasSecret flag.
func (s *Service) GetConfig(ctx context.Context) (*SentryConfig, error) {
	cfg, err := s.store.GetConfig(ctx)
	if err != nil || cfg == nil {
		return cfg, err
	}
	if s.secrets == nil {
		return cfg, nil
	}
	exists, existsErr := s.secrets.Exists(ctx, SecretKey)
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
	if err := validateConfigRequest(req); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrInvalidConfig, err.Error())
	}
	cfg := &SentryConfig{
		AuthMethod:         req.AuthMethod,
		DefaultOrgSlug:     req.DefaultOrgSlug,
		DefaultProjectSlug: req.DefaultProjectSlug,
	}
	if err := s.store.UpsertConfig(ctx, cfg); err != nil {
		return nil, fmt.Errorf("upsert sentry config: %w", err)
	}
	if req.Secret != "" && s.secrets != nil {
		if err := s.secrets.Set(ctx, SecretKey, "Sentry auth token", req.Secret); err != nil {
			return nil, fmt.Errorf("store sentry secret: %w", err)
		}
	}
	s.invalidateClient()
	// Probe asynchronously so a slow Sentry doesn't stall the save response.
	go func() {
		s.RecordAuthHealth(context.Background())
	}()
	return s.GetConfig(ctx)
}

// DeleteConfig removes both the config row and the stored secret.
func (s *Service) DeleteConfig(ctx context.Context) error {
	if err := s.store.DeleteConfig(ctx); err != nil {
		return err
	}
	if s.secrets != nil {
		if err := s.secrets.Delete(ctx, SecretKey); err != nil {
			s.log.Warn("sentry: secret delete failed", zap.Error(err))
		}
	}
	s.invalidateClient()
	return nil
}

// TestConnection validates credentials either from a fresh SetConfigRequest
// (before persisting) or from the stored config (after saving).
func (s *Service) TestConnection(ctx context.Context, req *SetConfigRequest) (*TestConnectionResult, error) {
	cfg, secret, err := s.resolveCredentials(ctx, req)
	if err != nil {
		return &TestConnectionResult{OK: false, Error: err.Error()}, nil
	}
	client := s.clientFn(cfg, secret)
	return client.TestAuth(ctx)
}

// ProbeAuth validates the stored credentials.
func (s *Service) ProbeAuth(ctx context.Context) (*TestConnectionResult, error) {
	client, err := s.clientFor(ctx)
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
	probeCtx, cancel := context.WithTimeout(ctx, authProbeTimeout)
	defer cancel()
	res, err := s.ProbeAuth(probeCtx)
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
	if updateErr := s.store.UpdateAuthHealth(writeCtx, ok, errMsg, time.Now().UTC()); updateErr != nil {
		s.log.Warn("sentry: update auth health failed", zap.Error(updateErr))
	}
	s.mu.Lock()
	hook := s.probeHook
	s.mu.Unlock()
	if hook != nil {
		hook()
	}
}

// ListProjects returns the projects the stored token can access.
func (s *Service) ListProjects(ctx context.Context) ([]SentryProject, error) {
	client, err := s.clientFor(ctx)
	if err != nil {
		return nil, err
	}
	return client.ListProjects(ctx)
}

// SearchIssues runs a filtered search.
func (s *Service) SearchIssues(ctx context.Context, filter SearchFilter, cursor string) (*SearchResult, error) {
	client, err := s.clientFor(ctx)
	if err != nil {
		return nil, err
	}
	if filter.OrgSlug == "" {
		filter.OrgSlug = s.defaultOrgSlug(ctx)
	}
	return client.SearchIssues(ctx, filter, cursor)
}

// GetIssue loads a single issue by short ID or numeric ID.
func (s *Service) GetIssue(ctx context.Context, idOrShortID string) (*SentryIssue, error) {
	client, err := s.clientFor(ctx)
	if err != nil {
		return nil, err
	}
	return client.GetIssue(ctx, idOrShortID)
}

// defaultOrgSlug reads the stored DefaultOrgSlug so search callers can omit it.
func (s *Service) defaultOrgSlug(ctx context.Context) string {
	cfg, err := s.store.GetConfig(ctx)
	if err != nil || cfg == nil {
		return ""
	}
	return cfg.DefaultOrgSlug
}

// clientFor returns the cached client, creating it if needed.
func (s *Service) clientFor(ctx context.Context) (Client, error) {
	s.mu.Lock()
	if s.client != nil {
		c := s.client
		s.mu.Unlock()
		return c, nil
	}
	s.mu.Unlock()

	cfg, err := s.store.GetConfig(ctx)
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		return nil, ErrNotConfigured
	}
	secret := ""
	if s.secrets != nil {
		secret, err = s.secrets.Reveal(ctx, SecretKey)
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
	if s.client != nil {
		return s.client, nil
	}
	s.client = client
	return client, nil
}

// invalidateClient drops the cached client so the next request rebuilds it.
func (s *Service) invalidateClient() {
	s.mu.Lock()
	s.client = nil
	s.mu.Unlock()
}

// resolveCredentials picks credentials for a test: inline if the request
// carries a secret, otherwise the stored secret.
func (s *Service) resolveCredentials(ctx context.Context, req *SetConfigRequest) (*SentryConfig, string, error) {
	cfg := &SentryConfig{
		AuthMethod:         req.AuthMethod,
		DefaultOrgSlug:     req.DefaultOrgSlug,
		DefaultProjectSlug: req.DefaultProjectSlug,
	}
	if req.Secret != "" {
		return cfg, req.Secret, nil
	}
	if s.secrets == nil {
		return nil, "", errors.New("no secret store configured")
	}
	secret, err := s.secrets.Reveal(ctx, SecretKey)
	if err != nil {
		s.log.Warn("sentry: secret reveal failed", zap.Error(err))
		return nil, "", fmt.Errorf("read sentry secret: %w", err)
	}
	if secret == "" {
		return nil, "", errors.New("no auth token stored — paste one to test")
	}
	stored, storeErr := s.store.GetConfig(ctx)
	if storeErr != nil {
		s.log.Warn("sentry: load stored config for credential resolution failed", zap.Error(storeErr))
	}
	if stored != nil && cfg.AuthMethod == "" {
		cfg.AuthMethod = stored.AuthMethod
	}
	return cfg, secret, nil
}

func validateConfigRequest(req *SetConfigRequest) error {
	if req.AuthMethod == "" {
		req.AuthMethod = AuthMethodAuthToken
	}
	if req.AuthMethod != AuthMethodAuthToken {
		return fmt.Errorf("unknown auth method: %q", req.AuthMethod)
	}
	return nil
}
