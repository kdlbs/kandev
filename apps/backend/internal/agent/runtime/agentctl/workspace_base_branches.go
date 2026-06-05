package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// SetBaseBranches replaces the agentctl instance's per-repo base-branch map.
// Called by the kandev backend after persisting a new
// task_repositories.base_branch value via the changes-panel "Compare
// against" dropdown, so the live tracker stamps the new ref onto its next
// emit without waiting for a session restart.
//
// Pass a nil or empty map to clear the override (every tracker falls back
// to origin/main → master). Best-effort: failures are returned but the DB
// write the caller already made is the source of truth — the next session
// launch rebuilds trackers from the persisted map.
func (c *Client) SetBaseBranches(ctx context.Context, branches map[string]string) error {
	body, err := json.Marshal(struct {
		BaseBranches map[string]string `json:"base_branches"`
	}{BaseBranches: branches})
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/workspace/base-branches", bytes.NewReader(body))
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
		return fmt.Errorf("set base branches failed with status %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}
