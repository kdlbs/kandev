package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/prompts/dto"
	promptstore "github.com/kandev/kandev/internal/prompts/store"
)

func TestSavePromptVersionsSwitchesLosslessly(t *testing.T) {
	svc, cleanup := createService(t)
	t.Cleanup(cleanup)
	ctx := context.Background()
	applySynced(t, svc, SyncApplyInput{Desired: []SyncedPromptFile{{Name: "hook", Content: "Default", Path: "p/hook.md"}}})
	prompt := promptNamed(t, svc, "hook")
	events := subscribePromptEvents(t, svc)

	overridden, err := svc.SavePromptVersions(ctx, prompt.ID, VersionSave{UseLocal: true})
	require.NoError(t, err)
	assert.True(t, overridden.IsOverride())
	assert.Equal(t, "Default", overridden.Content, "without a local version the override starts from the synced content")
	assert.Len(t, *events, 1)
	assert.True(t, dto.FromPrompt(overridden).AllowAgentEdits)
	assert.Equal(t, "override", dto.FromPrompt(overridden).Source)

	edited := "  Mine edited  "
	reverted, err := svc.SavePromptVersions(ctx, prompt.ID, VersionSave{LocalContent: &edited})
	require.NoError(t, err)
	assert.True(t, reverted.IsSynced(), "saving with the switch off keeps an edit made with it on")
	assert.Equal(t, "Default", reverted.Content)
	assert.Equal(t, "Mine edited", reverted.AlternateContent)

	restored, err := svc.SavePromptVersions(ctx, prompt.ID, VersionSave{UseLocal: true, ExpectedUpdatedAt: &reverted.UpdatedAt})
	require.NoError(t, err)
	assert.Equal(t, "Mine edited", restored.Content)
	assert.Equal(t, "Default", restored.AlternateContent)
	assert.Equal(t, "Default", dto.FromPrompt(restored).AlternateContent, "the editor previews the other version")
}

func TestSavePromptVersionsRejectsInvalidSaves(t *testing.T) {
	svc, cleanup := createService(t)
	t.Cleanup(cleanup)
	ctx := context.Background()
	applySynced(t, svc, SyncApplyInput{Desired: []SyncedPromptFile{{Name: "hook", Content: "Default", Path: "p/hook.md"}}})
	prompt := promptNamed(t, svc, "hook")

	stale := prompt.UpdatedAt.Add(-time.Minute)
	_, err := svc.SavePromptVersions(ctx, prompt.ID, VersionSave{UseLocal: true, ExpectedUpdatedAt: &stale})
	assert.ErrorIs(t, err, promptstore.ErrPromptWriteRejected)
	blank := "   "
	_, err = svc.SavePromptVersions(ctx, prompt.ID, VersionSave{UseLocal: true, LocalContent: &blank})
	assert.ErrorIs(t, err, ErrInvalidPrompt)

	manual, err := svc.CreatePrompt(ctx, "manual", "m")
	require.NoError(t, err)
	_, err = svc.SavePromptVersions(ctx, manual.ID, VersionSave{UseLocal: true})
	assert.ErrorIs(t, err, ErrPromptNotLinked)
	_, err = svc.SavePromptVersions(ctx, "missing", VersionSave{})
	assert.ErrorIs(t, err, ErrPromptNotFound)
	assert.True(t, promptNamed(t, svc, "hook").IsSynced(), "a rejected save changes nothing")
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
		local := "Local " + name
		_, err = svc.SavePromptVersions(ctx, prompt.ID, VersionSave{UseLocal: true, LocalContent: &local})
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
