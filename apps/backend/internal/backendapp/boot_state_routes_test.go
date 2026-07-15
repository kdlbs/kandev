package backendapp

import (
	"testing"

	userdto "github.com/kandev/kandev/internal/user/dto"
	usermodels "github.com/kandev/kandev/internal/user/models"
)

func TestMapUserSettingsStateIncludesPortableTaskAndSidebarSettings(t *testing.T) {
	state := mapUserSettingsState(userdto.UserSettingsResponse{
		Settings: userdto.UserSettingsDTO{
			SidebarViews: []usermodels.SidebarView{{
				ID:   "view-1",
				Name: "My view",
			}},
			SidebarActiveViewID: "view-1",
			SidebarDraft: &usermodels.SidebarViewDraft{
				BaseViewID: "view-1",
				Group:      "repository",
			},
			SidebarTaskPrefs: usermodels.SidebarTaskPrefs{
				PinnedTaskIDs:          []string{"task-1"},
				OrderedTaskIDs:         []string{"task-2"},
				SubtaskOrderByParentID: map[string][]string{"task-1": {"task-3"}},
			},
			TaskCreateLastUsed: usermodels.TaskCreateLastUsed{
				RepositoryID:      "repo-1",
				Branch:            "main",
				AgentProfileID:    "agent-1",
				ExecutorProfileID: "executor-1",
			},
		},
	}, "workspace-1")

	if state["sidebarActiveViewId"] != "view-1" {
		t.Fatalf("sidebarActiveViewId = %#v, want view-1", state["sidebarActiveViewId"])
	}
	draft, ok := state["sidebarDraft"].(map[string]any)
	if !ok || draft["baseViewId"] != "view-1" || draft["group"] != "repository" {
		t.Fatalf("sidebarDraft = %#v, want mapped draft", state["sidebarDraft"])
	}
	prefs, ok := state["sidebarTaskPrefs"].(map[string]any)
	if !ok || len(prefs["pinnedTaskIds"].([]string)) != 1 {
		t.Fatalf("sidebarTaskPrefs = %#v, want mapped preferences", state["sidebarTaskPrefs"])
	}
	lastUsed, ok := state["taskCreateLastUsed"].(map[string]any)
	if !ok || lastUsed["repositoryId"] != "repo-1" || lastUsed["synced"] != true {
		t.Fatalf("taskCreateLastUsed = %#v, want mapped settings", state["taskCreateLastUsed"])
	}
}

func TestMapUserSettingsStateNormalizesNilSubtaskOrder(t *testing.T) {
	state := mapUserSettingsState(userdto.UserSettingsResponse{}, "workspace-1")
	prefs := state["sidebarTaskPrefs"].(map[string]any)
	order, ok := prefs["subtaskOrderByParentId"].(map[string][]string)
	if !ok || order == nil || len(order) != 0 {
		t.Fatalf("subtaskOrderByParentId = %#v, want empty map", prefs["subtaskOrderByParentId"])
	}
}
