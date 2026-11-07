package usage

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"
)

// This account-aware client leaves the legacy OAuth usage client unchanged.
// Its profile and usage requests use the same resolved token snapshot.
type claudeExecutionUsageClient struct {
	base                    *ClaudeUsageClient
	directToken, profileURL string
	mu                      sync.Mutex
	identityToken           string
	identity                executionAccount
	identityAt              time.Time
}

func (c *claudeExecutionUsageClient) matchesSource(source executionUsageSource) bool {
	return source.provider == "anthropic" && source.path == c.base.credentialsPath && source.token == c.directToken
}

func (c *claudeExecutionUsageClient) token(ctx context.Context) (string, string, error) {
	if c.directToken != "" {
		return c.directToken, "", nil
	}
	creds, err := c.base.readCredentials()
	if err != nil {
		return "", "", accountFailure("credentials_read", 0, err)
	}
	if creds.ClaudeAiOauth == nil || creds.ClaudeAiOauth.AccessToken == "" {
		return "", "", accountFailure("credentials_missing_oauth", 0, nil)
	}
	token, err := c.base.freshAccessToken(ctx, creds.ClaudeAiOauth)
	if err != nil {
		return "", "", accountFailure("token_refresh", 0, err)
	}
	return token, creds.ClaudeAiOauth.SubscriptionType, nil
}

func (c *claudeExecutionUsageClient) resolveIdentity(ctx context.Context, token string) (executionAccount, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.identityToken == token && !c.identityAt.IsZero() && time.Since(c.identityAt) <= cacheTTL {
		return c.identity, nil
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.profileURL, nil)
	if err != nil {
		return executionAccount{}, accountFailure("profile_request", 0, err)
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Cache-Control", "no-cache")
	response, err := c.base.httpClient.Do(request)
	if err != nil {
		return executionAccount{}, accountFailure("profile_transport", 0, err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return executionAccount{}, accountFailure("profile_http", response.StatusCode, nil)
	}
	var profile struct {
		Account struct {
			UUID string `json:"uuid"`
		} `json:"account"`
		Organization struct {
			UUID string `json:"uuid"`
		} `json:"organization"`
	}
	if err := json.NewDecoder(response.Body).Decode(&profile); err != nil {
		return executionAccount{}, accountFailure("profile_decode", 0, err)
	}
	if profile.Account.UUID == "" || profile.Organization.UUID == "" {
		return executionAccount{}, accountFailure("profile_identity", 0, nil)
	}
	c.identity = executionAccount{account: profile.Account.UUID, organization: profile.Organization.UUID}
	c.identityToken, c.identityAt = token, time.Now()
	return c.identity, nil
}

func (c *claudeExecutionUsageClient) accountIdentity(ctx context.Context) (executionAccount, error) {
	token, _, err := c.token(ctx)
	if err != nil {
		return executionAccount{}, err
	}
	return c.resolveIdentity(ctx, token)
}

func (c *claudeExecutionUsageClient) FetchUsage(ctx context.Context) (*ProviderUsage, error) {
	token, plan, err := c.token(ctx)
	if err != nil {
		return nil, err
	}
	identity, err := c.resolveIdentity(ctx, token)
	if err != nil {
		return nil, err
	}
	body, err := c.base.getUsage(ctx, token)
	if err != nil {
		return nil, err
	}
	var raw claudeExecutionUsageResponse
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}
	now := time.Now()
	return &ProviderUsage{AccountRef: identity.account, OrganizationRef: identity.organization,
		Provider: "anthropic", Plan: plan, Windows: claudeExecutionWindows(raw, now), FetchedAt: now}, nil
}

// Native-only compatibility fields are verified against the installed official
// CLI schema. The legacy client's response and wire behavior are unchanged.
type claudeExecutionUsageResponse struct {
	claudeUsageResponse
	SevenDayOpus   *claudeUsageWindow `json:"seven_day_opus"`
	SevenDaySonnet *claudeUsageWindow `json:"seven_day_sonnet"`
}

// Retain each observed bucket even when its reset is absent. Rich rows take
// precedence for the same label/scope; legacy rows fill missing buckets.
func claudeExecutionWindows(raw claudeExecutionUsageResponse, _ time.Time) []UtilizationWindow {
	windows := make([]UtilizationWindow, 0, len(raw.Limits))
	for _, limit := range raw.Limits {
		label := claudeLimitLabel(limit)
		if label == "" {
			continue
		}
		windows = appendClaudeObservedWindow(windows, label, claudeLimitModel(limit), limit.Percent, limit.ResetsAt)
	}
	for _, legacy := range []struct {
		label, model string
		window       *claudeUsageWindow
	}{
		{claudeLabel5Hour, "", raw.FiveHour}, {claudeLabel7Day, "", raw.SevenDay},
		{"7-day (Opus)", "Opus", raw.SevenDayOpus}, {"7-day (Sonnet)", "Sonnet", raw.SevenDaySonnet},
	} {
		if legacy.window == nil {
			continue
		}
		windows = appendClaudeObservedWindow(windows, legacy.label, legacy.model, legacy.window.Utilization, legacy.window.ResetsAt)
	}
	return windows
}

func appendClaudeObservedWindow(windows []UtilizationWindow, label, model string, value *float64, resetText string) []UtilizationWindow {
	reset, err := time.Parse(time.RFC3339, resetText)
	item := UtilizationWindow{Label: label, Model: model, UtilizationPct: legacyUtilization(value), ResetAt: reset,
		Observation: &WindowObservation{UtilizationObserved: nativeUtilizationObserved(value), ResetAtObserved: err == nil}}
	for index, prior := range windows {
		if prior.Label != label || !strings.EqualFold(prior.Model, model) {
			continue
		}
		if prior.Observation == nil || !prior.Observation.UtilizationObserved {
			windows[index] = item
		}
		return windows
	}
	return append(windows, item)
}

func nativeUtilizationObserved(value *float64) bool {
	return value != nil && *value >= 0 && !math.IsNaN(*value) && !math.IsInf(*value, 0)
}
