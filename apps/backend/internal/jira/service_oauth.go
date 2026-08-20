package jira

import (
	"context"
	"errors"
	"fmt"

	"go.uber.org/zap"
)

// RevealAccessToken reads the current OAuth access token from the secret store.
// Implements the TokenRefresher interface for CloudClient/MCPClient auto-refresh.
func (s *Service) RevealAccessToken(ctx context.Context, workspaceID string) (string, error) {
	if s.secrets == nil {
		return "", nil
	}
	return s.secrets.Reveal(ctx, OAuthAccessTokenKeyForWorkspace(workspaceID))
}

// SetOAuthRedirectURI sets the OAuth callback URL. Called at Provide time.
func (s *Service) SetOAuthRedirectURI(uri string) {
	if s != nil {
		s.oauthRedirectURI = uri
	}
}

// resolveCredentials picks the credentials to test: if the request carries a
// secret, use it inline (pre-save); otherwise fall back to the stored secret
// (post-save re-test). For OAuth, reads the OAuth access token instead of
// the legacy secret key.
func (s *Service) resolveCredentials(ctx context.Context, workspaceID string, req *SetConfigRequest) (*JiraConfig, string, error) {
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
		if cfg.AuthMethod == AuthMethodOAuth {
			secret, err = s.secrets.Reveal(ctx, OAuthAccessTokenKeyForWorkspace(workspaceID))
		} else {
			secret, err = s.revealSecret(ctx, workspaceID)
		}
		if err != nil {
			s.log.Warn("jira: secret reveal failed", zap.Error(err))
			return nil, "", fmt.Errorf("read jira secret: %w", err)
		}
		if secret == "" {
			return nil, "", errors.New("no token stored - paste one to test")
		}
	}
	needsStoredConfig := cfg.SiteURL == "" ||
		cfg.AuthMethod == "" ||
		cfg.InstanceType == "" ||
		cfg.AuthMethod == AuthMethodOAuth ||
		(cfg.AuthMethod == AuthMethodAPIToken && cfg.Email == "")
	if needsStoredConfig {
		if err := s.fillConfigFromStored(ctx, workspaceID, cfg); err != nil {
			return nil, "", err
		}
	}
	if cfg.InstanceType == "" {
		cfg.InstanceType = InstanceTypeCloud
	}
	if err := validateAuthInstance(cfg.AuthMethod, cfg.InstanceType, cfg.Email); err != nil {
		return nil, "", err
	}
	return cfg, secret, nil
}

// fillConfigFromStored reads the persisted config and copies any fields the
// caller left blank onto cfg. Includes OAuth fields (ClientID, CloudID).
func (s *Service) fillConfigFromStored(ctx context.Context, workspaceID string, cfg *JiraConfig) error {
	stored, err := s.store.GetConfigForWorkspace(ctx, workspaceID)
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
	if cfg.ClientID == "" {
		cfg.ClientID = stored.ClientID
	}
	if cfg.CloudID == "" {
		cfg.CloudID = stored.CloudID
	}
	return nil
}
