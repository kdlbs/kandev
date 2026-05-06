package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// ListeningPort represents a TCP port with an active listener on the remote executor.
type ListeningPort struct {
	Port    int    `json:"port"`
	Address string `json:"address"`
	Process string `json:"process,omitempty"`
}

// ListPorts returns all TCP ports currently listening inside the executor.
func (c *Client) ListPorts(ctx context.Context) ([]ListeningPort, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/api/v1/ports", nil)
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
		return nil, fmt.Errorf("list ports failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Ports []ListeningPort `json:"ports"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse list ports response: %w", err)
	}
	return result.Ports, nil
}
