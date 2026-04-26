package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	commentMarker = "<!-- kandev-preview -->"
	githubAPIBase = "https://api.github.com"
)

// upsertComment finds an existing preview comment on the PR and updates it,
// or creates a new one if none exists.
func upsertComment(ctx context.Context, token, repo string, pr int, body string) error {
	existing, err := findPreviewComment(ctx, token, repo, pr)
	if err != nil {
		return fmt.Errorf("find comment: %w", err)
	}

	if existing != 0 {
		return updateComment(ctx, token, repo, existing, body)
	}
	return createComment(ctx, token, repo, pr, body)
}

func findPreviewComment(ctx context.Context, token, repo string, pr int) (int64, error) {
	url := fmt.Sprintf("%s/repos/%s/issues/%d/comments?per_page=100", githubAPIBase, repo, pr)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return 0, fmt.Errorf("list comments status %d: %s", resp.StatusCode, body)
	}

	var comments []struct {
		ID   int64  `json:"id"`
		Body string `json:"body"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&comments); err != nil {
		return 0, err
	}

	for _, c := range comments {
		if containsMarker(c.Body) {
			return c.ID, nil
		}
	}
	return 0, nil
}

func containsMarker(body string) bool {
	return strings.Contains(body, commentMarker)
}

func createComment(ctx context.Context, token, repo string, pr int, body string) error {
	url := fmt.Sprintf("%s/repos/%s/issues/%d/comments", githubAPIBase, repo, pr)
	return postJSON(ctx, token, http.MethodPost, url, map[string]string{"body": body})
}

func updateComment(ctx context.Context, token, repo string, commentID int64, body string) error {
	url := fmt.Sprintf("%s/repos/%s/issues/comments/%d", githubAPIBase, repo, commentID)
	return postJSON(ctx, token, http.MethodPatch, url, map[string]string{"body": body})
}

func postJSON(ctx context.Context, token, method, url string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("%s %s: status %d: %s", method, url, resp.StatusCode, body)
	}
	return nil
}

// buildDeployComment returns the markdown body for a deploy comment.
func buildDeployComment(previewURL, sha string) string {
	shaDisplay := sha
	if len(sha) > 7 {
		shaDisplay = sha[:7]
	}
	return fmt.Sprintf(`%s
### Preview Environment

| | |
|---|---|
| **URL** | %s |
| **Commit** | `+"`%s`"+` |
| **Agent** | Mock agent |

> Environment updates automatically on each new commit. Destroyed when the PR is closed.`,
		commentMarker, previewURL, shaDisplay)
}

// buildCleanupComment returns the markdown body for a post-close summary comment.
func buildCleanupComment(runtime time.Duration) string {
	runtimeStr := "unknown"
	if runtime > 0 {
		runtimeStr = fmt.Sprintf("~%d minutes", int(runtime.Minutes()))
	}
	return fmt.Sprintf(`%s
### Preview Environment — Closed

The preview environment for this PR has been destroyed.

| | |
|---|---|
| **Runtime** | %s |`,
		commentMarker, runtimeStr)
}
