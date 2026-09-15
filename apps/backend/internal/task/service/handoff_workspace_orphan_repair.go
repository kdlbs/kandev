package service

import (
	"context"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/task/models"
)

// workspaceOrphanRepairRepository is the optional capability the startup
// repair uses to find repair candidates. A wiring without it (never true in
// production) runs neither selection rather than failing startup.
type workspaceOrphanRepairRepository interface {
	ListOrphanRepairCandidates(ctx context.Context) ([]models.OrphanRepairCandidate, error)
	ListStaleOrphanMarkers(ctx context.Context) ([]models.StaleOrphanMarker, error)
	CountMalformedTaskMetadata(ctx context.Context) (int, error)
}

// RepairOrphanedWorkspaceMarkers stamps the orphan marker onto the
// historical population no archive event will ever reach (AC-003.1/.7), and
// retracts a marker a crashed clear left behind (AC-003.9). Safe on every
// startup: idempotent (AC-003.3), warn-and-continue per task (AC-003.6), and
// never aborts the caller on a failed selection (AC-003.11).
func (s *HandoffService) RepairOrphanedWorkspaceMarkers(ctx context.Context) {
	repo, ok := s.tasks.(workspaceOrphanRepairRepository)
	if !ok {
		s.logf().Info("task repository does not support the orphan marker repair; skipping")
		return
	}

	if count, err := repo.CountMalformedTaskMetadata(ctx); err != nil {
		s.logf().Warn("count malformed task metadata for orphan marker repair failed", zap.Error(err))
	} else if count > 0 {
		s.logf().Warn("tasks with malformed metadata excluded from orphan marker repair",
			zap.Int("count", count))
	}

	// Clear stale claims first so a child with a stale marker is eligible for
	// the stamping query again when its current parent is still archived.
	cleared := s.repairClearStaleOrphanMarkers(ctx, repo)
	stamped := s.repairStampOrphanMarkers(ctx, repo)
	s.logf().Info("orphan marker repair pass complete",
		zap.Int("stamped", stamped), zap.Int("cleared", cleared))
}

func (s *HandoffService) repairStampOrphanMarkers(ctx context.Context, repo workspaceOrphanRepairRepository) int {
	candidates, err := repo.ListOrphanRepairCandidates(ctx)
	if err != nil {
		s.logf().Warn("list orphan repair candidates failed, skipping stamping pass this boot", zap.Error(err))
		return 0
	}
	stamped := 0
	for _, candidate := range candidates {
		if s.repairStampOne(ctx, candidate) {
			stamped++
		}
	}
	return stamped
}

func (s *HandoffService) repairStampOne(ctx context.Context, candidate models.OrphanRepairCandidate) bool {
	workspace := candidate.Workspace
	if workspace == nil {
		workspace = map[string]interface{}{}
	}
	// The guard is built from the row as selected, before this pass mutates
	// it — the same rule as every other caller of the guarded write.
	guard := models.ObservedWorkspaceGuard(workspace)
	guard.RequireParentArchivedID = candidate.ParentID
	guard.RequireParentID = candidate.ParentID
	guard.RequireNoOwnEnvironment = true
	guard.RequireTaskNotArchived = true
	stampOrphanedWorkspaceMetadata(workspace, candidate.ParentID)

	landed, err := s.updateWorkspaceMetadataByID(ctx, candidate.TaskID, guard, workspace)
	if err != nil {
		s.logf().Warn("orphan marker repair: stamp failed",
			zap.String("task_id", candidate.TaskID), zap.String("parent_task_id", candidate.ParentID), zap.Error(err))
		return false
	}
	if !landed {
		s.logf().Debug("orphan marker repair: stamp lost its guard",
			zap.String("task_id", candidate.TaskID), zap.String("parent_task_id", candidate.ParentID))
		return false
	}
	s.publishRepairedTask(ctx, candidate.TaskID)
	return true
}

func (s *HandoffService) repairClearStaleOrphanMarkers(ctx context.Context, repo workspaceOrphanRepairRepository) int {
	markers, err := repo.ListStaleOrphanMarkers(ctx)
	if err != nil {
		s.logf().Warn("list stale orphan markers failed, skipping clearing pass this boot", zap.Error(err))
		return 0
	}
	cleared := 0
	for _, marker := range markers {
		if s.repairClearOne(ctx, marker) {
			cleared++
		}
	}
	return cleared
}

func (s *HandoffService) repairClearOne(ctx context.Context, marker models.StaleOrphanMarker) bool {
	workspace := marker.Workspace
	if workspace == nil {
		return false
	}
	guard := models.ObservedWorkspaceGuard(workspace)
	// The selection already proved this claim names a present, unarchived
	// parent — guard the write against a re-archive landing in the window
	// between selection and write (AC-003.10).
	guard.RequireParentUnarchivedID = guard.ExpectedOrphanedParentID
	if !clearOrphanedWorkspaceMetadata(workspace) {
		return false
	}

	landed, err := s.updateWorkspaceMetadataByID(ctx, marker.TaskID, guard, workspace)
	if err != nil {
		s.logf().Warn("orphan marker repair: clear failed", zap.String("task_id", marker.TaskID), zap.Error(err))
		return false
	}
	if !landed {
		s.logf().Debug("orphan marker repair: clear lost its guard", zap.String("task_id", marker.TaskID))
		return false
	}
	s.publishRepairedTask(ctx, marker.TaskID)
	return true
}

// updateWorkspaceMetadataByID writes the whole workspace sub-map guarded by
// cas for a task the caller only has the ID for — the repair's selections
// return rows, not *models.Task.
func (s *HandoffService) updateWorkspaceMetadataByID(
	ctx context.Context, taskID string, guard models.OrphanWriteGuard, workspace map[string]interface{},
) (bool, error) {
	setter, ok := s.tasks.(taskWorkspaceMetadataCASSetter)
	if !ok {
		s.logf().Warn("task repository does not support guarded workspace metadata writes", zap.String("task_id", taskID))
		return false, nil
	}
	return setter.SetTaskWorkspaceMetadataIfUnchanged(ctx, taskID, guard, workspace)
}

// publishRepairedTask re-reads the task AFTER the write and publishes that
// (AC-003.5): re-reading before the write, or reusing the selection row,
// would broadcast the pre-write metadata — an explicit workspace_orphaned:
// false immediately after storing true — which preserveOmittedField cannot
// repair because the key is present rather than omitted. A failed re-read is
// logged at Warn and the pass continues: the marker is durably stored
// either way, and the next boot payload carries it (AC-003.8).
func (s *HandoffService) publishRepairedTask(ctx context.Context, taskID string) {
	if s.eventPublisher == nil {
		return
	}
	task, err := s.tasks.GetTask(ctx, taskID)
	if err != nil {
		s.logf().Warn("orphan marker repair: re-read after write failed", zap.String("task_id", taskID), zap.Error(err))
		return
	}
	s.eventPublisher.PublishTaskUpdated(ctx, task)
}
