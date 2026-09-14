package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"go.uber.org/zap"

	orchmodels "github.com/kandev/kandev/internal/office/models"
)

type managedDirectoryCreator interface {
	CreateManagedDirectory(path string, mode os.FileMode) error
}

// restoreCleanedGroups walks the workspace groups affected by an
// unarchive cascade and, for each one currently in cleanup_status=cleaned,
// recreates the materialized workspace (or marks it pending recreation)
// from restore_config_json. Failures flip the group to
// restore_status=restore_failed so callers can surface
// task.workspace_status="requires_configuration" via the context API.
//
// v1 strategy by kind:
//   - plain_folder: mkdir the path with the managed-root guard.
//   - single_repo / multi_repo: clear cleanup_status to active and set
//     restore_status=restorable; the next launch attempt picks task
//     repositories up from the existing task_repositories rows and
//     creates fresh worktrees through the standard worktree manager
//     path. We deliberately do NOT call the worktree manager here
//     because creating a worktree requires the parent task's repo
//     paths and base branches to be valid on disk — work that the
//     normal launch already handles correctly.
//   - remote_environment: same as repo kinds — restorable, deferred.
func (s *HandoffService) restoreCleanedGroups(ctx context.Context, groupIDs []string) error {
	if s.wsGroups == nil {
		return nil
	}
	var errs []error
	for _, gid := range groupIDs {
		g, err := s.wsGroups.GetWorkspaceGroup(ctx, gid)
		if err != nil {
			errs = append(errs, fmt.Errorf("load workspace group %s: %w", gid, err))
			continue
		}
		if g == nil {
			errs = append(errs, fmt.Errorf("workspace group %s not found", gid))
			continue
		}
		mu := s.workspaceGroupLock.lockFor(gid)
		mu.Lock()
		if g.CleanupStatus != orchmodels.WorkspaceCleanupStatusCleaned {
			mu.Unlock()
			continue
		}
		if err := s.restoreCleanedGroup(ctx, g); err != nil {
			s.logf().Error("restore cleaned group",
				zap.String("group_id", g.ID), zap.Error(err))
			statusErr := s.wsGroups.UpdateWorkspaceGroupRestoreStatus(ctx, g.ID,
				orchmodels.WorkspaceRestoreStatusFailed, err.Error())
			errs = append(errs, errors.Join(
				fmt.Errorf("restore workspace group %s: %w", gid, err), statusErr))
		}
		mu.Unlock()
	}

	return errors.Join(errs...)
}

func (s *HandoffService) restoreCleanedGroup(ctx context.Context, g *orchmodels.WorkspaceGroup) error {
	rc, err := decodeRestoreConfig(g.RestoreConfigJSON)
	if err != nil {
		return fmt.Errorf("decode restore_config_json: %w", err)
	}
	switch g.MaterializedKind {
	case orchmodels.WorkspaceGroupKindPlainFolder:
		return s.restorePlainFolder(ctx, g, rc)
	case orchmodels.WorkspaceGroupKindSingleRepo,
		orchmodels.WorkspaceGroupKindMultiRepo,
		orchmodels.WorkspaceGroupKindRemoteEnvironment:
		return s.markRestorable(ctx, g)
	default:
		return fmt.Errorf("unknown materialized kind: %q", g.MaterializedKind)
	}
}

// restorePlainFolder mkdirs the materialized path with 0o755 permissions.
// The same managed-root validator used by cleanup runs before mkdir, and the
// persisted restore path must agree with the workspace-group row.
func (s *HandoffService) restorePlainFolder(ctx context.Context, g *orchmodels.WorkspaceGroup, rc restoreConfig) error {
	if g.MaterializedPath == "" {
		return errors.New("plain folder restore: materialized_path is empty")
	}
	if rc.Path == "" {
		return errors.New("plain folder restore: restore path is empty")
	}
	materializedPath, err := filepath.Abs(filepath.Clean(g.MaterializedPath))
	if err != nil {
		return fmt.Errorf("plain folder restore: materialized path: %w", err)
	}
	restorePath, err := filepath.Abs(filepath.Clean(rc.Path))
	if err != nil {
		return fmt.Errorf("plain folder restore: restore path: %w", err)
	}
	if materializedPath != restorePath {
		return fmt.Errorf("plain folder restore: restore path %q does not match materialized path %q", rc.Path, g.MaterializedPath)
	}
	if s.cleaner == nil {
		return errors.New("plain folder restore: managed-root validator is not configured")
	}
	if err := s.cleaner.ValidateManagedRoot(g.MaterializedPath); err != nil {
		return err
	}
	if creator, ok := s.cleaner.(managedDirectoryCreator); ok {
		if err := creator.CreateManagedDirectory(g.MaterializedPath, 0o755); err != nil {
			return err
		}
	} else if err := os.MkdirAll(g.MaterializedPath, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", g.MaterializedPath, err)
	}
	if err := s.wsGroups.UpdateWorkspaceGroupCleanupStatus(ctx, g.ID,
		orchmodels.WorkspaceCleanupStatusActive, "", nil); err != nil {
		return err
	}
	return s.wsGroups.UpdateWorkspaceGroupRestoreStatus(ctx, g.ID,
		orchmodels.WorkspaceRestoreStatusRestored, "")
}

// markRestorable transitions a worktree-backed group out of cleaned and
// into "active + restorable". The next launch attempt for any active
// member will recreate the worktree(s) via the standard worktree
// manager path — task_repositories rows survive archive so the
// material to recreate from is intact.
func (s *HandoffService) markRestorable(ctx context.Context, g *orchmodels.WorkspaceGroup) error {
	if err := s.wsGroups.UpdateWorkspaceGroupCleanupStatus(ctx, g.ID,
		orchmodels.WorkspaceCleanupStatusActive, "", nil); err != nil {
		return err
	}
	return s.wsGroups.UpdateWorkspaceGroupRestoreStatus(ctx, g.ID,
		orchmodels.WorkspaceRestoreStatusRestorable, "")
}
