package models

import "fmt"

// WorkflowSessionTargetKind identifies the conversation a workflow step may
// reuse when it starts a session.
type WorkflowSessionTargetKind string

const (
	WorkflowSessionTargetInitial WorkflowSessionTargetKind = "initial"
	WorkflowSessionTargetStep    WorkflowSessionTargetKind = "step"
)

// WorkflowSessionTarget is the durable workflow configuration for session
// reuse. StepID is required only for a step target and refers to an earlier
// step in the same workflow.
type WorkflowSessionTarget struct {
	Kind   WorkflowSessionTargetKind `json:"kind" yaml:"kind"`
	StepID string                    `json:"step_id,omitempty" yaml:"step_id,omitempty"`
}

// ValidateWorkflowSessionTarget validates the closed target union without
// needing the surrounding workflow. Position and ownership rules are checked
// by workflow validators.
func ValidateWorkflowSessionTarget(target *WorkflowSessionTarget) error {
	if target == nil {
		return nil
	}
	switch target.Kind {
	case WorkflowSessionTargetInitial:
		if target.StepID != "" {
			return fmt.Errorf("initial session target must not include step_id")
		}
	case WorkflowSessionTargetStep:
		if target.StepID == "" {
			return fmt.Errorf("step session target requires step_id")
		}
	default:
		return fmt.Errorf("unknown session target kind %q", target.Kind)
	}
	return nil
}

// CloneWorkflowSessionTarget returns an independent target value.
func CloneWorkflowSessionTarget(target *WorkflowSessionTarget) *WorkflowSessionTarget {
	if target == nil {
		return nil
	}
	clone := *target
	return &clone
}

// RemapWorkflowSessionTarget replaces a template step alias with its newly
// generated workflow-step ID.
func RemapWorkflowSessionTarget(target *WorkflowSessionTarget, idMap map[string]string) *WorkflowSessionTarget {
	clone := CloneWorkflowSessionTarget(target)
	if clone == nil || clone.Kind != WorkflowSessionTargetStep {
		return clone
	}
	clone.StepID = RemapStepID(clone.StepID, idMap)
	return clone
}

// EqualWorkflowSessionTarget compares nullable target values.
func EqualWorkflowSessionTarget(left, right *WorkflowSessionTarget) bool {
	if left == nil || right == nil {
		return left == right
	}
	return left.Kind == right.Kind && left.StepID == right.StepID
}
