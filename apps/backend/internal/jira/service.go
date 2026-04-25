package jira

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
)

// SecretStore is the subset of the secrets store the service needs. Kept small
// so tests can fake it easily.
type SecretStore interface {
	Reveal(ctx context.Context, id string) (string, error)
	Set(ctx context.Context, id, name, value string) error
	Delete(ctx context.Context, id string) error
	Exists(ctx context.Context, id string) (bool, error)
}

// Service orchestrates Jira config storage, the per-workspace client cache, and
// the fetch/transition operations used by the WebSocket + HTTP handlers.
type Service struct {
	store    *Store
	secrets  SecretStore
	log      *logger.Logger
	mu       sync.Mutex
	clientFn ClientFactory
	cache    map[string]Client // workspaceID → client, cleared on config change.
}

// ClientFactory builds a Client for the given config + secret. Overridable so
// tests can inject fakes without touching HTTP.
type ClientFactory func(cfg *JiraConfig, secret string) Client

// DefaultClientFactory returns a real CloudClient.
func DefaultClientFactory(cfg *JiraConfig, secret string) Client {
	return NewCloudClient(cfg, secret)
}

// NewService wires the service with a store, secret backend, and client
// factory. Pass nil for clientFn to use DefaultClientFactory.
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

// GetConfig returns the workspace config enriched with a HasSecret flag so the
// UI can distinguish "configured but empty" from "needs credentials". For
// session_cookie auth, it also tries to decode the JWT in the stored cookie
// and surface its expiry so the UI can warn the user before the session dies.
func (s *Service) GetConfig(ctx context.Context, workspaceID string) (*JiraConfig, error) {
	cfg, err := s.store.GetConfig(ctx, workspaceID)
	if err != nil || cfg == nil {
		return cfg, err
	}
	if s.secrets == nil {
		return cfg, nil
	}
	key := SecretKeyForWorkspace(workspaceID)
	exists, existsErr := s.secrets.Exists(ctx, key)
	if existsErr != nil {
		s.log.Warn("jira: secret exists check failed",
			zap.String("workspace_id", workspaceID), zap.Error(existsErr))
	}
	cfg.HasSecret = exists
	if exists && cfg.AuthMethod == AuthMethodSessionCookie {
		if secret, revealErr := s.secrets.Reveal(ctx, key); revealErr == nil {
			cfg.SecretExpiresAt = parseSessionCookieExpiry(secret)
		}
	}
	return cfg, nil
}

// ErrInvalidConfig is returned by SetConfig when the request fails validation
// (missing workspace, bad auth method, etc.). Callers map it to HTTP 400.
var ErrInvalidConfig = errors.New("jira: invalid configuration")

// SetConfig is upsert: it writes the workspace row and, when a new secret is
// provided, stores it in the encrypted secret store. An empty Secret means
// "keep the existing token" — this lets the UI edit auxiliary fields without
// forcing the user to paste the token again.
func (s *Service) SetConfig(ctx context.Context, req *SetConfigRequest) (*JiraConfig, error) {
	if err := validateConfigRequest(req); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrInvalidConfig, err.Error())
	}
	cfg := &JiraConfig{
		WorkspaceID:       req.WorkspaceID,
		SiteURL:           normalizeSiteURL(req.SiteURL),
		Email:             req.Email,
		AuthMethod:        req.AuthMethod,
		DefaultProjectKey: req.DefaultProjectKey,
	}
	if err := s.store.UpsertConfig(ctx, cfg); err != nil {
		return nil, fmt.Errorf("upsert jira config: %w", err)
	}
	if req.Secret != "" && s.secrets != nil {
		if err := s.secrets.Set(ctx,
			SecretKeyForWorkspace(req.WorkspaceID),
			"Jira token ("+req.WorkspaceID+")",
			req.Secret,
		); err != nil {
			return nil, fmt.Errorf("store jira secret: %w", err)
		}
	}
	s.invalidateClient(req.WorkspaceID)
	// Probe asynchronously so a slow Atlassian doesn't stall the save response.
	// Use a detached context with its own timeout: the request ctx may be
	// cancelled when this returns, but we still want the probe to complete and
	// update the auth-health row that the UI is about to poll.
	go func(workspaceID string) {
		probeCtx, cancel := context.WithTimeout(context.Background(), authProbeTimeout)
		defer cancel()
		s.RecordAuthHealth(probeCtx, workspaceID)
	}(req.WorkspaceID)
	return s.GetConfig(ctx, req.WorkspaceID)
}

// DeleteConfig removes both the config row and the stored secret.
func (s *Service) DeleteConfig(ctx context.Context, workspaceID string) error {
	if err := s.store.DeleteConfig(ctx, workspaceID); err != nil {
		return err
	}
	if s.secrets != nil {
		_ = s.secrets.Delete(ctx, SecretKeyForWorkspace(workspaceID))
	}
	s.invalidateClient(workspaceID)
	return nil
}

// TestConnection validates credentials either from a fresh SetConfigRequest
// (before persisting) or from the stored config (after saving). Returns a
// structured result rather than an error so the UI can render the failure
// inline.
func (s *Service) TestConnection(ctx context.Context, req *SetConfigRequest) (*TestConnectionResult, error) {
	cfg, secret, err := s.resolveCredentials(ctx, req)
	if err != nil {
		return &TestConnectionResult{OK: false, Error: err.Error()}, nil
	}
	client := s.clientFn(cfg, secret)
	return client.TestAuth(ctx)
}

// ProbeAuth validates the stored credentials for a workspace by hitting the
// cheapest authenticated endpoint. Used by the background auth-health poller
// to detect expired session cookies / step-up auth before the user clicks
// through to a tab that would 303. Errors are returned as a result so the
// poller can persist them as a failure rather than dropping them.
func (s *Service) ProbeAuth(ctx context.Context, workspaceID string) (*TestConnectionResult, error) {
	client, err := s.clientFor(ctx, workspaceID)
	if err != nil {
		return &TestConnectionResult{OK: false, Error: err.Error()}, nil
	}
	return client.TestAuth(ctx)
}

// Store exposes the underlying store so background workers (e.g. the auth
// health poller) can persist state without re-implementing the secrets+config
// resolution that the service already encapsulates.
func (s *Service) Store() *Store {
	return s.store
}

// authProbeTimeout caps a single auth-health probe so a slow workspace can't
// stall the caller. The /myself endpoint typically responds in <500ms.
const authProbeTimeout = 15 * time.Second

// RecordAuthHealth probes the workspace's stored credentials and writes the
// outcome onto the JiraConfig row. Used both at config-save time (so the UI
// reflects the new credentials immediately) and by the background poller.
// Errors during the persist step are logged but not returned: this is a
// best-effort health signal, never the source of truth for callers.
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
	if updateErr := s.store.UpdateAuthHealth(ctx, workspaceID, ok, errMsg, time.Now().UTC()); updateErr != nil {
		s.log.Warn("jira: update auth health failed",
			zap.String("workspace_id", workspaceID), zap.Error(updateErr))
	}
}

// GetTicket loads a Jira ticket by key using the workspace's stored
// credentials.
func (s *Service) GetTicket(ctx context.Context, workspaceID, ticketKey string) (*JiraTicket, error) {
	client, err := s.clientFor(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	return client.GetTicket(ctx, ticketKey)
}

// DoTransition applies a transition to a ticket.
func (s *Service) DoTransition(ctx context.Context, workspaceID, ticketKey, transitionID string) error {
	client, err := s.clientFor(ctx, workspaceID)
	if err != nil {
		return err
	}
	return client.DoTransition(ctx, ticketKey, transitionID)
}

// ListProjects is used by the settings UI to populate the project selector.
func (s *Service) ListProjects(ctx context.Context, workspaceID string) ([]JiraProject, error) {
	client, err := s.clientFor(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	return client.ListProjects(ctx)
}

// SearchTickets runs a JQL search for the workspace, returning a page of
// tickets. pageToken is the cursor returned in the previous page's
// NextPageToken; pass "" for the first page.
func (s *Service) SearchTickets(ctx context.Context, workspaceID, jql, pageToken string, maxResults int) (*SearchResult, error) {
	client, err := s.clientFor(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	return client.SearchTickets(ctx, jql, pageToken, maxResults)
}

// clientFor returns a cached client, creating one if needed. The cache is
// invalidated whenever the config changes so stale credentials never linger.
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
		if err != nil || secret == "" {
			return nil, ErrNotConfigured
		}
	}
	client := s.clientFn(cfg, secret)
	s.mu.Lock()
	defer s.mu.Unlock()
	// Re-check: another caller may have populated the cache while we were
	// fetching the config and secret. Returning the existing client keeps the
	// cache identity stable so callers comparing pointers don't see flapping.
	if existing, ok := s.cache[workspaceID]; ok {
		return existing, nil
	}
	s.cache[workspaceID] = client
	return client, nil
}

// invalidateClient drops a cached client so the next request rebuilds it with
// fresh credentials.
func (s *Service) invalidateClient(workspaceID string) {
	s.mu.Lock()
	delete(s.cache, workspaceID)
	s.mu.Unlock()
}

// resolveCredentials picks the credentials to test: if the request carries a
// secret, use it inline (pre-save); otherwise fall back to the stored secret
// (post-save re-test).
func (s *Service) resolveCredentials(ctx context.Context, req *SetConfigRequest) (*JiraConfig, string, error) {
	cfg := &JiraConfig{
		WorkspaceID: req.WorkspaceID,
		SiteURL:     normalizeSiteURL(req.SiteURL),
		Email:       req.Email,
		AuthMethod:  req.AuthMethod,
	}
	if req.Secret != "" {
		return cfg, req.Secret, nil
	}
	if s.secrets == nil {
		return nil, "", errors.New("no secret store configured")
	}
	secret, err := s.secrets.Reveal(ctx, SecretKeyForWorkspace(req.WorkspaceID))
	if err != nil || secret == "" {
		return nil, "", errors.New("no token stored — paste one to test")
	}
	// Merge with persisted config so the test uses saved site/email if the
	// caller only passed a partial request.
	if stored, _ := s.store.GetConfig(ctx, req.WorkspaceID); stored != nil {
		if cfg.SiteURL == "" {
			cfg.SiteURL = stored.SiteURL
		}
		if cfg.Email == "" {
			cfg.Email = stored.Email
		}
		if cfg.AuthMethod == "" {
			cfg.AuthMethod = stored.AuthMethod
		}
	}
	return cfg, secret, nil
}

func validateConfigRequest(req *SetConfigRequest) error {
	if req.WorkspaceID == "" {
		return errors.New("workspaceId required")
	}
	if req.SiteURL == "" {
		return errors.New("siteUrl required")
	}
	switch req.AuthMethod {
	case AuthMethodAPIToken:
		if req.Email == "" {
			return errors.New("email required for api_token auth")
		}
	case AuthMethodSessionCookie:
		// email is optional for session cookies.
	default:
		return fmt.Errorf("unknown auth method: %q", req.AuthMethod)
	}
	return nil
}

// normalizeSiteURL trims trailing slashes and prepends https:// when the user
// typed only a hostname (e.g. "acme.atlassian.net"). Without a scheme the Go
// HTTP client fails with "unsupported protocol scheme".
func normalizeSiteURL(raw string) string {
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
