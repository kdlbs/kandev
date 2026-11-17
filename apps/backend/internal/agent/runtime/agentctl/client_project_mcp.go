package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/kandev/kandev/internal/agentctl/types"
)

var (
	// ErrProjectMCPUnsupported means the helper predates project MCP file
	// materialization.
	ErrProjectMCPUnsupported = errors.New("remote helper does not support project MCP files; update the execution runtime")
	// ErrProjectMCPConflict means a stale preparation generation or another
	// execution owns the configuration.
	ErrProjectMCPConflict = errors.New("project MCP file ownership changed. Reconnect before retrying")
	// ErrProjectMCPUnavailable means the helper is alive but its project-file
	// owner is temporarily unavailable.
	ErrProjectMCPUnavailable = errors.New("remote project MCP file service is unavailable")
)

func (c *Client) projectMCPJSON(ctx context.Context, method, route string, body, result any) error {
	var payload []byte
	var err error
	if body != nil {
		payload, err = json.Marshal(body)
		if err != nil {
			return err
		}
	}
	request, err := http.NewRequestWithContext(ctx, method, c.baseURL+"/api/v1/mcp/project"+route, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := c.httpClient.Do(request)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		switch response.StatusCode {
		case http.StatusNotFound:
			return ErrProjectMCPUnsupported
		case http.StatusServiceUnavailable:
			return ErrProjectMCPUnavailable
		case http.StatusConflict:
			return ErrProjectMCPConflict
		}
		return fmt.Errorf("remote project MCP request failed (HTTP %d)", response.StatusCode)
	}
	if result == nil {
		_, err = io.Copy(io.Discard, io.LimitReader(response.Body, 1024*1024))
		return err
	}
	return json.NewDecoder(io.LimitReader(response.Body, 16*1024*1024)).Decode(result)
}

// GetProjectMCP returns the helper's executor-local project MCP workspace and
// loopback endpoint.
func (c *Client) GetProjectMCP(ctx context.Context) (*types.ProjectMCPTarget, error) {
	var target types.ProjectMCPTarget
	if err := c.projectMCPJSON(ctx, http.MethodGet, "", nil, &target); err != nil {
		return nil, err
	}
	if target.Version != types.ProjectMCPFilesVersion {
		return nil, ErrProjectMCPUnsupported
	}
	return &target, nil
}

// PrepareProjectMCP materializes generated project files on the helper and
// returns read-back evidence for each file.
func (c *Client) PrepareProjectMCP(
	ctx context.Context,
	request types.ProjectMCPPrepareRequest,
) (*types.ProjectMCPPrepareResponse, error) {
	request.Version = types.ProjectMCPFilesVersion
	var response types.ProjectMCPPrepareResponse
	if err := c.projectMCPJSON(ctx, http.MethodPost, "/prepare", request, &response); err != nil {
		return nil, err
	}
	if response.Version != types.ProjectMCPFilesVersion {
		return nil, ErrProjectMCPUnsupported
	}
	return &response, nil
}

// CleanupProjectMCP releases this execution's project-file claims on the
// helper. Files owned by another execution or by the user remain.
func (c *Client) CleanupProjectMCP(ctx context.Context) error {
	return c.projectMCPJSON(ctx, http.MethodPost, "/cleanup", struct{}{}, nil)
}
