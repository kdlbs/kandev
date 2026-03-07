package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

// StartShellTerminal creates a new per-terminal shell session on agentctl.
// Retries on transient connection errors (common with Sprites proxy tunnels).
func (c *Client) StartShellTerminal(ctx context.Context, terminalID string, cols, rows int) error {
	payload, _ := json.Marshal(map[string]any{
		"terminal_id": terminalID,
		"cols":        cols,
		"rows":        rows,
	})

	const maxAttempts = 3
	backoff := 200 * time.Millisecond
	var lastErr error

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/v1/shell/terminal/start", bytes.NewReader(payload))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = err
			if attempt < maxAttempts && isTransientConnError(err) {
				time.Sleep(backoff)
				backoff *= 2
				continue
			}
			return err
		}

		status := resp.StatusCode
		_ = resp.Body.Close()

		if status == http.StatusOK {
			return nil
		}
		return fmt.Errorf("start shell terminal failed: %d", status)
	}
	return lastErr
}

func isTransientConnError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "connection refused") ||
		err == io.EOF ||
		strings.Contains(msg, io.ErrUnexpectedEOF.Error())
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
