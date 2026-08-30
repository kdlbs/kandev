package jira

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	mcptransport "github.com/mark3labs/mcp-go/client/transport"
)

// Atlassian MCP OAuth 2.1 endpoints. The MCP server acts as both
// authorization server and resource server, proxying to auth.atlassian.com.
// Dynamic Client Registration (RFC 7591) + PKCE (S256) replaces the need for
// a pre-created app — no client_id/client_secret from the developer console.
const (
	mcpResourceURL          = "https://mcp.atlassian.com"
	mcpScopes               = "offline_access read:me read:account read:jira-user read:jira-work write:jira-work search:jira-work"
	mcpStateTTL             = 10 * time.Minute
	oauthPersistenceTimeout = 10 * time.Second
)

// mcpTokenResponse is the response from the token endpoint.
type mcpTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"` // seconds
	Scope        string `json:"scope"`
}

// mcpRegisterResponse is the response from dynamic client registration.
type mcpRegisterResponse struct {
	ClientID                string   `json:"client_id"`
	ClientIDIssuedAt        int64    `json:"client_id_issued_at"`
	RedirectURIs            []string `json:"redirect_uris"`
	GrantTypes              []string `json:"grant_types"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
}

// mcpPendingState holds the CSRF state + PKCE verifier during the OAuth flow.
type mcpPendingState struct {
	State         string
	WorkspaceID   string
	SiteURL       string
	ClientID      string // from dynamic registration
	CodeVerifier  string // PKCE verifier (kept server-side)
	RedirectURI   string
	TokenEndpoint string
	CreatedAt     time.Time
}

type mcpOAuthEndpoints struct {
	Authorization string
	Token         string
	Registration  string
}

// mcpStateManager holds pending OAuth states in memory.
type mcpStateManager struct {
	mu     sync.Mutex
	states map[string]mcpPendingState
}

func newMCPStateManager() *mcpStateManager {
	return &mcpStateManager{states: make(map[string]mcpPendingState)}
}

func (m *mcpStateManager) store(state mcpPendingState) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.states[state.State] = state
	m.cleanupLocked()
}

func (m *mcpStateManager) loadAndDelete(state string) (mcpPendingState, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.states[state]
	if ok {
		delete(m.states, state)
	}
	return s, ok
}

func (m *mcpStateManager) cleanupLocked() {
	cutoff := time.Now().Add(-mcpStateTTL)
	for k, s := range m.states {
		if s.CreatedAt.Before(cutoff) {
			delete(m.states, k)
		}
	}
}

func generateRandomString(nBytes int) (string, error) {
	b := make([]byte, nBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// generatePKCEPair creates a code_verifier and code_challenge (S256).
func generatePKCEPair() (verifier, challenge string, err error) {
	verifier, err = generateRandomString(32)
	if err != nil {
		return "", "", err
	}
	h := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(h[:])
	return verifier, challenge, nil
}

// registerMCPClient performs Dynamic Client Registration (RFC 7591) against
// the MCP server. Returns a client_id that is used for the authorization flow.
// No client_secret is returned — the flow uses PKCE with
// token_endpoint_auth_method="none".
func registerMCPClient(ctx context.Context, client *http.Client, registrationEndpoint, redirectURI string) (string, error) {
	body, _ := json.Marshal(map[string]interface{}{
		"client_name":                "Kandev",
		"redirect_uris":              []string{redirectURI},
		"grant_types":                []string{"authorization_code", "refresh_token"},
		"response_types":             []string{"code"},
		"token_endpoint_auth_method": "none",
		"scope":                      mcpScopes,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, registrationEndpoint, strings.NewReader(string(body)))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("dynamic registration failed (%d): %s", resp.StatusCode, string(raw))
	}
	var reg mcpRegisterResponse
	if err := json.Unmarshal(raw, &reg); err != nil {
		return "", fmt.Errorf("decode registration response: %w", err)
	}
	if reg.ClientID == "" {
		return "", errors.New("registration returned empty client_id")
	}
	return reg.ClientID, nil
}

func discoverMCPOAuthEndpoints(ctx context.Context, client *http.Client) (*mcpOAuthEndpoints, error) {
	handler := mcptransport.NewOAuthHandler(mcptransport.OAuthConfig{HTTPClient: client})
	handler.SetBaseURL(mcpResourceURL)
	metadata, err := handler.GetServerMetadata(ctx)
	if err != nil {
		return nil, fmt.Errorf("discover OAuth server: %w", err)
	}
	if metadata.AuthorizationEndpoint == "" || metadata.TokenEndpoint == "" || metadata.RegistrationEndpoint == "" {
		return nil, errors.New("OAuth server metadata is missing required endpoints")
	}
	return &mcpOAuthEndpoints{
		Authorization: metadata.AuthorizationEndpoint,
		Token:         metadata.TokenEndpoint,
		Registration:  metadata.RegistrationEndpoint,
	}, nil
}

func newMCPOAuthHTTPClient() *http.Client {
	return &http.Client{Timeout: 15 * time.Second}
}

// oauthPersistenceContext keeps credential writes alive after the request
// ends. The token exchange invalidates the previous refresh token, so losing
// the new token between the exchange and persistence would force re-auth.
func oauthPersistenceContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), oauthPersistenceTimeout)
}

// StartOAuthFlow registers a client dynamically, generates PKCE, and builds the
// authorization URL. The user never needs to create an Atlassian app.
func (s *Service) StartOAuthFlow(ctx context.Context, workspaceID, siteURL, redirectURI string) (string, error) {
	if redirectURI == "" {
		return "", errors.New("redirect URI is required")
	}
	resolvedWorkspaceID, err := s.resolveWorkspaceID(ctx, workspaceID)
	if err != nil {
		return "", err
	}
	client := newMCPOAuthHTTPClient()
	endpoints, err := discoverMCPOAuthEndpoints(ctx, client)
	if err != nil {
		return "", err
	}
	clientID, err := registerMCPClient(ctx, client, endpoints.Registration, redirectURI)
	if err != nil {
		return "", fmt.Errorf("dynamic client registration: %w", err)
	}
	verifier, challenge, err := generatePKCEPair()
	if err != nil {
		return "", fmt.Errorf("generate PKCE: %w", err)
	}
	state, err := generateRandomString(16)
	if err != nil {
		return "", err
	}
	s.oauthStates.store(mcpPendingState{
		State:         state,
		WorkspaceID:   resolvedWorkspaceID,
		SiteURL:       siteURL,
		ClientID:      clientID,
		CodeVerifier:  verifier,
		RedirectURI:   redirectURI,
		TokenEndpoint: endpoints.Token,
		CreatedAt:     time.Now(),
	})
	params := url.Values{
		"response_type":         {"code"},
		"client_id":             {clientID},
		"redirect_uri":          {redirectURI},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
		"state":                 {state},
		"resource":              {mcpResourceURL},
		"scope":                 {mcpScopes},
	}
	return endpoints.Authorization + "?" + params.Encode(), nil
}

// CompleteOAuthFlow exchanges the authorization code for tokens (using PKCE,
// no client_secret), resolves the cloudId, and persists config + secrets.
func (s *Service) CompleteOAuthFlow(ctx context.Context, code, state string) (*JiraConfig, error) {
	st, ok := s.oauthStates.loadAndDelete(state)
	if !ok || time.Since(st.CreatedAt) > mcpStateTTL {
		return nil, errors.New("invalid or expired OAuth state")
	}
	// Bind the completing user to the workspace the flow was started for, so a
	// leaked state cannot persist a victim's tokens into someone else's workspace.
	if err := s.authorizeWorkspaceAccess(ctx, st.WorkspaceID); err != nil {
		return nil, fmt.Errorf("authorize workspace: %w", err)
	}
	tok, err := exchangeMCPCode(ctx, newMCPOAuthHTTPClient(), st.TokenEndpoint, st.ClientID, code, st.RedirectURI, st.CodeVerifier)
	if err != nil {
		return nil, fmt.Errorf("token exchange: %w", err)
	}
	// cloudId is required for every MCP tool call; a workspace saved without it
	// looks connected but fails on every request, so fail the flow loudly here.
	cloudID, err := resolveCloudIDViaMCP(ctx, tok.AccessToken, st.SiteURL)
	if err != nil {
		return nil, fmt.Errorf("resolve Atlassian cloud ID for %q: %w", st.SiteURL, err)
	}
	expiresAt := time.Now().Add(time.Duration(tok.ExpiresIn) * time.Second)
	cfg := &JiraConfig{
		WorkspaceID:    st.WorkspaceID,
		SiteURL:        st.SiteURL,
		AuthMethod:     AuthMethodOAuth,
		InstanceType:   InstanceTypeCloud,
		ClientID:       st.ClientID,
		CloudID:        cloudID,
		TokenExpiresAt: &expiresAt,
	}
	// UpsertConfigForWorkspace is a full-row write; carry over user-owned fields
	// so connecting OAuth doesn't wipe an existing email / default project key.
	if existing, err := s.store.GetConfigForWorkspace(ctx, st.WorkspaceID); err == nil && existing != nil {
		cfg.Email = existing.Email
		cfg.DefaultProjectKey = existing.DefaultProjectKey
	}
	if tok.AccessToken == "" || tok.RefreshToken == "" {
		return nil, errors.New("token endpoint did not return the required access and refresh tokens")
	}
	persistCtx, cancel := oauthPersistenceContext(ctx)
	defer cancel()
	// Persist the rotating refresh token first. A later failure then leaves a
	// credential that can recover, instead of retaining an invalidated token.
	if err := s.secrets.Set(persistCtx, OAuthRefreshTokenKeyForWorkspace(st.WorkspaceID), "oauth_refresh_token", tok.RefreshToken); err != nil {
		return nil, fmt.Errorf("store refresh token: %w", err)
	}
	if err := s.secrets.Set(persistCtx, OAuthAccessTokenKeyForWorkspace(st.WorkspaceID), "oauth_access_token", tok.AccessToken); err != nil {
		return nil, fmt.Errorf("store access token: %w", err)
	}
	if err := s.store.UpsertConfigForWorkspace(persistCtx, st.WorkspaceID, cfg); err != nil {
		return nil, fmt.Errorf("persist config: %w", err)
	}
	s.invalidateClient(st.WorkspaceID)
	return cfg, nil
}

// RefreshOAuthToken uses the stored refresh token to get a new access token.
// PKCE-based flow: only client_id is sent (no client_secret). Refresh tokens
// are rotating — the old one is invalidated and a new one is returned.
func (s *Service) RefreshOAuthToken(ctx context.Context, workspaceID, staleToken string) error {
	resolvedWorkspaceID, err := s.resolveWorkspaceID(ctx, workspaceID)
	if err != nil {
		return err
	}
	workspaceID = resolvedWorkspaceID
	s.oauthMu.Lock(workspaceID)
	defer s.oauthMu.Unlock(workspaceID)
	return s.refreshOAuthTokenLocked(ctx, workspaceID, staleToken)
}

func (s *Service) refreshOAuthTokenLocked(ctx context.Context, workspaceID, staleToken string) error {
	// If another goroutine already rotated the token while we waited for the
	// lock, skip: rotating again with the now-invalid refresh token would fail.
	if s.oauthRefreshAlreadyCompleted(ctx, workspaceID, staleToken) {
		return nil
	}
	refreshToken, err := s.secrets.Reveal(ctx, OAuthRefreshTokenKeyForWorkspace(workspaceID))
	if err != nil {
		return fmt.Errorf("read refresh token: %w", err)
	}
	if refreshToken == "" {
		return errors.New("no refresh token — re-authentication required")
	}
	cfg, err := s.store.GetConfigForWorkspace(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("read config: %w", err)
	}
	if cfg == nil {
		return errors.New("no config for workspace")
	}
	endpoints, err := discoverMCPOAuthEndpoints(ctx, newMCPOAuthHTTPClient())
	if err != nil {
		return err
	}
	tok, err := refreshMCPToken(ctx, newMCPOAuthHTTPClient(), endpoints.Token, cfg.ClientID, refreshToken)
	if err != nil {
		return fmt.Errorf("refresh: %w", err)
	}
	if tok.AccessToken == "" {
		return errors.New("token endpoint returned an empty access token")
	}
	return s.persistRefreshedOAuthToken(ctx, workspaceID, refreshToken, tok)
}

func (s *Service) oauthRefreshAlreadyCompleted(ctx context.Context, workspaceID, staleToken string) bool {
	if staleToken == "" {
		return false
	}
	current, err := s.secrets.Reveal(ctx, OAuthAccessTokenKeyForWorkspace(workspaceID))
	return err == nil && current != "" && current != staleToken
}

func (s *Service) persistRefreshedOAuthToken(
	ctx context.Context,
	workspaceID, previousRefreshToken string,
	tok *mcpTokenResponse,
) error {
	persistCtx, cancel := oauthPersistenceContext(ctx)
	defer cancel()
	newRefreshToken := tok.RefreshToken
	if newRefreshToken == "" {
		newRefreshToken = previousRefreshToken
	}
	if err := s.secrets.Set(persistCtx, OAuthRefreshTokenKeyForWorkspace(workspaceID), "oauth_refresh_token", newRefreshToken); err != nil {
		return fmt.Errorf("store refresh token: %w", err)
	}
	if err := s.secrets.Set(persistCtx, OAuthAccessTokenKeyForWorkspace(workspaceID), "oauth_access_token", tok.AccessToken); err != nil {
		return fmt.Errorf("store access token: %w", err)
	}
	expiresAt := time.Now().Add(time.Duration(tok.ExpiresIn) * time.Second)
	if err := s.store.UpdateTokenExpiryForWorkspace(persistCtx, workspaceID, expiresAt); err != nil {
		return fmt.Errorf("update token expiry: %w", err)
	}
	s.invalidateClient(workspaceID)
	return nil
}

// exchangeMCPCode swaps the authorization code for tokens using PKCE.
// No client_secret is sent — token_endpoint_auth_method="none".
func exchangeMCPCode(ctx context.Context, client *http.Client, tokenEndpoint, clientID, code, redirectURI, codeVerifier string) (*mcpTokenResponse, error) {
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"client_id":     {clientID},
		"code_verifier": {codeVerifier},
		"resource":      {mcpResourceURL},
	}
	return postMCPTokenRequest(ctx, client, tokenEndpoint, form.Encode())
}

// refreshMCPToken uses a refresh token to get a new access token. Only
// client_id is sent (no client_secret — PKCE public client).
func refreshMCPToken(ctx context.Context, client *http.Client, tokenEndpoint, clientID, refreshToken string) (*mcpTokenResponse, error) {
	form := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
		"client_id":     {clientID},
		"resource":      {mcpResourceURL},
	}
	return postMCPTokenRequest(ctx, client, tokenEndpoint, form.Encode())
}

func postMCPTokenRequest(ctx context.Context, client *http.Client, tokenEndpoint, body string) (*mcpTokenResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenEndpoint, strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var errResp struct {
			Error            string `json:"error"`
			ErrorDescription string `json:"error_description"`
		}
		_ = json.Unmarshal(raw, &errResp)
		if errResp.Error != "" {
			return nil, fmt.Errorf("%s: %s", errResp.Error, errResp.ErrorDescription)
		}
		return nil, fmt.Errorf("token endpoint returned %d: %s", resp.StatusCode, string(raw))
	}
	var tok mcpTokenResponse
	if err := json.Unmarshal(raw, &tok); err != nil {
		return nil, fmt.Errorf("decode token response: %w", err)
	}
	return &tok, nil
}

// resolveCloudIDViaMCP uses the MCP getAccessibleAtlassianResources tool to
// resolve the cloudId. The MCP OAuth token doesn't work for direct
// api.atlassian.com calls, but it works through the MCP server.
func resolveCloudIDViaMCP(ctx context.Context, accessToken, siteURL string) (string, error) {
	client := NewMCPClient(accessToken, "", "", siteURL)
	result, err := client.call(ctx, "tools/call", map[string]interface{}{
		"name": "getAccessibleAtlassianResources", "arguments": map[string]interface{}{},
	})
	if err != nil {
		return "", err
	}
	return matchAccessibleResource(result, siteURL)
}

// matchAccessibleResource parses accessible-resources JSON and matches by URL.
func matchAccessibleResource(raw, siteURL string) (string, error) {
	var resources []accessibleResource
	if err := json.Unmarshal([]byte(raw), &resources); err != nil {
		return "", fmt.Errorf("decode accessible-resources: %w", err)
	}
	normalized := strings.TrimRight(siteURL, "/")
	if normalized != "" && !strings.Contains(normalized, "://") {
		normalized = "https://" + normalized
	}
	for _, r := range resources {
		if strings.TrimRight(r.URL, "/") == normalized {
			return r.ID, nil
		}
	}
	return "", fmt.Errorf("site URL %q not found in accessible resources", siteURL)
}

// (resolveCloudID removed — the MCP OAuth token doesn't work for direct
// api.atlassian.com calls, so resolveCloudIDViaMCP is used instead.)

// accessibleResource is one entry from the accessible-resources response.
type accessibleResource struct {
	ID     string   `json:"id"`
	Name   string   `json:"name"`
	URL    string   `json:"url"`
	Scopes []string `json:"scopes"`
}

// perWorkspaceMutex provides a lock per workspace ID for OAuth token refresh.
type perWorkspaceMutex struct {
	mu    sync.Mutex
	locks map[string]*workspaceLock
}

type workspaceLock struct {
	mu   sync.Mutex
	refs int
}

func newPerWorkspaceMutex() *perWorkspaceMutex {
	return &perWorkspaceMutex{locks: make(map[string]*workspaceLock)}
}

func (p *perWorkspaceMutex) Lock(workspaceID string) {
	p.mu.Lock()
	entry, ok := p.locks[workspaceID]
	if !ok {
		entry = &workspaceLock{}
		p.locks[workspaceID] = entry
	}
	entry.refs++
	p.mu.Unlock()
	entry.mu.Lock()
}

func (p *perWorkspaceMutex) Unlock(workspaceID string) {
	p.mu.Lock()
	entry, ok := p.locks[workspaceID]
	p.mu.Unlock()
	if !ok {
		return
	}
	entry.mu.Unlock()
	p.mu.Lock()
	entry.refs--
	if entry.refs == 0 {
		delete(p.locks, workspaceID)
	}
	p.mu.Unlock()
}
