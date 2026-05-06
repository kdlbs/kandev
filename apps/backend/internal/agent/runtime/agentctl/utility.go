package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/kandev/kandev/internal/agentctl/server/utility"
)

// doLongRunningJSON posts a JSON body to the given path on the long-running
// HTTP client and decodes a JSON response body into out. It is used by the
// utility endpoints (inference prompt, probe) where the underlying LLM call
// or agent cold-start can take minutes.
func (c *Client) doLongRunningJSON(ctx context.Context, path, label string, in, out any) error {
	body, err := json.Marshal(in)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.longRunningHTTPClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := readResponseBody(resp)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%s failed with status %d: %s", label, resp.StatusCode, truncateBody(respBody))
	}
	if err := json.Unmarshal(respBody, out); err != nil {
		return fmt.Errorf("failed to parse %s response (status %d, body: %s): %w", label, resp.StatusCode, truncateBody(respBody), err)
	}
	return nil
}

// InferencePrompt executes a one-shot inference prompt via agentctl.
// Uses the long-running HTTP client since LLM inference can take several minutes.
func (c *Client) InferencePrompt(ctx context.Context, req *utility.PromptRequest) (*utility.PromptResponse, error) {
	var result utility.PromptResponse
	if err := c.doLongRunningJSON(ctx, "/api/v1/inference/prompt", "utility prompt", req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// Probe runs an ACP handshake (initialize + session/new) against the agent
// to discover its capabilities, auth methods, models, and modes.
func (c *Client) Probe(ctx context.Context, req *utility.ProbeRequest) (*utility.ProbeResponse, error) {
	var result utility.ProbeResponse
	if err := c.doLongRunningJSON(ctx, "/api/v1/inference/probe", "probe", req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}
