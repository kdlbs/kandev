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

	"github.com/kandev/kandev/internal/system/storage"
	storageworkspaces "github.com/kandev/kandev/internal/system/storage/workspaces"
)

func (m *Manager) PruneQuarantinedWorkspace(ctx context.Context, entry storage.QuarantineEntry) error {
	worktrees, err := m.GetAllByTaskID(ctx, entry.TaskID)
	if err != nil {
		return err
	}
	seen := make(map[string]struct{})
	var errs []error
	for _, wt := range worktrees {
		if wt == nil || wt.RepositoryPath == "" || wt.Path == "" {
			continue
		}
		key := wt.RepositoryPath + "\x00" + filepath.Clean(wt.Path)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		if err := m.pruneQuarantinedWorktree(ctx, wt); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (m *Manager) pruneQuarantinedWorktree(ctx context.Context, wt *Worktree) error {
	repoLock := m.getRepoLock(wt.RepositoryPath)
	repoLock.Lock()
	defer func() {
		repoLock.Unlock()
		m.releaseRepoLock(wt.RepositoryPath)
	}()

	cmd := newGitCommand(ctx, "worktree", "remove", "--force", wt.Path)
	cmd.Dir = wt.RepositoryPath
	if _, err := runGitCmdCombinedOutput(ctx, cmd); err != nil {
		present, inspectErr := worktreeRegistrationExists(ctx, wt.RepositoryPath, wt.Path)
		if inspectErr != nil {
			return fmt.Errorf("verify worktree registration for %s: %w", wt.Path, inspectErr)
		}
		if present {
			return fmt.Errorf("remove worktree registration for %s: %w", wt.Path, err)
		}
	}
	return nil
}

func worktreeRegistrationExists(ctx context.Context, repoPath, worktreePath string) (bool, error) {
	cmd := newGitCommand(ctx, "worktree", "list", "--porcelain", "-z")
	cmd.Dir = repoPath
	output, err := runGitCmdCombinedOutput(ctx, cmd)
	if err != nil {
		return false, err
	}
	want := filepath.Clean(worktreePath)
	for _, field := range strings.Split(string(output), "\x00") {
		if path, ok := strings.CutPrefix(field, "worktree "); ok && filepath.Clean(path) == want {
			return true, nil
		}
	}
	return false, nil
}

func inspectWorktreeRegistrationOwnership(
	ctx context.Context, repoPath, worktreePath, branchRef, headOID string,
) (bool, error) {
	cmd := newGitCommand(ctx, "worktree", "list", "--porcelain", "-z")
	cmd.Dir = repoPath
	output, err := runGitCmdCombinedOutput(ctx, cmd)
	if err != nil {
		return false, err
	}
	wantPath, err := normalizedWorktreeTargetPath(worktreePath)
	if err != nil {
		return false, err
	}
	var exactTarget, branchElsewhere bool
	var normalizationErr error
	var currentPath, currentHead, currentBranch string
	inspectRegistration := func() {
		if currentBranch != branchRef || normalizationErr != nil {
			return
		}
		currentPathKey, normalizeErr := normalizedWorktreeTargetPath(currentPath)
		if normalizeErr != nil {
			normalizationErr = normalizeErr
			return
		}
		if currentPathKey == wantPath && currentHead == headOID {
			exactTarget = true
			return
		}
		branchElsewhere = true
	}
	for _, field := range strings.Split(string(output), "\x00") {
		switch {
		case strings.HasPrefix(field, "worktree "):
			inspectRegistration()
			currentPath = strings.TrimPrefix(field, "worktree ")
			currentHead = ""
			currentBranch = ""
		case strings.HasPrefix(field, "HEAD "):
			currentHead = strings.TrimPrefix(field, "HEAD ")
		case strings.HasPrefix(field, "branch "):
			currentBranch = strings.TrimPrefix(field, "branch ")
		}
	}
	inspectRegistration()
	if normalizationErr != nil {
		return false, normalizationErr
	}
	return exactTarget && !branchElsewhere, nil
}

// RemoveByID removes a specific worktree by its ID and optionally its branch.
func (m *Manager) RemoveByID(ctx context.Context, worktreeID string, removeBranch bool) error {
	wt, err := m.GetByID(ctx, worktreeID)
	if err != nil {
		return err
	}
	return m.removeWorktree(ctx, wt, removeBranch)
}

// removeWorktree performs the actual removal of a worktree.
func (m *Manager) removeWorktree(ctx context.Context, wt *Worktree, removeBranch bool) error {
	// Get repository lock
	repoLock := m.getRepoLock(wt.RepositoryPath)
	repoLock.Lock()
	defer func() {
		repoLock.Unlock()
		m.releaseRepoLock(wt.RepositoryPath)
	}()
	activeReferences, err := m.CountActiveWorktreeReferences(ctx, wt.ID, []string{wt.SessionID})
	if err != nil {
		return fmt.Errorf("count active references for worktree %s: %w", wt.ID, err)
	}
	if activeReferences > 0 {
		if err := m.ReleaseWorktreeReference(ctx, wt); err != nil {
			return fmt.Errorf("release shared worktree reference %s: %w", wt.ID, err)
		}
		m.logger.Info("preserved worktree still referenced by another task session",
			zap.String("worktree_id", wt.ID),
			zap.String("session_id", wt.SessionID),
			zap.Int("non_deleted_references", activeReferences))
		return nil
	}

	// Execute cleanup script BEFORE removing directory
	m.runWorktreeCleanupScript(ctx, wt)

	// Remove worktree directory
	if err := m.removeWorktreeDir(ctx, wt.Path, wt.RepositoryPath); err != nil {
		m.logger.Warn("failed to remove worktree directory",
			zap.String("path", wt.Path),
			zap.Error(err))
	}

	// Optionally remove the branch from the main repository
	if removeBranch {
		m.logger.Info("deleting branch from main repository",
			zap.String("branch", wt.Branch),
			zap.String("repository_path", wt.RepositoryPath))

		cmd := newGitCommand(ctx, "branch", "-D", wt.Branch)
		cmd.Dir = wt.RepositoryPath
		if output, err := runGitCmdCombinedOutput(ctx, cmd); err != nil {
			m.logger.Warn("failed to delete branch from main repository",
				zap.String("branch", wt.Branch),
				zap.String("repository_path", wt.RepositoryPath),
				zap.String("output", string(output)),
				zap.Error(err))
		} else {
			m.logger.Info("successfully deleted branch from main repository",
				zap.String("branch", wt.Branch),
				zap.String("repository_path", wt.RepositoryPath))
		}
	}

	// Update store
	if m.store != nil {
		if err := m.ReleaseWorktreeReference(ctx, wt); err != nil {
			// Record may already be deleted by another cleanup path (e.g. task deletion).
			// This is expected and harmless - only log at debug level.
			m.logger.Debug("failed to update worktree status (may already be deleted)",
				zap.String("worktree_id", wt.ID),
				zap.Error(err))
		}
	}

	// Update cache: delete the (session, repo) entry. Removing a worktree only
	// affects its own repo; siblings on other repos must remain cached.
	m.mu.Lock()
	if wt.SessionID != "" {
		delete(m.worktrees, cacheKey(wt.SessionID, wt.RepositoryID, wt.BranchSlug))
	}
	m.mu.Unlock()

	m.logger.Info("removed worktree",
		zap.String("task_id", wt.TaskID),
		zap.String("worktree_id", wt.ID),
		zap.String("path", wt.Path),
		zap.Bool("branch_removed", removeBranch))

	return nil
}

// ReleaseWorktreeReference marks one session's association deleted without
// removing the shared directory or branch.
func (m *Manager) ReleaseWorktreeReference(ctx context.Context, wt *Worktree) error {
	if wt == nil || wt.SessionID == "" {
		return fmt.Errorf("session ID is required to release worktree reference")
	}
	now := time.Now().UTC()
	wt.Status = StatusDeleted
	wt.DeletedAt = &now
	wt.UpdatedAt = now
	if err := m.store.UpdateWorktree(ctx, wt); err != nil && !errors.Is(err, ErrWorktreeNotFound) {
		return err
	}
	m.mu.Lock()
	delete(m.worktrees, cacheKey(wt.SessionID, wt.RepositoryID, wt.BranchSlug))
	m.mu.Unlock()
	return nil
}

// runWorktreeSetupScript runs the repository's setup script in the freshly
// created worktree. Setup script failures are non-fatal: the worktree is kept
// and the failure is recorded on wt as a warning (surfaced by the env preparer)
// so the agent can still launch. This mirrors the task-level setup script
// behavior in runSetupScriptStep — a broken setup script must not block the task.
func (m *Manager) runWorktreeSetupScript(ctx context.Context, wt *Worktree) {
	if m.scriptMsgHandler == nil || m.repoProvider == nil {
		return
	}
	if wt.RepositoryID == "" {
		// Nothing to set up without a linked repository; upstream may not
		// always populate this field.
		return
	}
	repo, err := m.repoProvider.GetRepository(ctx, wt.RepositoryID)
	if err != nil {
		m.logger.Warn("failed to fetch repository for setup script",
			zap.String("repository_id", wt.RepositoryID),
			zap.Error(err))
		return
	}
	if strings.TrimSpace(repo.SetupScript) == "" {
		return
	}
	m.logger.Info("executing setup script for worktree",
		zap.String("worktree_id", wt.ID),
		zap.String("repository_id", wt.RepositoryID))
	scriptReq := ScriptExecutionRequest{
		SessionID:    wt.SessionID,
		TaskID:       wt.TaskID,
		RepositoryID: wt.RepositoryID,
		Script:       repo.SetupScript,
		WorkingDir:   wt.Path,
		ScriptType:   "setup",
		Env:          m.managedScriptEnvironment(ctx),
	}
	if err := m.scriptMsgHandler.ExecuteSetupScript(ctx, scriptReq); err != nil {
		// Non-fatal: keep the worktree and surface a warning. The detailed
		// script output is already streamed to the script_execution chat
		// message; here we only record a concise, user-facing warning.
		m.logger.Warn("setup script failed, continuing without it",
			zap.String("worktree_id", wt.ID),
			zap.Error(err))
		wt.SetupScriptWarning = "Repository setup script failed; the worktree was created without it. Fix the setup script and re-run if needed."
		wt.SetupScriptWarningDetail = err.Error()
		return
	}
	m.logger.Info("setup script completed successfully", zap.String("worktree_id", wt.ID))
}

// runWorktreeCleanupScript executes the repository cleanup script for a worktree before removal.
func (m *Manager) runWorktreeCleanupScript(ctx context.Context, wt *Worktree) {
	if m.scriptMsgHandler == nil || m.repoProvider == nil {
		return
	}
	repo, err := m.repoProvider.GetRepository(ctx, wt.RepositoryID)
	if err != nil {
		m.logger.Warn("failed to fetch repository for cleanup script",
			zap.String("repository_id", wt.RepositoryID),
			zap.Error(err))
		return
	}
	if strings.TrimSpace(repo.CleanupScript) == "" {
		return
	}
	m.logger.Info("executing cleanup script for worktree",
		zap.String("worktree_id", wt.ID),
		zap.String("repository_id", wt.RepositoryID))
	scriptReq := ScriptExecutionRequest{
		SessionID:    wt.SessionID,
		TaskID:       wt.TaskID,
		RepositoryID: wt.RepositoryID,
		Script:       repo.CleanupScript,
		WorkingDir:   wt.Path,
		ScriptType:   "cleanup",
		Env:          m.managedScriptEnvironment(ctx),
	}
	if err := m.scriptMsgHandler.ExecuteCleanupScript(ctx, scriptReq); err != nil {
		m.logger.Warn("cleanup script failed, proceeding with deletion",
			zap.String("worktree_id", wt.ID),
			zap.Error(err))
	} else {
		m.logger.Info("cleanup script completed successfully",
			zap.String("worktree_id", wt.ID))
	}
}

func (m *Manager) managedScriptEnvironment(ctx context.Context) map[string]string {
	if m.scriptEnvProvider == nil {
		return nil
	}
	env, err := m.scriptEnvProvider.ExecutionEnvironment(ctx)
	if err != nil {
		m.logger.Warn("failed to resolve managed script environment", zap.Error(err))
		return nil
	}
	cachePath := env["GOCACHE"]
	if cachePath == "" || !filepath.IsAbs(cachePath) {
		return nil
	}
	return map[string]string{"GOCACHE": filepath.Clean(cachePath)}
}

// CleanupWorktrees removes provided worktrees without re-fetching from the store.
func (m *Manager) CleanupWorktrees(ctx context.Context, worktrees []*Worktree) error {
	if len(worktrees) == 0 {
		return nil
	}

	var lastErr error
	for _, wt := range worktrees {
		if wt == nil {
			continue
		}
		if err := m.removeWorktree(ctx, wt, true); err != nil {
			m.logger.Warn("failed to remove worktree on task deletion",
				zap.String("task_id", wt.TaskID),
				zap.String("worktree_id", wt.ID),
				zap.Error(err))
			lastErr = err
		}
	}

	m.mu.Lock()
	for _, wt := range worktrees {
		if wt == nil {
			continue
		}
		if wt.SessionID != "" {
			delete(m.worktrees, cacheKey(wt.SessionID, wt.RepositoryID, wt.BranchSlug))
		}
	}
	m.mu.Unlock()

	return lastErr
}

// OnTaskDeleted cleans up all worktrees for a task when it is deleted.
func (m *Manager) OnTaskDeleted(ctx context.Context, taskID string) error {
	// Get all worktrees for this task
	worktrees, err := m.GetAllByTaskID(ctx, taskID)
	if err != nil {
		return err
	}

	return m.CleanupWorktrees(ctx, worktrees)
}

// removeWorktreeDir removes a worktree directory using git worktree remove.
// After the inner directory is gone it tries to rmdir the parent task
// directory left behind by the nested {tasksBase}/{taskDirName}/{repoName}
// layout (see issue #1266). The rmdir is a best-effort no-op if the parent
// still has siblings or contains workspace-scoped content.
func (m *Manager) removeWorktreeDir(ctx context.Context, worktreePath, repoPath string) error {
	// First try git worktree remove
	cmd := newGitCommand(ctx, "worktree", "remove", "--force", worktreePath)
	cmd.Dir = repoPath
	if output, err := runGitCmdCombinedOutput(ctx, cmd); err != nil {
		m.logger.Debug("git worktree remove failed, falling back to filesystem removal",
			zap.String("output", string(output)),
			zap.Error(err))

		if err := m.forceRemoveDir(worktreePath); err != nil {
			return err
		}

		// Prune stale worktree entries
		pruneCmd := newGitCommand(ctx, "worktree", "prune")
		pruneCmd.Dir = repoPath
		if err := runGitCmd(ctx, pruneCmd); err != nil {
			m.logger.Debug("git worktree prune failed", zap.Error(err))
		}
	}
	m.tryRemoveEmptyTaskDir(worktreePath)
	return nil
}

// tryRemoveEmptyTaskDir rmdirs the parent of a removed worktree if that
// parent is an immediate child of TasksBasePath (i.e. a per-task container
// directory) and is now empty. Silently skips otherwise.
func (m *Manager) tryRemoveEmptyTaskDir(worktreePath string) {
	tasksBase, err := m.config.ExpandedTasksBasePath()
	if err != nil || tasksBase == "" {
		return
	}
	// Normalize both sides: ExpandedTasksBasePath returns the configured
	// value verbatim for absolute paths (incl. trailing slashes or doubled
	// separators), while filepath.Dir always yields a cleaned form.
	tasksBase = filepath.Clean(tasksBase)
	parent := filepath.Dir(worktreePath)
	// Guard: only act on direct children of tasksBase. Never touch
	// tasksBase itself or any deeper / unrelated location.
	if filepath.Dir(parent) != tasksBase {
		return
	}
	entries, readErr := os.ReadDir(parent)
	if readErr == nil && len(entries) == 1 && entries[0].Name() == storageworkspaces.OwnershipMarkerFilename && entries[0].Type().IsRegular() {
		if err := os.Remove(filepath.Join(parent, storageworkspaces.OwnershipMarkerFilename)); err != nil && !os.IsNotExist(err) {
			m.logger.Debug("task ownership marker not removed", zap.String("path", parent), zap.Error(err))
			return
		}
	}
	if err := os.Remove(parent); err != nil && !os.IsNotExist(err) {
		m.logger.Debug("task dir not removed (likely non-empty)",
			zap.String("path", parent),
			zap.Error(err))
	}
}

// forceRemoveDir removes a directory, retrying transient filesystem failures.
// It intentionally remains within Go's portable filesystem API so cleanup also
// works in native Windows environments where a Unix rm binary is unavailable.
func (m *Manager) forceRemoveDir(dir string) error {
	const maxRetries = 3
	const retryDelay = 200 * time.Millisecond
	return m.removeDirWithRetries(dir, maxRetries, retryDelay, os.RemoveAll)
}

func (m *Manager) removeDirWithRetries(
	dir string, maxRetries int, retryDelay time.Duration, removeAll func(string) error,
) error {
	var lastErr error

	for i := range maxRetries {
		lastErr = removeAll(dir)
		if lastErr == nil {
			return nil
		}
		if i < maxRetries-1 {
			m.logger.Debug("os.RemoveAll failed, retrying",
				zap.String("path", dir),
				zap.Int("attempt", i+1),
				zap.Error(lastErr))
			time.Sleep(retryDelay)
		}
	}
	return fmt.Errorf("remove directory %s after %d attempts: %w", dir, maxRetries, lastErr)
}
