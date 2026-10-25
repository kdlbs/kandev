package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// SetWorkspacePollMode tells the agentctl instance how aggressively to poll
// the workspace for git/file changes. The gateway calls this when a session's
// UI subscription/focus state changes — see lifecycle/manager_subscription.go.
//
// Best-effort: failures are returned but do not block the gateway. Callers
// should log and move on; the agentctl will still be polling in its previous
// mode (or its default).
func (c *Client) SetWorkspacePollMode(ctx context.Context, mode string) error {
	body, err := json.Marshal(struct {
		Mode string `json:"mode"`
	}{Mode: mode})
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/workspace/poll-mode", bytes.NewReader(body))
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
		return fmt.Errorf("set workspace poll mode failed with status %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

// RefreshWorkspace requests one file and Git scan from agentctl. The
// turn_complete trigger is used by lifecycle completion; manual_refresh is
// the default explicit retry from a visible workspace surface.
func (c *Client) RefreshWorkspace(ctx context.Context, trigger string) error {
	if trigger == "" {
		trigger = "manual_refresh"
	}
	return c.updateWorkspace(ctx, "/api/v1/workspace/refresh", "refresh", struct {
		Trigger string `json:"trigger"`
	}{Trigger: trigger})
}
