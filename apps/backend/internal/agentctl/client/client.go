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
	"github.com/kandev/kandev/internal/agentctl/tracing"
	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/common/logger"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

// Client communicates with agentctl via HTTP and WebSocket
type Client struct {
	baseURL     string
	httpClient  *http.Client
	logger      *logger.Logger
	executionID string
	sessionID   string

	// Optional trace context for session-scoped spans in background goroutines.
	// When set, stream read loops use this as parent context for tracing instead of context.Background().
	traceCtx context.Context

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

// ClientOption configures optional Client settings.
type ClientOption func(*Client)

// WithExecutionID sets the execution ID used for tracing spans.
func WithExecutionID(id string) ClientOption {
	return func(c *Client) {
		c.executionID = id
	}
}

// WithSessionID sets the session ID used for tracing spans.
func WithSessionID(id string) ClientOption {
	return func(c *Client) {
		c.sessionID = id
	}
}

// SetTraceContext sets the trace context used as parent for spans created in
// background goroutines (stream read loops). Thread-safe: can be called after construction.
func (c *Client) SetTraceContext(ctx context.Context) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.traceCtx = ctx
}

// getTraceCtx returns the trace context for background operations.
// Returns context.Background() when no trace context is set.
func (c *Client) getTraceCtx() context.Context {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.traceCtx != nil {
		return c.traceCtx
	}
	return context.Background()
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
func NewClient(host string, port int, log *logger.Logger, opts ...ClientOption) *Client {
	c := &Client{
		baseURL: fmt.Sprintf("http://%s:%d", host, port),
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
		logger:          log.WithFields(zap.String("component", "agentctl-client")),
		pendingRequests: make(map[string]chan *ws.Message),
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
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
	ctx, span := tracing.TraceHTTPRequest(ctx, "GET", "/api/v1/status", c.executionID)
	defer span.End()

	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/api/v1/status", nil)
	if err != nil {
		tracing.TraceHTTPResponse(span, 0, err)
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		tracing.TraceHTTPResponse(span, 0, err)
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := readResponseBody(resp)
	if err != nil {
		tracing.TraceHTTPResponse(span, resp.StatusCode, err)
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		httpErr := fmt.Errorf("status request failed with status %d: %s", resp.StatusCode, string(respBody))
		tracing.TraceHTTPResponse(span, resp.StatusCode, httpErr)
		return nil, httpErr
	}

	var status StatusResponse
	if err := json.Unmarshal(respBody, &status); err != nil {
		tracing.TraceHTTPResponse(span, resp.StatusCode, err)
		return nil, fmt.Errorf("failed to parse status response (status %d, body: %s): %w", resp.StatusCode, truncateBody(respBody), err)
	}

	tracing.TraceHTTPResponse(span, resp.StatusCode, nil)
	return &status, nil
}

// ConfigureAgent configures the agent command and optional approval policy. Must be called before Start().
func (c *Client) ConfigureAgent(ctx context.Context, command string, env map[string]string, approvalPolicy string) error {
	ctx, span := tracing.TraceHTTPRequest(ctx, "POST", "/api/v1/agent/configure", c.executionID)
	defer span.End()

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
		tracing.TraceHTTPResponse(span, 0, err)
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/v1/agent/configure", bytes.NewReader(body))
	if err != nil {
		tracing.TraceHTTPResponse(span, 0, err)
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		tracing.TraceHTTPResponse(span, 0, err)
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := readResponseBody(resp)
	if err != nil {
		tracing.TraceHTTPResponse(span, resp.StatusCode, err)
		return fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		httpErr := fmt.Errorf("configure request failed with status %d: %s", resp.StatusCode, string(respBody))
		tracing.TraceHTTPResponse(span, resp.StatusCode, httpErr)
		return httpErr
	}

	var result struct {
		Success bool   `json:"success"`
		Error   string `json:"error,omitempty"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		tracing.TraceHTTPResponse(span, resp.StatusCode, err)
		return fmt.Errorf("failed to parse configure response (status %d, body: %s): %w", resp.StatusCode, truncateBody(respBody), err)
	}
	if !result.Success {
		cfgErr := fmt.Errorf("configure failed: %s", result.Error)
		tracing.TraceHTTPResponse(span, resp.StatusCode, cfgErr)
		return cfgErr
	}

	tracing.TraceHTTPResponse(span, resp.StatusCode, nil)
	return nil
}

// Start starts the agent process and returns the full command that was executed.
func (c *Client) Start(ctx context.Context) (string, error) {
	ctx, span := tracing.TraceHTTPRequest(ctx, "POST", "/api/v1/start", c.executionID)
	defer span.End()

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/v1/start", nil)
	if err != nil {
		tracing.TraceHTTPResponse(span, 0, err)
		return "", err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		tracing.TraceHTTPResponse(span, 0, err)
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := readResponseBody(resp)
	if err != nil {
		tracing.TraceHTTPResponse(span, resp.StatusCode, err)
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		httpErr := fmt.Errorf("start request failed with status %d: %s", resp.StatusCode, string(respBody))
		tracing.TraceHTTPResponse(span, resp.StatusCode, httpErr)
		return "", httpErr
	}

	var result struct {
		Success bool   `json:"success"`
		Command string `json:"command,omitempty"`
		Error   string `json:"error,omitempty"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		tracing.TraceHTTPResponse(span, resp.StatusCode, err)
		return "", fmt.Errorf("failed to parse start response (status %d, body: %s): %w", resp.StatusCode, truncateBody(respBody), err)
	}
	if !result.Success {
		startErr := fmt.Errorf("start failed: %s", result.Error)
		tracing.TraceHTTPResponse(span, resp.StatusCode, startErr)
		return "", startErr
	}

	tracing.TraceHTTPResponse(span, resp.StatusCode, nil)
	return result.Command, nil
}

// Stop stops the agent process
func (c *Client) Stop(ctx context.Context) error {
	ctx, span := tracing.TraceHTTPRequest(ctx, "POST", "/api/v1/stop", c.executionID)
	defer span.End()

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/v1/stop", nil)
	if err != nil {
		tracing.TraceHTTPResponse(span, 0, err)
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		tracing.TraceHTTPResponse(span, 0, err)
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := readResponseBody(resp)
	if err != nil {
		tracing.TraceHTTPResponse(span, resp.StatusCode, err)
		return fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		httpErr := fmt.Errorf("stop request failed with status %d: %s", resp.StatusCode, string(respBody))
		tracing.TraceHTTPResponse(span, resp.StatusCode, httpErr)
		return httpErr
	}

	var result struct {
		Success bool   `json:"success"`
		Error   string `json:"error,omitempty"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		tracing.TraceHTTPResponse(span, resp.StatusCode, err)
		return fmt.Errorf("failed to parse stop response (status %d, body: %s): %w", resp.StatusCode, truncateBody(respBody), err)
	}
	if !result.Success {
		stopErr := fmt.Errorf("stop failed: %s", result.Error)
		tracing.TraceHTTPResponse(span, resp.StatusCode, stopErr)
		return stopErr
	}

	tracing.TraceHTTPResponse(span, resp.StatusCode, nil)
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
