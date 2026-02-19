package dto

import (
	"github.com/kandev/kandev/internal/task/service"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

// FromWorkflowStep converts a workflow step model to a WorkflowStepDTO.
// This is the base conversion without timestamps.
func FromWorkflowStep(step *wfmodels.WorkflowStep) WorkflowStepDTO {
	result := WorkflowStepDTO{
		ID:                    step.ID,
		WorkflowID:            step.WorkflowID,
		Name:                  step.Name,
		Position:              step.Position,
		Color:                 step.Color,
		Prompt:                step.Prompt,
		AllowManualMove:       step.AllowManualMove,
		AutoArchiveAfterHours: step.AutoArchiveAfterHours,
	}
	if len(step.Events.OnEnter) > 0 || len(step.Events.OnTurnComplete) > 0 {
		events := &StepEventsDTO{}
		for _, a := range step.Events.OnEnter {
			events.OnEnter = append(events.OnEnter, StepActionDTO{
				Type:   string(a.Type),
				Config: a.Config,
			})
		}
		for _, a := range step.Events.OnTurnComplete {
			events.OnTurnComplete = append(events.OnTurnComplete, StepActionDTO{
				Type:   string(a.Type),
				Config: a.Config,
			})
		}
		result.Events = events
	}
	return result
}

// FromWorkflowStepWithTimestamps converts a workflow step model to a WorkflowStepDTO,
// including CreatedAt and UpdatedAt timestamps.
func FromWorkflowStepWithTimestamps(step *wfmodels.WorkflowStep) WorkflowStepDTO {
	result := FromWorkflowStep(step)
	result.CreatedAt = step.CreatedAt
	result.UpdatedAt = step.UpdatedAt
	return result
}

// FromBranch converts a service Branch to a BranchDTO.
func FromBranch(b service.Branch) BranchDTO {
	return BranchDTO{
		Name:   b.Name,
		Type:   b.Type,
		Remote: b.Remote,
	}
}
