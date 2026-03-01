package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/kandev/kandev/internal/agentctl/server/utility"
)

// InferencePrompt executes a one-shot inference prompt via agentctl.
func (c *Client) InferencePrompt(ctx context.Context, req *utility.PromptRequest) (*utility.PromptResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/v1/inference/prompt", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := readResponseBody(resp)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("utility prompt failed with status %d: %s", resp.StatusCode, truncateBody(respBody))
	}

	var result utility.PromptResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response (status %d, body: %s): %w", resp.StatusCode, truncateBody(respBody), err)
	}

	return &result, nil
}
