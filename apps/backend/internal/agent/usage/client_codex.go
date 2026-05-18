package usage

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

const codexUsageURL = "https://chatgpt.com/backend-api/wham/usage"

// CodexUsageClient fetches utilization from the OpenAI Codex usage API.
// It reads the bearer token from ~/.config/codex/auth.json and inspects
// response headers for utilization percentages. Fail-open: if the token is
// missing or the request fails, nil is returned (no utilization data).
type CodexUsageClient struct {
	authPath   string
	httpClient *http.Client
}

// NewCodexUsageClient creates a client that reads from the default auth path.
func NewCodexUsageClient() *CodexUsageClient {
	home, _ := os.UserHomeDir()
	return &CodexUsageClient{
		authPath:   filepath.Join(home, ".config", "codex", "auth.json"),
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// NewCodexUsageClientWithPath creates a client with an explicit auth path (for tests).
func NewCodexUsageClientWithPath(authPath string) *CodexUsageClient {
	return &CodexUsageClient{
		authPath:   authPath,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// AuthPath returns the path this client reads credentials from.
func (c *CodexUsageClient) AuthPath() string {
	return c.authPath
}

type codexAuthJSON struct {
	Token string `json:"token"`
}

// FetchUsage implements ProviderUsageClient.
// Returns nil, nil if the auth file is missing or the token cannot be read —
// this is the fail-open path for Codex.
func (c *CodexUsageClient) FetchUsage(ctx context.Context) (*ProviderUsage, error) {
	token, err := c.readToken()
	if err != nil {
		// Fail-open: no Codex auth available.
		return nil, nil //nolint:nilerr
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, codexUsageURL, nil)
	if err != nil {
		return nil, nil //nolint:nilerr
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, nil //nolint:nilerr
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, nil
	}

	primary := parseHeaderPercent(resp.Header.Get("x-codex-primary-used-percent"))
	secondary := parseHeaderPercent(resp.Header.Get("x-codex-secondary-used-percent"))

	now := time.Now()
	windows := []UtilizationWindow{
		{Label: "primary", UtilizationPct: primary, ResetAt: now.Add(24 * time.Hour)},
		{Label: "secondary", UtilizationPct: secondary, ResetAt: now.Add(7 * 24 * time.Hour)},
	}
	return &ProviderUsage{
		Provider:  "openai",
		Windows:   windows,
		FetchedAt: now,
	}, nil
}

func (c *CodexUsageClient) readToken() (string, error) {
	data, err := os.ReadFile(c.authPath)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", c.authPath, err)
	}
	var auth codexAuthJSON
	if err := json.Unmarshal(data, &auth); err != nil {
		return "", fmt.Errorf("parse %s: %w", c.authPath, err)
	}
	if auth.Token == "" {
		return "", fmt.Errorf("empty token in %s", c.authPath)
	}
	return auth.Token, nil
}

func parseHeaderPercent(v string) float64 {
	if v == "" {
		return 0
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0
	}
	return f
}
