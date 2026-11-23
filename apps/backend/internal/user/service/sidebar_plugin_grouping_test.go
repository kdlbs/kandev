package service

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/user/models"
)

// @covers AC-UI-SIDEBAR-PLUGIN-GROUPING-001.2
func TestSidebarPluginGroupingPreferencesRejectMalformedReferencesAtomically(t *testing.T) {
	for _, tc := range []struct {
		name, group, draft string
		origin             *models.SidebarGeneratedPluginGrouping
	}{
		{name: "saved view", group: "plugin:acme:"},
		{name: "generated identity", group: "plugin:acme:labels", origin: &models.SidebarGeneratedPluginGrouping{Reference: "facet:acme:labels", Name: "Labels"}},
		{name: "generated name", group: "plugin:acme:labels", origin: &models.SidebarGeneratedPluginGrouping{Reference: "plugin:acme:labels"}},
		{name: "draft", group: "plugin:acme:labels", draft: "plugin:acme:"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, repo, _ := sidebarService(t)
			if _, err := svc.GetUserSettings(t.Context()); err != nil {
				t.Fatal(err)
			}
			before := repo.snapshot().Revision
			views := []models.SidebarView{{ID: "grouped", Name: "Labels", Group: tc.group, GeneratedPluginGrouping: tc.origin}}
			active := "grouped"
			patch := &models.SidebarWorkspacePatch{WorkspaceID: "a", Views: &views, ActiveViewID: &active}
			if tc.draft != "" {
				body, err := json.Marshal(models.SidebarViewDraft{BaseViewID: active, Group: tc.draft})
				if err != nil {
					t.Fatal(err)
				}
				patch.Draft = body
			}
			_, err := svc.UpdateUserSettings(t.Context(), &UpdateUserSettingsRequest{SidebarViewState: patch})
			if !errors.Is(err, ErrValidation) || repo.snapshot().Revision != before {
				t.Fatalf("malformed plugin preference persisted: %v", err)
			}
		})
	}
}

func TestSidebarPluginGroupingUnavailableProviderPreferencesSurviveReload(t *testing.T) {
	svc, _, _ := sidebarService(t)
	views := []models.SidebarView{{ID: "grouped", Name: "My labels", Group: "plugin:missing:labels", CollapsedGroups: []string{"plugin:missing:labels:value:kept"}, GeneratedPluginGrouping: &models.SidebarGeneratedPluginGrouping{Reference: "plugin:missing:labels", Name: "Labels"}}}
	active := "grouped"
	_, err := svc.UpdateUserSettings(t.Context(), &UpdateUserSettingsRequest{SidebarViewState: &models.SidebarWorkspacePatch{WorkspaceID: "a", Views: &views, ActiveViewID: &active}})
	if err != nil {
		t.Fatal(err)
	}
	settings, err := svc.GetUserSettings(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	selected := settings.SidebarViewsByWorkspace["a"].Views[0]
	if selected.Group != views[0].Group || selected.Name != "My labels" || len(selected.CollapsedGroups) != 1 || selected.GeneratedPluginGrouping == nil {
		t.Fatalf("provider availability erased saved view state: %+v", selected)
	}
	if settings.SidebarViewsByWorkspace["b"].Views[0].Name != "Original" {
		t.Fatal("plugin grouping changed another workspace")
	}
}
