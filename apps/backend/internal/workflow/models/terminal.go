package models

import "strings"

// IsFinalStep reports whether no workflow step follows step by position.
// A step tied at the maximum position is also final.
func IsFinalStep(step, nextStep *WorkflowStep) bool {
	if step == nil {
		return false
	}
	return nextStep == nil || nextStep.Position <= step.Position
}

// CompletesTaskOnEnter reports whether a workflow step is both final and
// configured to complete its task on entry.
func CompletesTaskOnEnter(step, nextStep *WorkflowStep) bool {
	return IsFinalStep(step, nextStep) && step.CompleteTaskOnEnter
}

// FinalStepSQL returns a SQL predicate for an aliased workflow_steps row.
// The alias is an internal SQL identifier supplied by the caller.
func FinalStepSQL(alias string) string {
	return `(NOT EXISTS (
		SELECT 1 FROM workflow_steps AS completion_successor
		WHERE completion_successor.workflow_id = ` + alias + `.workflow_id
		  AND completion_successor.position > ` + alias + `.position
	))`
}

// CompletingStepSQL returns a SQL predicate for an aliased workflow_steps row.
// The alias is an internal SQL identifier supplied by the caller.
func CompletingStepSQL(alias string) string {
	return `(` + alias + `.complete_task_on_enter = 1 AND ` + FinalStepSQL(alias) + `)`
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
