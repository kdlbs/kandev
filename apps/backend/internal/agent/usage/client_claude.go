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
	claudeUsageURL   = "https://api.anthropic.com/api/oauth/usage"
	claudeRefreshURL = "https://platform.claude.com/v1/oauth/token"
	claudeBetaHeader = "oauth-2025-04-20"

	claudeLabel5Hour = "5-hour"
	claudeLabel7Day  = "7-day"
)

// ClaudeUsageClient fetches utilization from the Anthropic OAuth usage API.
type ClaudeUsageClient struct {
	credentialsPath string
	usageURL        string
	refreshURL      string
	httpClient      *http.Client
}

// NewClaudeUsageClientWithPath creates a client with an explicit credentials path (for tests).
func NewClaudeUsageClientWithPath(credentialsPath string) *ClaudeUsageClient {
	return &ClaudeUsageClient{
		credentialsPath: credentialsPath,
		usageURL:        claudeUsageURL,
		refreshURL:      claudeRefreshURL,
		httpClient:      &http.Client{Timeout: 10 * time.Second},
	}
}

// CredentialsPath returns the path this client reads credentials from.
func (c *ClaudeUsageClient) CredentialsPath() string {
	return c.credentialsPath
}

// HasSubscriptionCredentials reports whether the credentials file exists and
// carries an OAuth (subscription) token.
func (c *ClaudeUsageClient) HasSubscriptionCredentials() bool {
	creds, err := c.readCredentials()
	return err == nil && creds.ClaudeAiOauth != nil && creds.ClaudeAiOauth.AccessToken != ""
}

type claudeCredentials struct {
	ClaudeAiOauth *claudeOAuthToken `json:"claudeAiOauth"`
}

type claudeOAuthToken struct {
	AccessToken      string `json:"accessToken"`
	RefreshToken     string `json:"refreshToken,omitempty"`
	ExpiresAt        int64  `json:"expiresAt"` // Unix milliseconds
	SubscriptionType string `json:"subscriptionType,omitempty"`
}

type claudeUsageWindow struct {
	Utilization float64 `json:"utilization"`
	ResetsAt    string  `json:"resets_at,omitempty"`
}

type claudeLimitScope struct {
	Model *struct {
		DisplayName string `json:"display_name"`
	} `json:"model"`
}

type claudeLimit struct {
	Kind     string            `json:"kind"`
	Percent  float64           `json:"percent"`
	ResetsAt string            `json:"resets_at"`
	Scope    *claudeLimitScope `json:"scope"`
}

type claudeUsageResponse struct {
	FiveHour *claudeUsageWindow `json:"five_hour"`
	SevenDay *claudeUsageWindow `json:"seven_day"`
	Limits   []claudeLimit      `json:"limits"`
}

// FetchUsage implements ProviderUsageClient.
func (c *ClaudeUsageClient) FetchUsage(ctx context.Context) (*ProviderUsage, error) {
	creds, err := c.readCredentials()
	if err != nil {
		return nil, fmt.Errorf("claude usage: read credentials: %w", err)
	}
	if creds.ClaudeAiOauth == nil {
		return nil, fmt.Errorf("claude usage: no claudeAiOauth entry in %s", c.credentialsPath)
	}
	token, err := c.freshAccessToken(ctx, creds.ClaudeAiOauth)
	if err != nil {
		return nil, fmt.Errorf("claude usage: %w", err)
	}

	body, err := c.getUsage(ctx, token)
	if err != nil {
		return nil, err
	}

	var raw claudeUsageResponse
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("claude usage: decode: %w", err)
	}

	now := time.Now()
	return &ProviderUsage{
		Provider:  "anthropic",
		Plan:      creds.ClaudeAiOauth.SubscriptionType,
		Windows:   claudeWindows(raw, now),
		FetchedAt: now,
	}, nil
}

func (c *ClaudeUsageClient) getUsage(ctx context.Context, token string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.usageURL, nil)
	if err != nil {
		return nil, fmt.Errorf("claude usage: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("anthropic-beta", claudeBetaHeader)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("claude usage: http: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("claude usage: read body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("claude usage: unexpected status %d: %s", resp.StatusCode, body)
	}
	return body, nil
}

// claudeWindows prefers the richer limits[] array (session, weekly, per-model
// weekly) and falls back to the legacy five_hour/seven_day pair. Always
// returns a non-nil slice so the API serializes `windows` as an array.
func claudeWindows(raw claudeUsageResponse, now time.Time) []UtilizationWindow {
	if windows := claudeLimitWindows(raw.Limits); len(windows) > 0 {
		return windows
	}
	windows := make([]UtilizationWindow, 0, 2)
	if raw.FiveHour != nil {
		windows = append(windows, UtilizationWindow{
			Label:          claudeLabel5Hour,
			UtilizationPct: raw.FiveHour.Utilization,
			ResetAt:        parseResetAt(raw.FiveHour.ResetsAt, now, 5*time.Hour),
		})
	}
	if raw.SevenDay != nil {
		windows = append(windows, UtilizationWindow{
			Label:          claudeLabel7Day,
			UtilizationPct: raw.SevenDay.Utilization,
			ResetAt:        parseResetAt(raw.SevenDay.ResetsAt, now, 7*24*time.Hour),
		})
	}
	return windows
}

func claudeLimitWindows(limits []claudeLimit) []UtilizationWindow {
	var windows []UtilizationWindow
	for _, l := range limits {
		label := claudeLimitLabel(l)
		if label == "" {
			continue
		}
		resetAt, err := time.Parse(time.RFC3339, l.ResetsAt)
		if err != nil {
			continue
		}
		windows = append(windows, UtilizationWindow{
			Label:          label,
			UtilizationPct: l.Percent,
			ResetAt:        resetAt,
		})
	}
	return windows
}

func claudeLimitLabel(l claudeLimit) string {
	switch l.Kind {
	case "session":
		return claudeLabel5Hour
	case "weekly_all":
		return claudeLabel7Day
	case "weekly_scoped":
		if l.Scope != nil && l.Scope.Model != nil && l.Scope.Model.DisplayName != "" {
			return "7-day (" + l.Scope.Model.DisplayName + ")"
		}
		return "7-day (model)"
	default:
		// Unknown kinds are skipped rather than shown with a cryptic label.
		return ""
	}
}

// freshAccessToken returns a valid access token, refreshing if expired.
func (c *ClaudeUsageClient) freshAccessToken(ctx context.Context, tok *claudeOAuthToken) (string, error) {
	// ExpiresAt is in milliseconds; treat as expired if within 60 s of now.
	expiresAt := time.UnixMilli(tok.ExpiresAt)
	if time.Until(expiresAt) > 60*time.Second {
		return tok.AccessToken, nil
	}
	if tok.RefreshToken == "" {
		return "", fmt.Errorf("claude token expired and no refresh token available")
	}
	newTok, err := c.refreshToken(ctx, tok.RefreshToken)
	if err != nil {
		return "", fmt.Errorf("refresh token: %w", err)
	}
	// Write new token back. Non-fatal — we have the new token in memory even if
	// persistence fails, but log the error so it doesn't go unnoticed.
	if writeErr := c.persistRefreshedToken(newTok); writeErr != nil {
		fmt.Fprintf(os.Stderr, "claude usage: persist refreshed token: %v\n", writeErr)
	}
	return newTok.AccessToken, nil
}

func (c *ClaudeUsageClient) readCredentials() (*claudeCredentials, error) {
	data, err := os.ReadFile(c.credentialsPath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", c.credentialsPath, err)
	}
	var creds claudeCredentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return nil, fmt.Errorf("parse %s: %w", c.credentialsPath, err)
	}
	return &creds, nil
}

// persistRefreshedToken updates only the token fields inside claudeAiOauth,
// preserving unknown siblings (scopes, subscriptionType, rateLimitTier, ...).
func (c *ClaudeUsageClient) persistRefreshedToken(tok *claudeOAuthToken) error {
	data, err := os.ReadFile(c.credentialsPath)
	if err != nil {
		return err
	}
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		return err
	}
	oauth, _ := root["claudeAiOauth"].(map[string]any)
	if oauth == nil {
		oauth = map[string]any{}
	}
	oauth["accessToken"] = tok.AccessToken
	oauth["refreshToken"] = tok.RefreshToken
	oauth["expiresAt"] = tok.ExpiresAt
	root["claudeAiOauth"] = oauth
	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(c.credentialsPath, out, 0o600)
}

type claudeRefreshRequest struct {
	GrantType    string `json:"grant_type"`
	RefreshToken string `json:"refresh_token"`
}

type claudeRefreshResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"` // seconds
}

func (c *ClaudeUsageClient) refreshToken(ctx context.Context, refreshToken string) (*claudeOAuthToken, error) {
	payload := claudeRefreshRequest{GrantType: "refresh_token", RefreshToken: refreshToken}
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
	var r claudeRefreshResponse
	if err := json.Unmarshal(respBody, &r); err != nil {
		return nil, err
	}
	expiresAt := time.Now().Add(time.Duration(r.ExpiresIn) * time.Second).UnixMilli()
	newRefresh := r.RefreshToken
	if newRefresh == "" {
		newRefresh = refreshToken // keep old if not rotated
	}
	return &claudeOAuthToken{
		AccessToken:  r.AccessToken,
		RefreshToken: newRefresh,
		ExpiresAt:    expiresAt,
	}, nil
}

// parseResetAt parses an ISO timestamp or falls back to now+duration.
func parseResetAt(raw string, now time.Time, windowDuration time.Duration) time.Time {
	if raw != "" {
		if t, err := time.Parse(time.RFC3339, raw); err == nil {
			return t
		}
	}
	return now.Add(windowDuration)
}
