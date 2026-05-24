package gitlab

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
)

// secretNameToken is the canonical secret-store name for the GitLab PAT.
const secretNameToken = "GITLAB_TOKEN"

// ErrInvalidToken is returned by ConfigureToken when the supplied token is
// empty or fails the /user probe. The controller layer uses errors.Is to
// route this to HTTP 400 instead of 500 — a sentinel is more durable than
// string-matching against the error message, which would silently break
// the status mapping if the message wording changes.
var ErrInvalidToken = errors.New("invalid token")

// SecretManager handles secret create/update/delete for the token.
type SecretManager interface {
	Create(ctx context.Context, name, value string) (id string, err error)
	Update(ctx context.Context, id, value string) error
	Delete(ctx context.Context, id string) error
}

// HostStore persists the configured GitLab host.
type HostStore interface {
	GetHost(ctx context.Context) (string, error)
	SetHost(ctx context.Context, host string) error
}

// Service coordinates GitLab integration operations. v1 surface is
// deliberately small: status, token configure/clear, host configure, MR
// feedback fetch, and MR discussion reply/resolve. Watches, presets, and
// stats are intentionally deferred to a follow-up.
type Service struct {
	mu            sync.RWMutex
	host          string
	client        Client
	authMethod    string
	secrets       SecretProvider
	secretManager SecretManager
	hostStore     HostStore
	store         *Store
	logger        *logger.Logger
}

// SetStore wires the task↔MR persistence layer. Optional — when nil the
// task-mr endpoints return empty results and SyncTaskMR is a no-op.
func (s *Service) SetStore(store *Store) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.store = store
}

// NewService builds a Service from an already-resolved client. Callers
// typically use Provide() instead of constructing this directly.
func NewService(host string, client Client, authMethod string, secrets SecretProvider, log *logger.Logger) *Service {
	if host == "" {
		host = DefaultHost
	}
	return &Service{
		host:       host,
		client:     client,
		authMethod: authMethod,
		secrets:    secrets,
		logger:     log,
	}
}

// SetSecretManager wires the secret-write dependency.
func (s *Service) SetSecretManager(m SecretManager) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.secretManager = m
}

// SetHostStore wires the host-persistence dependency.
func (s *Service) SetHostStore(h HostStore) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hostStore = h
}

// Client returns the current underlying Client (may be a NoopClient).
func (s *Service) Client() Client {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.client
}

// Host returns the configured GitLab base URL.
func (s *Service) Host() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.host
}

// GetStatus returns the connection status surfaced to the frontend.
func (s *Service) GetStatus(ctx context.Context) (*Status, error) {
	s.mu.RLock()
	client := s.client
	host := s.host
	authMethod := s.authMethod
	s.mu.RUnlock()

	tokenConfigured, tokenSecretID, err := s.findTokenSecret(ctx)
	if err != nil {
		return nil, fmt.Errorf("look up token secret: %w", err)
	}

	if client == nil {
		// Defensive — every Provide/NewClient path returns at least a
		// NoopClient, so in practice this branch is unreachable. Keep
		// RequiredScopes as an empty slice so the JSON contract matches
		// the always-an-array shape declared on the TypeScript side.
		return &Status{
			AuthMethod:      AuthMethodNone,
			Host:            host,
			TokenConfigured: tokenConfigured,
			TokenSecretID:   tokenSecretID,
			RequiredScopes:  []string{},
		}, nil
	}

	authenticated, authErr := client.IsAuthenticated(ctx)
	username := ""
	if authenticated {
		username, _ = client.GetAuthenticatedUser(ctx)
	}

	status := &Status{
		Authenticated:   authenticated,
		Username:        username,
		AuthMethod:      authMethod,
		Host:            host,
		TokenConfigured: tokenConfigured,
		TokenSecretID:   tokenSecretID,
		RequiredScopes:  []string{"api", "read_user"},
	}
	if authErr != nil {
		// IsAuthenticated returns (false, nil) for 401/403 — that's a
		// known "bad token" signal. Anything reaching here is a transport
		// failure (network, 5xx, parse error) the user needs to see as
		// "GitLab unreachable" rather than "not connected", so they don't
		// delete a valid token during a transient outage.
		status.ConnectionError = authErr.Error()
	}
	if g, ok := client.(*GLabClient); ok {
		status.GLabVersion = g.Version()
	}
	return status, nil
}

// ConfigureToken stores a new PAT in the secret manager and rebuilds the
// client. Validates the token by calling /user before persisting.
func (s *Service) ConfigureToken(ctx context.Context, token string) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return fmt.Errorf("%w: empty", ErrInvalidToken)
	}
	if s.secretManager == nil {
		return errors.New("secret manager not configured")
	}

	s.mu.RLock()
	host := s.host
	s.mu.RUnlock()

	probe := NewPATClient(host, token)
	if _, probeErr := probe.GetAuthenticatedUser(ctx); probeErr != nil {
		// Only 401 / 403 mean "the token is bad". Network failures, 5xx,
		// DNS errors etc. propagate without the ErrInvalidToken wrap so the
		// controller surfaces them as "GitLab unreachable" instead of
		// "invalid token" — otherwise during a GitLab outage a user might
		// delete their (valid) token assuming it had been rejected.
		var apiErr *APIError
		if errors.As(probeErr, &apiErr) && (apiErr.StatusCode == http.StatusUnauthorized || apiErr.StatusCode == http.StatusForbidden) {
			return fmt.Errorf("%w: %w", ErrInvalidToken, probeErr)
		}
		return fmt.Errorf("probe token: %w", probeErr)
	}

	configured, secretID, err := s.findTokenSecret(ctx)
	if err != nil {
		return fmt.Errorf("look up token secret: %w", err)
	}
	switch {
	case configured && secretID != "":
		if err := s.secretManager.Update(ctx, secretID, token); err != nil {
			return fmt.Errorf("update token: %w", err)
		}
	default:
		if _, err := s.secretManager.Create(ctx, secretNameToken, token); err != nil {
			return fmt.Errorf("create token: %w", err)
		}
	}

	// Build the installed client inside the write lock using the *current*
	// s.host — if ConfigureHost ran between our snapshot above and now we'd
	// otherwise install a client pointing at the previous host, leaving
	// s.host and s.client desynced until the next reconfigure. The token
	// was validated against `probe`; here we just construct a fresh client
	// at the up-to-date host.
	s.mu.Lock()
	s.client = NewPATClient(s.host, token)
	s.authMethod = AuthMethodPAT
	installedHost := s.host
	s.mu.Unlock()
	// Log the host the installed client actually targets, not the
	// pre-validation snapshot — a concurrent ConfigureHost between the
	// two would otherwise leave the log entry referring to a stale value.
	s.logger.Info("GitLab token configured", zap.String("host", installedHost))
	return nil
}

// ClearToken removes the stored PAT and falls back to noop / glab.
func (s *Service) ClearToken(ctx context.Context) error {
	if s.secretManager == nil {
		return errors.New("secret manager not configured")
	}
	configured, secretID, err := s.findTokenSecret(ctx)
	if err != nil {
		return fmt.Errorf("look up token secret: %w", err)
	}
	if !configured || secretID == "" {
		return nil
	}
	if err := s.secretManager.Delete(ctx, secretID); err != nil {
		return fmt.Errorf("delete token: %w", err)
	}

	// Build the fallback client outside the write lock — NewClient may
	// shell out to `glab auth status` (up to glabAuthTimeout = 10s), and
	// holding the write lock for that long would freeze every concurrent
	// GetStatus / GetMRFeedback / Client() call. Pre-build against a
	// snapshot of s.host, then swap inside a short critical section.
	// Guard the swap with a host-equality check so a concurrent
	// ConfigureHost that already installed a newer client for a new
	// host doesn't get clobbered by our stale-host build.
	s.mu.RLock()
	hostSnap := s.host
	s.mu.RUnlock()
	client, authMethod, _ := NewClient(ctx, hostSnap, s.secrets, s.logger)
	s.mu.Lock()
	if s.host == hostSnap {
		s.client = client
		s.authMethod = authMethod
	}
	// else: ConfigureHost moved s.host while we were probing — leave
	// its (newer, correctly-targeted) client in place. The next
	// reconfigure or auth-health poller will converge if needed.
	s.mu.Unlock()
	return nil
}

// ConfigureHost persists a new GitLab host URL and rebuilds the client.
// The host is normalized by stripping trailing slashes; an empty string
// resets to DefaultHost.
func (s *Service) ConfigureHost(ctx context.Context, host string) error {
	host = strings.TrimRight(strings.TrimSpace(host), "/")
	if host == "" {
		host = DefaultHost
	}
	if !strings.HasPrefix(host, "https://") && !strings.HasPrefix(host, "http://") {
		return errors.New("host must include scheme (https:// or http://)")
	}
	if err := CheckHost(ctx, host); err != nil {
		return fmt.Errorf("host unreachable: %w", err)
	}

	// Persist before mutating the in-memory state so a failed write
	// doesn't leave the running service pointing at a host that wasn't
	// committed to disk.
	if s.hostStore != nil {
		if err := s.hostStore.SetHost(ctx, host); err != nil {
			return fmt.Errorf("persist host: %w", err)
		}
	}

	client, authMethod, _ := NewClient(ctx, host, s.secrets, s.logger)
	s.mu.Lock()
	s.host = host
	s.client = client
	s.authMethod = authMethod
	s.mu.Unlock()
	return nil
}

// GetMRFeedback proxies to the underlying client.
func (s *Service) GetMRFeedback(ctx context.Context, projectPath string, iid int) (*MRFeedback, error) {
	return s.Client().GetMRFeedback(ctx, projectPath, iid)
}

// CreateMRDiscussionNote proxies to the underlying client.
func (s *Service) CreateMRDiscussionNote(ctx context.Context, projectPath string, iid int, discussionID, body string) (*MRNote, error) {
	return s.Client().CreateMRDiscussionNote(ctx, projectPath, iid, discussionID, body)
}

// ResolveMRDiscussion proxies to the underlying client.
func (s *Service) ResolveMRDiscussion(ctx context.Context, projectPath string, iid int, discussionID string) error {
	return s.Client().ResolveMRDiscussion(ctx, projectPath, iid, discussionID)
}

// findTokenSecret reports whether a GitLab token is stored in the secret
// store. Returns (configured, secretID, error). A nil secrets provider is
// treated as "not configured" without error; a List failure is returned so
// callers don't mistake a backend outage for "token absent" (which would
// hide a still-present secret from ClearToken / GetStatus).
func (s *Service) findTokenSecret(ctx context.Context) (bool, string, error) {
	if s.secrets == nil {
		return false, "", nil
	}
	items, err := s.secrets.List(ctx)
	if err != nil {
		return false, "", fmt.Errorf("list secrets: %w", err)
	}
	for _, item := range items {
		if !item.HasValue {
			continue
		}
		if item.Name == secretNameToken || item.Name == "gitlab_token" {
			return true, item.ID, nil
		}
	}
	return false, "", nil
}

// --- Task ↔ MR association ---

// SyncTaskMR fetches the MR's current state from GitLab and upserts the
// task↔MR row. Called from the orchestrator when an agent creates an MR via
// the `pr` skill, and from the topbar's manual refresh. Returns the upserted
// row so the caller can broadcast it on the WS bus.
//
// repositoryID is the task's repository UUID (empty for single-repo tasks).
// projectPath is the GitLab namespace/path. iid is the MR's per-project id.
func (s *Service) SyncTaskMR(ctx context.Context, taskID, repositoryID, projectPath string, iid int) (*TaskMR, error) {
	s.mu.RLock()
	client := s.client
	store := s.store
	host := s.host
	s.mu.RUnlock()
	if store == nil {
		return nil, errors.New("gitlab store not configured")
	}
	if client == nil {
		return nil, ErrNoClient
	}
	status, err := client.GetMRStatus(ctx, projectPath, iid)
	if err != nil {
		return nil, fmt.Errorf("fetch MR status: %w", err)
	}
	if status == nil || status.MR == nil {
		return nil, fmt.Errorf("MR !%d not found in %s", iid, projectPath)
	}
	now := time.Now().UTC()
	mr := status.MR
	row := &TaskMR{
		TaskID:            taskID,
		RepositoryID:      repositoryID,
		Host:              host,
		ProjectPath:       projectPath,
		MRIID:             mr.IID,
		MRURL:             mr.WebURL,
		MRTitle:           mr.Title,
		HeadBranch:        mr.HeadBranch,
		BaseBranch:        mr.BaseBranch,
		AuthorUsername:    mr.AuthorUsername,
		State:             mr.State,
		ApprovalState:     status.ApprovalState,
		PipelineState:     status.PipelineState,
		MergeStatus:       status.MergeStatus,
		Draft:             mr.Draft,
		ApprovalCount:     status.ApprovalCount,
		RequiredApprovals: status.RequiredApprovals,
		PipelineJobsTotal: status.PipelineJobsTotal,
		PipelineJobsPass:  status.PipelineJobsPassing,
		CreatedAt:         mr.CreatedAt,
		MergedAt:          mr.MergedAt,
		ClosedAt:          mr.ClosedAt,
		LastSyncedAt:      &now,
	}
	if err := store.UpsertTaskMR(ctx, row); err != nil {
		return nil, fmt.Errorf("upsert task MR: %w", err)
	}
	return row, nil
}

// ListTaskMRsByWorkspace surfaces all MR associations under a workspace,
// grouped by task ID. Returns an empty map when no store is configured.
func (s *Service) ListTaskMRsByWorkspace(ctx context.Context, workspaceID string) (map[string][]*TaskMR, error) {
	s.mu.RLock()
	store := s.store
	s.mu.RUnlock()
	if store == nil {
		return map[string][]*TaskMR{}, nil
	}
	return store.ListTaskMRsByWorkspaceID(ctx, workspaceID)
}

// ListTaskMRsByTask returns the MR(s) linked to a single task.
func (s *Service) ListTaskMRsByTask(ctx context.Context, taskID string) ([]*TaskMR, error) {
	s.mu.RLock()
	store := s.store
	s.mu.RUnlock()
	if store == nil {
		return nil, nil
	}
	return store.ListTaskMRsByTask(ctx, taskID)
}
