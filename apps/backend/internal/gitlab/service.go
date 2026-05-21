package gitlab

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
)

// secretNameToken is the canonical secret-store name for the GitLab PAT.
const secretNameToken = "GITLAB_TOKEN"

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
	logger        *logger.Logger
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
		return errors.New("invalid token: empty")
	}
	if s.secretManager == nil {
		return errors.New("secret manager not configured")
	}

	s.mu.RLock()
	host := s.host
	s.mu.RUnlock()

	probe := NewPATClient(host, token)
	if _, probeErr := probe.GetAuthenticatedUser(ctx); probeErr != nil {
		return fmt.Errorf("invalid token: %w", probeErr)
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
	s.mu.Unlock()
	s.logger.Info("GitLab token configured", zap.String("host", host))
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

	s.mu.RLock()
	host := s.host
	s.mu.RUnlock()

	client, authMethod, _ := NewClient(ctx, host, s.secrets, s.logger)
	s.mu.Lock()
	s.client = client
	s.authMethod = authMethod
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
