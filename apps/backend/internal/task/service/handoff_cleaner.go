package service

import (
	"context"
	"errors"
	"fmt"

	orchmodels "github.com/kandev/kandev/internal/office/models"
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
	rc, _ := decodeRestoreConfig(g.RestoreConfigJSON)
	switch g.MaterializedKind {
	case orchmodels.WorkspaceGroupKindPlainFolder:
		if g.MaterializedPath == "" {
			return errors.New("plain folder cleanup: materialized_path is empty")
		}
		return s.cleaner.CleanupPlainFolder(ctx, g.MaterializedPath)
	case orchmodels.WorkspaceGroupKindSingleRepo:
		if len(rc.WorktreeIDs) == 0 {
			return errors.New("single-repo cleanup: no worktree IDs in restore_config_json")
		}
		for _, wtID := range rc.WorktreeIDs {
			if err := s.cleaner.CleanupSingleRepoWorktree(ctx, wtID); err != nil {
				return fmt.Errorf("worktree %s: %w", wtID, err)
			}
		}
		return nil
	case orchmodels.WorkspaceGroupKindMultiRepo:
		ids := make([]string, 0, len(rc.WorktreeIDs))
		for _, id := range rc.WorktreeIDs {
			ids = append(ids, id)
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
