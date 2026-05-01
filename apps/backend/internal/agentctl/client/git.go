package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
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
// repo is the multi-repo subpath (e.g. "kandev"); empty for single-repo workspaces.
func (c *Client) GitPull(ctx context.Context, rebase bool, repo string) (*GitOperationResult, error) {
	payload := struct {
		Rebase bool   `json:"rebase"`
		Repo   string `json:"repo,omitempty"`
	}{
		Rebase: rebase,
		Repo:   repo,
	}
	return c.gitOperation(ctx, "/api/v1/git/pull", payload)
}

// GitPush performs a git push operation on the worktree.
// If force is true, uses --force-with-lease.
// If setUpstream is true, uses --set-upstream.
// repo is the multi-repo subpath (e.g. "kandev"); empty for single-repo workspaces.
func (c *Client) GitPush(ctx context.Context, force, setUpstream bool, repo string) (*GitOperationResult, error) {
	payload := struct {
		Force       bool   `json:"force"`
		SetUpstream bool   `json:"set_upstream"`
		Repo        string `json:"repo,omitempty"`
	}{
		Force:       force,
		SetUpstream: setUpstream,
		Repo:        repo,
	}
	return c.gitOperation(ctx, "/api/v1/git/push", payload)
}

// GitRebase rebases the worktree branch onto the specified base branch.
// It first fetches origin/<baseBranch>, then rebases onto it.
// repo is the multi-repo subpath (e.g. "kandev"); empty for single-repo workspaces.
func (c *Client) GitRebase(ctx context.Context, baseBranch, repo string) (*GitOperationResult, error) {
	payload := struct {
		BaseBranch string `json:"base_branch"`
		Repo       string `json:"repo,omitempty"`
	}{
		BaseBranch: baseBranch,
		Repo:       repo,
	}
	return c.gitOperation(ctx, "/api/v1/git/rebase", payload)
}

// GitMerge merges the specified base branch into the worktree branch.
// It first fetches origin/<baseBranch>, then merges it.
// repo is the multi-repo subpath (e.g. "kandev"); empty for single-repo workspaces.
func (c *Client) GitMerge(ctx context.Context, baseBranch, repo string) (*GitOperationResult, error) {
	payload := struct {
		BaseBranch string `json:"base_branch"`
		Repo       string `json:"repo,omitempty"`
	}{
		BaseBranch: baseBranch,
		Repo:       repo,
	}
	return c.gitOperation(ctx, "/api/v1/git/merge", payload)
}

// GitAbort aborts an in-progress merge or rebase operation.
// The operation parameter must be "merge" or "rebase".
// repo is the multi-repo subpath (e.g. "kandev"); empty for single-repo workspaces.
func (c *Client) GitAbort(ctx context.Context, operation, repo string) (*GitOperationResult, error) {
	payload := struct {
		Operation string `json:"operation"`
		Repo      string `json:"repo,omitempty"`
	}{
		Operation: operation,
		Repo:      repo,
	}
	return c.gitOperation(ctx, "/api/v1/git/abort", payload)
}

// GitCommit creates a commit with the specified message.
// If stageAll is true, all changes are staged before committing.
// If amend is true, it amends the previous commit instead of creating a new one.
// repo is the multi-repo subpath (e.g. "kandev"); empty for single-repo workspaces.
func (c *Client) GitCommit(ctx context.Context, message string, stageAll bool, amend bool, repo string) (*GitOperationResult, error) {
	payload := struct {
		Message  string `json:"message"`
		StageAll bool   `json:"stage_all"`
		Amend    bool   `json:"amend"`
		Repo     string `json:"repo,omitempty"`
	}{
		Message:  message,
		StageAll: stageAll,
		Amend:    amend,
		Repo:     repo,
	}
	return c.gitOperation(ctx, "/api/v1/git/commit", payload)
}

// GitRenameBranch renames the current branch to a new name.
// repo is the multi-repo subpath (e.g. "kandev"); empty for single-repo workspaces.
func (c *Client) GitRenameBranch(ctx context.Context, newName, repo string) (*GitOperationResult, error) {
	payload := struct {
		NewName string `json:"new_name"`
		Repo    string `json:"repo,omitempty"`
	}{
		NewName: newName,
		Repo:    repo,
	}
	return c.gitOperation(ctx, "/api/v1/git/rename-branch", payload)
}

// GitStage stages files for commit.
// If paths is empty, stages all changes (git add -A).
// repo is the multi-repo subpath (e.g. "kandev"); empty for single-repo workspaces.
func (c *Client) GitStage(ctx context.Context, paths []string, repo string) (*GitOperationResult, error) {
	payload := struct {
		Paths []string `json:"paths"`
		Repo  string   `json:"repo,omitempty"`
	}{
		Paths: paths,
		Repo:  repo,
	}
	return c.gitOperation(ctx, "/api/v1/git/stage", payload)
}

// GitUnstage unstages files from the index.
// If paths is empty, unstages all changes (git reset HEAD).
// repo is the multi-repo subpath (e.g. "kandev"); empty for single-repo workspaces.
func (c *Client) GitUnstage(ctx context.Context, paths []string, repo string) (*GitOperationResult, error) {
	payload := struct {
		Paths []string `json:"paths"`
		Repo  string   `json:"repo,omitempty"`
	}{
		Paths: paths,
		Repo:  repo,
	}
	return c.gitOperation(ctx, "/api/v1/git/unstage", payload)
}

// GitDiscard discards changes to files, reverting them to HEAD.
// Paths must not be empty - at least one file must be specified.
// repo is the multi-repo subpath (e.g. "kandev"); empty for single-repo workspaces.
func (c *Client) GitDiscard(ctx context.Context, paths []string, repo string) (*GitOperationResult, error) {
	payload := struct {
		Paths []string `json:"paths"`
		Repo  string   `json:"repo,omitempty"`
	}{
		Paths: paths,
		Repo:  repo,
	}
	return c.gitOperation(ctx, "/api/v1/git/discard", payload)
}

// GitRevertCommit undoes the latest commit using git reset --soft, keeping changes staged.
// repo is the multi-repo subpath (e.g. "kandev"); empty for single-repo workspaces.
func (c *Client) GitRevertCommit(ctx context.Context, commitSHA, repo string) (*GitOperationResult, error) {
	payload := struct {
		CommitSHA string `json:"commit_sha"`
		Repo      string `json:"repo,omitempty"`
	}{
		CommitSHA: commitSHA,
		Repo:      repo,
	}
	return c.gitOperation(ctx, "/api/v1/git/revert-commit", payload)
}

// GitReset resets HEAD to the specified commit.
// Mode can be "soft" (keep changes staged), "mixed" (keep changes unstaged), or "hard" (discard all changes).
// repo is the multi-repo subpath (e.g. "kandev"); empty for single-repo workspaces.
func (c *Client) GitReset(ctx context.Context, commitSHA, mode, repo string) (*GitOperationResult, error) {
	payload := struct {
		CommitSHA string `json:"commit_sha"`
		Mode      string `json:"mode"`
		Repo      string `json:"repo,omitempty"`
	}{
		CommitSHA: commitSHA,
		Mode:      mode,
		Repo:      repo,
	}
	return c.gitOperation(ctx, "/api/v1/git/reset", payload)
}

// GitCreatePR creates a pull request using the gh CLI.
// repo is the multi-repo subpath (e.g. "kandev"); empty for single-repo workspaces.
func (c *Client) GitCreatePR(ctx context.Context, title, body, baseBranch string, draft bool, repo string) (*PRCreateResult, error) {
	payload := struct {
		Title      string `json:"title"`
		Body       string `json:"body"`
		BaseBranch string `json:"base_branch"`
		Draft      bool   `json:"draft"`
		Repo       string `json:"repo,omitempty"`
	}{
		Title:      title,
		Body:       body,
		BaseBranch: baseBranch,
		Draft:      draft,
		Repo:       repo,
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
// repo is the multi-repo subpath (e.g. "kandev"); empty for single-repo workspaces.
// Multi-repo tasks must pass the owning repo because a SHA from one repo's
// commit graph isn't resolvable in any other repo.
func (c *Client) GitShowCommit(ctx context.Context, commitSHA, repo string) (*CommitDiffResult, error) {
	reqURL := c.baseURL + "/api/v1/git/commit/" + commitSHA
	if repo != "" {
		reqURL += "?repo=" + url.QueryEscape(repo)
	}
	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
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

// GitLogResult represents the result of a git log operation.
type GitLogResult struct {
	Success bool             `json:"success"`
	Commits []*GitCommitInfo `json:"commits"`
	Error   string           `json:"error,omitempty"`
	// PerRepoErrors lists per-repo failures during a multi-repo log fan-out.
	// Empty/nil for single-repo responses or when every repo succeeded. Mirrors
	// the server's process.GitLogResult.PerRepoErrors field.
	PerRepoErrors []GitLogRepoError `json:"per_repo_errors,omitempty"`
}

// GitLogRepoError describes a single per-repo failure from a multi-repo log
// fan-out. Mirrors the server's process.GitLogRepoError type so callers using
// the agentctl client can deserialize and surface partial failures.
type GitLogRepoError struct {
	RepositoryName string `json:"repository_name"`
	Error          string `json:"error"`
}

// GitCommitInfo represents a single commit in the log.
// Field names match GitCommitData and SessionCommit for frontend consistency.
type GitCommitInfo struct {
	CommitSHA     string `json:"commit_sha"`
	ParentSHA     string `json:"parent_sha"`
	CommitMessage string `json:"commit_message"`
	AuthorName    string `json:"author_name"`
	AuthorEmail   string `json:"author_email"`
	CommittedAt   string `json:"committed_at"`
	FilesChanged  int    `json:"files_changed"`
	Insertions    int    `json:"insertions"`
	Deletions     int    `json:"deletions"`
	// RepositoryName tags commits when fetched from a multi-repo subpath. Empty
	// for single-repo workspaces. Stamped client-side after the per-repo call
	// so callers can fan out across repos and merge.
	RepositoryName string `json:"repository_name,omitempty"`
}

// GitLog gets the commit log from baseCommit to HEAD.
// If baseCommit is empty, returns recent commits (limited by limit).
// If targetBranch is provided, computes merge-base dynamically for accurate filtering.
// repo is the multi-repo subpath (e.g. "kandev"); empty for single-repo workspaces.
func (c *Client) GitLog(ctx context.Context, baseCommit string, limit int, targetBranch, repo string) (*GitLogResult, error) {
	reqURL := fmt.Sprintf("%s/api/v1/git/log?since=%s&limit=%d",
		c.baseURL, url.QueryEscape(baseCommit), limit)
	if targetBranch != "" {
		reqURL += "&target_branch=" + url.QueryEscape(targetBranch)
	}
	if repo != "" {
		reqURL += "&repo=" + url.QueryEscape(repo)
	}

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
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

	var result GitLogResult
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response (status %d, body: %s): %w",
			resp.StatusCode, truncateBody(respBody), err)
	}

	if resp.StatusCode >= 400 {
		return &result, fmt.Errorf("git log failed with status %d: %s", resp.StatusCode, result.Error)
	}

	return &result, nil
}

// CumulativeDiffResult represents the cumulative diff from base commit to HEAD.
type CumulativeDiffResult struct {
	Success      bool                   `json:"success"`
	BaseCommit   string                 `json:"base_commit"`
	HeadCommit   string                 `json:"head_commit"`
	TotalCommits int                    `json:"total_commits"`
	Files        map[string]interface{} `json:"files"`
	Error        string                 `json:"error,omitempty"`
}

// GetCumulativeDiff gets the cumulative diff from baseCommit to HEAD.
func (c *Client) GetCumulativeDiff(ctx context.Context, baseCommit string) (*CumulativeDiffResult, error) {
	reqURL := fmt.Sprintf("%s/api/v1/git/cumulative-diff?base=%s", c.baseURL, url.QueryEscape(baseCommit))

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
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

	var result CumulativeDiffResult
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response (status %d, body: %s): %w",
			resp.StatusCode, truncateBody(respBody), err)
	}

	if resp.StatusCode >= 400 {
		return &result, fmt.Errorf("cumulative diff failed with status %d: %s", resp.StatusCode, result.Error)
	}

	return &result, nil
}

// GitStatusResult represents the result of a git status query.
type GitStatusResult struct {
	Success         bool                   `json:"success"`
	Branch          string                 `json:"branch"`
	RemoteBranch    string                 `json:"remote_branch"`
	HeadCommit      string                 `json:"head_commit"`
	BaseCommit      string                 `json:"base_commit"` // Merge-base with origin branch
	Ahead           int                    `json:"ahead"`
	Behind          int                    `json:"behind"`
	Modified        []string               `json:"modified"`
	Added           []string               `json:"added"`
	Deleted         []string               `json:"deleted"`
	Untracked       []string               `json:"untracked"`
	Renamed         []string               `json:"renamed"`
	Files           map[string]interface{} `json:"files"`
	Timestamp       string                 `json:"timestamp"`
	BranchAdditions int                    `json:"branch_additions,omitempty"`
	BranchDeletions int                    `json:"branch_deletions,omitempty"`
	Error           string                 `json:"error,omitempty"`
}

// fetchJSONResult performs a GET against `path` and decodes the response into
// `out`. Returns the HTTP status code so callers can decide whether to wrap
// any structured-error body into the typed result. Centralises the boilerplate
// shared between the status endpoints (which is what triggers `dupl`).
func (c *Client) fetchJSONResult(ctx context.Context, path string, out any) (int, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+path, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to create request: %w", err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("failed to execute request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, err := readResponseBody(resp)
	if err != nil {
		return resp.StatusCode, fmt.Errorf("failed to read response body: %w", err)
	}
	if err := json.Unmarshal(respBody, out); err != nil {
		return resp.StatusCode, fmt.Errorf("failed to parse response (status %d, body: %s): %w",
			resp.StatusCode, truncateBody(respBody), err)
	}
	return resp.StatusCode, nil
}

// GetGitStatus gets the current git status from the workspace.
func (c *Client) GetGitStatus(ctx context.Context) (*GitStatusResult, error) {
	var result GitStatusResult
	status, err := c.fetchJSONResult(ctx, "/api/v1/git/status", &result)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return &result, fmt.Errorf("git status failed with status %d: %s", status, result.Error)
	}
	return &result, nil
}

// PerRepoGitStatus pairs a repository_name with its status. Mirrors the
// server-side shape so the gateway can fan a single subscribe out into one
// notification per repo (multi-repo workspaces) without re-running git
// commands once per call.
type PerRepoGitStatus struct {
	RepositoryName string          `json:"repository_name"`
	Status         GitStatusResult `json:"status"`
}

// MultiRepoGitStatusResult is the response shape for /api/v1/git/status/multi.
type MultiRepoGitStatusResult struct {
	Success bool               `json:"success"`
	Repos   []PerRepoGitStatus `json:"repos"`
	Error   string             `json:"error,omitempty"`
}

// GetGitStatusMulti returns one status entry per repo (multi-repo) or a
// single untagged entry (single-repo). Used by the session-subscribe handler
// in the main backend to seed the frontend's per-repo state on page reload —
// without it, the frontend never sees per-repo grouping until something else
// (a file change, a poll) pushes a stamped notification.
func (c *Client) GetGitStatusMulti(ctx context.Context) (*MultiRepoGitStatusResult, error) {
	var result MultiRepoGitStatusResult
	status, err := c.fetchJSONResult(ctx, "/api/v1/git/status/multi", &result)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return &result, fmt.Errorf("git status multi failed with status %d: %s", status, result.Error)
	}
	return &result, nil
}
