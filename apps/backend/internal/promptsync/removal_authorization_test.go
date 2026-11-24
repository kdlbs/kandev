package promptsync

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeleteConfigRemovesConfigurationWhenItsWorkspaceWasDeleted(t *testing.T) {
	f := newSyncFixture(t)
	ctx := context.Background()
	f.gitlab.files["hook.md"] = "Default"
	_, err := f.svc.Sync(ctx)
	require.NoError(t, err)
	f.svc.SetWorkspaceAuthorizer(func(context.Context, string) error { return errors.New("workspace not found") })
	f.svc.workspaceExists = func(context.Context, string) (bool, error) { return false, nil }

	require.NoError(t, f.svc.DeleteConfig(ctx, Removal{}))

	cfg, err := f.svc.GetConfig(ctx)
	require.NoError(t, err)
	assert.Nil(t, cfg)
	prompt, err := f.prompts.GetPromptByName(ctx, "hook")
	require.NoError(t, err)
	assert.False(t, prompt.IsSynced(), "the released prompt is manual again")
}

func TestDeleteConfigStillAuthorizesAnExistingWorkspace(t *testing.T) {
	f := newSyncFixture(t)
	ctx := context.Background()
	denied := errors.New("denied")
	f.svc.SetWorkspaceAuthorizer(func(context.Context, string) error { return denied })

	assert.ErrorIs(t, f.svc.DeleteConfig(ctx, Removal{}), denied)
	cfg, err := f.svc.GetConfig(ctx)
	require.NoError(t, err)
	assert.NotNil(t, cfg, "a denied removal keeps the configuration")
}
