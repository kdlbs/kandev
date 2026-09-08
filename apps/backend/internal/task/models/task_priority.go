package models

import "fmt"

// Canonical task priority enum values, shared by the task service and any
// lower-tier caller (such as the plugin host) that must validate a priority
// without importing internal/task/service.
const (
	TaskPriorityCritical = "critical"
	TaskPriorityHigh     = "high"
	TaskPriorityMedium   = "medium"
	TaskPriorityLow      = "low"
)

// ValidateTaskPriority checks the canonical priority enum.
func ValidateTaskPriority(priority string) error {
	switch priority {
	case TaskPriorityCritical, TaskPriorityHigh, TaskPriorityMedium, TaskPriorityLow:
		return nil
	default:
		return fmt.Errorf("invalid task priority %q", priority)
	}
}
