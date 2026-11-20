package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/prompts/dto"
)

func TestOverrideAndRevertSyncedPrompt(t *testing.T) {
	svc, cleanup := createService(t)
	t.Cleanup(cleanup)
	ctx := context.Background()
	applySynced(t, svc, SyncApplyInput{Desired: []SyncedPromptFile{{Name: "hook", Content: "Default", Path: "p/hook.md"}}})
	prompt, err := svc.GetPromptByName(ctx, "hook")
	require.NoError(t, err)
	events := subscribePromptEvents(t, svc)

	overridden, err := svc.OverrideSyncedPrompt(ctx, prompt.ID)
	require.NoError(t, err)
	assert.True(t, overridden.IsOverride())
	assert.Len(t, *events, 1)
	assert.True(t, dto.FromPrompt(overridden).AllowAgentEdits)
	assert.Equal(t, "override", dto.FromPrompt(overridden).Source)

	mine := "Mine"
	_, err = svc.UpdatePrompt(ctx, prompt.ID, nil, &mine)
	require.NoError(t, err)

	_, err = svc.OverrideSyncedPrompt(ctx, prompt.ID)
	assert.ErrorIs(t, err, ErrPromptNotSynced)

	reverted, err := svc.RevertOverride(ctx, prompt.ID)
	require.NoError(t, err)
	assert.True(t, reverted.IsSynced())
	_, err = svc.RevertOverride(ctx, prompt.ID)
	assert.ErrorIs(t, err, ErrPromptNotOverride)

	manual, err := svc.CreatePrompt(ctx, "manual", "m")
	require.NoError(t, err)
	_, err = svc.OverrideSyncedPrompt(ctx, manual.ID)
	assert.ErrorIs(t, err, ErrPromptNotSynced)
	_, err = svc.RevertOverride(ctx, manual.ID)
	assert.ErrorIs(t, err, ErrPromptNotOverride)
}

func TestApplySyncedPromptsLeavesOverridesAlone(t *testing.T) {
	svc, cleanup := createService(t)
	t.Cleanup(cleanup)
	ctx := context.Background()
	applySynced(t, svc, SyncApplyInput{Desired: []SyncedPromptFile{
		{Name: "hook", Content: "Default", Path: "p/hook.md"},
		{Name: "kept-local", Content: "Default", Path: "p/kept-local.md"},
	}})
	for _, name := range []string{"hook", "kept-local"} {
		prompt, err := svc.GetPromptByName(ctx, name)
		require.NoError(t, err)
		_, err = svc.OverrideSyncedPrompt(ctx, prompt.ID)
		require.NoError(t, err)
		local := "Local " + name
		_, err = svc.UpdatePrompt(ctx, prompt.ID, nil, &local)
		require.NoError(t, err)
	}

	result := applySynced(t, svc, SyncApplyInput{Desired: []SyncedPromptFile{
		{Name: "hook", Content: "Default v2", Path: "p/hook.md"},
	}})
	assert.Equal(t, []string{"hook"}, result.Overridden)
	assert.Empty(t, result.Warnings, "an override is not a warning")
	assert.Empty(t, result.Deleted, "a vanished file never deletes an override")
	hook, err := svc.GetPromptByName(ctx, "hook")
	require.NoError(t, err)
	assert.Equal(t, "Local hook", hook.Content)
	_, err = svc.GetPromptByName(ctx, "kept-local")
	require.NoError(t, err)

	require.NoError(t, svc.DeletePrompt(ctx, hook.ID))
	result = applySynced(t, svc, SyncApplyInput{Desired: []SyncedPromptFile{
		{Name: "hook", Content: "Default v2", Path: "p/hook.md"},
	}})
	assert.Equal(t, []string{"hook"}, result.Created, "deleting an override restores the synced prompt")
}
