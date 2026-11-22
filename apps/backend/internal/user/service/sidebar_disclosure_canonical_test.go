package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSidebarDisclosureCanonicalizationPreservesFreshKeys(t *testing.T) {
	for _, preview := range []bool{false, true} {
		t.Run(map[bool]string{false: "saved", true: "draft"}[preview], func(t *testing.T) {
			svc, _, _ := sidebarService(t)
			keys := make([]string, 1002)
			for i := range keys {
				keys[i] = "kept"
			}
			keys = append(keys, "fresh-key")
			view := map[string]any{"id": "base", "name": "Saved", "filters": []any{},
				"sort":  map[string]string{"key": "state", "direction": "asc"},
				"group": "plugin:tags:tags", "collapsed_groups": keys}
			patch := map[string]any{"workspace_id": "a", "views_upsert": []any{view}, "active_view_id": "base"}
			if preview {
				view["group"], view["collapsed_groups"] = "state", []string{"source-key"}
				patch["draft"] = map[string]any{"base_view_id": "base", "filters": []any{},
					"sort":  map[string]string{"key": "state", "direction": "asc"},
					"group": "plugin:tags:tags", "collapsed_groups": keys}
			}
			body, err := json.Marshal(map[string]any{"SidebarViewState": patch})
			require.NoError(t, err)
			_, err = svc.UpdateUserSettings(context.Background(), sidebarPatch(t, string(body)))
			require.NoError(t, err)
			disclosure := map[string]any{"group": "plugin:tags:tags", "canonicalize": true}
			intent := map[string]any{"workspace_id": "a"}
			if preview {
				intent["draft_update"] = map[string]any{"base_view_id": "base", "expected_group": "plugin:tags:tags", "disclosure": disclosure}
			} else {
				intent["views_update"] = []any{map[string]any{"id": "base", "disclosure": disclosure}}
			}
			body, err = json.Marshal(map[string]any{"SidebarViewState": intent})
			require.NoError(t, err)
			_, err = svc.UpdateUserSettings(context.Background(), sidebarPatch(t, string(body)))
			require.NoError(t, err)
			reloaded, err := svc.GetUserSettings(context.Background())
			require.NoError(t, err)
			state := reloaded.SidebarViewsByWorkspace["a"]
			if preview {
				require.Equal(t, []string{"kept", "fresh-key"}, state.Draft.CollapsedGroups)
			} else {
				for _, saved := range state.Views {
					if saved.ID == "base" {
						require.Equal(t, []string{"kept", "fresh-key"}, saved.CollapsedGroups)
					}
				}
			}
		})
	}
}
