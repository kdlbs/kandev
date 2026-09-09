package models

import (
	"context"
	"strings"
	"time"
)

// Runner mutability reason codes. This is the closed vocabulary
// runner_ineligible_reason is always a member of: the ten ordered condition
// codes plus "eligible" and the fail-closed "evaluation_unavailable"
// backstop.
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
// outcome. It is target-dependent and, unlike the codes above, is never
// projected on the task's runner_ineligible_reason field.
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

// RunnerMutabilitySignals is the raw, task-scoped state the ten ordered
// conditions in EvaluateRunnerMutability read. It carries nothing derived
// from workflow state, step, or priority.
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

// RunnerMutabilityVerdict is the projected pair: always both present, reason
// always a member of the closed vocabulary, never the empty string.
type RunnerMutabilityVerdict struct {
	Editable bool
	Reason   string
}

// EvaluateRunnerMutability is the single implementation of the ordered
// condition list. Every caller — the projection and the switch action
// alike — uses this function, so a projected verdict and an enforced
// verdict cannot disagree.
//
// Condition 8's "non-empty" boundary for a stored workspace path is
// resolved here: a whitespace-only value is treated as not set, the same
// blank test applied to payload identifiers elsewhere in this feature. This
// boundary is deliberately left open: a false-immutable verdict is
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

// RunnerSignalsFromTask fills the fields of RunnerMutabilitySignals that
// live on the task row itself (archived, declared workspace path, declared
// workspace mode, has-parent) rather than in another table. The caller
// fills in RepositoryCount and the four row-existence checks, which the
// task row does not carry.
func RunnerSignalsFromTask(task *Task) RunnerMutabilitySignals {
	return RunnerMutabilitySignals{
		Archived:      task.ArchivedAt != nil,
		WorkspacePath: runnerWorkspacePathFromMetadata(task.Metadata),
		HasParent:     task.ParentID != "",
		WorkspaceMode: runnerWorkspaceModeFromMetadata(task.Metadata),
	}
}

func runnerWorkspacePathFromMetadata(metadata map[string]interface{}) string {
	if v, ok := metadata[MetaKeyWorkspacePath].(string); ok {
		return v
	}
	return ""
}

// runnerWorkspaceModeFromMetadata reads the nested "workspace"."mode" block
// WorkspacePolicy.MetadataBlock (internal/task/service/handoff_service.go)
// writes onto the task row.
func runnerWorkspaceModeFromMetadata(metadata map[string]interface{}) string {
	ws, ok := metadata["workspace"].(map[string]interface{})
	if !ok {
		return ""
	}
	mode, _ := ws["mode"].(string)
	return mode
}

// RunnerSwitchRequest bundles a runner-switch write's inputs: the target
// profile, the compatibility gate's pre-transaction resolution, and the
// office-owned workspace-group membership check (mutability condition 9)
// that the task repository cannot reach directly — GroupMembershipChecker is
// called after the task row lock is acquired, so its answer is as current as
// every other condition's.
type RunnerSwitchRequest struct {
	TaskID            string
	ExecutorProfileID string
	// CompatibilityApplicable is true when the target executor requires a
	// clone URL at all, independent of whether resolution could actually run.
	CompatibilityApplicable bool
	// CompatibilityChecked is true when resolution actually ran: the target
	// executor required a clone URL and the task had exactly one repository
	// attachment at resolution time. When false but CompatibilityApplicable
	// is true, the gate was applicable but unresolved (the repository shape
	// did not allow evaluation) and ResolvedRepository* are unused.
	CompatibilityChecked bool
	// CompatibilityResolutionFailed is true when the gate applies but
	// resolution could not complete (a repository read failed, or the
	// clone-URL lookup errored or timed out) rather than being skipped for a
	// shape reason. Reported only after the mutability gate has passed, so a
	// resolution failure never preempts a stable mutability conflict.
	CompatibilityResolutionFailed bool
	// CompatibilityCloneURLFound is meaningful only when CompatibilityChecked
	// is true: whether resolution found a usable clone URL for
	// ResolvedRepositoryID.
	CompatibilityCloneURLFound  bool
	ResolvedRepositoryID        string
	ResolvedRepositoryUpdatedAt time.Time
	GroupMembershipChecker      func(ctx context.Context, taskID string) (bool, error)
}

// RunnerSwitchResult is SwitchTaskRunner's success outcome.
type RunnerSwitchResult struct {
	Task *Task
	// Changed is false for the no-op success case: the requested profile
	// already equals the stored one, both gates still passed, but nothing
	// was written.
	Changed bool
}
