package dto

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/kandev/kandev/internal/user/models"
)

// TestFromUserSettingsIncludesAtomicRevision verifies the DTO carries the settings revision.
func TestFromUserSettingsIncludesAtomicRevision(t *testing.T) {
	got := FromUserSettings(&models.UserSettings{Revision: 42}).Revision
	if got != 42 {
		t.Fatalf("Revision = %d, want 42", got)
	}
}

func TestSidebarTaskColorAutomationDTOContract(t *testing.T) {
	value := models.SidebarTaskColorAutomation{
		Enabled: true,
		Rules: []models.SidebarTaskColorRule{{
			ID:        "blocked",
			Enabled:   true,
			Condition: models.SidebarTaskColorCondition{Dimension: models.SidebarTaskColorDimensionTaskState, Value: "BLOCKED", Label: "Blocked"},
			Output:    models.SidebarTaskColorOutput{Kind: models.SidebarTaskColorOutputFixed, Color: "red"},
		}},
	}
	got := FromUserSettings(&models.UserSettings{SidebarTaskColorAutomation: value})
	if !reflect.DeepEqual(got.SidebarTaskColorAutomation, value) {
		t.Fatalf("SidebarTaskColorAutomation = %#v, want %#v", got.SidebarTaskColorAutomation, value)
	}

	var request UpdateUserSettingsRequest
	if err := json.Unmarshal([]byte(`{"sidebar_task_color_automation":{"enabled":true,"rules":[]}}`), &request); err != nil {
		t.Fatalf("decode automatic color patch: %v", err)
	}
	if request.SidebarTaskColorAutomation == nil || !request.SidebarTaskColorAutomation.Enabled {
		t.Fatalf("SidebarTaskColorAutomation patch = %#v, want explicit enabled value", request.SidebarTaskColorAutomation)
	}
}

func TestSidebarTaskColorsDTOContract(t *testing.T) {
	red := "red"
	value := map[string]*string{"task-red": &red, "task-cleared": nil}
	got := FromUserSettings(&models.UserSettings{SidebarTaskColors: value})
	if !reflect.DeepEqual(got.SidebarTaskColors, value) {
		t.Fatalf("SidebarTaskColors = %#v, want %#v", got.SidebarTaskColors, value)
	}
	if got.SidebarTaskColors["task-red"] == value["task-red"] {
		t.Fatal("DTO color map aliases the model pointer")
	}

	var request UpdateUserSettingsRequest
	if err := json.Unmarshal([]byte(`{"sidebar_task_color_patch":{"if_missing":true,"colors":{"task-red":"red","task-cleared":null}}}`), &request); err != nil {
		t.Fatalf("decode manual color patch: %v", err)
	}
	if request.SidebarTaskColorPatch == nil || !request.SidebarTaskColorPatch.IfMissing {
		t.Fatalf("SidebarTaskColorPatch = %#v, want missing-only patch", request.SidebarTaskColorPatch)
	}
}

// TestUpdateUserSettingsRequestExposesAzureDevOpsBrowsePreferences verifies the patch request exposes the Azure DevOps browse preferences field.
func TestUpdateUserSettingsRequestExposesAzureDevOpsBrowsePreferences(t *testing.T) {
	field, ok := reflect.TypeFor[UpdateUserSettingsRequest]().FieldByName("AzureDevOpsBrowsePreferences")
	if !ok || field.Tag.Get("json") != "azure_devops_browse_preferences,omitempty" {
		t.Fatalf("Azure DevOps browse preferences patch field = %+v, want JSON preference field", field)
	}
}

// TestFromUserSettingsMapsAzureDevOpsBrowsePreferences verifies the DTO passes the Azure DevOps browse preferences through unchanged.
func TestFromUserSettingsMapsAzureDevOpsBrowsePreferences(t *testing.T) {
	preferences := json.RawMessage(`{"project-1":{"teamId":"team-1"}}`)
	settings := FromUserSettings(&models.UserSettings{AzureDevOpsBrowsePreferences: preferences})
	if string(settings.AzureDevOpsBrowsePreferences) != string(preferences) {
		t.Fatalf("AzureDevOpsBrowsePreferences = %s, want %s", settings.AzureDevOpsBrowsePreferences, preferences)
	}
}

// TestUpdateUserSettingsRequestExposesKanbanHiddenStepIDs verifies the patch request exposes the kanban hidden step IDs field.
func TestUpdateUserSettingsRequestExposesKanbanHiddenStepIDs(t *testing.T) {
	field, ok := reflect.TypeFor[UpdateUserSettingsRequest]().FieldByName("KanbanHiddenStepIDs")
	if !ok || field.Tag.Get("json") != "kanban_hidden_step_ids,omitempty" {
		t.Fatalf("KanbanHiddenStepIDs patch field = %+v, want JSON kanban_hidden_step_ids field", field)
	}
}

// TestFromUserSettingsMapsKanbanHiddenStepIDs verifies the DTO carries the per-workflow hidden step ID map.
func TestFromUserSettingsMapsKanbanHiddenStepIDs(t *testing.T) {
	hidden := map[string][]string{"wf-1": {"step-a", "step-b"}}
	settings := FromUserSettings(&models.UserSettings{KanbanHiddenStepIDs: hidden})
	if !reflect.DeepEqual(settings.KanbanHiddenStepIDs, hidden) {
		t.Fatalf("KanbanHiddenStepIDs = %#v, want %#v", settings.KanbanHiddenStepIDs, hidden)
	}
}

// TestUserSettingsDTOKanbanHiddenStepIDsAlwaysSerializesEvenWhenEmpty verifies an empty hidden step ID map serializes as {} instead of being omitted.
func TestUserSettingsDTOKanbanHiddenStepIDsAlwaysSerializesEvenWhenEmpty(t *testing.T) {
	// Regression test: the response field must never use `omitempty`. A
	// client that clears its hidden set to {} needs that {} to actually
	// appear in the JSON response — if the field were omitted (as it would
	// be with `omitempty` on a zero-length map), the frontend's
	// `s.kanban_hidden_step_ids ?? current.hiddenWorkflowStepIds` merge
	// treats the missing key as "field not sent, preserve current value"
	// and leaves the previous (now-stale) hidden set in place instead of
	// clearing it.
	settings := FromUserSettings(&models.UserSettings{KanbanHiddenStepIDs: map[string][]string{}})
	raw, err := json.Marshal(settings)
	if err != nil {
		t.Fatalf("marshal settings: %v", err)
	}
	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal settings: %v", err)
	}
	value, present := decoded["kanban_hidden_step_ids"]
	if !present {
		t.Fatal("kanban_hidden_step_ids key is absent from the serialized response, want present as {}")
	}
	if string(value) != "{}" {
		t.Fatalf("kanban_hidden_step_ids = %s, want {}", value)
	}
}

// TestKanbanHiddenStepIDsRequestDecode verifies decoding distinguishes an omitted field from an explicit empty map.
func TestKanbanHiddenStepIDsRequestDecode(t *testing.T) {
	t.Run("omitted value stays nil", func(t *testing.T) {
		var req UpdateUserSettingsRequest
		if err := json.Unmarshal([]byte(`{}`), &req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.KanbanHiddenStepIDs != nil {
			t.Fatalf("KanbanHiddenStepIDs = %#v, want nil", req.KanbanHiddenStepIDs)
		}
	})

	t.Run("explicit empty map is retained, not treated as omitted", func(t *testing.T) {
		var req UpdateUserSettingsRequest
		if err := json.Unmarshal([]byte(`{"kanban_hidden_step_ids":{}}`), &req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.KanbanHiddenStepIDs == nil || len(*req.KanbanHiddenStepIDs) != 0 {
			t.Fatalf("KanbanHiddenStepIDs = %#v, want non-nil empty map", req.KanbanHiddenStepIDs)
		}
	})
}

func TestWorkflowIDsWithAutoHideEmptyStepsContract(t *testing.T) {
	field, ok := reflect.TypeFor[UpdateUserSettingsRequest]().FieldByName("WorkflowIDsWithAutoHideEmptySteps")
	if !ok || field.Tag.Get("json") != "workflow_ids_with_auto_hide_empty_steps,omitempty" {
		t.Fatalf("WorkflowIDsWithAutoHideEmptySteps patch field = %+v, want JSON preference field", field)
	}

	defaultSettings := FromUserSettings(&models.UserSettings{})
	encoded, err := json.Marshal(defaultSettings)
	if err != nil {
		t.Fatalf("encode default settings: %v", err)
	}
	var serialized struct {
		WorkflowIDs []string `json:"workflow_ids_with_auto_hide_empty_steps"`
	}
	if err := json.Unmarshal(encoded, &serialized); err != nil {
		t.Fatalf("decode default settings: %v", err)
	}
	if serialized.WorkflowIDs == nil || len(serialized.WorkflowIDs) != 0 {
		t.Fatalf("serialized default WorkflowIDs = %#v, want non-nil empty", serialized.WorkflowIDs)
	}

	want := []string{"wf-a", "wf-b"}
	source := &models.UserSettings{}
	sourceField := reflect.ValueOf(source).Elem().FieldByName("WorkflowIDsWithAutoHideEmptySteps")
	if !sourceField.IsValid() {
		t.Fatal("models.UserSettings.WorkflowIDsWithAutoHideEmptySteps field is absent")
	}
	sourceField.Set(reflect.ValueOf(want))
	settings := FromUserSettings(source)
	value := reflect.ValueOf(settings)
	mapped := value.FieldByName("WorkflowIDsWithAutoHideEmptySteps")
	if !mapped.IsValid() || !reflect.DeepEqual(mapped.Interface(), want) {
		t.Fatalf("WorkflowIDsWithAutoHideEmptySteps = %#v, want %#v", mapped, want)
	}

	var omitted UpdateUserSettingsRequest
	if err := json.Unmarshal([]byte(`{}`), &omitted); err != nil {
		t.Fatalf("decode omitted preference: %v", err)
	}
	if omitted.WorkflowIDsWithAutoHideEmptySteps != nil {
		t.Fatalf("omitted WorkflowIDsWithAutoHideEmptySteps = %#v, want nil", omitted.WorkflowIDsWithAutoHideEmptySteps)
	}

	var cleared UpdateUserSettingsRequest
	if err := json.Unmarshal([]byte(`{"workflow_ids_with_auto_hide_empty_steps":[]}`), &cleared); err != nil {
		t.Fatalf("decode cleared preference: %v", err)
	}
	if cleared.WorkflowIDsWithAutoHideEmptySteps == nil || len(*cleared.WorkflowIDsWithAutoHideEmptySteps) != 0 {
		t.Fatalf("cleared WorkflowIDsWithAutoHideEmptySteps = %#v, want non-nil empty slice", cleared.WorkflowIDsWithAutoHideEmptySteps)
	}
}

// TestTasksListShowDetailsDTO verifies the DTO mapping and the nil-versus-explicit-false patch semantics.
func TestTasksListShowDetailsDTO(t *testing.T) {
	if !FromUserSettings(&models.UserSettings{TasksListShowDetails: true}).TasksListShowDetails {
		t.Fatal("TasksListShowDetails = false, want true")
	}

	t.Run("omitted value stays nil", func(t *testing.T) {
		var req UpdateUserSettingsRequest
		if err := json.Unmarshal([]byte(`{}`), &req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.TasksListShowDetails != nil {
			t.Fatalf("TasksListShowDetails = %#v, want nil", req.TasksListShowDetails)
		}
	})

	t.Run("explicit false is retained", func(t *testing.T) {
		var req UpdateUserSettingsRequest
		if err := json.Unmarshal([]byte(`{"tasks_list_show_details":false}`), &req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.TasksListShowDetails == nil || *req.TasksListShowDetails {
			t.Fatalf("TasksListShowDetails = %#v, want false", req.TasksListShowDetails)
		}
	})
}

// TestAppStatusBarOrderDTOAndPatchSemantics verifies the status bar order DTO mapping and its patch semantics.
func TestAppStatusBarOrderDTOAndPatchSemantics(t *testing.T) {
	want := models.AppStatusBarOrder{
		LeftItemIDs:  []string{"builtin:connection", "plugin:left"},
		RightItemIDs: []string{"builtin:metrics"},
	}
	got := FromUserSettings(&models.UserSettings{AppStatusBarOrder: want}).AppStatusBarOrder
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("AppStatusBarOrder = %#v, want %#v", got, want)
	}

	t.Run("omitted value stays nil", func(t *testing.T) {
		var req UpdateUserSettingsRequest
		if err := json.Unmarshal([]byte(`{}`), &req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.AppStatusBarOrder != nil {
			t.Fatalf("AppStatusBarOrder = %#v, want nil", req.AppStatusBarOrder)
		}
	})

	t.Run("explicit replacement is retained", func(t *testing.T) {
		var req UpdateUserSettingsRequest
		if err := json.Unmarshal([]byte(`{"app_status_bar_order":{"left_item_ids":["left"],"right_item_ids":[]}}`), &req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.AppStatusBarOrder == nil || !reflect.DeepEqual(req.AppStatusBarOrder.LeftItemIDs, []string{"left"}) {
			t.Fatalf("AppStatusBarOrder = %#v, want explicit replacement", req.AppStatusBarOrder)
		}
	})
}

// TestLspStatusLocationDTOAndPatchSemantics verifies the LSP status location DTO normalization and its patch semantics.
func TestLspStatusLocationDTOAndPatchSemantics(t *testing.T) {
	t.Run("response normalizes missing and unknown values to toolbar", func(t *testing.T) {
		for _, value := range []string{"", "future_location"} {
			got := FromUserSettings(&models.UserSettings{LspStatusLocation: value}).LspStatusLocation
			if got != models.LspStatusLocationToolbar {
				t.Fatalf("LspStatusLocation = %q, want toolbar", got)
			}
		}
	})

	t.Run("response preserves status bar", func(t *testing.T) {
		got := FromUserSettings(&models.UserSettings{
			LspStatusLocation: models.LspStatusLocationStatusBar,
		}).LspStatusLocation
		if got != models.LspStatusLocationStatusBar {
			t.Fatalf("LspStatusLocation = %q, want status_bar", got)
		}
	})

	t.Run("omitted patch stays nil", func(t *testing.T) {
		var req UpdateUserSettingsRequest
		if err := json.Unmarshal([]byte(`{}`), &req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.LspStatusLocation != nil {
			t.Fatalf("LspStatusLocation = %#v, want nil", req.LspStatusLocation)
		}
	})

	t.Run("explicit patch is retained", func(t *testing.T) {
		var req UpdateUserSettingsRequest
		if err := json.Unmarshal([]byte(`{"lsp_status_location":"status_bar"}`), &req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.LspStatusLocation == nil || *req.LspStatusLocation != models.LspStatusLocationStatusBar {
			t.Fatalf("LspStatusLocation = %#v, want status_bar", req.LspStatusLocation)
		}
	})
}

// TestUpdateUserSettingsRequestSystemMetricsDisplayPreservesOmittedFields verifies omitted system metrics display fields stay absent when re-marshaled.
func TestUpdateUserSettingsRequestSystemMetricsDisplayPreservesOmittedFields(t *testing.T) {
	var req UpdateUserSettingsRequest
	if err := json.Unmarshal([]byte(`{"system_metrics_display":{"show_in_topbar":true}}`), &req); err != nil {
		t.Fatalf("decode request: %v", err)
	}

	encoded, err := json.Marshal(req.SystemMetricsDisplay)
	if err != nil {
		t.Fatalf("marshal system metrics display: %v", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatalf("decode system metrics display: %v", err)
	}
	if _, ok := fields["simplified"]; ok {
		t.Fatalf("simplified = %s, want omitted field to remain absent", fields["simplified"])
	}
}

// TestFromUserSettingsIncludesArchiveConfirmation verifies the DTO carries the task archive confirmation flag.
func TestFromUserSettingsIncludesArchiveConfirmation(t *testing.T) {
	for _, want := range []bool{true, false} {
		dto := FromUserSettings(&models.UserSettings{ConfirmTaskArchive: want})
		if dto.ConfirmTaskArchive != want {
			t.Fatalf("ConfirmTaskArchive = %v, want %v", dto.ConfirmTaskArchive, want)
		}
	}
}

// TestAgentGeneratedTaskTitlesDTOAndPatchSemantics verifies the agent-generated task titles DTO mapping and its patch semantics.
func TestAgentGeneratedTaskTitlesDTOAndPatchSemantics(t *testing.T) {
	if !FromUserSettings(&models.UserSettings{AgentGeneratedTaskTitles: true}).AgentGeneratedTaskTitles {
		t.Fatal("AgentGeneratedTaskTitles = false, want true")
	}

	var omitted UpdateUserSettingsRequest
	if err := json.Unmarshal([]byte(`{}`), &omitted); err != nil {
		t.Fatalf("decode omitted request: %v", err)
	}
	if omitted.AgentGeneratedTaskTitles != nil {
		t.Fatalf("AgentGeneratedTaskTitles = %#v, want nil for omitted field", omitted.AgentGeneratedTaskTitles)
	}

	var explicit UpdateUserSettingsRequest
	if err := json.Unmarshal([]byte(`{"agent_generated_task_titles":true}`), &explicit); err != nil {
		t.Fatalf("decode explicit request: %v", err)
	}
	if explicit.AgentGeneratedTaskTitles == nil || !*explicit.AgentGeneratedTaskTitles {
		t.Fatalf("AgentGeneratedTaskTitles = %#v, want true", explicit.AgentGeneratedTaskTitles)
	}

	var disabled UpdateUserSettingsRequest
	if err := json.Unmarshal([]byte(`{"agent_generated_task_titles":false}`), &disabled); err != nil {
		t.Fatalf("decode explicit false request: %v", err)
	}
	if disabled.AgentGeneratedTaskTitles == nil || *disabled.AgentGeneratedTaskTitles {
		t.Fatalf("AgentGeneratedTaskTitles = %#v, want false", disabled.AgentGeneratedTaskTitles)
	}
}

// TestFromUserSettingsIncludesNormalizedMCPTaskAgentProfileDefault verifies the DTO normalizes the MCP task agent profile default.
func TestFromUserSettingsIncludesNormalizedMCPTaskAgentProfileDefault(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{name: "workspace default", value: models.MCPTaskAgentProfileDefaultWorkspaceDefault, want: models.MCPTaskAgentProfileDefaultWorkspaceDefault},
		{name: "unknown defaults to current task", value: "future_value", want: models.MCPTaskAgentProfileDefaultCurrentTask},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw, err := json.Marshal(FromUserSettings(&models.UserSettings{MCPTaskAgentProfileDefault: tt.value}))
			if err != nil {
				t.Fatalf("marshal DTO: %v", err)
			}
			var payload map[string]any
			if err := json.Unmarshal(raw, &payload); err != nil {
				t.Fatalf("decode DTO: %v", err)
			}
			if got := payload["mcp_task_agent_profile_default"]; got != tt.want {
				t.Fatalf("mcp_task_agent_profile_default = %#v, want %q", got, tt.want)
			}
		})
	}
}

// TestFromUserSettingsIncludesNormalizedStartupPage verifies the DTO normalizes the startup page.
func TestFromUserSettingsIncludesNormalizedStartupPage(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{name: "last task", value: models.StartupPageLastTask, want: models.StartupPageLastTask},
		{name: "unknown defaults to task overview", value: "future_value", want: models.StartupPageTaskOverview},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw, err := json.Marshal(FromUserSettings(&models.UserSettings{StartupPage: tt.value}))
			if err != nil {
				t.Fatalf("marshal DTO: %v", err)
			}
			var payload map[string]any
			if err := json.Unmarshal(raw, &payload); err != nil {
				t.Fatalf("decode DTO: %v", err)
			}
			if got := payload["startup_page"]; got != tt.want {
				t.Fatalf("startup_page = %#v, want %q", got, tt.want)
			}
		})
	}
}

// TestFromUserSettingsIncludesNormalizedLastSeenDisplay verifies the DTO normalizes the last-seen display mode.
func TestFromUserSettingsIncludesNormalizedLastSeenDisplay(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{name: "relative", value: models.LastSeenDisplayRelative, want: models.LastSeenDisplayRelative},
		{name: "unknown defaults to absolute", value: "future_value", want: models.LastSeenDisplayAbsolute},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw, err := json.Marshal(FromUserSettings(&models.UserSettings{LastSeenDisplay: tt.value}))
			if err != nil {
				t.Fatalf("marshal DTO: %v", err)
			}
			var payload map[string]any
			if err := json.Unmarshal(raw, &payload); err != nil {
				t.Fatalf("decode DTO: %v", err)
			}
			if got := payload["last_seen_display"]; got != tt.want {
				t.Fatalf("last_seen_display = %#v, want %q", got, tt.want)
			}
		})
	}
}

// TestUpdateUserSettingsRequestStartupPagePatchSemantics verifies startup page patch decoding distinguishes omitted and explicit values.
func TestUpdateUserSettingsRequestStartupPagePatchSemantics(t *testing.T) {
	t.Run("omitted value stays nil", func(t *testing.T) {
		var req UpdateUserSettingsRequest
		if err := json.Unmarshal([]byte(`{}`), &req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.StartupPage != nil {
			t.Fatalf("StartupPage = %#v, want nil", req.StartupPage)
		}
	})

	t.Run("explicit last task is retained", func(t *testing.T) {
		var req UpdateUserSettingsRequest
		if err := json.Unmarshal([]byte(`{"startup_page":"last_task"}`), &req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.StartupPage == nil || *req.StartupPage != models.StartupPageLastTask {
			t.Fatalf("StartupPage = %#v, want last_task", req.StartupPage)
		}
	})
}

// TestUpdateUserSettingsRequestMCPTaskAgentProfileDefaultPatchSemantics verifies MCP task agent profile default patch decoding distinguishes omitted and explicit values.
func TestUpdateUserSettingsRequestMCPTaskAgentProfileDefaultPatchSemantics(t *testing.T) {
	t.Run("omitted value stays nil", func(t *testing.T) {
		var req UpdateUserSettingsRequest
		if err := json.Unmarshal([]byte(`{}`), &req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.MCPTaskAgentProfileDefault != nil {
			t.Fatalf("MCPTaskAgentProfileDefault = %q, want nil", *req.MCPTaskAgentProfileDefault)
		}
	})

	t.Run("explicit value is retained", func(t *testing.T) {
		var req UpdateUserSettingsRequest
		if err := json.Unmarshal([]byte(`{"mcp_task_agent_profile_default":"workspace_default"}`), &req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.MCPTaskAgentProfileDefault == nil || *req.MCPTaskAgentProfileDefault != models.MCPTaskAgentProfileDefaultWorkspaceDefault {
			t.Fatalf("MCPTaskAgentProfileDefault = %#v, want workspace_default", req.MCPTaskAgentProfileDefault)
		}
	})
}

// TestNullableSidebarDraft verifies sidebar draft decoding distinguishes omitted, null, and object values.
func TestNullableSidebarDraft(t *testing.T) {
	t.Run("omitted field is not set", func(t *testing.T) {
		var req UpdateUserSettingsRequest
		if err := json.Unmarshal([]byte(`{}`), &req); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if req.SidebarDraft.Set {
			t.Fatal("expected omitted sidebar_draft to remain unset")
		}
		if req.SidebarDraft.ServiceValue() != nil {
			t.Fatal("expected omitted sidebar_draft to map to nil service value")
		}
	})

	t.Run("null field is set to nil draft", func(t *testing.T) {
		var req UpdateUserSettingsRequest
		if err := json.Unmarshal([]byte(`{"sidebar_draft":null}`), &req); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		serviceValue := req.SidebarDraft.ServiceValue()
		if !req.SidebarDraft.Set || serviceValue == nil || *serviceValue != nil {
			t.Fatalf("expected explicit null to map to set nil draft, got set=%v value=%v", req.SidebarDraft.Set, serviceValue)
		}
	})

	t.Run("object field is set to draft", func(t *testing.T) {
		var req UpdateUserSettingsRequest
		raw := []byte(`{"sidebar_draft":{"base_view_id":"view-1","filters":[],"sort":{"key":"state","direction":"asc"},"group":"state"}}`)
		if err := json.Unmarshal(raw, &req); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		serviceValue := req.SidebarDraft.ServiceValue()
		if !req.SidebarDraft.Set || serviceValue == nil || *serviceValue == nil || (*serviceValue).BaseViewID != "view-1" {
			t.Fatalf("expected object to map to draft, got set=%v value=%v", req.SidebarDraft.Set, serviceValue)
		}
	})
}

// TestNullableRawMessage verifies raw-message decoding distinguishes omitted, null, and JSON values.
func TestNullableRawMessage(t *testing.T) {
	t.Run("omitted field is not set", func(t *testing.T) {
		var req UpdateUserSettingsRequest
		if err := json.Unmarshal([]byte(`{}`), &req); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if req.JiraSavedViews.Set {
			t.Fatal("expected omitted jira_saved_views to remain unset")
		}
		if req.JiraSavedViews.ServiceValue() != nil {
			t.Fatal("expected omitted jira_saved_views to map to nil service value")
		}
	})

	t.Run("null field is set to nil raw message", func(t *testing.T) {
		var req UpdateUserSettingsRequest
		if err := json.Unmarshal([]byte(`{"jira_saved_views":null}`), &req); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		serviceValue := req.JiraSavedViews.ServiceValue()
		if !req.JiraSavedViews.Set || serviceValue == nil || *serviceValue != nil {
			t.Fatalf("expected explicit null to map to set nil raw message, got set=%v value=%v", req.JiraSavedViews.Set, serviceValue)
		}
	})

	t.Run("json field is set to raw message", func(t *testing.T) {
		var req UpdateUserSettingsRequest
		if err := json.Unmarshal([]byte(`{"jira_saved_views":[{"id":"view-1"}]}`), &req); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		serviceValue := req.JiraSavedViews.ServiceValue()
		if !req.JiraSavedViews.Set || serviceValue == nil || *serviceValue == nil || string(**serviceValue) != `[{"id":"view-1"}]` {
			t.Fatalf("expected JSON value to map to raw message, got set=%v value=%v", req.JiraSavedViews.Set, serviceValue)
		}
	})
}
