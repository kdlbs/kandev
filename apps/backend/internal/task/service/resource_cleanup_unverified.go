package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/worktree"
)

// Cleanup keeps the original inventory until every resource is removed. A
// failed resource cannot prevent independent worktrees from making progress.
// The final error fences all cleanup when inventory or progress cannot be saved.
func (s *Service) cleanupUnverifiedWorktrees(ctx context.Context, job *models.TaskResourceCleanupJob, snapshot *taskResourceCleanupSnapshot, stop taskRuntimeStopOutcome) ([]error, error) {
	if err := s.prepareUnverifiedCleanupAttempt(ctx, job, snapshot, len(stop.failed) == 0); err != nil {
		return nil, err
	}
	var errs []error
	if len(stop.failed) > 0 {
		errs = append(errs, fmt.Errorf("%d runtime stop operations failed", len(stop.failed)))
	}
	preserve := taskCleanupPreserveRows(stop)
	for _, wt := range snapshot.Worktrees {
		if wt == nil || snapshot.CompletedWorktrees[wt.ID] {
			continue
		}
		resourceErrs, err := s.cleanupUnverifiedWorktree(ctx, job, snapshot, wt, preserve)
		if err != nil {
			return errs, err
		}
		if err := errors.Join(resourceErrs...); err != nil {
			errs = append(errs, fmt.Errorf("cleanup worktree %s: %w", wt.ID, err))
			continue
		}
		snapshot.CompletedWorktrees[wt.ID] = true
		if err := s.persistTaskDirectorySnapshot(ctx, job, snapshot); err != nil {
			return errs, err
		}
	}
	if len(snapshot.CleanupInventoryConflicts) > 0 {
		errs = append(errs, errors.New("prior workspace inventory has conflicting identities"))
	}
	if len(stop.failed) == 0 {
		errs = append(errs, s.cleanupPriorTaskEnvironments(ctx, job, snapshot))
	}
	return errs, nil
}

func (s *Service) prepareUnverifiedCleanupAttempt(ctx context.Context, job *models.TaskResourceCleanupJob, snapshot *taskResourceCleanupSnapshot, runtimeStopped bool) error {
	if snapshot.CleanupVersion != 1 || !snapshot.CleanupInventoryCaptured || !snapshot.DiscardWorktreeChanges || !taskResourceCleanupDeletesTask(job.Trigger) {
		return errors.New("unverified cleanup inventory is incomplete")
	}
	snapshot.RuntimeStopCompleted = runtimeStopped
	if snapshot.CompletedWorktrees == nil {
		snapshot.CompletedWorktrees = make(map[string]bool)
	}
	return s.persistTaskDirectorySnapshot(ctx, job, snapshot)
}

func (s *Service) cleanupPriorTaskEnvironments(ctx context.Context, job *models.TaskResourceCleanupJob, snapshot *taskResourceCleanupSnapshot) error {
	var errs []error
	for _, env := range snapshot.PriorTaskEnvironments {
		if env == nil || (snapshot.TaskEnvironment != nil && env.ID == snapshot.TaskEnvironment.ID && env.OwnershipGeneration == snapshot.TaskEnvironment.OwnershipGeneration) {
			continue
		}
		errs = append(errs, s.cleanupDestructiveTaskResources(ctx, job.TaskID, snapshot.Sessions, nil, taskEnvironmentCleanup{env: taskEnvironmentWithoutSnapshotWorktrees(env, snapshot.Worktrees), allowUnverifiedWorkspaceChanges: true, discardWorktreeChanges: true}, nil)...)
	}
	return errors.Join(errs...)
}

func (s *Service) cleanupUnverifiedWorktree(ctx context.Context, job *models.TaskResourceCleanupJob, snapshot *taskResourceCleanupSnapshot, wt *worktree.Worktree, preserve map[string]struct{}) ([]error, error) {
	if slices.Contains(snapshot.CleanupInventoryConflicts, wt.ID) {
		return []error{errors.New("prior workspace inventory has conflicting identities")}, nil
	}
	wt.CleanupHeadOID = snapshot.WorktreeHeadOIDs[wt.ID]
	wt.CleanupHeadOIDUnavailable = wt.CleanupHeadOID == ""
	if wt.CleanupHeadOIDUnavailable {
		return []error{errors.New("original workspace identity is unavailable")}, nil
	}
	if !hasWorktreeSourceManifest(snapshot.ArchiveSourceManifest, wt.ID) {
		manifest, err := s.captureArchiveSourceManifest(ctx, job.ID, job.TaskID, []*worktree.Worktree{wt})
		if err != nil {
			return []error{err}, nil
		}
		snapshot.ArchiveSourceManifest = append(snapshot.ArchiveSourceManifest, manifest...)
		if err := s.persistTaskDirectorySnapshot(ctx, job, snapshot); err != nil {
			return nil, err
		}
	}
	return s.cleanupDestructiveTaskResources(ctx, job.TaskID, snapshot.Sessions, []*worktree.Worktree{wt}, taskEnvironmentCleanup{
		env: snapshot.TaskEnvironment, deferEnvironment: true,
		discardWorktreeChanges: true, allowUnverifiedWorkspaceChanges: true,
	}, preserve), nil
}

func hasWorktreeSourceManifest(manifest []ArchiveSourceManifest, id string) bool {
	for _, item := range manifest {
		if item.WorktreeID == id {
			return true
		}
	}
	return false
}

func cleanupRetryLimitReached(job *models.TaskResourceCleanupJob) bool {
	if job.Attempts < taskResourceCleanupMaxAttempts {
		return false
	}
	if !isCascadeCriticalCleanupTrigger(job.Trigger) {
		return true
	}
	var options struct {
		AllowUnverified bool `json:"allow_unverified_workspace_changes"`
	}
	return json.Unmarshal([]byte(job.ResourceSnapshot), &options) == nil && options.AllowUnverified
}
