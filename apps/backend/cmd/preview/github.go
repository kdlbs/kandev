package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const (
	sectionStart  = "<!-- kandev-preview-start -->"
	sectionEnd    = "<!-- kandev-preview-end -->"
	githubAPIBase = "https://api.github.com"
)

// upsertDescriptionSection appends (or updates) the kandev preview section at
// the end of the PR description. Other bots may also append sections; we only
// touch the block delimited by our own markers.
func upsertDescriptionSection(ctx context.Context, token, repo string, pr int, section string) error {
	body, err := getPRBody(ctx, token, repo, pr)
	if err != nil {
		return fmt.Errorf("get PR body: %w", err)
	}

	newBody := replaceOrAppendSection(body, section)
	return updatePRBody(ctx, token, repo, pr, newBody)
}

// removeDescriptionSection removes the kandev preview section from the PR
// description on cleanup. If no section is present this is a no-op.
func removeDescriptionSection(ctx context.Context, token, repo string, pr int) error {
	body, err := getPRBody(ctx, token, repo, pr)
	if err != nil {
		return fmt.Errorf("get PR body: %w", err)
	}

	if !strings.Contains(body, sectionStart) {
		return nil // nothing to remove
	}

	newBody := stripSection(body)
	return updatePRBody(ctx, token, repo, pr, newBody)
}

// replaceOrAppendSection replaces an existing kandev section in body with
// section, or appends it if none exists. Preserves any trailing newline.
func replaceOrAppendSection(body, section string) string {
	marked := sectionStart + "\n" + section + "\n" + sectionEnd
	if strings.Contains(body, sectionStart) {
		return replaceSection(body, marked)
	}
	sep := "\n\n"
	if body == "" {
		sep = ""
	}
	return body + sep + marked
}

// replaceSection replaces everything between our markers (inclusive) with marked.
// If sectionEnd is missing (orphaned start marker), the orphan is stripped
// and marked is appended to avoid duplicating sectionStart.
func replaceSection(body, marked string) string {
	start := strings.Index(body, sectionStart)
	end := strings.Index(body, sectionEnd)
	if start == -1 {
		return body + "\n\n" + marked
	}
	if end == -1 || end < start {
		// Orphaned start marker — strip from the start marker to end of string,
		// then append the new section.
		prefix := strings.TrimRight(body[:start], "\n")
		if prefix == "" {
			return marked
		}
		return prefix + "\n\n" + marked
	}
	end += len(sectionEnd)
	return body[:start] + marked + body[end:]
}

// stripSection removes the entire kandev section block (markers + content)
// including any leading blank line that we added before the section.
func stripSection(body string) string {
	start := strings.Index(body, sectionStart)
	end := strings.Index(body, sectionEnd)
	if start == -1 || end == -1 {
		return body
	}
	end += len(sectionEnd)

	// Strip the blank line separator we added before the section.
	prefix := body[:start]
	prefix = strings.TrimRight(prefix, "\n")

	suffix := body[end:]
	if prefix == "" {
		return strings.TrimLeft(suffix, "\n")
	}
	return prefix + suffix
}

func getPRBody(ctx context.Context, token, repo string, pr int) (string, error) {
	url := fmt.Sprintf("%s/repos/%s/pulls/%d", githubAPIBase, repo, pr)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("get PR status %d: %s", resp.StatusCode, b)
	}

	var result struct {
		Body string `json:"body"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	return result.Body, nil
}

func updatePRBody(ctx context.Context, token, repo string, pr int, body string) error {
	url := fmt.Sprintf("%s/repos/%s/pulls/%d", githubAPIBase, repo, pr)
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
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("%s %s: status %d: %s", method, url, resp.StatusCode, b)
	}
	return nil
}

// buildDeploySection returns the markdown section body for a deploy update.
func buildDeploySection(previewURL, sha string) string {
	shaDisplay := sha
	if len(sha) > 7 {
		shaDisplay = sha[:7]
	}
	return fmt.Sprintf(`### Preview Environment

| | |
|---|---|
| **URL** | %s |
| **Commit** | `+"`%s`"+` |
| **Agent** | Mock agent |

> Updates automatically on each push. Destroyed when the PR is closed.`,
		previewURL, shaDisplay)
}
