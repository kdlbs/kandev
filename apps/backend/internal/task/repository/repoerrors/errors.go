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

// ErrRepositoryBranchPolicyNotFound reports that no policy row matched the id.
var ErrRepositoryBranchPolicyNotFound = errors.New("repository branch policy not found")

// ErrRepositoryBranchPoliciesExist reports that a Gitflow starter cannot seed
// a repository that already has one or more policies.
var ErrRepositoryBranchPoliciesExist = errors.New("repository branch policies already exist")

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
