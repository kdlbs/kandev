package models

import (
	"strings"
	"testing"
)

// @covers AC-UI-LIST-STEP-GROUPING-001.5
func TestNormalizeTasksListGroup(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{"state", "workflow_step"},
		{" state ", "workflow_step"},
		{"workflow_step", "workflow_step"},
		{"", "workflow_step"},
		{"invalid", "workflow_step"},
		{"workflow", "workflow"},
		{"repository", "repository"},
		{"none", "none"},
	} {
		t.Run(tc.input, func(t *testing.T) {
			if got := NormalizeTasksListGroup(tc.input); got != tc.want {
				t.Fatalf("NormalizeTasksListGroup(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestIsValidTasksListGroupWorkflowStep(t *testing.T) {
	for _, value := range []string{"workflow_step", "state", " workflow_step "} {
		if !IsValidTasksListGroup(value) {
			t.Errorf("IsValidTasksListGroup(%q) = false", value)
		}
	}
}

// @covers REQ-PLUGINS-TASKLIST-FACETS-003
func TestTaskListFacetPreferences(t *testing.T) {
	accepted := []string{
		"facet:kandev-plugin-tags:tags",
		"facet:com.example.tags:tags",
		"facet:my_plugin:tags",
		" facet:plugin:tags ",
	}
	rejected := []string{
		"facet:",
		"facet:Plugin:tags",
		"facet:plugin:tag id",
		"facet:plugin:",
		"facet:tags",
		"facet:plugin:Tags",
		"facet:plugin:tags:extra",
		"unknown",
	}
	for _, value := range accepted {
		if !IsTaskListFacetValue(value) {
			t.Errorf("IsTaskListFacetValue(%q) = false, want true", value)
		}
		if !IsValidTasksListSort(value) || !IsValidTasksListGroup(value) {
			t.Errorf("IsValid* rejected accepted facet %q", value)
		}
		if got := NormalizeTasksListSort(value); got != strings.TrimSpace(value) {
			t.Errorf("NormalizeTasksListSort(%q) = %q, want trimmed facet", value, got)
		}
		if got := NormalizeTasksListGroup(value); got != strings.TrimSpace(value) {
			t.Errorf("NormalizeTasksListGroup(%q) = %q, want trimmed facet", value, got)
		}
	}
	for _, value := range rejected {
		if IsTaskListFacetValue(value) {
			t.Errorf("IsTaskListFacetValue(%q) = true, want false", value)
		}
		if got := NormalizeTasksListSort(value); got != TasksListSortDefault {
			t.Errorf("NormalizeTasksListSort(%q) = %q, want default", value, got)
		}
		if got := NormalizeTasksListGroup(value); got != TasksListGroupDefault {
			t.Errorf("NormalizeTasksListGroup(%q) = %q, want default", value, got)
		}
	}
}

// @covers REQ-PLUGINS-TASKLIST-FACETS-003
func TestTasksListApiSortNeverForwardsFacet(t *testing.T) {
	cases := []struct{ input, want string }{
		{"facet:kandev-plugin-tags:tags", TasksListSortDefault},
		{" facet:plugin:tags ", TasksListSortDefault},
		{"title_asc", "title_asc"},
		{" created_desc ", "created_desc"},
		{"bogus", TasksListSortDefault},
	}
	for _, tc := range cases {
		if got := TasksListApiSort(tc.input); got != tc.want {
			t.Errorf("TasksListApiSort(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}
