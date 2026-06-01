package worktree

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.uber.org/zap"
)

// buildWorktreeRecord constructs an in-memory Worktree value from a completed git worktree add.
func (m *Manager) buildWorktreeRecord(worktreeID string, req CreateRequest, worktreePath, branchName string) *Worktree {
	now := time.Now()
	return &Worktree{
		ID:             worktreeID,
		SessionID:      req.SessionID,
		TaskID:         req.TaskID,
		RepositoryID:   req.RepositoryID,
		BranchSlug:     SanitizeBranchSlug(req.BranchSlug),
		RepositoryPath: req.RepositoryPath,
		Path:           worktreePath,
		Branch:         branchName,
		BaseBranch:     req.BaseBranch,
		Status:         StatusActive,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
}

// persistAndCacheWorktree saves the worktree to the store and updates the in-memory cache.
func (m *Manager) persistAndCacheWorktree(ctx context.Context, wt *Worktree, req CreateRequest, worktreePath string) error {
	if m.store != nil {
		if err := m.persistWorktree(ctx, wt, req, worktreePath); err != nil {
			return err
		}
	}

	// Update cache keyed by (sessionID, repositoryID, branchSlug) so
	// multi-repo and multi-branch sessions can hold multiple entries. Use
	// wt.BranchSlug (already sanitized) so the read side via
	// GetBySessionAndRepo / tryReuseExisting, which calls
	// SanitizeBranchSlug on its input, lands on the same key.
	if req.SessionID != "" {
		m.mu.Lock()
		m.worktrees[cacheKey(req.SessionID, req.RepositoryID, wt.BranchSlug)] = wt
		m.mu.Unlock()
	}
	return nil
}

// persistWorktree writes the worktree to persistent storage, logging a warning when
// session_id is missing and cleaning up the git worktree directory on failure.
func (m *Manager) persistWorktree(ctx context.Context, wt *Worktree, req CreateRequest, worktreePath string) error {
	if req.SessionID == "" {
		m.logger.Warn("skipping worktree persistence: missing session_id",
			zap.String("task_id", req.TaskID),
			zap.String("worktree_id", wt.ID))
		return nil
	}
	if err := m.store.CreateWorktree(ctx, wt); err != nil {
		// Cleanup git worktree on store failure
		if cleanupErr := m.removeWorktreeDir(ctx, worktreePath, req.RepositoryPath); cleanupErr != nil {
			m.logger.Warn("failed to cleanup worktree after persist failure", zap.Error(cleanupErr))
		}
		return fmt.Errorf("failed to persist worktree: %w", err)
	}
	return nil
}

// cacheKey computes the cache key for a (sessionID, repositoryID, branchSlug)
// triple. Used by all read/write paths against m.worktrees so multi-repo and
// multi-branch sessions can hold multiple worktrees concurrently — without
// branchSlug, two worktrees of the same repo on different branches would
// share a key and silently collapse to one in-memory entry.
func cacheKey(sessionID, repositoryID, branchSlug string) string {
	return sessionID + "|" + repositoryID + "|" + branchSlug
}

// firstCacheEntryForSession scans the cache for any entry belonging to
// sessionID. Returns the first match or nil. Caller must hold m.mu.
func (m *Manager) firstCacheEntryForSession(sessionID string) *Worktree {
	prefix := sessionID + "|"
	for k, wt := range m.worktrees {
		if strings.HasPrefix(k, prefix) {
			return wt
		}
	}
	return nil
}

// GetBySessionID returns one worktree for the session. For multi-repo sessions
// it returns whichever worktree happens to come first; callers that need a
// specific repo's worktree should use GetBySessionAndRepo, and callers that
// need them all should use GetAllBySessionID.
func (m *Manager) GetBySessionID(ctx context.Context, sessionID string) (*Worktree, error) {
	// Check cache first
	m.mu.RLock()
	if wt := m.firstCacheEntryForSession(sessionID); wt != nil {
		m.mu.RUnlock()
		return wt, nil
	}
	m.mu.RUnlock()

	// Check store
	if m.store != nil {
		wt, err := m.store.GetWorktreeBySessionID(ctx, sessionID)
		if err != nil {
			return nil, err
		}
		if wt != nil {
			// Update cache under the wt's repository key (or empty for legacy
			// records that never had one).
			m.mu.Lock()
			m.worktrees[cacheKey(sessionID, wt.RepositoryID, wt.BranchSlug)] = wt
			m.mu.Unlock()
			return wt, nil
		}
	}

	return nil, ErrWorktreeNotFound
}

// GetAllBySessionID returns all active worktrees for a session. Empty slice
// (not error) if the session has none.
func (m *Manager) GetAllBySessionID(ctx context.Context, sessionID string) ([]*Worktree, error) {
	if m.store == nil {
		// Cache-only path: scan our in-memory map.
		m.mu.RLock()
		defer m.mu.RUnlock()
		prefix := sessionID + "|"
		var out []*Worktree
		for k, wt := range m.worktrees {
			if strings.HasPrefix(k, prefix) {
				out = append(out, wt)
			}
		}
		return out, nil
	}
	if multi, ok := m.store.(MultiRepoStore); ok {
		wts, err := multi.GetWorktreesBySessionID(ctx, sessionID)
		if err != nil {
			return nil, err
		}
		// Refresh cache while we're at it.
		m.mu.Lock()
		for _, wt := range wts {
			m.worktrees[cacheKey(sessionID, wt.RepositoryID, wt.BranchSlug)] = wt
		}
		m.mu.Unlock()
		return wts, nil
	}
	// Single-repo store fallback: at most one worktree per session.
	wt, err := m.store.GetWorktreeBySessionID(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if wt == nil {
		return nil, nil
	}
	return []*Worktree{wt}, nil
}

// GetBySessionAndRepo returns the active worktree for the
// (session, repo, branchSlug) triple, or ErrWorktreeNotFound if none exists.
// branchSlug scopes multi-branch tasks; passing "" matches the legacy
// single-branch shape so existing call sites continue to work unchanged.
// Falls back to GetBySessionID when the store does not implement
// MultiRepoStore (legacy single-repo).
func (m *Manager) GetBySessionAndRepo(ctx context.Context, sessionID, repositoryID, branchSlug string) (*Worktree, error) {
	if sessionID == "" {
		return nil, ErrWorktreeNotFound
	}
	// Check cache first.
	m.mu.RLock()
	if wt, ok := m.worktrees[cacheKey(sessionID, repositoryID, branchSlug)]; ok {
		m.mu.RUnlock()
		return wt, nil
	}
	m.mu.RUnlock()

	if m.store == nil {
		return nil, ErrWorktreeNotFound
	}
	if multi, ok := m.store.(MultiRepoStore); ok {
		wt, err := multi.GetWorktreeBySessionAndRepository(ctx, sessionID, repositoryID, branchSlug)
		if err != nil {
			return nil, err
		}
		if wt == nil {
			return nil, ErrWorktreeNotFound
		}
		m.mu.Lock()
		m.worktrees[cacheKey(sessionID, repositoryID, branchSlug)] = wt
		m.mu.Unlock()
		return wt, nil
	}
	// Legacy single-repo store: best we can do is the session lookup.
	wt, err := m.store.GetWorktreeBySessionID(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if wt == nil || (repositoryID != "" && wt.RepositoryID != repositoryID) {
		return nil, ErrWorktreeNotFound
	}
	return wt, nil
}

// GetByID returns a worktree by its unique ID.
func (m *Manager) GetByID(ctx context.Context, worktreeID string) (*Worktree, error) {
	if m.store == nil {
		return nil, ErrWorktreeNotFound
	}

	wt, err := m.store.GetWorktreeByID(ctx, worktreeID)
	if err != nil {
		return nil, err
	}
	if wt == nil {
		return nil, ErrWorktreeNotFound
	}
	return wt, nil
}

// GetAllByTaskID returns all worktrees for a task.
func (m *Manager) GetAllByTaskID(ctx context.Context, taskID string) ([]*Worktree, error) {
	if m.store == nil {
		return nil, nil
	}
	return m.store.GetWorktreesByTaskID(ctx, taskID)
}

// IsValid checks if a worktree directory is valid and usable.
func (m *Manager) IsValid(path string) bool {
	// Check directory exists
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return false
	}

	// Check .git file exists (worktrees have .git file, not directory)
	gitFile := filepath.Join(path, ".git")
	content, err := os.ReadFile(gitFile)
	if err != nil {
		return false
	}

	// .git file should contain "gitdir: <path>"
	if !strings.HasPrefix(string(content), "gitdir:") {
		return false
	}

	return true
}
