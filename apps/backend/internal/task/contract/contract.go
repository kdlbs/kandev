// Package contract contains task values shared across dependency boundaries.
package contract

import "fmt"

// PlanWriteMode selects how a plan write composes with the stored plan.
type PlanWriteMode string

const (
	// PlanWriteModeReplace submits the complete plan document.
	PlanWriteModeReplace PlanWriteMode = "replace"
	// PlanWriteModeAppend submits a fragment to add to the stored plan.
	PlanWriteModeAppend PlanWriteMode = "append"

	// TaskTitleMaxLength is the maximum number of characters in a task title.
	TaskTitleMaxLength = 60
	// MaxPlanRevisionPageLimit bounds one plan history-list call.
	MaxPlanRevisionPageLimit = 100
)

// ParsePlanWriteMode validates a caller-supplied mode. An empty string means
// replace. Values are case-sensitive so a typo cannot silently overwrite a
// plan with a fragment.
func ParsePlanWriteMode(raw string) (PlanWriteMode, error) {
	switch raw {
	case "":
		return PlanWriteModeReplace, nil
	case string(PlanWriteModeReplace):
		return PlanWriteModeReplace, nil
	case string(PlanWriteModeAppend):
		return PlanWriteModeAppend, nil
	default:
		return "", fmt.Errorf("mode must be %q or %q", PlanWriteModeReplace, PlanWriteModeAppend)
	}
}
