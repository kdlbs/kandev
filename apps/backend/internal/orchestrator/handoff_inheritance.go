package orchestrator

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// WorkspaceMaterializer is the office task-handoffs hook the
// orchestrator calls when an owner task's session is prepared. The
// implementation (task/service.HandoffService) is responsible for
// flipping owned_by_kandev / cleanup_policy on the workspace group AND
// for returning the materialized environment id used by shared_group
// launch inheritance. A missing materializer is unsafe for an already
// materialized group and must fail closed rather than creating a sibling.
type WorkspaceMaterializer interface {
	MarkOwnerSessionMaterialized(ctx context.Context, taskID string)
	// GetSharedGroupEnvironment returns the materialized environment
	// ID of the task's active workspace group, or "" when the task is
	// not in a group / the group has not yet been materialized.
	GetSharedGroupEnvironment(ctx context.Context, taskID string) string
}

// SetWorkspaceMaterializer wires the office task-handoffs materializer.
// Called by cmd/kandev after the HandoffService is constructed.
func (s *Service) SetWorkspaceMaterializer(m WorkspaceMaterializer) {
	s.workspaceMaterializer = m
}

func (s *Service) markWorkspaceGroupMaterialized(ctx context.Context, taskID string) {
	if s.workspaceMaterializer != nil {
		s.workspaceMaterializer.MarkOwnerSessionMaterialized(ctx, taskID)
	}
}

// admitInheritedEnvironment checks executor ownership before session creation
// can publish a binding to another task's canonical environment.
func (s *Service) admitInheritedEnvironment(ctx context.Context, task *v1.Task, executorID, executorProfileID string) error {
	mode, _ := workspacePolicyMode(task.Metadata)
	var envID string
	switch mode {
	case models.WorkspaceModeInheritParent:
		if task.ParentID == "" {
			return fmt.Errorf("%w: inherit_parent task has no parent", models.ErrWorkspaceReuseUnsafe)
		}
		var err error
		envID, _, _, err = s.resolveInheritedEnvironmentChecked(ctx, task)
		if err != nil {
			return err
		}
	case models.WorkspaceModeSharedGroup:
		if s.workspaceMaterializer != nil {
			envID = s.workspaceMaterializer.GetSharedGroupEnvironment(ctx, task.ID)
		}
	default:
		return nil
	}
	if envID == "" {
		return nil
	}
	available, err := s.validateInheritedEnvironmentReference(ctx, task, envID)
	if err != nil {
		return err
	}
	if !available {
		return fmt.Errorf("%w: inherited environment could not be verified", models.ErrWorkspaceReuseUnsafe)
	}
	env, err := s.repo.GetTaskEnvironment(ctx, envID)
	if err != nil {
		return fmt.Errorf("%w: inherited environment could not be verified", models.ErrWorkspaceReuseUnsafe)
	}
	return s.executor.AdmitInheritedEnvironment(ctx, task, env, executorID, executorProfileID)
}

// propagateInheritedEnvironment is the launch-time consumer of the
// workspace policy persisted by office task-handoffs phase 4. When the
// task's metadata.workspace.mode is "inherit_parent" or "shared_group"
// AND a source environment is resolvable (parent's primary session for
// inherit_parent, the workspace group's MaterializedEnvironmentID for
// shared_group), the new session's TaskEnvironmentID is overwritten so
// the executor binds to the existing environment instead of creating a
// fresh one.
//
// Also marks the owner task's workspace group as materialized once a
// session has been prepared — every PrepareTaskSession call is an
// opportunity to record the materialized state, and
// MarkOwnerSessionMaterialized is idempotent so repeated calls are a
// no-op after the first successful flip.
//
// Inheritance is a workspace ownership contract, not a best-effort
// optimization. A required inherited environment that cannot be resolved
// fails closed before the new session is exposed.
func (s *Service) propagateInheritedEnvironment(ctx context.Context, task *v1.Task, sessionID string) error {
	if task == nil || sessionID == "" {
		return nil
	}
	mode, _ := workspacePolicyMode(task.Metadata)
	switch mode {
	case "inherit_parent":
		if err := s.inheritFromParentEnvironment(ctx, task, sessionID); err != nil {
			return err
		}
		// Parent may have launched in this same call (or earlier); flip
		// the group to materialized on the parent's behalf so the next
		// cleanup evaluation can decide.
		if s.workspaceMaterializer != nil && task.ParentID != "" {
			s.workspaceMaterializer.MarkOwnerSessionMaterialized(ctx, task.ParentID)
		}
	case "shared_group":
		// shared_group: the session binder elects a creating canonical
		// environment before this hook runs. Propagate that durable group
		// binding (or fail closed if it cannot be read) rather than
		// allowing a member to create a task-local fallback workspace.
		if err := s.inheritFromSharedGroup(ctx, task, sessionID); err != nil {
			return err
		}
	}
	// Whether or not this task has a workspace policy, the task itself
	// may be the owner of a workspace group (e.g. a parent task launching
	// after its child created the group). Try to mark.
	s.markWorkspaceGroupMaterialized(ctx, task.ID)
	return nil
}

func (s *Service) inheritFromParentEnvironment(ctx context.Context, task *v1.Task, sessionID string) error {
	if task.ParentID == "" {
		return fmt.Errorf("%w: inherit_parent task has no parent", models.ErrWorkspaceReuseUnsafe)
	}
	envID, source, pending, resolveErr := s.resolveInheritedEnvironmentChecked(ctx, task)
	if resolveErr != nil {
		return resolveErr
	}
	if envID == "" {
		parent, _ := s.repo.GetTask(ctx, task.ParentID)
		if pending {
			return fmt.Errorf("%w: %w: %s", models.ErrWorkspaceReuseUnsafe, models.ErrInheritedWorkspacePending,
				models.DescribeInheritedEnvironmentPending(task.ParentID))
		}
		return fmt.Errorf("%w: %s", models.ErrWorkspaceReuseUnsafe,
			models.DescribeInheritedEnvironmentUnavailable(task.ParentID, parent))
	}
	target, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil || target == nil {
		return fmt.Errorf("inherit_parent load target session: %w", err)
	}
	if err := s.admitSessionInheritedEnvironment(ctx, task, target, envID); err != nil {
		return err
	}
	if target.TaskEnvironmentID == envID {
		return nil
	}
	target.TaskEnvironmentID = envID
	target.UpdatedAt = time.Now().UTC()
	if err := s.repo.UpdateTaskSession(ctx, target); err != nil {
		return fmt.Errorf("inherit_parent bind session: %w", err)
	}
	s.logger.Info("inherit_parent: propagated environment",
		zap.String("task_id", task.ID),
		zap.String("session_id", sessionID),
		zap.String("task_environment_id", envID),
		zap.String("source", source))
	return nil
}

// resolveInheritedEnvironment looks up the parent's primary-session env
// first, then falls back to the workspace group's materialized env id
// when the parent has no live session (post-review #2). The fallback is
// what lets a child task re-launch after the parent's session has been
// stopped — without it, the child would silently launch into a fresh env
// and the workspace inheritance contract would break.
//
// The parent session's TaskEnvironmentID is a bare foreign-key-less string:
// nothing stops the referenced task_environments row from being deleted out
// from under it, and archiving the parent does NOT do this by itself —
// archive preserves the task_environments row and only tears down its
// runtime resources (worktree, container/sandbox), so an archived parent's
// env row still exists but its worktree is gone. A definitively archived
// parent is therefore rejected up front, before either branch below is even
// attempted. Parent lookup errors also fail closed. The row can be deleted
// outright by a DELETE cascade or an explicit ResetTaskEnvironment; that
// case is caught by verifying the id against task_environments on both
// branches below. A missing environment row then returns an empty result and
// the caller reports a typed unsafe-reuse error. Without the second check,
// the group branch would hand back exactly the id the parent_session branch
// just rejected: an inherit_parent child is a member of the parent's
// workspace group, and MarkOwnerSessionMaterialized records the parent as
// that group's owner.
func (s *Service) resolveInheritedEnvironment(ctx context.Context, task *v1.Task) (envID, source string) {
	envID, source, _, _ = s.resolveInheritedEnvironmentChecked(ctx, task)
	return envID, source
}

// resolveInheritedEnvironmentChecked resolves the environment a child inherits
// and reports whether an unresolved inheritance is still pending. The pending
// verdict is derived from the same reads that resolve the environment, so a
// parent that materializes its workspace concurrently cannot change the
// classification after this pass (which would turn a retryable refusal into a
// permanent one).
func (s *Service) resolveInheritedEnvironmentChecked(ctx context.Context, task *v1.Task) (envID, source string, pending bool, err error) {
	parent, err := s.repo.GetTask(ctx, task.ParentID)
	if err != nil || parent == nil {
		s.logger.Warn("inherit_parent: load parent task failed",
			zap.String("task_id", task.ID),
			zap.String("parent_task_id", task.ParentID),
			zap.Error(err))
		return "", "", false, fmt.Errorf("%w: parent task %s could not be verified", models.ErrWorkspaceReuseUnsafe, task.ParentID)
	}
	if parent.ArchivedAt != nil {
		s.logger.Warn("inherit_parent: parent task is archived, environment inheritance unavailable",
			zap.String("task_id", task.ID),
			zap.String("parent_task_id", task.ParentID))
		return "", "", false, fmt.Errorf("%w: %s", models.ErrWorkspaceReuseUnsafe,
			models.DescribeInheritedEnvironmentUnavailable(task.ParentID, parent))
	}
	pending, envID, source, err = s.resolveLiveParentEnvironment(ctx, task, parent)
	return envID, source, pending, err
}

// resolveLiveParentEnvironment resolves a live parent's environment and
// reports whether an unresolved inheritance is still pending. Every source is
// read in this one pass: a parent that materializes its workspace concurrently
// cannot change the pending classification afterwards, which would turn a
// retryable refusal into a permanent one (or the reverse).
func (s *Service) resolveLiveParentEnvironment(
	ctx context.Context, task *v1.Task, parent *models.Task,
) (pending bool, envID, source string, err error) {
	// An unresolved inheritance is pending only for a live parent whose
	// sessions, owned environment row, and workspace group all name no
	// environment yet.
	pending = !isTerminalTaskState(parent.State)
	parentSessions, err := s.repo.ListTaskSessions(ctx, task.ParentID)
	if err != nil {
		s.logger.Warn("inherit_parent: list parent sessions failed",
			zap.String("task_id", task.ID),
			zap.String("parent_task_id", task.ParentID),
			zap.Error(err))
		pending = false
	} else {
		if anyParentSessionEnvironment(parentSessions) {
			pending = false
		}
		if candidate, candidateSource := parentSessionEnvironmentCandidate(parentSessions); candidate != "" {
			envID, resolveErr := s.availableInheritedEnvironment(ctx, task, candidate, candidateSource)
			if resolveErr != nil || envID != "" {
				return false, envID, candidateSource, resolveErr
			}
		}
	}
	envID, parentEnvExists, envErr := s.parentOwnedEnvironment(ctx, task)
	if parentEnvExists {
		pending = false
	}
	if envErr != nil || envID != "" {
		return false, envID, "parent_task_environment", envErr
	}
	groupEnvID, groupNamed, groupErr := s.workspaceGroupEnvironment(ctx, task)
	if groupNamed {
		pending = false
	}
	if groupErr != nil || groupEnvID != "" {
		return false, groupEnvID, "workspace_group", groupErr
	}
	return pending, "", "", nil
}

// anyParentSessionEnvironment reports whether any parent session already names
// an environment, even one that failed validation: such a reference makes the
// refusal permanent rather than pending.
func anyParentSessionEnvironment(sessions []*models.TaskSession) bool {
	for _, session := range sessions {
		if session != nil && session.TaskEnvironmentID != "" {
			return true
		}
	}
	return false
}

// workspaceGroupEnvironment resolves the workspace group's materialized
// environment and reports whether the group names one at all.
func (s *Service) workspaceGroupEnvironment(ctx context.Context, task *v1.Task) (envID string, named bool, err error) {
	if s.workspaceMaterializer == nil {
		return "", false, nil
	}
	envID = s.workspaceMaterializer.GetSharedGroupEnvironment(ctx, task.ID)
	if envID == "" {
		return "", false, nil
	}
	available, validateErr := s.validateInheritedEnvironmentReference(ctx, task, envID)
	if validateErr != nil {
		return "", true, validateErr
	}
	if available {
		return envID, true, nil
	}
	s.logger.Warn("inherit_parent: workspace group environment no longer exists",
		zap.String("task_id", task.ID),
		zap.String("parent_task_id", task.ParentID),
		zap.String("task_environment_id", envID))
	return "", true, nil
}

func isTerminalTaskState(state v1.TaskState) bool {
	return state == v1.TaskStateCompleted || state == v1.TaskStateFailed || state == v1.TaskStateCancelled
}

// parentSessionEnvironmentCandidate picks the environment a parent's sessions
// point at. The primary session wins. Office sessions are never primary, so a
// parent without a primary session falls back to the environment its
// sessions hold, but only when they all agree on one environment: sessions
// bound to different environments leave the choice to the parent-owned
// environment and the workspace group instead of guessing.
func parentSessionEnvironmentCandidate(sessions []*models.TaskSession) (envID, source string) {
	if primary := findPrimarySession(sessions); primary != nil {
		if primary.TaskEnvironmentID == "" {
			return "", ""
		}
		return primary.TaskEnvironmentID, "parent_session"
	}
	for _, session := range sessions {
		if session == nil || session.TaskEnvironmentID == "" {
			continue
		}
		if envID != "" && envID != session.TaskEnvironmentID {
			return "", ""
		}
		envID = session.TaskEnvironmentID
	}
	if envID == "" {
		return "", ""
	}
	return envID, "parent_environment_session"
}

// availableInheritedEnvironment returns envID when it still exists and its
// owner is not archived. A missing environment returns "" so the caller can
// try the next source; an unverifiable owner fails closed.
func (s *Service) availableInheritedEnvironment(ctx context.Context, task *v1.Task, envID, source string) (string, error) {
	available, err := s.validateInheritedEnvironmentReference(ctx, task, envID)
	if err != nil {
		return "", err
	}
	if available {
		return envID, nil
	}
	s.logger.Warn("inherit_parent: parent environment no longer exists",
		zap.String("task_id", task.ID),
		zap.String("parent_task_id", task.ParentID),
		zap.String("task_environment_id", envID),
		zap.String("source", source))
	return "", nil
}

// parentOwnedEnvironment resolves the environment row the parent task itself
// owns. It reports whether the row exists so the pending classification uses
// the same read: a dangling reference makes the refusal permanent.
func (s *Service) parentOwnedEnvironment(ctx context.Context, task *v1.Task) (envID string, exists bool, err error) {
	env, err := s.repo.GetTaskEnvironmentByTaskID(ctx, task.ParentID)
	if err != nil {
		return "", false, fmt.Errorf("%w: parent task %s environment could not be read", models.ErrWorkspaceReuseUnsafe, task.ParentID)
	}
	if env == nil || env.ID == "" {
		return "", false, nil
	}
	envID, err = s.availableInheritedEnvironment(ctx, task, env.ID, "parent_task_environment")
	return envID, true, err
}

// validateInheritedEnvironmentReference confirms that an inherited
// environment still exists and that its owner is not archived. This check
// covers nested handoffs where a live child still points at an environment
// materialized by an archived ancestor. Owner lookup errors fail closed so a
// new session cannot bind to an unverifiable worktree.
func (s *Service) validateInheritedEnvironmentReference(ctx context.Context, task *v1.Task, envID string) (bool, error) {
	env, err := s.repo.GetTaskEnvironment(ctx, envID)
	if err != nil || env == nil {
		return false, nil
	}
	if env.TaskID == "" || (task != nil && env.TaskID == task.ID) {
		return true, nil
	}
	owner, err := s.repo.GetTask(ctx, env.TaskID)
	if err != nil || owner == nil {
		return false, fmt.Errorf("%w: inherited task environment owner %s could not be verified", models.ErrWorkspaceReuseUnsafe, env.TaskID)
	}
	if owner.ArchivedAt != nil {
		return false, fmt.Errorf("%w: %s", models.ErrWorkspaceReuseUnsafe,
			models.DescribeInheritedEnvironmentUnavailable(env.TaskID, owner))
	}
	return true, nil
}

// inheritFromSharedGroup propagates the workspace group's materialized
// environment id onto the new session. Mirrors inheritFromParentEnvironment
// but reads from the workspace group instead of the parent's primary
// session — so any member of a shared_group ends up bound to the same
// TaskEnvironment as every other member.
func (s *Service) inheritFromSharedGroup(ctx context.Context, task *v1.Task, sessionID string) error {
	if s.workspaceMaterializer == nil {
		return fmt.Errorf("%w: shared workspace group is unavailable", models.ErrWorkspaceReuseUnsafe)
	}
	envID := s.workspaceMaterializer.GetSharedGroupEnvironment(ctx, task.ID)
	if envID == "" {
		return fmt.Errorf("%w: shared workspace group has no canonical environment", models.ErrWorkspaceReuseUnsafe)
	}
	available, validateErr := s.validateInheritedEnvironmentReference(ctx, task, envID)
	if validateErr != nil {
		return validateErr
	}
	if !available {
		return fmt.Errorf("%w: shared workspace group environment %s no longer exists", models.ErrWorkspaceReuseUnsafe, envID)
	}
	target, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil || target == nil {
		return fmt.Errorf("shared_group load target session: %w", err)
	}
	if err := s.admitSessionInheritedEnvironment(ctx, task, target, envID); err != nil {
		return err
	}
	if target.TaskEnvironmentID == envID {
		return nil
	}
	target.TaskEnvironmentID = envID
	target.UpdatedAt = time.Now().UTC()
	if err := s.repo.UpdateTaskSession(ctx, target); err != nil {
		return fmt.Errorf("shared_group bind session: %w", err)
	}
	s.logger.Info("shared_group: propagated group environment",
		zap.String("task_id", task.ID),
		zap.String("session_id", sessionID),
		zap.String("task_environment_id", envID))
	return nil
}

// Propagation uses the prepared session's resolved executor before binding a
// parent or group environment that may have become available after preflight.
func (s *Service) admitSessionInheritedEnvironment(ctx context.Context, task *v1.Task, session *models.TaskSession, envID string) error {
	env, err := s.repo.GetTaskEnvironment(ctx, envID)
	if err != nil || env == nil {
		return fmt.Errorf("%w: inherited environment could not be verified", models.ErrWorkspaceReuseUnsafe)
	}
	return s.executor.AdmitInheritedEnvironment(ctx, task, env, session.ExecutorID, session.ExecutorProfileID)
}

// workspacePolicyMode reads metadata.workspace.mode (set by
// AttachWorkspacePolicy via the MCP create_task / delegate_task path).
func workspacePolicyMode(meta map[string]interface{}) (string, bool) {
	ws, ok := meta["workspace"].(map[string]interface{})
	if !ok {
		return "", false
	}
	v, ok := ws["mode"].(string)
	return v, ok
}
