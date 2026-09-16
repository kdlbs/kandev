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
	// ErrProjectRequired is returned when a root task has no resolvable
	// project (no explicit project_id, and the run's current task has none
	// either) in a workspace that has at least one project to choose from.
	// Caller-correctable: the caller should retry with an explicit project_id.
	ErrProjectRequired = fmt.Errorf("project_id is required")
	// ErrInvalidWakeReason is returned when SpawnAgentRunInput.Reason is
	// not a member of shared.WakeReasonRegistry, the empty string
	// included (AC-OFFICE-LAUNCH-SAFETY-004.3). Caller-correctable: the
	// caller should retry with a registry member.
	ErrInvalidWakeReason = fmt.Errorf("reason must be a member of the declared wake-reason registry")
)
