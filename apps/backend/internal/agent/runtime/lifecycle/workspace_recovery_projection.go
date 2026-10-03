package lifecycle

import (
	"context"

	"github.com/kandev/kandev/internal/task/models"
)

// WorkspaceRecoveryErrorReporter projects a verified relocation refusal into
// the session's durable error contract. Lifecycle depends only on this narrow
// callback, not on task-service persistence.
type WorkspaceRecoveryErrorReporter interface {
	ReportManagedCloneRelocationRequired(
		context.Context,
		models.WorkspaceRecoveryErrorObservation,
	) (string, error)
}

// WorkspaceRecoveryProjectionError carries only the durable stamp needed by
// the caller to return the structured relocation conflict.
type WorkspaceRecoveryProjectionError struct {
	Stamp         string
	Stale         bool
	PersistFailed bool
}

func (e *WorkspaceRecoveryProjectionError) Error() string {
	if e == nil || e.PersistFailed {
		return "workspace recovery state could not be saved"
	}
	if e.Stale {
		return "workspace recovery options changed; reload the session before continuing"
	}
	return "task workspace needs explicit file-preserving recovery"
}
