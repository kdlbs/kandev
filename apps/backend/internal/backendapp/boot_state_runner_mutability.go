package backendapp

import (
	taskdto "github.com/kandev/kandev/internal/task/dto"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

// bootRunnerMutabilityProjection adapts the task service's derived
// RunnerMutabilityView to the DTO projection type. Mirrors the same adapter
// in the task handlers and bootDependencyProjection above; both exist because
// dto must stay importable from service without a cycle.
func bootRunnerMutabilityProjection(view taskservice.RunnerMutabilityView) taskdto.TaskRunnerMutabilityProjection {
	return taskdto.TaskRunnerMutabilityProjection{
		Editable: view.Editable,
		Reason:   view.Reason,
	}
}
