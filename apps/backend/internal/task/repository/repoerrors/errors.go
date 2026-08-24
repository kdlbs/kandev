package repoerrors

import "errors"

// ErrWorkspaceNameMismatch reports that a confirmed workspace delete did not
// match the workspace row's current name.
var ErrWorkspaceNameMismatch = errors.New("workspace name mismatch")

// ErrWorkspaceNotFound reports that no workspace row matched the supplied id.
var ErrWorkspaceNotFound = errors.New("workspace not found")

// ErrTaskNotFound reports that no task row matched the supplied id.
var ErrTaskNotFound = errors.New("task not found")

// ErrTaskParentMismatch reports that a task no longer has the parent/workspace
// relation a cross-task mutation was authorized against.
var ErrTaskParentMismatch = errors.New("task parent relation no longer matches")

// ErrTaskPlanNotFound reports that no task plan row matched the supplied task id.
var ErrTaskPlanNotFound = errors.New("task plan not found")

// ErrRepositoryNotFound reports that no live repository row matched the supplied id.
var ErrRepositoryNotFound = errors.New("repository not found")

// ErrRepositorySetNotFound reports that no repository set row matched the supplied id.
var ErrRepositorySetNotFound = errors.New("repository set not found")

// ErrTaskEnvironmentNotFound reports that no task environment row matched the supplied id.
var ErrTaskEnvironmentNotFound = errors.New("task environment not found")

// ErrExternalIDConflict reports that a task insert lost the uniqueness race
// on uniq_tasks_external_id — the TOCTOU backstop for the create sequence's
// step-3 lookup (docs/specs/tasks/system-design/external-id-idempotency.md). Callers
// must re-read by (workspace_id, external_id) and return the winner as a
// Found outcome rather than surfacing this error.
var ErrExternalIDConflict = errors.New("external_id already claimed by another task")

// ErrTaskCleanupInProgress reports that a task lifecycle cleanup barrier is
// active, so new sessions or physical worktrees cannot be admitted for the
// task. Creation races resolve by rejecting the late comer; the cleanup
// inventory was captured under the same barrier.
var ErrTaskCleanupInProgress = errors.New("task cleanup in progress")

// ErrWorkspaceAgentPrincipalNotFound reports that no workspace agent principal
// row matched, or that the matched principal is revoked (revoked principals
// are indistinguishable from absent ones at write sites, so revocation stays
// immediate and opaque).
var ErrWorkspaceAgentPrincipalNotFound = errors.New("workspace agent principal not found")

// ErrWorkspaceAgentPrincipalConflict reports that a principal insert lost the
// uniqueness race on the (workspace_id, plugin_installation_id, logical_key)
// context constraint. Callers should re-read by context and reuse the winner.
var ErrWorkspaceAgentPrincipalConflict = errors.New("workspace agent principal context already registered")

// ErrCoordinatorGrantNotFound reports that no coordinator grant row matched,
// or that the matched grant is already revoked.
var ErrCoordinatorGrantNotFound = errors.New("coordinator grant not found")

// ErrCoordinatorGrantConflict reports that a grant insert lost the uniqueness
// race on the principal-scoped partial unique index - the principal already
// holds an active grant for the same (scope_kind, scope_id).
var ErrCoordinatorGrantConflict = errors.New("coordinator grant already exists for scope")
