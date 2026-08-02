package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/worktree/copyfiles"
	"go.uber.org/zap"
)

// RequestFileTree requests a file tree via HTTP GET
func (c *Client) RequestFileTree(ctx context.Context, path string, depth int) (*FileTreeResponse, error) {
	reqURL := fmt.Sprintf(
		"%s/api/v1/workspace/tree?path=%s&depth=%d",
		c.baseURL,
		url.QueryEscape(path),
		depth,
	)

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to request file tree: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			c.logger.Debug("failed to close file tree response body", zap.Error(err))
		}
	}()

	var response FileTreeResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if response.Error != "" {
		return nil, fmt.Errorf("file tree error: %s", response.Error)
	}

	return &response, nil
}

// SearchFiles searches for files matching the query via HTTP GET
func (c *Client) SearchFiles(ctx context.Context, query string, limit int) (*FileSearchResponse, error) {
	reqURL := fmt.Sprintf("%s/api/v1/workspace/search?q=%s&limit=%d", c.baseURL, url.QueryEscape(query), limit)

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to search files: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			c.logger.Debug("failed to close file search response body", zap.Error(err))
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("file search failed with status %d", resp.StatusCode)
	}

	var result FileSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}

// SearchWorkspaceContent searches matching lines in every task repository.
func (c *Client) SearchWorkspaceContent(
	ctx context.Context,
	query string,
	limit int,
) (*WorkspaceContentSearchResponse, error) {
	reqURL := fmt.Sprintf(
		"%s/api/v1/workspace/content-search?q=%s&limit_per_repo=%d",
		c.baseURL,
		url.QueryEscape(query),
		limit,
	)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create content search request: %w", err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to search workspace content: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			c.logger.Debug("failed to close content search response body", zap.Error(err))
		}
	}()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("workspace content search failed with status %d", resp.StatusCode)
	}

	var result WorkspaceContentSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode content search response: %w", err)
	}
	if result.Error != "" {
		return nil, fmt.Errorf("workspace content search error: %s", result.Error)
	}
	return &result, nil
}

// RequestFileContent requests file content via HTTP GET.
// repo is the multi-repo subpath (e.g. "kandev"); empty for single-repo workspaces.
func (c *Client) RequestFileContent(ctx context.Context, path, repo string) (*FileContentResponse, error) {
	reqURL := fmt.Sprintf("%s/api/v1/workspace/file/content?path=%s", c.baseURL, url.QueryEscape(path))
	if repo != "" {
		reqURL += "&repo=" + url.QueryEscape(repo)
	}

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to request file content: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			c.logger.Debug("failed to close file content response body", zap.Error(err))
		}
	}()

	var response FileContentResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if response.Error != "" {
		return nil, fmt.Errorf("file content error: %s", response.Error)
	}

	return &response, nil
}

// RequestFileContentAtRef requests file content at a specific git ref via HTTP GET.
// repo is the multi-repo subpath (e.g. "kandev"); empty for single-repo workspaces.
func (c *Client) RequestFileContentAtRef(ctx context.Context, path, ref, repo string) (*FileContentResponse, error) {
	reqURL := fmt.Sprintf("%s/api/v1/workspace/file/content-at-ref?path=%s&ref=%s", c.baseURL, url.QueryEscape(path), url.QueryEscape(ref))
	if repo != "" {
		reqURL += "&repo=" + url.QueryEscape(repo)
	}

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to request file content at ref: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			c.logger.Debug("failed to close file content at ref response body", zap.Error(err))
		}
	}()

	var response FileContentResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if response.Error != "" {
		return nil, fmt.Errorf("file content at ref error: %s", response.Error)
	}

	return &response, nil
}

// ApplyFileDiff applies a unified diff to a file via HTTP POST.
// repo is the multi-repo subpath (e.g. "kandev"); empty for single-repo workspaces.
func (c *Client) ApplyFileDiff(ctx context.Context, path, diff, originalHash, repo string, desiredContent *string) (*streams.FileUpdateResponse, error) {
	reqBody := streams.FileUpdateRequest{
		Path:           path,
		Repo:           repo,
		Diff:           diff,
		OriginalHash:   originalHash,
		DesiredContent: desiredContent,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	reqURL := fmt.Sprintf("%s/api/v1/workspace/file/content", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, "POST", reqURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to apply file diff: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			c.logger.Debug("failed to close file update response body", zap.Error(err))
		}
	}()

	var response streams.FileUpdateResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if response.Error != "" {
		return nil, fmt.Errorf("file update error: %s", response.Error)
	}

	if !response.Success {
		return nil, fmt.Errorf("file update failed")
	}

	return &response, nil
}

// CreateFile creates a new file via HTTP POST.
// repo is the multi-repo subpath (e.g. "kandev"); empty for single-repo workspaces.
func (c *Client) CreateFile(ctx context.Context, path, repo string) (*streams.FileCreateResponse, error) {
	reqBody := streams.FileCreateRequest{Path: path, Repo: repo}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	reqURL := fmt.Sprintf("%s/api/v1/workspace/file/create", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, "POST", reqURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to create file: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			c.logger.Debug("failed to close file create response body", zap.Error(err))
		}
	}()

	var response streams.FileCreateResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if response.Error != "" {
		return nil, fmt.Errorf("file create error: %s", response.Error)
	}

	if !response.Success {
		return nil, fmt.Errorf("file create failed")
	}

	return &response, nil
}

// DeleteFile deletes a file via HTTP DELETE.
// repo is the multi-repo subpath (e.g. "kandev"); empty for single-repo workspaces.
func (c *Client) DeleteFile(ctx context.Context, path, repo string) (*streams.FileDeleteResponse, error) {
	reqURL := fmt.Sprintf("%s/api/v1/workspace/file?path=%s", c.baseURL, url.QueryEscape(path))
	if repo != "" {
		reqURL += "&repo=" + url.QueryEscape(repo)
	}

	req, err := http.NewRequestWithContext(ctx, "DELETE", reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to delete file: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			c.logger.Debug("failed to close file delete response body", zap.Error(err))
		}
	}()

	var response streams.FileDeleteResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if response.Error != "" {
		return nil, fmt.Errorf("file delete error: %s", response.Error)
	}

	if !response.Success {
		return nil, fmt.Errorf("file delete failed")
	}

	return &response, nil
}

// RenameFile renames/moves a file or directory via HTTP POST.
// repo is the multi-repo subpath (e.g. "kandev"); empty for single-repo workspaces.
func (c *Client) RenameFile(ctx context.Context, oldPath, newPath, repo string) (*streams.FileRenameResponse, error) {
	reqBody := streams.FileRenameRequest{OldPath: oldPath, NewPath: newPath, Repo: repo}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	reqURL := fmt.Sprintf("%s/api/v1/workspace/file/rename", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, "POST", reqURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to rename file: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			c.logger.Debug("failed to close file rename response body", zap.Error(err))
		}
	}()

	var response streams.FileRenameResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if response.Error != "" {
		return nil, fmt.Errorf("file rename error: %s", response.Error)
	}

	if !response.Success {
		return nil, fmt.Errorf("file rename failed")
	}

	return &response, nil
}

type (
	FileSearchResult               = streams.FileSearchResult
	FileSearchResponse             = streams.FileSearchResponse
	WorkspaceContentMatchRange     = streams.WorkspaceContentMatchRange
	WorkspaceContentSearchResult   = streams.WorkspaceContentSearchResult
	WorkspaceContentSearchResponse = streams.WorkspaceContentSearchResponse
)

// CopyFilesRequest is the body for POST /workspace/copy-files. Mirrors the
// agentctl-side api.CopyFilesRequest shape — kept local rather than imported
// to keep the client free of the api package dependency.
type CopyFilesRequest struct {
	Repo    string            `json:"repo,omitempty"`
	Entries []copyfiles.Entry `json:"entries"`
}

// CopyFilesResponse mirrors api.CopyFilesResponse.
type CopyFilesResponse struct {
	Copied   []string `json:"copied,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
	Error    string   `json:"error,omitempty"`
}

type DiagnosticMaterializeResponse struct {
	Path  string `json:"path"`
	Bytes int64  `json:"bytes"`
	Error string `json:"error,omitempty"`
}

// MaterializeDiagnosticBundle streams a bounded archive to the server-owned
// diagnostics directory inside this agentctl workspace.
func (c *Client) MaterializeDiagnosticBundle(
	ctx context.Context,
	bundleID string,
	source io.Reader,
) (*DiagnosticMaterializeResponse, error) {
	reqURL := fmt.Sprintf("%s/api/v1/workspace/diagnostics/%s", c.baseURL, url.PathEscape(bundleID))
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, source)
	if err != nil {
		return nil, fmt.Errorf("create diagnostic materialization request: %w", err)
	}
	request.Header.Set("Content-Type", "application/zip")
	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("materialize diagnostic bundle: %w", err)
	}
	defer func() {
		if err := response.Body.Close(); err != nil {
			c.logger.Debug("failed to close diagnostic response body", zap.Error(err))
		}
	}()
	var result DiagnosticMaterializeResponse
	if err := json.NewDecoder(io.LimitReader(response.Body, 64*1024)).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode diagnostic materialization response: %w", err)
	}
	if response.StatusCode != http.StatusOK || result.Error != "" {
		return nil, fmt.Errorf("diagnostic materialization failed: status=%d error=%s",
			response.StatusCode, result.Error)
	}
	return &result, nil
}

// CopyFiles ships a batch of pre-planned files (from copyfiles.Plan on the
// host) to agentctl's POST /workspace/copy-files endpoint, which writes
// them into the workspace under the optional repo subpath. Idempotent —
// existing destinations are skipped. Used by remote executors (Docker,
// Sprites) to seed gitignored files after the in-container clone.
func (c *Client) CopyFiles(ctx context.Context, repo string, entries []copyfiles.Entry) (*CopyFilesResponse, error) {
	bodyBytes, err := json.Marshal(CopyFilesRequest{Repo: repo, Entries: entries})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	reqURL := fmt.Sprintf("%s/api/v1/workspace/copy-files", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, "POST", reqURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to copy files: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			c.logger.Debug("failed to close copy-files response body", zap.Error(err))
		}
	}()

	var response CopyFilesResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	if response.Error != "" {
		return &response, fmt.Errorf("copy-files error: %s", response.Error)
	}
	return &response, nil
}
