package orchestrator

import (
	"context"

	"github.com/kandev/kandev/internal/office/dashboard"
)

// OfficeTaskStatusUpdater routes an Office task's status changes through
// Office's own pipeline (approval gate included) instead of a raw task
// write. Implemented by *dashboard.DashboardService.
type OfficeTaskStatusUpdater interface {
	UpdateTaskStatus(ctx context.Context, req dashboard.TaskStatusUpdateRequest) error
}

// SetOfficeTaskStatusUpdater wires the Office status pipeline used to route
// an Office task's terminal-step completion through the approval gate.
func (s *Service) SetOfficeTaskStatusUpdater(u OfficeTaskStatusUpdater) {
	s.officeTaskStatusUpdater = u
}
