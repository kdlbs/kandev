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
	"github.com/kandev/kandev/internal/common/securityutil"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/watchreset"
)

// SecretStore is the subset of the secrets store the service needs. Kept small
// so tests can fake it easily.
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

// Service orchestrates Jira config storage, the cached client, and the
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
	// taskDeleter is the cascade-delete entry point used by the watch
	// reset flow. Wired post-construction via SetTaskDeleter to avoid
	// the import cycle that would result from passing the concrete
	// *task.HandoffService into NewService.
	taskDeleter watchreset.TaskDeleter
	// repoLookup validates an optional repository binding on create/update.
	// Wired post-construction via SetRepositoryLookup. When nil (e.g. unit
	// tests), the binding is accepted as-is and default-branch fill is skipped.
	repoLookup RepositoryLookup
	// mockClient is non-nil only when Provide built the service with a MockClient
	// (KANDEV_MOCK_JIRA=true). Exposed via MockClient() so the e2e control routes
	// can drive the same instance the clientFn returns.
	mockClient *MockClient
	// workspaceAuthorizer is the per-user workspace access boundary, wired
	// post-construction via SetWorkspaceAuthorizer. Nil (unit tests, auth
	// disabled) means unscoped — every workspace is visible, as before auth.
	workspaceAuthorizer func(context.Context, string) error
	// oauthStates holds pending OAuth CSRF states in memory.
	oauthStates *mcpStateManager
	// oauthMu provides per-workspace locks for token refresh to prevent
	// concurrent refreshes from racing on the rotating refresh token.
	oauthMu *perWorkspaceMutex
	// oauthRedirectURI is the fully-qualified callback URL. Set at Provide time.
	oauthRedirectURI string
}

// SetWorkspaceAuthorizer installs the per-user workspace access boundary applied
// before user-facing Jira config reads/mutations. Wired to
// taskSvc.AuthorizeWorkspaceAccess so a caller cannot reach a workspace it does
// not own by naming its ID (e.g. the copy target, or the resolved default).
func (s *Service) SetWorkspaceAuthorizer(authorizer func(context.Context, string) error) {
	if s != nil {
		s.workspaceAuthorizer = authorizer
	}
}

func (s *Service) authorizeWorkspaceAccess(ctx context.Context, workspaceID string) error {
	if s == nil || s.workspaceAuthorizer == nil {
		return nil
	}
	return s.workspaceAuthorizer(ctx, workspaceID)
}

// resolveWorkspaceID normalizes an optional workspace ID (empty → default) and
// authorizes the caller against the resolved workspace. Using it instead of
// normalizeWorkspaceID on user-facing entry points closes the default-workspace
// fallback: a scoped caller that omits workspace_id cannot land on a global
// default it does not own.
func (s *Service) resolveWorkspaceID(ctx context.Context, workspaceID string) (string, error) {
	workspaceID, err := s.normalizeWorkspaceID(workspaceID)
	if err != nil {
		return "", err
	}
	if err := s.authorizeWorkspaceAccess(ctx, workspaceID); err != nil {
		return "", err
	}
	return workspaceID, nil
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
type ClientFactory func(cfg *JiraConfig, secret string) Client

// DefaultClientFactory returns a real CloudClient for token-based auth, or
// MCPClient for OAuth auth (which routes through the MCP server).
func DefaultClientFactory(cfg *JiraConfig, secret string) Client {
	if cfg.AuthMethod == AuthMethodOAuth {
		return NewMCPClient(secret, cfg.CloudID, cfg.WorkspaceID, cfg.SiteURL)
	}
	return NewCloudClient(cfg, secret)
}

// NewService wires the service with a store, secret backend, and client
// factory. Pass nil for clientFn to use DefaultClientFactory.
func NewService(store *Store, secrets SecretStore, clientFn ClientFactory, log *logger.Logger) *Service {
	if clientFn == nil {
		clientFn = DefaultClientFactory
	}
	return &Service{
		store:       store,
		secrets:     secrets,
		log:         log,
		clientFn:    clientFn,
		clients:     make(map[string]Client),
		oauthStates: newMCPStateManager(),
		oauthMu:     newPerWorkspaceMutex(),
	}
}

// GetConfig returns the singleton config enriched with a HasSecret flag so the
// UI can distinguish "configured but empty" from "needs credentials". For
// session_cookie auth, it also tries to decode the JWT in the stored cookie
// and surface its expiry so the UI can warn the user before the session dies.
func (s *Service) GetConfig(ctx context.Context) (*JiraConfig, error) {
	workspaceID, err := s.defaultWorkspaceID()
	if err != nil {
		return nil, err
	}
	return s.GetConfigForWorkspace(ctx, workspaceID)
}

// GetConfigForWorkspace returns a workspace config enriched with a HasSecret
// flag so the UI can distinguish "configured but empty" from "needs
// credentials".
func (s *Service) GetConfigForWorkspace(ctx context.Context, workspaceID string) (*JiraConfig, error) {
	workspaceID, err := s.resolveWorkspaceID(ctx, workspaceID)
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
		s.log.Warn("jira: secret exists check failed", zap.Error(existsErr))
	}
	cfg.HasSecret = exists
	if exists && cfg.AuthMethod == AuthMethodSessionCookie {
		secret, revealErr := s.revealSecret(ctx, workspaceID)
		if revealErr != nil {
			s.log.Warn("jira: secret reveal failed", zap.Error(revealErr))
		} else {
			cfg.SecretExpiresAt = parseSessionCookieExpiry(secret)
		}
	}
	// For OAuth, HasSecret should reflect whether the OAuth access token exists.
	if cfg.AuthMethod == AuthMethodOAuth && s.secrets != nil {
		oauthExists, oauthErr := s.secrets.Exists(ctx, OAuthAccessTokenKeyForWorkspace(workspaceID))
		if oauthErr == nil {
			cfg.HasSecret = oauthExists
		}
	}
	return cfg, nil
}

// ErrInvalidConfig is returned by SetConfig when the request fails validation
// (missing site URL, bad auth method, etc.). Callers map it to HTTP 400.
var ErrInvalidConfig = errors.New("jira: invalid configuration")

// SetConfig is upsert: it writes the singleton row and, when a new secret is
// provided, stores it in the encrypted secret store. An empty Secret means
// "keep the existing token" — this lets the UI edit auxiliary fields without
// forcing the user to paste the token again.
func (s *Service) SetConfig(ctx context.Context, req *SetConfigRequest) (*JiraConfig, error) {
	workspaceID, err := s.defaultWorkspaceID()
	if err != nil {
		return nil, err
	}
	return s.SetConfigForWorkspace(ctx, workspaceID, req)
}

// SetConfigForWorkspace is upsert for one workspace.
func (s *Service) SetConfigForWorkspace(ctx context.Context, workspaceID string, req *SetConfigRequest) (*JiraConfig, error) {
	workspaceID, err := s.resolveWorkspaceID(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	if err := validateConfigRequest(req); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrInvalidConfig, err.Error())
	}
	cfg := &JiraConfig{
		SiteURL:           normalizeSiteURL(req.SiteURL),
		Email:             req.Email,
		AuthMethod:        req.AuthMethod,
		InstanceType:      req.InstanceType,
		DefaultProjectKey: req.DefaultProjectKey,
		ClientID:          req.ClientID,
	}
	// Preserve cloudId for OAuth — it's resolved during the OAuth flow and
	// shouldn't be overwritten by a regular config save.
	if req.AuthMethod == AuthMethodOAuth {
		existing, err := s.store.GetConfigForWorkspace(ctx, workspaceID)
		if err != nil {
			return nil, fmt.Errorf("read existing config: %w", err)
		}
		if existing != nil {
			cfg.CloudID = existing.CloudID
			cfg.TokenExpiresAt = existing.TokenExpiresAt
		}
	}
	if err := s.store.UpsertConfigForWorkspace(ctx, workspaceID, cfg); err != nil {
		return nil, fmt.Errorf("upsert jira config: %w", err)
	}
	if req.Secret != "" && s.secrets != nil {
		if err := s.secrets.Set(ctx, SecretKeyForWorkspace(workspaceID), "Jira token", req.Secret); err != nil {
			return nil, fmt.Errorf("store jira secret: %w", err)
		}
	}
	// OAuth: client_id is stored in the config row (UpsertConfigForWorkspace
	// above). No client_secret — the flow uses PKCE with
	// token_endpoint_auth_method="none".
	s.invalidateClient(workspaceID)
	// Probe asynchronously so a slow Atlassian doesn't stall the save response.
	// RecordAuthHealth manages its own probe timeout, so a fresh
	// context.Background() is enough — the request ctx may be cancelled when
	// this returns, but the probe and the DB write must still complete.
	go func() {
		s.RecordAuthHealthForWorkspace(context.Background(), workspaceID)
	}()
	return s.GetConfigForWorkspace(ctx, workspaceID)
}

// DeleteConfig removes both the config row and the stored secret. A failure to
// delete the secret is logged (not returned) so the config row deletion still
// surfaces success — the orphaned secret is rare and the user can retry by
// re-saving + deleting again.
func (s *Service) DeleteConfig(ctx context.Context) error {
	workspaceID, err := s.defaultWorkspaceID()
	if err != nil {
		return err
	}
	return s.DeleteConfigForWorkspace(ctx, workspaceID)
}

// DeleteConfigForWorkspace removes both the config row and the stored secret.
func (s *Service) DeleteConfigForWorkspace(ctx context.Context, workspaceID string) error {
	workspaceID, err := s.resolveWorkspaceID(ctx, workspaceID)
	if err != nil {
		return err
	}
	if err := s.store.DeleteConfigForWorkspace(ctx, workspaceID); err != nil {
		return err
	}
	if s.secrets != nil {
		if err := s.secrets.Delete(ctx, SecretKeyForWorkspace(workspaceID)); err != nil {
			s.log.Warn("jira: secret delete failed", zap.Error(err))
		}
		// Clean up OAuth secrets if they exist. Log failures so a stale
		// refresh token left in the store after config removal is visible.
		if err := s.secrets.Delete(ctx, OAuthAccessTokenKeyForWorkspace(workspaceID)); err != nil {
			s.log.Warn("jira: oauth access token delete failed", zap.Error(err))
		}
		if err := s.secrets.Delete(ctx, OAuthRefreshTokenKeyForWorkspace(workspaceID)); err != nil {
			s.log.Warn("jira: oauth refresh token delete failed", zap.Error(err))
		}
	}
	s.invalidateClient(workspaceID)
	return nil
}

// TestConnection validates credentials either from a fresh SetConfigRequest
// (before persisting) or from the stored config (after saving). Returns a
// structured result rather than an error so the UI can render the failure
// inline.
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
	// Authorize before revealing/using the workspace's stored token: an omitted
	// workspace_id resolves the global default, and a caller-supplied siteUrl in
	// req would otherwise exfiltrate that workspace's token to an attacker host.
	// Return a hard error (not a swallowed {OK:false} result) so the denial is a
	// 404 rather than a probeable "test failed".
	if err := s.authorizeWorkspaceAccess(ctx, workspaceID); err != nil {
		return nil, err
	}
	cfg, secret, err := s.resolveCredentials(ctx, workspaceID, req)
	if err != nil {
		return &TestConnectionResult{OK: false, Error: err.Error()}, nil
	}
	// Set WorkspaceID on the config so MCPClient/SetRefresher get the right ID.
	cfg.WorkspaceID = workspaceID
	client := s.clientFn(cfg, secret)
	// Wire the OAuth token refresher for test-connection clients too,
	// so an expired access token is auto-refreshed during testing.
	if cfg.AuthMethod == AuthMethodOAuth {
		if mc, ok := client.(*MCPClient); ok {
			mc.SetRefresher(workspaceID, s)
		}
	}
	return client.TestAuth(ctx)
}

// ProbeAuth validates the stored credentials by hitting the cheapest
// authenticated endpoint. Used by the background auth-health poller to detect
// expired session cookies / step-up auth before the user clicks through to a
// tab that would 303. Errors are returned as a result so the poller can
// persist them as a failure rather than dropping them.
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

// Store exposes the underlying store so background workers (e.g. the auth
// health poller) can persist state without re-implementing the secrets+config
// resolution that the service already encapsulates.
func (s *Service) Store() *Store {
	return s.store
}

// authProbeTimeout caps a single auth-health probe so a slow Atlassian can't
// stall the caller. The /myself endpoint typically responds in <500ms.
const authProbeTimeout = 15 * time.Second

// authHealthWriteTimeout bounds the DB write that persists the probe outcome.
// Always derived from a fresh context.Background so a probe that exhausted its
// own deadline can still record the failure.
const authHealthWriteTimeout = 5 * time.Second

// SetProbeHook installs a callback fired at the end of each RecordAuthHealth
// call (after the DB write). Production code never sets this; tests use it to
// synchronize on probe completion without sleep-polling.
func (s *Service) SetProbeHook(fn func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.probeHook = fn
}

// RecordAuthHealth probes the stored credentials and writes the outcome onto
// the JiraConfig row. Used both at config-save time (so the UI reflects the
// new credentials immediately) and by the background poller. Errors during
// the persist step are logged but not returned: this is a best-effort health
// signal, never the source of truth for callers.
func (s *Service) RecordAuthHealth(ctx context.Context) {
	workspaceIDs, err := s.store.ListConfigWorkspaceIDs(ctx)
	if err != nil {
		s.log.Warn("jira: list config workspaces failed", zap.Error(err))
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
		s.log.Warn("jira: resolve workspace for auth health failed", zap.Error(normalizeErr))
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
	// Detach the DB write from ctx: a probe that exhausted its 15s deadline
	// would otherwise pass an already-expired context to UpdateAuthHealth and
	// drop the failure record on the floor (so the UI never flips to "auth
	// failed" until the next poll).
	writeCtx, writeCancel := context.WithTimeout(context.Background(), authHealthWriteTimeout)
	defer writeCancel()
	if updateErr := s.store.UpdateAuthHealthForWorkspace(writeCtx, workspaceID, ok, errMsg, time.Now().UTC()); updateErr != nil {
		s.log.Warn("jira: update auth health failed", zap.Error(updateErr))
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

// GetTicket loads a Jira ticket by key using the stored credentials.
func (s *Service) GetTicket(ctx context.Context, ticketKey string) (*JiraTicket, error) {
	workspaceID, err := s.defaultWorkspaceID()
	if err != nil {
		return nil, err
	}
	client, err := s.clientFor(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	return client.GetTicket(ctx, ticketKey)
}

// DoTransition applies a transition to a ticket.
func (s *Service) DoTransition(ctx context.Context, ticketKey, transitionID string) error {
	workspaceID, err := s.defaultWorkspaceID()
	if err != nil {
		return err
	}
	client, err := s.clientFor(ctx, workspaceID)
	if err != nil {
		return err
	}
	return client.DoTransition(ctx, ticketKey, transitionID)
}

// ListProjects is used by the settings UI to populate the project selector.
func (s *Service) ListProjects(ctx context.Context) ([]JiraProject, error) {
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

// ListProjectStatuses returns the workflow statuses defined for a project,
// used by the ticket-list status filter so users can filter by the real
// statuses in the selected project rather than the three coarse categories.
func (s *Service) ListProjectStatuses(ctx context.Context, projectKey string) ([]JiraStatus, error) {
	workspaceID, err := s.defaultWorkspaceID()
	if err != nil {
		return nil, err
	}
	return s.ListProjectStatusesForWorkspace(ctx, workspaceID, projectKey)
}

func (s *Service) ListProjectStatusesForWorkspace(ctx context.Context, workspaceID, projectKey string) ([]JiraStatus, error) {
	workspaceID, err := s.normalizeWorkspaceID(workspaceID)
	if err != nil {
		return nil, err
	}
	client, err := s.clientFor(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	return client.ListProjectStatuses(ctx, projectKey)
}

// SearchTickets runs a JQL search, returning a page of tickets. pageToken is
// the cursor returned in the previous page's NextPageToken; pass "" for the
// first page.
func (s *Service) SearchTickets(ctx context.Context, jql, pageToken string, maxResults int) (*SearchResult, error) {
	workspaceID, err := s.defaultWorkspaceID()
	if err != nil {
		return nil, err
	}
	return s.SearchTicketsForWorkspace(ctx, workspaceID, jql, pageToken, maxResults)
}

// SearchTicketsForWorkspace runs a JQL search against one workspace's client.
func (s *Service) SearchTicketsForWorkspace(ctx context.Context, workspaceID, jql, pageToken string, maxResults int) (*SearchResult, error) {
	workspaceID, err := s.normalizeWorkspaceID(workspaceID)
	if err != nil {
		return nil, err
	}
	client, err := s.clientFor(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	return client.SearchTickets(ctx, jql, pageToken, maxResults)
}

// clientFor returns the cached client, creating it if needed. The cache is
// invalidated whenever the config changes so stale credentials never linger.
// It is the single chokepoint for every data-plane read (projects, statuses,
// tickets, transitions), so authorizing the resolved workspace here scopes all
// of them — an omitted workspace_id must not resolve another user's default.
func (s *Service) clientFor(ctx context.Context, workspaceID string) (Client, error) {
	workspaceID, err := s.resolveWorkspaceID(ctx, workspaceID)
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
		if cfg.AuthMethod == AuthMethodOAuth {
			secret, err = s.secrets.Reveal(ctx, OAuthAccessTokenKeyForWorkspace(workspaceID))
		} else {
			secret, err = s.revealSecret(ctx, workspaceID)
		}
		if err != nil {
			// Don't conflate a transient secret-store failure with "user never
			// configured Jira". The UI gates on ErrNotConfigured to render a
			// configure-CTA; surfacing the real error gives the user a path to
			// retry rather than leaving them stuck.
			return nil, fmt.Errorf("read jira secret: %w", err)
		}
		if secret == "" {
			return nil, ErrNotConfigured
		}
	}
	client := s.clientFn(cfg, secret)
	// Wire the OAuth token refresher so clients can auto-refresh on 401.
	// This covers both the cached path (clientFor) and the test-connection
	// path (resolveCredentials → clientFn) — both create a fresh client that
	// needs the refresher wired.
	if cfg.AuthMethod == AuthMethodOAuth {
		if cc, ok := client.(*CloudClient); ok {
			cc.SetRefresher(workspaceID, s)
		}
		if mc, ok := client.(*MCPClient); ok {
			mc.SetRefresher(workspaceID, s)
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	// Re-check: another caller may have populated the cache while we were
	// fetching the config and secret. Returning the existing client keeps the
	// cache identity stable so callers comparing pointers don't see flapping.
	if s.clients == nil {
		s.clients = make(map[string]Client)
	}
	if s.clients[workspaceID] != nil {
		return s.clients[workspaceID], nil
	}
	s.clients[workspaceID] = client
	return client, nil
}

// invalidateClient drops the cached client so the next request rebuilds it
// with fresh credentials.
func (s *Service) invalidateClient(workspaceID string) {
	s.mu.Lock()
	if s.clients != nil {
		delete(s.clients, workspaceID)
	}
	s.mu.Unlock()
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
	if !s.legacySecretAppliesTo(workspaceID) {
		return false, nil
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
	if !s.legacySecretAppliesTo(workspaceID) {
		return "", err
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

func (s *Service) legacySecretAppliesTo(workspaceID string) bool {
	target := s.store.MigratedFromWorkspace()
	return target != "" && workspaceID == target
}

func validateConfigRequest(req *SetConfigRequest) error {
	if req.SiteURL == "" {
		return errors.New("siteUrl required")
	}
	// Require an explicit instanceType — defaulting to cloud here would let a
	// stale client (one that pre-dates Server/DC support) silently downgrade
	// a saved Server config back to Cloud on the next partial save.
	switch req.InstanceType {
	case InstanceTypeCloud, InstanceTypeServer:
	case "":
		return errors.New("instanceType required (cloud or server)")
	default:
		return fmt.Errorf("unknown instance type: %q", req.InstanceType)
	}
	if err := validateAuthInstance(req.AuthMethod, req.InstanceType, req.Email); err != nil {
		return err
	}
	// OAuth requires a client_id at config-save time. The client_secret is
	// stored during the OAuth connect flow, not here.
	if req.AuthMethod == AuthMethodOAuth && req.ClientID == "" {
		return errors.New("clientId required for oauth auth")
	}
	// OAuth client_id is populated by the dynamic registration flow, not
	// by the user — so we don't validate it here for non-oauth methods.
	return nil
}

// validateAuthInstance enforces the supported (auth method, instance type)
// combinations. Shared by config-save and the test-connection flow so a
// pre-save test surfaces the same diagnostic the eventual save would, instead
// of letting Jira return an opaque 401 for an unsupported combo.
func validateAuthInstance(authMethod, instanceType, email string) error {
	switch authMethod {
	case AuthMethodAPIToken:
		// API tokens are Cloud-only — Atlassian Server/DC rejects Basic auth
		// with `id.atlassian.com` tokens and redirects to a login page, which
		// makes the failure mode opaque. Catch it at config time.
		if instanceType != InstanceTypeCloud {
			return errors.New("api_token auth is supported only on Atlassian Cloud — use pat for Server/DC")
		}
		if email == "" {
			return errors.New("email required for api_token auth")
		}
	case AuthMethodPAT:
		// Personal Access Tokens are a Server/DC concept; Cloud expects
		// id.atlassian.com tokens via Basic, not Bearer.
		if instanceType != InstanceTypeServer {
			return errors.New("pat auth is supported only on Jira Server/Data Center — use api_token for Cloud")
		}
	case AuthMethodSessionCookie:
		// The client wraps the session secret under cloud.session.token and
		// tenant.session.token — both Atlassian-Cloud-specific cookie names.
		// Server/DC uses JSESSIONID, so the current wrapping is a no-op there;
		// reject the combo until we add a Server-aware session-cookie path.
		if instanceType != InstanceTypeCloud {
			return errors.New("session_cookie auth is supported only on Atlassian Cloud — use pat for Server/DC")
		}
	case AuthMethodOAuth:
		// OAuth 2.0 (3LO) is an Atlassian Cloud feature. The authorization,
		// token, and accessible-resources endpoints are Cloud-only.
		if instanceType != InstanceTypeCloud {
			return errors.New("oauth auth is supported only on Atlassian Cloud — use pat for Server/DC")
		}
	default:
		return fmt.Errorf("unknown auth method: %q", authMethod)
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

// SetEventBus wires the bus used to publish NewJiraIssueEvent. Optional: if
// unset the poller still runs but observed tickets do not become Kandev tasks
// — useful in tests that exercise the polling loop in isolation.
func (s *Service) SetEventBus(eb bus.EventBus) {
	s.mu.Lock()
	s.eventBus = eb
	s.mu.Unlock()
}

// --- Issue watch CRUD (thin pass-throughs to the store) ---

// ErrIssueWatchNotFound is returned when GetIssueWatch's caller looks up an ID
// that doesn't exist. Callers map this to HTTP 404.

// MinIssueWatchPollInterval / MaxIssueWatchPollInterval bound the per-watch
// JQL re-run cadence. The lower limit protects Atlassian from rapid-fire
// polling; the upper limit keeps "stuck" rows from polling so rarely they
// look broken to the user.
const (
	MinIssueWatchPollInterval = 60
	MaxIssueWatchPollInterval = 3600
)

// resolveRepositoryBinding validates the watch's optional repository binding
// against its workspace and fills an empty base branch with the repository's
// default branch. An empty repositoryID clears the binding (and forces an empty
// base branch), preserving the historical repo-less behaviour. When no
// RepositoryLookup is wired (unit tests, early boot), the binding is accepted
// as-is and the default-branch fill is skipped.
func (s *Service) resolveRepositoryBinding(ctx context.Context, workspaceID, repositoryID, baseBranch string) (string, string, error) {
	repositoryID = strings.TrimSpace(repositoryID)
	baseBranch = strings.TrimSpace(baseBranch)
	if repositoryID == "" {
		return "", "", nil
	}
	// Reject a non-empty base branch that isn't a safe git ref before it can be
	// persisted and copied into watcher-created tasks (then fail at worktree
	// launch). Empty defers to the repo's default branch below.
	if baseBranch != "" && !securityutil.IsValidBaseBranchRef(baseBranch) {
		return "", "", fmt.Errorf("%w: base branch %q is not a valid git ref", ErrInvalidConfig, baseBranch)
	}
	rl := s.getRepositoryLookup()
	if rl == nil {
		return repositoryID, baseBranch, nil
	}
	repoWorkspace, defaultBranch, ok := rl.GetRepository(ctx, repositoryID)
	if !ok {
		return "", "", fmt.Errorf("%w: repository %q not found", ErrInvalidConfig, repositoryID)
	}
	if repoWorkspace != workspaceID {
		return "", "", fmt.Errorf("%w: repository %q does not belong to this workspace", ErrInvalidConfig, repositoryID)
	}
	if baseBranch == "" {
		baseBranch = defaultBranch
	}
	return repositoryID, baseBranch, nil
}

func validateIssueWatchCreate(req *CreateIssueWatchRequest) error {
	if req.WorkspaceID == "" {
		return fmt.Errorf("%w: workspaceId required", ErrInvalidConfig)
	}
	if req.WorkflowID == "" || req.WorkflowStepID == "" {
		return fmt.Errorf("%w: workflowId and workflowStepId required", ErrInvalidConfig)
	}
	if strings.TrimSpace(req.JQL) == "" {
		return fmt.Errorf("%w: jql required", ErrInvalidConfig)
	}
	// 0 / unset is allowed — the store coerces to DefaultIssueWatchPollInterval.
	// Any positive value must fall inside the documented bounds.
	if req.PollIntervalSeconds != 0 {
		if err := validatePollInterval(req.PollIntervalSeconds); err != nil {
			return err
		}
	}
	if err := validateMaxInflightTasks(req.MaxInflightTasks); err != nil {
		return err
	}
	return nil
}

// validateMaxInflightTasks rejects non-positive caps. A nil pointer is
// "uncapped" and explicitly allowed.
func validateMaxInflightTasks(v *int) error {
	if v == nil {
		return nil
	}
	if *v <= 0 {
		return fmt.Errorf("%w: maxInflightTasks must be a positive integer", ErrInvalidConfig)
	}
	return nil
}

func validatePollInterval(seconds int) error {
	if seconds < MinIssueWatchPollInterval || seconds > MaxIssueWatchPollInterval {
		return fmt.Errorf("%w: pollIntervalSeconds must be between %d and %d",
			ErrInvalidConfig, MinIssueWatchPollInterval, MaxIssueWatchPollInterval)
	}
	return nil
}

func applyIssueWatchPatch(w *IssueWatch, req *UpdateIssueWatchRequest) {
	if req.WorkflowID != nil {
		w.WorkflowID = *req.WorkflowID
	}
	if req.WorkflowStepID != nil {
		w.WorkflowStepID = *req.WorkflowStepID
	}
	// RepositoryID / BaseBranch are applied here; UpdateIssueWatch then runs them
	// through resolveRepositoryBinding (workspace check + default-branch fill, or
	// clear when empty). An empty RepositoryID unbinds the watch. Switching to a
	// different repository without an explicit base branch resets the branch so
	// the new repo's default is used instead of carrying the old repo's branch.
	if req.RepositoryID != nil {
		if *req.RepositoryID != w.RepositoryID && req.BaseBranch == nil {
			w.BaseBranch = ""
		}
		w.RepositoryID = *req.RepositoryID
	}
	if req.BaseBranch != nil {
		w.BaseBranch = *req.BaseBranch
	}
	if req.JQL != nil {
		w.JQL = strings.TrimSpace(*req.JQL)
	}
	if req.AgentProfileID != nil {
		w.AgentProfileID = *req.AgentProfileID
	}
	if req.ExecutorProfileID != nil {
		w.ExecutorProfileID = *req.ExecutorProfileID
	}
	if req.Prompt != nil {
		w.Prompt = *req.Prompt
	}
	if req.Enabled != nil {
		w.Enabled = *req.Enabled
	}
	if req.PollIntervalSeconds != nil {
		w.PollIntervalSeconds = *req.PollIntervalSeconds
	}
	// MaxInflightTasks is tri-state (optional.Int): only apply it when the
	// PATCH actually carried the key, so a partial update that omits it
	// leaves the cap intact. Present+null clears the cap; present+int sets it.
	if req.MaxInflightTasks.Present {
		w.MaxInflightTasks = req.MaxInflightTasks.Value
	}
}
