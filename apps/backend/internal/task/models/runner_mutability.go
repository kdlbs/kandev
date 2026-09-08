package models

import "strings"

// Runner mutability reason codes. This is the closed vocabulary
// AC-TASKS-RUNNER-SWITCH-001.1 requires runner_ineligible_reason to be a
// member of: the ten ordered condition codes plus "eligible" and the
// fail-closed "evaluation_unavailable" backstop.
const (
	RunnerReasonEligible                       = "eligible"
	RunnerReasonEvaluationUnavailable          = "evaluation_unavailable"
	RunnerReasonTaskArchived                   = "task_archived"
	RunnerReasonNoRepository                   = "no_repository"
	RunnerReasonMultipleRepositories           = "multiple_repositories"
	RunnerReasonSessionExists                  = "session_exists"
	RunnerReasonEnvironmentExists              = "environment_exists"
	RunnerReasonExecutorRunning                = "executor_running"
	RunnerReasonWorkspaceFolderAttached        = "workspace_folder_attached"
	RunnerReasonWorkspacePathSet               = "workspace_path_set"
	RunnerReasonWorkspaceGroupMember           = "workspace_group_member"
	RunnerReasonWorkspaceBindingNotIndependent = "workspace_binding_not_independent"
)

// RunnerConflictTargetCannotMaterializeRepository is the compatibility-gate
// outcome (AC-TASKS-RUNNER-SWITCH-002.7). It is target-dependent and, unlike
// the codes above, is never projected on REQ-TASKS-RUNNER-SWITCH-001's
// runner_ineligible_reason field.
const RunnerConflictTargetCannotMaterializeRepository = "target_cannot_materialize_repository"

// WorkspaceModeNewWorkspace and WorkspaceModeSharedGroup are the two
// workspace-mode values EvaluateRunnerMutability's condition 10 inspects.
// Any other value (including "inherit_parent" and the empty string) is
// treated as "not new_workspace" for that condition's purposes.
const (
	WorkspaceModeInheritParent = "inherit_parent"
	WorkspaceModeNewWorkspace  = "new_workspace"
	WorkspaceModeSharedGroup   = "shared_group"
)

// RunnerMutabilitySignals is the raw, task-scoped state
// AC-TASKS-RUNNER-SWITCH-001.3's ten ordered conditions read. It carries
// nothing derived from workflow state, step, or priority
// (AC-TASKS-RUNNER-SWITCH-001.12).
type RunnerMutabilitySignals struct {
	Archived                 bool
	RepositoryCount          int
	HasSession               bool
	HasEnvironment           bool
	HasExecutorRunning       bool
	HasWorkspaceFolder       bool
	WorkspacePath            string
	HasActiveGroupMembership bool
	HasParent                bool
	// WorkspaceMode is the task's declared workspace mode, or "" when none
	// is declared (an absent mode is treated as inherit_parent for
	// independence purposes on a task with a parent).
	WorkspaceMode string
}

// RunnerMutabilityVerdict is the projected pair
// AC-TASKS-RUNNER-SWITCH-001.1 requires: always both present, reason always
// a member of the closed vocabulary, never the empty string.
type RunnerMutabilityVerdict struct {
	Editable bool
	Reason   string
}

// EvaluateRunnerMutability is the single implementation of the ordered
// condition list (AC-TASKS-RUNNER-SWITCH-001.3). Every caller — the
// projection and the switch action alike — uses this function, so a
// projected verdict and an enforced verdict cannot disagree.
//
// Condition 8's "non-empty" boundary for a stored workspace path is
// resolved here: a whitespace-only value is treated as not set, the same
// "blank" test AC-TASKS-RUNNER-SWITCH-002.8 applies to payload identifiers.
// The spec (F23) leaves this boundary open; a false-immutable verdict is
// unrecoverable from the product, while a false-editable one risks nothing
// because nothing has materialized, so the boundary favors editable.
func EvaluateRunnerMutability(s RunnerMutabilitySignals) RunnerMutabilityVerdict {
	switch {
	case s.Archived:
		return RunnerMutabilityVerdict{false, RunnerReasonTaskArchived}
	case s.RepositoryCount == 0:
		return RunnerMutabilityVerdict{false, RunnerReasonNoRepository}
	case s.RepositoryCount > 1:
		return RunnerMutabilityVerdict{false, RunnerReasonMultipleRepositories}
	case s.HasSession:
		return RunnerMutabilityVerdict{false, RunnerReasonSessionExists}
	case s.HasEnvironment:
		return RunnerMutabilityVerdict{false, RunnerReasonEnvironmentExists}
	case s.HasExecutorRunning:
		return RunnerMutabilityVerdict{false, RunnerReasonExecutorRunning}
	case s.HasWorkspaceFolder:
		return RunnerMutabilityVerdict{false, RunnerReasonWorkspaceFolderAttached}
	case strings.TrimSpace(s.WorkspacePath) != "":
		return RunnerMutabilityVerdict{false, RunnerReasonWorkspacePathSet}
	case s.HasActiveGroupMembership:
		return RunnerMutabilityVerdict{false, RunnerReasonWorkspaceGroupMember}
	case !runnerWorkspaceBindingIndependent(s.HasParent, s.WorkspaceMode):
		return RunnerMutabilityVerdict{false, RunnerReasonWorkspaceBindingNotIndependent}
	default:
		return RunnerMutabilityVerdict{true, RunnerReasonEligible}
	}
}

// runnerWorkspaceBindingIndependent implements condition 10: a task with no
// parent is independent unless it declares shared-group mode; a task with a
// parent is independent only when it declares new_workspace explicitly.
func runnerWorkspaceBindingIndependent(hasParent bool, mode string) bool {
	if !hasParent {
		return mode != WorkspaceModeSharedGroup
	}
	return mode == WorkspaceModeNewWorkspace
}
