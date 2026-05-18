package usage

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const (
	claudeUsageURL   = "https://api.anthropic.com/api/oauth/usage"
	claudeRefreshURL = "https://platform.claude.com/v1/oauth/token"
	claudeBetaHeader = "oauth-2025-04-20"
)

// ClaudeUsageClient fetches utilization from the Anthropic OAuth usage API.
type ClaudeUsageClient struct {
	credentialsPath string
	httpClient      *http.Client
}

// NewClaudeUsageClient creates a client that reads from the default credentials path.
func NewClaudeUsageClient() *ClaudeUsageClient {
	home, _ := os.UserHomeDir()
	return &ClaudeUsageClient{
		credentialsPath: filepath.Join(home, ".claude", ".credentials.json"),
		httpClient:      &http.Client{Timeout: 10 * time.Second},
	}
}

// NewClaudeUsageClientWithPath creates a client with an explicit credentials path (for tests).
func NewClaudeUsageClientWithPath(credentialsPath string) *ClaudeUsageClient {
	return &ClaudeUsageClient{
		credentialsPath: credentialsPath,
		httpClient:      &http.Client{Timeout: 10 * time.Second},
	}
}

// CredentialsPath returns the path this client reads credentials from.
func (c *ClaudeUsageClient) CredentialsPath() string {
	return c.credentialsPath
}

type claudeCredentials struct {
	ClaudeAiOauth *claudeOAuthToken `json:"claudeAiOauth"`
}

type claudeOAuthToken struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken,omitempty"`
	ExpiresAt    int64  `json:"expiresAt"` // Unix milliseconds
}

type claudeUsageResponse struct {
	FiveHour struct {
		Utilization int    `json:"utilization"`
		ResetsAt    string `json:"resets_at,omitempty"`
	} `json:"five_hour"`
	SevenDay struct {
		Utilization int    `json:"utilization"`
		ResetsAt    string `json:"resets_at,omitempty"`
	} `json:"seven_day"`
}

// FetchUsage implements ProviderUsageClient.
func (c *ClaudeUsageClient) FetchUsage(ctx context.Context) (*ProviderUsage, error) {
	token, err := c.accessToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("claude usage: read token: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, claudeUsageURL, nil)
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

	var raw claudeUsageResponse
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("claude usage: decode: %w", err)
	}

	now := time.Now()
	windows := []UtilizationWindow{
		{
			Label:          "5-hour",
			UtilizationPct: float64(raw.FiveHour.Utilization),
			ResetAt:        parseResetAt(raw.FiveHour.ResetsAt, now, 5*time.Hour),
		},
		{
			Label:          "7-day",
			UtilizationPct: float64(raw.SevenDay.Utilization),
			ResetAt:        parseResetAt(raw.SevenDay.ResetsAt, now, 7*24*time.Hour),
		},
	}
	return &ProviderUsage{
		Provider:  "anthropic",
		Windows:   windows,
		FetchedAt: now,
	}, nil
}

// accessToken returns a valid access token, refreshing if expired.
func (c *ClaudeUsageClient) accessToken(ctx context.Context) (string, error) {
	creds, err := c.readCredentials()
	if err != nil {
		return "", err
	}
	if creds.ClaudeAiOauth == nil {
		return "", fmt.Errorf("no claudeAiOauth entry in credentials file")
	}
	tok := creds.ClaudeAiOauth
	// ExpiresAt is in milliseconds; treat as expired if within 60 s of now.
	expiresAt := time.UnixMilli(tok.ExpiresAt)
	if time.Until(expiresAt) > 60*time.Second {
		return tok.AccessToken, nil
	}
	// Token is expired or near expiry — refresh it.
	if tok.RefreshToken == "" {
		return "", fmt.Errorf("claude token expired and no refresh token available")
	}
	newTok, err := c.refreshToken(ctx, tok.RefreshToken)
	if err != nil {
		return "", fmt.Errorf("refresh token: %w", err)
	}
	// Write new token back. Non-fatal — we have the new token in memory even if
	// persistence fails, but log the error so it doesn't go unnoticed.
	creds.ClaudeAiOauth = newTok
	if writeErr := c.writeCredentials(creds); writeErr != nil {
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

func (c *ClaudeUsageClient) writeCredentials(creds *claudeCredentials) error {
	data, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(c.credentialsPath, data, 0600)
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
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, claudeRefreshURL, bytes.NewReader(body))
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
