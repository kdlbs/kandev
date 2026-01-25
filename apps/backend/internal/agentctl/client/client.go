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
	acpConn             *websocket.Conn
	workspaceStreamConn *websocket.Conn
	mu                  sync.RWMutex
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

// Re-export types from types package for convenience.
// These types are defined in the streams subpackage and re-exported through types.
type (
	GitStatusUpdate        = types.GitStatusUpdate
	GitCommitNotification  = types.GitCommitNotification
	GitResetNotification   = types.GitResetNotification
	FileInfo               = types.FileInfo
	FileListUpdate         = types.FileListUpdate
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

type StartProcessRequest struct {
	SessionID      string            `json:"session_id"`
	Kind           ProcessKind       `json:"kind"`
	ScriptName     string            `json:"script_name,omitempty"`
	Command        string            `json:"command"`
	WorkingDir     string            `json:"working_dir"`
	Env            map[string]string `json:"env,omitempty"`
	BufferMaxBytes int64             `json:"buffer_max_bytes,omitempty"`
}

type ProcessOutputChunk struct {
	Stream    string    `json:"stream"`
	Data      string    `json:"data"`
	Timestamp time.Time `json:"timestamp"`
}

type ProcessInfo struct {
	ID         string               `json:"id"`
	SessionID  string               `json:"session_id"`
	Kind       ProcessKind          `json:"kind"`
	ScriptName string               `json:"script_name,omitempty"`
	Command    string               `json:"command"`
	WorkingDir string               `json:"working_dir"`
	Status     ProcessStatus        `json:"status"`
	ExitCode   *int                 `json:"exit_code,omitempty"`
	StartedAt  time.Time            `json:"started_at"`
	UpdatedAt  time.Time            `json:"updated_at"`
	Output     []ProcessOutputChunk `json:"output,omitempty"`
}

type startProcessResponse struct {
	Process *ProcessInfo `json:"process,omitempty"`
	Error   string       `json:"error,omitempty"`
}

type stopProcessResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
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

// Close closes all connections
func (c *Client) Close() {
	c.CloseUpdatesStream()
	c.CloseWorkspaceStream()
}

// Note: ShellStatusResponse, ShellMessage, ShellBufferResponse are re-exported
// from streams package at the top of this file in the type block.

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

// StartShell starts the shell session without starting the agent process.
// This is used in passthrough mode where the agent runs directly via InteractiveRunner.
func (c *Client) StartShell(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/v1/shell/start", nil)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("start shell failed: %s", resp.Status)
	}

	return nil
}

func (c *Client) StartProcess(ctx context.Context, req StartProcessRequest) (*ProcessInfo, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/v1/processes/start", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := readResponseBody(resp)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("start process failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var result startProcessResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse start process response: %w", err)
	}
	if result.Process == nil {
		if result.Error != "" {
			return nil, fmt.Errorf("start process failed: %s", result.Error)
		}
		return nil, fmt.Errorf("start process failed: no process returned")
	}
	return result.Process, nil
}

func (c *Client) StopProcess(ctx context.Context, processID string) error {
	body, err := json.Marshal(map[string]string{"process_id": processID})
	if err != nil {
		return err
	}
	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/v1/processes/stop", bytes.NewReader(body))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := readResponseBody(resp)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("stop process failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var result stopProcessResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return fmt.Errorf("failed to parse stop process response: %w", err)
	}
	if !result.Success {
		return fmt.Errorf("stop process failed: %s", result.Error)
	}
	return nil
}

func (c *Client) ListProcesses(ctx context.Context, sessionID string) ([]ProcessInfo, error) {
	url := c.baseURL + "/api/v1/processes"
	if sessionID != "" {
		url = url + "?session_id=" + sessionID
	}
	httpReq, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("list processes failed with status %d", resp.StatusCode)
	}
	var result []ProcessInfo
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result, nil
}

func (c *Client) GetProcess(ctx context.Context, id string, includeOutput bool) (*ProcessInfo, error) {
	url := c.baseURL + "/api/v1/processes/" + id
	if includeOutput {
		url = url + "?include_output=true"
	}
	httpReq, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("get process failed with status %d", resp.StatusCode)
	}
	var result ProcessInfo
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

// WorkspaceStreamCallbacks defines callbacks for workspace stream events
type WorkspaceStreamCallbacks struct {
	OnShellOutput   func(data string)
	OnShellExit     func(code int)
	OnGitStatus     func(update *GitStatusUpdate)
	OnGitCommit     func(notification *GitCommitNotification)
	OnGitReset      func(notification *GitResetNotification)
	OnFileChange    func(notification *FileChangeNotification)
	OnFileList      func(update *FileListUpdate)
	OnProcessOutput func(output *types.ProcessOutput)
	OnProcessStatus func(status *types.ProcessStatusUpdate)
	OnConnected     func()
	OnError         func(err string)
}

// WorkspaceStream represents an active workspace stream connection
type WorkspaceStream struct {
	conn      *websocket.Conn
	inputCh   chan types.WorkspaceStreamMessage
	closeCh   chan struct{}
	closeOnce sync.Once
	logger    *logger.Logger
}

// StreamWorkspace opens a unified WebSocket connection for all workspace events
func (c *Client) StreamWorkspace(ctx context.Context, callbacks WorkspaceStreamCallbacks) (*WorkspaceStream, error) {
	c.mu.Lock()
	if c.workspaceStreamConn != nil {
		c.mu.Unlock()
		return nil, fmt.Errorf("workspace stream already connected")
	}
	c.mu.Unlock()

	wsURL := "ws" + c.baseURL[4:] + "/api/v1/workspace/stream"
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, wsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to workspace stream: %w", err)
	}

	c.mu.Lock()
	c.workspaceStreamConn = conn
	c.mu.Unlock()

	c.logger.Info("connected to workspace stream", zap.String("url", wsURL))

	ws := &WorkspaceStream{
		conn:    conn,
		inputCh: make(chan types.WorkspaceStreamMessage, 64),
		closeCh: make(chan struct{}),
		logger:  c.logger,
	}

	// Read goroutine - dispatches to callbacks
	go func() {
		defer func() {
			c.mu.Lock()
			c.workspaceStreamConn = nil
			c.mu.Unlock()
			ws.Close()
		}()

		for {
			var msg types.WorkspaceStreamMessage
			if err := conn.ReadJSON(&msg); err != nil {
				if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
					c.logger.Debug("workspace stream read error", zap.Error(err))
				}
				return
			}

			switch msg.Type {
			case types.WorkspaceMessageTypeShellOutput:
				if callbacks.OnShellOutput != nil {
					callbacks.OnShellOutput(msg.Data)
				}
			case types.WorkspaceMessageTypeShellExit:
				if callbacks.OnShellExit != nil {
					callbacks.OnShellExit(msg.Code)
				}
			case types.WorkspaceMessageTypeGitStatus:
				if callbacks.OnGitStatus != nil && msg.GitStatus != nil {
					callbacks.OnGitStatus(msg.GitStatus)
				}
			case types.WorkspaceMessageTypeGitCommit:
				if callbacks.OnGitCommit != nil && msg.GitCommit != nil {
					callbacks.OnGitCommit(msg.GitCommit)
				}
			case types.WorkspaceMessageTypeGitReset:
				if callbacks.OnGitReset != nil && msg.GitReset != nil {
					callbacks.OnGitReset(msg.GitReset)
				}
			case types.WorkspaceMessageTypeFileChange:
				if callbacks.OnFileChange != nil && msg.FileChange != nil {
					callbacks.OnFileChange(msg.FileChange)
				}
			case types.WorkspaceMessageTypeFileList:
				if callbacks.OnFileList != nil && msg.FileList != nil {
					callbacks.OnFileList(msg.FileList)
				}
			case types.WorkspaceMessageTypeProcessOutput:
				if callbacks.OnProcessOutput != nil && msg.ProcessOutput != nil {
					callbacks.OnProcessOutput(msg.ProcessOutput)
				}
			case types.WorkspaceMessageTypeProcessStatus:
				if callbacks.OnProcessStatus != nil && msg.ProcessStatus != nil {
					callbacks.OnProcessStatus(msg.ProcessStatus)
				}
			case types.WorkspaceMessageTypeConnected:
				if callbacks.OnConnected != nil {
					callbacks.OnConnected()
				}
			case types.WorkspaceMessageTypeError:
				if callbacks.OnError != nil {
					callbacks.OnError(msg.Error)
				}
			}
		}
	}()

	// Write goroutine - sends from inputCh
	go func() {
		for {
			select {
			case <-ws.closeCh:
				return
			case msg, ok := <-ws.inputCh:
				if !ok {
					return
				}
				if err := conn.WriteJSON(msg); err != nil {
					ws.logger.Debug("workspace stream write error", zap.Error(err))
					return
				}
			}
		}
	}()

	return ws, nil
}

// WriteShellInput sends input to the shell through the workspace stream
func (ws *WorkspaceStream) WriteShellInput(data string) error {
	msg := types.NewWorkspaceShellInput(data)
	select {
	case ws.inputCh <- msg:
		return nil
	case <-ws.closeCh:
		return fmt.Errorf("workspace stream closed")
	}
}

// ResizeShell sends a shell resize command through the workspace stream
func (ws *WorkspaceStream) ResizeShell(cols, rows int) error {
	msg := types.NewWorkspaceShellResize(cols, rows)
	select {
	case ws.inputCh <- msg:
		return nil
	case <-ws.closeCh:
		return fmt.Errorf("workspace stream closed")
	}
}

// Ping sends a ping message through the workspace stream
func (ws *WorkspaceStream) Ping() error {
	msg := types.NewWorkspacePing()
	select {
	case ws.inputCh <- msg:
		return nil
	case <-ws.closeCh:
		return fmt.Errorf("workspace stream closed")
	}
}

// Close closes the workspace stream
func (ws *WorkspaceStream) Close() {
	ws.closeOnce.Do(func() {
		close(ws.closeCh)
		if ws.conn != nil {
			if err := ws.conn.Close(); err != nil {
				ws.logger.Debug("failed to close workspace stream connection", zap.Error(err))
			}
		}
	})
}

// Done returns a channel that is closed when the stream is closed
func (ws *WorkspaceStream) Done() <-chan struct{} {
	return ws.closeCh
}

// CloseWorkspaceStream closes the workspace stream connection
func (c *Client) CloseWorkspaceStream() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.workspaceStreamConn != nil {
		if err := c.workspaceStreamConn.Close(); err != nil {
			c.logger.Debug("failed to close workspace stream", zap.Error(err))
		}
		c.workspaceStreamConn = nil
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
