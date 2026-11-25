package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSavePromptVersionsRejectsRepositoryWhenTheFileIsGone(t *testing.T) {
	svc, cleanup := createService(t)
	t.Cleanup(cleanup)
	ctx := context.Background()
	applySynced(t, svc, SyncApplyInput{Desired: []SyncedPromptFile{{Name: "hook", Content: "Default", Path: "p/hook.md"}}})
	prompt := promptNamed(t, svc, "hook")
	mine := "Mine"
	_, err := svc.SavePromptVersions(ctx, prompt.ID, VersionSave{UseLocal: true, LocalContent: &mine})
	require.NoError(t, err)
	applySynced(t, svc, SyncApplyInput{})
	detached := promptNamed(t, svc, "hook")
	require.True(t, detached.IsOverride())
	require.Empty(t, detached.AlternateContent)

	_, err = svc.SavePromptVersions(ctx, prompt.ID, VersionSave{})
	assert.ErrorIs(t, err, ErrNoRepositoryVersion)
	after := promptNamed(t, svc, "hook")
	assert.True(t, after.IsOverride(), "the only copy stays a local override")
	assert.Equal(t, "Mine", after.Content)

	edited := "Mine edited"
	saved, err := svc.SavePromptVersions(ctx, prompt.ID, VersionSave{UseLocal: true, LocalContent: &edited})
	require.NoError(t, err)
	assert.Equal(t, "Mine edited", saved.Content, "the local version stays editable")
}
