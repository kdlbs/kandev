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
func (c *Client) RescanWorkspace(ctx context.Context, workDir string, sourceRoots ...[]string) error {
	return c.updateWorkspace(ctx, "/api/v1/workspace/rescan", "rescan", struct {
		WorkDir              string   `json:"work_dir"`
		WorkspaceSourceRoots []string `json:"workspace_source_roots,omitempty"`
	}{WorkDir: workDir, WorkspaceSourceRoots: firstSourceRoots(sourceRoots)})
}

// ReconcileWorkspace prunes tracker state against the current root after a
// rollback has removed newly-created workspace checkouts. It is intentionally
// distinct from RescanWorkspace, whose empty-root form only appends trackers.
func (c *Client) ReconcileWorkspace(ctx context.Context, sourceRoots ...[]string) error {
	return c.updateWorkspace(ctx, "/api/v1/workspace/reconcile", "reconcile", struct {
		WorkspaceSourceRoots []string `json:"workspace_source_roots,omitempty"`
	}{WorkspaceSourceRoots: firstSourceRoots(sourceRoots)})
}

// RebindWorkspace replaces agentctl's workspace root and every tracker after
// lifecycle has stopped the native child. It is deliberately not a best-effort
// rescan: callers must treat a failure as an adoption failure and roll back.
func (c *Client) RebindWorkspace(ctx context.Context, workDir string, sourceRoots ...[]string) error {
	return c.updateWorkspace(ctx, "/api/v1/workspace/rebind", "rebind", struct {
		WorkDir              string   `json:"work_dir"`
		WorkspaceSourceRoots []string `json:"workspace_source_roots,omitempty"`
	}{WorkDir: workDir, WorkspaceSourceRoots: firstSourceRoots(sourceRoots)})
}

func (c *Client) updateWorkspace(ctx context.Context, path, operation string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		body, _ := readResponseBody(resp)
		return fmt.Errorf("%s workspace failed with status %d: %s", operation, resp.StatusCode, string(body))
	}
	return nil
}

func firstSourceRoots(sourceRoots [][]string) []string {
	if len(sourceRoots) == 0 {
		return nil
	}
	return sourceRoots[0]
}
