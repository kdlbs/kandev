package service

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/kandev/kandev/internal/user/models"
	"github.com/stretchr/testify/require"
)

func TestSidebarWorkspaceDeltasPreserveLargeIndependentCopies(t *testing.T) {
	svc, _, _ := sidebarService(t)
	ctx := context.Background()
	_, err := svc.GetUserSettings(ctx)
	require.NoError(t, err)
	keys := make([]string, 1000)
	for i := range keys {
		keys[i] = fmt.Sprintf("plugin-group:%064x", i)
	}
	write := func(view models.SidebarView) *models.UserSettings {
		raw, err := json.Marshal(map[string]any{"SidebarViewState": map[string]any{
			"workspace_id": "a", "views_upsert": []models.SidebarView{view}, "active_view_id": view.ID,
		}})
		require.NoError(t, err)
		require.Less(t, len(raw), 2*1024*1024)
		var req UpdateUserSettingsRequest
		require.NoError(t, json.Unmarshal(raw, &req))
		result, err := svc.UpdateUserSettings(ctx, &req)
		require.NoError(t, err)
		return result
	}
	for i := range 27 {
		id := fmt.Sprintf("copy-%d", i)
		result := write(models.SidebarView{ID: id, Name: id, Group: "plugin:tags:tags", CollapsedGroups: keys})
		require.Len(t, result.SidebarViewsByWorkspace["a"].Views, i+2)
	}
	result := write(models.SidebarView{ID: "copy-26", Name: "Changed", Group: "plugin:tags:tags", CollapsedGroups: keys[1:]})
	require.Equal(t, "copy-26", result.SidebarViewsByWorkspace["a"].ActiveViewID)
	reloaded, err := svc.GetUserSettings(ctx)
	require.NoError(t, err)
	for _, view := range reloaded.SidebarViewsByWorkspace["a"].Views {
		if view.ID == "copy-0" {
			require.Equal(t, keys, view.CollapsedGroups)
		}
		if view.ID == "copy-26" {
			require.Equal(t, keys[1:], view.CollapsedGroups)
			require.Equal(t, "Changed", view.Name)
		}
	}
	require.Equal(t, "Original", reloaded.SidebarViewsByWorkspace["b"].Views[0].Name)
}

func TestSidebarWorkspaceDeltaOrderAndDeletionAreAtomic(t *testing.T) {
	svc, repo, _ := sidebarService(t)
	ctx := context.Background()
	_, err := svc.GetUserSettings(ctx)
	require.NoError(t, err)
	apply := func(raw string) error { _, err := svc.UpdateUserSettings(ctx, sidebarPatch(t, raw)); return err }
	require.NoError(t, apply(`{"SidebarViewState":{"workspace_id":"a","views_upsert":[{"id":"new","name":"New"}],"view_order":["new","legacy"],"active_view_id":"new"}}`))
	require.Equal(t, "new", repo.snapshot().SidebarViewsByWorkspace["a"].Views[0].ID)
	revision := repo.snapshot().Revision
	require.ErrorIs(t, apply(`{"SidebarViewState":{"workspace_id":"a","view_order":["new"]}}`), ErrValidation)
	require.Equal(t, revision, repo.snapshot().Revision)
	require.NoError(t, apply(`{"SidebarViewState":{"workspace_id":"a","view_ids_delete":["new"],"active_view_id":"legacy"}}`))
	require.Len(t, repo.snapshot().SidebarViewsByWorkspace["a"].Views, 1)
	require.ErrorIs(t, apply(`{"SidebarViewState":{"workspace_id":"a","views":[],"views_upsert":[{"id":"x","name":"X"}]}}`), ErrValidation)
	require.ErrorIs(t, apply(`{"SidebarViewState":{"workspace_id":"a","views":[],"views_upsert":[]}}`), ErrValidation)
}
