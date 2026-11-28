package automation

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUpdateAutomationRejectsStaleResourceRevision(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()
	a := &Automation{WorkspaceID: "workspace-revision", Name: "Revision test"}
	require.NoError(t, store.CreateAutomation(ctx, a))

	stale, err := store.GetAutomation(ctx, a.ID)
	require.NoError(t, err)
	prompt := "newer prompt"
	require.NoError(t, store.UpdateAutomation(ctx, a.ID, &UpdateAutomationRequest{Prompt: &prompt}))

	name := "stale name"
	_, err = store.updateAutomationReturningFromSnapshot(ctx, a.ID, &UpdateAutomationRequest{Name: &name}, stale)
	require.ErrorIs(t, err, ErrAutomationSettingsConflict)

	saved, err := store.GetAutomation(ctx, a.ID)
	require.NoError(t, err)
	require.Equal(t, prompt, saved.Prompt)
	require.NotEqual(t, name, saved.Name)
}
