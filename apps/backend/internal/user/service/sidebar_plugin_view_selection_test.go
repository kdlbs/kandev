package service

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

func sidebarPluginCandidate(id string) map[string]any {
	return map[string]any{
		"id": id, "name": "Tag", "filters": []any{},
		"sort":  map[string]any{"key": "state", "direction": "asc"},
		"group": "plugin:tags:tags", "group_indent": true, "collapsed_groups": []string{},
		"generated_plugin_grouping": map[string]any{"reference": "plugin:tags:tags", "name": "Tag"},
	}
}

func selectSidebarPlugin(t *testing.T, svc *Service, id string) error {
	t.Helper()
	body, err := json.Marshal(map[string]any{"SidebarViewState": map[string]any{
		"workspace_id": "a", "select_plugin_grouping": sidebarPluginCandidate(id),
	}})
	require.NoError(t, err)
	_, err = svc.UpdateUserSettings(context.Background(), sidebarPatch(t, string(body)))
	return err
}

func TestSidebarPluginSelectionReusesLatestGeneratedView(t *testing.T) {
	svc, _, _ := sidebarService(t)
	require.NoError(t, selectSidebarPlugin(t, svc, "first"))
	_, err := svc.UpdateUserSettings(context.Background(), sidebarPatch(t, `{"SidebarViewState":{"workspace_id":"a","views_update":[{"id":"first","disclosure":{"group":"plugin:tags:tags","add":["kept"]}}]}}`))
	require.NoError(t, err)
	require.NoError(t, selectSidebarPlugin(t, svc, "stale-tab-candidate"))
	reloaded, err := svc.GetUserSettings(context.Background())
	require.NoError(t, err)
	state := reloaded.SidebarViewsByWorkspace["a"]
	require.Equal(t, "first", state.ActiveViewID)
	require.Len(t, state.Views, 2)
	require.Equal(t, []string{"kept"}, state.Views[1].CollapsedGroups)
}

func TestSidebarPluginSelectionPreservesCustomizedViews(t *testing.T) {
	for _, change := range []string{"manual", "renamed", "indent", "sort"} {
		t.Run(change, func(t *testing.T) {
			svc, _, _ := sidebarService(t)
			custom := sidebarPluginCandidate("custom")
			switch change {
			case "manual":
				delete(custom, "generated_plugin_grouping")
			case "renamed":
				custom["name"] = "My tags"
			case "indent":
				custom["group_indent"] = false
			case "sort":
				custom["sort"] = map[string]any{"key": "title", "direction": "desc"}
			}
			body, err := json.Marshal(map[string]any{"SidebarViewState": map[string]any{
				"workspace_id": "a", "views_upsert": []any{custom},
			}})
			require.NoError(t, err)
			_, err = svc.UpdateUserSettings(context.Background(), sidebarPatch(t, string(body)))
			require.NoError(t, err)
			require.NoError(t, selectSidebarPlugin(t, svc, "generated"))
			reloaded, err := svc.GetUserSettings(context.Background())
			require.NoError(t, err)
			state := reloaded.SidebarViewsByWorkspace["a"]
			require.Equal(t, "generated", state.ActiveViewID)
			require.Len(t, state.Views, 3)
			require.Equal(t, "custom", state.Views[1].ID)
			if change != "renamed" {
				require.Equal(t, "Tag 2", state.Views[2].Name)
			}
		})
	}
}

func TestSidebarPluginSelectionProtectsDraftAndViewLimit(t *testing.T) {
	t.Run("draft", func(t *testing.T) {
		svc, _, _ := sidebarService(t)
		require.NoError(t, selectSidebarPlugin(t, svc, "first"))
		_, err := svc.UpdateUserSettings(context.Background(), sidebarPatch(t, `{"SidebarViewState":{"workspace_id":"a","draft":{"base_view_id":"first","filters":[],"sort":{"key":"state","direction":"asc"},"group":"plugin:tags:tags","group_indent":true}}}`))
		require.NoError(t, err)
		require.ErrorIs(t, selectSidebarPlugin(t, svc, "second"), ErrValidation)
		reloaded, err := svc.GetUserSettings(context.Background())
		require.NoError(t, err)
		require.NotNil(t, reloaded.SidebarViewsByWorkspace["a"].Draft)
	})
	t.Run("limit allows reuse only", func(t *testing.T) {
		svc, _, _ := sidebarService(t)
		views := make([]any, maxSidebarViews)
		for i := range views {
			view := sidebarPluginCandidate("view-" + strconv.Itoa(i))
			if i != 0 {
				delete(view, "generated_plugin_grouping")
				view["name"] = "Custom " + strconv.Itoa(i)
			}
			views[i] = view
		}
		body, err := json.Marshal(map[string]any{"SidebarViewState": map[string]any{
			"workspace_id": "a", "views": views,
		}})
		require.NoError(t, err)
		_, err = svc.UpdateUserSettings(context.Background(), sidebarPatch(t, string(body)))
		require.NoError(t, err)
		require.NoError(t, selectSidebarPlugin(t, svc, "reuse"))
		reloaded, err := svc.GetUserSettings(context.Background())
		require.NoError(t, err)
		require.Equal(t, "view-0", reloaded.SidebarViewsByWorkspace["a"].ActiveViewID)
		other := sidebarPluginCandidate("overflow")
		other["group"] = "plugin:tags:other"
		other["generated_plugin_grouping"] = map[string]any{"reference": "plugin:tags:other", "name": "Tag"}
		body, err = json.Marshal(map[string]any{"SidebarViewState": map[string]any{
			"workspace_id": "a", "select_plugin_grouping": other,
		}})
		require.NoError(t, err)
		_, err = svc.UpdateUserSettings(context.Background(), sidebarPatch(t, string(body)))
		require.ErrorIs(t, err, ErrValidation)
	})
}
