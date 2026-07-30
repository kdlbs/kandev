package backendapp

import (
	"testing"

	userdto "github.com/kandev/kandev/internal/user/dto"
	usermodels "github.com/kandev/kandev/internal/user/models"
)

func TestMapUserSettingsStateIncludesArchiveConfirmation(t *testing.T) {
	state := mapUserSettingsState(userdto.UserSettingsResponse{
		Settings: userdto.UserSettingsDTO{ConfirmTaskArchive: true},
	}, "workspace-1")

	got, ok := state["confirmTaskArchive"].(bool)
	if !ok || !got {
		t.Fatalf("confirmTaskArchive = %#v, want true", state["confirmTaskArchive"])
	}
}

func TestMapUserSettingsStateIncludesTasksListShowDetails(t *testing.T) {
	state := mapUserSettingsState(userdto.UserSettingsResponse{
		Settings: userdto.UserSettingsDTO{TasksListShowDetails: true},
	}, "workspace-1")

	if got, ok := state["tasksListShowDetails"].(bool); !ok || !got {
		t.Fatalf("tasksListShowDetails = %#v, want true", state["tasksListShowDetails"])
	}
}

func TestMapUserSettingsStateIncludesNormalizedMCPTaskAgentProfileDefault(t *testing.T) {
	state := mapUserSettingsState(userdto.UserSettingsResponse{
		Settings: userdto.UserSettingsDTO{MCPTaskAgentProfileDefault: "future_value"},
	}, "workspace-1")

	got, ok := state["mcpTaskAgentProfileDefault"].(string)
	if !ok || got != usermodels.MCPTaskAgentProfileDefaultCurrentTask {
		t.Fatalf("mcpTaskAgentProfileDefault = %#v, want current_task", state["mcpTaskAgentProfileDefault"])
	}
}

func TestMapUserSettingsStateIncludesAppStatusBarOrder(t *testing.T) {
	state := mapUserSettingsState(userdto.UserSettingsResponse{
		Settings: userdto.UserSettingsDTO{AppStatusBarOrder: usermodels.AppStatusBarOrder{
			LeftItemIDs:  []string{"left"},
			RightItemIDs: []string{"right"},
		}},
	}, "workspace-1")

	got, ok := state["appStatusBarOrder"].(map[string]any)
	if !ok {
		t.Fatalf("appStatusBarOrder = %#v, want map", state["appStatusBarOrder"])
	}
	if left, ok := got["leftItemIds"].([]string); !ok || len(left) != 1 || left[0] != "left" {
		t.Fatalf("leftItemIds = %#v, want [left]", got["leftItemIds"])
	}
}

func TestMapUserSettingsStateIncludesSystemMetricsDisplayPreference(t *testing.T) {
	state := mapUserSettingsState(userdto.UserSettingsResponse{
		Settings: userdto.UserSettingsDTO{SystemMetricsDisplay: usermodels.SystemMetricsDisplaySettings{
			ShowInTopbar: true,
			Simplified:   true,
		}},
	}, "workspace-1")

	got, ok := state["systemMetricsDisplay"].(map[string]any)
	if !ok {
		t.Fatalf("systemMetricsDisplay = %#v, want map", state["systemMetricsDisplay"])
	}
	if simplified, ok := got["simplified"].(bool); !ok || !simplified {
		t.Fatalf("simplified = %#v, want true", got["simplified"])
	}
}

func TestMapUserSettingsStateIncludesPRPanelPlacementDefault(t *testing.T) {
	state := mapUserSettingsState(userdto.UserSettingsResponse{}, "workspace-1")

	if got := state["prPanelPlacement"]; got != "agent" {
		t.Fatalf("prPanelPlacement = %#v, want agent", got)
	}
}

func TestMapUserSettingsStateIncludesExplicitPRPanelPlacement(t *testing.T) {
	state := mapUserSettingsState(userdto.UserSettingsResponse{
		Settings: userdto.UserSettingsDTO{PRPanelPlacement: usermodels.PRPanelPlacementRight},
	}, "workspace-1")

	if got := state["prPanelPlacement"]; got != "right" {
		t.Fatalf("prPanelPlacement = %#v, want right", got)
	}
}
