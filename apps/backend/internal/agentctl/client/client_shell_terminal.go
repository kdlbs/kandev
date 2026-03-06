package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gorilla/websocket"
)

// StartShellTerminal creates a new per-terminal shell session on agentctl.
func (c *Client) StartShellTerminal(ctx context.Context, terminalID string, cols, rows int) error {
	body, _ := json.Marshal(map[string]any{
		"terminal_id": terminalID,
		"cols":        cols,
		"rows":        rows,
	})

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/v1/shell/terminal/start", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("start shell terminal failed: %s", resp.Status)
	}
	return nil
}

// StreamShellTerminal opens a binary WebSocket connection to a per-terminal shell.
// The caller is responsible for closing the returned connection.
func (c *Client) StreamShellTerminal(ctx context.Context, terminalID string) (*websocket.Conn, error) {
	wsURL := "ws" + c.baseURL[4:] + "/api/v1/shell/terminal/" + terminalID + "/stream"
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, wsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to shell terminal stream: %w", err)
	}
	return conn, nil
}

// ShellTerminalBuffer returns the buffered output for a per-terminal shell session.
func (c *Client) ShellTerminalBuffer(ctx context.Context, terminalID string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/api/v1/shell/terminal/"+terminalID+"/buffer", nil)
	if err != nil {
		return "", err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("shell terminal buffer failed: %s", resp.Status)
	}

	var result ShellBufferResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	return result.Data, nil
}

// StopShellTerminal stops a per-terminal shell session.
func (c *Client) StopShellTerminal(ctx context.Context, terminalID string) error {
	req, err := http.NewRequestWithContext(ctx, "DELETE", c.baseURL+"/api/v1/shell/terminal/"+terminalID, nil)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("stop shell terminal failed: %s", resp.Status)
	}
	return nil
}
