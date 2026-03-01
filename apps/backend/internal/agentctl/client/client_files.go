package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"go.uber.org/zap"
)

// RequestFileTree requests a file tree via HTTP GET
func (c *Client) RequestFileTree(ctx context.Context, path string, depth int) (*FileTreeResponse, error) {
	reqURL := fmt.Sprintf("%s/api/v1/workspace/tree?path=%s&depth=%d", c.baseURL, path, depth)

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

// RequestFileContent requests file content via HTTP GET
func (c *Client) RequestFileContent(ctx context.Context, path string) (*FileContentResponse, error) {
	reqURL := fmt.Sprintf("%s/api/v1/workspace/file/content?path=%s", c.baseURL, path)

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

// ApplyFileDiff applies a unified diff to a file via HTTP POST
func (c *Client) ApplyFileDiff(ctx context.Context, path string, diff string, originalHash string) (*streams.FileUpdateResponse, error) {
	reqBody := streams.FileUpdateRequest{
		Path:         path,
		Diff:         diff,
		OriginalHash: originalHash,
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

// CreateFile creates a new file via HTTP POST
func (c *Client) CreateFile(ctx context.Context, path string) (*streams.FileCreateResponse, error) {
	reqBody := streams.FileCreateRequest{Path: path}

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

// DeleteFile deletes a file via HTTP DELETE
func (c *Client) DeleteFile(ctx context.Context, path string) (*streams.FileDeleteResponse, error) {
	reqURL := fmt.Sprintf("%s/api/v1/workspace/file?path=%s", c.baseURL, path)

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

// RenameFile renames/moves a file via HTTP POST
func (c *Client) RenameFile(ctx context.Context, oldPath, newPath string) (*streams.FileRenameResponse, error) {
	reqBody := streams.FileRenameRequest{OldPath: oldPath, NewPath: newPath}

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

// FileSearchResponse represents a response with matching files
type FileSearchResponse struct {
	Files []string `json:"files"`
	Error string   `json:"error,omitempty"`
}
