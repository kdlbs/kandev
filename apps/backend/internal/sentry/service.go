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

// ErrInvalidConfig is returned when a create/update request fails validation.
var ErrInvalidConfig = errors.New("sentry: invalid configuration")

// ErrInstanceNotFound is returned when an instance ID does not exist or does
// not belong to the requesting workspace. Callers map this to HTTP 404.
var ErrInstanceNotFound = errors.New("sentry: instance not found")

// ErrInstanceRequired is returned when an operation needs an instance ID but
// none was supplied. Callers map this to HTTP 400.
var ErrInstanceRequired = errors.New("sentry: a Sentry instance must be selected")

// ErrDuplicateInstanceName is returned when a create/update would collide with
// an existing instance name in the same workspace. Callers map this to HTTP 409.
var ErrDuplicateInstanceName = errors.New("sentry: an instance with this name already exists in this workspace")

// ErrInstanceInUse is returned when an instance cannot be deleted because issue
// watches still reference it. WatchCount is the number of referencing watches
// (enabled + disabled). Callers map this to HTTP 409.
type ErrInstanceInUse struct{ WatchCount int }

func (e ErrInstanceInUse) Error() string {
	return fmt.Sprintf("sentry: instance is referenced by %d issue watch(es)", e.WatchCount)
}

// Service orchestrates Sentry instance storage, the per-instance cached client,
// and the browse operations used by the HTTP handlers.
type Service struct {
	store    *Store
	secrets  SecretStore
	log      *logger.Logger
	mu       sync.Mutex
	clientFn ClientFactory
	// clients caches one Client per instance ID.
	clients map[string]Client
	// clientGen is incremented by invalidateClient each time a cached client
	// is discarded. clientForInstance captures it before I/O and only stores a
	// newly built client when the value is unchanged, preventing a stale client
	// from clobbering a concurrent invalidation.
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

// ListInstances returns every instance in a workspace, each enriched with a
// HasSecret flag.
func (s *Service) ListInstances(ctx context.Context, workspaceID string) ([]*SentryConfig, error) {
	cfgs, err := s.store.ListInstances(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	for _, cfg := range cfgs {
		cfg.HasSecret = s.secretExists(ctx, cfg.ID)
	}
	return cfgs, nil
}

// GetInstance returns one instance enriched with a HasSecret flag, or
// ErrInstanceNotFound when the ID does not exist in the workspace.
func (s *Service) GetInstance(ctx context.Context, workspaceID, id string) (*SentryConfig, error) {
	cfg, err := s.store.GetInstance(ctx, id)
	if err != nil {
		return nil, err
	}
	if cfg == nil || cfg.WorkspaceID != workspaceID {
		return nil, ErrInstanceNotFound
	}
	cfg.HasSecret = s.secretExists(ctx, cfg.ID)
	return cfg, nil
}

// CreateInstance validates the request, persists a new instance, stores the
// secret, and kicks off an async auth-health probe.
func (s *Service) CreateInstance(ctx context.Context, workspaceID string, req *CreateConfigRequest) (*SentryConfig, error) {
	authMethod, instanceURL := req.AuthMethod, req.URL
	if err := validateInstanceWrite(req.Name, &authMethod, &instanceURL); err != nil {
		return nil, err
	}
	cfg := &SentryConfig{
		WorkspaceID: workspaceID,
		Name:        strings.TrimSpace(req.Name),
		AuthMethod:  authMethod,
		URL:         instanceURL,
	}
	if err := s.store.CreateInstance(ctx, cfg); err != nil {
		return nil, err
	}
	if req.Secret != "" && s.secrets != nil {
		if err := s.secrets.Set(ctx, secretKeyForInstance(cfg.ID), "Sentry auth token", req.Secret); err != nil {
			return nil, fmt.Errorf("store sentry secret: %w", err)
		}
	}
	s.invalidateClient(cfg.ID)
	go s.RecordAuthHealthForInstance(context.Background(), cfg.ID)
	return s.GetInstance(ctx, workspaceID, cfg.ID)
}

// UpdateInstance validates and applies an update to an existing instance. An
// empty Secret keeps the existing token; a non-empty Secret replaces it.
func (s *Service) UpdateInstance(ctx context.Context, workspaceID, id string, req *UpdateConfigRequest) (*SentryConfig, error) {
	existing, err := s.store.GetInstance(ctx, id)
	if err != nil {
		return nil, err
	}
	if existing == nil || existing.WorkspaceID != workspaceID {
		return nil, ErrInstanceNotFound
	}
	authMethod, instanceURL := req.AuthMethod, req.URL
	if err := validateInstanceWrite(req.Name, &authMethod, &instanceURL); err != nil {
		return nil, err
	}
	existing.Name = strings.TrimSpace(req.Name)
	existing.AuthMethod = authMethod
	existing.URL = instanceURL
	if err := s.store.UpdateInstance(ctx, existing); err != nil {
		return nil, err
	}
	if req.Secret != "" && s.secrets != nil {
		if err := s.secrets.Set(ctx, secretKeyForInstance(id), "Sentry auth token", req.Secret); err != nil {
			return nil, fmt.Errorf("store sentry secret: %w", err)
		}
	}
	s.invalidateClient(id)
	go s.RecordAuthHealthForInstance(context.Background(), id)
	return s.GetInstance(ctx, workspaceID, id)
}

// DeleteInstance removes an instance, its secret, and its cached client. It
// refuses deletion when issue watches still reference the instance, returning
// ErrInstanceInUse with the watch count; the DB ON DELETE RESTRICT foreign key
// is the atomic net for a watch created between the count and the delete.
func (s *Service) DeleteInstance(ctx context.Context, workspaceID, id string) error {
	existing, err := s.store.GetInstance(ctx, id)
	if err != nil {
		return err
	}
	if existing == nil || existing.WorkspaceID != workspaceID {
		return ErrInstanceNotFound
	}
	count, err := s.store.CountWatchesForInstance(ctx, id)
	if err != nil {
		return err
	}
	if count > 0 {
		return ErrInstanceInUse{WatchCount: count}
	}
	if err := s.store.DeleteInstance(ctx, id); err != nil {
		var inUse ErrInstanceInUse
		if errors.As(err, &inUse) {
			return ErrInstanceInUse{WatchCount: s.recountWatches(ctx, id)}
		}
		return err
	}
	if s.secrets != nil {
		if err := s.secrets.Delete(ctx, secretKeyForInstance(id)); err != nil {
			s.log.Warn("sentry: secret delete failed", zap.Error(err))
		}
	}
	s.invalidateClient(id)
	return nil
}

// recountWatches returns the current watch count for an instance, defaulting to
// 1 when the recount itself fails so the caller still reports "in use".
func (s *Service) recountWatches(ctx context.Context, instanceID string) int {
	if n, err := s.store.CountWatchesForInstance(ctx, instanceID); err == nil {
		return n
	}
	return 1
}

// TestConnectionCandidate validates arbitrary credentials supplied by the UI
// before they are persisted (the pre-save "Test connection" flow).
func (s *Service) TestConnectionCandidate(ctx context.Context, req *CreateConfigRequest) (*TestConnectionResult, error) {
	authMethod, instanceURL := req.AuthMethod, req.URL
	if err := prepareInstanceCredentials(&authMethod, &instanceURL); err != nil {
		return &TestConnectionResult{OK: false, Error: err.Error()}, nil
	}
	if req.Secret == "" {
		return &TestConnectionResult{OK: false, Error: "no auth token provided — paste one to test"}, nil
	}
	cfg := &SentryConfig{AuthMethod: authMethod, URL: instanceURL}
	return s.clientFn(cfg, req.Secret).TestAuth(ctx)
}

// TestInstance validates the stored credentials of an existing instance after
// resolving and ownership-checking it against the workspace.
func (s *Service) TestInstance(ctx context.Context, workspaceID, instanceID string) (*TestConnectionResult, error) {
	if _, err := s.requireInstance(ctx, workspaceID, instanceID); err != nil {
		return nil, err
	}
	client, err := s.clientForInstance(ctx, instanceID)
	if err != nil {
		return &TestConnectionResult{OK: false, Error: err.Error()}, nil
	}
	return client.TestAuth(ctx)
}

// authProbeTimeout caps a single auth-health probe.
const authProbeTimeout = 15 * time.Second

// authHealthWriteTimeout bounds the DB write that persists the probe outcome.
const authHealthWriteTimeout = 5 * time.Second

// authHealthPoolSize bounds concurrent per-instance probes so one instance on a
// 15s timeout cannot stall the whole poll while many instances are configured.
const authHealthPoolSize = 6

// SetProbeHook installs a callback fired at the end of each RecordAuthHealth /
// RecordAuthHealthForInstance call. Production code never sets this; tests use
// it to synchronise on probe completion without sleep-polling.
func (s *Service) SetProbeHook(fn func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.probeHook = fn
}

// RecordAuthHealth probes every instance's credentials through a bounded worker
// pool and writes each outcome onto its row.
func (s *Service) RecordAuthHealth(ctx context.Context) {
	instances, err := s.store.ListAllInstances(ctx)
	if err != nil {
		s.log.Warn("sentry: list instances for auth health failed", zap.Error(err))
		return
	}
	if len(instances) == 0 {
		s.fireProbeHook()
		return
	}
	sem := make(chan struct{}, authHealthPoolSize)
	var wg sync.WaitGroup
	for _, inst := range instances {
		if ctx.Err() != nil {
			break
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(id string) {
			defer wg.Done()
			defer func() { <-sem }()
			s.recordAuthHealthForInstance(ctx, id)
		}(inst.ID)
	}
	wg.Wait()
	s.fireProbeHook()
}

// RecordAuthHealthForInstance probes one instance's credentials, writes the
// outcome, and fires the probe hook.
func (s *Service) RecordAuthHealthForInstance(ctx context.Context, instanceID string) {
	s.recordAuthHealthForInstance(ctx, instanceID)
	s.fireProbeHook()
}

func (s *Service) recordAuthHealthForInstance(ctx context.Context, instanceID string) {
	probeCtx, cancel := context.WithTimeout(ctx, authProbeTimeout)
	defer cancel()
	res, err := s.probeInstance(probeCtx, instanceID)
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
	if updateErr := s.store.UpdateAuthHealthForInstance(writeCtx, instanceID, ok, errMsg, time.Now().UTC()); updateErr != nil {
		s.log.Warn("sentry: update auth health failed", zap.Error(updateErr))
	}
}

// probeInstance builds the instance client and runs a TestAuth probe. A build
// failure (e.g. no secret) is reported as a non-OK result, not an error, so the
// health row records the cause.
func (s *Service) probeInstance(ctx context.Context, instanceID string) (*TestConnectionResult, error) {
	client, err := s.clientForInstance(ctx, instanceID)
	if err != nil {
		return &TestConnectionResult{OK: false, Error: err.Error()}, nil
	}
	return client.TestAuth(ctx)
}

func (s *Service) fireProbeHook() {
	s.mu.Lock()
	hook := s.probeHook
	s.mu.Unlock()
	if hook != nil {
		hook()
	}
}

// ListOrganizations returns the organizations the instance's token can access.
func (s *Service) ListOrganizations(ctx context.Context, workspaceID, instanceID string) ([]SentryOrganization, error) {
	client, err := s.browseClient(ctx, workspaceID, instanceID)
	if err != nil {
		return nil, err
	}
	return client.ListOrganizations(ctx)
}

// ListProjects returns the projects the instance's token can access.
func (s *Service) ListProjects(ctx context.Context, workspaceID, instanceID string) ([]SentryProject, error) {
	client, err := s.browseClient(ctx, workspaceID, instanceID)
	if err != nil {
		return nil, err
	}
	return client.ListProjects(ctx)
}

// SearchIssues runs a filtered search against one instance's client.
func (s *Service) SearchIssues(ctx context.Context, workspaceID, instanceID string, filter SearchFilter, cursor string) (*SearchResult, error) {
	client, err := s.browseClient(ctx, workspaceID, instanceID)
	if err != nil {
		return nil, err
	}
	return client.SearchIssues(ctx, filter, cursor)
}

// GetIssue loads a single issue by short ID or numeric ID from one instance.
func (s *Service) GetIssue(ctx context.Context, workspaceID, instanceID, idOrShortID string) (*SentryIssue, error) {
	client, err := s.browseClient(ctx, workspaceID, instanceID)
	if err != nil {
		return nil, err
	}
	return client.GetIssue(ctx, idOrShortID)
}

// browseClient resolves and ownership-checks the instance, then returns its
// cached client. Returns ErrInstanceRequired / ErrInstanceNotFound before any
// client is built.
func (s *Service) browseClient(ctx context.Context, workspaceID, instanceID string) (Client, error) {
	if _, err := s.requireInstance(ctx, workspaceID, instanceID); err != nil {
		return nil, err
	}
	return s.clientForInstance(ctx, instanceID)
}

// requireInstance resolves an instance and asserts it belongs to the workspace.
// Returns ErrInstanceRequired for a blank ID and ErrInstanceNotFound when the
// instance is missing or owned by another workspace.
func (s *Service) requireInstance(ctx context.Context, workspaceID, instanceID string) (*SentryConfig, error) {
	if instanceID == "" {
		return nil, ErrInstanceRequired
	}
	cfg, err := s.store.GetInstance(ctx, instanceID)
	if err != nil {
		return nil, err
	}
	if cfg == nil || cfg.WorkspaceID != workspaceID {
		return nil, ErrInstanceNotFound
	}
	return cfg, nil
}

// clientForInstance returns the cached client for an instance, creating it if
// needed. It captures the generation counter before releasing the lock for I/O
// so a concurrent invalidateClient during the build window is not silently
// overwritten: if the counter changed the freshly built client is returned to
// the caller without being cached, and the next call rebuilds.
func (s *Service) clientForInstance(ctx context.Context, instanceID string) (Client, error) {
	if instanceID == "" {
		return nil, ErrNotConfigured
	}
	s.mu.Lock()
	if s.clients == nil {
		s.clients = make(map[string]Client)
	}
	if s.clientGen == nil {
		s.clientGen = make(map[string]uint64)
	}
	if c := s.clients[instanceID]; c != nil {
		s.mu.Unlock()
		return c, nil
	}
	gen := s.clientGen[instanceID]
	s.mu.Unlock()

	cfg, err := s.store.GetInstance(ctx, instanceID)
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		return nil, ErrNotConfigured
	}
	secret := ""
	if s.secrets != nil {
		revealed, ok, revealErr := s.revealInstanceSecret(ctx, instanceID)
		if revealErr != nil {
			return nil, fmt.Errorf("read sentry secret: %w", revealErr)
		}
		if !ok {
			return nil, ErrNotConfigured
		}
		secret = revealed
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
	if c := s.clients[instanceID]; c != nil {
		// Another goroutine already cached a client; use that one.
		return c, nil
	}
	if s.clientGen[instanceID] != gen {
		// invalidateClient ran while we were doing I/O — the config/secret we
		// read is now stale. Return the client to the caller for this call only;
		// the next call rebuilds with fresh credentials.
		return client, nil
	}
	s.clients[instanceID] = client
	return client, nil
}

// invalidateClient drops the cached client for an instance so the next request
// rebuilds it. The generation counter is incremented so a concurrent
// clientForInstance build does not restore a client from the now-stale config.
func (s *Service) invalidateClient(instanceID string) {
	s.mu.Lock()
	if s.clients != nil {
		delete(s.clients, instanceID)
	}
	if s.clientGen == nil {
		s.clientGen = make(map[string]uint64)
	}
	s.clientGen[instanceID]++
	s.mu.Unlock()
}

// secretExists reports whether an instance has a stored token, distinguishing
// absence from a backend error (logged, treated as absent).
func (s *Service) secretExists(ctx context.Context, instanceID string) bool {
	if s.secrets == nil {
		return false
	}
	exists, err := s.secrets.Exists(ctx, secretKeyForInstance(instanceID))
	if err != nil {
		s.log.Warn("sentry: secret exists check failed", zap.Error(err))
		return false
	}
	return exists
}

// revealInstanceSecret returns the stored token for an instance. ok is false
// (with a nil error) when no token is stored, so callers can map that to
// ErrNotConfigured without conflating it with a backend failure.
func (s *Service) revealInstanceSecret(ctx context.Context, instanceID string) (string, bool, error) {
	if s.secrets == nil {
		return "", false, nil
	}
	exists, err := s.secrets.Exists(ctx, secretKeyForInstance(instanceID))
	if err != nil {
		return "", false, err
	}
	if !exists {
		return "", false, nil
	}
	secret, err := s.secrets.Reveal(ctx, secretKeyForInstance(instanceID))
	if err != nil {
		return "", false, err
	}
	return secret, secret != "", nil
}

// validateInstanceWrite validates a create/update request: a non-empty name and
// a usable auth method + URL (both normalized in place). All failures are
// wrapped in ErrInvalidConfig so the handler maps them to HTTP 400.
func validateInstanceWrite(name string, authMethod, instanceURL *string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("%w: name is required", ErrInvalidConfig)
	}
	if err := prepareInstanceCredentials(authMethod, instanceURL); err != nil {
		return fmt.Errorf("%w: %s", ErrInvalidConfig, err.Error())
	}
	return nil
}

// prepareInstanceCredentials defaults a blank auth method + URL and validates
// them in place, returning a plain error (callers wrap or surface it).
func prepareInstanceCredentials(authMethod, instanceURL *string) error {
	if *authMethod == "" {
		*authMethod = AuthMethodAuthToken
	}
	if *authMethod != AuthMethodAuthToken {
		return fmt.Errorf("unknown auth method: %q", *authMethod)
	}
	*instanceURL = normalizeSentryURL(*instanceURL)
	if *instanceURL == "" {
		*instanceURL = DefaultSentryURL
	}
	return validateSentryURL(*instanceURL)
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
