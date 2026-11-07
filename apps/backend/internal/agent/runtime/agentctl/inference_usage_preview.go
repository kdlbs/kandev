package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/kandev/kandev/internal/agent/usage"
	"github.com/kandev/kandev/internal/agentctl/server/utility"
	"net/http"
	"strings"
)

func (c *Client) PreviewInferenceUsage(ctx context.Context, input *utility.UsagePreviewRequest) (*usage.ExecutionUsageReport, error) {
	body, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/inference/usage/preview", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := readResponseBody(resp)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusNotFound && strings.HasPrefix(resp.Header.Get("Content-Type"), "text/plain") && strings.TrimSpace(string(data)) == "404 page not found" {
		return &usage.ExecutionUsageReport{Reason: "native_usage_unsupported"}, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("inference usage preview failed: status %d", resp.StatusCode)
	}
	var report usage.ExecutionUsageReport
	if err := json.Unmarshal(data, &report); err != nil {
		return nil, fmt.Errorf("invalid inference usage preview")
	}
	return &report, nil
}
