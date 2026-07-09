package models

import "strings"

// IsTerminalStep reports whether a workflow step represents finished work by
// the existing board convention: a final column named Done/Complete/Approved.
func IsTerminalStep(step, nextStep *WorkflowStep) bool {
	if step == nil || nextStep != nil {
		return false
	}
	return IsTerminalStepName(step.Name)
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
