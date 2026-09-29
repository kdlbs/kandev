package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

// ExecuteBackgroundWorkAction executes an action on a background workload in agentctl.
func (c *Client) ExecuteBackgroundWorkAction(ctx context.Context, req streams.BackgroundWorkActionRequest) (streams.BackgroundWorkActionResponse, error) {
	var resp streams.BackgroundWorkActionResponse
	bodyBytes, err := json.Marshal(req)
	if err != nil {
		return resp, fmt.Errorf("marshal background action request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/v1/agent/background-work/action", bytes.NewReader(bodyBytes))
	if err != nil {
		return resp, fmt.Errorf("create background action request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpResp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return resp, fmt.Errorf("execute background action: %w", err)
	}
	defer func() { _ = httpResp.Body.Close() }()
	if err := json.NewDecoder(httpResp.Body).Decode(&resp); err != nil {
		return resp, fmt.Errorf("decode background action response: %w", err)
	}
	return resp, nil
}
