package updates

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// defaultClientTimeout caps the GitHub poll when the caller passes a nil
// http.Client. The 6h interval already provides plenty of slack; an
// unbounded timeout could hang a goroutine for hours on a stalled socket.
const defaultClientTimeout = 30 * time.Second

// DefaultReleaseURL is the GitHub Releases API endpoint polled by the updates
// service. Exposed as a package-level variable so tests can point a stub
// server at the github client without surgery on Service.
var DefaultReleaseURL = "https://api.github.com/repos/kdlbs/kandev/releases/latest"

// ErrGitHubRateLimited is returned by FetchLatestRelease when GitHub responds
// 403 with X-RateLimit-Remaining: 0. Callers should log at info (not warn)
// and back off without burning further requests.
var ErrGitHubRateLimited = errors.New("github api rate limited")

type releasePayload struct {
	TagName string `json:"tag_name"`
	HTMLURL string `json:"html_url"`
}

// FetchLatestRelease performs an unauthenticated GET against the GitHub
// Releases API and returns the tag name + release URL. On non-2xx it returns
// an error including the HTTP status; on rate-limit responses it returns
// ErrGitHubRateLimited so callers can branch on it.
func FetchLatestRelease(ctx context.Context, client *http.Client) (string, string, error) {
	return FetchLatestReleaseFrom(ctx, client, DefaultReleaseURL)
}

// FetchLatestReleaseFrom is like FetchLatestRelease but accepts an explicit
// URL. Used internally and by tests pointing at a stub server.
func FetchLatestReleaseFrom(ctx context.Context, client *http.Client, url string) (string, string, error) {
	if client == nil {
		client = &http.Client{Timeout: defaultClientTimeout}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", "", fmt.Errorf("build github request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "kandev-updates-poller")

	resp, err := client.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("github request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// GitHub returns 403 with X-RateLimit-Remaining: 0 for the primary unauth
	// rate limit, and 429 for secondary/abuse-detection limits — both should
	// surface as ErrGitHubRateLimited so the poller backs off identically.
	if resp.StatusCode == http.StatusTooManyRequests ||
		(resp.StatusCode == http.StatusForbidden && resp.Header.Get("X-RateLimit-Remaining") == "0") {
		return "", "", ErrGitHubRateLimited
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", "", fmt.Errorf("github status %d: %s", resp.StatusCode, string(body))
	}

	var payload releasePayload
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", "", fmt.Errorf("decode github response: %w", err)
	}
	if payload.TagName == "" {
		return "", "", errors.New("github response missing tag_name")
	}
	return payload.TagName, payload.HTMLURL, nil
}
