package service

import (
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/user/models"
)

// @covers REQ-PLUGINS-TASKLIST-FACETS-003
func TestApplyBasicSettings_TasksListFacetPreferences(t *testing.T) {
	t.Run("persists a facet sort and group unchanged", func(t *testing.T) {
		settings := &models.UserSettings{}
		req := &UpdateUserSettingsRequest{
			TasksListSort:  ptr("facet:kandev-plugin-tags:tags"),
			TasksListGroup: ptr("facet:com.example.tags:tags"),
		}
		if err := applyBasicSettings(settings, req); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if settings.TasksListSort != "facet:kandev-plugin-tags:tags" {
			t.Fatalf("TasksListSort = %q, want facet unchanged", settings.TasksListSort)
		}
		if settings.TasksListGroup != "facet:com.example.tags:tags" {
			t.Fatalf("TasksListGroup = %q, want facet unchanged", settings.TasksListGroup)
		}
	})

	t.Run("rejection names both accepted forms", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			req  *UpdateUserSettingsRequest
			want string
		}{
			{"sort", &UpdateUserSettingsRequest{TasksListSort: ptr("facet:Plugin:tags")}, "updated_desc"},
			{"group", &UpdateUserSettingsRequest{TasksListGroup: ptr("facet:plugin:")}, "workflow_step"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				err := applyBasicSettings(&models.UserSettings{}, tc.req)
				if err == nil {
					t.Fatal("expected rejection of a malformed facet")
				}
				msg := err.Error()
				for _, want := range []string{tc.want, "facet:<pluginId>:<facetId>"} {
					if !strings.Contains(msg, want) {
						t.Fatalf("error %q does not name %q", msg, want)
					}
				}
			})
		}
	})
}
