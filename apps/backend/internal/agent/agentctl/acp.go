package agentctl

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

// InitializeResponse from agentctl
type InitializeResponse struct {
	Success  bool                  `json:"success"`
	Response *acp.InitializeResponse `json:"response,omitempty"`
	Error    string                `json:"error,omitempty"`
}

// Initialize sends the ACP initialize request
func (c *Client) Initialize(ctx context.Context, clientName, clientVersion string) (*acp.InitializeResponse, error) {
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
	defer resp.Body.Close()

	var result InitializeResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	if !result.Success {
		return nil, fmt.Errorf("initialize failed: %s", result.Error)
	}
	return result.Response, nil
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
	defer resp.Body.Close()

	var result NewSessionResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	if !result.Success {
		return "", fmt.Errorf("new session failed: %s", result.Error)
	}
	return result.SessionID, nil
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
	defer resp.Body.Close()

	var result PromptResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	if !result.Success {
		return nil, fmt.Errorf("prompt failed: %s", result.Error)
	}
	return &result, nil
}

// SessionNotification represents a session update from the agent
type SessionNotification = acp.SessionNotification

// PermissionOption represents a permission choice
type PermissionOption struct {
	OptionID string `json:"option_id"`
	Name     string `json:"name"`
	Kind     string `json:"kind"` // allow_once, allow_always, reject_once, reject_always
}

// PermissionNotification is received when the agent requests permission
type PermissionNotification struct {
	PendingID  string             `json:"pending_id"`
	SessionID  string             `json:"session_id"`
	ToolCallID string             `json:"tool_call_id"`
	Title      string             `json:"title"`
	Options    []PermissionOption `json:"options"`
	CreatedAt  time.Time          `json:"created_at"`
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
func (c *Client) StreamUpdates(ctx context.Context, handler func(acp.SessionNotification)) error {
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
			conn.Close()
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

			var notification acp.SessionNotification
			if err := json.Unmarshal(message, &notification); err != nil {
				c.logger.Warn("failed to parse session notification", zap.Error(err))
				continue
			}

			handler(notification)
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
		c.acpConn.Close()
		c.acpConn = nil
	}
}

// StreamOutput opens a WebSocket connection for streaming output
func (c *Client) StreamOutput(ctx context.Context, includeHistory bool, handler func(*OutputLine)) error {
	wsURL := "ws" + c.baseURL[4:] + "/api/v1/output/stream"
	if includeHistory {
		wsURL += "?history=true"
	}

	conn, _, err := websocket.DefaultDialer.DialContext(ctx, wsURL, nil)
	if err != nil {
		return fmt.Errorf("failed to connect to output stream: %w", err)
	}

	c.mu.Lock()
	c.outputConn = conn
	c.mu.Unlock()

	c.logger.Info("connected to output stream", zap.String("url", wsURL))

	// Read messages in a goroutine
	go func() {
		defer func() {
			c.mu.Lock()
			c.outputConn = nil
			c.mu.Unlock()
			conn.Close()
		}()

		for {
			_, message, err := conn.ReadMessage()
			if err != nil {
				if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
					c.logger.Info("output stream closed normally")
				} else {
					c.logger.Debug("output stream error", zap.Error(err))
				}
				return
			}

			var line OutputLine
			if err := json.Unmarshal(message, &line); err != nil {
				c.logger.Warn("failed to parse output line", zap.Error(err))
				continue
			}

			handler(&line)
		}
	}()

	return nil
}

// CloseOutputStream closes the output stream connection
func (c *Client) CloseOutputStream() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.outputConn != nil {
		c.outputConn.Close()
		c.outputConn = nil
	}
}

// Close closes all connections
func (c *Client) Close() {
	c.CloseACPStream()
	c.CloseOutputStream()
	c.ClosePermissionStream()
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
			conn.Close()
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
		c.permissionConn.Close()
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
	defer resp.Body.Close()

	var result PermissionRespondResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}
	if !result.Success {
		return fmt.Errorf("permission response failed: %s", result.Error)
	}
	return nil
}
