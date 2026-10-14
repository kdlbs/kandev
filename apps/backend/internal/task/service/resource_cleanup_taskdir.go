package service

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/worktree"
)

const taskDirectoryExecutorType = "worktree"

type taskDirectoryCleaner interface {
	CaptureTaskDirectoryCleanup(context.Context, *worktree.Worktree) (*worktree.TaskDirectoryCleanup, error)
	CleanupTaskDirectory(context.Context, *worktree.TaskDirectoryCleanup) (bool, error)
}

func (s *Service) captureArchiveTaskDirectory(ctx context.Context, job *models.TaskResourceCleanupJob, snapshot *taskResourceCleanupSnapshot) error {
	cleaner, ok := s.worktreeCleanup.(taskDirectoryCleaner)
	if !ok || !job.IsArchive() || snapshot.TaskDirectory != nil {
		return nil
	}
	env := snapshot.TaskEnvironment
	if env == nil || env.TaskID != job.TaskID || env.ExecutorType != taskDirectoryExecutorType {
		return nil
	}
	current, err := s.taskEnvironmentCleanupOwnershipIsCurrent(ctx, env)
	if err != nil || !current {
		return err
	}
	for _, wt := range snapshot.Worktrees {
		if wt == nil || wt.TaskID != job.TaskID {
			continue
		}
		target, err := cleaner.CaptureTaskDirectoryCleanup(ctx, wt)
		if err != nil {
			return err
		}
		if target == nil {
			continue
		}
		if filepath.Clean(env.WorkspacePath) != target.Path {
			return errors.New("archive task directory path differs from environment")
		}
		snapshot.TaskDirectory = target
		return s.persistTaskDirectorySnapshot(ctx, job, snapshot)
	}
	return nil
}

func (s *Service) persistTaskDirectorySnapshot(ctx context.Context, job *models.TaskResourceCleanupJob, snapshot any) error {
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	updated, err := s.resourceCleanups.UpdateClaimedTaskResourceCleanupSnapshot(ctx, job.ID, job.Attempts, string(encoded))
	if err != nil {
		return err
	}
	if !updated {
		return errors.New("task directory cleanup claim changed")
	}
	job.ResourceSnapshot = string(encoded)
	return nil
}

func (s *Service) cleanupArchiveTaskDirectory(ctx context.Context, taskID string, env *models.TaskEnvironment, target *worktree.TaskDirectoryCleanup) error {
	if target == nil {
		return nil
	}
	if target.TaskID != taskID || env == nil || env.TaskID != taskID || env.ExecutorType != taskDirectoryExecutorType {
		return errors.New("archive task directory environment identity mismatch")
	}
	current, err := s.taskEnvironmentCleanupOwnershipIsCurrent(ctx, env)
	if err != nil || !current {
		return err
	}
	runtimes, err := s.executors.ListExecutorsRunningByTaskID(ctx, taskID)
	if err != nil || len(runtimes) > 0 {
		return err
	}
	shared, err := s.hasActiveOtherTaskSessionsForEnvironment(ctx, taskID, env)
	if err != nil || shared {
		return err
	}
	cleaner, ok := s.worktreeCleanup.(taskDirectoryCleaner)
	if !ok {
		return errors.New("archive task directory cleaner is unavailable")
	}
	_, err = cleaner.CleanupTaskDirectory(ctx, target)
	return err
}

func (s *Service) captureReclaimTaskDirectory(ctx context.Context, job *models.TaskResourceCleanupJob, snapshot *archiveReclaimSnapshot, wt *worktree.Worktree) error {
	cleaner, ok := s.worktreeCleanup.(taskDirectoryCleaner)
	if !ok || snapshot.TaskDirectory != nil {
		return nil
	}
	env, err := s.taskEnvironments.GetTaskEnvironmentByTaskID(ctx, job.TaskID)
	if err != nil {
		return err
	}
	if env == nil || env.TaskID != job.TaskID || env.ExecutorType != taskDirectoryExecutorType {
		return nil
	}
	target, err := cleaner.CaptureTaskDirectoryCleanup(ctx, wt)
	if err != nil || target == nil {
		return err
	}
	if filepath.Clean(env.WorkspacePath) != target.Path {
		return errors.New("archive task directory path differs from environment")
	}
	snapshot.TaskDirectory = target
	snapshot.TaskEnvironment = env
	return s.persistTaskDirectorySnapshot(ctx, job, snapshot)
}

func (s *Service) finishArchiveReclaimDirectory(ctx context.Context, job *models.TaskResourceCleanupJob, snapshot archiveReclaimSnapshot) error {
	if snapshot.TaskDirectory == nil {
		return nil
	}
	if snapshot.TaskEnvironment == nil {
		return errors.New("reclaim task directory lacks environment identity")
	}
	current, err := s.archiveReclaimTaskIsCurrent(ctx, job, snapshot)
	if err != nil || !current {
		return err
	}
	env, err := s.taskEnvironments.GetTaskEnvironment(ctx, snapshot.TaskEnvironment.ID)
	if err != nil {
		return err
	}
	if env == nil {
		return errors.New("reclaim task directory environment is unavailable")
	}
	for _, repo := range env.Repos {
		if repo == nil {
			return errors.New("reclaim task directory inventory is incomplete")
		}
		if repo.WorktreeID == snapshot.WorktreeID && repo.Status == worktree.StatusDeleted && repo.WorktreePath == snapshot.WorktreePath {
			return s.cleanupArchiveTaskDirectory(ctx, job.TaskID, snapshot.TaskEnvironment, snapshot.TaskDirectory)
		}
	}
	return nil
}
