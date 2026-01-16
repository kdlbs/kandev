// Package agentctl provides a client for communicating with agentctl running inside containers
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
)

// Client communicates with agentctl via HTTP
type Client struct {
	baseURL    string
	httpClient *http.Client
	logger     *logger.Logger

	// WebSocket connections for streaming
	acpConn         *websocket.Conn
	permissionConn  *websocket.Conn
	gitStatusConn   *websocket.Conn
	fileChangesConn *websocket.Conn
	shellConn       *websocket.Conn
	mu              sync.RWMutex
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
			Timeout: 30 * time.Second,
		},
		logger: log.WithFields(zap.String("component", "agentctl-client")),
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

// ConfigureAgent configures the agent command. Must be called before Start().
func (c *Client) ConfigureAgent(ctx context.Context, command string, env map[string]string) error {
	payload := struct {
		Command string            `json:"command"`
		Env     map[string]string `json:"env,omitempty"`
	}{
		Command: command,
		Env:     env,
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

// Start starts the agent process
func (c *Client) Start(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/v1/start", nil)
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
		return fmt.Errorf("start request failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Success bool   `json:"success"`
		Error   string `json:"error,omitempty"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return fmt.Errorf("failed to parse start response (status %d, body: %s): %w", resp.StatusCode, truncateBody(respBody), err)
	}
	if !result.Success {
		return fmt.Errorf("start failed: %s", result.Error)
	}
	return nil
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

// BaseURL returns the base URL of the agentctl client
func (c *Client) BaseURL() string {
	return c.baseURL
}

// Re-export types from types package for convenience
type (
	GitStatusUpdate        = types.GitStatusUpdate
	FileInfo               = types.FileInfo
	FileListUpdate         = types.FileListUpdate
	FileEntry              = types.FileEntry
	FileTreeNode           = types.FileTreeNode
	FileTreeRequest        = types.FileTreeRequest
	FileTreeResponse       = types.FileTreeResponse
	FileContentRequest     = types.FileContentRequest
	FileContentResponse    = types.FileContentResponse
	FileChangeNotification = types.FileChangeNotification
)

// StreamGitStatus opens a WebSocket connection for streaming git status updates
func (c *Client) StreamGitStatus(ctx context.Context, handler func(*GitStatusUpdate)) error {
	wsURL := "ws" + c.baseURL[4:] + "/api/v1/workspace/git-status/stream"

	conn, _, err := websocket.DefaultDialer.DialContext(ctx, wsURL, nil)
	if err != nil {
		return fmt.Errorf("failed to connect to git status stream: %w", err)
	}

	c.mu.Lock()
	c.gitStatusConn = conn
	c.mu.Unlock()

	c.logger.Info("connected to git status stream", zap.String("url", wsURL))

	// Read messages in a goroutine
	go func() {
		defer func() {
			c.mu.Lock()
			c.gitStatusConn = nil
			c.mu.Unlock()
			if err := conn.Close(); err != nil {
				c.logger.Debug("failed to close git status websocket", zap.Error(err))
			}
		}()

		for {
			_, message, err := conn.ReadMessage()
			if err != nil {
				if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
					c.logger.Info("git status stream closed normally")
				} else {
					c.logger.Debug("git status stream error", zap.Error(err))
				}
				return
			}

			var update GitStatusUpdate
			if err := json.Unmarshal(message, &update); err != nil {
				c.logger.Warn("failed to parse git status update", zap.Error(err))
				continue
			}

			handler(&update)
		}
	}()

	return nil
}

// CloseGitStatusStream closes the git status stream connection
func (c *Client) CloseGitStatusStream() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.gitStatusConn != nil {
		if err := c.gitStatusConn.Close(); err != nil {
			c.logger.Debug("failed to close git status stream", zap.Error(err))
		}
		c.gitStatusConn = nil
	}
}

// StreamFileChanges opens a WebSocket connection for streaming file change notifications
func (c *Client) StreamFileChanges(ctx context.Context, handler func(*FileChangeNotification)) error {
	wsURL := "ws" + c.baseURL[4:] + "/api/v1/workspace/file-changes/stream"

	conn, _, err := websocket.DefaultDialer.DialContext(ctx, wsURL, nil)
	if err != nil {
		return fmt.Errorf("failed to connect to file changes stream: %w", err)
	}

	c.mu.Lock()
	c.fileChangesConn = conn
	c.mu.Unlock()

	c.logger.Info("connected to file changes stream", zap.String("url", wsURL))

	// Read messages in a goroutine
	go func() {
		defer func() {
			c.mu.Lock()
			c.fileChangesConn = nil
			c.mu.Unlock()
			if err := conn.Close(); err != nil {
				c.logger.Debug("failed to close file changes websocket", zap.Error(err))
			}
		}()

		for {
			_, message, err := conn.ReadMessage()
			if err != nil {
				if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
					c.logger.Info("file changes stream closed normally")
				} else {
					c.logger.Debug("file changes stream error", zap.Error(err))
				}
				return
			}

			var notification FileChangeNotification
			if err := json.Unmarshal(message, &notification); err != nil {
				c.logger.Warn("failed to parse file change notification", zap.Error(err))
				continue
			}

			handler(&notification)
		}
	}()

	return nil
}

// RequestFileTree requests a file tree via HTTP GET
func (c *Client) RequestFileTree(ctx context.Context, path string, depth int) (*FileTreeResponse, error) {
	url := fmt.Sprintf("%s/api/v1/workspace/tree?path=%s&depth=%d", c.baseURL, path, depth)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
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

// RequestFileContent requests file content via HTTP GET
func (c *Client) RequestFileContent(ctx context.Context, path string) (*FileContentResponse, error) {
	url := fmt.Sprintf("%s/api/v1/workspace/file/content?path=%s", c.baseURL, path)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
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

// CloseFileChangesStream closes the file changes stream connection
func (c *Client) CloseFileChangesStream() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.fileChangesConn != nil {
		if err := c.fileChangesConn.Close(); err != nil {
			c.logger.Debug("failed to close file changes stream", zap.Error(err))
		}
		c.fileChangesConn = nil
	}
}

// Close closes all connections (ACP, permissions, git status, file changes, shell)
func (c *Client) Close() {
	c.CloseACPStream()
	c.ClosePermissionStream()
	c.CloseGitStatusStream()
	c.CloseFileChangesStream()
	c.CloseShellStream()
}

// ShellStatusResponse from agentctl shell status endpoint
type ShellStatusResponse struct {
	Running   bool   `json:"running"`
	Pid       int    `json:"pid"`
	Shell     string `json:"shell"`
	Cwd       string `json:"cwd"`
	StartedAt string `json:"started_at"`
}

// ShellStatus gets the status of the embedded shell session
func (c *Client) ShellStatus(ctx context.Context) (*ShellStatusResponse, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/api/v1/shell/status", nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("shell status failed: %s", resp.Status)
	}

	var result ShellStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return &result, nil
}

// ShellMessage represents a message to/from the shell
type ShellMessage struct {
	Type string `json:"type"` // "input", "output", "ping", "pong", "exit"
	Data string `json:"data,omitempty"`
	Code int    `json:"code,omitempty"` // For exit type
}

// ShellBufferResponse is the response from the shell buffer endpoint
type ShellBufferResponse struct {
	Data string `json:"data"`
}

// ShellBuffer returns the buffered shell output
func (c *Client) ShellBuffer(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/api/v1/shell/buffer", nil)
	if err != nil {
		return "", err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("shell buffer failed: %s", resp.Status)
	}

	var result ShellBufferResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	return result.Data, nil
}

// StreamShell connects to the shell WebSocket stream and returns channels for I/O
func (c *Client) StreamShell(ctx context.Context) (<-chan ShellMessage, chan<- ShellMessage, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.shellConn != nil {
		return nil, nil, fmt.Errorf("shell stream already connected")
	}

	wsURL := "ws" + c.baseURL[4:] + "/api/v1/shell/stream"
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, wsURL, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to connect shell stream: %w", err)
	}
	c.shellConn = conn

	outputCh := make(chan ShellMessage, 256)
	inputCh := make(chan ShellMessage, 64)

	// Read from WebSocket
	go func() {
		defer close(outputCh)
		for {
			var msg ShellMessage
			if err := conn.ReadJSON(&msg); err != nil {
				if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
					c.logger.Debug("shell stream read error", zap.Error(err))
				}
				return
			}
			select {
			case outputCh <- msg:
			default:
				// Channel full, skip
			}
		}
	}()

	// Write to WebSocket
	go func() {
		for msg := range inputCh {
			if err := conn.WriteJSON(msg); err != nil {
				c.logger.Debug("shell stream write error", zap.Error(err))
				return
			}
		}
	}()

	return outputCh, inputCh, nil
}

// CloseShellStream closes the shell stream connection
func (c *Client) CloseShellStream() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.shellConn != nil {
		if err := c.shellConn.Close(); err != nil {
			c.logger.Debug("failed to close shell stream", zap.Error(err))
		}
		c.shellConn = nil
	}
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
