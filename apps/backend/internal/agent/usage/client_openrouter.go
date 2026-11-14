package usage

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	openRouterProvider   = "openrouter"
	openRouterKeyURL     = "https://openrouter.ai/api/v1/key"
	openRouterDailyLabel = "1-day"
)

// OpenRouterUsageClient reads the account's free-model daily request count from
// the OpenRouter key endpoint. OpenRouter counts free-model requests across the
// whole account, so the provider's own counter is used instead of a local one.
type OpenRouterUsageClient struct {
	authPath   string
	keyURL     string
	httpClient *http.Client
	now        func() time.Time
}

func NewOpenRouterUsageClient(authPath string) *OpenRouterUsageClient {
	return &OpenRouterUsageClient{
		authPath: authPath,
		keyURL:   openRouterKeyURL,
		httpClient: &http.Client{
			Timeout:       10 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
		now: time.Now,
	}
}

type openRouterKeyResponse struct {
	Data struct {
		IsFreeTier             bool `json:"is_free_tier"`
		FreeModelDailyRequests *struct {
			Used      float64  `json:"used"`
			Limit     float64  `json:"limit"`
			Remaining *float64 `json:"remaining"`
		} `json:"free_model_daily_requests"`
	} `json:"data"`
}

// FetchUsage implements ProviderUsageClient.
func (c *OpenRouterUsageClient) FetchUsage(ctx context.Context) (*ProviderUsage, error) {
	key, err := c.apiKey()
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.keyURL, nil)
	if err != nil {
		return nil, &UsageReadError{Reason: "network"}
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Accept", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, &UsageReadError{Reason: "network"}
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		reason := "http_status"
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			reason = "unauthorized"
		}
		return nil, &UsageReadError{Reason: reason, Status: resp.StatusCode}
	}
	const maxResponse = 1 << 20
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponse+1))
	if err != nil {
		return nil, &UsageReadError{Reason: "network"}
	}
	var raw openRouterKeyResponse
	if len(body) > maxResponse || json.Unmarshal(body, &raw) != nil {
		return nil, &UsageReadError{Reason: "decode"}
	}
	return parseOpenRouterUsage(raw, c.now()), nil
}

func (c *OpenRouterUsageClient) apiKey() (string, error) {
	body, err := os.ReadFile(c.authPath)
	if err != nil {
		return "", &UsageReadError{Reason: "credential_missing"}
	}
	var entries map[string]struct {
		Type string `json:"type"`
		Key  string `json:"key"`
	}
	if json.Unmarshal(body, &entries) == nil {
		if key := strings.TrimSpace(entries[openRouterProvider].Key); key != "" {
			return key, nil
		}
	}
	return "", &UsageReadError{Reason: "credential_missing"}
}

// parseOpenRouterUsage turns the free-request counter into a daily window. The
// quota resets at midnight UTC, so the window spans the current UTC day. A
// response without the counter yields no window rather than a guessed count.
func parseOpenRouterUsage(raw openRouterKeyResponse, now time.Time) *ProviderUsage {
	plan := "credits"
	if raw.Data.IsFreeTier {
		plan = "free"
	}
	usage := &ProviderUsage{Provider: openRouterProvider, Plan: plan, Windows: []UtilizationWindow{}, FetchedAt: now}
	counter := raw.Data.FreeModelDailyRequests
	if counter == nil || counter.Limit <= 0 || counter.Used < 0 {
		return usage
	}
	remaining := counter.Limit - counter.Used
	if counter.Remaining != nil {
		remaining = *counter.Remaining
	}
	if remaining < 0 {
		remaining = 0
	}
	usage.FreeModelDailyRequests = &FreeModelCounter{Used: counter.Used, Limit: counter.Limit, Remaining: remaining}
	start := now.UTC().Truncate(24 * time.Hour)
	reset := start.Add(24 * time.Hour)
	usage.Windows = append(usage.Windows, UtilizationWindow{
		Label:           openRouterDailyLabel,
		UtilizationPct:  100 * counter.Used / counter.Limit,
		ResetAt:         reset,
		StartAt:         start,
		DurationSeconds: int64(24 * time.Hour / time.Second),
		Scope:           WindowScopeFreeModels,
		LimitReached:    counter.Used >= counter.Limit || remaining == 0,
	})
	return usage
}
