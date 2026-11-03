package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
)

// RequireCapability fails closed when the target agentctl does not advertise
// the named capability. Retained sidecars may ignore newer request fields, so
// every caller that relies on one must negotiate first.
func (c *Client) RequireCapability(ctx context.Context, capability string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/identity", nil)
	if err != nil {
		return fmt.Errorf("create agentctl identity request: %w", err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("read agentctl identity: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusNotFound {
			return fmt.Errorf(
				"agentctl does not support %s; update the execution runtime before retrying",
				capability,
			)
		}
		return fmt.Errorf("read agentctl identity: status %d", resp.StatusCode)
	}
	var identity struct {
		Capabilities []string `json:"capabilities"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&identity); err != nil {
		return fmt.Errorf("decode agentctl identity: %w", err)
	}
	if slices.Contains(identity.Capabilities, capability) {
		return nil
	}
	return fmt.Errorf(
		"agentctl does not support %s; update the execution runtime before retrying",
		capability,
	)
}
