package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
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

// Prompt sends a fire-and-forget prompt to the agent.
// The agentctl server returns 202 immediately; completion is signaled via the WebSocket complete event.
// Attachments (images) are passed to the agent if provided.
func (c *Client) Prompt(ctx context.Context, text string, attachments []v1.MessageAttachment) error {
	reqBody := struct {
		Text        string                 `json:"text"`
		Attachments []v1.MessageAttachment `json:"attachments,omitempty"`
	}{Text: text, Attachments: attachments}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/v1/agent/prompt", bytes.NewReader(body))
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
			c.logger.Debug("failed to close prompt response body", zap.Error(err))
		}
	}()

	respBody, err := readResponseBody(resp)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	// Accept both 200 (backwards compat) and 202 (async)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("prompt request failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Success bool   `json:"success"`
		Error   string `json:"error,omitempty"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return fmt.Errorf("failed to parse prompt response (status %d, body: %s): %w", resp.StatusCode, truncateBody(respBody), err)
	}
	if !result.Success {
		c.logger.Warn("prompt returned failure response",
			zap.String("error", result.Error),
			zap.Int("status_code", resp.StatusCode),
			zap.String("response_body", truncateBody(respBody)))
		return fmt.Errorf("prompt failed: %s", result.Error)
	}
	return nil
}

// Note: AgentEvent, PermissionNotification, PermissionOption, and
// PermissionRespondRequest are re-exported from streams package at the top of this file.

// MCPHandler is the interface for handling MCP requests from agentctl.
type MCPHandler interface {
	// Dispatch handles an MCP request and returns a response.
	Dispatch(ctx context.Context, msg *ws.Message) (*ws.Message, error)
}

// StreamUpdates opens a WebSocket connection for streaming agent events.
// Events include message chunks, reasoning, tool calls, plan updates, and completion/error.
// If mcpHandler is provided, MCP requests from agentctl will be dispatched to it and responses sent back.
// If onDisconnect is provided, it is called when the WebSocket read goroutine exits (e.g., on error or close).
func (c *Client) StreamUpdates(ctx context.Context, handler func(AgentEvent), mcpHandler MCPHandler, onDisconnect func(err error)) error {
	wsURL := "ws" + c.baseURL[4:] + "/api/v1/agent/stream"

	conn, _, err := websocket.DefaultDialer.DialContext(ctx, wsURL, nil)
	if err != nil {
		return fmt.Errorf("failed to connect to updates stream: %w", err)
	}

	c.mu.Lock()
	c.agentStreamConn = conn
	c.mu.Unlock()

	c.logger.Info("connected to updates stream", zap.String("url", wsURL))

	// Use mutex for writing responses
	var writeMu sync.Mutex
	writeMessage := func(data []byte) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		return conn.WriteMessage(websocket.TextMessage, data)
	}

	// Read messages in a goroutine
	go func() {
		var lastErr error
		defer func() {
			c.mu.Lock()
			c.agentStreamConn = nil
			c.mu.Unlock()
			if err := conn.Close(); err != nil {
				c.logger.Debug("failed to close updates websocket", zap.Error(err))
			}
			// Signal disconnect to caller
			if onDisconnect != nil {
				onDisconnect(lastErr)
			}
		}()

		for {
			_, message, err := conn.ReadMessage()
			if err != nil {
				if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
					c.logger.Info("updates stream closed normally")
					// Normal close — don't report as disconnect error
				} else {
					c.logger.Debug("updates stream error", zap.Error(err))
					lastErr = err
				}
				return
			}

			// Try to parse as ws.Message to check if it's an MCP request
			var wsMsg ws.Message
			if err := json.Unmarshal(message, &wsMsg); err == nil && wsMsg.Type == ws.MessageTypeRequest {
				// This is an MCP request - dispatch it
				if mcpHandler != nil {
					go func(msg ws.Message) {
						resp, err := mcpHandler.Dispatch(ctx, &msg)
						if err != nil {
							c.logger.Error("MCP dispatch error", zap.Error(err))
							resp, _ = ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, err.Error(), nil)
						}
						if resp != nil {
							data, err := json.Marshal(resp)
							if err != nil {
								c.logger.Error("failed to marshal MCP response", zap.Error(err))
								return
							}
							if err := writeMessage(data); err != nil {
								c.logger.Debug("failed to write MCP response", zap.Error(err))
							}
						}
					}(wsMsg)
				}
				continue
			}

			// Not an MCP request, parse as agent event
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

	if c.agentStreamConn != nil {
		if err := c.agentStreamConn.Close(); err != nil {
			c.logger.Debug("failed to close agent events stream", zap.Error(err))
		}
		c.agentStreamConn = nil
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
// GetAgentStderr returns recent stderr lines from the agent process.
func (c *Client) GetAgentStderr(ctx context.Context) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/api/v1/agent/stderr", nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("agent stderr request failed with status %d", resp.StatusCode)
	}

	var result struct {
		Lines []string `json:"lines"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to parse agent stderr response: %w", err)
	}
	return result.Lines, nil
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
