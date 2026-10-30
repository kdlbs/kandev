package azuredevops

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

// oauthRenewalMargin renews access tokens before they expire so a request
// never starts with a token that lapses mid-flight.
const oauthRenewalMargin = 5 * time.Minute

// workspaceLocks serializes token renewal per workspace.
type workspaceLocks struct {
	mu    sync.Mutex
	locks map[string]*sync.Mutex
}

func (l *workspaceLocks) lock(workspaceID string) func() {
	l.mu.Lock()
	if l.locks == nil {
		l.locks = make(map[string]*sync.Mutex)
	}
	lock, ok := l.locks[workspaceID]
	if !ok {
		lock = &sync.Mutex{}
		l.locks[workspaceID] = lock
	}
	l.mu.Unlock()
	lock.Lock()
	return lock.Unlock
}

// accessToken returns a Microsoft access token that stays valid for at least
// the renewal margin, renewing it first when needed.
func (s *Service) accessToken(ctx context.Context, workspaceID string) (string, error) {
	if token, ok, err := s.freshAccessToken(ctx, workspaceID); err != nil || ok {
		return token, err
	}
	unlock := s.oauthLocks.lock(workspaceID)
	defer unlock()
	// A concurrent caller may have renewed while this one waited.
	if token, ok, err := s.freshAccessToken(ctx, workspaceID); err != nil || ok {
		return token, err
	}
	if err := s.refreshLocked(ctx, workspaceID); err != nil {
		return "", err
	}
	return s.storedAccessToken(ctx, workspaceID)
}

// renewAccessToken replaces an access token that Azure rejected. When a
// concurrent caller already replaced it, the rotation is not repeated, because
// renewing again would spend the freshly rotated refresh token.
func (s *Service) renewAccessToken(ctx context.Context, workspaceID, rejected string) (bool, error) {
	unlock := s.oauthLocks.lock(workspaceID)
	defer unlock()
	if _, err := s.oauthConfig(ctx, workspaceID); err != nil {
		return false, err
	}
	if current, err := s.storedAccessToken(ctx, workspaceID); err == nil && rejected != "" && current != rejected {
		return true, nil
	}
	if err := s.refreshLocked(ctx, workspaceID); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Service) freshAccessToken(ctx context.Context, workspaceID string) (string, bool, error) {
	cfg, err := s.oauthConfig(ctx, workspaceID)
	if err != nil {
		return "", false, err
	}
	if cfg.TokenExpiresAt == nil || time.Until(*cfg.TokenExpiresAt) <= oauthRenewalMargin {
		return "", false, nil
	}
	token, err := s.storedAccessToken(ctx, workspaceID)
	if err != nil {
		return "", false, nil
	}
	return token, true, nil
}

// refreshLocked redeems the stored refresh token. Callers hold the
// workspace's renewal lock.
func (s *Service) refreshLocked(ctx context.Context, workspaceID string) error {
	cfg, err := s.oauthConfig(ctx, workspaceID)
	if err != nil {
		return err
	}
	clientID := s.oauthClientID(cfg)
	if s.entra == nil || clientID == "" {
		return ErrOAuthUnavailable
	}
	refreshToken, err := s.secrets.Reveal(ctx, OAuthRefreshTokenKey(workspaceID))
	if err != nil || refreshToken == "" {
		return s.requireSignIn(ctx, workspaceID, "missing_refresh_token")
	}
	token, err := s.entra.Refresh(ctx, oauthTenant(cfg), clientID, refreshToken)
	if err != nil {
		var entraErr *EntraError
		if errors.As(err, &entraErr) && entraErr.RequiresSignIn() {
			return s.requireSignIn(ctx, workspaceID, entraErr.Code)
		}
		return fmt.Errorf("renew microsoft access token: %w", err)
	}
	if err := s.persistRenewedToken(ctx, workspaceID, refreshToken, token); err != nil {
		return err
	}
	s.log.Info("azure devops oauth renewed", zap.String("workspace_id", workspaceID))
	return nil
}

// oauthConfig returns a workspace's Microsoft account connection, failing
// fast while it waits for a new sign-in.
func (s *Service) oauthConfig(ctx context.Context, workspaceID string) (*Config, error) {
	cfg, err := s.store.GetConfig(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("read azure devops config: %w", err)
	}
	if cfg == nil || cfg.AuthMethod != AuthMethodOAuth {
		return nil, ErrNotConfigured
	}
	if cfg.ReauthRequired {
		return nil, ErrReauthRequired
	}
	return cfg, nil
}

func oauthTenant(cfg *Config) string {
	if cfg.OAuthTenantID != "" {
		return cfg.OAuthTenantID
	}
	return defaultEntraTenant
}

func (s *Service) requireSignIn(ctx context.Context, workspaceID, reason string) error {
	persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), oauthPersistenceTimeout)
	defer cancel()
	if err := s.store.SetOAuthReauthRequired(persistCtx, workspaceID, true); err != nil {
		return errors.Join(ErrReauthRequired, fmt.Errorf("record sign-in requirement: %w", err))
	}
	s.log.Warn("azure devops oauth reauth_required",
		zap.String("workspace_id", workspaceID), zap.String("code", reason))
	return ErrReauthRequired
}

// persistRenewedToken stores the rotated refresh token before the access
// token, keeping the previous refresh token when Microsoft returns none.
func (s *Service) persistRenewedToken(
	ctx context.Context, workspaceID, previousRefreshToken string, token *entraToken,
) error {
	persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), oauthPersistenceTimeout)
	defer cancel()
	refreshToken := token.RefreshToken
	if refreshToken == "" {
		refreshToken = previousRefreshToken
	}
	if err := s.storeOAuthTokens(persistCtx, workspaceID, refreshToken, token.AccessToken); err != nil {
		return err
	}
	expiresAt := time.Now().UTC().Add(time.Duration(token.ExpiresIn) * time.Second)
	if err := s.store.UpdateOAuthTokenExpiry(persistCtx, workspaceID, expiresAt); err != nil {
		return fmt.Errorf("record microsoft token expiry: %w", err)
	}
	return nil
}

// storeOAuthTokens writes the refresh token first: a failure after it leaves a
// credential that can still renew, instead of a retained invalidated one.
func (s *Service) storeOAuthTokens(ctx context.Context, workspaceID, refreshToken, accessToken string) error {
	if err := s.secrets.Set(ctx, OAuthRefreshTokenKey(workspaceID), "Azure DevOps Microsoft refresh token", refreshToken); err != nil {
		return fmt.Errorf("store microsoft refresh token: %w", err)
	}
	if err := s.secrets.Set(ctx, OAuthAccessTokenKey(workspaceID), "Azure DevOps Microsoft access token", accessToken); err != nil {
		return fmt.Errorf("store microsoft access token: %w", err)
	}
	return nil
}
