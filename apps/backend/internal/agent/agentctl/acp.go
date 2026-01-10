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

// Prompt sends a prompt to the agent (non-blocking - returns immediately, use StreamUpdates for responses)
func (c *Client) Prompt(ctx context.Context, text string) error {
	reqBody := struct {
		Text string `json:"text"`
	}{Text: text}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return err
	}

	// Use a longer timeout for prompt - agent may take time
	client := &http.Client{
		Timeout: 10 * time.Minute,
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/v1/acp/prompt", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
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
		return fmt.Errorf("prompt failed: %s", result.Error)
	}
	return nil
}

// SessionNotification represents a session update from the agent
type SessionNotification = acp.SessionNotification

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
}

