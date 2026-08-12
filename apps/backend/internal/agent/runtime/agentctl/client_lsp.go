package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/gorilla/websocket"
	sharedlsp "github.com/kandev/kandev/internal/lsp"
)

const taskLSPTaskIDHeader = "X-Kandev-LSP-Task-ID"

type taskLSPStartBody struct {
	Generation    uint64          `json:"generation"`
	AutoInstall   bool            `json:"auto_install"`
	Configuration json.RawMessage `json:"configuration,omitempty"`
}

type taskLSPStopBody struct {
	Generation uint64 `json:"generation,omitempty"`
	Reason     string `json:"reason,omitempty"`
}

type taskLSPConfigurationBody struct {
	Generation    uint64          `json:"generation"`
	Configuration json.RawMessage `json:"configuration"`
}

func (c *Client) StartTaskLSP(
	ctx context.Context,
	request sharedlsp.TaskHostStartRequest,
) (*sharedlsp.RuntimeSnapshot, error) {
	return c.taskLSPMutation(ctx, request.TaskID, request.Language, "start", taskLSPStartBody{
		Generation: request.Generation, AutoInstall: request.AutoInstall, Configuration: request.Configuration,
	})
}

func (c *Client) RestartTaskLSP(
	ctx context.Context,
	request sharedlsp.TaskHostStartRequest,
) (*sharedlsp.RuntimeSnapshot, error) {
	return c.taskLSPMutation(ctx, request.TaskID, request.Language, "restart", taskLSPStartBody{
		Generation: request.Generation, AutoInstall: request.AutoInstall, Configuration: request.Configuration,
	})
}

func (c *Client) StopTaskLSP(
	ctx context.Context,
	request sharedlsp.TaskHostStopRequest,
) (*sharedlsp.RuntimeSnapshot, error) {
	return c.taskLSPMutation(ctx, request.TaskID, request.Language, "stop", taskLSPStopBody{
		Generation: request.Generation, Reason: request.Reason,
	})
}

func (c *Client) UpdateTaskLSPConfiguration(
	ctx context.Context,
	request sharedlsp.TaskHostConfigurationRequest,
) (*sharedlsp.RuntimeSnapshot, error) {
	return c.taskLSPMutation(ctx, request.TaskID, request.Language, "configuration", taskLSPConfigurationBody{
		Generation: request.Generation, Configuration: request.Configuration,
	})
}

func (c *Client) TaskLSPSnapshot(
	ctx context.Context,
	taskID string,
	language string,
) (*sharedlsp.RuntimeSnapshot, error) {
	return c.taskLSPRequest(ctx, taskID, http.MethodGet, language, "", nil)
}

func (c *Client) taskLSPMutation(
	ctx context.Context,
	taskID, language, action string,
	body any,
) (*sharedlsp.RuntimeSnapshot, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("encode task LSP %s request: %w", action, err)
	}
	return c.taskLSPRequest(ctx, taskID, http.MethodPost, language, action, payload)
}

func (c *Client) taskLSPRequest(
	ctx context.Context,
	taskID, method, language, action string,
	body []byte,
) (*sharedlsp.RuntimeSnapshot, error) {
	path := c.baseURL + "/api/v1/lsp/languages/" + url.PathEscape(language)
	if action != "" {
		path += "/" + action
	}
	request, err := http.NewRequestWithContext(ctx, method, path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	request.Header.Set(taskLSPTaskIDHeader, taskID)
	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer func() { _ = response.Body.Close() }()
	responseBody, err := readResponseBody(response)
	if err != nil {
		return nil, fmt.Errorf("read task LSP response: %w", err)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		var envelope struct {
			Snapshot *sharedlsp.RuntimeSnapshot `json:"snapshot"`
		}
		_ = json.Unmarshal(responseBody, &envelope)
		return envelope.Snapshot, fmt.Errorf(
			"task LSP %s failed with status %d: %s", action, response.StatusCode, truncateBody(responseBody),
		)
	}
	var snapshot sharedlsp.RuntimeSnapshot
	if err := json.Unmarshal(responseBody, &snapshot); err != nil {
		return nil, fmt.Errorf("decode task LSP response: %w", err)
	}
	return &snapshot, nil
}

func (c *Client) DialTaskLSPAttach(
	ctx context.Context,
	taskID string,
	language string,
	generation uint64,
) (*websocket.Conn, *http.Response, error) {
	u, err := url.Parse(c.baseURL)
	if err != nil {
		return nil, nil, err
	}
	if u.Scheme == "https" {
		u.Scheme = "wss"
	} else {
		u.Scheme = "ws"
	}
	u.Path = "/api/v1/lsp/languages/" + url.PathEscape(language) + "/attach"
	query := u.Query()
	query.Set("generation", strconv.FormatUint(generation, 10))
	u.RawQuery = query.Encode()
	headers := c.wsAuthHeaders()
	headers.Set(taskLSPTaskIDHeader, taskID)
	return websocket.DefaultDialer.DialContext(ctx, u.String(), headers)
}

func (c *Client) WatchTaskLSP(
	ctx context.Context,
	taskID string,
	language string,
	onSnapshot func(sharedlsp.RuntimeSnapshot) error,
) error {
	u, err := url.Parse(c.baseURL)
	if err != nil {
		return err
	}
	if u.Scheme == "https" {
		u.Scheme = "wss"
	} else {
		u.Scheme = "ws"
	}
	u.Path = "/api/v1/lsp/languages/" + url.PathEscape(language) + "/watch"
	headers := c.wsAuthHeaders()
	headers.Set(taskLSPTaskIDHeader, taskID)
	conn, response, err := websocket.DefaultDialer.DialContext(ctx, u.String(), headers)
	if err != nil {
		status := 0
		if response != nil {
			status = response.StatusCode
		}
		return fmt.Errorf("dial task LSP watch (status %d): %w", status, err)
	}
	closed := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-closed:
		}
	}()
	defer close(closed)
	defer func() { _ = conn.Close() }()
	for {
		var snapshot sharedlsp.RuntimeSnapshot
		if err := conn.ReadJSON(&snapshot); err != nil {
			if ctx.Err() != nil {
				return context.Cause(ctx)
			}
			return fmt.Errorf("read task LSP watch: %w", err)
		}
		if err := onSnapshot(snapshot); err != nil {
			return err
		}
	}
}

// DiscoverLSP scans names in the already-running task host. It does not
// create/resume an execution or start/install a language server.
func (c *Client) DiscoverLSP(ctx context.Context, taskID string) (*sharedlsp.DiscoveryResult, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/v1/lsp/discovery", nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set(taskLSPTaskIDHeader, taskID)
	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer func() { _ = response.Body.Close() }()
	body, err := readResponseBody(response)
	if err != nil {
		return nil, fmt.Errorf("read LSP discovery response: %w", err)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("LSP discovery failed with status %d: %s", response.StatusCode, body)
	}
	var result sharedlsp.DiscoveryResult
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("decode LSP discovery response: %w", err)
	}
	return &result, nil
}

// RefreshTaskLSPWorkspace asks the existing task host to recompute ordered
// workspace folders and update capable live servers. It never starts a host.
func (c *Client) RefreshTaskLSPWorkspace(
	ctx context.Context,
	taskID string,
	workspace sharedlsp.TaskHostWorkspaceRequest,
) (*sharedlsp.WorkspaceUpdateResult, error) {
	payload, err := json.Marshal(struct {
		WorkspacePath  string   `json:"workspace_path"`
		WorkspaceRoots []string `json:"workspace_roots,omitempty"`
	}{WorkspacePath: workspace.WorkspacePath, WorkspaceRoots: workspace.WorkspaceRoots})
	if err != nil {
		return nil, fmt.Errorf("encode task LSP workspace refresh: %w", err)
	}
	request, err := http.NewRequestWithContext(
		ctx, http.MethodPost, c.baseURL+"/api/v1/lsp/workspace/refresh", bytes.NewReader(payload),
	)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(taskLSPTaskIDHeader, taskID)
	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer func() { _ = response.Body.Close() }()
	body, err := readResponseBody(response)
	if err != nil {
		return nil, fmt.Errorf("read LSP workspace refresh response: %w", err)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		var envelope struct {
			Result sharedlsp.WorkspaceUpdateResult `json:"result"`
		}
		if json.Unmarshal(body, &envelope) == nil {
			return &envelope.Result, fmt.Errorf("LSP workspace refresh failed with status %d: %s", response.StatusCode, truncateBody(body))
		}
		return nil, fmt.Errorf("LSP workspace refresh failed with status %d: %s", response.StatusCode, truncateBody(body))
	}
	var result sharedlsp.WorkspaceUpdateResult
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("decode LSP workspace refresh response: %w", err)
	}
	return &result, nil
}
