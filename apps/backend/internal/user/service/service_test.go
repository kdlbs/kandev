package service

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/user/models"
)

func ptr[T any](v T) *T { return &v }

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
