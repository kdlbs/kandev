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
