package service

import (
	"context"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/worktree"
)

// BranchRecovery reports, after an unarchive, whether a repo's prior
// worktree branch still exists somewhere and can be picked back up by the
// next session launch. Status values are worktree.BranchStatus*.
type BranchRecovery struct {
	TaskID       string `json:"task_id"`
	RepositoryID string `json:"repository_id"`
	Branch       string `json:"branch"`
	Status       string `json:"status"`
}

// BranchStatusProber reports where a worktree branch still exists.
// Implemented by *worktree.Manager (BranchRecoveryStatus).
type BranchStatusProber interface {
	BranchRecoveryStatus(ctx context.Context, repoPath, branch string) string
}

// RecoverTaskBranches inspects the task's historical worktree records
// (archive keeps the rows and deletes only the local branch + directory)
// and, per repository, probes whether the branch still exists locally or
// on origin. When it does and the task_repositories row has no
// checkout_branch yet, the branch is written back as checkout_branch so a
// brand-new session checks the old work back out instead of minting a
// fresh branch. Best-effort: failures are logged and reported as
// status=missing rather than blocking the unarchive.
func (s *Service) RecoverTaskBranches(ctx context.Context, taskID string) []BranchRecovery {
	provider, okProvider := s.worktreeCleanup.(WorktreeProvider)
	prober, okProber := s.worktreeCleanup.(BranchStatusProber)
	if !okProvider || !okProber {
		return nil
	}
	worktrees, err := provider.GetAllByTaskID(ctx, taskID)
	if err != nil {
		s.logger.Warn("branch recovery: failed to list worktrees",
			zap.String("task_id", taskID), zap.Error(err))
		return nil
	}
	latest := latestWorktreePerRepo(worktrees)
	if len(latest) == 0 {
		return nil
	}
	reposByID := s.taskRepositoriesByRepoID(ctx, taskID)
	out := make([]BranchRecovery, 0, len(latest))
	restored := false
	for _, wt := range latest {
		status := prober.BranchRecoveryStatus(ctx, wt.RepositoryPath, wt.Branch)
		if status != worktree.BranchStatusMissing {
			restored = s.restoreCheckoutBranch(ctx, reposByID[wt.RepositoryID], wt.Branch) || restored
		}
		out = append(out, BranchRecovery{
			TaskID:       taskID,
			RepositoryID: wt.RepositoryID,
			Branch:       wt.Branch,
			Status:       status,
		})
	}
	// checkout_branch rides along on task events (taskRepositoriesForEvent),
	// so a successful restore must re-publish task.updated — the unarchive
	// event already went out with the pre-restore repository rows. Mirrors
	// the UpdateRepositoryBaseBranch flow.
	if restored {
		if task, err := s.tasks.GetTask(ctx, taskID); err == nil && task != nil {
			s.publishTaskEvent(ctx, events.TaskUpdated, task, nil)
		}
	}
	return out
}

// latestWorktreePerRepo picks the most recently created worktree record
// per repository. Multiple sessions leave multiple records; the newest
// branch is the one the user last worked on.
func latestWorktreePerRepo(worktrees []*worktree.Worktree) map[string]*worktree.Worktree {
	latest := make(map[string]*worktree.Worktree)
	for _, wt := range worktrees {
		if wt == nil || wt.Branch == "" || wt.RepositoryPath == "" {
			continue
		}
		cur, ok := latest[wt.RepositoryID]
		if !ok || wt.CreatedAt.After(cur.CreatedAt) {
			latest[wt.RepositoryID] = wt
		}
	}
	return latest
}

func (s *Service) taskRepositoriesByRepoID(ctx context.Context, taskID string) map[string]*models.TaskRepository {
	byID := map[string]*models.TaskRepository{}
	taskRepos, err := s.taskRepos.ListTaskRepositories(ctx, taskID)
	if err != nil {
		s.logger.Warn("branch recovery: failed to list task repositories",
			zap.String("task_id", taskID), zap.Error(err))
		return byID
	}
	rowsPerRepo := map[string]int{}
	for _, tr := range taskRepos {
		rowsPerRepo[tr.RepositoryID]++
	}
	for _, tr := range taskRepos {
		// Multi-branch tasks keep one task_repositories row per branch
		// (add_branch flow). Restoring the newest worktree branch into an
		// arbitrary sibling row could clobber another branch's identity, so
		// leave multi-row repos alone — session resume still recovers those
		// via the per-worktree records.
		if rowsPerRepo[tr.RepositoryID] > 1 {
			continue
		}
		byID[tr.RepositoryID] = tr
	}
	return byID
}

// restoreCheckoutBranch writes the recovered branch back as the task
// repository's checkout_branch so the next session launch checks it out
// (executor CheckoutBranch flow) instead of creating a new branch. An
// already-set checkout_branch (e.g. a PR head) is never overwritten.
// Returns whether the row was updated.
func (s *Service) restoreCheckoutBranch(ctx context.Context, tr *models.TaskRepository, branch string) bool {
	if tr == nil || tr.CheckoutBranch != "" || branch == "" {
		return false
	}
	tr.CheckoutBranch = branch
	if err := s.taskRepos.UpdateTaskRepository(ctx, tr); err != nil {
		s.logger.Warn("branch recovery: failed to restore checkout_branch",
			zap.String("task_id", tr.TaskID),
			zap.String("repository_id", tr.RepositoryID),
			zap.String("branch", branch),
			zap.Error(err))
		return false
	}
	return true
}
