package handlers

import (
	"context"
	"encoding/json"
	"testing"

	promptservice "github.com/kandev/kandev/internal/prompts/service"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/auth/authn"
)

// @covers AC-INTEGRATIONS-PROMPT-SYNC-010.1
func TestSyncedPromptWriteTellsAgentHowTheUserCanOverride(t *testing.T) {
	_, svc, dispatcher := newPromptWriteFixture(t)
	ctx := authn.WithIdentity(context.Background(), authn.Identity{Synthetic: true})
	_, err := svc.ApplySyncedPrompts(ctx, promptservice.SyncApplyInput{
		Desired: []promptservice.SyncedPromptFile{{Name: "locked", Content: "Repo", Path: "p/locked.md"}},
	})
	require.NoError(t, err)

	resp, err := dispatcher.Dispatch(ctx, makeWSMessage(t, ws.ActionMCPUpdateSharedPrompt, map[string]any{"name": "locked", "content": "Agent"}))
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeForbidden)
	var payload struct {
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(resp.Payload, &payload))
	for _, part := range []string{"Prompt Sync", "no effect", "switch it to Local", "Settings > Prompts", "Agents cannot"} {
		assert.Contains(t, payload.Message, part)
	}
}

// @covers AC-INTEGRATIONS-PROMPT-SYNC-010.2
// @covers AC-INTEGRATIONS-PROMPT-SYNC-010.4
func TestMCPPromptReadsReportConflictAndGuidance(t *testing.T) {
	_, svc, dispatcher := newPromptWriteFixture(t)
	ctx := authn.WithIdentity(context.Background(), authn.Identity{Synthetic: true})
	_, err := svc.CreatePrompt(ctx, "mine", "Local")
	require.NoError(t, err)
	_, err = svc.CreatePrompt(ctx, "plain", "Plain")
	require.NoError(t, err)
	_, err = svc.ApplySyncedPrompts(ctx, promptservice.SyncApplyInput{Desired: []promptservice.SyncedPromptFile{
		{Name: "mine", Content: "Repo", Path: "p/mine.md"},
		{Name: "locked", Content: "Repo", Path: "p/locked.md"},
	}})
	require.NoError(t, err)

	read := func(name string) sharedPromptRead {
		resp, err := dispatcher.Dispatch(ctx, makeWSMessage(t, ws.ActionMCPGetSharedPrompt, map[string]any{"name": name}))
		require.NoError(t, err)
		var result sharedPromptRead
		require.NoError(t, json.Unmarshal(resp.Payload, &result))
		return result
	}
	mine := read("mine")
	assert.Equal(t, "override", mine.Source)
	assert.True(t, mine.Conflict)
	assert.NotEmpty(t, mine.EditGuidance)
	assert.NotEmpty(t, read("locked").EditGuidance)
	assert.Empty(t, read("plain").EditGuidance)

	resp, err := dispatcher.Dispatch(ctx, makeWSMessage(t, ws.ActionMCPListSharedPrompts, map[string]any{}))
	require.NoError(t, err)
	var list struct {
		SharedPrompts []sharedPromptSummary `json:"shared_prompts"`
	}
	require.NoError(t, json.Unmarshal(resp.Payload, &list))
	found := false
	for _, summary := range list.SharedPrompts {
		if summary.Name == "mine" {
			found = true
			assert.True(t, summary.Conflict)
			assert.NotEmpty(t, summary.EditGuidance)
		}
	}
	require.True(t, found)

	resp, err = dispatcher.Dispatch(ctx, makeWSMessage(t, ws.ActionMCPUpdateSharedPrompt, map[string]any{"name": "mine", "content": "Agent edit"}))
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, resp.Type, string(resp.Payload))
	saved, err := svc.GetPromptByName(ctx, "mine")
	require.NoError(t, err)
	assert.Equal(t, "Agent edit", saved.Content)
	assert.True(t, saved.Conflict, "an agent edit changes the local version and leaves the conflict")
}
