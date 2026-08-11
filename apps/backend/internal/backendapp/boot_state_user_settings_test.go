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

func TestMapUserSettingsStateIncludesAgentGeneratedTaskTitles(t *testing.T) {
	state := mapUserSettingsState(userdto.UserSettingsResponse{
		Settings: userdto.UserSettingsDTO{AgentGeneratedTaskTitles: true},
	}, "workspace-1")

	got, ok := state["agentGeneratedTaskTitles"].(bool)
	if !ok || !got {
		t.Fatalf("agentGeneratedTaskTitles = %#v, want true", state["agentGeneratedTaskTitles"])
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

func TestMapUserSettingsStateIncludesNormalizedStartupPage(t *testing.T) {
	state := mapUserSettingsState(userdto.UserSettingsResponse{
		Settings: userdto.UserSettingsDTO{StartupPage: "future_value"},
	}, "workspace-1")

	got, ok := state["startupPage"].(string)
	if !ok || got != usermodels.StartupPageTaskOverview {
		t.Fatalf("startupPage = %#v, want task_overview", state["startupPage"])
	}
}

func TestMapUserSettingsStateIncludesHiddenWorkflowStepIds(t *testing.T) {
	state := mapUserSettingsState(userdto.UserSettingsResponse{
		Settings: userdto.UserSettingsDTO{
			KanbanHiddenStepIDs: map[string][]string{"wf-1": {"step-a", "step-b"}},
		},
	}, "workspace-1")

	// The boot payload key MUST be the store-field name (hiddenWorkflowStepIds),
	// not a camelCased form of the DTO's snake_case wire name
	// (kanbanHiddenStepIds) — the frontend's deepMerge hydrator matches keys
	// directly with no snake->camel remapping, so the wrong key silently
	// leaves the store field at {} on every cold boot.
	got, ok := state["hiddenWorkflowStepIds"].(map[string][]string)
	if !ok {
		t.Fatalf("hiddenWorkflowStepIds = %#v, want map[string][]string", state["hiddenWorkflowStepIds"])
	}
	if ids := got["wf-1"]; len(ids) != 2 || ids[0] != "step-a" || ids[1] != "step-b" {
		t.Fatalf("hiddenWorkflowStepIds[wf-1] = %#v, want [step-a step-b]", ids)
	}
	if _, wrongKey := state["kanbanHiddenStepIds"]; wrongKey {
		t.Fatal("boot state must not emit the dead kanbanHiddenStepIds key")
	}
}

func TestMapUserSettingsStateDefaultsHiddenWorkflowStepIdsToEmptyMap(t *testing.T) {
	state := mapUserSettingsState(userdto.UserSettingsResponse{
		Settings: userdto.UserSettingsDTO{},
	}, "workspace-1")

	got, ok := state["hiddenWorkflowStepIds"].(map[string][]string)
	if !ok || len(got) != 0 {
		t.Fatalf("hiddenWorkflowStepIds = %#v, want empty map[string][]string", state["hiddenWorkflowStepIds"])
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

func TestMapUserSettingsStateNormalizesLspStatusLocation(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{name: "status bar is preserved", value: usermodels.LspStatusLocationStatusBar, want: usermodels.LspStatusLocationStatusBar},
		{name: "empty uses toolbar", value: "", want: usermodels.LspStatusLocationToolbar},
		{name: "unknown uses toolbar", value: "future_location", want: usermodels.LspStatusLocationToolbar},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := mapUserSettingsState(userdto.UserSettingsResponse{
				Settings: userdto.UserSettingsDTO{LspStatusLocation: tt.value},
			}, "workspace-1")

			if got := state["lspStatusLocation"]; got != tt.want {
				t.Fatalf("lspStatusLocation = %#v, want %q", got, tt.want)
			}
		})
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
