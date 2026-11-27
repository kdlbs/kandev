package models

import (
	"regexp"
	"slices"
	"strings"
)

const (
	TasksListSortUpdatedDesc = "updated_desc"
	TasksListSortUpdatedAsc  = "updated_asc"
	TasksListSortCreatedDesc = "created_desc"
	TasksListSortCreatedAsc  = "created_asc"
	TasksListSortTitleAsc    = "title_asc"
	TasksListSortTitleDesc   = "title_desc"
	TasksListSortDefault     = TasksListSortUpdatedDesc

	TasksListGroupState        = "state"
	TasksListGroupWorkflowStep = "workflow_step"
	TasksListGroupWorkflow     = "workflow"
	TasksListGroupRepository   = "repository"
	TasksListGroupNone         = "none"
	TasksListGroupDefault      = TasksListGroupWorkflowStep

	// TaskListFacetPrefix marks a sort or group value that names a plugin facet
	// as facet:<pluginId>:<facetId>. Facet values are page-local presentation
	// choices; they must never reach the task-list order-by input.
	TaskListFacetPrefix = "facet:"

	// TaskListFacetFormat is the accepted spelling of a plugin facet value,
	// quoted in validation errors.
	TaskListFacetFormat = "facet:<pluginId>:<facetId>"
)

var (
	tasksListSortValues = []string{
		TasksListSortUpdatedDesc,
		TasksListSortUpdatedAsc,
		TasksListSortCreatedDesc,
		TasksListSortCreatedAsc,
		TasksListSortTitleAsc,
		TasksListSortTitleDesc,
	}
	tasksListGroupValues = []string{
		TasksListGroupWorkflowStep,
		TasksListGroupWorkflow,
		TasksListGroupRepository,
		TasksListGroupNone,
	}
	// The plugin-id segment follows the manifest identity grammar; the facet-id
	// segment follows the host facet registry grammar.
	taskListFacetPluginIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)
	taskListFacetIDPattern       = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
)

func TasksListSortValues() []string {
	return append([]string(nil), tasksListSortValues...)
}

func TasksListGroupValues() []string {
	return append([]string(nil), tasksListGroupValues...)
}

// IsTaskListFacetValue reports whether value (after trimming) is a well-formed
// facet:<pluginId>:<facetId> selection.
func IsTaskListFacetValue(value string) bool {
	rest, ok := strings.CutPrefix(strings.TrimSpace(value), TaskListFacetPrefix)
	if !ok {
		return false
	}
	pluginID, facetID, ok := strings.Cut(rest, ":")
	if !ok {
		return false
	}
	return taskListFacetPluginIDPattern.MatchString(pluginID) &&
		taskListFacetIDPattern.MatchString(facetID)
}

func IsValidTasksListSort(value string) bool {
	value = strings.TrimSpace(value)
	return slices.Contains(tasksListSortValues, value) || IsTaskListFacetValue(value)
}

func IsValidTasksListGroup(value string) bool {
	value = strings.TrimSpace(value)
	return value == TasksListGroupState ||
		slices.Contains(tasksListGroupValues, value) ||
		IsTaskListFacetValue(value)
}

func NormalizeTasksListSort(value string) string {
	value = strings.TrimSpace(value)
	if IsValidTasksListSort(value) {
		return value
	}
	return TasksListSortDefault
}

func NormalizeTasksListGroup(value string) string {
	value = strings.TrimSpace(value)
	if value == TasksListGroupState {
		return TasksListGroupWorkflowStep
	}
	if IsValidTasksListGroup(value) {
		return value
	}
	return TasksListGroupDefault
}

// TasksListApiSort resolves a sort value for the task-list query. A facet
// selection maps to the default order because facets are applied page-locally
// by the client. It deliberately does not reuse IsValidTasksListSort, which
// accepts facets.
func TasksListApiSort(value string) string {
	value = strings.TrimSpace(value)
	if IsTaskListFacetValue(value) {
		return TasksListSortDefault
	}
	return NormalizeTasksListSort(value)
}
