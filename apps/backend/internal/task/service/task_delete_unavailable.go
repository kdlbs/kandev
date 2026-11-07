package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/worktree"
)

func recoverableDeleteInspection(ctx context.Context, err error) bool {
	if ctx.Err() != nil || errors.Is(err, context.Canceled) {
		return false
	}
	var inspection *worktree.CleanupInspectionError
	return errors.As(err, &inspection) && inspection.Reason != worktree.CleanupInspectionReasonGitStartFailed
}

func (s *Service) captureUnavailableCleanupInventory(ctx context.Context, taskID string, snapshot *taskResourceCleanupSnapshot) error {
	if !snapshot.AllowUnverifiedWorkspaceChanges {
		return nil
	}
	inspector, ok := s.resourceCleanups.(taskResourceCleanupTaskInspector)
	if !ok {
		return errors.New("durable cleanup inventory unavailable")
	}
	jobs, err := inspector.ListTaskResourceCleanupJobs(ctx, taskID)
	if err != nil {
		return err
	}
	ids := make(map[string]*worktree.Worktree)
	if snapshot.WorktreeHeadOIDs == nil {
		snapshot.WorktreeHeadOIDs = make(map[string]string)
	}
	for _, wt := range snapshot.Worktrees {
		if wt != nil {
			ids[wt.ID] = wt
		}
	}
	targets := restoreStopTargets(snapshot.StopTargets)
	for index := len(jobs) - 1; index >= 0; index-- {
		job := jobs[index]
		if job.State == models.TaskResourceCleanupStateSucceeded || job.State == models.TaskResourceCleanupStateCancelled {
			continue
		}
		var previous taskResourceCleanupSnapshot
		if err := json.Unmarshal([]byte(job.ResourceSnapshot), &previous); err != nil {
			return fmt.Errorf("read prior cleanup inventory: %w", err)
		}
		targets = mergeTaskStopTargets(targets, restoreStopTargets(previous.StopTargets))

		mergePriorCleanupInventory(snapshot, &previous, json.RawMessage(job.ResourceSnapshot), ids)
	}
	if s.executors == nil {
		return errors.New("runtime cleanup inventory unavailable")
	}
	runtimes, err := s.executors.ListExecutorsRunningByTaskID(ctx, taskID)
	if err != nil {
		return fmt.Errorf("capture runtime cleanup inventory: %w", err)
	}
	snapshot.RetainedExecutors = append(snapshot.RetainedExecutors, runtimes...)
	targets = mergeTaskStopTargets(targets, taskStopTargetsFromRunningRows(snapshot.RetainedExecutors))
	snapshot.StopTargets = persistStopTargets(targets)
	snapshot.CleanupVersion = 1
	snapshot.CleanupInventoryCaptured = true
	return nil
}

func mergePriorCleanupInventory(snapshot, previous *taskResourceCleanupSnapshot, payload json.RawMessage, ids map[string]*worktree.Worktree) {
	mergePriorCleanupMetadata(snapshot, previous)
	snapshot.PriorCleanupInventory = append(snapshot.PriorCleanupInventory, payload)
	snapshot.PriorTaskEnvironments = append(snapshot.PriorTaskEnvironments, previous.PriorTaskEnvironments...)
	if previous.TaskEnvironment != nil {
		snapshot.PriorTaskEnvironments = append(snapshot.PriorTaskEnvironments, previous.TaskEnvironment)
	}
	snapshot.Attachments = append(snapshot.Attachments, previous.Attachments...)
	snapshot.Sessions = append(snapshot.Sessions, previous.Sessions...)
	snapshot.SSHTaskDirs = append(snapshot.SSHTaskDirs, previous.SSHTaskDirs...)
	snapshot.AgentHistory = append(snapshot.AgentHistory, previous.AgentHistory...)
	snapshot.RetainedExecutors = append(snapshot.RetainedExecutors, previous.RetainedExecutors...)
	snapshot.CleanupInventoryConflicts = append(snapshot.CleanupInventoryConflicts, previous.CleanupInventoryConflicts...)
	for _, wt := range previous.Worktrees {
		if wt == nil {
			continue
		}
		if current := ids[wt.ID]; current != nil {
			if current.Path != wt.Path || current.RepositoryPath != wt.RepositoryPath {
				snapshot.CleanupInventoryConflicts = append(snapshot.CleanupInventoryConflicts, wt.ID)
			}
		} else {
			ids[wt.ID] = wt
			snapshot.Worktrees = append(snapshot.Worktrees, wt)
		}
		// Earlier obligations keep their original identity, including unknown identity.
		delete(snapshot.WorktreeHeadOIDs, wt.ID)
		if head := previous.WorktreeHeadOIDs[wt.ID]; head != "" {
			snapshot.WorktreeHeadOIDs[wt.ID] = head
		}
	}
}

func mergePriorCleanupMetadata(snapshot, previous *taskResourceCleanupSnapshot) {
	if snapshot.WorktreeTaskDirNames == nil {
		snapshot.WorktreeTaskDirNames = make(map[string]string)
	}
	maps.Copy(snapshot.WorktreeTaskDirNames, previous.WorktreeTaskDirNames)
	if snapshot.WorktreeBranchMetadata == nil {
		snapshot.WorktreeBranchMetadata = make(map[string]persistedWorktreeBranchMetadata)
	}
	maps.Copy(snapshot.WorktreeBranchMetadata, previous.WorktreeBranchMetadata)
	if snapshot.WorktreeAdoptions == nil {
		snapshot.WorktreeAdoptions = make(map[string]*models.WorkspaceAdoptionReceipt)
	}
	maps.Copy(snapshot.WorktreeAdoptions, previous.WorktreeAdoptions)
	snapshot.OrphanReapRoots = mergeOrphanReapRoots(snapshot.OrphanReapRoots, previous.OrphanReapRoots)
}
