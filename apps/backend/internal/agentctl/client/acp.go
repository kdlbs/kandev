package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/coder/acp-go-sdk"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

// AgentInfo contains information about the connected agent.
type AgentInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// InitializeResponse from agentctl
type InitializeResponse struct {
	Success   bool       `json:"success"`
	AgentInfo *AgentInfo `json:"agent_info,omitempty"`
	Error     string     `json:"error,omitempty"`
}

// Initialize sends the ACP initialize request
func (c *Client) Initialize(ctx context.Context, clientName, clientVersion string) (*AgentInfo, error) {
	reqBody := struct {
		ClientName    string `json:"client_name"`
		ClientVersion string `json:"client_version"`
	}{
		ClientName:    clientName,
		ClientVersion: clientVersion,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/v1/acp/initialize", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			c.logger.Debug("failed to close initialize response body", zap.Error(err))
		}
	}()

	respBody, err := readResponseBody(resp)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("initialize request failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var result InitializeResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse initialize response (status %d, body: %s): %w", resp.StatusCode, truncateBody(respBody), err)
	}
	if !result.Success {
		return nil, fmt.Errorf("initialize failed: %s", result.Error)
	}
	return result.AgentInfo, nil
}

// NewSessionResponse from agentctl
type NewSessionResponse struct {
	Success   bool   `json:"success"`
	SessionID string `json:"session_id,omitempty"`
	Error     string `json:"error,omitempty"`
}

// NewSession creates a new ACP session
func (c *Client) NewSession(ctx context.Context, cwd string) (string, error) {
	reqBody := struct {
		Cwd string `json:"cwd"`
	}{Cwd: cwd}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/v1/acp/session/new", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			c.logger.Debug("failed to close session response body", zap.Error(err))
		}
	}()

	respBody, err := readResponseBody(resp)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("new session request failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var result NewSessionResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("failed to parse new session response (status %d, body: %s): %w", resp.StatusCode, truncateBody(respBody), err)
	}
	if !result.Success {
		return "", fmt.Errorf("new session failed: %s", result.Error)
	}
	return result.SessionID, nil
}

// LoadSession resumes an existing ACP session
func (c *Client) LoadSession(ctx context.Context, sessionID string) error {
	reqBody := struct {
		SessionID string `json:"session_id"`
	}{SessionID: sessionID}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/v1/acp/session/load", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			c.logger.Debug("failed to close load session response body", zap.Error(err))
		}
	}()

	respBody, err := readResponseBody(resp)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("load session request failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Success bool   `json:"success"`
		Error   string `json:"error,omitempty"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return fmt.Errorf("failed to parse load session response (status %d, body: %s): %w", resp.StatusCode, truncateBody(respBody), err)
	}
	if !result.Success {
		return fmt.Errorf("load session failed: %s", result.Error)
	}
	return nil
}

// PromptResponse contains the response from a prompt call
type PromptResponse struct {
	Success    bool           `json:"success"`
	StopReason acp.StopReason `json:"stop_reason,omitempty"`
	Error      string         `json:"error,omitempty"`
}

// Prompt sends a prompt to the agent and returns when the agent completes
// The StopReason indicates why the agent stopped (e.g., "end_turn", "needs_input")
func (c *Client) Prompt(ctx context.Context, text string) (*PromptResponse, error) {
	reqBody := struct {
		Text string `json:"text"`
	}{Text: text}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	// Use a longer timeout for prompt - agent may take time
	client := &http.Client{
		Timeout: 10 * time.Minute,
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/v1/acp/prompt", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			c.logger.Debug("failed to close prompt response body", zap.Error(err))
		}
	}()

	respBody, err := readResponseBody(resp)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("prompt request failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var result PromptResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse prompt response (status %d, body: %s): %w", resp.StatusCode, truncateBody(respBody), err)
	}
	if !result.Success {
		return nil, fmt.Errorf("prompt failed: %s", result.Error)
	}
	return &result, nil
}

// SessionUpdate represents a session update from the agent.
// This matches adapter.SessionUpdate that agentctl sends over WebSocket.
type SessionUpdate struct {
	Type        string `json:"type"`
	SessionID   string `json:"session_id,omitempty"`
	OperationID string `json:"operation_id,omitempty"` // Turn/operation ID for cancellation

	// Message fields
	Text string `json:"text,omitempty"`

	// Reasoning fields (for "reasoning" type)
	ReasoningText    string `json:"reasoning_text,omitempty"`
	ReasoningSummary string `json:"reasoning_summary,omitempty"`

	// Tool call fields
	ToolCallID string                 `json:"tool_call_id,omitempty"`
	ToolName   string                 `json:"tool_name,omitempty"`
	ToolTitle  string                 `json:"tool_title,omitempty"`
	ToolStatus string                 `json:"tool_status,omitempty"`
	ToolArgs   map[string]interface{} `json:"tool_args,omitempty"`
	ToolResult interface{}            `json:"tool_result,omitempty"`

	// Diff for file changes
	Diff string `json:"diff,omitempty"`

	// Error and raw data
	Error string                 `json:"error,omitempty"`
	Data  map[string]interface{} `json:"data,omitempty"`
}

// PermissionOption represents a permission choice
type PermissionOption struct {
	OptionID string `json:"option_id"`
	Name     string `json:"name"`
	Kind     string `json:"kind"` // allow_once, allow_always, reject_once, reject_always
}

// PermissionNotification is received when the agent requests permission
type PermissionNotification struct {
	PendingID     string                 `json:"pending_id"`
	SessionID     string                 `json:"session_id"`
	ToolCallID    string                 `json:"tool_call_id"`
	Title         string                 `json:"title"`
	Options       []PermissionOption     `json:"options"`
	ActionType    string                 `json:"action_type,omitempty"`    // command, file_write, file_read, network, mcp_tool, other
	ActionDetails map[string]interface{} `json:"action_details,omitempty"` // Additional details about the action
	CreatedAt     time.Time              `json:"created_at"`
}

// PermissionResponse is the user's response to a permission request
type PermissionRespondRequest struct {
	PendingID string `json:"pending_id"`
	OptionID  string `json:"option_id,omitempty"`
	Cancelled bool   `json:"cancelled,omitempty"`
}

// PermissionRespondResponse from agentctl
type PermissionRespondResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

// StreamUpdates opens a WebSocket connection for streaming session updates
func (c *Client) StreamUpdates(ctx context.Context, handler func(SessionUpdate)) error {
	wsURL := "ws" + c.baseURL[4:] + "/api/v1/acp/stream"

	conn, _, err := websocket.DefaultDialer.DialContext(ctx, wsURL, nil)
	if err != nil {
		return fmt.Errorf("failed to connect to updates stream: %w", err)
	}

	c.mu.Lock()
	c.acpConn = conn
	c.mu.Unlock()

	c.logger.Info("connected to updates stream", zap.String("url", wsURL))

	// Read messages in a goroutine
	go func() {
		defer func() {
			c.mu.Lock()
			c.acpConn = nil
			c.mu.Unlock()
			if err := conn.Close(); err != nil {
				c.logger.Debug("failed to close updates websocket", zap.Error(err))
			}
		}()

		for {
			_, message, err := conn.ReadMessage()
			if err != nil {
				if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
					c.logger.Info("updates stream closed normally")
				} else {
					c.logger.Debug("updates stream error", zap.Error(err))
				}
				return
			}

			var update SessionUpdate
			if err := json.Unmarshal(message, &update); err != nil {
				c.logger.Warn("failed to parse session update", zap.Error(err))
				continue
			}

			handler(update)
		}
	}()

	return nil
}

// CloseUpdatesStream closes the updates stream connection
func (c *Client) CloseUpdatesStream() {
	c.CloseACPStream()
}

// CloseACPStream closes the ACP stream connection
func (c *Client) CloseACPStream() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.acpConn != nil {
		if err := c.acpConn.Close(); err != nil {
			c.logger.Debug("failed to close ACP stream", zap.Error(err))
		}
		c.acpConn = nil
	}
}

// StreamPermissions opens a WebSocket connection for streaming permission requests
func (c *Client) StreamPermissions(ctx context.Context, handler func(*PermissionNotification)) error {
	wsURL := "ws" + c.baseURL[4:] + "/api/v1/acp/permissions/stream"

	conn, _, err := websocket.DefaultDialer.DialContext(ctx, wsURL, nil)
	if err != nil {
		return fmt.Errorf("failed to connect to permission stream: %w", err)
	}

	c.mu.Lock()
	c.permissionConn = conn
	c.mu.Unlock()

	c.logger.Info("connected to permission stream", zap.String("url", wsURL))

	// Read messages in a goroutine
	go func() {
		defer func() {
			c.mu.Lock()
			c.permissionConn = nil
			c.mu.Unlock()
			if err := conn.Close(); err != nil {
				c.logger.Debug("failed to close permissions websocket", zap.Error(err))
			}
		}()

		for {
			_, message, err := conn.ReadMessage()
			if err != nil {
				if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
					c.logger.Info("permission stream closed normally")
				} else {
					c.logger.Debug("permission stream error", zap.Error(err))
				}
				return
			}

			var notification PermissionNotification
			if err := json.Unmarshal(message, &notification); err != nil {
				c.logger.Warn("failed to parse permission notification", zap.Error(err))
				continue
			}

			handler(&notification)
		}
	}()

	return nil
}

// ClosePermissionStream closes the permission stream connection
func (c *Client) ClosePermissionStream() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.permissionConn != nil {
		if err := c.permissionConn.Close(); err != nil {
			c.logger.Debug("failed to close permission stream", zap.Error(err))
		}
		c.permissionConn = nil
	}
}

// RespondToPermission sends a response to a permission request
func (c *Client) RespondToPermission(ctx context.Context, pendingID, optionID string, cancelled bool) error {
	reqBody := PermissionRespondRequest{
		PendingID: pendingID,
		OptionID:  optionID,
		Cancelled: cancelled,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/v1/acp/permissions/respond", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			c.logger.Debug("failed to close permission response body", zap.Error(err))
		}
	}()

	respBody, err := readResponseBody(resp)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("permission response request failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var result PermissionRespondResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return fmt.Errorf("failed to parse permission response (status %d, body: %s): %w", resp.StatusCode, truncateBody(respBody), err)
	}
	if !result.Success {
		return fmt.Errorf("permission response failed: %s", result.Error)
	}
	return nil
}

// GetPendingPermissions retrieves all pending permission requests from agentctl
func (c *Client) GetPendingPermissions(ctx context.Context) ([]PermissionNotification, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/api/v1/acp/permissions", nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			c.logger.Debug("failed to close permissions response body", zap.Error(err))
		}
	}()

	respBody, err := readResponseBody(resp)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("get pending permissions failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var result []PermissionNotification
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse permissions response: %w", err)
	}
	return result, nil
}
