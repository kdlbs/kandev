package service

import "fmt"

// PlanWriteMode selects how a plan write's content composes against the
// stored plan (REQ-TASKS-PLAN-APPEND-001).
type PlanWriteMode string

const (
	// PlanWriteModeReplace is the default: content is the whole document.
	PlanWriteModeReplace PlanWriteMode = "replace"
	// PlanWriteModeAppend composes the stored content plus a separator plus
	// content, which is only a fragment in this mode.
	PlanWriteModeAppend PlanWriteMode = "append"
)

// ParsePlanWriteMode validates a caller-supplied mode string. An empty
// string means replace, the default every existing caller relies on. Any
// other value — including one differing only in letter case, such as
// "Append" or "APPEND" — is rejected. It must not be interpreted as replace,
// which would let a typo overwrite a plan with a fragment.
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
