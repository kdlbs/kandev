package service

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSidebarDraftSaveTransfersLatestOwnedDisclosure(t *testing.T) {
	for _, sameGroup := range []bool{false, true} {
		for _, copyView := range []bool{false, true} {
			t.Run(fmt.Sprintf("same_group=%t/copy=%t", sameGroup, copyView), func(t *testing.T) {
				svc, _, _ := sidebarService(t)
				group := "plugin:tags:multi-tags"
				if sameGroup {
					group = "plugin:tags:tags"
				}
				_, err := svc.UpdateUserSettings(context.Background(), sidebarPatch(t, fmt.Sprintf(`{"SidebarViewState":{"workspace_id":"a","views_upsert":[{"id":"base","name":"Saved Tag","filters":[],"sort":{"key":"state","direction":"asc"},"group":"plugin:tags:tags","collapsed_groups":["kept"]}],"active_view_id":"base","draft":{"base_view_id":"base","filters":[],"sort":{"key":"title","direction":"asc"},"group":%q,"group_indent":false,"collapsed_groups":["kept"]}}}`, group)))
				require.NoError(t, err)
				if sameGroup {
					_, err = svc.UpdateUserSettings(context.Background(), sidebarPatch(t, fmt.Sprintf(`{"SidebarViewState":{"workspace_id":"a","views_update":[{"id":"base","disclosure":{"group":%q,"add":["newly-collapsed"]}}]}}`, group)))
				} else {
					_, err = svc.UpdateUserSettings(context.Background(), sidebarPatch(t, fmt.Sprintf(`{"SidebarViewState":{"workspace_id":"a","draft_update":{"base_view_id":"base","expected_group":%q,"disclosure":{"group":%q,"add":["newly-collapsed"]}}}}`, group, group)))
				}
				require.NoError(t, err)
				target := "base"
				if copyView {
					target = "copy"
				}
				_, err = svc.UpdateUserSettings(context.Background(), sidebarPatch(t, fmt.Sprintf(`{"SidebarViewState":{"workspace_id":"a","save_draft":{"base_view_id":"base","expected_group":%q,"target_view_id":%q,"name":"Saved copy"}}}`, group, target)))
				require.NoError(t, err)
				reloaded, err := svc.GetUserSettings(context.Background())
				require.NoError(t, err)
				state := reloaded.SidebarViewsByWorkspace["a"]
				require.Equal(t, target, state.ActiveViewID)
				require.Nil(t, state.Draft)
				for _, view := range state.Views {
					if view.ID == target {
						require.Equal(t, group, view.Group)
						require.Equal(t, []string{"kept", "newly-collapsed"}, view.CollapsedGroups)
						require.Equal(t, "title", view.Sort.Key)
						require.NotNil(t, view.GroupIndent)
						require.False(t, *view.GroupIndent)
					}
				}
				if copyView {
					_, err = svc.UpdateUserSettings(context.Background(), sidebarPatch(t, fmt.Sprintf(`{"SidebarViewState":{"workspace_id":"a","views_update":[{"id":"copy","disclosure":{"group":%q,"remove":["kept"]}}]}}`, group)))
					require.NoError(t, err)
					reloaded, err = svc.GetUserSettings(context.Background())
					require.NoError(t, err)
					for _, view := range reloaded.SidebarViewsByWorkspace["a"].Views {
						if view.ID != "base" {
							continue
						}
						require.Equal(t, "plugin:tags:tags", view.Group)
						if sameGroup {
							require.Equal(t, []string{"kept", "newly-collapsed"}, view.CollapsedGroups)
						} else {
							require.Equal(t, []string{"kept"}, view.CollapsedGroups)
						}
					}
				}
			})
		}
	}
}

func TestSidebarDraftSaveRejectsAChangedOwnerWithoutMutation(t *testing.T) {
	svc, _, _ := sidebarService(t)
	_, err := svc.UpdateUserSettings(context.Background(), sidebarPatch(t, `{"SidebarViewState":{"workspace_id":"a","views_upsert":[{"id":"base","name":"Saved Tag","group":"plugin:tags:tags","collapsed_groups":["base-kept"]}],"active_view_id":"base","draft":{"base_view_id":"base","filters":[],"sort":{"key":"state","direction":"asc"},"group":"plugin:tags:new-owner","collapsed_groups":["remote-kept"]}}}`))
	require.NoError(t, err)
	_, err = svc.UpdateUserSettings(context.Background(), sidebarPatch(t, `{"SidebarViewState":{"workspace_id":"a","save_draft":{"base_view_id":"base","expected_group":"plugin:tags:multi-tags","target_view_id":"copy","name":"Copy"}}}`))
	require.ErrorIs(t, err, ErrValidation)
	reloaded, err := svc.GetUserSettings(context.Background())
	require.NoError(t, err)
	state := reloaded.SidebarViewsByWorkspace["a"]
	require.Equal(t, "base", state.ActiveViewID)
	require.Equal(t, "plugin:tags:new-owner", state.Draft.Group)
	require.Equal(t, []string{"remote-kept"}, state.Draft.CollapsedGroups)
	for _, view := range state.Views {
		require.NotEqual(t, "copy", view.ID)
	}
}
