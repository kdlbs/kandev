package worktree

import (
	"context"
	"errors"
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
		ID:                worktreeID,
		SessionID:         req.SessionID,
		TaskID:            req.TaskID,
		TaskDirName:       req.TaskDirName,
		TaskEnvironmentID: req.TaskEnvironmentID,
		RepositoryID:      req.RepositoryID,
		BranchSlug:        requestBranchIdentitySlug(req),
		RepositoryPath:    req.RepositoryPath,
		Path:              worktreePath,
		Branch:            branchName,
		BaseBranch:        req.BaseBranch,
		Status:            StatusActive,
		CreatedAt:         now,
		UpdatedAt:         now,
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

// persistWorktree writes the worktree to persistent storage, logging a debug
// entry when session_id or a resolvable task environment is missing and
// cleaning up the git worktree directory on failure.
//
// Initial materialization runs before the executor persists the task
// environment, so the store cannot resolve an environment yet — that launch's
// persistence happens atomically in persistTaskEnvironment, and a worktree
// that cannot be recorded there is compensated. Resume, recreate, and
// mid-session materialization paths always have an environment and persist
// here.
func (m *Manager) persistWorktree(ctx context.Context, wt *Worktree, req CreateRequest, worktreePath string) error {
	if req.SessionID == "" {
		m.logger.Warn("skipping worktree persistence: missing session_id",
			zap.String("task_id", req.TaskID),
			zap.String("worktree_id", wt.ID))
		return nil
	}
	if err := m.store.CreateWorktree(ctx, wt); err != nil {
		if errors.Is(err, ErrEnvironmentNotResolved) {
			m.logger.Debug("skipping worktree persistence: session has no task environment yet",
				zap.String("task_id", req.TaskID),
				zap.String("session_id", req.SessionID),
				zap.String("worktree_id", wt.ID))
			return nil
		}
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

type linkedWorktreeClass string

const (
	linkedWorktreeHealthy          linkedWorktreeClass = "healthy"
	linkedWorktreeMissingAdmin     linkedWorktreeClass = "missing-admin-target"
	linkedWorktreeBacklinkMismatch linkedWorktreeClass = "backlink-mismatch"
	linkedWorktreeAmbiguous        linkedWorktreeClass = "ambiguous"
)

type linkedWorktreeInspection struct {
	class            linkedWorktreeClass
	adminPath        string
	expectedBacklink string
	actualBacklink   string
	reason           string
}

// IsValid checks if a worktree directory is valid and usable.
func (m *Manager) IsValid(path string) bool {
	return inspectLinkedWorktree(path).class == linkedWorktreeHealthy
}

//nolint:cyclop // Each metadata boundary must classify independently and fail closed.
func inspectLinkedWorktree(path string) linkedWorktreeInspection {
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return linkedWorktreeInspection{class: linkedWorktreeAmbiguous, reason: "checkout path is not a directory"}
	}

	gitFile := filepath.Join(path, ".git")
	gitInfo, err := os.Lstat(gitFile)
	if err != nil || !gitInfo.Mode().IsRegular() {
		return linkedWorktreeInspection{class: linkedWorktreeAmbiguous, reason: "checkout git pointer is not a regular file"}
	}
	content, err := os.ReadFile(gitFile)
	if err != nil {
		return linkedWorktreeInspection{class: linkedWorktreeAmbiguous, reason: "cannot read checkout git pointer"}
	}
	adminPath, found := strings.CutPrefix(strings.TrimSpace(string(content)), "gitdir:")
	if !found {
		return linkedWorktreeInspection{class: linkedWorktreeAmbiguous, reason: "checkout git pointer has no gitdir target"}
	}
	adminPath = strings.TrimSpace(adminPath)
	if adminPath == "" || !filepath.IsAbs(adminPath) {
		return linkedWorktreeInspection{class: linkedWorktreeAmbiguous, adminPath: adminPath, reason: "checkout git pointer has an invalid gitdir target"}
	}
	adminInfo, err := os.Lstat(adminPath)
	if err != nil || !adminInfo.IsDir() || adminInfo.Mode()&os.ModeSymlink != 0 {
		return linkedWorktreeInspection{class: linkedWorktreeMissingAdmin, adminPath: adminPath, reason: fmt.Sprintf("linked-worktree admin target %q is missing", adminPath)}
	}
	commonDir, err := linkedWorktreeCommonDir(adminPath)
	if err != nil {
		return linkedWorktreeInspection{class: linkedWorktreeAmbiguous, adminPath: adminPath, reason: err.Error()}
	}
	if info, statErr := os.Lstat(commonDir); statErr != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return linkedWorktreeInspection{class: linkedWorktreeAmbiguous, adminPath: adminPath, reason: fmt.Sprintf("linked-worktree common directory %q is unavailable", commonDir)}
	}

	backlinkPath := filepath.Join(adminPath, "gitdir")
	backlinkInfo, err := os.Lstat(backlinkPath)
	if err != nil || !backlinkInfo.Mode().IsRegular() {
		return linkedWorktreeInspection{class: linkedWorktreeAmbiguous, adminPath: adminPath, reason: "linked-worktree admin target has no reciprocal gitdir backlink"}
	}
	backlink, err := os.ReadFile(backlinkPath)
	if err != nil {
		return linkedWorktreeInspection{class: linkedWorktreeAmbiguous, adminPath: adminPath, reason: "cannot read linked-worktree admin backlink"}
	}
	backlinkTarget := strings.TrimSpace(string(backlink))
	if backlinkTarget == "" {
		return linkedWorktreeInspection{class: linkedWorktreeAmbiguous, adminPath: adminPath, reason: "linked-worktree admin backlink is empty"}
	}
	expectedBacklink, err := filepath.Abs(gitFile)
	if err != nil {
		return linkedWorktreeInspection{class: linkedWorktreeAmbiguous, adminPath: adminPath, actualBacklink: backlinkTarget, reason: "cannot resolve expected checkout backlink"}
	}
	actualBacklink, err := filepath.Abs(backlinkTarget)
	if err != nil || actualBacklink != expectedBacklink {
		return linkedWorktreeInspection{class: linkedWorktreeBacklinkMismatch, adminPath: adminPath, expectedBacklink: expectedBacklink, actualBacklink: backlinkTarget, reason: fmt.Sprintf("linked-worktree admin target %q backlink mismatch", adminPath)}
	}

	return linkedWorktreeInspection{class: linkedWorktreeHealthy, adminPath: adminPath, expectedBacklink: expectedBacklink, actualBacklink: actualBacklink}
}

func linkedWorktreeCommonDir(adminPath string) (string, error) {
	commonDirPath := filepath.Join(adminPath, "commondir")
	info, err := os.Lstat(commonDirPath)
	if err != nil || !info.Mode().IsRegular() {
		return "", fmt.Errorf("linked-worktree admin target %q has no commondir", adminPath)
	}
	contents, err := os.ReadFile(commonDirPath)
	if err != nil {
		return "", fmt.Errorf("cannot read linked-worktree commondir")
	}
	commonDir := strings.TrimSpace(string(contents))
	if commonDir == "" || filepath.IsAbs(commonDir) {
		return "", fmt.Errorf("linked-worktree commondir is invalid")
	}
	resolved, err := filepath.Abs(filepath.Join(adminPath, commonDir))
	if err != nil {
		return "", fmt.Errorf("cannot resolve linked-worktree commondir")
	}
	return resolved, nil
}

func linkedWorktreeIntegrityReason(path string) string {
	inspection := inspectLinkedWorktree(path)
	if inspection.class == linkedWorktreeBacklinkMismatch {
		return fmt.Sprintf("%s: expected %q, actual %q", inspection.reason, inspection.expectedBacklink, inspection.actualBacklink)
	}
	if inspection.reason != "" {
		return inspection.reason
	}
	return "linked-worktree metadata is invalid"
}

func isAdminDirectoryMissing(worktreePath string) bool {
	content, err := os.ReadFile(filepath.Join(worktreePath, ".git"))
	if err != nil {
		return false
	}
	adminPath, found := strings.CutPrefix(strings.TrimSpace(string(content)), "gitdir:")
	if !found {
		return false
	}
	info, statErr := os.Lstat(strings.TrimSpace(adminPath))
	return statErr != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0
}

// linkedWorktreeRecoveryReason distinguishes a recoverable missing admin entry
// from content that has no validated Git commit to attach to. A branch name
// stored in the worktree row is not enough: reconstructing HEAD when its ref
// is gone would create an unborn branch and make the preserved checkout appear
// entirely deleted or untracked.
func (m *Manager) linkedWorktreeRecoveryReason(ctx context.Context, wt *Worktree) string {
	reason := linkedWorktreeIntegrityReason(wt.Path)
	if !isAdminDirectoryMissing(wt.Path) ||
		wt.RepositoryPath == "" || wt.Branch == "" {
		return reason + "; checkout preserved"
	}

	exists, err := m.branchExists(ctx, wt.RepositoryPath, "refs/heads/"+wt.Branch)
	if err != nil {
		return reason + "; branch validation unavailable; checkout preserved"
	}
	if !exists {
		return fmt.Sprintf("%s; recorded branch %q has no reachable ref; content-only preservation required", reason, wt.Branch)
	}
	return reason + "; checkout preserved"
}
