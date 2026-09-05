package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/user/models"
	"github.com/kandev/kandev/internal/user/store"
	"go.uber.org/zap"
)

// ptr returns a pointer to a copy of v.
func ptr[T any](v T) *T { return &v }

// rawPatch wraps v in a double pointer for use as a raw JSON patch value.
func rawPatch(v json.RawMessage) **json.RawMessage {
	return ptr(ptr(v))
}

// rawClear returns a double pointer to a nil RawMessage for clearing a blob preference.
func rawClear() **json.RawMessage {
	return ptr((*json.RawMessage)(nil))
}

// TestApplyBasicSettingsTasksListShowDetails verifies applyBasicSettings preserves TasksListShowDetails when omitted and applies explicit values.
func TestApplyBasicSettingsTasksListShowDetails(t *testing.T) {
	t.Run("omission preserves saved value", func(t *testing.T) {
		settings := &models.UserSettings{TasksListShowDetails: true}
		if err := applyBasicSettings(settings, &UpdateUserSettingsRequest{}); err != nil {
			t.Fatalf("apply basic settings: %v", err)
		}
		if !settings.TasksListShowDetails {
			t.Fatal("TasksListShowDetails = false, want true")
		}
	})

	for _, value := range []bool{false, true} {
		t.Run(fmt.Sprintf("explicit %t is applied", value), func(t *testing.T) {
			settings := &models.UserSettings{TasksListShowDetails: !value}
			if err := applyBasicSettings(settings, &UpdateUserSettingsRequest{TasksListShowDetails: ptr(value)}); err != nil {
				t.Fatalf("apply basic settings: %v", err)
			}
			if settings.TasksListShowDetails != value {
				t.Fatalf("TasksListShowDetails = %t, want %t", settings.TasksListShowDetails, value)
			}
		})
	}
}

// TestApplyBasicSettingsSystemMetricsDisplayPreservesOmittedFields verifies omitted SystemMetricsDisplay subfields are preserved by applyBasicSettings.
func TestApplyBasicSettingsSystemMetricsDisplayPreservesOmittedFields(t *testing.T) {
	settings := &models.UserSettings{
		SystemMetricsDisplay: models.SystemMetricsDisplaySettings{
			ShowInTopbar: false,
			Simplified:   true,
		},
	}
	req := &UpdateUserSettingsRequest{
		SystemMetricsDisplay: &SystemMetricsDisplaySettingsPatch{ShowInTopbar: ptr(true)},
	}

	if err := applyBasicSettings(settings, req); err != nil {
		t.Fatalf("apply basic settings: %v", err)
	}
	if !settings.SystemMetricsDisplay.Simplified {
		t.Fatal("simplified = false, want existing value preserved when omitted")
	}
}

func TestApplyBasicSettingsQuickChatTabOrderValidation(t *testing.T) {
	tooManyWorkspaces := make(map[string][]string, 201)
	for index := 0; index < 201; index++ {
		tooManyWorkspaces[fmt.Sprintf("workspace-%d", index)] = []string{"conversation:one"}
	}
	tooManyReferences := make([]string, 201)
	for index := range tooManyReferences {
		tooManyReferences[index] = fmt.Sprintf("conversation:%d", index)
	}
	tooManyBytes := map[string][]string{
		"workspace-1": {strings.Repeat("x", maxUserPreferenceBlobBytes)},
	}

	tests := []struct {
		name    string
		order   map[string][]string
		wantErr string
	}{
		{
			name:    "rejects too many workspaces",
			order:   tooManyWorkspaces,
			wantErr: "quick_chat_tab_order_by_workspace: max 200 workspaces allowed",
		},
		{
			name: "rejects too many references in one workspace",
			order: map[string][]string{
				"workspace-1": tooManyReferences,
			},
			wantErr: "quick_chat_tab_order_by_workspace[workspace-1]: max 200 tab references allowed",
		},
		{
			name:    "rejects an oversized serialized order",
			order:   tooManyBytes,
			wantErr: "quick_chat_tab_order_by_workspace: max 65536 bytes allowed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			settings := &models.UserSettings{
				QuickChatTabOrderByWorkspace: map[string][]string{"existing": {"conversation:keep"}},
			}
			err := applyBasicSettings(settings, &UpdateUserSettingsRequest{
				QuickChatTabOrderByWorkspace: ptr(tt.order),
			})
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want error containing %q", err, tt.wantErr)
			}
			if !reflect.DeepEqual(settings.QuickChatTabOrderByWorkspace, map[string][]string{"existing": {"conversation:keep"}}) {
				t.Fatalf("rejected update changed settings to %#v", settings.QuickChatTabOrderByWorkspace)
			}
		})
	}
}

// makeLayouts builds n SavedLayout fixtures with distinct IDs and names.
func makeLayouts(n int) []models.SavedLayout {
	layouts := make([]models.SavedLayout, n)
	for i := range layouts {
		layouts[i] = models.SavedLayout{
			ID:        fmt.Sprintf("layout-%d", i),
			Name:      fmt.Sprintf("Layout %d", i),
			IsDefault: false,
			Layout:    json.RawMessage(`{}`),
			CreatedAt: "2026-01-01T00:00:00Z",
		}
	}
	return layouts
}

// TestApplyBasicSettings_ReleaseNotes verifies release-note fields are preserved when nil and set or cleared when provided.
func TestApplyBasicSettings_ReleaseNotes(t *testing.T) {
	t.Run("nil fields leave settings unchanged", func(t *testing.T) {
		settings := &models.UserSettings{
			ShowReleaseNotification:     true,
			ReleaseNotesLastSeenVersion: "1.0.0",
		}
		req := &UpdateUserSettingsRequest{}
		if err := applyBasicSettings(settings, req); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if settings.ShowReleaseNotification != true {
			t.Fatalf("expected ShowReleaseNotification=true, got %v", settings.ShowReleaseNotification)
		}
		if settings.ReleaseNotesLastSeenVersion != "1.0.0" {
			t.Fatalf("expected ReleaseNotesLastSeenVersion=1.0.0, got %s", settings.ReleaseNotesLastSeenVersion)
		}
	})

	t.Run("ShowReleaseNotification set to false", func(t *testing.T) {
		settings := &models.UserSettings{ShowReleaseNotification: true}
		req := &UpdateUserSettingsRequest{ShowReleaseNotification: ptr(false)}
		if err := applyBasicSettings(settings, req); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if settings.ShowReleaseNotification != false {
			t.Fatalf("expected ShowReleaseNotification=false, got %v", settings.ShowReleaseNotification)
		}
	})

	t.Run("ShowReleaseNotification set to true", func(t *testing.T) {
		settings := &models.UserSettings{ShowReleaseNotification: false}
		req := &UpdateUserSettingsRequest{ShowReleaseNotification: ptr(true)}
		if err := applyBasicSettings(settings, req); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if settings.ShowReleaseNotification != true {
			t.Fatalf("expected ShowReleaseNotification=true, got %v", settings.ShowReleaseNotification)
		}
	})

	t.Run("ReleaseNotesLastSeenVersion updated", func(t *testing.T) {
		settings := &models.UserSettings{ReleaseNotesLastSeenVersion: "1.0.0"}
		req := &UpdateUserSettingsRequest{ReleaseNotesLastSeenVersion: ptr("2.0.0")}
		if err := applyBasicSettings(settings, req); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if settings.ReleaseNotesLastSeenVersion != "2.0.0" {
			t.Fatalf("expected ReleaseNotesLastSeenVersion=2.0.0, got %s", settings.ReleaseNotesLastSeenVersion)
		}
	})

	t.Run("ReleaseNotesLastSeenVersion cleared with empty string", func(t *testing.T) {
		settings := &models.UserSettings{ReleaseNotesLastSeenVersion: "1.0.0"}
		req := &UpdateUserSettingsRequest{ReleaseNotesLastSeenVersion: ptr("")}
		if err := applyBasicSettings(settings, req); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if settings.ReleaseNotesLastSeenVersion != "" {
			t.Fatalf("expected empty ReleaseNotesLastSeenVersion, got %s", settings.ReleaseNotesLastSeenVersion)
		}
	})
}

// TestApplyBasicSettings_ConfirmTaskArchive verifies archive confirmation stays enabled when omitted and is toggled by explicit values.
func TestApplyBasicSettings_ConfirmTaskArchive(t *testing.T) {
	t.Run("omitted value leaves confirmation enabled", func(t *testing.T) {
		settings := &models.UserSettings{ConfirmTaskArchive: true}
		if err := applyBasicSettings(settings, &UpdateUserSettingsRequest{}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !settings.ConfirmTaskArchive {
			t.Fatal("expected archive confirmation to remain enabled")
		}
	})

	t.Run("explicit false disables confirmation", func(t *testing.T) {
		settings := &models.UserSettings{ConfirmTaskArchive: true}
		if err := applyBasicSettings(settings, &UpdateUserSettingsRequest{
			ConfirmTaskArchive: ptr(false),
		}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if settings.ConfirmTaskArchive {
			t.Fatal("expected archive confirmation to be disabled")
		}
	})

	t.Run("explicit true re-enables confirmation", func(t *testing.T) {
		settings := &models.UserSettings{ConfirmTaskArchive: false}
		if err := applyBasicSettings(settings, &UpdateUserSettingsRequest{
			ConfirmTaskArchive: ptr(true),
		}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !settings.ConfirmTaskArchive {
			t.Fatal("expected archive confirmation to be enabled")
		}
	})
}

// TestApplyBasicSettings_PreventAutoStartAgentOnOpen verifies the auto-start setting is preserved when omitted and toggled by explicit values.
func TestApplyBasicSettings_PreventAutoStartAgentOnOpen(t *testing.T) {
	t.Run("omitted value leaves the setting unchanged", func(t *testing.T) {
		settings := &models.UserSettings{PreventAutoStartAgentOnOpen: true}
		if err := applyBasicSettings(settings, &UpdateUserSettingsRequest{}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !settings.PreventAutoStartAgentOnOpen {
			t.Fatal("expected the setting to remain enabled")
		}
	})

	t.Run("explicit true enables the setting", func(t *testing.T) {
		settings := &models.UserSettings{PreventAutoStartAgentOnOpen: false}
		if err := applyBasicSettings(settings, &UpdateUserSettingsRequest{
			PreventAutoStartAgentOnOpen: ptr(true),
		}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !settings.PreventAutoStartAgentOnOpen {
			t.Fatal("expected the setting to be enabled")
		}
	})

	t.Run("explicit false disables the setting", func(t *testing.T) {
		settings := &models.UserSettings{PreventAutoStartAgentOnOpen: true}
		if err := applyBasicSettings(settings, &UpdateUserSettingsRequest{
			PreventAutoStartAgentOnOpen: ptr(false),
		}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if settings.PreventAutoStartAgentOnOpen {
			t.Fatal("expected the setting to be disabled")
		}
	})
}

// TestApplyBasicSettingsAgentGeneratedTaskTitles verifies AgentGeneratedTaskTitles is preserved when omitted and applied when provided.
func TestApplyBasicSettingsAgentGeneratedTaskTitles(t *testing.T) {
	settings := &models.UserSettings{AgentGeneratedTaskTitles: false}
	if err := applyBasicSettings(settings, &UpdateUserSettingsRequest{}); err != nil {
		t.Fatalf("apply omitted setting: %v", err)
	}
	if settings.AgentGeneratedTaskTitles {
		t.Fatal("AgentGeneratedTaskTitles changed on omitted patch")
	}

	if err := applyBasicSettings(settings, &UpdateUserSettingsRequest{AgentGeneratedTaskTitles: ptr(true)}); err != nil {
		t.Fatalf("apply enabled setting: %v", err)
	}
	if !settings.AgentGeneratedTaskTitles {
		t.Fatal("AgentGeneratedTaskTitles = false, want true")
	}

	if err := applyBasicSettings(settings, &UpdateUserSettingsRequest{AgentGeneratedTaskTitles: ptr(false)}); err != nil {
		t.Fatalf("apply disabled setting: %v", err)
	}
	if settings.AgentGeneratedTaskTitles {
		t.Fatal("AgentGeneratedTaskTitles = true, want false")
	}
}

// TestApplyBasicSettingsAppStatusBarOrder verifies the status bar order is preserved when omitted and replaced when provided.
func TestApplyBasicSettingsAppStatusBarOrder(t *testing.T) {
	saved := models.AppStatusBarOrder{
		LeftItemIDs:  []string{"builtin:connection"},
		RightItemIDs: []string{"builtin:metrics"},
	}
	t.Run("omission preserves saved order", func(t *testing.T) {
		settings := &models.UserSettings{AppStatusBarOrder: saved}
		if err := applyBasicSettings(settings, &UpdateUserSettingsRequest{}); err != nil {
			t.Fatalf("apply settings: %v", err)
		}
		if fmt.Sprint(settings.AppStatusBarOrder) != fmt.Sprint(saved) {
			t.Fatalf("AppStatusBarOrder = %#v, want %#v", settings.AppStatusBarOrder, saved)
		}
	})

	t.Run("explicit value replaces saved order", func(t *testing.T) {
		next := models.AppStatusBarOrder{
			LeftItemIDs:  []string{"builtin:metrics"},
			RightItemIDs: []string{"builtin:connection"},
		}
		settings := &models.UserSettings{AppStatusBarOrder: saved}
		if err := applyBasicSettings(settings, &UpdateUserSettingsRequest{AppStatusBarOrder: &next}); err != nil {
			t.Fatalf("apply settings: %v", err)
		}
		if fmt.Sprint(settings.AppStatusBarOrder) != fmt.Sprint(next) {
			t.Fatalf("AppStatusBarOrder = %#v, want %#v", settings.AppStatusBarOrder, next)
		}
	})
}

// TestApplyLSPSettingsRejectsManualOnlyAutoInstallLanguage verifies manual-install-only languages are rejected without mutating settings.
func TestApplyLSPSettingsRejectsManualOnlyAutoInstallLanguage(t *testing.T) {
	settings := &models.UserSettings{}
	req := &UpdateUserSettingsRequest{
		LspAutoInstallLanguages: ptr([]string{"kotlin"}),
	}

	err := applyLSPSettings(settings, req)
	if err == nil || !strings.Contains(err.Error(), "does not support auto-install") {
		t.Fatalf("applyLSPSettings() error = %v, want manual-install-only error", err)
	}
	if len(settings.LspAutoInstallLanguages) != 0 {
		t.Fatalf("LspAutoInstallLanguages = %v, want unchanged", settings.LspAutoInstallLanguages)
	}
}

// TestApplyLSPSettingsAcceptsTaskHostAutoInstallPreference verifies a task-host language auto-install preference is accepted and stored.
func TestApplyLSPSettingsAcceptsTaskHostAutoInstallPreference(t *testing.T) {
	settings := &models.UserSettings{}
	req := &UpdateUserSettingsRequest{
		LspAutoInstallLanguages: ptr([]string{"rust"}),
	}

	if err := applyLSPSettings(settings, req); err != nil {
		t.Fatalf("applyLSPSettings() error = %v, want Rust preference accepted", err)
	}
	if !slices.Equal(settings.LspAutoInstallLanguages, []string{"rust"}) {
		t.Fatalf("LspAutoInstallLanguages = %v, want [rust]", settings.LspAutoInstallLanguages)
	}
}

// TestApplyLspStatusLocation verifies LspStatusLocation is preserved when omitted, applied when valid, and rejected when invalid.
func TestApplyLspStatusLocation(t *testing.T) {
	t.Run("omission preserves saved value", func(t *testing.T) {
		settings := &models.UserSettings{LspStatusLocation: models.LspStatusLocationStatusBar}
		if err := applyLSPSettings(settings, &UpdateUserSettingsRequest{}); err != nil {
			t.Fatalf("apply LSP settings: %v", err)
		}
		if settings.LspStatusLocation != models.LspStatusLocationStatusBar {
			t.Fatalf("LspStatusLocation = %q, want status_bar", settings.LspStatusLocation)
		}
	})

	t.Run("valid values are accepted", func(t *testing.T) {
		for _, value := range []string{
			models.LspStatusLocationToolbar,
			models.LspStatusLocationStatusBar,
		} {
			settings := &models.UserSettings{LspStatusLocation: models.LspStatusLocationToolbar}
			err := applyLSPSettings(settings, &UpdateUserSettingsRequest{
				LspStatusLocation: ptr(value),
			})
			if err != nil {
				t.Fatalf("apply %q: %v", value, err)
			}
			if settings.LspStatusLocation != value {
				t.Fatalf("LspStatusLocation = %q, want %q", settings.LspStatusLocation, value)
			}
		}
	})

	t.Run("invalid value is rejected without mutation", func(t *testing.T) {
		settings := &models.UserSettings{LspStatusLocation: models.LspStatusLocationToolbar}
		err := applyLSPSettings(settings, &UpdateUserSettingsRequest{
			LspStatusLocation: ptr("sidebar"),
		})
		if err == nil || !strings.Contains(err.Error(), "lsp_status_location") {
			t.Fatalf("apply invalid location error = %v, want lsp_status_location validation", err)
		}
		if settings.LspStatusLocation != models.LspStatusLocationToolbar {
			t.Fatalf("LspStatusLocation = %q after invalid patch, want toolbar", settings.LspStatusLocation)
		}
	})
}

// TestApplyBasicSettingsMCPTaskAgentProfileDefault verifies the MCP task-agent profile default is preserved, applied, and validated.
func TestApplyBasicSettingsMCPTaskAgentProfileDefault(t *testing.T) {
	t.Run("omission preserves saved value", func(t *testing.T) {
		settings := &models.UserSettings{MCPTaskAgentProfileDefault: models.MCPTaskAgentProfileDefaultWorkspaceDefault}
		if err := applyBasicSettings(settings, &UpdateUserSettingsRequest{}); err != nil {
			t.Fatalf("apply settings: %v", err)
		}
		if settings.MCPTaskAgentProfileDefault != models.MCPTaskAgentProfileDefaultWorkspaceDefault {
			t.Fatalf("MCPTaskAgentProfileDefault = %q, want workspace_default", settings.MCPTaskAgentProfileDefault)
		}
	})

	t.Run("valid values are accepted", func(t *testing.T) {
		for _, value := range []string{
			models.MCPTaskAgentProfileDefaultCurrentTask,
			models.MCPTaskAgentProfileDefaultWorkspaceDefault,
		} {
			settings := &models.UserSettings{MCPTaskAgentProfileDefault: models.MCPTaskAgentProfileDefaultCurrentTask}
			err := applyBasicSettings(settings, &UpdateUserSettingsRequest{MCPTaskAgentProfileDefault: ptr(value)})
			if err != nil {
				t.Fatalf("apply %q: %v", value, err)
			}
			if settings.MCPTaskAgentProfileDefault != value {
				t.Fatalf("MCPTaskAgentProfileDefault = %q, want %q", settings.MCPTaskAgentProfileDefault, value)
			}
		}
	})

	t.Run("invalid value is rejected without mutation", func(t *testing.T) {
		settings := &models.UserSettings{MCPTaskAgentProfileDefault: models.MCPTaskAgentProfileDefaultWorkspaceDefault}
		err := applyBasicSettings(settings, &UpdateUserSettingsRequest{MCPTaskAgentProfileDefault: ptr("expensive_profile")})
		if err == nil {
			t.Fatal("expected validation error")
		}
		if settings.MCPTaskAgentProfileDefault != models.MCPTaskAgentProfileDefaultWorkspaceDefault {
			t.Fatalf("MCPTaskAgentProfileDefault = %q after invalid update, want workspace_default", settings.MCPTaskAgentProfileDefault)
		}
	})
}

// TestApplyBasicSettingsShowAnchoredPromptBar verifies ShowAnchoredPromptBar is preserved when omitted and toggled by explicit values.
func TestApplyBasicSettingsShowAnchoredPromptBar(t *testing.T) {
	t.Run("omission preserves saved value", func(t *testing.T) {
		settings := &models.UserSettings{ShowAnchoredPromptBar: true}
		if err := applyBasicSettings(settings, &UpdateUserSettingsRequest{}); err != nil {
			t.Fatalf("apply settings: %v", err)
		}
		if !settings.ShowAnchoredPromptBar {
			t.Fatal("ShowAnchoredPromptBar = false, want true (unchanged)")
		}
	})

	t.Run("explicit value replaces saved value", func(t *testing.T) {
		settings := &models.UserSettings{ShowAnchoredPromptBar: false}
		if err := applyBasicSettings(settings, &UpdateUserSettingsRequest{ShowAnchoredPromptBar: ptr(true)}); err != nil {
			t.Fatalf("apply settings: %v", err)
		}
		if !settings.ShowAnchoredPromptBar {
			t.Fatal("ShowAnchoredPromptBar = false, want true")
		}
	})

	t.Run("explicit false disables it", func(t *testing.T) {
		settings := &models.UserSettings{ShowAnchoredPromptBar: true}
		if err := applyBasicSettings(settings, &UpdateUserSettingsRequest{ShowAnchoredPromptBar: ptr(false)}); err != nil {
			t.Fatalf("apply settings: %v", err)
		}
		if settings.ShowAnchoredPromptBar {
			t.Fatal("ShowAnchoredPromptBar = true, want false")
		}
	})
}

// TestApplyBasicSettingsTodoListPanel verifies ShowTodoListPanel is preserved when omitted and toggled by explicit values.
func TestApplyBasicSettingsTodoListPanel(t *testing.T) {
	t.Run("omission preserves saved value", func(t *testing.T) {
		settings := &models.UserSettings{ShowTodoListPanel: true}
		if err := applyBasicSettings(settings, &UpdateUserSettingsRequest{}); err != nil {
			t.Fatalf("apply settings: %v", err)
		}
		if !settings.ShowTodoListPanel {
			t.Fatal("ShowTodoListPanel = false, want true (unchanged)")
		}
	})

	t.Run("explicit value replaces saved value", func(t *testing.T) {
		settings := &models.UserSettings{ShowTodoListPanel: false}
		if err := applyBasicSettings(settings, &UpdateUserSettingsRequest{ShowTodoListPanel: ptr(true)}); err != nil {
			t.Fatalf("apply settings: %v", err)
		}
		if !settings.ShowTodoListPanel {
			t.Fatal("ShowTodoListPanel = false, want true")
		}
	})

	t.Run("explicit false disables it", func(t *testing.T) {
		settings := &models.UserSettings{ShowTodoListPanel: true}
		if err := applyBasicSettings(settings, &UpdateUserSettingsRequest{ShowTodoListPanel: ptr(false)}); err != nil {
			t.Fatalf("apply settings: %v", err)
		}
		if settings.ShowTodoListPanel {
			t.Fatal("ShowTodoListPanel = true, want false")
		}
	})
}

// TestApplyBasicSettingsTodoListPanelOnlyWhenNotEmpty verifies ShowTodoListPanelOnlyWhenNotEmpty is preserved when omitted and toggled by explicit values.
func TestApplyBasicSettingsTodoListPanelOnlyWhenNotEmpty(t *testing.T) {
	t.Run("omission preserves saved value", func(t *testing.T) {
		settings := &models.UserSettings{ShowTodoListPanelOnlyWhenNotEmpty: true}
		if err := applyBasicSettings(settings, &UpdateUserSettingsRequest{}); err != nil {
			t.Fatalf("apply settings: %v", err)
		}
		if !settings.ShowTodoListPanelOnlyWhenNotEmpty {
			t.Fatal("ShowTodoListPanelOnlyWhenNotEmpty = false, want true (unchanged)")
		}
	})

	t.Run("explicit value replaces saved value", func(t *testing.T) {
		settings := &models.UserSettings{ShowTodoListPanelOnlyWhenNotEmpty: false}
		if err := applyBasicSettings(settings, &UpdateUserSettingsRequest{ShowTodoListPanelOnlyWhenNotEmpty: ptr(true)}); err != nil {
			t.Fatalf("apply settings: %v", err)
		}
		if !settings.ShowTodoListPanelOnlyWhenNotEmpty {
			t.Fatal("ShowTodoListPanelOnlyWhenNotEmpty = false, want true")
		}
	})

	t.Run("explicit false disables it", func(t *testing.T) {
		settings := &models.UserSettings{ShowTodoListPanelOnlyWhenNotEmpty: true}
		if err := applyBasicSettings(settings, &UpdateUserSettingsRequest{ShowTodoListPanelOnlyWhenNotEmpty: ptr(false)}); err != nil {
			t.Fatalf("apply settings: %v", err)
		}
		if settings.ShowTodoListPanelOnlyWhenNotEmpty {
			t.Fatal("ShowTodoListPanelOnlyWhenNotEmpty = true, want false")
		}
	})
}

// TestApplyBasicSettingsTranscriptNavigation verifies transcript navigation flags are applied while omitted flags are preserved.
func TestApplyBasicSettingsTranscriptNavigation(t *testing.T) {
	settings := &models.UserSettings{
		ShowScrollToLastPrompt:          true,
		ShowScrollToStart:               true,
		ShowTranscriptAutoScrollControl: true,
	}
	if err := applyBasicSettings(
		settings,
		&UpdateUserSettingsRequest{
			ShowScrollToLastPrompt:          ptr(false),
			ShowTranscriptAutoScrollControl: ptr(false),
		},
	); err != nil {
		t.Fatalf("apply settings: %v", err)
	}
	if settings.ShowScrollToLastPrompt || !settings.ShowScrollToStart || settings.ShowTranscriptAutoScrollControl {
		t.Fatalf(
			"transcript controls = (%t, %t, %t), want (false, true, false)",
			settings.ShowScrollToLastPrompt,
			settings.ShowScrollToStart,
			settings.ShowTranscriptAutoScrollControl,
		)
	}
}

// TestApplyBasicSettings_TasksListPreferences verifies tasks list sort and group are applied and invalid values rejected.
func TestApplyBasicSettings_TasksListPreferences(t *testing.T) {
	t.Run("sets valid sort and group", func(t *testing.T) {
		settings := &models.UserSettings{}
		req := &UpdateUserSettingsRequest{
			TasksListSort:  ptr("title_asc"),
			TasksListGroup: ptr("repository"),
		}
		if err := applyBasicSettings(settings, req); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if settings.TasksListSort != "title_asc" {
			t.Fatalf("TasksListSort = %q, want title_asc", settings.TasksListSort)
		}
		if settings.TasksListGroup != "repository" {
			t.Fatalf("TasksListGroup = %q, want repository", settings.TasksListGroup)
		}
	})

	t.Run("rejects invalid sort", func(t *testing.T) {
		settings := &models.UserSettings{}
		req := &UpdateUserSettingsRequest{TasksListSort: ptr("priority_desc")}
		if err := applyBasicSettings(settings, req); err == nil {
			t.Fatal("expected invalid sort error")
		}
	})

	t.Run("rejects invalid group", func(t *testing.T) {
		settings := &models.UserSettings{}
		req := &UpdateUserSettingsRequest{TasksListGroup: ptr("assignee")}
		if err := applyBasicSettings(settings, req); err == nil {
			t.Fatal("expected invalid group error")
		}
	})
}

// TestApplyBasicSettings_TerminalFontFamily verifies the terminal font family is preserved, set, trimmed, and cleared.
func TestApplyBasicSettings_TerminalFontFamily(t *testing.T) {
	t.Run("nil leaves settings unchanged", func(t *testing.T) {
		settings := &models.UserSettings{TerminalFontFamily: "Fira Code"}
		req := &UpdateUserSettingsRequest{}
		if err := applyBasicSettings(settings, req); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if settings.TerminalFontFamily != "Fira Code" {
			t.Fatalf("expected TerminalFontFamily=Fira Code, got %s", settings.TerminalFontFamily)
		}
	})

	t.Run("sets value when provided", func(t *testing.T) {
		settings := &models.UserSettings{}
		req := &UpdateUserSettingsRequest{TerminalFontFamily: ptr("JetBrains Mono")}
		if err := applyBasicSettings(settings, req); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if settings.TerminalFontFamily != "JetBrains Mono" {
			t.Fatalf("expected TerminalFontFamily=JetBrains Mono, got %s", settings.TerminalFontFamily)
		}
	})

	t.Run("trims whitespace", func(t *testing.T) {
		settings := &models.UserSettings{}
		req := &UpdateUserSettingsRequest{TerminalFontFamily: ptr("  Fira Code  ")}
		if err := applyBasicSettings(settings, req); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if settings.TerminalFontFamily != "Fira Code" {
			t.Fatalf("expected TerminalFontFamily=Fira Code, got %q", settings.TerminalFontFamily)
		}
	})

	t.Run("clears with empty string", func(t *testing.T) {
		settings := &models.UserSettings{TerminalFontFamily: "Fira Code"}
		req := &UpdateUserSettingsRequest{TerminalFontFamily: ptr("")}
		if err := applyBasicSettings(settings, req); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if settings.TerminalFontFamily != "" {
			t.Fatalf("expected empty TerminalFontFamily, got %s", settings.TerminalFontFamily)
		}
	})
}

// TestApplyChangesPanelLayout verifies the changes panel layout is preserved, set, trimmed, and validated.
func TestApplyChangesPanelLayout(t *testing.T) {
	t.Run("nil leaves settings unchanged", func(t *testing.T) {
		settings := &models.UserSettings{ChangesPanelLayout: "tree"}
		req := &UpdateUserSettingsRequest{}
		if err := applyBasicSettings(settings, req); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if settings.ChangesPanelLayout != "tree" {
			t.Fatalf("expected ChangesPanelLayout=tree, got %s", settings.ChangesPanelLayout)
		}
	})

	t.Run("sets tree when provided", func(t *testing.T) {
		settings := &models.UserSettings{}
		req := &UpdateUserSettingsRequest{ChangesPanelLayout: ptr("tree")}
		if err := applyBasicSettings(settings, req); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if settings.ChangesPanelLayout != "tree" {
			t.Fatalf("expected ChangesPanelLayout=tree, got %s", settings.ChangesPanelLayout)
		}
	})

	t.Run("sets flat when provided", func(t *testing.T) {
		settings := &models.UserSettings{ChangesPanelLayout: "tree"}
		req := &UpdateUserSettingsRequest{ChangesPanelLayout: ptr("flat")}
		if err := applyBasicSettings(settings, req); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if settings.ChangesPanelLayout != "flat" {
			t.Fatalf("expected ChangesPanelLayout=flat, got %s", settings.ChangesPanelLayout)
		}
	})

	t.Run("rejects invalid value", func(t *testing.T) {
		settings := &models.UserSettings{}
		req := &UpdateUserSettingsRequest{ChangesPanelLayout: ptr("grid")}
		if err := applyBasicSettings(settings, req); err == nil {
			t.Fatal("expected error for invalid layout, got nil")
		}
	})

	t.Run("trims whitespace before validation", func(t *testing.T) {
		settings := &models.UserSettings{}
		req := &UpdateUserSettingsRequest{ChangesPanelLayout: ptr("  tree  ")}
		if err := applyBasicSettings(settings, req); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if settings.ChangesPanelLayout != "tree" {
			t.Fatalf("expected ChangesPanelLayout=tree, got %q", settings.ChangesPanelLayout)
		}
	})
}

// TestApplyStartupPage verifies the startup page is preserved when omitted, applied when valid, and rejected when invalid.
func TestApplyStartupPage(t *testing.T) {
	t.Run("omission preserves saved value", func(t *testing.T) {
		settings := &models.UserSettings{StartupPage: models.StartupPageLastTask}
		if err := applyBasicSettings(settings, &UpdateUserSettingsRequest{}); err != nil {
			t.Fatalf("apply basic settings: %v", err)
		}
		if settings.StartupPage != models.StartupPageLastTask {
			t.Fatalf("StartupPage = %q, want %q", settings.StartupPage, models.StartupPageLastTask)
		}
	})

	for _, value := range []string{models.StartupPageTaskOverview, models.StartupPageLastTask} {
		t.Run("applies "+value, func(t *testing.T) {
			settings := &models.UserSettings{StartupPage: models.StartupPageTaskOverview}
			if err := applyBasicSettings(settings, &UpdateUserSettingsRequest{StartupPage: ptr(value)}); err != nil {
				t.Fatalf("apply basic settings: %v", err)
			}
			if settings.StartupPage != value {
				t.Fatalf("StartupPage = %q, want %q", settings.StartupPage, value)
			}
		})
	}

	t.Run("rejects invalid value", func(t *testing.T) {
		settings := &models.UserSettings{StartupPage: models.StartupPageLastTask}
		err := applyBasicSettings(settings, &UpdateUserSettingsRequest{StartupPage: ptr("future_value")})
		if err == nil {
			t.Fatal("expected invalid startup page error")
		}
		if settings.StartupPage != models.StartupPageLastTask {
			t.Fatalf("StartupPage = %q, want unchanged %q", settings.StartupPage, models.StartupPageLastTask)
		}
	})
}

// TestApplyBasicSettings_TerminalFontSize verifies font size application, reset to zero, and bounds validation.
func TestApplyBasicSettings_TerminalFontSize(t *testing.T) {
	t.Run("nil leaves settings unchanged", func(t *testing.T) {
		settings := &models.UserSettings{TerminalFontSize: 14}
		req := &UpdateUserSettingsRequest{}
		if err := applyBasicSettings(settings, req); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if settings.TerminalFontSize != 14 {
			t.Fatalf("expected TerminalFontSize=14, got %d", settings.TerminalFontSize)
		}
	})

	t.Run("sets value when provided", func(t *testing.T) {
		settings := &models.UserSettings{}
		req := &UpdateUserSettingsRequest{TerminalFontSize: ptr(16)}
		if err := applyBasicSettings(settings, req); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if settings.TerminalFontSize != 16 {
			t.Fatalf("expected TerminalFontSize=16, got %d", settings.TerminalFontSize)
		}
	})

	t.Run("value below 8 returns error", func(t *testing.T) {
		settings := &models.UserSettings{}
		req := &UpdateUserSettingsRequest{TerminalFontSize: ptr(7)}
		if err := applyBasicSettings(settings, req); err == nil {
			t.Fatal("expected error for font size 7, got nil")
		}
	})

	t.Run("value above 24 returns error", func(t *testing.T) {
		settings := &models.UserSettings{}
		req := &UpdateUserSettingsRequest{TerminalFontSize: ptr(25)}
		if err := applyBasicSettings(settings, req); err == nil {
			t.Fatal("expected error for font size 25, got nil")
		}
	})

	t.Run("resets to 0 when 0 is provided", func(t *testing.T) {
		settings := &models.UserSettings{TerminalFontSize: 14}
		req := &UpdateUserSettingsRequest{TerminalFontSize: ptr(0)}
		if err := applyBasicSettings(settings, req); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if settings.TerminalFontSize != 0 {
			t.Fatalf("expected TerminalFontSize=0, got %d", settings.TerminalFontSize)
		}
	})
}

// TestApplySavedLayouts table-tests saved layout count limits and validation errors.
func TestApplySavedLayouts(t *testing.T) {
	tests := []struct {
		name        string
		req         *UpdateUserSettingsRequest
		wantErr     string
		wantCount   int
		wantApplied bool
	}{
		{
			name:        "nil request is a no-op",
			req:         &UpdateUserSettingsRequest{SavedLayouts: nil},
			wantApplied: false,
		},
		{
			name:        "empty slice is accepted",
			req:         &UpdateUserSettingsRequest{SavedLayouts: ptr([]models.SavedLayout{})},
			wantCount:   0,
			wantApplied: true,
		},
		{
			name: "valid single layout is applied",
			req: &UpdateUserSettingsRequest{
				SavedLayouts: ptr(makeLayouts(1)),
			},
			wantCount:   1,
			wantApplied: true,
		},
		{
			name: "valid layout with one default is applied",
			req: &UpdateUserSettingsRequest{
				SavedLayouts: ptr([]models.SavedLayout{
					{ID: "l1", Name: "Default layout", IsDefault: true, Layout: json.RawMessage(`{}`)},
					{ID: "l2", Name: "Other layout", Layout: json.RawMessage(`{}`)},
				}),
			},
			wantCount:   2,
			wantApplied: true,
		},
		{
			name: "valid reserved override default is applied",
			req: &UpdateUserSettingsRequest{
				SavedLayouts: ptr([]models.SavedLayout{
					{ID: "layout-override-default", Name: "Default", IsDefault: true, Layout: json.RawMessage(`{}`)},
				}),
			},
			wantCount:   1,
			wantApplied: true,
		},
		{
			name: "valid mixed custom and reserved override layouts are applied",
			req: &UpdateUserSettingsRequest{
				SavedLayouts: ptr([]models.SavedLayout{
					{ID: "layout-custom", Name: "Custom", Layout: json.RawMessage(`{}`)},
					{ID: "layout-override-plan", Name: "Plan Mode", Layout: json.RawMessage(`{}`)},
				}),
			},
			wantCount:   2,
			wantApplied: true,
		},
		{
			name: "valid mixed layouts allow one reserved override default",
			req: &UpdateUserSettingsRequest{
				SavedLayouts: ptr([]models.SavedLayout{
					{ID: "layout-custom", Name: "Custom", Layout: json.RawMessage(`{}`)},
					{ID: "layout-override-default", Name: "Default", IsDefault: true, Layout: json.RawMessage(`{}`)},
				}),
			},
			wantCount:   2,
			wantApplied: true,
		},
		{
			name: "exactly max layouts is accepted",
			req: &UpdateUserSettingsRequest{
				SavedLayouts: ptr(makeLayouts(maxSavedLayouts)),
			},
			wantCount:   maxSavedLayouts,
			wantApplied: true,
		},
		{
			name: "exceeding max layouts returns error",
			req: &UpdateUserSettingsRequest{
				SavedLayouts: ptr(makeLayouts(maxSavedLayouts + 1)),
			},
			wantErr: fmt.Sprintf("saved_layouts: max %d layouts allowed", maxSavedLayouts),
		},
		{
			name: "empty name returns error",
			req: &UpdateUserSettingsRequest{
				SavedLayouts: ptr([]models.SavedLayout{
					{ID: "l1", Name: "", Layout: json.RawMessage(`{}`)},
				}),
			},
			wantErr: "saved_layouts: layout name must not be empty",
		},
		{
			name: "whitespace-only name returns error",
			req: &UpdateUserSettingsRequest{
				SavedLayouts: ptr([]models.SavedLayout{
					{ID: "l1", Name: "   ", Layout: json.RawMessage(`{}`)},
				}),
			},
			wantErr: "saved_layouts: layout name must not be empty",
		},
		{
			name: "empty id returns error",
			req: &UpdateUserSettingsRequest{
				SavedLayouts: ptr([]models.SavedLayout{
					{ID: "", Name: "Layout", Layout: json.RawMessage(`{}`)},
				}),
			},
			wantErr: "saved_layouts: layout id must not be empty",
		},
		{
			name: "whitespace-only id returns error",
			req: &UpdateUserSettingsRequest{
				SavedLayouts: ptr([]models.SavedLayout{
					{ID: "   ", Name: "Layout", Layout: json.RawMessage(`{}`)},
				}),
			},
			wantErr: "saved_layouts: layout id must not be empty",
		},
		{
			name: "duplicate ids return error",
			req: &UpdateUserSettingsRequest{
				SavedLayouts: ptr([]models.SavedLayout{
					{ID: "l1", Name: "First", Layout: json.RawMessage(`{}`)},
					{ID: "l1", Name: "Second", Layout: json.RawMessage(`{}`)},
				}),
			},
			wantErr: `saved_layouts: duplicate layout id "l1"`,
		},
		{
			name: "mixed custom and reserved override defaults return error",
			req: &UpdateUserSettingsRequest{
				SavedLayouts: ptr([]models.SavedLayout{
					{ID: "layout-custom", Name: "Custom", IsDefault: true, Layout: json.RawMessage(`{}`)},
					{ID: "layout-override-default", Name: "Default", IsDefault: true, Layout: json.RawMessage(`{}`)},
				}),
			},
			wantErr: "saved_layouts: at most one default layout allowed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			settings := &models.UserSettings{
				SavedLayouts: makeLayouts(2), // pre-existing layouts
			}
			err := applySavedLayouts(settings, tt.req)

			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %q", tt.wantErr, err.Error())
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if !tt.wantApplied {
				// Nil request should leave settings unchanged
				if len(settings.SavedLayouts) != 2 {
					t.Fatalf("expected settings unchanged (2 layouts), got %d", len(settings.SavedLayouts))
				}
				return
			}

			if len(settings.SavedLayouts) != tt.wantCount {
				t.Fatalf("expected %d layouts, got %d", tt.wantCount, len(settings.SavedLayouts))
			}
		})
	}
}

// TestApplyWorkspaceAndTaskListPreferencesKanbanHiddenStepIDs table-tests kanban hidden step ID count and byte-budget limits.
func TestApplyWorkspaceAndTaskListPreferencesKanbanHiddenStepIDs(t *testing.T) {
	makeIDs := func(n int) []string {
		ids := make([]string, n)
		for i := range ids {
			ids[i] = fmt.Sprintf("step-%d", i)
		}
		return ids
	}
	makeWorkflows := func(n int) map[string][]string {
		hidden := make(map[string][]string, n)
		for i := 0; i < n; i++ {
			hidden[fmt.Sprintf("wf-%d", i)] = []string{"step-0"}
		}
		return hidden
	}

	tests := []struct {
		name    string
		req     *UpdateUserSettingsRequest
		wantErr string
	}{
		{
			name: "nil request is a no-op",
			req:  &UpdateUserSettingsRequest{KanbanHiddenStepIDs: nil},
		},
		{
			name: "empty map is accepted",
			req:  &UpdateUserSettingsRequest{KanbanHiddenStepIDs: ptr(map[string][]string{})},
		},
		{
			name: "a normal hidden set is applied",
			req: &UpdateUserSettingsRequest{
				KanbanHiddenStepIDs: ptr(map[string][]string{"wf-1": {"step-a", "step-b"}}),
			},
		},
		{
			name: "exactly max workflows is accepted",
			req: &UpdateUserSettingsRequest{
				KanbanHiddenStepIDs: ptr(makeWorkflows(maxKanbanHiddenStepWorkflows)),
			},
		},
		{
			name: "exceeding max workflows returns error",
			req: &UpdateUserSettingsRequest{
				KanbanHiddenStepIDs: ptr(makeWorkflows(maxKanbanHiddenStepWorkflows + 1)),
			},
			wantErr: fmt.Sprintf("kanban_hidden_step_ids: max %d workflows allowed", maxKanbanHiddenStepWorkflows),
		},
		{
			name: "exactly max ids per workflow is accepted",
			req: &UpdateUserSettingsRequest{
				KanbanHiddenStepIDs: ptr(map[string][]string{"wf-1": makeIDs(maxKanbanHiddenStepIDsPerWorkflow)}),
			},
		},
		{
			name: "exceeding max ids for one workflow returns error",
			req: &UpdateUserSettingsRequest{
				KanbanHiddenStepIDs: ptr(map[string][]string{"wf-1": makeIDs(maxKanbanHiddenStepIDsPerWorkflow + 1)}),
			},
			wantErr: fmt.Sprintf("kanban_hidden_step_ids[wf-1]: max %d step ids allowed", maxKanbanHiddenStepIDsPerWorkflow),
		},
		{
			name: "a handful of very long ids under the total byte budget is accepted",
			req: &UpdateUserSettingsRequest{
				KanbanHiddenStepIDs: ptr(map[string][]string{"wf-1": {strings.Repeat("a", 1024)}}),
			},
		},
		{
			name: "ids within count limits but exceeding the total byte budget returns error",
			req: &UpdateUserSettingsRequest{
				// 200 ids (at the count cap) x 400 bytes each = 80,000 bytes,
				// comfortably over maxKanbanHiddenStepIDsTotalBytes (64KB) —
				// this is the exact shape the count-only caps used to miss:
				// a few oversized strings smuggled past a per-entry count check.
				KanbanHiddenStepIDs: ptr(map[string][]string{
					"wf-1": func() []string {
						ids := make([]string, maxKanbanHiddenStepIDsPerWorkflow)
						for i := range ids {
							ids[i] = strings.Repeat("a", 400)
						}
						return ids
					}(),
				}),
			},
			wantErr: fmt.Sprintf("kanban_hidden_step_ids: max %d bytes allowed", maxKanbanHiddenStepIDsTotalBytes),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			settings := &models.UserSettings{
				KanbanHiddenStepIDs: map[string][]string{"wf-existing": {"step-x"}},
			}
			err := applyWorkspaceAndTaskListPreferences(settings, tt.req)

			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %q", tt.wantErr, err.Error())
				}
				// Rejected write must not clobber the existing value.
				if len(settings.KanbanHiddenStepIDs) != 1 {
					t.Fatalf("expected settings unchanged on error, got %v", settings.KanbanHiddenStepIDs)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.req.KanbanHiddenStepIDs == nil {
				if len(settings.KanbanHiddenStepIDs) != 1 {
					t.Fatalf("expected settings unchanged, got %v", settings.KanbanHiddenStepIDs)
				}
				return
			}

			if !reflect.DeepEqual(settings.KanbanHiddenStepIDs, *tt.req.KanbanHiddenStepIDs) {
				t.Fatalf("expected %v, got %v", *tt.req.KanbanHiddenStepIDs, settings.KanbanHiddenStepIDs)
			}
		})
	}
}

func TestApplyWorkspaceAndTaskListPreferencesAutoHideWorkflowIDs(t *testing.T) {
	makeWorkflowIDs := func(n int) []string {
		ids := make([]string, n)
		for i := range ids {
			ids[i] = fmt.Sprintf("wf-%d", i)
		}
		return ids
	}
	duplicateWorkflowIDs := make([]string, maxWorkflowIDsWithAutoHideEmptySteps+1)
	for i := range duplicateWorkflowIDs {
		duplicateWorkflowIDs[i] = "wf-duplicate"
	}

	tests := []struct {
		name    string
		value   []string
		want    []string
		wantErr string
	}{
		{name: "empty list clears the preference", value: []string{}, want: []string{}},
		{
			name:  "normalizes duplicate workflow ids",
			value: []string{"wf-b", "wf-a", "wf-b"},
			want:  []string{"wf-a", "wf-b"},
		},
		{
			name:  "applies the count limit after deduplication",
			value: duplicateWorkflowIDs,
			want:  []string{"wf-duplicate"},
		},
		{
			name:    "rejects too many workflow ids",
			value:   makeWorkflowIDs(maxWorkflowIDsWithAutoHideEmptySteps + 1),
			wantErr: fmt.Sprintf("workflow_ids_with_auto_hide_empty_steps: max %d workflow ids allowed", maxWorkflowIDsWithAutoHideEmptySteps),
		},
		{
			name:    "rejects an oversized preference",
			value:   []string{strings.Repeat("x", maxUserPreferenceBlobBytes+1)},
			wantErr: fmt.Sprintf("workflow_ids_with_auto_hide_empty_steps: max %d bytes allowed", maxUserPreferenceBlobBytes),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			settings := &models.UserSettings{
				WorkflowIDsWithAutoHideEmptySteps: []string{"wf-existing"},
			}
			err := applyWorkspaceAndTaskListPreferences(settings, &UpdateUserSettingsRequest{
				WorkflowIDsWithAutoHideEmptySteps: &tt.value,
			})
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want error containing %q", err, tt.wantErr)
				}
				if !reflect.DeepEqual(settings.WorkflowIDsWithAutoHideEmptySteps, []string{"wf-existing"}) {
					t.Fatalf("rejected update changed settings to %#v", settings.WorkflowIDsWithAutoHideEmptySteps)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(settings.WorkflowIDsWithAutoHideEmptySteps, tt.want) {
				t.Fatalf("WorkflowIDsWithAutoHideEmptySteps = %#v, want %#v", settings.WorkflowIDsWithAutoHideEmptySteps, tt.want)
			}
		})
	}
}

// makeSidebarViews builds n SidebarView fixtures with distinct IDs and names.
func makeSidebarViews(n int) []models.SidebarView {
	views := make([]models.SidebarView, n)
	for i := range views {
		views[i] = models.SidebarView{
			ID:              fmt.Sprintf("view-%d", i),
			Name:            fmt.Sprintf("View %d", i),
			Filters:         []models.SidebarViewClause{},
			Sort:            models.SidebarViewSort{Key: "state", Direction: "asc"},
			Group:           "repository",
			CollapsedGroups: []string{},
		}
	}
	return views
}

// TestApplySidebarViews table-tests sidebar view count limits and validation errors.
func TestApplySidebarViews(t *testing.T) {
	tests := []struct {
		name        string
		req         *UpdateUserSettingsRequest
		wantErr     string
		wantCount   int
		wantApplied bool
	}{
		{
			name:        "nil request is a no-op",
			req:         &UpdateUserSettingsRequest{SidebarViews: nil},
			wantApplied: false,
		},
		{
			name:        "empty slice restores the canonical default",
			req:         &UpdateUserSettingsRequest{SidebarViews: ptr([]models.SidebarView{})},
			wantCount:   1,
			wantApplied: true,
		},
		{
			name:        "valid single view is applied",
			req:         &UpdateUserSettingsRequest{SidebarViews: ptr(makeSidebarViews(1))},
			wantCount:   1,
			wantApplied: true,
		},
		{
			name:        "exactly max views is accepted",
			req:         &UpdateUserSettingsRequest{SidebarViews: ptr(makeSidebarViews(maxSidebarViews))},
			wantCount:   maxSidebarViews,
			wantApplied: true,
		},
		{
			name:    "exceeding max views returns error",
			req:     &UpdateUserSettingsRequest{SidebarViews: ptr(makeSidebarViews(maxSidebarViews + 1))},
			wantErr: fmt.Sprintf("sidebar_views: max %d views allowed", maxSidebarViews),
		},
		{
			name: "empty id returns error",
			req: &UpdateUserSettingsRequest{SidebarViews: ptr([]models.SidebarView{
				{ID: "", Name: "X"},
			})},
			wantErr: "sidebar_views: view id must not be empty",
		},
		{
			name: "empty name returns error",
			req: &UpdateUserSettingsRequest{SidebarViews: ptr([]models.SidebarView{
				{ID: "v1", Name: ""},
			})},
			wantErr: "sidebar_views: view name must not be empty",
		},
		{
			name: "duplicate ids return error",
			req: &UpdateUserSettingsRequest{SidebarViews: ptr([]models.SidebarView{
				{ID: "v1", Name: "A"},
				{ID: "v1", Name: "B"},
			})},
			wantErr: `sidebar_views: duplicate view id "v1"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			settings := &models.UserSettings{SidebarViews: makeSidebarViews(2)}
			err := applySidebarViews(settings, tt.req)

			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %q", tt.wantErr, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !tt.wantApplied {
				if len(settings.SidebarViews) != 2 {
					t.Fatalf("expected settings unchanged (2 views), got %d", len(settings.SidebarViews))
				}
				return
			}
			if len(settings.SidebarViews) != tt.wantCount {
				t.Fatalf("expected %d views, got %d", tt.wantCount, len(settings.SidebarViews))
			}
		})
	}
}

// TestApplySidebarViewState verifies active view and draft application, clearing, and validation.
func TestApplySidebarViewState(t *testing.T) {
	t.Run("nil fields leave active view and draft unchanged", func(t *testing.T) {
		settings := &models.UserSettings{
			SidebarActiveViewID: "view-existing",
			SidebarDraft: &models.SidebarViewDraft{
				BaseViewID: "view-existing",
				Filters:    []models.SidebarViewClause{},
				Sort:       models.SidebarViewSort{Key: "state", Direction: "asc"},
				Group:      "state",
			},
		}
		if err := applySidebarViewState(settings, &UpdateUserSettingsRequest{}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if settings.SidebarActiveViewID != "view-existing" {
			t.Fatalf("expected active view unchanged, got %q", settings.SidebarActiveViewID)
		}
		if settings.SidebarDraft == nil || settings.SidebarDraft.BaseViewID != "view-existing" {
			t.Fatalf("expected draft unchanged, got %+v", settings.SidebarDraft)
		}
	})

	t.Run("applies active view and draft", func(t *testing.T) {
		draft := &models.SidebarViewDraft{
			BaseViewID: "view-1",
			Filters: []models.SidebarViewClause{{
				ID:        "clause-1",
				Dimension: "titleMatch",
				Op:        "matches",
				Value:     json.RawMessage(`"bug"`),
			}},
			Sort:  models.SidebarViewSort{Key: "updatedAt", Direction: "desc"},
			Group: "workflow",
		}
		settings := &models.UserSettings{SidebarViews: []models.SidebarView{{ID: "view-1", Name: "View 1"}}}
		req := &UpdateUserSettingsRequest{
			SidebarActiveViewID: ptr("view-1"),
			SidebarDraft:        &draft,
		}
		if err := applySidebarViewState(settings, req); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if settings.SidebarActiveViewID != "view-1" {
			t.Fatalf("expected active view view-1, got %q", settings.SidebarActiveViewID)
		}
		if settings.SidebarDraft == nil || settings.SidebarDraft.Group != "workflow" {
			t.Fatalf("expected draft to be applied, got %+v", settings.SidebarDraft)
		}
	})

	t.Run("clears draft when null is provided", func(t *testing.T) {
		settings := &models.UserSettings{
			SidebarDraft: &models.SidebarViewDraft{BaseViewID: "view-1"},
		}
		req := &UpdateUserSettingsRequest{SidebarDraft: ptr((*models.SidebarViewDraft)(nil))}
		if err := applySidebarViewState(settings, req); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if settings.SidebarDraft != nil {
			t.Fatalf("expected draft cleared, got %+v", settings.SidebarDraft)
		}
	})

	t.Run("rejects whitespace-only active view id", func(t *testing.T) {
		req := &UpdateUserSettingsRequest{SidebarActiveViewID: ptr("  ")}
		if err := applySidebarViewState(&models.UserSettings{}, req); err == nil {
			t.Fatal("expected validation error, got nil")
		}
	})

	t.Run("rejects active view id missing from saved views", func(t *testing.T) {
		settings := &models.UserSettings{SidebarViews: []models.SidebarView{{ID: "view-1", Name: "View 1"}}}
		req := &UpdateUserSettingsRequest{SidebarActiveViewID: ptr("missing")}
		if err := applySidebarViewState(settings, req); err == nil {
			t.Fatal("expected validation error, got nil")
		}
		if settings.SidebarActiveViewID != "" {
			t.Fatalf("expected active view unchanged, got %q", settings.SidebarActiveViewID)
		}
	})
}

// TestApplyUserPreferenceBlobs verifies blob preferences are patched while task-create last-used is preserved.
func TestApplyUserPreferenceBlobs(t *testing.T) {
	settings := &models.UserSettings{
		TaskCreateLastUsed: models.TaskCreateLastUsed{
			RepositoryID:      "repo-1",
			Branch:            "main",
			AgentProfileID:    "agent-1",
			ExecutorProfileID: "exec-1",
		},
	}
	patch := models.TaskCreateLastUsed{Branch: "feature"}

	if err := applyUserPreferenceBlobs(settings, &UpdateUserSettingsRequest{
		TaskCreateLastUsed: &patch,
		GitHubSavedPresets: rawPatch(json.RawMessage(`[{"id":"p1"}]`)),
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if settings.TaskCreateLastUsed.RepositoryID != "repo-1" {
		t.Fatalf("expected repository id to be preserved, got %q", settings.TaskCreateLastUsed.RepositoryID)
	}
	if settings.TaskCreateLastUsed.Branch != "main" {
		t.Fatalf("expected task-create last-used to stay unchanged, got %q", settings.TaskCreateLastUsed.Branch)
	}
	if string(settings.GitHubSavedPresets) != `[{"id":"p1"}]` {
		t.Fatalf("expected GitHub presets to apply, got %s", string(settings.GitHubSavedPresets))
	}
}

// TestUpdateUserSettingsCombinesSettingsAndTaskCreatePatch verifies UpdateUserSettings folds the task-create patch into a single preserving write.
func TestUpdateUserSettingsCombinesSettingsAndTaskCreatePatch(t *testing.T) {
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatalf("logger.NewFromZap: %v", err)
	}
	patch := models.TaskCreateLastUsed{
		RepositoryID:   "repo-2",
		Branch:         "feature",
		AgentProfileID: "agent-2",
	}
	updatedSettings := &models.UserSettings{
		UserID:           store.DefaultUserID,
		TerminalFontSize: 16,
		TaskCreateLastUsed: models.TaskCreateLastUsed{
			RepositoryID:   "repo-2",
			Branch:         "feature",
			AgentProfileID: "agent-2",
		},
	}
	repo := &recordingUserRepository{
		getSettings:        &models.UserSettings{UserID: store.DefaultUserID},
		preservingSettings: updatedSettings,
		updateSettings:     updatedSettings,
	}
	eventBus := &recordingEventBus{}
	svc := NewService(repo, eventBus, log)

	settings, err := svc.UpdateUserSettings(context.Background(), &UpdateUserSettingsRequest{
		TerminalFontSize:   ptr(16),
		TaskCreateLastUsed: &patch,
	})
	if err != nil {
		t.Fatalf("UpdateUserSettings: %v", err)
	}

	if settings != updatedSettings {
		t.Fatalf("expected returned settings from preserving writer, got %+v", settings)
	}
	if repo.upsertUserSettingsPreservingLastUsedCalls != 1 {
		t.Fatalf("expected one preserving settings write, got %d", repo.upsertUserSettingsPreservingLastUsedCalls)
	}
	if repo.updateCalls != 0 {
		t.Fatalf("expected task-create patch to be folded into settings write, got %d separate update calls", repo.updateCalls)
	}
	if repo.preservingPatch == nil || !reflect.DeepEqual(*repo.preservingPatch, patch) {
		t.Fatalf("expected preserving write patch %+v, got %+v", patch, repo.preservingPatch)
	}
	if len(eventBus.publishedEvents) != 1 {
		t.Fatalf("expected one settings event, got %d", len(eventBus.publishedEvents))
	}
}

// TestPublishUserSettingsEventIncludesArchiveConfirmation verifies the settings event carries confirm_task_archive.
func TestPublishUserSettingsEventIncludesArchiveConfirmation(t *testing.T) {
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatalf("logger.NewFromZap: %v", err)
	}
	eventBus := &recordingEventBus{}
	svc := NewService(&recordingUserRepository{}, eventBus, log)
	svc.publishUserSettingsEvent(context.Background(), &models.UserSettings{ConfirmTaskArchive: false})

	if len(eventBus.publishedEvents) != 1 {
		t.Fatalf("expected one settings event, got %d", len(eventBus.publishedEvents))
	}
	eventData, ok := eventBus.publishedEvents[0].Data.(map[string]interface{})
	if !ok {
		t.Fatalf("expected event data map, got %T", eventBus.publishedEvents[0].Data)
	}
	if confirmTaskArchive, ok := eventData["confirm_task_archive"].(bool); !ok || confirmTaskArchive {
		t.Fatalf("confirm_task_archive = %#v, want false", eventData["confirm_task_archive"])
	}
}

// TestPublishUserSettingsEventIncludesTasksListShowDetails verifies the settings event carries tasks_list_show_details.
func TestPublishUserSettingsEventIncludesTasksListShowDetails(t *testing.T) {
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatalf("logger.NewFromZap: %v", err)
	}
	eventBus := &recordingEventBus{}
	svc := NewService(&recordingUserRepository{}, eventBus, log)
	svc.publishUserSettingsEvent(context.Background(), &models.UserSettings{TasksListShowDetails: true})

	if len(eventBus.publishedEvents) != 1 {
		t.Fatalf("expected one settings event, got %d", len(eventBus.publishedEvents))
	}
	eventData, ok := eventBus.publishedEvents[0].Data.(map[string]interface{})
	if !ok {
		t.Fatalf("expected event data map, got %T", eventBus.publishedEvents[0].Data)
	}
	if showDetails, ok := eventData["tasks_list_show_details"].(bool); !ok || !showDetails {
		t.Fatalf("tasks_list_show_details = %#v, want true", eventData["tasks_list_show_details"])
	}
}

// TestPublishUserSettingsEventIncludesNormalizedMCPTaskAgentProfileDefault verifies the event normalizes unknown profile defaults to current_task.
func TestPublishUserSettingsEventIncludesNormalizedMCPTaskAgentProfileDefault(t *testing.T) {
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatalf("logger.NewFromZap: %v", err)
	}
	eventBus := &recordingEventBus{}
	svc := NewService(&recordingUserRepository{}, eventBus, log)
	svc.publishUserSettingsEvent(context.Background(), &models.UserSettings{
		MCPTaskAgentProfileDefault: "future_value",
	})

	if len(eventBus.publishedEvents) != 1 {
		t.Fatalf("expected one settings event, got %d", len(eventBus.publishedEvents))
	}
	eventData, ok := eventBus.publishedEvents[0].Data.(map[string]interface{})
	if !ok {
		t.Fatalf("expected event data map, got %T", eventBus.publishedEvents[0].Data)
	}
	if got := eventData["mcp_task_agent_profile_default"]; got != models.MCPTaskAgentProfileDefaultCurrentTask {
		t.Fatalf("mcp_task_agent_profile_default = %#v, want current_task", got)
	}
}

// TestPublishUserSettingsEventIncludesNormalizedStartupPage verifies the event normalizes unknown startup pages to task_overview.
func TestPublishUserSettingsEventIncludesNormalizedStartupPage(t *testing.T) {
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatalf("logger.NewFromZap: %v", err)
	}
	eventBus := &recordingEventBus{}
	svc := NewService(&recordingUserRepository{}, eventBus, log)
	svc.publishUserSettingsEvent(context.Background(), &models.UserSettings{StartupPage: "future_value"})

	eventData, ok := eventBus.publishedEvents[0].Data.(map[string]interface{})
	if !ok {
		t.Fatalf("expected event data map, got %T", eventBus.publishedEvents[0].Data)
	}
	if got := eventData["startup_page"]; got != models.StartupPageTaskOverview {
		t.Fatalf("startup_page = %#v, want task_overview", got)
	}
}

// TestPublishUserSettingsEventIncludesAppStatusBarOrder verifies the settings event carries the app status bar order.
func TestPublishUserSettingsEventIncludesAppStatusBarOrder(t *testing.T) {
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatalf("logger.NewFromZap: %v", err)
	}
	eventBus := &recordingEventBus{}
	svc := NewService(&recordingUserRepository{}, eventBus, log)
	want := models.AppStatusBarOrder{
		LeftItemIDs:  []string{"left"},
		RightItemIDs: []string{"right"},
	}
	svc.publishUserSettingsEvent(context.Background(), &models.UserSettings{AppStatusBarOrder: want})

	eventData, ok := eventBus.publishedEvents[0].Data.(map[string]interface{})
	if !ok {
		t.Fatalf("expected event data map, got %T", eventBus.publishedEvents[0].Data)
	}
	if got, ok := eventData["app_status_bar_order"].(models.AppStatusBarOrder); !ok || fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("app_status_bar_order = %#v, want %#v", eventData["app_status_bar_order"], want)
	}
}

// TestPublishUserSettingsEventIncludesLspStatusLocation verifies the settings event carries lsp_status_location.
func TestPublishUserSettingsEventIncludesLspStatusLocation(t *testing.T) {
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatalf("logger.NewFromZap: %v", err)
	}
	eventBus := &recordingEventBus{}
	svc := NewService(&recordingUserRepository{}, eventBus, log)
	svc.publishUserSettingsEvent(context.Background(), &models.UserSettings{
		LspStatusLocation: models.LspStatusLocationStatusBar,
	})

	eventData, ok := eventBus.publishedEvents[0].Data.(map[string]interface{})
	if !ok {
		t.Fatalf("expected event data map, got %T", eventBus.publishedEvents[0].Data)
	}
	if got := eventData["lsp_status_location"]; got != models.LspStatusLocationStatusBar {
		t.Fatalf("lsp_status_location = %#v, want status_bar", got)
	}
}

// TestUpdateUserSettingsRejectsInvalidMCPTaskAgentProfileDefaultWithoutPersisting verifies an invalid profile default is rejected without persisting or publishing.
func TestUpdateUserSettingsRejectsInvalidMCPTaskAgentProfileDefaultWithoutPersisting(t *testing.T) {
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatalf("logger.NewFromZap: %v", err)
	}
	repo := &recordingUserRepository{getSettings: &models.UserSettings{
		MCPTaskAgentProfileDefault: models.MCPTaskAgentProfileDefaultWorkspaceDefault,
	}}
	eventBus := &recordingEventBus{}
	svc := NewService(repo, eventBus, log)

	_, err = svc.UpdateUserSettings(context.Background(), &UpdateUserSettingsRequest{
		MCPTaskAgentProfileDefault: ptr("unknown"),
	})
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("UpdateUserSettings error = %v, want validation error", err)
	}
	if repo.upsertUserSettingsPreservingLastUsedCalls != 0 {
		t.Fatalf("persist calls = %d, want 0", repo.upsertUserSettingsPreservingLastUsedCalls)
	}
	if repo.getSettings.MCPTaskAgentProfileDefault != models.MCPTaskAgentProfileDefaultWorkspaceDefault {
		t.Fatalf("saved preference = %q, want workspace_default", repo.getSettings.MCPTaskAgentProfileDefault)
	}
	if len(eventBus.publishedEvents) != 0 {
		t.Fatalf("published events = %d, want 0", len(eventBus.publishedEvents))
	}
}

// TestClearDefaultEditorIDPreservesTaskCreateLastUsed verifies clearing the default editor preserves task-create last-used in the write and event.
func TestClearDefaultEditorIDPreservesTaskCreateLastUsed(t *testing.T) {
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatalf("logger.NewFromZap: %v", err)
	}
	updatedSettings := &models.UserSettings{
		UserID:          store.DefaultUserID,
		DefaultEditorID: "",
		TaskCreateLastUsed: models.TaskCreateLastUsed{
			RepositoryID: "repo-2",
			Branch:       "feature",
		},
	}
	repo := &recordingUserRepository{
		getSettings: &models.UserSettings{
			UserID:          store.DefaultUserID,
			DefaultEditorID: "editor-1",
			TaskCreateLastUsed: models.TaskCreateLastUsed{
				RepositoryID: "repo-1",
				Branch:       "main",
			},
		},
		preservingSettings: updatedSettings,
	}
	eventBus := &recordingEventBus{}
	svc := NewService(repo, eventBus, log)

	if err := svc.ClearDefaultEditorID(context.Background(), "editor-1"); err != nil {
		t.Fatalf("ClearDefaultEditorID: %v", err)
	}

	if repo.upsertUserSettingsPreservingLastUsedCalls != 1 {
		t.Fatalf("expected preserving settings upsert, got %d calls", repo.upsertUserSettingsPreservingLastUsedCalls)
	}
	if len(eventBus.publishedEvents) != 1 {
		t.Fatalf("expected one settings event, got %d", len(eventBus.publishedEvents))
	}
	data, ok := eventBus.publishedEvents[0].Data.(map[string]interface{})
	if !ok {
		t.Fatalf("expected event data map, got %T", eventBus.publishedEvents[0].Data)
	}
	if !reflect.DeepEqual(data["task_create_last_used"], updatedSettings.TaskCreateLastUsed) {
		t.Fatalf("expected event to include preserved task-create state %+v, got %+v", updatedSettings.TaskCreateLastUsed, data["task_create_last_used"])
	}
}

// TestRecordTaskCreateLastUsed verifies RecordTaskCreateLastUsed skips empty patches and updates and publishes non-empty ones.
func TestRecordTaskCreateLastUsed(t *testing.T) {
	newTestService := func(repo *recordingUserRepository, eventBus *recordingEventBus) *Service {
		log, err := logger.NewFromZap(zap.NewNop())
		if err != nil {
			t.Fatalf("logger.NewFromZap: %v", err)
		}
		return NewService(repo, eventBus, log)
	}

	t.Run("empty patch skips repo update and publish", func(t *testing.T) {
		repo := &recordingUserRepository{}
		eventBus := &recordingEventBus{}
		svc := newTestService(repo, eventBus)

		if err := svc.RecordTaskCreateLastUsed(context.Background(), models.TaskCreateLastUsed{}); err != nil {
			t.Fatalf("RecordTaskCreateLastUsed: %v", err)
		}
		if repo.updateCalls != 0 {
			t.Fatalf("expected no repo update, got %d", repo.updateCalls)
		}
		if len(eventBus.publishedEvents) != 0 {
			t.Fatalf("expected no settings event, got %d", len(eventBus.publishedEvents))
		}
	})

	t.Run("non-empty patch updates repo and publishes settings event", func(t *testing.T) {
		patch := models.TaskCreateLastUsed{
			RepositoryID:      "repo-1",
			Branch:            "feature",
			AgentProfileID:    "agent-1",
			ExecutorProfileID: "exec-1",
		}
		updatedSettings := &models.UserSettings{
			UserID:    store.DefaultUserID,
			Revision:  42,
			UpdatedAt: time.Unix(123, 0).UTC(),
			TaskCreateLastUsed: models.TaskCreateLastUsed{
				RepositoryID: "repo-1",
				Branch:       "feature",
			},
		}
		repo := &recordingUserRepository{updateSettings: updatedSettings}
		eventBus := &recordingEventBus{}
		svc := newTestService(repo, eventBus)

		if err := svc.RecordTaskCreateLastUsed(context.Background(), patch); err != nil {
			t.Fatalf("RecordTaskCreateLastUsed: %v", err)
		}
		if repo.updateCalls != 1 {
			t.Fatalf("expected one update call, got %d", repo.updateCalls)
		}
		if repo.updateUserID != store.DefaultUserID {
			t.Fatalf("expected update user id %q, got %q", store.DefaultUserID, repo.updateUserID)
		}
		if !reflect.DeepEqual(repo.updatePatch, patch) {
			t.Fatalf("expected patch %+v, got %+v", patch, repo.updatePatch)
		}
		if len(eventBus.publishedEvents) != 1 {
			t.Fatalf("expected one published event, got %d", len(eventBus.publishedEvents))
		}
		if eventBus.publishedSubjects[0] != events.UserSettingsUpdated {
			t.Fatalf("expected subject %q, got %q", events.UserSettingsUpdated, eventBus.publishedSubjects[0])
		}
		published := eventBus.publishedEvents[0]
		if published.Type != events.UserSettingsUpdated {
			t.Fatalf("expected event type %q, got %q", events.UserSettingsUpdated, published.Type)
		}
		data, ok := published.Data.(map[string]interface{})
		if !ok {
			t.Fatalf("expected event data map, got %T", published.Data)
		}
		if !reflect.DeepEqual(data["task_create_last_used"], updatedSettings.TaskCreateLastUsed) {
			t.Fatalf("expected event task-create state %+v, got %+v", updatedSettings.TaskCreateLastUsed, data["task_create_last_used"])
		}
		if got := data["revision"]; got != int64(42) {
			t.Fatalf("revision = %#v, want 42", got)
		}
	})

	t.Run("workflow-only patch updates repo and publishes settings event", func(t *testing.T) {
		patch := models.TaskCreateLastUsed{
			WorkflowIDsByWorkspace: map[string]string{"workspace-1": "workflow-1"},
		}
		updatedSettings := &models.UserSettings{
			UserID: store.DefaultUserID,
			TaskCreateLastUsed: models.TaskCreateLastUsed{
				WorkflowIDsByWorkspace: map[string]string{"workspace-1": "workflow-1"},
			},
		}
		repo := &recordingUserRepository{updateSettings: updatedSettings}
		eventBus := &recordingEventBus{}
		svc := newTestService(repo, eventBus)

		if err := svc.RecordTaskCreateLastUsed(context.Background(), patch); err != nil {
			t.Fatalf("RecordTaskCreateLastUsed: %v", err)
		}
		if repo.updateCalls != 1 || !reflect.DeepEqual(repo.updatePatch, patch) {
			t.Fatalf("expected workflow-only patch to update repo once: calls=%d patch=%+v", repo.updateCalls, repo.updatePatch)
		}
		if len(eventBus.publishedEvents) != 1 {
			t.Fatalf("expected one settings event, got %d", len(eventBus.publishedEvents))
		}
	})

	t.Run("repo error is propagated without publishing", func(t *testing.T) {
		repoErr := errors.New("update failed")
		repo := &recordingUserRepository{updateErr: repoErr}
		eventBus := &recordingEventBus{}
		svc := newTestService(repo, eventBus)

		err := svc.RecordTaskCreateLastUsed(context.Background(), models.TaskCreateLastUsed{Branch: "feature"})
		if !errors.Is(err, repoErr) {
			t.Fatalf("expected repo error, got %v", err)
		}
		if repo.updateCalls != 1 {
			t.Fatalf("expected one update call, got %d", repo.updateCalls)
		}
		if len(eventBus.publishedEvents) != 0 {
			t.Fatalf("expected no events, got %d", len(eventBus.publishedEvents))
		}
	})
}

// TestApplyUserPreferenceBlobsValidation verifies blob preferences accept arrays, objects, and null and reject scalars and oversized blobs.
func TestApplyUserPreferenceBlobsValidation(t *testing.T) {
	t.Run("accepts arrays objects and null", func(t *testing.T) {
		settings := &models.UserSettings{}
		req := &UpdateUserSettingsRequest{
			JiraSavedViews:            rawPatch(json.RawMessage(`[]`)),
			GitHubDefaultQueryPresets: rawPatch(json.RawMessage(`{"pr":[],"issue":[]}`)),
			GitLabSavedPresets:        rawClear(),
		}
		if err := applyUserPreferenceBlobs(settings, req); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("rejects scalar blobs", func(t *testing.T) {
		settings := &models.UserSettings{}
		req := &UpdateUserSettingsRequest{GitHubSavedPresets: rawPatch(json.RawMessage(`"bad"`))}
		err := applyUserPreferenceBlobs(settings, req)
		if err == nil || !strings.Contains(err.Error(), "github_saved_presets") {
			t.Fatalf("expected github_saved_presets validation error, got %v", err)
		}
	})

	t.Run("rejects oversized blobs", func(t *testing.T) {
		settings := &models.UserSettings{}
		raw := json.RawMessage(`["` + strings.Repeat("x", maxUserPreferenceBlobBytes) + `"]`)
		req := &UpdateUserSettingsRequest{JiraSavedViews: rawPatch(raw)}
		err := applyUserPreferenceBlobs(settings, req)
		if err == nil || !strings.Contains(err.Error(), "max") {
			t.Fatalf("expected size validation error, got %v", err)
		}
	})

	t.Run("explicit null clears blob", func(t *testing.T) {
		settings := &models.UserSettings{JiraSavedViews: json.RawMessage(`[{"id":"view"}]`)}
		req := &UpdateUserSettingsRequest{JiraSavedViews: rawClear()}
		if err := applyUserPreferenceBlobs(settings, req); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if settings.JiraSavedViews != nil {
			t.Fatalf("expected explicit null to clear blob, got %s", string(settings.JiraSavedViews))
		}
	})

}

// TestAzureDevOpsBrowsePreferencesArePatched verifies Azure DevOps browse preferences are patched, persisted, and published.
func TestAzureDevOpsBrowsePreferencesArePatched(t *testing.T) {
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	preferences := json.RawMessage(`{"workspace-a":{"mode":"board","projectId":"project-a"}}`)
	updatedSettings := &models.UserSettings{
		UserID:                       store.DefaultUserID,
		AzureDevOpsBrowsePreferences: preferences,
	}
	repo := &recordingUserRepository{
		getSettings:        &models.UserSettings{UserID: store.DefaultUserID},
		preservingSettings: updatedSettings,
	}
	eventBus := &recordingEventBus{}
	settings, err := NewService(repo, eventBus, log).UpdateUserSettings(context.Background(), &UpdateUserSettingsRequest{
		AzureDevOpsBrowsePreferences: rawPatch(preferences),
	})
	if err != nil {
		t.Fatalf("update Azure DevOps preferences: %v", err)
	}
	if string(settings.AzureDevOpsBrowsePreferences) != string(preferences) {
		t.Fatalf("AzureDevOpsBrowsePreferences = %s, want %s", settings.AzureDevOpsBrowsePreferences, preferences)
	}
	if repo.preservingInput == nil || string(repo.preservingInput.AzureDevOpsBrowsePreferences) != string(preferences) {
		t.Fatalf("persisted AzureDevOpsBrowsePreferences = %+v, want %s", repo.preservingInput, preferences)
	}
	if len(eventBus.publishedEvents) != 1 {
		t.Fatalf("settings events = %d, want 1", len(eventBus.publishedEvents))
	}
	eventData, ok := eventBus.publishedEvents[0].Data.(map[string]interface{})
	if !ok {
		t.Fatalf("settings event data = %T, want map", eventBus.publishedEvents[0].Data)
	}
	if got, ok := eventData["azure_devops_browse_preferences"].(json.RawMessage); !ok || string(got) != string(preferences) {
		t.Fatalf("event AzureDevOpsBrowsePreferences = %s, want %s", got, preferences)
	}
}

type recordingUserRepository struct {
	getUserCalls                              int
	getDefaultUserCalls                       int
	getUserSettingsCalls                      int
	getSettingsUserID                         string
	upsertUserSettingsPreservingLastUsedCalls int
	updateCalls                               int
	updateUserID                              string
	updatePatch                               models.TaskCreateLastUsed
	updateSettings                            *models.UserSettings
	updateErr                                 error
	getSettings                               *models.UserSettings
	getErr                                    error
	preservingSettings                        *models.UserSettings
	preservingInput                           *models.UserSettings
	preservingPatch                           *models.TaskCreateLastUsed
	preservingErr                             error
	closeCalls                                int
}

// GetUser records the call and returns an unexpected-call error.
func (r *recordingUserRepository) GetUser(context.Context, string) (*models.User, error) {
	r.getUserCalls++
	return nil, errors.New("unexpected GetUser call")
}

// GetDefaultUser records the call and returns an unexpected-call error.
func (r *recordingUserRepository) GetDefaultUser(context.Context) (*models.User, error) {
	r.getDefaultUserCalls++
	return nil, errors.New("unexpected GetDefaultUser call")
}

// GetUserSettings records the call and returns the configured settings or error.
func (r *recordingUserRepository) GetUserSettings(_ context.Context, userID string) (*models.UserSettings, error) {
	r.getUserSettingsCalls++
	r.getSettingsUserID = userID
	if r.getErr != nil {
		return nil, r.getErr
	}
	if r.getSettings != nil {
		return r.getSettings, nil
	}
	return nil, errors.New("unexpected GetUserSettings call")
}

// UpsertUserSettingsPreservingTaskCreateLastUsed records the preserving-write inputs and returns the configured result or error.
func (r *recordingUserRepository) UpsertUserSettingsPreservingTaskCreateLastUsed(
	_ context.Context,
	settings *models.UserSettings,
	patch *models.TaskCreateLastUsed,
	_ int64,
) (*models.UserSettings, error) {
	r.upsertUserSettingsPreservingLastUsedCalls++
	settingsCopy := *settings
	r.preservingInput = &settingsCopy
	if patch != nil {
		patchCopy := *patch
		r.preservingPatch = &patchCopy
	}
	if r.preservingErr != nil {
		return nil, r.preservingErr
	}
	if r.preservingSettings != nil {
		return r.preservingSettings, nil
	}
	return nil, errors.New("unexpected UpsertUserSettingsPreservingTaskCreateLastUsed call")
}

// UpdateTaskCreateLastUsed records the update inputs and returns the configured result or error.
func (r *recordingUserRepository) UpdateTaskCreateLastUsed(
	_ context.Context,
	userID string,
	patch models.TaskCreateLastUsed,
) (*models.UserSettings, error) {
	r.updateCalls++
	r.updateUserID = userID
	r.updatePatch = patch
	if r.updateErr != nil {
		return nil, r.updateErr
	}
	return r.updateSettings, nil
}

// Close records the close call.
func (r *recordingUserRepository) Close() error {
	r.closeCalls++
	return nil
}

type recordingEventBus struct {
	publishedSubjects []string
	publishedEvents   []*bus.Event
}

// Publish records the published subject and event.
func (b *recordingEventBus) Publish(_ context.Context, subject string, event *bus.Event) error {
	b.publishedSubjects = append(b.publishedSubjects, subject)
	b.publishedEvents = append(b.publishedEvents, event)
	return nil
}

// Subscribe returns an unexpected-call error.
func (b *recordingEventBus) Subscribe(string, bus.EventHandler) (bus.Subscription, error) {
	return nil, errors.New("unexpected Subscribe call")
}

// QueueSubscribe returns an unexpected-call error.
func (b *recordingEventBus) QueueSubscribe(string, string, bus.EventHandler) (bus.Subscription, error) {
	return nil, errors.New("unexpected QueueSubscribe call")
}

// Request returns an unexpected-call error.
func (b *recordingEventBus) Request(context.Context, string, *bus.Event, time.Duration) (*bus.Event, error) {
	return nil, errors.New("unexpected Request call")
}

// Close is a no-op.
func (b *recordingEventBus) Close() {}

// IsConnected reports the fake bus as connected.
func (b *recordingEventBus) IsConnected() bool {
	return true
}
