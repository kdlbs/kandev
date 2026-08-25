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
	if s.workspaceMaterializer != nil {
		s.workspaceMaterializer.MarkOwnerSessionMaterialized(ctx, task.ID)
	}
	return nil
}

func (s *Service) inheritFromParentEnvironment(ctx context.Context, task *v1.Task, sessionID string) error {
	if task.ParentID == "" {
		return fmt.Errorf("%w: inherit_parent task has no parent", models.ErrWorkspaceReuseUnsafe)
	}
	envID, source := s.resolveInheritedEnvironment(ctx, task)
	if envID == "" {
		return fmt.Errorf("%w: parent workspace is unavailable", models.ErrWorkspaceReuseUnsafe)
	}
	target, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil || target == nil {
		return fmt.Errorf("inherit_parent load target session: %w", err)
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
func (s *Service) resolveInheritedEnvironment(ctx context.Context, task *v1.Task) (envID, source string) {
	parentSessions, err := s.repo.ListTaskSessions(ctx, task.ParentID)
	if err != nil {
		s.logger.Warn("inherit_parent: list parent sessions failed",
			zap.String("task_id", task.ID),
			zap.String("parent_task_id", task.ParentID),
			zap.Error(err))
	} else if parent := findPrimarySession(parentSessions); parent != nil && parent.TaskEnvironmentID != "" {
		return parent.TaskEnvironmentID, "parent_session"
	}
	if s.workspaceMaterializer == nil {
		return "", ""
	}
	if envID := s.workspaceMaterializer.GetSharedGroupEnvironment(ctx, task.ID); envID != "" {
		return envID, "workspace_group"
	}
	return "", ""
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
	target, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil || target == nil {
		return fmt.Errorf("shared_group load target session: %w", err)
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
