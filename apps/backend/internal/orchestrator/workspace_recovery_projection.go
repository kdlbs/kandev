package orchestrator

import (
	"context"

	"github.com/kandev/kandev/internal/task/models"
)

type workspaceRecoveryErrorReporter interface {
	ReportManagedCloneRelocationRequired(
		context.Context,
		models.WorkspaceRecoveryErrorObservation,
	) (string, error)
}

// SetWorkspaceRecoveryErrorReporter wires task-service ownership for durable
// relocation errors discovered by manual resume preflight.
func (s *Service) SetWorkspaceRecoveryErrorReporter(reporter workspaceRecoveryErrorReporter) {
	s.workspaceRecoveryErrorReporter = reporter
}
