package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// GitOperationResult represents the result of a git operation.
// This matches the server-side process.GitOperationResult.
type GitOperationResult struct {
	Success       bool     `json:"success"`
	Operation     string   `json:"operation"`
	Output        string   `json:"output"`
	Error         string   `json:"error,omitempty"`
	ConflictFiles []string `json:"conflict_files,omitempty"`
}

// PRCreateResult represents the result of a PR creation operation.
// This matches the server-side process.PRCreateResult.
type PRCreateResult struct {
	Success bool   `json:"success"`
	PRURL   string `json:"pr_url,omitempty"`
	Output  string `json:"output,omitempty"`
	Error   string `json:"error,omitempty"`
}

// GitPull performs a git pull operation on the worktree.
// If rebase is true, uses git pull --rebase.
func (c *Client) GitPull(ctx context.Context, rebase bool) (*GitOperationResult, error) {
	payload := struct {
		Rebase bool `json:"rebase"`
	}{
		Rebase: rebase,
	}
	return c.gitOperation(ctx, "/api/v1/git/pull", payload)
}

// GitPush performs a git push operation on the worktree.
// If force is true, uses --force-with-lease.
// If setUpstream is true, uses --set-upstream.
func (c *Client) GitPush(ctx context.Context, force, setUpstream bool) (*GitOperationResult, error) {
	payload := struct {
		Force       bool `json:"force"`
		SetUpstream bool `json:"set_upstream"`
	}{
		Force:       force,
		SetUpstream: setUpstream,
	}
	return c.gitOperation(ctx, "/api/v1/git/push", payload)
}

// GitRebase rebases the worktree branch onto the specified base branch.
// It first fetches origin/<baseBranch>, then rebases onto it.
func (c *Client) GitRebase(ctx context.Context, baseBranch string) (*GitOperationResult, error) {
	payload := struct {
		BaseBranch string `json:"base_branch"`
	}{
		BaseBranch: baseBranch,
	}
	return c.gitOperation(ctx, "/api/v1/git/rebase", payload)
}

// GitMerge merges the specified base branch into the worktree branch.
// It first fetches origin/<baseBranch>, then merges it.
func (c *Client) GitMerge(ctx context.Context, baseBranch string) (*GitOperationResult, error) {
	payload := struct {
		BaseBranch string `json:"base_branch"`
	}{
		BaseBranch: baseBranch,
	}
	return c.gitOperation(ctx, "/api/v1/git/merge", payload)
}

// GitAbort aborts an in-progress merge or rebase operation.
// The operation parameter must be "merge" or "rebase".
func (c *Client) GitAbort(ctx context.Context, operation string) (*GitOperationResult, error) {
	payload := struct {
		Operation string `json:"operation"`
	}{
		Operation: operation,
	}
	return c.gitOperation(ctx, "/api/v1/git/abort", payload)
}

// GitCommit creates a commit with the specified message.
// If stageAll is true, all changes are staged before committing.
func (c *Client) GitCommit(ctx context.Context, message string, stageAll bool) (*GitOperationResult, error) {
	payload := struct {
		Message  string `json:"message"`
		StageAll bool   `json:"stage_all"`
	}{
		Message:  message,
		StageAll: stageAll,
	}
	return c.gitOperation(ctx, "/api/v1/git/commit", payload)
}

// GitStage stages files for commit.
// If paths is empty, stages all changes (git add -A).
func (c *Client) GitStage(ctx context.Context, paths []string) (*GitOperationResult, error) {
	payload := struct {
		Paths []string `json:"paths"`
	}{
		Paths: paths,
	}
	return c.gitOperation(ctx, "/api/v1/git/stage", payload)
}

// GitUnstage unstages files from the index.
// If paths is empty, unstages all changes (git reset HEAD).
func (c *Client) GitUnstage(ctx context.Context, paths []string) (*GitOperationResult, error) {
	payload := struct {
		Paths []string `json:"paths"`
	}{
		Paths: paths,
	}
	return c.gitOperation(ctx, "/api/v1/git/unstage", payload)
}

// GitDiscard discards changes to files, reverting them to HEAD.
// Paths must not be empty - at least one file must be specified.
func (c *Client) GitDiscard(ctx context.Context, paths []string) (*GitOperationResult, error) {
	payload := struct {
		Paths []string `json:"paths"`
	}{
		Paths: paths,
	}
	return c.gitOperation(ctx, "/api/v1/git/discard", payload)
}

// GitCreatePR creates a pull request using the gh CLI.
func (c *Client) GitCreatePR(ctx context.Context, title, body, baseBranch string, draft bool) (*PRCreateResult, error) {
	payload := struct {
		Title      string `json:"title"`
		Body       string `json:"body"`
		BaseBranch string `json:"base_branch"`
		Draft      bool   `json:"draft"`
	}{
		Title:      title,
		Body:       body,
		BaseBranch: baseBranch,
		Draft:      draft,
	}

	reqBody, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/v1/git/create-pr", bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := readResponseBody(resp)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var result PRCreateResult
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response (status %d, body: %s): %w",
			resp.StatusCode, truncateBody(respBody), err)
	}

	// For 409 Conflict (operation in progress), return the result with error set
	// For other HTTP errors, return as error
	if resp.StatusCode >= 400 && resp.StatusCode != http.StatusConflict {
		return &result, fmt.Errorf("create PR failed with status %d: %s", resp.StatusCode, result.Error)
	}

	return &result, nil
}

// gitOperation is a helper that performs a git operation via HTTP POST.
func (c *Client) gitOperation(ctx context.Context, path string, payload interface{}) (*GitOperationResult, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := readResponseBody(resp)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var result GitOperationResult
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response (status %d, body: %s): %w",
			resp.StatusCode, truncateBody(respBody), err)
	}

	// For 409 Conflict (operation in progress), return the result with error set
	// For other HTTP errors, return as error
	if resp.StatusCode >= 400 && resp.StatusCode != http.StatusConflict {
		return &result, fmt.Errorf("git operation failed with status %d: %s", resp.StatusCode, result.Error)
	}

	return &result, nil
}

// CommitDiffResult represents the result of getting a commit's diff.
type CommitDiffResult struct {
	Success      bool                   `json:"success"`
	CommitSHA    string                 `json:"commit_sha"`
	Message      string                 `json:"message"`
	Author       string                 `json:"author"`
	Date         string                 `json:"date"`
	Files        map[string]interface{} `json:"files"`
	FilesChanged int                    `json:"files_changed"`
	Insertions   int                    `json:"insertions"`
	Deletions    int                    `json:"deletions"`
	Error        string                 `json:"error,omitempty"`
}

// GitShowCommit gets the diff for a specific commit.
func (c *Client) GitShowCommit(ctx context.Context, commitSHA string) (*CommitDiffResult, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/api/v1/git/commit/"+commitSHA, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := readResponseBody(resp)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var result CommitDiffResult
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response (status %d, body: %s): %w",
			resp.StatusCode, truncateBody(respBody), err)
	}

	if resp.StatusCode >= 400 {
		return &result, fmt.Errorf("git show commit failed with status %d: %s", resp.StatusCode, result.Error)
	}

	return &result, nil
}
