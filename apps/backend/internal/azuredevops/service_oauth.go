package azuredevops

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/auth/authn"
)

const (
	oauthFlowTTL            = 10 * time.Minute
	oauthPersistenceTimeout = 10 * time.Second
	oauthStateBytes         = 32
)

var (
	// ErrOAuthUnavailable means the installation has no Entra application.
	ErrOAuthUnavailable = errors.New("azure devops: microsoft sign-in is not configured")
	// ErrOAuthFlowInvalid covers unknown, expired, reused, and foreign
	// sign-in attempts without revealing which one applied.
	ErrOAuthFlowInvalid = errors.New("azure devops: microsoft sign-in attempt is invalid or expired")
	// ErrReauthRequired means only a new Microsoft sign-in restores access.
	ErrReauthRequired = errors.New("azure devops: microsoft sign-in expired; sign in again")
	// ErrOAuthProviderUnavailable means Azure DevOps or Microsoft could not be
	// reached while preparing sign-in.
	ErrOAuthProviderUnavailable = errors.New("azure devops: microsoft sign-in is temporarily unavailable")
)

// StartOAuthRequest selects the organization and default project to connect.
type StartOAuthRequest struct {
	OrganizationURL    string `json:"organizationUrl"`
	DefaultProjectID   string `json:"defaultProjectId"`
	DefaultProjectName string `json:"defaultProjectName"`
	// ClientID and RedirectURL select the workspace's own Entra application;
	// empty values use the installation defaults.
	ClientID    string `json:"clientId"`
	RedirectURL string `json:"redirectUrl"`
}

// StartOAuthResponse carries the Microsoft authorization URL and the state the
// browser uses to match the callback.
type StartOAuthResponse struct {
	AuthURL string `json:"authUrl"`
	State   string `json:"state"`
}

// CompleteOAuthRequest is the authorization result returned by Microsoft.
type CompleteOAuthRequest struct {
	Code  string `json:"code"`
	State string `json:"state"`
}

// OAuthRefreshTokenKey is the encrypted secret key of a workspace's
// Microsoft refresh token.
func OAuthRefreshTokenKey(workspaceID string) string {
	return "azure_devops:" + workspaceID + ":oauth_refresh_token"
}

// OAuthAccessTokenKey is the encrypted secret key of a workspace's Microsoft
// access token.
func OAuthAccessTokenKey(workspaceID string) string {
	return "azure_devops:" + workspaceID + ":oauth_access_token"
}

func hashOAuthState(state string) string {
	sum := sha256.Sum256([]byte(state))
	return hex.EncodeToString(sum[:])
}

func randomURLToken(size int) (string, error) {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func pkceChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func oauthUserID(ctx context.Context) string {
	if identity, ok := authn.IdentityFromContext(ctx); ok {
		return strings.TrimSpace(identity.UserID)
	}
	return ""
}

// StartOAuth records a pending Microsoft sign-in for the workspace and returns
// the authorization URL.
func (s *Service) StartOAuth(ctx context.Context, workspaceID string, req *StartOAuthRequest) (*StartOAuthResponse, error) {
	if err := validateWorkspaceID(workspaceID); err != nil {
		return nil, err
	}
	if err := s.authorizeWorkspaceAccess(ctx, workspaceID); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("%w: request required", ErrInvalidConfig)
	}
	organizationURL, err := ValidateOrganizationURL(req.OrganizationURL)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrInvalidConfig, err.Error())
	}
	app, err := s.resolveOAuthApplication(req)
	if err != nil {
		return nil, err
	}
	tenant, err := s.entra.DiscoverTenant(ctx, organizationURL)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrOAuthProviderUnavailable, err)
	}
	state, err := randomURLToken(oauthStateBytes)
	if err != nil {
		return nil, fmt.Errorf("generate sign-in state: %w", err)
	}
	verifier, err := randomURLToken(oauthStateBytes)
	if err != nil {
		return nil, fmt.Errorf("generate PKCE verifier: %w", err)
	}
	now := time.Now().UTC()
	flow := &oauthFlow{
		StateHash: hashOAuthState(state), WorkspaceID: workspaceID, UserID: oauthUserID(ctx),
		OrganizationURL: organizationURL, DefaultProjectID: req.DefaultProjectID,
		DefaultProjectName: req.DefaultProjectName, TenantID: tenant, ClientID: app.ClientID,
		PKCEVerifier: verifier, RedirectURL: app.RedirectURL, ExpiresAt: now.Add(oauthFlowTTL),
	}
	if err := s.store.InsertOAuthFlow(ctx, flow, now); err != nil {
		return nil, fmt.Errorf("record microsoft sign-in: %w", err)
	}
	s.log.Info("azure devops oauth started", zap.String("workspace_id", workspaceID))
	return &StartOAuthResponse{
		AuthURL: s.entra.AuthorizeURL(entraAuthorizeRequest{
			Tenant: tenant, ClientID: flow.ClientID, RedirectURL: flow.RedirectURL,
			State: state, CodeChallenge: pkceChallenge(verifier),
		}),
		State: state,
	}, nil
}

// CompleteOAuth exchanges the authorization code of a pending sign-in and
// switches the workspace connection to the Microsoft account.
func (s *Service) CompleteOAuth(ctx context.Context, workspaceID string, req *CompleteOAuthRequest) (*Config, error) {
	if err := validateWorkspaceID(workspaceID); err != nil {
		return nil, err
	}
	if err := s.authorizeWorkspaceAccess(ctx, workspaceID); err != nil {
		return nil, err
	}
	flow, err := s.claimOAuthFlow(ctx, workspaceID, req)
	if err != nil {
		return nil, err
	}
	token, err := s.entra.ExchangeCode(ctx, flow.TenantID, flow.ClientID, req.Code, flow.RedirectURL, flow.PKCEVerifier)
	if err != nil {
		s.logOAuthFailure(workspaceID, err)
		return nil, err
	}
	if token.RefreshToken == "" {
		return nil, errors.New("azure devops: microsoft sign-in returned no refresh token")
	}
	cfg := &Config{
		WorkspaceID: workspaceID, OrganizationURL: flow.OrganizationURL,
		DefaultProjectID: flow.DefaultProjectID, DefaultProjectName: flow.DefaultProjectName,
		AuthMethod: AuthMethodOAuth,
	}
	identity, err := s.clientFn(cfg, staticBearerAuthorizer{token: token.AccessToken}).TestAuth(ctx)
	if err != nil {
		return nil, err
	}
	if identity == nil || !identity.OK {
		return nil, errors.New("azure devops: the signed-in account cannot access this organization")
	}
	if err := s.replaceOAuthConnection(ctx, cfg, flow, oauthAccountName(identity), token); err != nil {
		return nil, err
	}
	s.log.Info("azure devops oauth completed", zap.String("workspace_id", workspaceID))
	return s.finishConfigUpdate(ctx, workspaceID, true)
}

func (s *Service) claimOAuthFlow(ctx context.Context, workspaceID string, req *CompleteOAuthRequest) (*oauthFlow, error) {
	if s.entra == nil {
		return nil, ErrOAuthUnavailable
	}
	if req == nil || req.Code == "" || req.State == "" {
		return nil, ErrOAuthFlowInvalid
	}
	flow, err := s.store.ConsumeOAuthFlow(ctx, hashOAuthState(req.State), time.Now().UTC())
	if err != nil {
		return nil, fmt.Errorf("claim microsoft sign-in: %w", err)
	}
	if flow == nil || flow.WorkspaceID != workspaceID || flow.UserID != oauthUserID(ctx) {
		return nil, ErrOAuthFlowInvalid
	}
	return flow, nil
}

func oauthAccountName(identity *TestConnectionResult) string {
	if identity.DisplayName != "" {
		return identity.DisplayName
	}
	return identity.Email
}

// replaceOAuthConnection installs a new Microsoft credential under the
// workspace's renewal lock, so a renewal that started for the previous
// credential cannot overwrite it afterwards.
func (s *Service) replaceOAuthConnection(
	ctx context.Context, cfg *Config, flow *oauthFlow, account string, token *entraToken,
) error {
	unlock := s.oauthLocks.lock(cfg.WorkspaceID)
	defer unlock()
	return s.persistOAuthConnection(ctx, cfg, flow, account, token)
}

// persistOAuthConnection stores the tokens, switches the workspace to the
// Microsoft account, and removes the replaced PAT. If any write fails, the
// previous Microsoft tokens are restored so the existing connection stays
// usable. Callers hold the workspace's renewal lock.
func (s *Service) persistOAuthConnection(
	ctx context.Context, cfg *Config, flow *oauthFlow, account string, token *entraToken,
) error {
	if s.secrets == nil {
		return errors.New("azure devops: no secret store configured")
	}
	persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), oauthPersistenceTimeout)
	defer cancel()
	workspaceID := cfg.WorkspaceID
	previous, err := s.snapshotOAuthTokens(persistCtx, workspaceID)
	if err != nil {
		return err
	}
	if err := s.writeOAuthConnection(persistCtx, cfg, flow, account, token); err != nil {
		return joinMutationError(err, s.restoreOAuthTokens(persistCtx, workspaceID, previous))
	}
	s.removeStoredPAT(persistCtx, workspaceID)
	return nil
}

func (s *Service) writeOAuthConnection(
	ctx context.Context, cfg *Config, flow *oauthFlow, account string, token *entraToken,
) error {
	if err := s.storeOAuthTokens(ctx, cfg.WorkspaceID, token.RefreshToken, token.AccessToken); err != nil {
		return err
	}
	if err := s.store.UpsertConfig(ctx, cfg); err != nil {
		return fmt.Errorf("upsert azure devops config: %w", err)
	}
	expiresAt := time.Now().UTC().Add(time.Duration(token.ExpiresIn) * time.Second)
	if err := s.store.SetOAuthCredentialState(ctx, cfg.WorkspaceID, oauthCredentialState{
		Account: account, TenantID: flow.TenantID, ClientID: flow.ClientID,
		RedirectURL: flow.RedirectURL, TokenExpiresAt: &expiresAt,
	}); err != nil {
		return fmt.Errorf("record microsoft sign-in: %w", err)
	}
	return nil
}

func (s *Service) logOAuthFailure(workspaceID string, err error) {
	var entraErr *EntraError
	code := "error"
	if errors.As(err, &entraErr) {
		code = entraErr.Code
	}
	s.log.Warn("azure devops oauth failed", zap.String("workspace_id", workspaceID), zap.String("code", code))
}

// storedAccessToken reveals the current Microsoft access token.
func (s *Service) storedAccessToken(ctx context.Context, workspaceID string) (string, error) {
	if s.secrets == nil {
		return "", ErrNotConfigured
	}
	token, err := s.secrets.Reveal(ctx, OAuthAccessTokenKey(workspaceID))
	if err != nil || token == "" {
		return "", ErrReauthRequired
	}
	return token, nil
}

// oauthCredentialExists reports whether a Microsoft credential is stored.
func (s *Service) oauthCredentialExists(ctx context.Context, workspaceID string) (bool, error) {
	if s.secrets == nil {
		return false, nil
	}
	return s.secrets.Exists(ctx, OAuthRefreshTokenKey(workspaceID))
}
