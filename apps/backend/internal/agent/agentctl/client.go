// Package agentctl provides a client for communicating with agentctl running inside containers
package agentctl

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/kandev/kandev/internal/agentctl/process"
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
	mu              sync.RWMutex
}

// StatusResponse from agentctl
type StatusResponse struct {
	AgentStatus string                 `json:"agent_status"`
	ProcessInfo map[string]interface{} `json:"process_info"`
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
	defer resp.Body.Close()

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
	defer resp.Body.Close()

	var status StatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return nil, err
	}
	return &status, nil
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
	defer resp.Body.Close()

	var result struct {
		Success bool   `json:"success"`
		Error   string `json:"error,omitempty"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
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
	defer resp.Body.Close()

	var result struct {
		Success bool   `json:"success"`
		Error   string `json:"error,omitempty"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
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

// GitStatusUpdate represents a git status update
type GitStatusUpdate struct {
	Timestamp    time.Time         `json:"timestamp"`
	Modified     []string          `json:"modified"`
	Added        []string          `json:"added"`
	Deleted      []string          `json:"deleted"`
	Untracked    []string          `json:"untracked"`
	Renamed      []string          `json:"renamed"`
	Ahead        int               `json:"ahead"`
	Behind       int               `json:"behind"`
	Branch       string            `json:"branch"`
	RemoteBranch string            `json:"remote_branch,omitempty"`
	Files        map[string]FileInfo `json:"files,omitempty"`
}

// FileInfo represents information about a file
type FileInfo struct {
	Path      string `json:"path"`
	Status    string `json:"status"`
	Additions int    `json:"additions,omitempty"`
	Deletions int    `json:"deletions,omitempty"`
	OldPath   string `json:"old_path,omitempty"`
	Diff      string `json:"diff,omitempty"`
}

// Re-export types from process package for convenience
type (
	FileListUpdate         = process.FileListUpdate
	FileEntry              = process.FileEntry
	FileTreeNode           = process.FileTreeNode
	FileTreeRequest        = process.FileTreeRequest
	FileTreeResponse       = process.FileTreeResponse
	FileContentRequest     = process.FileContentRequest
	FileContentResponse    = process.FileContentResponse
	FileChangeNotification = process.FileChangeNotification
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
			conn.Close()
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
		c.gitStatusConn.Close()
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
			conn.Close()
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
	defer resp.Body.Close()

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
	defer resp.Body.Close()

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
		c.fileChangesConn.Close()
		c.fileChangesConn = nil
	}
}
