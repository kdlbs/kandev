package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/coder/acp-go-sdk"
	"github.com/gorilla/websocket"
	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/common/constants"
	"go.uber.org/zap"
)

// Re-export stream types for convenience.
type (
	AgentEvent                = streams.AgentEvent
	PermissionNotification    = streams.PermissionNotification
	PermissionOption          = streams.PermissionOption
	PermissionRespondRequest  = streams.PermissionRespondRequest
	PermissionRespondResponse = streams.PermissionRespondResponse
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

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/v1/agent/initialize", bytes.NewReader(body))
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
func (c *Client) NewSession(ctx context.Context, cwd string, mcpServers []types.McpServer) (string, error) {
	reqBody := struct {
		Cwd        string            `json:"cwd"`
		McpServers []types.McpServer `json:"mcp_servers,omitempty"`
	}{Cwd: cwd, McpServers: mcpServers}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/v1/agent/session/new", bytes.NewReader(body))
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

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/v1/agent/session/load", bytes.NewReader(body))
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
		Timeout: constants.PromptTimeout,
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/v1/agent/prompt", bytes.NewReader(body))
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

// Note: AgentEvent, PermissionNotification, PermissionOption, and
// PermissionRespondRequest are re-exported from streams package at the top of this file.

// StreamUpdates opens a WebSocket connection for streaming agent events.
// Events include message chunks, reasoning, tool calls, plan updates, and completion/error.
func (c *Client) StreamUpdates(ctx context.Context, handler func(AgentEvent)) error {
	wsURL := "ws" + c.baseURL[4:] + "/api/v1/agent/stream"

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

			var event AgentEvent
			if err := json.Unmarshal(message, &event); err != nil {
				c.logger.Warn("failed to parse agent event", zap.Error(err))
				continue
			}

			handler(event)
		}
	}()

	return nil
}

// CloseUpdatesStream closes the agent events stream connection.
func (c *Client) CloseUpdatesStream() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.acpConn != nil {
		if err := c.acpConn.Close(); err != nil {
			c.logger.Debug("failed to close agent events stream", zap.Error(err))
		}
		c.acpConn = nil
	}
}

// CancelResponse is the response from the agentctl cancel endpoint.
type CancelResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

// Cancel interrupts the current agent turn.
func (c *Client) Cancel(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/v1/agent/cancel", nil)
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
			c.logger.Debug("failed to close cancel response body", zap.Error(err))
		}
	}()

	respBody, err := readResponseBody(resp)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("cancel request failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var result CancelResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return fmt.Errorf("failed to parse cancel response (status %d, body: %s): %w", resp.StatusCode, truncateBody(respBody), err)
	}
	if !result.Success {
		return fmt.Errorf("cancel failed: %s", result.Error)
	}
	return nil
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

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/v1/agent/permissions/respond", bytes.NewReader(body))
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
