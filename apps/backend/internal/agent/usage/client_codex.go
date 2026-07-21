package usage

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

const (
	codexUsageURL      = "https://chatgpt.com/backend-api/wham/usage"
	codexRefreshURL    = "https://auth.openai.com/oauth/token"
	codexOAuthClientID = "app_EMoamEEZ73f0CkXaXp7hrann"
)

// CodexUsageClient fetches utilization from the ChatGPT Codex usage API.
// It reads OAuth tokens from ~/.codex/auth.json (written by `codex login`)
// and parses the rate-limit windows from the response body.
type CodexUsageClient struct {
	authPath   string
	usageURL   string
	refreshURL string
	httpClient *http.Client
}

// NewCodexUsageClientWithPath creates a client with an explicit auth path (for tests).
func NewCodexUsageClientWithPath(authPath string) *CodexUsageClient {
	return &CodexUsageClient{
		authPath:   authPath,
		usageURL:   codexUsageURL,
		refreshURL: codexRefreshURL,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// AuthPath returns the path this client reads credentials from.
func (c *CodexUsageClient) AuthPath() string {
	return c.authPath
}

// HasSubscriptionCredentials reports whether auth.json exists and carries
// ChatGPT OAuth tokens (an auth.json with only OPENAI_API_KEY is API-key billing).
func (c *CodexUsageClient) HasSubscriptionCredentials() bool {
	auth, err := c.readAuth()
	return err == nil && auth.Tokens != nil && auth.Tokens.AccessToken != ""
}

type codexAuthTokens struct {
	IDToken      string `json:"id_token"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	AccountID    string `json:"account_id"`
}

type codexAuthJSON struct {
	Tokens *codexAuthTokens `json:"tokens"`
}

type codexWindow struct {
	UsedPercent        float64 `json:"used_percent"`
	LimitWindowSeconds int64   `json:"limit_window_seconds"`
	ResetAfterSeconds  int64   `json:"reset_after_seconds"`
	ResetAt            int64   `json:"reset_at"` // Unix seconds
}

type codexUsageResponse struct {
	PlanType  string `json:"plan_type"`
	RateLimit struct {
		PrimaryWindow   *codexWindow `json:"primary_window"`
		SecondaryWindow *codexWindow `json:"secondary_window"`
	} `json:"rate_limit"`
}

// FetchUsage implements ProviderUsageClient.
func (c *CodexUsageClient) FetchUsage(ctx context.Context) (*ProviderUsage, error) {
	auth, err := c.readAuth()
	if err != nil {
		return nil, fmt.Errorf("codex usage: %w", err)
	}
	if auth.Tokens == nil || auth.Tokens.AccessToken == "" {
		return nil, fmt.Errorf("codex usage: no ChatGPT OAuth tokens in %s", c.authPath)
	}

	status, body, err := c.getUsage(ctx, auth.Tokens)
	if err != nil {
		return nil, err
	}
	if status == http.StatusUnauthorized && auth.Tokens.RefreshToken != "" {
		refreshed, refreshErr := c.refreshTokens(ctx, auth.Tokens)
		if refreshErr != nil {
			return nil, fmt.Errorf("codex usage: refresh token: %w", refreshErr)
		}
		status, body, err = c.getUsage(ctx, refreshed)
		if err != nil {
			return nil, err
		}
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("codex usage: unexpected status %d: %s", status, body)
	}
	return parseCodexUsage(body, time.Now())
}

func (c *CodexUsageClient) getUsage(ctx context.Context, tokens *codexAuthTokens) (int, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.usageURL, nil)
	if err != nil {
		return 0, nil, fmt.Errorf("codex usage: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+tokens.AccessToken)
	if tokens.AccountID != "" {
		req.Header.Set("chatgpt-account-id", tokens.AccountID)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("codex usage: http: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, fmt.Errorf("codex usage: read body: %w", err)
	}
	return resp.StatusCode, body, nil
}

func parseCodexUsage(body []byte, now time.Time) (*ProviderUsage, error) {
	var raw codexUsageResponse
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("codex usage: decode: %w", err)
	}
	// Non-nil so the API serializes `windows` as an array even when empty.
	windows := make([]UtilizationWindow, 0, 2)
	for _, w := range []*codexWindow{raw.RateLimit.PrimaryWindow, raw.RateLimit.SecondaryWindow} {
		if w == nil {
			continue
		}
		windows = append(windows, UtilizationWindow{
			Label:          codexWindowLabel(w.LimitWindowSeconds),
			UtilizationPct: w.UsedPercent,
			ResetAt:        codexResetAt(w, now),
		})
	}
	return &ProviderUsage{
		Provider:  "openai",
		Plan:      raw.PlanType,
		Windows:   windows,
		FetchedAt: now,
	}, nil
}

// codexWindowLabel renders a window duration like the Claude labels:
// 18000 s → "5-hour", 604800 s → "7-day", 2592000 s → "30-day".
func codexWindowLabel(seconds int64) string {
	hours := seconds / 3600
	switch {
	case hours <= 0:
		return "current"
	case hours < 24:
		return fmt.Sprintf("%d-hour", hours)
	default:
		return fmt.Sprintf("%d-day", hours/24)
	}
}

func codexResetAt(w *codexWindow, now time.Time) time.Time {
	if w.ResetAt > 0 {
		return time.Unix(w.ResetAt, 0)
	}
	return now.Add(time.Duration(w.ResetAfterSeconds) * time.Second)
}

func (c *CodexUsageClient) readAuth() (*codexAuthJSON, error) {
	data, err := os.ReadFile(c.authPath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", c.authPath, err)
	}
	var auth codexAuthJSON
	if err := json.Unmarshal(data, &auth); err != nil {
		return nil, fmt.Errorf("parse %s: %w", c.authPath, err)
	}
	return &auth, nil
}

type codexRefreshRequest struct {
	ClientID     string `json:"client_id"`
	GrantType    string `json:"grant_type"`
	RefreshToken string `json:"refresh_token"`
	Scope        string `json:"scope"`
}

type codexRefreshResponse struct {
	IDToken      string `json:"id_token"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

func (c *CodexUsageClient) refreshTokens(ctx context.Context, old *codexAuthTokens) (*codexAuthTokens, error) {
	payload := codexRefreshRequest{
		ClientID:     codexOAuthClientID,
		GrantType:    "refresh_token",
		RefreshToken: old.RefreshToken,
		Scope:        "openid profile email",
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.refreshURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("refresh: status %d: %s", resp.StatusCode, respBody)
	}
	var r codexRefreshResponse
	if err := json.Unmarshal(respBody, &r); err != nil {
		return nil, err
	}
	if r.AccessToken == "" {
		// Never persist an empty access token: that would wipe working
		// credentials and hide Codex usage until the next `codex login`.
		return nil, fmt.Errorf("refresh: empty access_token in response")
	}

	updated := &codexAuthTokens{
		IDToken:      valueOr(r.IDToken, old.IDToken),
		AccessToken:  r.AccessToken,
		RefreshToken: valueOr(r.RefreshToken, old.RefreshToken),
		AccountID:    old.AccountID,
	}
	// Non-fatal — we have the new tokens in memory even if persistence fails.
	if writeErr := c.persistTokens(updated); writeErr != nil {
		fmt.Fprintf(os.Stderr, "codex usage: persist refreshed tokens: %v\n", writeErr)
	}
	return updated, nil
}

// persistTokens updates only the tokens/last_refresh fields in auth.json,
// preserving unknown siblings (OPENAI_API_KEY, future fields).
func (c *CodexUsageClient) persistTokens(tokens *codexAuthTokens) error {
	data, err := os.ReadFile(c.authPath)
	if err != nil {
		return err
	}
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		return err
	}
	toks, _ := root["tokens"].(map[string]any)
	if toks == nil {
		toks = map[string]any{}
	}
	toks["id_token"] = tokens.IDToken
	toks["access_token"] = tokens.AccessToken
	toks["refresh_token"] = tokens.RefreshToken
	toks["account_id"] = tokens.AccountID
	root["tokens"] = toks
	root["last_refresh"] = time.Now().UTC().Format(time.RFC3339)
	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(c.authPath, out, 0o600)
}

func valueOr(v, fallback string) string {
	if v != "" {
		return v
	}
	return fallback
}
