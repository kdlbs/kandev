package azuredevops

import (
	"context"
	"errors"
	"fmt"

	"go.uber.org/zap"
)

// errSignInForOrganization rejects Microsoft account settings that are not
// backed by a sign-in for the same organization.
var errSignInForOrganization = fmt.Errorf("%w: sign in with Microsoft for this organization", ErrInvalidConfig)

// saveOAuthSettings updates the default project of an existing Microsoft
// account connection. Changing the organization needs a new sign-in, because
// the stored tokens belong to the organization's directory.
func (s *Service) saveOAuthSettings(ctx context.Context, workspaceID string, cfg, previous *Config) (*Config, error) {
	if err := requireSignedInOrganization(previous, cfg.OrganizationURL); err != nil {
		return nil, err
	}
	if err := s.store.UpsertConfig(ctx, cfg); err != nil {
		return nil, fmt.Errorf("upsert azure devops config: %w", err)
	}
	return s.GetConfigForWorkspace(ctx, workspaceID)
}

func requireSignedInOrganization(previous *Config, organizationURL string) error {
	if previous == nil || previous.AuthMethod != AuthMethodOAuth || previous.OrganizationURL != organizationURL {
		return errSignInForOrganization
	}
	return nil
}

// discardOAuthCredential removes a workspace's Microsoft tokens and their
// projection after another credential replaced them. Readers follow
// auth_method, so a leftover token is never used; failures are only logged.
func (s *Service) discardOAuthCredential(ctx context.Context, workspaceID string) {
	unlock := s.oauthLocks.lock(workspaceID)
	defer unlock()
	if s.secrets != nil {
		for _, key := range []string{OAuthRefreshTokenKey(workspaceID), OAuthAccessTokenKey(workspaceID)} {
			if exists, err := s.secrets.Exists(ctx, key); err != nil || !exists {
				continue
			}
			if err := s.secrets.Delete(ctx, key); err != nil {
				s.log.Warn("azure devops: remove replaced microsoft token failed",
					zap.String("workspace_id", workspaceID), zap.Error(err))
			}
		}
	}
	if err := s.store.ClearOAuthCredentialState(ctx, workspaceID); err != nil {
		s.log.Warn("azure devops: clear microsoft sign-in state failed",
			zap.String("workspace_id", workspaceID), zap.Error(err))
	}
}

// copyOAuthConfig copies a Microsoft account connection's settings without its
// tokens. The target must sign in itself, since sharing one rotating refresh
// token between workspaces would let either invalidate the other.
func (s *Service) copyOAuthConfig(ctx context.Context, source *Config, targetWorkspaceID string) (*Config, error) {
	target := &Config{
		WorkspaceID: targetWorkspaceID, OrganizationURL: source.OrganizationURL,
		DefaultProjectID: source.DefaultProjectID, DefaultProjectName: source.DefaultProjectName,
		AuthMethod: AuthMethodOAuth,
	}
	if err := s.store.UpsertConfig(ctx, target); err != nil {
		return nil, fmt.Errorf("upsert target azure devops config: %w", err)
	}
	s.removeStoredPAT(ctx, targetWorkspaceID)
	s.discardOAuthCredential(ctx, targetWorkspaceID)
	if err := s.store.SetOAuthApplication(ctx, targetWorkspaceID, source.OAuthClientID, source.OAuthRedirectURL); err != nil {
		return nil, fmt.Errorf("copy target microsoft application: %w", err)
	}
	if err := s.store.SetOAuthReauthRequired(ctx, targetWorkspaceID, true); err != nil {
		return nil, fmt.Errorf("mark target sign-in required: %w", err)
	}
	if err := s.store.ResetAuthHealth(ctx, targetWorkspaceID); err != nil {
		return nil, fmt.Errorf("reset target azure devops health: %w", err)
	}
	return s.GetConfigForWorkspace(ctx, targetWorkspaceID)
}

// removeStoredPAT deletes a workspace PAT that another credential replaced.
// Readers follow auth_method, so a failed deletion is only logged.
func (s *Service) removeStoredPAT(ctx context.Context, workspaceID string) {
	if s.secrets == nil {
		return
	}
	key := SecretKeyForWorkspace(workspaceID)
	if exists, err := s.secrets.Exists(ctx, key); err != nil || !exists {
		return
	}
	if err := s.secrets.Delete(ctx, key); err != nil {
		s.log.Warn("azure devops: remove replaced PAT failed", zap.String("workspace_id", workspaceID), zap.Error(err))
	}
}

// SignOutOAuth removes the workspace's Microsoft tokens so Kandev stops acting
// as the signed-in user, keeping the connection settings for a later sign-in.
// It does not end the user's Microsoft session or revoke tokens at Microsoft.
func (s *Service) SignOutOAuth(ctx context.Context, workspaceID string) (*Config, error) {
	if err := validateWorkspaceID(workspaceID); err != nil {
		return nil, err
	}
	if err := s.authorizeWorkspaceAccess(ctx, workspaceID); err != nil {
		return nil, err
	}
	unlock := s.oauthLocks.lock(workspaceID)
	defer unlock()
	cfg, err := s.store.GetConfig(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("read azure devops config: %w", err)
	}
	if cfg == nil || cfg.AuthMethod != AuthMethodOAuth {
		return nil, fmt.Errorf("%w: no Microsoft account connection to sign out", ErrInvalidConfig)
	}
	if s.secrets != nil {
		for _, key := range []string{OAuthRefreshTokenKey(workspaceID), OAuthAccessTokenKey(workspaceID)} {
			if exists, err := s.secrets.Exists(ctx, key); err != nil {
				return nil, fmt.Errorf("check microsoft token: %w", err)
			} else if exists {
				if err := s.secrets.Delete(ctx, key); err != nil {
					return nil, fmt.Errorf("delete microsoft token: %w", err)
				}
			}
		}
	}
	if err := s.store.MarkOAuthSignedOut(ctx, workspaceID); err != nil {
		return nil, fmt.Errorf("record microsoft sign-out: %w", err)
	}
	if err := s.store.ResetAuthHealth(ctx, workspaceID); err != nil {
		return nil, fmt.Errorf("reset azure devops auth health: %w", err)
	}
	s.log.Info("azure devops oauth signed_out", zap.String("workspace_id", workspaceID))
	return s.GetConfigForWorkspace(ctx, workspaceID)
}

// oauthTokenSnapshot holds a workspace's Microsoft tokens before a replacement.
type oauthTokenSnapshot struct {
	values map[string]string
}

func (s *Service) snapshotOAuthTokens(ctx context.Context, workspaceID string) (oauthTokenSnapshot, error) {
	snapshot := oauthTokenSnapshot{values: map[string]string{}}
	for _, key := range []string{OAuthRefreshTokenKey(workspaceID), OAuthAccessTokenKey(workspaceID)} {
		exists, err := s.secrets.Exists(ctx, key)
		if err != nil {
			return snapshot, fmt.Errorf("check microsoft token: %w", err)
		}
		if !exists {
			continue
		}
		value, err := s.secrets.Reveal(ctx, key)
		if err != nil {
			return snapshot, fmt.Errorf("read microsoft token: %w", err)
		}
		snapshot.values[key] = value
	}
	return snapshot, nil
}

// restoreOAuthTokens puts back the tokens captured before a failed
// replacement and removes tokens that did not exist before.
func (s *Service) restoreOAuthTokens(ctx context.Context, workspaceID string, snapshot oauthTokenSnapshot) error {
	var result error
	for _, key := range []string{OAuthRefreshTokenKey(workspaceID), OAuthAccessTokenKey(workspaceID)} {
		if value, ok := snapshot.values[key]; ok {
			result = errors.Join(result, s.secrets.Set(ctx, key, "Azure DevOps Microsoft token", value))
			continue
		}
		if exists, err := s.secrets.Exists(ctx, key); err == nil && exists {
			result = errors.Join(result, s.secrets.Delete(ctx, key))
		}
	}
	return result
}
