package handlers

import (
	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/service"
)

// runnerMutabilityProjection adapts the service's derived RunnerMutabilityView
// to the DTO package's projection type. The two are kept separate so dto
// stays importable from service without a cycle.
func runnerMutabilityProjection(view service.RunnerMutabilityView) dto.TaskRunnerMutabilityProjection {
	return dto.TaskRunnerMutabilityProjection{
		Editable: view.Editable,
		Reason:   view.Reason,
	}
}
