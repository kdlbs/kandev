package worktree

import (
	"context"
	"errors"
	"fmt"
	"go.uber.org/zap"
	"os"
	"path/filepath"
	"strings"
)

func (m *Manager) enrichCleanupWorktreeFromCache(ctx context.Context, wt *Worktree) *Worktree {
	if wt == nil {
		return nil
	}
	clone := *wt
	wt = &clone
	m.enrichCleanupWorktreeFromCachedFields(wt, m.cachedCleanupWorktree(wt))
	m.enrichCleanupWorktreeFromStore(ctx, wt)
	return wt
}

func (m *Manager) cachedCleanupWorktree(wt *Worktree) *Worktree {
	m.mu.RLock()
	cached := m.worktrees[cacheKey(wt.SessionID, wt.RepositoryID, wt.BranchSlug)]
	if cached == nil {
		for _, candidate := range m.worktrees {
			if candidate != nil && candidate.ID == wt.ID {
				cached = candidate
				break
			}
		}
	}
	m.mu.RUnlock()
	return cached
}

func (m *Manager) enrichCleanupWorktreeFromCachedFields(wt, cached *Worktree) {
	if cached == nil || cached.ID != wt.ID {
		return
	}
	if wt.RepositoryPath == "" {
		wt.RepositoryPath = cached.RepositoryPath
	}
	if wt.BaseBranch == "" {
		wt.BaseBranch = cached.BaseBranch
	}
	if wt.CleanupHeadOID == "" && !wt.CleanupHeadOIDUnavailable {
		wt.CleanupHeadOID = cached.CleanupHeadOID
	}
	if wt.BranchOwner == "" {
		wt.BranchOwner = cached.BranchOwner
	}
	if wt.IntegrationRef == "" {
		wt.IntegrationRef = cached.IntegrationRef
	}
	if wt.RecoveryHeadSHA == "" {
		wt.RecoveryHeadSHA = cached.RecoveryHeadSHA
	}
	if wt.BranchCompactedAt == nil {
		wt.BranchCompactedAt = cached.BranchCompactedAt
	}
}

func (m *Manager) enrichCleanupWorktreeFromStore(ctx context.Context, wt *Worktree) {
	if m.store == nil || wt.ID == "" {
		return
	}
	current, err := m.store.GetWorktreeByID(ctx, wt.ID)
	if err != nil || current == nil || current.ID != wt.ID {
		return
	}
	// A durable cleanup snapshot can outlive the manager cache. Reload the
	// internal branch state from the current row when it is available so a
	// newer recovery or compaction transition always wins over stale snapshot
	// data.
	wt.BranchOwner = current.BranchOwner
	wt.IntegrationRef = current.IntegrationRef
	wt.RecoveryHeadSHA = current.RecoveryHeadSHA
	wt.BranchCompactedAt = current.BranchCompactedAt
}

// CaptureCleanupHeadOIDs records the checkout commit for each worktree before
// a durable task cleanup job is persisted. Cleanup later compares this value
// with the live branch and registration so a replacement checkout cannot be
// mistaken for the original task workspace.
func (m *Manager) CaptureCleanupHeadOIDs(ctx context.Context, worktrees []*Worktree) (map[string]string, error) {
	identities := make(map[string]string, len(worktrees))
	for _, wt := range worktrees {
		if wt == nil || wt.ID == "" {
			continue
		}
		if err := m.checkCleanupInventoryIdentity(ctx, wt.ID); err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return nil, classifyCleanupInspectionError(CleanupInspectionStageCommit, err)
			}
			return nil, err
		}

		wt = m.enrichCleanupWorktreeFromCache(ctx, wt)
		if strings.TrimSpace(wt.RepositoryPath) == "" || strings.TrimSpace(wt.Path) == "" {
			// Older environment rows can outlive their repository/session rows.
			// Keep them in the durable snapshot, but let the later cleanup audit
			// fail closed instead of rejecting the task mutation itself.
			continue
		}
		oid, found, err := m.captureCleanupHeadOID(ctx, wt)
		if err != nil {
			return nil, err
		}
		if found {
			identities[wt.ID] = oid
		}
	}
	return identities, nil
}

func (m *Manager) captureCleanupHeadOID(ctx context.Context, wt *Worktree) (string, bool, error) {
	// A cleanup snapshot can race another job removing this checkout. Use the
	// same path-then-repository lock order as destructive worktree operations
	// so the path and branch identity are observed consistently.
	releasePath, err := acquireWorktreeTargetPath(ctx, wt.Path)
	if err != nil {
		return "", false, fmt.Errorf("lock cleanup identity path for %s: %w", wt.ID,
			classifyCleanupInspectionError(CleanupInspectionStageCommit, err))
	}
	defer releasePath()

	repoLock := m.getRepoLock(wt.RepositoryPath)
	repoLock.Lock()
	defer func() {
		repoLock.Unlock()
		m.releaseRepoLock(wt.RepositoryPath)
	}()

	checkoutPresent, err := cleanupCheckoutGitPresent(wt.Path)
	if err != nil {
		return "", false, fmt.Errorf("capture cleanup identity for %s: %w", wt.ID, err)
	}
	if !checkoutPresent {
		oid, found, err := m.captureCleanupIdentityFromBranch(ctx, wt)
		if err != nil {
			return "", false, fmt.Errorf("capture cleanup identity for %s: %w", wt.ID, err)
		}
		return oid, found, nil
	}

	output, err := m.runCheckoutLocalGitInspect(ctx, wt.Path, "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil {
		// Even under our own lock, an external actor (a raw filesystem cleanup,
		// not another Manager operation) can remove the checkout or its Git
		// metadata between inspection and this invocation. Capture the branch
		// identity when checkout-local Git metadata is confirmed absent.
		checkoutPresent, pathErr := cleanupCheckoutGitPresent(wt.Path)
		if pathErr != nil {
			return "", false, fmt.Errorf("capture cleanup identity for %s: %w", wt.ID, pathErr)
		}
		if !checkoutPresent {
			oid, found, branchErr := m.captureCleanupIdentityFromBranch(ctx, wt)
			if branchErr != nil {
				return "", false, fmt.Errorf("capture cleanup identity for %s: %w", wt.ID, branchErr)
			}
			return oid, found, nil
		}
		return "", false, fmt.Errorf("capture cleanup identity for %s: %w", wt.ID,
			classifyWorktreeCleanupInspectionError(CleanupInspectionStageCommit, err, wt))
	}
	oid, err := parseCleanupCommitOID(output)
	if err != nil {
		return "", false, fmt.Errorf("capture cleanup identity for %s: %w", wt.ID, err)
	}
	return oid, true, nil
}

// runCheckoutLocalGitInspect pins Git to the checkout's own .git. The presence
// check cannot hold off an external actor, so if that metadata disappears
// before Git starts, the inspection fails instead of discovering an enclosing
// repository and returning its HEAD as this worktree's identity.
func (m *Manager) runCheckoutLocalGitInspect(ctx context.Context, path string, args ...string) (string, error) {
	gitDir := "GIT_DIR=.git"
	return m.runBoundedGitInspectWithEnvironment(ctx, path, func(env []string) []string {
		return append(env, gitDir)
	}, args...)
}

func cleanupCheckoutGitPresent(path string) (bool, error) {
	present, err := cleanupPathPresent(path)
	if err != nil {
		return false, classifyCleanupInspectionError(CleanupInspectionStageCommit, err)
	}
	if !present {
		return false, nil
	}
	// Git can discover an ancestor repository when the checkout has no .git.
	// Only checkout-local metadata may supply this worktree's HEAD identity.
	_, err = os.Lstat(filepath.Join(path, ".git"))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, classifyCleanupInspectionError(CleanupInspectionStageCommit, err)
	}
	return true, nil
}

func (m *Manager) captureCleanupIdentityFromBranch(ctx context.Context, wt *Worktree) (string, bool, error) {
	branch := strings.TrimSpace(wt.Branch)
	if branch == "" {
		m.logger.Warn("cleanup worktree path is absent and branch is unknown",
			zap.String("task_id", wt.TaskID),
			zap.String("worktree_id", wt.ID),
			zap.String("repository_path", wt.RepositoryPath),
			zap.String("reason", "empty branch on environment row"))
		return "", false, nil
	}
	branchRef := "refs/heads/" + branch
	oid, found, err := m.captureCleanupBranchOID(ctx, wt.RepositoryPath, branchRef)
	if err != nil {
		return "", false, err
	}
	if !found {
		m.logger.Warn("cleanup worktree path and branch are absent",
			zap.String("task_id", wt.TaskID),
			zap.String("worktree_id", wt.ID),
			zap.String("repository_path", wt.RepositoryPath),
			zap.String("branch", branch),
			zap.String("reason", "local branch ref not found"))
		return "", false, nil
	}
	return oid, true, nil
}

// A snapshot or cached path cannot authorize cleanup of an ambiguous identity.
func (m *Manager) checkCleanupInventoryIdentity(ctx context.Context, id string) error {
	if m.store == nil {
		return nil
	}
	_, err := m.store.GetWorktreeByID(ctx, id)
	if errors.Is(err, ErrWorktreeNotFound) {
		return nil
	}
	return err
}
