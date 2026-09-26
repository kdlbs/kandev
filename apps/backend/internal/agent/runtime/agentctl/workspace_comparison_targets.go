package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/kandev/kandev/internal/task/models"
)

// SetComparisonTargets replaces the agentctl instance's per-repository
// provider-qualified comparison target map. The backend calls this after a
// task attachment changes; agentctl materializes each target and keeps an
// explicit unavailable state when a target cannot be fetched.
func (c *Client) SetComparisonTargets(ctx context.Context, targets map[string]models.ComparisonTarget) error {
	body, err := json.Marshal(struct {
		ComparisonTargets map[string]models.ComparisonTarget `json:"comparison_targets"`
	}{ComparisonTargets: targets})
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/workspace/comparison-targets", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := readResponseBody(resp)
		return fmt.Errorf("set comparison targets failed with status %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}
