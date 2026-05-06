package runtime

import (
	"fmt"

	"github.com/kandev/kandev/internal/office/shared"
)

var (
	// ErrCapabilityDenied is returned when a run lacks the requested capability.
	ErrCapabilityDenied = fmt.Errorf("%w: capability denied", shared.ErrForbidden)
	// ErrTaskOutOfScope is returned when a run tries to mutate an unscoped task.
	ErrTaskOutOfScope = fmt.Errorf("%w: task out of scope", shared.ErrForbidden)
	// ErrWorkspaceOutOfScope is returned when a run tries to mutate another workspace.
	ErrWorkspaceOutOfScope = fmt.Errorf("%w: workspace out of scope", shared.ErrForbidden)
	// ErrRuntimeDependencyMissing indicates that a runtime action was not fully wired.
	ErrRuntimeDependencyMissing = fmt.Errorf("runtime dependency missing")
)
