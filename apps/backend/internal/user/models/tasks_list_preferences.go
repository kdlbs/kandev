package models

import (
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
)

func TasksListSortValues() []string {
	return append([]string(nil), tasksListSortValues...)
}

func TasksListGroupValues() []string {
	return append([]string(nil), tasksListGroupValues...)
}

func IsValidTasksListSort(value string) bool {
	return slices.Contains(tasksListSortValues, strings.TrimSpace(value))
}

func IsValidTasksListGroup(value string) bool {
	value = strings.TrimSpace(value)
	return value == TasksListGroupState || slices.Contains(tasksListGroupValues, value)
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
