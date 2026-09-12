package usage

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const teamClaudeStatusResponseLimit = 1 << 20

// TeamClaudeUsageClient reads the local TeamClaude control-plane status. It
// reports the best usable account for each quota window, which reflects the
// capacity TeamClaude can actually route to rather than exposing a particular
// account's identity or credentials.
type TeamClaudeUsageClient struct {
	statusURL  string
	httpClient *http.Client
}

// NewTeamClaudeUsageClient creates a client for a validated TeamClaude status
// URL. The backend adapter limits automatic discovery to loopback URLs; the
// client refuses to follow a redirect away from that destination, since
// nothing revalidates a redirect target against the same loopback rule.
func NewTeamClaudeUsageClient(statusURL string) *TeamClaudeUsageClient {
	return &TeamClaudeUsageClient{
		statusURL: statusURL,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

type teamClaudeStatus struct {
	Accounts []teamClaudeAccount `json:"accounts"`
}

type teamClaudeAccount struct {
	Disabled    bool            `json:"disabled"`
	Status      string          `json:"status"`
	Unavailable *string         `json:"unavailable"`
	Quota       teamClaudeQuota `json:"quota"`
}

type teamClaudeQuota struct {
	Unified5Hour      *float64 `json:"unified5h"`
	Unified7Day       *float64 `json:"unified7d"`
	Unified7DaySonnet *float64 `json:"unified7dSonnet"`
	Unified7DayFable  *float64 `json:"unified7dFable"`
	Unified5HourReset int64    `json:"unified5hReset"`
	Unified7DayReset  int64    `json:"unified7dReset"`
	Sonnet7DayReset   int64    `json:"unified7dSonnetReset"`
	Fable7DayReset    int64    `json:"unified7dFableReset"`
}

// FetchUsage implements ProviderUsageClient.
func (c *TeamClaudeUsageClient) FetchUsage(ctx context.Context) (*ProviderUsage, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.statusURL, nil)
	if err != nil {
		return nil, fmt.Errorf("teamclaude usage: build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("teamclaude usage: http: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("teamclaude usage: unexpected status %d", resp.StatusCode)
	}
	var status teamClaudeStatus
	if err := json.NewDecoder(io.LimitReader(resp.Body, teamClaudeStatusResponseLimit)).Decode(&status); err != nil {
		return nil, fmt.Errorf("teamclaude usage: decode status: %w", err)
	}
	if status.Accounts == nil {
		return nil, fmt.Errorf("teamclaude usage: invalid status payload")
	}
	now := time.Now().UTC()
	available := 0
	for _, account := range status.Accounts {
		if teamClaudeAccountAvailable(account) {
			available++
		}
	}
	if available == 0 {
		// An empty pool (or one where every account is disabled, throttled, or
		// unavailable) means TeamClaude cannot route a request at all. Returning
		// a success with zero windows here would cache that as "not limited" —
		// IsPotentiallyRateLimited finds nothing to compare against an empty
		// Windows slice — masking a total outage as healthy capacity.
		return nil, fmt.Errorf("teamclaude usage: no usable account in pool of %d", len(status.Accounts))
	}
	return &ProviderUsage{
		Provider:  "teamclaude",
		Plan:      fmt.Sprintf("pool (%d available of %d)", available, len(status.Accounts)),
		Windows:   teamClaudePoolWindows(status.Accounts),
		FetchedAt: now,
	}, nil
}

func teamClaudeAccountAvailable(account teamClaudeAccount) bool {
	return !account.Disabled && account.Status == "active" &&
		(account.Unavailable == nil || *account.Unavailable == "")
}

type teamClaudeWindowSpec struct {
	label       string
	utilization func(teamClaudeQuota) *float64
	resetAt     func(teamClaudeQuota) int64
}

func teamClaudePoolWindows(accounts []teamClaudeAccount) []UtilizationWindow {
	specs := []teamClaudeWindowSpec{
		{"5-hour (pool)", func(q teamClaudeQuota) *float64 { return q.Unified5Hour }, func(q teamClaudeQuota) int64 { return q.Unified5HourReset }},
		{"7-day (pool)", func(q teamClaudeQuota) *float64 { return q.Unified7Day }, func(q teamClaudeQuota) int64 { return q.Unified7DayReset }},
		{"7-day Sonnet (pool)", func(q teamClaudeQuota) *float64 { return q.Unified7DaySonnet }, func(q teamClaudeQuota) int64 { return q.Sonnet7DayReset }},
		{"7-day Fable (pool)", func(q teamClaudeQuota) *float64 { return q.Unified7DayFable }, func(q teamClaudeQuota) int64 { return q.Fable7DayReset }},
	}
	windows := make([]UtilizationWindow, 0, len(specs))
	for _, spec := range specs {
		var best *UtilizationWindow
		for _, account := range accounts {
			if !teamClaudeAccountAvailable(account) {
				continue
			}
			utilization := spec.utilization(account.Quota)
			if utilization == nil || *utilization < 0 || *utilization > 1 {
				continue
			}
			candidate := UtilizationWindow{
				Label:          spec.label,
				UtilizationPct: *utilization * 100,
			}
			if resetAt := spec.resetAt(account.Quota); resetAt > 0 {
				candidate.ResetAt = time.UnixMilli(resetAt).UTC()
			}
			if best == nil || candidate.UtilizationPct < best.UtilizationPct {
				best = &candidate
			}
		}
		if best != nil {
			windows = append(windows, *best)
		}
	}
	return windows
}
