package handlers

import (
	"context"
	"encoding/json"
	"testing"

	promptservice "github.com/kandev/kandev/internal/prompts/service"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

func TestSyncedSharedPromptIsReadOnlyToAgents(t *testing.T) {
	_, svc, dispatcher := newPromptWriteFixture(t)
	ctx := context.Background()
	_, err := svc.ApplySyncedPrompts(ctx, promptservice.SyncApplyInput{
		Desired: []promptservice.SyncedPromptFile{{Name: "locked", Content: "Repo", Path: "p/locked.md"}},
	})
	require.NoError(t, err)

	resp, err := dispatcher.Dispatch(ctx, makeWSMessage(t, ws.ActionMCPUpdateSharedPrompt, map[string]any{"name": "locked", "content": "Agent"}))
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeForbidden)
	saved, err := svc.GetPromptByName(ctx, "locked")
	require.NoError(t, err)
	require.Equal(t, "Repo", saved.Content)

	resp, err = dispatcher.Dispatch(ctx, makeWSMessage(t, ws.ActionMCPGetSharedPrompt, map[string]any{"name": "locked"}))
	require.NoError(t, err)
	var read sharedPromptRead
	require.NoError(t, json.Unmarshal(resp.Payload, &read))
	require.False(t, read.AllowAgentEdits)
	require.Equal(t, "sync", read.Source)
	require.Equal(t, "p/locked.md", read.SourcePath)

	resp, err = dispatcher.Dispatch(ctx, makeWSMessage(t, ws.ActionMCPListSharedPrompts, map[string]any{}))
	require.NoError(t, err)
	var list struct {
		SharedPrompts []sharedPromptSummary `json:"shared_prompts"`
	}
	require.NoError(t, json.Unmarshal(resp.Payload, &list))
	for _, summary := range list.SharedPrompts {
		if summary.Name == "locked" {
			require.False(t, summary.AllowAgentEdits)
			require.Equal(t, "sync", summary.Source)
			return
		}
	}
	t.Fatal("synced prompt missing from list")
}
