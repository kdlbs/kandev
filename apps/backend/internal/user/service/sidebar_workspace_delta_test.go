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

func TestSidebarWorkspaceUpdateDoesNotRecreateDeletedView(t *testing.T) {
	for _, deleted := range []bool{false, true} {
		t.Run(fmt.Sprintf("deleted=%t", deleted), func(t *testing.T) {
			svc, _, _ := sidebarService(t)
			ctx := context.Background()
			apply := func(raw string) {
				t.Helper()
				_, err := svc.UpdateUserSettings(ctx, sidebarPatch(t, raw))
				require.NoError(t, err)
			}
			apply(`{"SidebarViewState":{"workspace_id":"a","views_upsert":[{"id":"grouped","name":"Original","group":"plugin:tags:tags"}],"active_view_id":"grouped"}}`)
			if deleted {
				apply(`{"SidebarViewState":{"workspace_id":"a","view_ids_delete":["grouped"],"active_view_id":"legacy"}}`)
			}
			apply(`{"SidebarViewState":{"workspace_id":"a","views_update":[{"id":"grouped","name":"Edited","disclosure":{"group":"plugin:tags:tags","replace":["kept"]}}]}}`)
			reloaded, err := svc.GetUserSettings(ctx)
			require.NoError(t, err)
			state := reloaded.SidebarViewsByWorkspace["a"]
			if deleted {
				require.Len(t, state.Views, 1)
				require.Equal(t, "legacy", state.ActiveViewID)
			} else {
				require.Len(t, state.Views, 2)
				require.Equal(t, "Edited", state.Views[1].Name)
				require.Equal(t, []string{"kept"}, state.Views[1].CollapsedGroups)
			}
		})
	}
}

func TestSidebarWorkspaceDisclosureIntentPreservesConcurrentFields(t *testing.T) {
	for _, changedGroup := range []bool{false, true} {
		t.Run(fmt.Sprintf("group_changed=%t", changedGroup), func(t *testing.T) {
			svc, _, _ := sidebarService(t)
			ctx := context.Background()
			apply := func(raw string) {
				t.Helper()
				_, err := svc.UpdateUserSettings(ctx, sidebarPatch(t, raw))
				require.NoError(t, err)
			}
			apply(`{"SidebarViewState":{"workspace_id":"a","views_upsert":[{"id":"grouped","name":"Original","group":"plugin:tags:tags","collapsed_groups":["old"]}]}}`)
			local := `{"SidebarViewState":{"workspace_id":"a","views_update":[{"id":"grouped","disclosure":{"group":"plugin:tags:tags","add":["local-key"],"remove":["old"]}}]}}`
			group := "plugin:tags:tags"
			if changedGroup {
				group = "plugin:tags:multi-tags"
			}
			apply(fmt.Sprintf(`{"SidebarViewState":{"workspace_id":"a","views_upsert":[{"id":"grouped","name":"Remote name","group":%q,"collapsed_groups":["remote-key"]}]}}`, group))
			apply(local)
			reloaded, err := svc.GetUserSettings(ctx)
			require.NoError(t, err)
			view := reloaded.SidebarViewsByWorkspace["a"].Views[1]
			require.Equal(t, "Remote name", view.Name)
			require.Equal(t, group, view.Group)
			if changedGroup {
				require.Equal(t, []string{"remote-key"}, view.CollapsedGroups)
			} else {
				require.Equal(t, []string{"remote-key", "local-key"}, view.CollapsedGroups)
			}
		})
	}
}

func TestSidebarWorkspaceDraftIntentPreservesConcurrentDisclosure(t *testing.T) {
	for _, changedOwner := range []bool{false, true} {
		t.Run(fmt.Sprintf("owner_changed=%t", changedOwner), func(t *testing.T) {
			svc, _, _ := sidebarService(t)
			ctx := context.Background()
			apply := func(raw string) {
				t.Helper()
				_, err := svc.UpdateUserSettings(ctx, sidebarPatch(t, raw))
				require.NoError(t, err)
			}
			apply(`{"SidebarViewState":{"workspace_id":"a","views_upsert":[{"id":"grouped","name":"Tags","group":"plugin:tags:tags"}],"active_view_id":"grouped","draft":{"base_view_id":"grouped","group":"plugin:tags:multi-tags","group_indent":true,"collapsed_groups":["kept","retired"]}}}`)
			owner := "plugin:tags:multi-tags"
			if changedOwner {
				owner = "plugin:tags:tags"
			}
			apply(fmt.Sprintf(`{"SidebarViewState":{"workspace_id":"a","draft":{"base_view_id":"grouped","group":%q,"group_indent":true,"collapsed_groups":["kept"]}}}`, owner))
			apply(`{"SidebarViewState":{"workspace_id":"a","draft_update":{"base_view_id":"grouped","expected_group":"plugin:tags:multi-tags","group_indent":false}}}`)
			reloaded, err := svc.GetUserSettings(ctx)
			require.NoError(t, err)
			draft := reloaded.SidebarViewsByWorkspace["a"].Draft
			require.NotNil(t, draft)
			require.Equal(t, owner, draft.Group)
			require.Equal(t, changedOwner, *draft.GroupIndent)
			require.Equal(t, []string{"kept"}, draft.CollapsedGroups)
		})
	}
}

func TestSidebarWorkspaceCreateIntentNeverReplacesAnExistingView(t *testing.T) {
	for _, exists := range []bool{false, true} {
		t.Run(fmt.Sprintf("exists=%t", exists), func(t *testing.T) {
			svc, _, _ := sidebarService(t)
			ctx := context.Background()
			_, err := svc.GetUserSettings(ctx)
			require.NoError(t, err)
			if exists {
				_, err = svc.UpdateUserSettings(ctx, sidebarPatch(t, `{"SidebarViewState":{"workspace_id":"a","views_upsert":[{"id":"created","name":"Remote rename"}]}}`))
				require.NoError(t, err)
			}
			_, err = svc.UpdateUserSettings(ctx, sidebarPatch(t, `{"SidebarViewState":{"workspace_id":"a","views_create":[{"id":"created","name":"Local creation"}]}}`))
			require.NoError(t, err)
			reloaded, err := svc.GetUserSettings(ctx)
			require.NoError(t, err)
			var saved models.SidebarView
			for _, view := range reloaded.SidebarViewsByWorkspace["a"].Views {
				if view.ID == "created" {
					saved = view
				}
			}
			want := "Local creation"
			if exists {
				want = "Remote rename"
			}
			require.Equal(t, want, saved.Name)
		})
	}
}

func TestSidebarWorkspaceCreateReplayAndSuccessorAreIndependent(t *testing.T) {
	svc, _, _ := sidebarService(t)
	ctx := context.Background()
	_, err := svc.UpdateUserSettings(ctx, sidebarPatch(t, `{"SidebarViewState":{"workspace_id":"a","views_upsert":[{"id":"created","name":"Remote rename","group":"plugin:tags:tags"}]}}`))
	require.NoError(t, err)
	_, err = svc.UpdateUserSettings(ctx, sidebarPatch(t, `{"SidebarViewState":{"workspace_id":"a","views_create":[{"id":"created","name":"Original creation","group":"plugin:tags:tags"}],"views_update":[{"id":"created","disclosure":{"group":"plugin:tags:tags","add":["kept"]}}]}}`))
	require.NoError(t, err)
	reloaded, err := svc.GetUserSettings(ctx)
	require.NoError(t, err)
	view := reloaded.SidebarViewsByWorkspace["a"].Views[1]
	require.Equal(t, "Remote rename", view.Name)
	require.Equal(t, []string{"kept"}, view.CollapsedGroups)
}

func TestSidebarWorkspaceReorderRejectsChangedObservedOrder(t *testing.T) {
	svc, _, _ := sidebarService(t)
	ctx := context.Background()
	apply := func(raw string) error {
		_, err := svc.UpdateUserSettings(ctx, sidebarPatch(t, raw))
		return err
	}
	require.NoError(t, apply(`{"SidebarViewState":{"workspace_id":"a","views_upsert":[{"id":"b","name":"B"},{"id":"c","name":"C"},{"id":"d","name":"D"}]}}`))
	require.NoError(t, apply(`{"SidebarViewState":{"workspace_id":"a","view_order":["legacy","b","d","c"]}}`))
	err := apply(`{"SidebarViewState":{"workspace_id":"a","view_order_base":["legacy","b","c","d"],"view_order":["b","legacy","c","d"]}}`)
	require.ErrorIs(t, err, ErrValidation)
	reloaded, err := svc.GetUserSettings(ctx)
	require.NoError(t, err)
	ids := make([]string, 0, 4)
	for _, view := range reloaded.SidebarViewsByWorkspace["a"].Views {
		ids = append(ids, view.ID)
	}
	require.Equal(t, []string{"legacy", "b", "d", "c"}, ids)
	require.NoError(t, apply(`{"SidebarViewState":{"workspace_id":"a","view_order_base":["legacy","b","d","c"],"view_order":["b","legacy","d","c"]}}`))
	require.NoError(t, apply(`{"SidebarViewState":{"workspace_id":"a","view_order_base":["legacy","b","d","c"],"view_order":["b","legacy","d","c"]}}`))
	reloaded, err = svc.GetUserSettings(ctx)
	require.NoError(t, err)
	ids = ids[:0]
	for _, view := range reloaded.SidebarViewsByWorkspace["a"].Views {
		ids = append(ids, view.ID)
	}
	require.Equal(t, []string{"b", "legacy", "d", "c"}, ids)
	require.NoError(t, apply(`{"SidebarViewState":{"workspace_id":"a","view_order_base":["legacy","b","d","c"],"view_order":["b","legacy","d","c"],"views_update":[{"id":"b","name":"Independent successor"}]}}`))
	reloaded, err = svc.GetUserSettings(ctx)
	require.NoError(t, err)
	require.Equal(t, "Independent successor", reloaded.SidebarViewsByWorkspace["a"].Views[0].Name)
}

func TestSidebarWorkspaceCommittedMoveCollectionSuccessor(t *testing.T) {
	for _, tc := range []struct {
		name  string
		patch string
		want  []string
	}{
		{"create", `{"view_order_base":["legacy","b","c","d"],"view_order":["b","legacy","c","d","e"],"views_create":[{"id":"e","name":"E"}]}`, []string{"b", "legacy", "c", "d", "e"}},
		{"delete", `{"view_order_base":["legacy","b","c","d"],"view_order":["b","legacy","c"],"view_ids_delete":["d"]}`, []string{"b", "legacy", "c"}},
		{"conflict", `{"view_order_base":["legacy","b","c","d"],"view_order":["legacy","c","b","d","e"],"views_create":[{"id":"e","name":"E"}]}`, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, _, _ := sidebarService(t)
			ctx := context.Background()
			_, err := svc.UpdateUserSettings(ctx, sidebarPatch(t, `{"SidebarViewState":{"workspace_id":"a","views_create":[{"id":"b","name":"B"},{"id":"c","name":"C"},{"id":"d","name":"D"}],"view_order":["b","legacy","c","d"]}}`))
			require.NoError(t, err)
			before, err := svc.GetUserSettings(ctx)
			require.NoError(t, err)
			raw := `{"SidebarViewState":{"workspace_id":"a",` + tc.patch[1:] + `}`
			_, err = svc.UpdateUserSettings(ctx, sidebarPatch(t, raw))
			if tc.want == nil {
				require.ErrorIs(t, err, ErrValidation)
				after, readErr := svc.GetUserSettings(ctx)
				require.NoError(t, readErr)
				require.Equal(t, before, after)
				return
			}
			require.NoError(t, err)
			after, err := svc.GetUserSettings(ctx)
			require.NoError(t, err)
			var ids []string
			for _, view := range after.SidebarViewsByWorkspace["a"].Views {
				ids = append(ids, view.ID)
			}
			require.Equal(t, tc.want, ids)
		})
	}
}
