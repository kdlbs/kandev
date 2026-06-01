package worktree

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"go.uber.org/zap"
)

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

		cmd := exec.CommandContext(ctx, "git", "branch", "-D", wt.Branch)
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
		now := time.Now()
		wt.Status = StatusDeleted
		wt.DeletedAt = &now
		wt.UpdatedAt = now
		if err := m.store.UpdateWorktree(ctx, wt); err != nil {
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
func (m *Manager) removeWorktreeDir(ctx context.Context, worktreePath, repoPath string) error {
	// First try git worktree remove
	cmd := exec.CommandContext(ctx, "git", "worktree", "remove", "--force", worktreePath)
	cmd.Dir = repoPath
	if output, err := runGitCmdCombinedOutput(ctx, cmd); err != nil {
		m.logger.Debug("git worktree remove failed, falling back to rm",
			zap.String("output", string(output)),
			zap.Error(err))

		if err := m.forceRemoveDir(ctx, worktreePath); err != nil {
			return err
		}

		// Prune stale worktree entries
		pruneCmd := exec.CommandContext(ctx, "git", "worktree", "prune")
		pruneCmd.Dir = repoPath
		if err := runGitCmd(ctx, pruneCmd); err != nil {
			m.logger.Debug("git worktree prune failed", zap.Error(err))
		}
	}
	return nil
}

// forceRemoveDir removes a directory, retrying on transient failures.
// On macOS, os.RemoveAll can fail with "directory not empty" when files
// have special attributes or were recently released by other processes
// (e.g. .next/dev build cache). Falls back to rm -rf as a last resort.
func (m *Manager) forceRemoveDir(ctx context.Context, dir string) error {
	const maxRetries = 3
	const retryDelay = 200 * time.Millisecond

	for i := range maxRetries {
		err := os.RemoveAll(dir)
		if err == nil {
			return nil
		}
		if i < maxRetries-1 {
			m.logger.Debug("os.RemoveAll failed, retrying",
				zap.String("path", dir),
				zap.Int("attempt", i+1),
				zap.Error(err))
			time.Sleep(retryDelay)
		}
	}

	// Last resort: shell out to rm -rf which handles macOS edge cases better
	cmd := exec.CommandContext(ctx, "rm", "-rf", dir)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("rm -rf failed: %w (output: %s)", err, string(output))
	}
	return nil
}
