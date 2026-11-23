package service

import (
	"encoding/json"
	"github.com/kandev/kandev/internal/user/models"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestSidebarDraftDisclosureCannotRecreateSavedOwner(t *testing.T) {
	for _, existing := range []bool{false, true} {
		t.Run(map[bool]string{false: "saved-owner", true: "draft-owner"}[existing], func(t *testing.T) {
			state := models.SidebarWorkspaceState{ActiveViewID: "base", Views: []models.SidebarView{{ID: "base", Group: "plugin:tags:multi-tags", CollapsedGroups: []string{"kept", "retired"}}}}
			if existing {
				state.Draft = &models.SidebarViewDraft{BaseViewID: "base", Group: "plugin:tags:multi-tags", CollapsedGroups: []string{"kept", "retired"}}
			}
			var update models.SidebarDraftUpdate
			require.NoError(t, json.Unmarshal([]byte(`{"base_view_id":"base","expected_group":"plugin:tags:multi-tags","require_existing":true,"disclosure":{"group":"plugin:tags:multi-tags","remove":["retired"]}}`), &update))
			require.NoError(t, applySidebarDraftUpdate(&state, &update))
			require.Equal(t, []string{"kept", "retired"}, state.Views[0].CollapsedGroups)
			if existing {
				require.Equal(t, []string{"kept"}, state.Draft.CollapsedGroups)
			} else {
				require.Nil(t, state.Draft)
			}
		})
	}
}
