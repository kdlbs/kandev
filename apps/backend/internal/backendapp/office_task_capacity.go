package backendapp

import (
	"errors"

	officeservice "github.com/kandev/kandev/internal/office/service"
	"github.com/kandev/kandev/internal/orchestrator"
	orchexecutor "github.com/kandev/kandev/internal/orchestrator/executor"
)

func (l *officeRunSessionLauncher) TaskRunCapacityEnabled() bool {
	return l != nil && l.capacity != nil && l.capacity.RunSessionCapacityEnabled()
}

func officeTaskCapacityResult(execution *orchexecutor.TaskExecution, err error) (*orchexecutor.TaskExecution, error) {
	if execution == nil && errors.Is(err, orchestrator.ErrOfficeTaskCapacityDeferred) {
		return nil, errors.Join(officeservice.ErrRunSessionAdmissionDeferred, err)
	}
	return execution, err
}
