package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/worktree"
	"go.uber.org/zap"
)

func (s *Service) captureCleanupIdentities(ctx context.Context, trees []*worktree.Worktree, trigger models.TaskResourceCleanupTrigger, options DeleteTaskOptions) (map[string]string, map[string]string, error) {
	if job := (models.TaskResourceCleanupJob{Trigger: trigger}); !job.IsArchive() {
		return s.captureCleanupHeadOIDsForMode(ctx, trees, options)
	}
	heads := make(map[string]string)
	failures := make(map[string]string)
	for _, wt := range trees {
		if wt == nil {
			continue
		}
		captured, err := s.captureWorktreeCleanupHeadOIDs(ctx, []*worktree.Worktree{wt})
		if ctx.Err() != nil {
			return nil, nil, ctx.Err()
		}
		if err != nil {
			var inspection *worktree.CleanupInspectionError
			if errors.Is(err, context.Canceled) || !errors.As(err, &inspection) {
				return nil, nil, err
			}
			failures[wt.ID] = "physical_identity_unavailable"
			s.logger.Warn("archive workspace inspection deferred", zap.String("task_id", wt.TaskID), zap.String("worktree_id", wt.ID), zap.String("stage", inspection.Stage), zap.String("reason", inspection.Reason))
			continue
		}
		for id, head := range captured {
			heads[id] = head
		}
	}
	return heads, failures, nil
}

// Each removal needs its own persisted evidence. Unresolved resources fence
// all enclosing-directory and environment cleanup in the caller.
func (s *Service) cleanupArchiveWorktrees(ctx context.Context, job *models.TaskResourceCleanupJob, snapshot *taskResourceCleanupSnapshot, stop taskRuntimeStopOutcome) error {
	if len(stop.failed) > 0 {
		return fmt.Errorf("%d runtime stop operations failed; defer archive cleanup", len(stop.failed))
	}
	if snapshot.CompletedWorktrees == nil {
		snapshot.CompletedWorktrees = make(map[string]bool)
	}
	var errs []error
	for _, wt := range snapshot.Worktrees {
		if wt == nil || snapshot.CompletedWorktrees[wt.ID] {
			continue
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if snapshot.CleanupPreparationErrors[wt.ID] != "" {
			errs = append(errs, fmt.Errorf("cleanup worktree %s: original workspace identity is unavailable", wt.ID))
			continue
		}
		if err := s.prepareArchiveWorktreeManifest(ctx, job, snapshot, wt); err != nil {
			// A failed persistence write invalidates the whole attempt.
			var saveErr *archiveManifestPersistenceError
			if errors.As(err, &saveErr) {
				return err
			}
			errs = append(errs, err)
			continue
		}
		cleanupErr := errors.Join(s.cleanupDestructiveTaskResources(ctx, job.TaskID, snapshot.Sessions, []*worktree.Worktree{wt}, taskEnvironmentCleanup{env: snapshot.TaskEnvironment, deferEnvironment: true, deferInUseWorktrees: true, preserveBranches: true, discardWorktreeChanges: snapshot.DiscardWorktreeChanges}, taskCleanupPreserveRows(stop))...)
		if cleanupErr != nil {
			errs = append(errs, cleanupErr)
			continue
		}
		snapshot.CompletedWorktrees[wt.ID] = true
		if err := s.persistTaskDirectorySnapshot(ctx, job, snapshot); err != nil {
			return err
		}
	}
	return errors.Join(errs...)
}

type archiveManifestPersistenceError struct{ error }

func (s *Service) prepareArchiveWorktreeManifest(ctx context.Context, job *models.TaskResourceCleanupJob, snapshot *taskResourceCleanupSnapshot, wt *worktree.Worktree) error {
	if snapshot.ArchiveSourceManifestCaptured || hasWorktreeSourceManifest(snapshot.ArchiveSourceManifest, wt.ID) {
		return nil
	}
	manifests, err := s.captureArchiveSourceManifest(ctx, job.ID, job.TaskID, []*worktree.Worktree{wt})
	if err != nil {
		return fmt.Errorf("capture archive source manifest for %s: %w", wt.ID, err)
	}
	snapshot.ArchiveSourceManifest = append(snapshot.ArchiveSourceManifest, manifests...)
	snapshot.ArchiveSourceManifestCaptured = true
	for _, candidate := range snapshot.Worktrees {
		if candidate != nil && !hasWorktreeSourceManifest(snapshot.ArchiveSourceManifest, candidate.ID) {
			snapshot.ArchiveSourceManifestCaptured = false
			break
		}
	}
	if err := s.persistTaskDirectorySnapshot(ctx, job, snapshot); err != nil {
		return &archiveManifestPersistenceError{err}
	}
	return nil
}

func (s *Service) checkArchiveReclaimInspection(ctx context.Context, taskID, worktreeID string) error {
	inspector, ok := s.resourceCleanups.(taskResourceCleanupTaskInspector)
	if !ok {
		return errors.New("archive cleanup evidence is unavailable")
	}
	jobs, err := inspector.ListTaskResourceCleanupJobs(ctx, taskID)
	if err != nil {
		return err
	}
	for _, job := range jobs {
		if !job.IsArchive() || job.State == models.TaskResourceCleanupStateCancelled {
			continue
		}
		var snapshot taskResourceCleanupSnapshot
		if err := json.Unmarshal([]byte(job.ResourceSnapshot), &snapshot); err != nil {
			return err
		}
		if snapshot.CleanupPreparationErrors[worktreeID] != "" {
			return errors.New("original workspace identity is unavailable")
		}
		if snapshot.ArchiveSourceManifestRequired && findWorktreeByID(snapshot.Worktrees, worktreeID) != nil &&
			!hasWorktreeSourceManifest(snapshot.ArchiveSourceManifest, worktreeID) {
			return errors.New("archive source manifest is unavailable")
		}
	}
	return nil
}
