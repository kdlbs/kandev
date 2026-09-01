package service

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/task/models"
)

// stampOrphanedWorkspaceMetadata and clearOrphanedWorkspaceMetadata are the
// single source of truth for the four orphan-marker keys written under a
// task's metadata.workspace map. Both Service.ArchiveTask (the MCP archive
// path) and HandoffService.ArchiveTaskTree (the WS/HTTP kanban archive path
// preferred whenever a HandoffService is wired) must produce byte-identical
// marker shapes, and both HandoffService.UnarchiveTaskTree entry points
// (cascade and manual root) must clear exactly what was stamped — routing
// the mutation through one place is what keeps them from drifting apart.
const (
	orphanedWorkspaceKey         = "orphaned"
	orphanedReasonWorkspaceKey   = "orphaned_reason"
	orphanedParentIDKey          = "orphaned_parent_id"
	orphanedAtKey                = "orphaned_at"
	orphanedReasonParentArchived = "parent_archived"
)

type taskMetadataKeySetter interface {
	SetTaskMetadataKey(ctx context.Context, taskID, key string, value interface{}) error
}

func stampOrphanedWorkspaceMetadata(workspace map[string]interface{}, parentID string) {
	workspace[orphanedWorkspaceKey] = true
	workspace[orphanedReasonWorkspaceKey] = orphanedReasonParentArchived
	workspace[orphanedParentIDKey] = parentID
	workspace[orphanedAtKey] = time.Now().UTC().Format(time.RFC3339)
}

// clearOrphanedWorkspaceMetadata removes the four orphan keys, if present,
// and reports whether anything changed so callers can skip a no-op write.
func clearOrphanedWorkspaceMetadata(workspace map[string]interface{}) bool {
	changed := false
	for _, key := range []string{orphanedWorkspaceKey, orphanedReasonWorkspaceKey, orphanedParentIDKey, orphanedAtKey} {
		if _, ok := workspace[key]; ok {
			delete(workspace, key)
			changed = true
		}
	}
	return changed
}

// markOrphanedInheritParentChildren stamps an orphan marker on archived's
// direct, non-archived inherit_parent children that have not materialized
// their own workspace. This is HandoffService's counterpart to
// Service.markOrphanedInheritParentChildren: the kanban WS archive action
// and the HTTP archive route both prefer HandoffService.ArchiveTaskTree
// whenever a HandoffService is wired (which production always does — see
// backendapp's registerRoutes), so that cascade path bypassing this
// detection would leave the marker dead for both of those, the primary
// user-facing archive entry points, and reachable only via the MCP
// archive_task_kandev tool's direct Service.ArchiveTask call.
func (s *HandoffService) markOrphanedInheritParentChildren(ctx context.Context, archived *models.Task) {
	if archived == nil {
		return
	}
	children, err := s.tasks.ListChildren(ctx, archived.ID)
	if err != nil {
		s.logf().Warn("list children for orphan marking failed",
			zap.String("task_id", archived.ID), zap.Error(err))
		return
	}
	envRepo, _ := s.tasks.(workspaceEnvironmentRepository)
	for _, child := range children {
		s.markOrphanedInheritParentChild(ctx, envRepo, archived, child)
	}
}

// markOrphanedInheritParentChild marks a single child, skipping any child
// that is not an unmaterialized inherit_parent workspace. A child that
// already has its own task_environments row is not orphaned: the executor's
// by-task-id environment lookup finds that row first and never falls
// through to the (now-gone) inherited one. envRepo may be nil if s.tasks
// does not implement the environment lookup in a given wiring (never true
// in production, where s.tasks is the single concrete SQLite repository);
// in that case the child is marked without the own-environment check rather
// than silently skipped, since failing to detect a real orphan is worse
// than an occasional over-eager mark.
func (s *HandoffService) markOrphanedInheritParentChild(
	ctx context.Context,
	envRepo workspaceEnvironmentRepository,
	archived, child *models.Task,
) {
	if child == nil || taskWorkspaceMode(child.Metadata) != workspaceModeInheritParent {
		return
	}
	if envRepo != nil {
		ownEnv, err := envRepo.GetTaskEnvironmentByTaskID(ctx, child.ID)
		if err != nil {
			s.logf().Warn("check child task environment for orphan marking failed",
				zap.String("task_id", child.ID), zap.String("parent_task_id", archived.ID), zap.Error(err))
			return
		}
		if ownEnv != nil {
			return
		}
	}

	workspace, _ := child.Metadata["workspace"].(map[string]interface{})
	stampOrphanedWorkspaceMetadata(workspace, archived.ID)

	if err := s.updateWorkspaceMetadata(ctx, child); err != nil {
		s.logf().Warn("mark orphaned inherit_parent child failed",
			zap.String("task_id", child.ID), zap.String("parent_task_id", archived.ID), zap.Error(err))
		return
	}
	if s.eventPublisher != nil {
		s.eventPublisher.PublishTaskUpdated(ctx, child)
	}
	s.logf().Info("marked inherit_parent child orphaned by parent archive",
		zap.String("task_id", child.ID), zap.String("parent_task_id", archived.ID))
}

// clearOrphanedInheritParentChildren is the inverse of
// markOrphanedInheritParentChildren, run when parentID comes back from
// archive. The marker asserted the parent's workspace was removed by
// archive cleanup, but that cleanup is an async job unarchive can cancel
// before it ever runs (resource_cleanup_jobs.go's cancelIfTaskUnarchived),
// so the claim can already be false the moment the parent is restored. It
// must not be left standing regardless.
//
// Deliberately uses ListChildrenIncludingArchived rather than ListChildren:
// a child can itself be archived at the moment its parent is restored (e.g.
// its own Done step's auto_archive_after_hours fired first), and nothing
// else ever revisits that child's own marker on its later unarchive — that
// call only clears the child's OWN children, never itself. Using the
// archived-excluding list here would make the marker permanent for exactly
// that ordering. The mark path (markOrphanedInheritParentChildren) stays on
// ListChildren deliberately: marking an already-archived child is wrong,
// and the cascade=true archive path (deepest-first) depends on that filter
// to skip children that are already archived by the same cascade.
func (s *HandoffService) clearOrphanedInheritParentChildren(ctx context.Context, parentID string) {
	children, err := s.tasks.ListChildrenIncludingArchived(ctx, parentID)
	if err != nil {
		s.logf().Warn("list children for orphan clearing failed",
			zap.String("task_id", parentID), zap.Error(err))
		return
	}
	for _, child := range children {
		s.clearOrphanedInheritParentChild(ctx, parentID, child)
	}
}

// clearOrphanedInheritParentChild clears the marker only when this child's
// orphan claim actually names parentID, so a child orphaned by a different
// (still-archived) ancestor is left alone.
func (s *HandoffService) clearOrphanedInheritParentChild(ctx context.Context, parentID string, child *models.Task) {
	if child == nil {
		return
	}
	workspace, _ := child.Metadata["workspace"].(map[string]interface{})
	if workspace == nil {
		return
	}
	if orphanedParentID, _ := workspace[orphanedParentIDKey].(string); orphanedParentID != parentID {
		return
	}
	if !clearOrphanedWorkspaceMetadata(workspace) {
		return
	}

	if err := s.updateWorkspaceMetadata(ctx, child); err != nil {
		s.logf().Warn("clear orphaned inherit_parent child marker failed",
			zap.String("task_id", child.ID), zap.String("parent_task_id", parentID), zap.Error(err))
		return
	}
	if s.eventPublisher != nil {
		s.eventPublisher.PublishTaskUpdated(ctx, child)
	}
	s.logf().Info("cleared inherit_parent child orphan marker after parent unarchive",
		zap.String("task_id", child.ID), zap.String("parent_task_id", parentID))
}

func (s *HandoffService) updateWorkspaceMetadata(ctx context.Context, task *models.Task) error {
	if task == nil {
		return nil
	}
	workspace, _ := task.Metadata["workspace"].(map[string]interface{})
	if setter, ok := s.tasks.(taskMetadataKeySetter); ok {
		return setter.SetTaskMetadataKey(ctx, task.ID, "workspace", workspace)
	}
	return s.tasks.UpdateTask(ctx, task)
}
