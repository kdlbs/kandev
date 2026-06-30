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
	"github.com/kandev/kandev/internal/events"
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
	client    Client // singleton, cleared on config change.
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
	}
}

// GetConfig returns the singleton config enriched with a HasSecret flag so the
// UI can distinguish "configured but empty" from "needs credentials". For
// session_cookie auth, it also tries to decode the JWT in the stored cookie
// and surface its expiry so the UI can warn the user before the session dies.
func (s *Service) GetConfig(ctx context.Context) (*JiraConfig, error) {
	cfg, err := s.store.GetConfig(ctx)
	if err != nil || cfg == nil {
		return cfg, err
	}
	if s.secrets == nil {
		return cfg, nil
	}
	exists, existsErr := s.secrets.Exists(ctx, SecretKey)
	if existsErr != nil {
		s.log.Warn("jira: secret exists check failed", zap.Error(existsErr))
	}
	cfg.HasSecret = exists
	if exists && cfg.AuthMethod == AuthMethodSessionCookie {
		secret, revealErr := s.secrets.Reveal(ctx, SecretKey)
		if revealErr != nil {
			s.log.Warn("jira: secret reveal failed", zap.Error(revealErr))
		} else {
			cfg.SecretExpiresAt = parseSessionCookieExpiry(secret)
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
	if err := validateConfigRequest(req); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrInvalidConfig, err.Error())
	}
	cfg := &JiraConfig{
		SiteURL:           normalizeSiteURL(req.SiteURL),
		Email:             req.Email,
		AuthMethod:        req.AuthMethod,
		InstanceType:      req.InstanceType,
		DefaultProjectKey: req.DefaultProjectKey,
	}
	if err := s.store.UpsertConfig(ctx, cfg); err != nil {
		return nil, fmt.Errorf("upsert jira config: %w", err)
	}
	if req.Secret != "" && s.secrets != nil {
		if err := s.secrets.Set(ctx, SecretKey, "Jira token", req.Secret); err != nil {
			return nil, fmt.Errorf("store jira secret: %w", err)
		}
	}
	s.invalidateClient()
	// Probe asynchronously so a slow Atlassian doesn't stall the save response.
	// RecordAuthHealth manages its own probe timeout, so a fresh
	// context.Background() is enough — the request ctx may be cancelled when
	// this returns, but the probe and the DB write must still complete.
	go func() {
		s.RecordAuthHealth(context.Background())
	}()
	return s.GetConfig(ctx)
}

// DeleteConfig removes both the config row and the stored secret. A failure to
// delete the secret is logged (not returned) so the config row deletion still
// surfaces success — the orphaned secret is rare and the user can retry by
// re-saving + deleting again.
func (s *Service) DeleteConfig(ctx context.Context) error {
	if err := s.store.DeleteConfig(ctx); err != nil {
		return err
	}
	if s.secrets != nil {
		if err := s.secrets.Delete(ctx, SecretKey); err != nil {
			s.log.Warn("jira: secret delete failed", zap.Error(err))
		}
	}
	s.invalidateClient()
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

// ProbeAuth validates the stored credentials by hitting the cheapest
// authenticated endpoint. Used by the background auth-health poller to detect
// expired session cookies / step-up auth before the user clicks through to a
// tab that would 303. Errors are returned as a result so the poller can
// persist them as a failure rather than dropping them.
func (s *Service) ProbeAuth(ctx context.Context) (*TestConnectionResult, error) {
	client, err := s.clientFor(ctx)
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
	// Detach the DB write from ctx: a probe that exhausted its 15s deadline
	// would otherwise pass an already-expired context to UpdateAuthHealth and
	// drop the failure record on the floor (so the UI never flips to "auth
	// failed" until the next poll).
	writeCtx, writeCancel := context.WithTimeout(context.Background(), authHealthWriteTimeout)
	defer writeCancel()
	if updateErr := s.store.UpdateAuthHealth(writeCtx, ok, errMsg, time.Now().UTC()); updateErr != nil {
		s.log.Warn("jira: update auth health failed", zap.Error(updateErr))
	}
	s.mu.Lock()
	hook := s.probeHook
	s.mu.Unlock()
	if hook != nil {
		hook()
	}
}

// GetTicket loads a Jira ticket by key using the stored credentials.
func (s *Service) GetTicket(ctx context.Context, ticketKey string) (*JiraTicket, error) {
	client, err := s.clientFor(ctx)
	if err != nil {
		return nil, err
	}
	return client.GetTicket(ctx, ticketKey)
}

// DoTransition applies a transition to a ticket.
func (s *Service) DoTransition(ctx context.Context, ticketKey, transitionID string) error {
	client, err := s.clientFor(ctx)
	if err != nil {
		return err
	}
	return client.DoTransition(ctx, ticketKey, transitionID)
}

// ListProjects is used by the settings UI to populate the project selector.
func (s *Service) ListProjects(ctx context.Context) ([]JiraProject, error) {
	client, err := s.clientFor(ctx)
	if err != nil {
		return nil, err
	}
	return client.ListProjects(ctx)
}

// SearchTickets runs a JQL search, returning a page of tickets. pageToken is
// the cursor returned in the previous page's NextPageToken; pass "" for the
// first page.
func (s *Service) SearchTickets(ctx context.Context, jql, pageToken string, maxResults int) (*SearchResult, error) {
	client, err := s.clientFor(ctx)
	if err != nil {
		return nil, err
	}
	return client.SearchTickets(ctx, jql, pageToken, maxResults)
}

// clientFor returns the cached client, creating it if needed. The cache is
// invalidated whenever the config changes so stale credentials never linger.
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
	s.mu.Lock()
	defer s.mu.Unlock()
	// Re-check: another caller may have populated the cache while we were
	// fetching the config and secret. Returning the existing client keeps the
	// cache identity stable so callers comparing pointers don't see flapping.
	if s.client != nil {
		return s.client, nil
	}
	s.client = client
	return client, nil
}

// invalidateClient drops the cached client so the next request rebuilds it
// with fresh credentials.
func (s *Service) invalidateClient() {
	s.mu.Lock()
	s.client = nil
	s.mu.Unlock()
}

// resolveCredentials picks the credentials to test: if the request carries a
// secret, use it inline (pre-save); otherwise fall back to the stored secret
// (post-save re-test).
func (s *Service) resolveCredentials(ctx context.Context, req *SetConfigRequest) (*JiraConfig, string, error) {
	cfg := &JiraConfig{
		SiteURL:      normalizeSiteURL(req.SiteURL),
		Email:        req.Email,
		AuthMethod:   req.AuthMethod,
		InstanceType: req.InstanceType,
	}
	var secret string
	if req.Secret != "" {
		secret = req.Secret
	} else {
		if s.secrets == nil {
			return nil, "", errors.New("no secret store configured")
		}
		var err error
		secret, err = s.secrets.Reveal(ctx, SecretKey)
		if err != nil {
			// Don't conflate a transient secret-store failure with "no token
			// stored". Surfacing the real error gives the user a path to retry
			// instead of telling them to paste a token they already have.
			s.log.Warn("jira: secret reveal failed", zap.Error(err))
			return nil, "", fmt.Errorf("read jira secret: %w", err)
		}
		if secret == "" {
			return nil, "", errors.New("no token stored — paste one to test")
		}
	}
	// Merge with persisted config only when the caller didn't pass enough on
	// its own. A fully-specified pre-save TestConnection request needs no DB
	// read — keeping the call gated avoids a transient DB error blocking auth
	// testing for a request that doesn't depend on persisted state. When a
	// read does happen, surface its error so a real DB failure isn't masked
	// as an opaque "missing field" validation message later on.
	needsStoredConfig := cfg.SiteURL == "" ||
		cfg.AuthMethod == "" ||
		cfg.InstanceType == "" ||
		(cfg.AuthMethod == AuthMethodAPIToken && cfg.Email == "")
	if needsStoredConfig {
		if err := s.fillConfigFromStored(ctx, cfg); err != nil {
			return nil, "", err
		}
	}
	if cfg.InstanceType == "" {
		cfg.InstanceType = InstanceTypeCloud
	}
	// Run the same auth/instance check the save path runs, so TestConnection
	// reports invalid combos (e.g. pat+cloud, api_token+server) with a clear
	// validation error instead of letting Jira return an opaque 401.
	if err := validateAuthInstance(cfg.AuthMethod, cfg.InstanceType, cfg.Email); err != nil {
		return nil, "", err
	}
	return cfg, secret, nil
}

// fillConfigFromStored reads the persisted singleton config and copies any
// fields the caller left blank onto cfg. A real read failure is returned so
// callers don't mask DB errors as opaque "missing field" validation errors;
// a missing row (stored == nil) is treated as "nothing to fill" rather than
// an error so first-time TestConnection requests still go through.
func (s *Service) fillConfigFromStored(ctx context.Context, cfg *JiraConfig) error {
	stored, err := s.store.GetConfig(ctx)
	if err != nil {
		return fmt.Errorf("read jira config: %w", err)
	}
	if stored == nil {
		return nil
	}
	if cfg.SiteURL == "" {
		cfg.SiteURL = stored.SiteURL
	}
	if cfg.Email == "" {
		cfg.Email = stored.Email
	}
	if cfg.AuthMethod == "" {
		cfg.AuthMethod = stored.AuthMethod
	}
	if cfg.InstanceType == "" {
		cfg.InstanceType = stored.InstanceType
	}
	return nil
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
	return validateAuthInstance(req.AuthMethod, req.InstanceType, req.Email)
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
var ErrIssueWatchNotFound = errors.New("jira: issue watch not found")

// CreateIssueWatch validates the request and persists a new watch row.
func (s *Service) CreateIssueWatch(ctx context.Context, req *CreateIssueWatchRequest) (*IssueWatch, error) {
	if err := validateIssueWatchCreate(req); err != nil {
		return nil, err
	}
	repositoryID, baseBranch, err := s.resolveRepositoryBinding(ctx, req.WorkspaceID, req.RepositoryID, req.BaseBranch)
	if err != nil {
		return nil, err
	}
	w := &IssueWatch{
		WorkspaceID:         req.WorkspaceID,
		WorkflowID:          req.WorkflowID,
		WorkflowStepID:      req.WorkflowStepID,
		RepositoryID:        repositoryID,
		BaseBranch:          baseBranch,
		JQL:                 strings.TrimSpace(req.JQL),
		AgentProfileID:      req.AgentProfileID,
		ExecutorProfileID:   req.ExecutorProfileID,
		Prompt:              req.Prompt,
		PollIntervalSeconds: req.PollIntervalSeconds,
		MaxInflightTasks:    req.MaxInflightTasks,
		Enabled:             true,
	}
	if req.Enabled != nil {
		w.Enabled = *req.Enabled
	}
	if err := s.store.CreateIssueWatch(ctx, w); err != nil {
		return nil, err
	}
	return w, nil
}

// ListIssueWatches returns the watches configured for a workspace.
func (s *Service) ListIssueWatches(ctx context.Context, workspaceID string) ([]*IssueWatch, error) {
	return s.store.ListIssueWatches(ctx, workspaceID)
}

// ListAllIssueWatches returns every watch across all workspaces.
func (s *Service) ListAllIssueWatches(ctx context.Context) ([]*IssueWatch, error) {
	return s.store.ListAllIssueWatches(ctx)
}

// GetIssueWatch returns a single watch by ID or ErrIssueWatchNotFound.
func (s *Service) GetIssueWatch(ctx context.Context, id string) (*IssueWatch, error) {
	w, err := s.store.GetIssueWatch(ctx, id)
	if err != nil {
		return nil, err
	}
	if w == nil {
		return nil, ErrIssueWatchNotFound
	}
	return w, nil
}

// UpdateIssueWatch applies a partial update by patching only the fields the
// caller explicitly set, then persists the result.
func (s *Service) UpdateIssueWatch(ctx context.Context, id string, req *UpdateIssueWatchRequest) (*IssueWatch, error) {
	w, err := s.GetIssueWatch(ctx, id)
	if err != nil {
		return nil, err
	}
	prevRepositoryID, prevBaseBranch := w.RepositoryID, w.BaseBranch
	applyIssueWatchPatch(w, req)
	// Empty-string PATCH writes (`{"workflowId": ""}` etc.) bypass the nil
	// guard in applyIssueWatchPatch — Go's JSON decoder returns a non-nil
	// pointer to "". Guard the post-patch state so a PATCH can't strip a row
	// of the fields the orchestrator needs to create tasks.
	if w.JQL == "" {
		return nil, fmt.Errorf("%w: jql cannot be empty", ErrInvalidConfig)
	}
	if w.WorkflowID == "" || w.WorkflowStepID == "" {
		return nil, fmt.Errorf("%w: workflowId and workflowStepId cannot be empty", ErrInvalidConfig)
	}
	if err := validatePollInterval(w.PollIntervalSeconds); err != nil {
		return nil, err
	}
	if err := validateMaxInflightTasks(w.MaxInflightTasks); err != nil {
		return nil, err
	}
	// Only validate/resolve the binding when its value actually changed. The
	// dialog re-sends repositoryId/baseBranch on every PATCH, and an unchanged
	// binding whose repo was since soft-deleted must not block edits to other
	// fields (prompt, JQL, …).
	if w.RepositoryID != prevRepositoryID || w.BaseBranch != prevBaseBranch {
		repositoryID, baseBranch, err := s.resolveRepositoryBinding(ctx, w.WorkspaceID, w.RepositoryID, w.BaseBranch)
		if err != nil {
			return nil, err
		}
		w.RepositoryID = repositoryID
		w.BaseBranch = baseBranch
	}
	if err := s.store.UpdateIssueWatch(ctx, w); err != nil {
		return nil, err
	}
	return w, nil
}

// DeleteIssueWatch removes the watch and its dedup rows. Idempotent: deleting
// a missing ID is a silent success.
func (s *Service) DeleteIssueWatch(ctx context.Context, id string) error {
	return s.store.DeleteIssueWatch(ctx, id)
}

// jiraIssueWatchResetter is the watchreset.Resetter adapter for a single
// Jira issue watch. Closes over the store and watch ID so the shared
// watchreset.Run helper stays integration-agnostic.
type jiraIssueWatchResetter struct {
	store   *Store
	watchID string
}

func (r *jiraIssueWatchResetter) ListTaskIDs(ctx context.Context) ([]string, error) {
	return r.store.ListIssueWatchTaskIDs(ctx, r.watchID)
}

func (r *jiraIssueWatchResetter) Clear(ctx context.Context) error {
	return r.store.ResetIssueWatchState(ctx, r.watchID)
}

// PreviewResetIssueWatch returns how many tasks ResetIssueWatch would
// cascade-delete. Used by the frontend to populate the confirmation dialog.
func (s *Service) PreviewResetIssueWatch(ctx context.Context, watchID string) (int, error) {
	return watchreset.Preview(ctx, &jiraIssueWatchResetter{store: s.store, watchID: watchID})
}

// ResetIssueWatch is destructive: cascade-deletes every task previously
// created by the watch (including archived), wipes the per-watch dedup
// rows, and nulls last_polled_at so the next poll re-imports every
// currently-matching ticket. Returns the count of tasks deleted.
func (s *Service) ResetIssueWatch(ctx context.Context, watchID string) (int, error) {
	s.mu.Lock()
	td := s.taskDeleter
	s.mu.Unlock()
	if td == nil {
		return 0, errors.New("jira: task deleter not wired; reset unavailable")
	}
	res, err := watchreset.Run(ctx,
		&jiraIssueWatchResetter{store: s.store, watchID: watchID},
		td, s.log)
	return res.TasksDeleted, err
}

// CheckIssueWatch runs the watch's JQL once and returns the tickets that
// haven't been turned into tasks yet. last_polled_at is stamped regardless of
// whether the search succeeded — a failing search still counts as "we tried"
// so the UI can show liveness.
func (s *Service) CheckIssueWatch(ctx context.Context, w *IssueWatch) ([]*JiraTicket, error) {
	defer s.stampLastPolled(w.ID)
	client, err := s.clientFor(ctx)
	if err != nil {
		return nil, err
	}
	// Single page is enough per tick: the next poll picks up anything that
	// overflowed maxResults. Pagination would let one chatty workspace starve
	// the others without any real freshness benefit.
	res, err := client.SearchTickets(ctx, w.JQL, "", issueWatchSearchPageSize)
	if err != nil {
		return nil, err
	}
	// Bulk-fetch the dedup set once per call instead of one query per ticket —
	// a broad JQL can return up to issueWatchSearchPageSize tickets, so a
	// per-ticket round-trip would multiply the per-tick cost.
	seen, err := s.store.ListSeenIssueKeys(ctx, w.ID)
	if err != nil {
		s.log.Warn("jira: dedup set fetch failed",
			zap.String("watch_id", w.ID), zap.Error(err))
		seen = nil // fall through: a missing dedup set is safer than dropping all tickets
	}
	out := make([]*JiraTicket, 0, len(res.Tickets))
	for i := range res.Tickets {
		t := res.Tickets[i]
		if _, ok := seen[t.Key]; ok {
			continue
		}
		out = append(out, &t)
	}
	return out, nil
}

// stampLastPolled writes the current timestamp onto the watch row using a
// fresh background context with a short write deadline. The caller's ctx may
// already be cancelled (typical at poller shutdown), which would cause the
// DB write to fail silently and the watch would look "never polled" on the
// next backend restart. Mirrors the pattern in RecordAuthHealth.
func (s *Service) stampLastPolled(watchID string) {
	ctx, cancel := context.WithTimeout(context.Background(), authHealthWriteTimeout)
	defer cancel()
	if err := s.store.UpdateIssueWatchLastPolled(ctx, watchID, time.Now().UTC()); err != nil {
		s.log.Warn("jira: update last_polled_at failed",
			zap.String("watch_id", watchID), zap.Error(err))
	}
}

// publishNewJiraIssueEvent emits the orchestrator-facing event for one freshly
// observed ticket. No-op when the event bus is not wired (tests, early boot).
func (s *Service) publishNewJiraIssueEvent(ctx context.Context, w *IssueWatch, ticket *JiraTicket) {
	s.mu.Lock()
	eb := s.eventBus
	s.mu.Unlock()
	if eb == nil {
		return
	}
	evt := bus.NewEvent(events.JiraNewIssue, "jira", &NewJiraIssueEvent{
		IssueWatchID:      w.ID,
		WorkspaceID:       w.WorkspaceID,
		WorkflowID:        w.WorkflowID,
		WorkflowStepID:    w.WorkflowStepID,
		RepositoryID:      w.RepositoryID,
		BaseBranch:        w.BaseBranch,
		AgentProfileID:    w.AgentProfileID,
		ExecutorProfileID: w.ExecutorProfileID,
		Prompt:            w.Prompt,
		MaxInflightTasks:  w.MaxInflightTasks,
		Issue:             ticket,
	})
	if err := eb.Publish(ctx, events.JiraNewIssue, evt); err != nil {
		s.log.Debug("jira: publish new issue event failed",
			zap.String("watch_id", w.ID), zap.String("issue_key", ticket.Key), zap.Error(err))
	}
}

// issueWatchSearchPageSize caps how many tickets a single CheckIssueWatch call
// pulls from JIRA. 50 is well under the API's 100-result limit and keeps the
// per-tick cost bounded even for very broad JQL queries.
const issueWatchSearchPageSize = 50

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
