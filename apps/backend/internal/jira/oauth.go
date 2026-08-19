package jira

import (
	"bytes"
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
)

// Atlassian MCP OAuth 2.1 endpoints. The MCP server acts as both
// authorization server and resource server, proxying to auth.atlassian.com.
// Dynamic Client Registration (RFC 7591) + PKCE (S256) replaces the need for
// a pre-created app — no client_id/client_secret from the developer console.
const (
	mcpAuthorizeURL     = "https://mcp.atlassian.com/v1/authorize"
	mcpTokenURL         = "https://mcp.atlassian.com/v1/token"
	mcpRegisterURL      = "https://mcp.atlassian.com/v1/register"
	mcpResourceURL      = "https://mcp.atlassian.com"
	mcpScopes           = "offline_access read:jira-user read:jira-work write:jira-work"
	mcpStateTTL         = 10 * time.Minute
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
	RedirectURIs           []string `json:"redirect_uris"`
	GrantTypes              []string `json:"grant_types"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
}

// mcpPendingState holds the CSRF state + PKCE verifier during the OAuth flow.
type mcpPendingState struct {
	State        string
	WorkspaceID  string
	SiteURL      string
	ClientID     string // from dynamic registration
	CodeVerifier string // PKCE verifier (kept server-side)
	RedirectURI  string
	CreatedAt    time.Time
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
func registerMCPClient(redirectURI string) (string, error) {
	body, _ := json.Marshal(map[string]interface{}{
		"client_name":                  "Kandev",
		"redirect_uris":                []string{redirectURI},
		"grant_types":                  []string{"authorization_code", "refresh_token"},
		"response_types":               []string{"code"},
		"token_endpoint_auth_method":   "none",
		"scope":                        mcpScopes,
	})
	req, err := http.NewRequest("POST", mcpRegisterURL, strings.NewReader(string(body)))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	client := &http.Client{Timeout: 15 * time.Second}
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

// StartOAuthFlow registers a client dynamically, generates PKCE, and builds the
// authorization URL. The user never needs to create an Atlassian app.
func (s *Service) StartOAuthFlow(ctx context.Context, workspaceID, siteURL, redirectURI string) (string, error) {
	if redirectURI == "" {
		return "", errors.New("redirect URI is required")
	}
	clientID, err := registerMCPClient(redirectURI)
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
		State:        state,
		WorkspaceID:  workspaceID,
		SiteURL:      siteURL,
		ClientID:     clientID,
		CodeVerifier: verifier,
		RedirectURI:  redirectURI,
		CreatedAt:    time.Now(),
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
	return mcpAuthorizeURL + "?" + params.Encode(), nil
}

// CompleteOAuthFlow exchanges the authorization code for tokens (using PKCE,
// no client_secret), resolves the cloudId, and persists config + secrets.
func (s *Service) CompleteOAuthFlow(ctx context.Context, code, state string) (*JiraConfig, error) {
	st, ok := s.oauthStates.loadAndDelete(state)
	if !ok || time.Since(st.CreatedAt) > mcpStateTTL {
		return nil, errors.New("invalid or expired OAuth state")
	}
	tok, err := exchangeMCPCode(st.ClientID, code, st.RedirectURI, st.CodeVerifier)
	if err != nil {
		return nil, fmt.Errorf("token exchange: %w", err)
	}
	cloudID, err := resolveCloudIDViaMCP(tok.AccessToken, st.SiteURL)
	if err != nil {
		// Don't block the flow — we'll resolve cloudId lazily on first API call.
		cloudID = ""
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
	if err := s.store.UpsertConfigForWorkspace(ctx, st.WorkspaceID, cfg); err != nil {
		return nil, fmt.Errorf("persist config: %w", err)
	}
	if err := s.secrets.Set(ctx, OAuthAccessTokenKeyForWorkspace(st.WorkspaceID), "oauth_access_token", tok.AccessToken); err != nil {
		return nil, fmt.Errorf("store access token: %w", err)
	}
	if tok.RefreshToken != "" {
		if err := s.secrets.Set(ctx, OAuthRefreshTokenKeyForWorkspace(st.WorkspaceID), "oauth_refresh_token", tok.RefreshToken); err != nil {
			return nil, fmt.Errorf("store refresh token: %w", err)
		}
	}
	s.invalidateClient(st.WorkspaceID)
	return cfg, nil
}

// RefreshOAuthToken uses the stored refresh token to get a new access token.
// PKCE-based flow: only client_id is sent (no client_secret). Refresh tokens
// are rotating — the old one is invalidated and a new one is returned.
func (s *Service) RefreshOAuthToken(ctx context.Context, workspaceID string) error {
	s.oauthMu.Lock(workspaceID)
	defer s.oauthMu.Unlock(workspaceID)

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
	tok, err := refreshMCPToken(cfg.ClientID, refreshToken)
	if err != nil {
		return fmt.Errorf("refresh: %w", err)
	}
	if err := s.secrets.Set(ctx, OAuthAccessTokenKeyForWorkspace(workspaceID), "oauth_access_token", tok.AccessToken); err != nil {
		return fmt.Errorf("store access token: %w", err)
	}
	if tok.RefreshToken != "" {
		if err := s.secrets.Set(ctx, OAuthRefreshTokenKeyForWorkspace(workspaceID), "oauth_refresh_token", tok.RefreshToken); err != nil {
			return fmt.Errorf("store refresh token: %w", err)
		}
	}
	expiresAt := time.Now().Add(time.Duration(tok.ExpiresIn) * time.Second)
	if err := s.store.UpdateTokenExpiryForWorkspace(ctx, workspaceID, expiresAt); err != nil {
		return fmt.Errorf("update token expiry: %w", err)
	}
	s.invalidateClient(workspaceID)
	return nil
}

// exchangeMCPCode swaps the authorization code for tokens using PKCE.
// No client_secret is sent — token_endpoint_auth_method="none".
func exchangeMCPCode(clientID, code, redirectURI, codeVerifier string) (*mcpTokenResponse, error) {
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":   {redirectURI},
		"client_id":      {clientID},
		"code_verifier":  {codeVerifier},
		"resource":       {mcpResourceURL},
	}
	return postMCPTokenRequest(form.Encode())
}

// refreshMCPToken uses a refresh token to get a new access token. Only
// client_id is sent (no client_secret — PKCE public client).
func refreshMCPToken(clientID, refreshToken string) (*mcpTokenResponse, error) {
	form := url.Values{
		"grant_type":     {"refresh_token"},
		"refresh_token":  {refreshToken},
		"client_id":      {clientID},
		"resource":       {mcpResourceURL},
	}
	return postMCPTokenRequest(form.Encode())
}

func postMCPTokenRequest(body string) (*mcpTokenResponse, error) {
	req, err := http.NewRequest("POST", mcpTokenURL, strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	client := &http.Client{Timeout: 15 * time.Second}
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
func resolveCloudIDViaMCP(accessToken, siteURL string) (string, error) {
	// Initialize MCP session
	initReq, _ := json.Marshal(mcpRequest{
		JSONRPC: "2.0",
		Method:  "initialize",
		Params: map[string]interface{}{
			"protocolVersion": mcpProtocol,
			"capabilities":    map[string]interface{}{},
			"clientInfo":      map[string]string{"name": "kandev", "version": "1.0"},
		},
		ID: nextMCPID(),
	})
	req, err := http.NewRequest("POST", mcpEndpoint, bytes.NewReader(initReq))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("MCP-Protocol-Version", mcpProtocol)
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	sessionID := resp.Header.Get("Mcp-Session-Id")
	_ = resp.Body.Close()
	if sessionID == "" {
		return "", errors.New("MCP initialize: no session ID")
	}

	// Call getAccessibleAtlassianResources
	callReq, _ := json.Marshal(mcpRequest{
		JSONRPC: "2.0",
		Method:  "tools/call",
		Params: map[string]interface{}{
			"name":      "getAccessibleAtlassianResources",
			"arguments": map[string]interface{}{},
		},
		ID: nextMCPID(),
	})
	req2, err := http.NewRequest("POST", mcpEndpoint, bytes.NewReader(callReq))
	if err != nil {
		return "", err
	}
	req2.Header.Set("Authorization", "Bearer "+accessToken)
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Accept", "application/json, text/event-stream")
	req2.Header.Set("MCP-Protocol-Version", mcpProtocol)
	req2.Header.Set("Mcp-Session-Id", sessionID)
	resp2, err := client.Do(req2)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp2.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp2.Body, 1<<20))
	if err != nil {
		return "", err
	}
	if resp2.StatusCode < 200 || resp2.StatusCode >= 300 {
		return "", fmt.Errorf("MCP getAccessibleAtlassianResources: %d: %s", resp2.StatusCode, string(raw))
	}
	data := parseSSE(raw)
	if data == "" {
		return "", errors.New("MCP returned empty response")
	}
	var mcpr mcpResponse
	if err := json.Unmarshal([]byte(data), &mcpr); err != nil {
		return "", fmt.Errorf("decode MCP response: %w", err)
	}
	if mcpr.Error != nil {
		return "", fmt.Errorf("MCP error %d: %s", mcpr.Error.Code, mcpr.Error.Message)
	}
	if len(mcpr.Result.Content) == 0 {
		return "", errors.New("MCP returned no content")
	}
	var resources []accessibleResource
	if err := json.Unmarshal([]byte(mcpr.Result.Content[0].Text), &resources); err != nil {
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

// resolveCloudID calls the accessible-resources endpoint directly. Only works
// with traditional OAuth 3LO tokens, not MCP tokens.
func resolveCloudID(accessToken, siteURL string) (string, error) {
	req, err := http.NewRequest("GET", "https://api.atlassian.com/oauth/token/accessible-resources", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")
	client := &http.Client{Timeout: 15 * time.Second}
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
		return "", fmt.Errorf("accessible-resources returned %d: %s", resp.StatusCode, string(raw))
	}
	var resources []accessibleResource
	if err := json.Unmarshal(raw, &resources); err != nil {
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
	if len(resources) > 0 {
		return resources[0].ID, nil
	}
	return "", errors.New("no accessible resources found")
}

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
	locks map[string]*sync.Mutex
}

func newPerWorkspaceMutex() *perWorkspaceMutex {
	return &perWorkspaceMutex{locks: make(map[string]*sync.Mutex)}
}

func (p *perWorkspaceMutex) Lock(workspaceID string) {
	p.mu.Lock()
	m, ok := p.locks[workspaceID]
	if !ok {
		m = &sync.Mutex{}
		p.locks[workspaceID] = m
	}
	p.mu.Unlock()
	m.Lock()
}

func (p *perWorkspaceMutex) Unlock(workspaceID string) {
	p.mu.Lock()
	m, ok := p.locks[workspaceID]
	p.mu.Unlock()
	if ok {
		m.Unlock()
	}
}
