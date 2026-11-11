package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/worktree"
)

func (s *Service) captureCleanupHeadOIDsForMode(ctx context.Context, worktrees []*worktree.Worktree, options DeleteTaskOptions) (map[string]string, map[string]string, error) {
	if options.retainsWorkspace() {
		return nil, nil, nil
	}
	if !options.AllowUnverifiedWorkspaceChanges {
		heads, err := s.captureWorktreeCleanupHeadOIDs(ctx, worktrees)
		return heads, nil, err
	}
	if _, ok := s.worktreeCleanup.(WorktreeCleanupIdentityProvider); len(worktrees) > 0 && !ok {
		return nil, nil, errors.New("worktree cleanup identity provider is unavailable")
	}
	heads := make(map[string]string)
	failures := make(map[string]string)
	for _, wt := range worktrees {
		if wt == nil {
			continue
		}
		captured, err := s.captureWorktreeCleanupHeadOIDs(ctx, []*worktree.Worktree{wt})
		if err != nil {
			if !recoverableDeleteInspection(ctx, err) {
				return nil, nil, err
			}
			failures[wt.ID] = "physical_identity_unavailable"
			continue
		}
		for id, head := range captured {
			heads[id] = head
		}
	}
	return heads, failures, nil
}

func (s *Service) executeRetainedTaskShutdown(ctx context.Context, job *models.TaskResourceCleanupJob, snapshot *taskResourceCleanupSnapshot) error {
	targets, err := s.refreshTaskRuntimeStopTargets(ctx, job.TaskID, restoreStopTargets(snapshot.StopTargets))
	if err != nil {
		return err
	}
	if len(targets) == 0 {
		return nil
	}
	if s.executionStopper == nil {
		return fmt.Errorf("workspace retained; runtime shutdown unavailable")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// Provider stop may destroy file-bearing compute. Unknown targets stay pending.
	safeTargets := make([]taskStopTarget, 0, len(targets))
	for _, target := range targets {
		if retainedTargetCanStop(job.TaskID, target, snapshot) {
			safeTargets = append(safeTargets, target)
		}
	}
	pending := len(targets) - len(safeTargets)
	targets = safeTargets
	s.registerTaskRuntimeStopOwners(targets, true)
	outcome := s.stopTaskRuntimeTargetsWithTaskDeleted(ctx, job.TaskID, targets, taskResourceCleanupStopReason(job.Trigger), "retained task runtime stop failed", true, true)
	if pending += len(outcome.failed); pending > 0 {
		return fmt.Errorf("workspace retained; %d runtime stops pending", pending)
	}
	return ctx.Err()
}

func (s *Service) captureRetainedRuntimeInventory(ctx context.Context, taskID string, snapshot *taskResourceCleanupSnapshot) error {
	if snapshot.WorkspaceCleanup != models.WorkspaceCleanupRetain {
		return nil
	}
	snapshot.RetentionVersion = 1
	snapshot.RetentionActorID, _ = taskDeletePreviewUserID(ctx)
	if err := s.capturePriorRetainedStopTargets(ctx, taskID, snapshot); err != nil {
		return err
	}
	if s.executors == nil {
		return nil
	}
	var err error
	snapshot.RetainedExecutors, err = s.executors.ListExecutorsRunningByTaskID(ctx, taskID)
	if err != nil {
		return fmt.Errorf("capture retained runtime inventory: %w", err)
	}
	snapshot.StopTargets = persistStopTargets(mergeTaskStopTargets(restoreStopTargets(snapshot.StopTargets), taskStopTargetsFromRunningRows(snapshot.RetainedExecutors)))
	return nil
}

func (s *Service) capturePriorRetainedStopTargets(ctx context.Context, taskID string, snapshot *taskResourceCleanupSnapshot) error {
	inspector, ok := s.resourceCleanups.(taskResourceCleanupTaskInspector)
	if !ok {
		return fmt.Errorf("retained cleanup inventory unavailable")
	}
	jobs, err := inspector.ListTaskResourceCleanupJobs(ctx, taskID)
	if err != nil {
		return err
	}
	targets := restoreStopTargets(snapshot.StopTargets)
	for _, job := range jobs {
		var previous taskResourceCleanupSnapshot
		if err := json.Unmarshal([]byte(job.ResourceSnapshot), &previous); err != nil {
			return err
		}
		targets = mergeTaskStopTargets(targets, restoreStopTargets(previous.StopTargets))
	}
	snapshot.StopTargets = persistStopTargets(targets)
	return nil
}

func retainedTargetCanStop(taskID string, target taskStopTarget, snapshot *taskResourceCleanupSnapshot) bool {
	runtime := target.runtime
	if runtime == "" {
		for _, saved := range snapshot.RetainedExecutors {
			if saved != nil && saved.TaskID == taskID && saved.SessionID == target.sessionID && saved.AgentExecutionID == target.executionID {
				runtime = saved.Runtime
				break
			}
		}
	}
	if runtime != "" {
		return runtime == agentruntime.RuntimeStandalone
	}
	env := snapshot.TaskEnvironment
	return env != nil && (models.ExecutorType(env.ExecutorType) == models.ExecutorTypeLocal || models.ExecutorType(env.ExecutorType) == models.ExecutorTypeWorktree)
}

func (s *Service) checkTaskWorkspaceCleanupAllowed(ctx context.Context, taskID string) error {
	if reader, ok := s.tasks.(interface {
		HasRetainedTaskWorkspace(context.Context, string) (bool, error)
	}); ok {
		retained, err := reader.HasRetainedTaskWorkspace(ctx, taskID)
		if err != nil {
			return err
		}
		if retained {
			return fmt.Errorf("workspace retained; resource cleanup is disabled")
		}
	}
	return nil
}
