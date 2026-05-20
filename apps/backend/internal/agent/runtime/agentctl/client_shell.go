package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// Note: ShellStatusResponse, ShellMessage, ShellBufferResponse are re-exported
// from types package in client.go.

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
