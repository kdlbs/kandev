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
		IsStartStep:           step.IsStartStep,
		ShowInCommandPanel:    step.ShowInCommandPanel,
		AutoArchiveAfterHours: step.AutoArchiveAfterHours,
		AgentProfileID:        step.AgentProfileID,
		WIPLimit:              step.WIPLimit,
		PullFromStepID:        step.PullFromStepID,
		StageType:             string(step.StageType),
	}
	if hasStepEvents(step.Events) {
		events := &StepEventsDTO{}
		for _, a := range step.Events.OnEnter {
			events.OnEnter = append(events.OnEnter, StepActionDTO{
				Type:   string(a.Type),
				Config: a.Config,
			})
		}
		for _, a := range step.Events.OnTurnStart {
			events.OnTurnStart = append(events.OnTurnStart, StepActionDTO{
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
		for _, a := range step.Events.OnExit {
			events.OnExit = append(events.OnExit, StepActionDTO{
				Type:   string(a.Type),
				Config: a.Config,
			})
		}
		events.OnComment = appendGenericActions(step.Events.OnComment)
		events.OnBlockerResolved = appendGenericActions(step.Events.OnBlockerResolved)
		events.OnChildrenCompleted = appendGenericActions(step.Events.OnChildrenCompleted)
		events.OnApprovalResolved = appendGenericActions(step.Events.OnApprovalResolved)
		events.OnHeartbeat = appendGenericActions(step.Events.OnHeartbeat)
		events.OnBudgetAlert = appendGenericActions(step.Events.OnBudgetAlert)
		events.OnAgentError = appendGenericActions(step.Events.OnAgentError)
		result.Events = events
	}
	return result
}

func hasStepEvents(events wfmodels.StepEvents) bool {
	return len(events.OnEnter) > 0 ||
		len(events.OnTurnStart) > 0 ||
		len(events.OnTurnComplete) > 0 ||
		len(events.OnExit) > 0 ||
		len(events.OnComment) > 0 ||
		len(events.OnBlockerResolved) > 0 ||
		len(events.OnChildrenCompleted) > 0 ||
		len(events.OnApprovalResolved) > 0 ||
		len(events.OnHeartbeat) > 0 ||
		len(events.OnBudgetAlert) > 0 ||
		len(events.OnAgentError) > 0
}

func appendGenericActions(actions []wfmodels.GenericAction) []StepActionDTO {
	out := make([]StepActionDTO, 0, len(actions))
	for _, a := range actions {
		out = append(out, StepActionDTO{
			Type:   string(a.Type),
			Config: a.Config,
		})
	}
	return out
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
