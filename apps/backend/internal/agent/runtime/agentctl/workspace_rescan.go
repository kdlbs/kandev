package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// RescanWorkspace asks the agentctl instance to re-discover repo subdirs
// and reconcile its tracker set. Called by the kandev backend's branch
// materializer after creating a sibling worktree (multi-branch add_branch
// flow); without it, the new worktree's git/file events stay invisible to
// the running session until restart.
//
// workDir is optional. Pass the task root (parent of repo dirs) when
// transitioning a single-branch task to multi-branch — the manager updates
// its tracking scope before scanning. Pass empty to rescan the current
// WorkDir in place.
//
// Best-effort: failures are returned but do not block the materializer.
// The worktree still exists on disk; the next session relaunch will
// rebuild trackers via prepareMultiRepo even when this call missed.
func (c *Client) RescanWorkspace(ctx context.Context, workDir string) error {
	body, err := json.Marshal(struct {
		WorkDir string `json:"work_dir"`
	}{WorkDir: workDir})
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/workspace/rescan", bytes.NewReader(body))
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
		return fmt.Errorf("rescan workspace failed with status %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}
