package backendapp

import (
	"encoding/json"
	"testing"

	userdto "github.com/kandev/kandev/internal/user/dto"
	usermodels "github.com/kandev/kandev/internal/user/models"
)

func TestMapUserSettingsStateIncludesAzureDevOpsBrowsePreferences(t *testing.T) {
	preferences := json.RawMessage(`{"workspace-1":{"mode":"board","filters":{"projectId":"project-2"},"board":{"teamId":"team-2","boardId":"board-2","focusedColumnId":"done"}}}`)
	state := mapUserSettingsState(userdto.UserSettingsResponse{
		Settings: userdto.UserSettingsDTO{AzureDevOpsBrowsePreferences: preferences},
	}, "workspace-1")

	encoded, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("marshal boot settings: %v", err)
	}
	var payload struct {
		Loaded      bool `json:"loaded"`
		Preferences map[string]struct {
			Mode    string `json:"mode"`
			Filters struct {
				ProjectID string `json:"projectId"`
			} `json:"filters"`
			Board struct {
				TeamID  string `json:"teamId"`
				BoardID string `json:"boardId"`
			} `json:"board"`
		} `json:"azureDevOpsBrowsePreferences"`
	}
	if err := json.Unmarshal(encoded, &payload); err != nil {
		t.Fatalf("decode boot settings: %v", err)
	}

	preference := payload.Preferences["workspace-1"]
	if !payload.Loaded || preference.Mode != "board" || preference.Filters.ProjectID != "project-2" || preference.Board.TeamID != "team-2" || preference.Board.BoardID != "board-2" {
		t.Fatalf("Azure browse preferences missing from loaded boot settings: %s", encoded)
	}
}

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
	prefs, ok := state["sidebarTaskPrefs"].(map[string]any)
	if !ok {
		t.Fatalf("sidebarTaskPrefs = %#v, want map[string]any", state["sidebarTaskPrefs"])
	}
	order, ok := prefs["subtaskOrderByParentId"].(map[string][]string)
	if !ok || order == nil || len(order) != 0 {
		t.Fatalf("subtaskOrderByParentId = %#v, want empty map", prefs["subtaskOrderByParentId"])
	}
}
