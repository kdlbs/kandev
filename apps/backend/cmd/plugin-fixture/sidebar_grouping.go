package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/kdlbs/kandev/pluginsdk"
)

func (p *fixturePlugin) sidebarGroupingAction(ctx context.Context, req *pluginsdk.PluginActionRequest) (*pluginsdk.PluginActionResponse, error) {
	host := p.Host()
	if host == nil || req.Context.WorkspaceID == "" {
		return nil, fmt.Errorf("plugin-fixture: grouping host context unavailable")
	}
	const key = "sidebar-grouping"
	if req.ActionKey == "sidebar-labels-write" {
		var state map[string]any
		if err := json.Unmarshal(req.Body, &state); err != nil {
			return nil, err
		}
		if err := host.SetState(ctx, "workspace", req.Context.WorkspaceID, key, state); err != nil {
			return nil, err
		}
		return &pluginsdk.PluginActionResponse{Body: []byte(`{"stored":true}`)}, nil
	}
	state, found, err := host.GetState(ctx, "workspace", req.Context.WorkspaceID, key)
	if err != nil {
		return nil, err
	}
	if !found {
		return &pluginsdk.PluginActionResponse{Body: []byte(`{"version":1,"revision":"empty","complete":true,"groups":[],"memberships":[]}`)}, nil
	}
	if delay, ok := state["delay_ms"].(float64); ok && delay > 0 && delay <= 60000 {
		timer := time.NewTimer(time.Duration(delay) * time.Millisecond)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
	if failed, _ := state["fail"].(bool); failed {
		return nil, fmt.Errorf("plugin-fixture: grouping unavailable")
	}
	return sidebarGroupingSnapshotResponse(state)
}

func sidebarGroupingSnapshotResponse(state map[string]any) (*pluginsdk.PluginActionResponse, error) {
	if raw, ok := state["snapshot_raw"].(string); ok {
		return &pluginsdk.PluginActionResponse{Body: []byte(raw)}, nil
	}
	body, err := json.Marshal(state["snapshot"])
	if err != nil {
		return nil, err
	}
	return &pluginsdk.PluginActionResponse{Body: body}, nil
}
