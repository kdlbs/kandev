package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

func ptr[T any](v T) *T { return &v }

func rawPatch(v json.RawMessage) **json.RawMessage {
	return ptr(ptr(v))
}

func rawClear() **json.RawMessage {
	return ptr((*json.RawMessage)(nil))
}

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

func TestApplyBasicSettingsUtilityAgentProfileID(t *testing.T) {
	t.Run("omission preserves saved value", func(t *testing.T) {
		settings := &models.UserSettings{UtilityAgentProfileID: "profile-1"}
		if err := applyBasicSettings(settings, &UpdateUserSettingsRequest{}); err != nil {
			t.Fatalf("apply settings: %v", err)
		}
		if settings.UtilityAgentProfileID != "profile-1" {
			t.Fatalf("UtilityAgentProfileID = %q, want profile-1", settings.UtilityAgentProfileID)
		}
	})

	t.Run("explicit value is trimmed and set", func(t *testing.T) {
		settings := &models.UserSettings{}
		if err := applyBasicSettings(settings, &UpdateUserSettingsRequest{UtilityAgentProfileID: ptr("  profile-42  ")}); err != nil {
			t.Fatalf("apply settings: %v", err)
		}
		if settings.UtilityAgentProfileID != "profile-42" {
			t.Fatalf("UtilityAgentProfileID = %q, want profile-42 (trimmed)", settings.UtilityAgentProfileID)
		}
	})

	t.Run("explicit empty clears the selection", func(t *testing.T) {
		settings := &models.UserSettings{UtilityAgentProfileID: "profile-1"}
		if err := applyBasicSettings(settings, &UpdateUserSettingsRequest{UtilityAgentProfileID: ptr("")}); err != nil {
			t.Fatalf("apply settings: %v", err)
		}
		if settings.UtilityAgentProfileID != "" {
			t.Fatalf("UtilityAgentProfileID = %q, want empty", settings.UtilityAgentProfileID)
		}
	})
}

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
			name:        "empty slice is accepted",
			req:         &UpdateUserSettingsRequest{SidebarViews: ptr([]models.SidebarView{})},
			wantCount:   0,
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
	if repo.preservingPatch == nil || *repo.preservingPatch != patch {
		t.Fatalf("expected preserving write patch %+v, got %+v", patch, repo.preservingPatch)
	}
	if len(eventBus.publishedEvents) != 1 {
		t.Fatalf("expected one settings event, got %d", len(eventBus.publishedEvents))
	}
}

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
	if data["task_create_last_used"] != updatedSettings.TaskCreateLastUsed {
		t.Fatalf("expected event to include preserved task-create state %+v, got %+v", updatedSettings.TaskCreateLastUsed, data["task_create_last_used"])
	}
}

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
		if repo.updatePatch != patch {
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
		if data["task_create_last_used"] != updatedSettings.TaskCreateLastUsed {
			t.Fatalf("expected event task-create state %+v, got %+v", updatedSettings.TaskCreateLastUsed, data["task_create_last_used"])
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

type recordingUserRepository struct {
	getUserCalls                              int
	getDefaultUserCalls                       int
	getUserSettingsCalls                      int
	upsertUserSettingsPreservingLastUsedCalls int
	updateCalls                               int
	updateUserID                              string
	updatePatch                               models.TaskCreateLastUsed
	updateSettings                            *models.UserSettings
	updateErr                                 error
	getSettings                               *models.UserSettings
	getErr                                    error
	preservingSettings                        *models.UserSettings
	preservingPatch                           *models.TaskCreateLastUsed
	preservingErr                             error
	closeCalls                                int
}

func (r *recordingUserRepository) GetUser(context.Context, string) (*models.User, error) {
	r.getUserCalls++
	return nil, errors.New("unexpected GetUser call")
}

func (r *recordingUserRepository) GetDefaultUser(context.Context) (*models.User, error) {
	r.getDefaultUserCalls++
	return nil, errors.New("unexpected GetDefaultUser call")
}

func (r *recordingUserRepository) GetUserSettings(context.Context, string) (*models.UserSettings, error) {
	r.getUserSettingsCalls++
	if r.getErr != nil {
		return nil, r.getErr
	}
	if r.getSettings != nil {
		return r.getSettings, nil
	}
	return nil, errors.New("unexpected GetUserSettings call")
}

func (r *recordingUserRepository) UpsertUserSettingsPreservingTaskCreateLastUsed(
	_ context.Context,
	_ *models.UserSettings,
	patch *models.TaskCreateLastUsed,
) (*models.UserSettings, error) {
	r.upsertUserSettingsPreservingLastUsedCalls++
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

func (r *recordingUserRepository) Close() error {
	r.closeCalls++
	return nil
}

type recordingEventBus struct {
	publishedSubjects []string
	publishedEvents   []*bus.Event
}

func (b *recordingEventBus) Publish(_ context.Context, subject string, event *bus.Event) error {
	b.publishedSubjects = append(b.publishedSubjects, subject)
	b.publishedEvents = append(b.publishedEvents, event)
	return nil
}

func (b *recordingEventBus) Subscribe(string, bus.EventHandler) (bus.Subscription, error) {
	return nil, errors.New("unexpected Subscribe call")
}

func (b *recordingEventBus) QueueSubscribe(string, string, bus.EventHandler) (bus.Subscription, error) {
	return nil, errors.New("unexpected QueueSubscribe call")
}

func (b *recordingEventBus) Request(context.Context, string, *bus.Event, time.Duration) (*bus.Event, error) {
	return nil, errors.New("unexpected Request call")
}

func (b *recordingEventBus) Close() {}

func (b *recordingEventBus) IsConnected() bool {
	return true
}

func TestApplyVoiceMode(t *testing.T) {
	t.Run("nil value leaves settings unchanged", func(t *testing.T) {
		settings := &models.UserSettings{
			VoiceMode: models.VoiceModeSettings{Engine: "webSpeech", Language: "en-US"},
		}
		if err := applyVoiceMode(settings, nil); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if settings.VoiceMode.Engine != "webSpeech" || settings.VoiceMode.Language != "en-US" {
			t.Fatalf("expected unchanged, got %+v", settings.VoiceMode)
		}
	})

	t.Run("happy path: applies a full update", func(t *testing.T) {
		settings := &models.UserSettings{}
		err := applyVoiceMode(settings, &models.VoiceModeSettings{
			Enabled:         true,
			Engine:          "whisperWeb",
			Language:        "pt-PT",
			Mode:            "hold",
			AutoSend:        true,
			WhisperWebModel: "small",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := models.VoiceModeSettings{
			Enabled:         true,
			Engine:          "whisperWeb",
			Language:        "pt-PT",
			Mode:            "hold",
			AutoSend:        true,
			WhisperWebModel: "small",
		}
		if settings.VoiceMode != want {
			t.Fatalf("expected %+v, got %+v", want, settings.VoiceMode)
		}
	})

	t.Run("enabled=false is honored (user disabled the feature)", func(t *testing.T) {
		settings := &models.UserSettings{VoiceMode: models.VoiceModeSettings{Enabled: true}}
		if err := applyVoiceMode(settings, &models.VoiceModeSettings{Enabled: false}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if settings.VoiceMode.Enabled {
			t.Fatalf("expected Enabled=false after disable, got true")
		}
	})

	t.Run("invalid engine is rejected", func(t *testing.T) {
		err := applyVoiceMode(&models.UserSettings{}, &models.VoiceModeSettings{Engine: "bogus"})
		if err == nil || !strings.Contains(err.Error(), "voice_mode.engine") {
			t.Fatalf("expected engine validation error, got %v", err)
		}
	})

	t.Run("invalid mode is rejected", func(t *testing.T) {
		err := applyVoiceMode(&models.UserSettings{}, &models.VoiceModeSettings{Mode: "tap"})
		if err == nil || !strings.Contains(err.Error(), "voice_mode.mode") {
			t.Fatalf("expected mode validation error, got %v", err)
		}
	})

	t.Run("invalid whisper_web_model is rejected", func(t *testing.T) {
		err := applyVoiceMode(&models.UserSettings{}, &models.VoiceModeSettings{WhisperWebModel: "huge"})
		if err == nil || !strings.Contains(err.Error(), "voice_mode.whisper_web_model") {
			t.Fatalf("expected model validation error, got %v", err)
		}
	})

	t.Run("partial update preserves string fields but zeroes booleans", func(t *testing.T) {
		settings := &models.UserSettings{
			VoiceMode: models.VoiceModeSettings{
				Enabled:         true,
				Engine:          "whisperServer",
				Language:        "en-GB",
				Mode:            "toggle",
				AutoSend:        true,
				WhisperWebModel: "tiny",
			},
		}
		// Empty strings on the new payload mean "no change" for the string fields,
		// but bools have no "unset" sentinel — every PATCH carries them. The settings
		// UI always sends the full VoiceMode object so partial updates here would
		// only happen in test or hand-crafted requests; the assertions below lock in
		// that explicit behavior so it doesn't drift silently.
		err := applyVoiceMode(settings, &models.VoiceModeSettings{Engine: "webSpeech"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if settings.VoiceMode.Engine != "webSpeech" {
			t.Fatalf("expected engine=webSpeech, got %q", settings.VoiceMode.Engine)
		}
		if settings.VoiceMode.Language != "en-GB" {
			t.Fatalf("expected language preserved, got %q", settings.VoiceMode.Language)
		}
		if settings.VoiceMode.Mode != "toggle" {
			t.Fatalf("expected mode preserved, got %q", settings.VoiceMode.Mode)
		}
		if settings.VoiceMode.WhisperWebModel != "tiny" {
			t.Fatalf("expected whisper model preserved, got %q", settings.VoiceMode.WhisperWebModel)
		}
		if settings.VoiceMode.Enabled {
			t.Fatalf("expected Enabled zeroed on partial update, got true")
		}
		if settings.VoiceMode.AutoSend {
			t.Fatalf("expected AutoSend zeroed on partial update, got true")
		}
	})
}
