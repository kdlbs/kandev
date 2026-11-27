package service

import (
	"testing"

	"github.com/kandev/kandev/internal/user/models"
)

func TestApplyBasicSettingsJiraDefaultViewPatch(t *testing.T) {
	t.Run("omitted field preserves previous default", func(t *testing.T) {
		settings := &models.UserSettings{JiraDefaultViewID: "old-view"}
		if err := applyBasicSettings(settings, &UpdateUserSettingsRequest{}); err != nil {
			t.Fatalf("apply basic settings: %v", err)
		}
		if settings.JiraDefaultViewID != "old-view" {
			t.Fatalf("JiraDefaultViewID = %q, want old-view", settings.JiraDefaultViewID)
		}
	})

	t.Run("nonempty value is trimmed and set", func(t *testing.T) {
		value := "  custom-view  "
		settings := &models.UserSettings{}
		if err := applyBasicSettings(settings, &UpdateUserSettingsRequest{JiraDefaultViewID: &value}); err != nil {
			t.Fatalf("apply basic settings: %v", err)
		}
		if settings.JiraDefaultViewID != "custom-view" {
			t.Fatalf("JiraDefaultViewID = %q, want custom-view", settings.JiraDefaultViewID)
		}
	})

	t.Run("empty value clears the default", func(t *testing.T) {
		value := ""
		settings := &models.UserSettings{JiraDefaultViewID: "old-view"}
		if err := applyBasicSettings(settings, &UpdateUserSettingsRequest{JiraDefaultViewID: &value}); err != nil {
			t.Fatalf("apply basic settings: %v", err)
		}
		if settings.JiraDefaultViewID != "" {
			t.Fatalf("JiraDefaultViewID = %q, want empty", settings.JiraDefaultViewID)
		}
	})
}
