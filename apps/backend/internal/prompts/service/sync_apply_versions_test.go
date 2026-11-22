package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApplyAdoptsIdenticalPromptsWithoutConflict(t *testing.T) {
	svc, cleanup := createService(t)
	t.Cleanup(cleanup)
	ctx := context.Background()
	_, err := svc.CreatePrompt(ctx, "same", "Shared text")
	require.NoError(t, err)

	result := applySynced(t, svc, SyncApplyInput{Desired: []SyncedPromptFile{
		{Name: "same", Content: "Shared text", Path: "p/same.md"},
	}})
	assert.Empty(t, result.Conflicts, "identical text is not a conflict")
	same := promptNamed(t, svc, "same")
	assert.True(t, same.IsSynced())
	assert.Equal(t, "Shared text", same.Content)
}

func TestApplyResolvesConflictedOverrideThatMatchesTheFile(t *testing.T) {
	svc, cleanup := createService(t)
	t.Cleanup(cleanup)
	ctx := context.Background()
	_, err := svc.CreatePrompt(ctx, "drift", "Mine")
	require.NoError(t, err)
	applySynced(t, svc, SyncApplyInput{Desired: []SyncedPromptFile{{Name: "drift", Content: "Repo", Path: "p/drift.md"}}})
	require.True(t, promptNamed(t, svc, "drift").Conflict)

	result := applySynced(t, svc, SyncApplyInput{Desired: []SyncedPromptFile{{Name: "drift", Content: "Mine", Path: "p/drift.md"}}})
	assert.Empty(t, result.Conflicts)
	drift := promptNamed(t, svc, "drift")
	assert.True(t, drift.IsSynced(), "a file that now matches ends the conflict")
	assert.False(t, drift.Conflict)
}

func TestApplyRecordsRepositoryVersionOnOverrides(t *testing.T) {
	svc, cleanup := createService(t)
	t.Cleanup(cleanup)
	ctx := context.Background()
	applySynced(t, svc, SyncApplyInput{Desired: []SyncedPromptFile{{Name: "hook", Content: "v1", Path: "p/hook.md"}}})
	hook := promptNamed(t, svc, "hook")
	_, err := svc.SavePromptVersions(ctx, hook.ID, VersionSave{UseLocal: true})
	require.NoError(t, err)

	result := applySynced(t, svc, SyncApplyInput{Desired: []SyncedPromptFile{{Name: "hook", Content: "v2", Path: "p/hook.md"}}})
	assert.True(t, result.Changed(), "a new repository version is a change the prompt list must see")
	hook = promptNamed(t, svc, "hook")
	assert.Equal(t, "v1", hook.Content, "the override keeps its content")
	assert.Equal(t, "v2", hook.AlternateContent, "and records the latest repository version")

	applySynced(t, svc, SyncApplyInput{})
	hook = promptNamed(t, svc, "hook")
	assert.True(t, hook.IsOverride())
	assert.Empty(t, hook.AlternateContent, "a removed file leaves no repository version")
}

func TestDetachedOverrideClearsAndRestoresRepositoryPath(t *testing.T) {
	svc, cleanup := createService(t)
	t.Cleanup(cleanup)
	ctx := context.Background()
	_, err := svc.CreatePrompt(ctx, "hook", "Mine")
	require.NoError(t, err)
	applySynced(t, svc, SyncApplyInput{Desired: []SyncedPromptFile{{Name: "hook", Content: "Repo", Path: "p/hook.md"}}})
	hook := promptNamed(t, svc, "hook")
	require.True(t, hook.Conflict)
	id := hook.ID

	removed := applySynced(t, svc, SyncApplyInput{})
	assert.True(t, removed.Changed())
	hook = promptNamed(t, svc, "hook")
	assert.Equal(t, id, hook.ID)
	assert.True(t, hook.IsOverride())
	assert.Equal(t, "Mine", hook.Content)
	assert.Empty(t, hook.AlternateContent)
	assert.Empty(t, hook.SourcePath)
	assert.False(t, hook.Conflict)

	returned := applySynced(t, svc, SyncApplyInput{Desired: []SyncedPromptFile{{Name: "hook", Content: "Repo", Path: "p/restored/hook.md"}}})
	assert.True(t, returned.Changed())
	hook = promptNamed(t, svc, "hook")
	assert.Equal(t, "Mine", hook.Content)
	assert.Equal(t, "Repo", hook.AlternateContent)
	assert.Equal(t, "p/restored/hook.md", hook.SourcePath)

	events := subscribePromptEvents(t, svc)
	pathOnly := applySynced(t, svc, SyncApplyInput{Desired: []SyncedPromptFile{{Name: "hook", Content: "Repo", Path: "p/moved/hook.md"}}})
	assert.True(t, pathOnly.Changed(), "a path-only update is published")
	assert.Equal(t, "p/moved/hook.md", promptNamed(t, svc, "hook").SourcePath)
	assert.Len(t, *events, 1, "a path-only update publishes the prompt change")

	unchanged := applySynced(t, svc, SyncApplyInput{Desired: []SyncedPromptFile{{Name: "hook", Content: "Repo", Path: "p/moved/hook.md"}}})
	assert.False(t, unchanged.Changed(), "unchanged path and repository text write nothing")
}

func TestSyncPreservesDraftWhenDetachingLegacyOverrideWithOnlyStalePath(t *testing.T) {
	svc, database, cleanup := createServiceWithDB(t)
	t.Cleanup(cleanup)
	ctx := context.Background()

	_, err := svc.CreatePrompt(ctx, "hook", "Mine")
	require.NoError(t, err)
	applySynced(t, svc, SyncApplyInput{Desired: []SyncedPromptFile{{Name: "hook", Content: "Repo", Path: "p/hook.md"}}})
	hook := promptNamed(t, svc, "hook")
	_, err = svc.SavePromptVersions(ctx, hook.ID, VersionSave{UseLocal: true})
	require.NoError(t, err)

	applySynced(t, svc, SyncApplyInput{})
	detached := promptNamed(t, svc, "hook")
	require.True(t, detached.IsOverride())
	require.Empty(t, detached.AlternateContent)
	require.Empty(t, detached.SourcePath)
	draftVersion := detached.UpdatedAt

	_, err = database.ExecContext(ctx, `UPDATE custom_prompts SET source_path = ? WHERE id = ?`, "p/legacy.md", detached.ID)
	require.NoError(t, err)

	result := applySynced(t, svc, SyncApplyInput{})
	assert.False(t, result.Changed(), "a stale path hidden by the read projection is not a new detach")

	localDraft := "Local draft"
	saved, err := svc.SavePromptVersions(ctx, detached.ID, VersionSave{
		UseLocal: true, LocalContent: &localDraft, ExpectedUpdatedAt: &draftVersion,
	})
	require.NoError(t, err)
	assert.Equal(t, "Local draft", saved.Content)
	assert.True(t, saved.UpdatedAt.After(draftVersion))
}
