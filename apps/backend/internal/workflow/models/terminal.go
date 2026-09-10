package models

import "strings"

// IsTerminalStep reports whether a workflow step is configured to complete its
// task and has no successor in the committed workflow order.
func IsTerminalStep(step, nextStep *WorkflowStep) bool {
	if step == nil || nextStep != nil {
		return false
	}
	return step.CompleteTaskOnEnter
}

// IsTerminalStepName recognizes the existing built-in terminal column names.
func IsTerminalStepName(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "done", "complete", "completed", "approved":
		return true
	default:
		return false
	}
}
