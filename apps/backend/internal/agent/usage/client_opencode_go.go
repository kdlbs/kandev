package usage

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const openCodeGoProvider = "opencode-go"

// UsageReadError exposes only a bounded failure, never a provider body or key.
type UsageReadError struct {
	Reason string
	Status int
}

func (e *UsageReadError) Error() string { return "provider usage: " + e.Reason }

// UsageReadFailure returns safe metadata, including for existing provider clients.
func UsageReadFailure(err error) (string, int) {
	var failure *UsageReadError
	if errors.As(err, &failure) {
		return failure.Reason, failure.Status
	}
	return "fetch_failed", 0
}

// OpenCodeAuthPath matches the local OpenCode CLI's XDG data credential file.
func OpenCodeAuthPath(home string) string {
	if dir := strings.TrimSpace(os.Getenv("XDG_DATA_HOME")); dir != "" {
		return filepath.Join(dir, "opencode", "auth.json")
	}
	return filepath.Join(home, ".local", "share", "opencode", "auth.json")
}

// OpenCodeGoUsageClient reads subscription windows shared by the account's paid
// models. Free Go models must not be bound to this client by a caller.
type OpenCodeGoUsageClient struct {
	authPath   string
	usageURL   string
	httpClient *http.Client
}

func NewOpenCodeGoUsageClient(authPath string) *OpenCodeGoUsageClient {
	return &OpenCodeGoUsageClient{
		authPath: authPath,
		usageURL: "https://opencode.ai/zen/go/v1/usage",
		httpClient: &http.Client{
			Timeout:       10 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}
}

type openCodeGoWindow struct {
	Status   string   `json:"status"`
	Percent  *float64 `json:"percent"`
	ResetsAt string   `json:"resetsAt"`
}

type openCodeGoResponse struct {
	Usage struct {
		Rolling *openCodeGoWindow `json:"rolling"`
		Weekly  *openCodeGoWindow `json:"weekly"`
		Monthly *openCodeGoWindow `json:"monthly"`
	} `json:"usage"`
}

func (c *OpenCodeGoUsageClient) FetchUsage(ctx context.Context) (*ProviderUsage, error) {
	key, err := c.apiKey()
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.usageURL, nil)
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
	var raw openCodeGoResponse
	if len(body) > maxResponse || json.Unmarshal(body, &raw) != nil {
		return nil, &UsageReadError{Reason: "decode"}
	}
	windows := openCodeGoWindows(raw)
	if len(windows) == 0 {
		return nil, &UsageReadError{Reason: "decode"}
	}
	return &ProviderUsage{Provider: openCodeGoProvider, Plan: "go", Windows: windows, FetchedAt: time.Now()}, nil
}

func (c *OpenCodeGoUsageClient) apiKey() (string, error) {
	body, err := os.ReadFile(c.authPath)
	if err != nil {
		return "", &UsageReadError{Reason: "credential_missing"}
	}
	var entries map[string]struct {
		Type string `json:"type"`
		Key  string `json:"key"`
	}
	if json.Unmarshal(body, &entries) == nil {
		for _, provider := range []string{openCodeGoProvider, "opencode"} {
			entry := entries[provider]
			if entry.Type == "api" && strings.TrimSpace(entry.Key) != "" {
				return strings.TrimSpace(entry.Key), nil
			}
		}
	}
	return "", &UsageReadError{Reason: "credential_missing"}
}

// HasCredentials verifies local binding without fetching paid utilization.
func (c *OpenCodeGoUsageClient) HasCredentials() bool {
	_, err := c.apiKey()
	return err == nil
}

func openCodeGoWindows(raw openCodeGoResponse) []UtilizationWindow {
	windows := make([]UtilizationWindow, 0, 3)
	for _, entry := range []struct {
		label string
		raw   *openCodeGoWindow
		span  time.Duration
	}{
		{"5-hour", raw.Usage.Rolling, 5 * time.Hour},
		{"7-day", raw.Usage.Weekly, 7 * 24 * time.Hour},
		{"monthly", raw.Usage.Monthly, 0},
	} {
		if entry.raw == nil || entry.raw.Percent == nil || *entry.raw.Percent < 0 {
			continue
		}
		w := UtilizationWindow{Label: entry.label, UtilizationPct: *entry.raw.Percent, Scope: "paid_models",
			Observation: &WindowObservation{UtilizationObserved: true}}
		w.LimitReached = entry.raw.Status != "" && !strings.EqualFold(entry.raw.Status, "ok")
		if reset, err := time.Parse(time.RFC3339, entry.raw.ResetsAt); err == nil {
			w.ResetAt = reset
			w.Observation.ResetAtObserved = !reset.IsZero()
			w.StartAt = reset.Add(-entry.span)
			if entry.span == 0 {
				w.StartAt = reset.AddDate(0, -1, 0)
			}
			w.DurationSeconds = int64(reset.Sub(w.StartAt) / time.Second)
		}
		windows = append(windows, w)
	}
	return windows
}
