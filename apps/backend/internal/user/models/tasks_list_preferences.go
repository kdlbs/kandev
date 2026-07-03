package models

import (
	"slices"
	"strings"
)

const (
	TasksListSortDefault  = "updated_desc"
	TasksListGroupDefault = "state"
)

var (
	tasksListSortValues  = []string{TasksListSortDefault, "updated_asc", "created_desc", "created_asc", "title_asc", "title_desc"}
	tasksListGroupValues = []string{TasksListGroupDefault, "workflow", "repository", "none"}
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
	return slices.Contains(tasksListGroupValues, strings.TrimSpace(value))
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
	if IsValidTasksListGroup(value) {
		return value
	}
	return TasksListGroupDefault
}
