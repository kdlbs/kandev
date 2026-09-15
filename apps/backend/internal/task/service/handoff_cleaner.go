package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	orchmodels "github.com/kandev/kandev/internal/office/models"
	"go.uber.org/zap"
)

// WorkspaceCleaner is the disk-cleanup surface evaluateWorkspaceGroupCleanup
// invokes once a Kandev-owned group's last active member is released and
// no active sessions reference its materialized environment. The
// implementation lives in the worktree package (handoff_cleanup.go in
// internal/worktree) and threads the managed-root guard before any
// destructive operation.
//
// Every method returns nil to signal a successful cleanup; any error
// causes evaluateWorkspaceGroupCleanup to flip cleanup_status to
// cleanup_failed with the error message attached, leaving the disk
// state untouched so a follow-up pass can retry.
type WorkspaceCleaner interface {
	// ValidateManagedRoot checks that a path resolves under a configured
	// Kandev-managed root without performing filesystem mutation.
	ValidateManagedRoot(path string) error
	// CleanupPlainFolder removes a Kandev-owned plain folder. The
	// implementation must reject paths outside the configured
	// kandev-managed roots.
	CleanupPlainFolder(ctx context.Context, path string) error
	// CleanupSingleRepoWorktree removes a single git worktree by ID.
	CleanupSingleRepoWorktree(ctx context.Context, worktreeID string) error
	// CleanupMultiRepoRoot removes every per-repo worktree under a
	// multi-repo task root, then removes the root directory itself.
	CleanupMultiRepoRoot(ctx context.Context, rootPath string, worktreeIDs []string) error
	// CleanupRemoteEnvironment deletes a remote environment via its
	// provider. The provider+id are read out of the group's restore
	// config; the implementation refuses unknown providers.
	CleanupRemoteEnvironment(ctx context.Context, provider, environmentID string) error
}

const workspaceGroupCleanupPollInterval = 100 * time.Millisecond

// CleanupWorkspaceGroups removes materialized Kandev-owned workspace groups
// before workspace deletion removes the rows containing their cleanup handles.
func (s *HandoffService) CleanupWorkspaceGroups(ctx context.Context, workspaceID string) error {
	if s.wsGroups == nil {
		return errors.New("workspace group repository is not configured")
	}
	groups, err := s.wsGroups.ListWorkspaceGroupsByWorkspace(ctx, workspaceID)
	if err != nil {
		return err
	}
	if s.cleaner == nil {
		for _, g := range groups {
			if shouldCleanupWorkspaceGroup(g) {
				return errors.New("workspace group cleaner is not configured")
			}
		}
		return nil
	}
	statusCtx, cancelStatus := detachedCleanupTransitionContext(ctx)
	defer cancelStatus()
	for _, g := range groups {
		if !shouldCleanupWorkspaceGroup(g) {
			continue
		}
		mu := s.workspaceGroupLock.lockFor(g.ID)
		mu.Lock()
		hasActive, err := s.hasActiveExecutionsForGroup(ctx, g.ID)
		if err != nil {
			mu.Unlock()
			return fmt.Errorf("check active workspace group %s: %w", g.ID, err)
		}
		if hasActive {
			s.logf().Warn("workspace group cleanup: active executions remain",
				zap.String("workspace_id", workspaceID),
				zap.String("group_id", g.ID))
			if err := s.waitForWorkspaceGroupIdle(ctx, workspaceID, g.ID); err != nil {
				mu.Unlock()
				return err
			}
		}
		claimed, err := claimWorkspaceGroupCleanup(statusCtx, s.wsGroups, g)
		if err != nil {
			mu.Unlock()
			return err
		}
		if !claimed {
			mu.Unlock()
			continue
		}
		if err := s.runWorkspaceGroupCleanup(ctx, g); err != nil {
			_ = completeWorkspaceGroupCleanup(statusCtx, s.wsGroups, g,
				orchmodels.WorkspaceCleanupStatusFailed, err.Error(), nil)
			mu.Unlock()
			return fmt.Errorf("clean workspace group %s: %w", g.ID, err)
		}
		now := time.Now().UTC()
		if err := completeWorkspaceGroupCleanup(statusCtx, s.wsGroups, g,
			orchmodels.WorkspaceCleanupStatusCleaned, "", &now); err != nil {
			mu.Unlock()
			return err
		}
		mu.Unlock()
	}
	return nil
}

func shouldCleanupWorkspaceGroup(g *orchmodels.WorkspaceGroup) bool {
	return g != nil &&
		g.OwnedByKandev &&
		g.CleanupPolicy == orchmodels.WorkspaceCleanupPolicyDeleteWhenLastMemberArchivedOrDel &&
		g.CleanupStatus != orchmodels.WorkspaceCleanupStatusCleaned
}

func (s *HandoffService) waitForWorkspaceGroupIdle(ctx context.Context, workspaceID, groupID string) error {
	ticker := time.NewTicker(workspaceGroupCleanupPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("workspace group %s still has active executions: %w", groupID, ctx.Err())
		case <-ticker.C:
			hasActive, err := s.hasActiveExecutionsForGroup(ctx, groupID)
			if err != nil {
				return fmt.Errorf("check active workspace group %s: %w", groupID, err)
			}
			if !hasActive {
				s.logf().Info("workspace group cleanup: active executions stopped",
					zap.String("workspace_id", workspaceID),
					zap.String("group_id", groupID))
				return nil
			}
		}
	}
}

// runWorkspaceGroupCleanup is the dispatcher invoked by
// evaluateWorkspaceGroupCleanup once it confirms the group is owned by
// Kandev, has no live members, and is configured to delete on last
// release. The dispatcher is wired through the WorkspaceCleaner
// interface so the disk-touching code lives in one well-tested package
// (internal/worktree) and the office service stays platform-agnostic.
func (s *HandoffService) runWorkspaceGroupCleanup(ctx context.Context, g *orchmodels.WorkspaceGroup) error {
	if s.cleaner == nil {
		// No cleaner configured (legacy / tests) — fall through. The
		// state machine already moved cleanup_status to cleanup_pending
		// so the operator sees the group is awaiting cleanup.
		return nil
	}
	if g.MaterializedKind == orchmodels.WorkspaceGroupKindPlainFolder {
		if g.MaterializedPath == "" {
			return errors.New("plain folder cleanup: materialized_path is empty")
		}
		return s.cleaner.CleanupPlainFolder(ctx, g.MaterializedPath)
	}
	rc, err := decodeRestoreConfig(g.RestoreConfigJSON)
	if err != nil {
		return fmt.Errorf("decode restore_config_json: %w", err)
	}
	switch g.MaterializedKind {
	case orchmodels.WorkspaceGroupKindSingleRepo:
		ids, err := restoreWorktreeIDs(rc.WorktreeIDs, "single-repo")
		if err != nil {
			return err
		}
		for _, wtID := range ids {
			if err := s.cleaner.CleanupSingleRepoWorktree(ctx, wtID); err != nil {
				return fmt.Errorf("worktree %s: %w", wtID, err)
			}
		}
		return nil
	case orchmodels.WorkspaceGroupKindMultiRepo:
		ids, err := restoreWorktreeIDs(rc.WorktreeIDs, "multi-repo")
		if err != nil {
			return err
		}
		return s.cleaner.CleanupMultiRepoRoot(ctx, g.MaterializedPath, ids)
	case orchmodels.WorkspaceGroupKindRemoteEnvironment:
		// remote_env is encoded as the env id; provider lookup is
		// future-work — for now treat any non-empty value as the
		// provider's environment id.
		return s.cleaner.CleanupRemoteEnvironment(ctx, "", rc.RemoteEnv)
	default:
		return fmt.Errorf("unknown materialized kind: %q", g.MaterializedKind)
	}

}

func restoreWorktreeIDs(worktreeIDs map[string]string, kind string) ([]string, error) {
	if len(worktreeIDs) == 0 {
		return nil, fmt.Errorf("%s cleanup: no worktree IDs in restore_config_json", kind)
	}
	ids := make([]string, 0, len(worktreeIDs))
	for repositoryID, worktreeID := range worktreeIDs {
		if strings.TrimSpace(repositoryID) == "" || strings.TrimSpace(worktreeID) == "" {
			return nil, fmt.Errorf("%s cleanup: restore_config_json contains an empty worktree identity", kind)
		}
		ids = append(ids, worktreeID)
	}
	return ids, nil
}
