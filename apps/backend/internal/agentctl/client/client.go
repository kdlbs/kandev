// Package agentctl provides a client for communicating with agentctl running inside containers
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/common/logger"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

// Client communicates with agentctl via HTTP and WebSocket
type Client struct {
	baseURL    string
	httpClient *http.Client
	logger     *logger.Logger

	// WebSocket connections for streaming
	agentStreamConn     *websocket.Conn
	workspaceStreamConn *websocket.Conn
	mu                  sync.RWMutex

	// Shared write mutex for agent stream (used by StreamUpdates and sendStreamRequest)
	streamWriteMu sync.Mutex

	// Pending request/response tracking for agent stream
	pendingRequests map[string]chan *ws.Message
	pendingMu       sync.Mutex
}

// StatusResponse from agentctl
type StatusResponse struct {
	AgentStatus string                 `json:"agent_status"`
	ProcessInfo map[string]interface{} `json:"process_info"`
}

// IsAgentRunning returns true if the agent process is running or starting
// (i.e., the agent is active and should not be considered stale)
func (s *StatusResponse) IsAgentRunning() bool {
	return s.AgentStatus == "running" || s.AgentStatus == "starting"
}

// NewClient creates a new agentctl client
func NewClient(host string, port int, log *logger.Logger) *Client {
	return &Client{
		baseURL: fmt.Sprintf("http://%s:%d", host, port),
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
		logger:          log.WithFields(zap.String("component", "agentctl-client")),
		pendingRequests: make(map[string]chan *ws.Message),
	}
}

// Health checks if agentctl is healthy
func (c *Client) Health(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/health", nil)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check failed: %d", resp.StatusCode)
	}
	return nil
}

// GetStatus returns the agent status
func (c *Client) GetStatus(ctx context.Context) (*StatusResponse, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/api/v1/status", nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := readResponseBody(resp)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("status request failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var status StatusResponse
	if err := json.Unmarshal(respBody, &status); err != nil {
		return nil, fmt.Errorf("failed to parse status response (status %d, body: %s): %w", resp.StatusCode, truncateBody(respBody), err)
	}
	return &status, nil
}

// ConfigureAgent configures the agent command and optional approval policy. Must be called before Start().
func (c *Client) ConfigureAgent(ctx context.Context, command string, env map[string]string, approvalPolicy string) error {
	payload := struct {
		Command        string            `json:"command"`
		Env            map[string]string `json:"env,omitempty"`
		ApprovalPolicy string            `json:"approval_policy,omitempty"`
	}{
		Command:        command,
		Env:            env,
		ApprovalPolicy: approvalPolicy,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/v1/agent/configure", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	// Read the response body for better error handling
	respBody, err := readResponseBody(resp)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	// Check HTTP status code first
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("configure request failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Success bool   `json:"success"`
		Error   string `json:"error,omitempty"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return fmt.Errorf("failed to parse configure response (status %d, body: %s): %w", resp.StatusCode, truncateBody(respBody), err)
	}
	if !result.Success {
		return fmt.Errorf("configure failed: %s", result.Error)
	}
	return nil
}

// Start starts the agent process and returns the full command that was executed.
func (c *Client) Start(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/v1/start", nil)
	if err != nil {
		return "", err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := readResponseBody(resp)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("start request failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Success bool   `json:"success"`
		Command string `json:"command,omitempty"`
		Error   string `json:"error,omitempty"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("failed to parse start response (status %d, body: %s): %w", resp.StatusCode, truncateBody(respBody), err)
	}
	if !result.Success {
		return "", fmt.Errorf("start failed: %s", result.Error)
	}
	return result.Command, nil
}

// Stop stops the agent process
func (c *Client) Stop(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/v1/stop", nil)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := readResponseBody(resp)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("stop request failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Success bool   `json:"success"`
		Error   string `json:"error,omitempty"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return fmt.Errorf("failed to parse stop response (status %d, body: %s): %w", resp.StatusCode, truncateBody(respBody), err)
	}
	if !result.Success {
		return fmt.Errorf("stop failed: %s", result.Error)
	}
	return nil
}

// WaitForReady waits until agentctl is ready to accept requests
func (c *Client) WaitForReady(ctx context.Context, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if time.Now().After(deadline) {
				return fmt.Errorf("timeout waiting for agentctl to be ready")
			}

			if err := c.Health(ctx); err == nil {
				c.logger.Info("agentctl is ready")
				return nil
			}
		}
	}
}

// Re-export VS Code types from shared types package.
type (
	VscodeStartResponse  = types.VscodeStartResponse
	VscodeStatusResponse = types.VscodeStatusResponse
	VscodeStopResponse   = types.VscodeStopResponse
)

// StartVscode starts the code-server with the given theme.
// The port is allocated by agentctl using an OS-assigned random port.
func (c *Client) StartVscode(ctx context.Context, theme string) (*VscodeStartResponse, error) {
	payload := struct {
		Theme string `json:"theme,omitempty"`
	}{Theme: theme}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/v1/vscode/start", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := readResponseBody(resp)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("vscode start failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var result VscodeStartResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse vscode start response: %w", err)
	}
	return &result, nil
}

// StopVscode stops the code-server.
func (c *Client) StopVscode(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/v1/vscode/stop", nil)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := readResponseBody(resp)
		return fmt.Errorf("vscode stop failed with status %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

// VscodeOpenFile opens a file in the running VS Code instance via agentctl.
func (c *Client) VscodeOpenFile(ctx context.Context, path string, line, col int) error {
	payload := types.VscodeOpenFileRequest{
		Path: path,
		Line: line,
		Col:  col,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/v1/vscode/open-file", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := readResponseBody(resp)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("vscode open-file failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var result types.VscodeOpenFileResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return fmt.Errorf("failed to parse vscode open-file response: %w", err)
	}
	if !result.Success {
		return fmt.Errorf("vscode open-file failed: %s", result.Error)
	}
	return nil
}

// VscodeStatus returns the current code-server state.
func (c *Client) VscodeStatus(ctx context.Context) (*VscodeStatusResponse, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/api/v1/vscode/status", nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := readResponseBody(resp)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("vscode status failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var result VscodeStatusResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse vscode status response: %w", err)
	}
	return &result, nil
}

// BaseURL returns the base URL of the agentctl client
func (c *Client) BaseURL() string {
	return c.baseURL
}

// Host returns the host portion (without port) of the agentctl client URL.
func (c *Client) Host() string {
	parsed, err := url.Parse(c.baseURL)
	if err != nil {
		return c.baseURL
	}
	return parsed.Hostname()
}

// Re-export types from types package for convenience.
// These types are defined in the streams subpackage and re-exported through types.
type (
	GitStatusUpdate        = types.GitStatusUpdate
	GitCommitNotification  = types.GitCommitNotification
	GitResetNotification   = types.GitResetNotification
	FileInfo               = types.FileInfo
	FileEntry              = types.FileEntry
	FileTreeNode           = types.FileTreeNode
	FileTreeRequest        = types.FileTreeRequest
	FileTreeResponse       = types.FileTreeResponse
	FileContentRequest     = types.FileContentRequest
	FileContentResponse    = types.FileContentResponse
	FileChangeNotification = types.FileChangeNotification
	ShellMessage           = types.ShellMessage
	ShellStatusResponse    = types.ShellStatusResponse
	ShellBufferResponse    = types.ShellBufferResponse
	ProcessKind            = types.ProcessKind
	ProcessStatus          = types.ProcessStatus
	ProcessOutput          = types.ProcessOutput
	ProcessStatusUpdate    = types.ProcessStatusUpdate
)

// Close closes all connections
func (c *Client) Close() {
	c.CloseUpdatesStream()
	c.CloseWorkspaceStream()
}

// readResponseBody reads and returns the response body
func readResponseBody(resp *http.Response) ([]byte, error) {
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(resp.Body); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// truncateBody truncates body for error messages to avoid huge logs
func truncateBody(body []byte) string {
	const maxLen = 200
	if len(body) > maxLen {
		return string(body[:maxLen]) + "..."
	}
	return string(body)
}
