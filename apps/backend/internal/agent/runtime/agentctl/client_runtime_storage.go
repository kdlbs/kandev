package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

// RuntimeStorage reads the executor-scoped advisory capacity snapshot. A
// missing route on an older agentctl is an unsupported capability, never a
// delivery downgrade or a backend-host measurement.
func (c *Client) RuntimeStorage(ctx context.Context) (*streams.RuntimeStorageSnapshot, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/v1/runtime/storage", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := readResponseBody(resp)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, &DeliveryHTTPError{StatusCode: resp.StatusCode, Body: truncateBody(body)}
	}
	var snapshot streams.RuntimeStorageSnapshot
	if err := json.Unmarshal(body, &snapshot); err != nil {
		return nil, fmt.Errorf("parse runtime storage response: %w", err)
	}
	normalized := snapshot.Normalize()
	if normalized == nil {
		return nil, fmt.Errorf("unsupported runtime storage response")
	}
	return normalized, nil
}
