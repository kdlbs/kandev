package dto

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/kandev/kandev/internal/user/models"
)

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

func TestFromUserSettingsIncludesArchiveConfirmation(t *testing.T) {
	for _, want := range []bool{true, false} {
		dto := FromUserSettings(&models.UserSettings{ConfirmTaskArchive: want})
		if dto.ConfirmTaskArchive != want {
			t.Fatalf("ConfirmTaskArchive = %v, want %v", dto.ConfirmTaskArchive, want)
		}
	}
}

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
