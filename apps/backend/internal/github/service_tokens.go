package github

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"
)

// ExhaustedRateLimit is a tiny DTO returned to the health package — the
// health package can't import github (cycle), so it consumes a structural
// shape via interface assertion.
type ExhaustedRateLimit struct {
	Resource string
	ResetAt  time.Time
}

// ExhaustedRateLimits lists every resource bucket currently out of quota.
// Returns an empty slice when none are exhausted, so callers can safely
// `len()` the result.
func (s *Service) ExhaustedRateLimits() []ExhaustedRateLimit {
	if s.rateTracker == nil {
		return nil
	}
	out := []ExhaustedRateLimit{}
	for resource, snap := range s.rateTracker.All() {
		if snap.Exhausted() {
			out = append(out, ExhaustedRateLimit{
				Resource: string(resource),
				ResetAt:  snap.ResetAt,
			})
		}
	}
	return out
}

// rateLimitInfo materializes the tracker's snapshots into the DTO shape
// returned by GetStatus and the rate-limit WS notification. Returns nil
// when no buckets are known yet so the field stays omitted.
func (s *Service) rateLimitInfo() *GitHubRateLimitInfo {
	if s.rateTracker == nil {
		return nil
	}
	all := s.rateTracker.All()
	if len(all) == 0 {
		return nil
	}
	info := &GitHubRateLimitInfo{}
	if snap, ok := all[ResourceCore]; ok {
		v := snap
		info.Core = &v
	}
	if snap, ok := all[ResourceGraphQL]; ok {
		v := snap
		info.GraphQL = &v
	}
	if snap, ok := all[ResourceSearch]; ok {
		v := snap
		info.Search = &v
	}
	return info
}

// RequiredGitHubScopes lists the GitHub token scopes needed for full functionality.
var RequiredGitHubScopes = []string{"repo", "read:org"}

// GetStatus returns the current GitHub connection status.
// If not authenticated, it retries client creation to pick up auth changes
// (e.g. GITHUB_TOKEN secret added after startup).
func (s *Service) GetStatus(ctx context.Context) (*GitHubStatus, error) {
	if !s.IsAuthenticated() {
		s.retryClientCreation(ctx)
	}

	s.mu.Lock()
	client := s.client
	authMethod := s.authMethod
	s.mu.Unlock()

	status := &GitHubStatus{
		AuthMethod:     authMethod,
		RequiredScopes: RequiredGitHubScopes,
		RateLimit:      s.rateLimitInfo(),
	}

	// Check if a GITHUB_TOKEN secret is configured
	tokenSecretID, hasToken := s.findGitHubTokenSecret(ctx)
	status.TokenConfigured = hasToken
	status.TokenSecretID = tokenSecretID

	if client == nil {
		status.Diagnostics = runGHDiagnostics(ctx)
		return status, nil
	}
	ok, err := client.IsAuthenticated(ctx)
	if err != nil {
		return status, nil
	}
	status.Authenticated = ok
	if ok {
		user, err := client.GetAuthenticatedUser(ctx)
		if err == nil {
			status.Username = user
		}
	} else {
		status.Diagnostics = runGHDiagnostics(ctx)
	}
	return status, nil
}

// findGitHubTokenSecret checks if a GITHUB_TOKEN secret exists in the secrets store.
// Returns the secret ID and whether a token is configured.
func (s *Service) findGitHubTokenSecret(ctx context.Context) (string, bool) {
	if s.secrets == nil {
		return "", false
	}
	items, err := s.secrets.List(ctx)
	if err != nil {
		return "", false
	}
	for _, item := range items {
		if !item.HasValue {
			continue
		}
		if item.Name == "GITHUB_TOKEN" || item.Name == "github_token" {
			return item.ID, true
		}
	}
	return "", false
}

// retryClientCreation attempts to create a GitHub client when not authenticated.
// This picks up auth changes made after startup (secrets added, env vars set).
func (s *Service) retryClientCreation(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.authMethod != AuthMethodNone {
		return // already authenticated
	}
	client, authMethod, err := NewClient(ctx, s.secrets, s.logger)
	if err != nil {
		s.logger.Debug("GitHub client retry failed", zap.Error(err))
		return
	}
	attachRateTracker(client, s.rateTracker, s.logger)
	s.client = client
	s.authMethod = authMethod
	s.logger.Info("GitHub client recovered after retry",
		zap.String("auth_method", authMethod))
}

// runGHDiagnostics runs gh auth status if the gh CLI is available.
func runGHDiagnostics(ctx context.Context) *AuthDiagnostics {
	if !GHAvailable() {
		return &AuthDiagnostics{
			Command:  "gh auth status",
			Output:   "gh CLI is not installed. Install it from https://cli.github.com",
			ExitCode: -1,
		}
	}
	return NewGHClient().RunAuthDiagnostics(ctx)
}

// ConfigureToken saves or updates a GitHub Personal Access Token in the secrets store.
// It validates the token by making a test API call before saving.
func (s *Service) ConfigureToken(ctx context.Context, token string) error {
	if s.secretManager == nil {
		return fmt.Errorf("secret manager not configured")
	}

	// Validate the token by testing authentication. Wire the rate tracker
	// onto the test client up front so the validation request seeds the
	// shared quota and subsequent PAT-backed calls keep feeding it.
	testClient := s.newPATClient(token)
	user, err := testClient.GetAuthenticatedUser(ctx)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidToken, err)
	}
	s.logger.Info("validated GitHub token", zap.String("user", user))

	// Check if a GITHUB_TOKEN secret already exists
	existingID, exists := s.findGitHubTokenSecret(ctx)
	if exists {
		// Update existing secret
		if err := s.secretManager.Update(ctx, existingID, token); err != nil {
			return fmt.Errorf("update token: %w", err)
		}
		s.logger.Info("updated GitHub token secret", zap.String("id", existingID))
	} else {
		// Create new secret
		newID, err := s.secretManager.Create(ctx, "GITHUB_TOKEN", token)
		if err != nil {
			return fmt.Errorf("create token: %w", err)
		}
		s.logger.Info("created GitHub token secret", zap.String("id", newID))
	}

	// Drop identity-scoped caches BEFORE publishing the new client.
	// Order matters: if we published the new client first, a concurrent
	// goroutine could snap s.client (new identity) but be served stale
	// cached data from the prior identity in the brief window before
	// the Clear* calls fire. Without this, the picker would surface
	// the prior user's repos for up to 60s after a token swap, a repo
	// classified as unresolvable under the old credentials would stay
	// "permanent: true" for up to 10 minutes, and stale PR review /
	// check data could leak across identities.
	s.ClearAccessibleReposCaches()
	s.ClearRepoErrorCache()
	s.ClearPRCaches()

	// Refresh the client to use the new token
	s.mu.Lock()
	s.client = testClient
	s.authMethod = AuthMethodPAT
	s.mu.Unlock()

	return nil
}

// ClearToken removes the configured GitHub token from the secrets store.
func (s *Service) ClearToken(ctx context.Context) error {
	if s.secretManager == nil {
		return fmt.Errorf("secret manager not configured")
	}

	existingID, exists := s.findGitHubTokenSecret(ctx)
	if !exists {
		return nil // No token to clear
	}

	if err := s.secretManager.Delete(ctx, existingID); err != nil {
		return fmt.Errorf("delete token: %w", err)
	}
	s.logger.Info("cleared GitHub token secret", zap.String("id", existingID))

	// Drop identity-scoped caches BEFORE clearing the client field —
	// see ConfigureToken for the publish-order rationale.
	s.ClearAccessibleReposCaches()
	s.ClearRepoErrorCache()
	s.ClearPRCaches()

	// Reset to try gh CLI or other methods
	s.mu.Lock()
	s.client = nil
	s.authMethod = AuthMethodNone
	s.mu.Unlock()

	// Try to re-establish connection via other methods
	s.retryClientCreation(ctx)

	return nil
}
