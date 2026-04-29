package linear

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
)

// SecretStore is the subset of the secrets store the service needs.
type SecretStore interface {
	Reveal(ctx context.Context, id string) (string, error)
	Set(ctx context.Context, id, name, value string) error
	Delete(ctx context.Context, id string) error
	Exists(ctx context.Context, id string) (bool, error)
}

// Service orchestrates Linear config storage, the per-workspace client cache,
// and the fetch/transition operations used by the WebSocket + HTTP handlers.
type Service struct {
	store     *Store
	secrets   SecretStore
	log       *logger.Logger
	mu        sync.Mutex
	clientFn  ClientFactory
	cache     map[string]Client // workspaceID → client, cleared on config change.
	probeHook func(workspaceID string)
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
		cache:    make(map[string]Client),
	}
}

// GetConfig returns the workspace config enriched with a HasSecret flag.
func (s *Service) GetConfig(ctx context.Context, workspaceID string) (*LinearConfig, error) {
	cfg, err := s.store.GetConfig(ctx, workspaceID)
	if err != nil || cfg == nil {
		return cfg, err
	}
	if s.secrets == nil {
		return cfg, nil
	}
	exists, existsErr := s.secrets.Exists(ctx, SecretKeyForWorkspace(workspaceID))
	if existsErr != nil {
		s.log.Warn("linear: secret exists check failed",
			zap.String("workspace_id", workspaceID), zap.Error(existsErr))
	}
	cfg.HasSecret = exists
	return cfg, nil
}

// ErrInvalidConfig is returned by SetConfig when the request fails validation.
var ErrInvalidConfig = errors.New("linear: invalid configuration")

// SetConfig is upsert. An empty Secret on update keeps the existing token.
func (s *Service) SetConfig(ctx context.Context, req *SetConfigRequest) (*LinearConfig, error) {
	if err := validateConfigRequest(req); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrInvalidConfig, err.Error())
	}
	cfg := &LinearConfig{
		WorkspaceID:    req.WorkspaceID,
		AuthMethod:     req.AuthMethod,
		DefaultTeamKey: req.DefaultTeamKey,
	}
	if err := s.store.UpsertConfig(ctx, cfg); err != nil {
		return nil, fmt.Errorf("upsert linear config: %w", err)
	}
	if req.Secret != "" && s.secrets != nil {
		if err := s.secrets.Set(ctx,
			SecretKeyForWorkspace(req.WorkspaceID),
			"Linear API key ("+req.WorkspaceID+")",
			req.Secret,
		); err != nil {
			return nil, fmt.Errorf("store linear secret: %w", err)
		}
	}
	s.invalidateClient(req.WorkspaceID)
	// Probe asynchronously so a slow Linear doesn't stall the save response.
	go func(workspaceID string) {
		s.RecordAuthHealth(context.Background(), workspaceID)
	}(req.WorkspaceID)
	return s.GetConfig(ctx, req.WorkspaceID)
}

// DeleteConfig removes both the config row and the stored secret.
func (s *Service) DeleteConfig(ctx context.Context, workspaceID string) error {
	if err := s.store.DeleteConfig(ctx, workspaceID); err != nil {
		return err
	}
	if s.secrets != nil {
		if err := s.secrets.Delete(ctx, SecretKeyForWorkspace(workspaceID)); err != nil {
			s.log.Warn("linear: secret delete failed",
				zap.String("workspace_id", workspaceID), zap.Error(err))
		}
	}
	s.invalidateClient(workspaceID)
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

// ProbeAuth validates the stored credentials for a workspace.
func (s *Service) ProbeAuth(ctx context.Context, workspaceID string) (*TestConnectionResult, error) {
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
func (s *Service) SetProbeHook(fn func(workspaceID string)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.probeHook = fn
}

// RecordAuthHealth probes credentials and writes the outcome onto the row.
func (s *Service) RecordAuthHealth(ctx context.Context, workspaceID string) {
	probeCtx, cancel := context.WithTimeout(ctx, authProbeTimeout)
	defer cancel()
	res, err := s.ProbeAuth(probeCtx, workspaceID)
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
	if updateErr := s.store.UpdateAuthHealth(writeCtx, workspaceID, ok, errMsg, orgSlug, time.Now().UTC()); updateErr != nil {
		s.log.Warn("linear: update auth health failed",
			zap.String("workspace_id", workspaceID), zap.Error(updateErr))
	}
	s.mu.Lock()
	hook := s.probeHook
	s.mu.Unlock()
	if hook != nil {
		hook(workspaceID)
	}
}

// GetIssue loads a Linear issue by identifier (e.g. "ENG-123").
func (s *Service) GetIssue(ctx context.Context, workspaceID, identifier string) (*LinearIssue, error) {
	client, err := s.clientFor(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	return client.GetIssue(ctx, identifier)
}

// SetIssueState moves an issue into the requested workflow state.
func (s *Service) SetIssueState(ctx context.Context, workspaceID, issueID, stateID string) error {
	client, err := s.clientFor(ctx, workspaceID)
	if err != nil {
		return err
	}
	return client.SetIssueState(ctx, issueID, stateID)
}

// ListTeams populates the team selector on the settings page.
func (s *Service) ListTeams(ctx context.Context, workspaceID string) ([]LinearTeam, error) {
	client, err := s.clientFor(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	return client.ListTeams(ctx)
}

// ListStates returns the workflow states for a team identified by its key.
// Mirrors the other Service methods so handlers never reach into clientFor
// directly.
func (s *Service) ListStates(ctx context.Context, workspaceID, teamKey string) ([]LinearWorkflowState, error) {
	client, err := s.clientFor(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	return client.ListStates(ctx, teamKey)
}

// SearchIssues runs a filtered search for the workspace.
func (s *Service) SearchIssues(ctx context.Context, workspaceID string, filter SearchFilter, pageToken string, maxResults int) (*SearchResult, error) {
	client, err := s.clientFor(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	return client.SearchIssues(ctx, filter, pageToken, maxResults)
}

// clientFor returns a cached client, creating one if needed.
func (s *Service) clientFor(ctx context.Context, workspaceID string) (Client, error) {
	s.mu.Lock()
	if c, ok := s.cache[workspaceID]; ok {
		s.mu.Unlock()
		return c, nil
	}
	s.mu.Unlock()

	cfg, err := s.store.GetConfig(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		return nil, ErrNotConfigured
	}
	secret := ""
	if s.secrets != nil {
		secret, err = s.secrets.Reveal(ctx, SecretKeyForWorkspace(workspaceID))
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
	if existing, ok := s.cache[workspaceID]; ok {
		return existing, nil
	}
	s.cache[workspaceID] = client
	return client, nil
}

// invalidateClient drops a cached client so the next request rebuilds it.
func (s *Service) invalidateClient(workspaceID string) {
	s.mu.Lock()
	delete(s.cache, workspaceID)
	s.mu.Unlock()
}

// resolveCredentials picks credentials for a test: inline if the request
// carries a secret, otherwise the stored secret.
func (s *Service) resolveCredentials(ctx context.Context, req *SetConfigRequest) (*LinearConfig, string, error) {
	cfg := &LinearConfig{
		WorkspaceID: req.WorkspaceID,
		AuthMethod:  req.AuthMethod,
	}
	if req.Secret != "" {
		return cfg, req.Secret, nil
	}
	if s.secrets == nil {
		return nil, "", errors.New("no secret store configured")
	}
	secret, err := s.secrets.Reveal(ctx, SecretKeyForWorkspace(req.WorkspaceID))
	if err != nil {
		s.log.Warn("linear: secret reveal failed",
			zap.String("workspace_id", req.WorkspaceID), zap.Error(err))
		return nil, "", fmt.Errorf("read linear secret: %w", err)
	}
	if secret == "" {
		return nil, "", errors.New("no api key stored — paste one to test")
	}
	stored, storeErr := s.store.GetConfig(ctx, req.WorkspaceID)
	if storeErr != nil {
		// Soft-fail: a transient DB error here only loses the saved-config
		// fallback values; the inline credentials still work for the test.
		s.log.Warn("linear: load stored config for credential resolution failed",
			zap.String("workspace_id", req.WorkspaceID), zap.Error(storeErr))
	}
	if stored != nil && cfg.AuthMethod == "" {
		cfg.AuthMethod = stored.AuthMethod
	}
	return cfg, secret, nil
}

func validateConfigRequest(req *SetConfigRequest) error {
	if req.WorkspaceID == "" {
		return errors.New("workspaceId required")
	}
	if req.AuthMethod == "" {
		req.AuthMethod = AuthMethodAPIKey
	}
	if req.AuthMethod != AuthMethodAPIKey {
		return fmt.Errorf("unknown auth method: %q", req.AuthMethod)
	}
	return nil
}
