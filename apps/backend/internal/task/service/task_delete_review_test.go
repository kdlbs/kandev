package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/retention"
	"github.com/kandev/kandev/internal/worktree"
	"github.com/stretchr/testify/require"
)

func TestFailedRetainPreparationDoesNotPoisonCleanupInventory(t *testing.T) {
	svc, _, repo := createTestService(t)
	svc.StopTaskResourceCleanupWorker()
	ctx := context.Background()
	seedTaskDeletePreflightTask(t, repo, "failed-retain", "", false)
	svc.SetWorktreeCleanup(&taskDeletePreflightCleanup{inventoryErr: errors.New("inventory unavailable")})
	err := svc.PrepareTaskResourceCleanupWithDeleteOptions(ctx, "failed-retain", models.TaskResourceCleanupTriggerDelete, "failed-reservation", false, DeleteTaskOptions{WorkspaceCleanup: models.WorkspaceCleanupRetain})
	require.ErrorContains(t, err, "inventory unavailable")
	_, err = repo.GetTask(ctx, "failed-retain")
	require.NoError(t, err)
	job, err := repo.GetTaskResourceCleanupJobByOperationID(ctx, "failed-reservation")
	require.NoError(t, err)
	require.Equal(t, models.TaskResourceCleanupStateCancelled, job.State)
	retained, err := repo.HasRetainedTaskWorkspace(ctx, "unrelated-task")
	require.NoError(t, err, "failed preparation must not block unrelated cleanup")
	require.False(t, retained)
}

type independentRetentionStopper struct {
	stubStopper
	leaseErr  error
	afterStop func()
}

func (s *independentRetentionStopper) StopExecution(context.Context, string, string, bool) error {
	s.stopExecutionCalls++
	// An independently admitted stop must finish before the cleanup caller can proceed.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, release, err := retention.AcquireMutation(ctx)
	s.leaseErr = err
	if err == nil {
		release()
	}
	if s.afterStop != nil {
		s.afterStop()
	}
	return err
}

func TestCleanupShutdownDoesNotHoldPhysicalMutationLease(t *testing.T) {
	for _, mode := range []models.WorkspaceCleanupMode{models.WorkspaceCleanupDelete, models.WorkspaceCleanupRetain} {
		t.Run(string(mode), func(t *testing.T) {
			svc, _, _ := createTestService(t)
			svc.StopTaskResourceCleanupWorker()
			stopper := &independentRetentionStopper{}
			svc.executionStopper = stopper
			snapshot := &taskResourceCleanupSnapshot{WorkspaceCleanup: mode, ArchiveSourceManifestCaptured: true,
				TaskEnvironment: &models.TaskEnvironment{ExecutorType: string(models.ExecutorTypeLocal)},
				StopTargets:     []persistedTaskStopTarget{{ExecutionID: "concurrent-stop", SessionID: "concurrent-session"}}}
			_ = svc.executeTaskResourceCleanupJob(context.Background(), &models.TaskResourceCleanupJob{TaskID: "deleted", Trigger: models.TaskResourceCleanupTriggerDelete}, snapshot)
			require.Equal(t, 1, stopper.stopExecutionCalls)
			require.NoError(t, stopper.leaseErr, "cleanup must not hold the lease while waiting for another stop")
		})
	}
}

func TestRetainedSharedTaskUsesSavedRuntimeForSafeShutdown(t *testing.T) {
	for _, runtime := range []agentruntime.Runtime{agentruntime.RuntimeStandalone, agentruntime.RuntimeDocker, ""} {
		t.Run(string(runtime), func(t *testing.T) {
			svc, _, _ := createTestService(t)
			svc.StopTaskResourceCleanupWorker()
			stopper := &stubStopper{}
			svc.executionStopper = stopper
			snapshot := &taskResourceCleanupSnapshot{WorkspaceCleanup: models.WorkspaceCleanupRetain,
				StopTargets:       []persistedTaskStopTarget{{ExecutionID: "shared-execution", SessionID: "shared-session"}},
				RetainedExecutors: []*models.ExecutorRunning{{TaskID: "child", SessionID: "shared-session", AgentExecutionID: "shared-execution", Runtime: runtime}}}
			err := svc.executeRetainedTaskShutdown(context.Background(), &models.TaskResourceCleanupJob{TaskID: "child"}, snapshot)
			if runtime == agentruntime.RuntimeStandalone {
				require.NoError(t, err)
				require.Equal(t, 1, stopper.stopExecutionCalls)
			} else {
				require.Error(t, err)
				require.Zero(t, stopper.stopExecutionCalls)
			}
		})
	}
}

func TestFailedRetainSnapshotWriteDoesNotPoisonCleanupInventory(t *testing.T) {
	svc, _, repo := createTestService(t)
	svc.StopTaskResourceCleanupWorker()
	ctx := context.Background()
	seedTaskDeletePreflightTask(t, repo, "failed-retain-write", "", false)
	svc.SetWorktreeCleanup(&taskDeletePreflightCleanup{})
	svc.resourceCleanups = &struct {
		failingCleanupSnapshotRepository
		taskResourceCleanupTaskInspector
	}{failingCleanupSnapshotRepository{TaskResourceCleanupRepository: repo, err: errors.New("snapshot unavailable")}, repo}
	err := svc.PrepareTaskResourceCleanupWithDeleteOptions(ctx, "failed-retain-write", models.TaskResourceCleanupTriggerDelete, "failed-write", false, DeleteTaskOptions{WorkspaceCleanup: models.WorkspaceCleanupRetain})
	require.ErrorContains(t, err, "snapshot unavailable")
	_, err = repo.GetTask(ctx, "failed-retain-write")
	require.NoError(t, err)
	retained, err := repo.HasRetainedTaskWorkspace(ctx, "unrelated-task")
	require.NoError(t, err)
	require.False(t, retained)
}

func TestRetainedShutdownRequiresMatchingRuntimeIdentity(t *testing.T) {
	target := taskStopTarget{sessionID: "session", executionID: "execution"}
	for _, saved := range []*models.ExecutorRunning{
		{TaskID: "other", SessionID: "session", AgentExecutionID: "execution", Runtime: agentruntime.RuntimeStandalone},
		{TaskID: "task", SessionID: "other", AgentExecutionID: "execution", Runtime: agentruntime.RuntimeStandalone},
		{TaskID: "task", SessionID: "session", AgentExecutionID: "other", Runtime: agentruntime.RuntimeStandalone},
	} {
		require.False(t, retainedTargetCanStop("task", target, &taskResourceCleanupSnapshot{RetainedExecutors: []*models.ExecutorRunning{saved}}))
	}
	target.runtime = agentruntime.RuntimeDocker
	require.False(t, retainedTargetCanStop("task", target, &taskResourceCleanupSnapshot{TaskEnvironment: &models.TaskEnvironment{ExecutorType: string(models.ExecutorTypeLocal)}}))
}

func TestCleanupRechecksRetentionPublishedDuringShutdown(t *testing.T) {
	svc, _, repo := createTestService(t)
	svc.StopTaskResourceCleanupWorker()
	ctx := context.Background()
	seedTaskDeletePreflightTask(t, repo, "retain-during-stop", "", false)
	cleaner := &recordingWorktreeCleanup{}
	svc.SetWorktreeCleanup(cleaner)
	var publishErr error
	stopper := &independentRetentionStopper{afterStop: func() {
		publishErr = svc.DeleteTaskWithOptions(ctx, "retain-during-stop", DeleteTaskOptions{WorkspaceCleanup: models.WorkspaceCleanupRetain})
	}}
	svc.executionStopper = stopper
	snapshot := &taskResourceCleanupSnapshot{ArchiveSourceManifestCaptured: true,
		Worktrees:   []*worktree.Worktree{{ID: "kept", TaskID: "retain-during-stop", Path: "/kept"}},
		StopTargets: []persistedTaskStopTarget{{ExecutionID: "execution", SessionID: "session"}}}
	err := svc.executeTaskResourceCleanupJob(ctx, &models.TaskResourceCleanupJob{TaskID: "retain-during-stop", Trigger: models.TaskResourceCleanupTriggerDelete}, snapshot)
	require.NoError(t, publishErr)
	require.ErrorContains(t, err, "workspace retained")
	require.Equal(t, 1, stopper.stopExecutionCalls)
	require.Empty(t, cleaner.cleanedIDs())
}

func TestRetainPreparationRequiresPriorCleanupInventory(t *testing.T) {
	svc, _, repo := createTestService(t)
	svc.StopTaskResourceCleanupWorker()
	ctx := context.Background()
	seedTaskDeletePreflightTask(t, repo, "missing-retain-inspector", "", false)
	svc.SetWorktreeCleanup(&taskDeletePreflightCleanup{})
	svc.resourceCleanups = &failingCleanupSnapshotRepository{TaskResourceCleanupRepository: repo}
	err := svc.PrepareTaskResourceCleanupWithDeleteOptions(ctx, "missing-retain-inspector", models.TaskResourceCleanupTriggerDelete, "missing-inspector", false, DeleteTaskOptions{WorkspaceCleanup: models.WorkspaceCleanupRetain})
	require.ErrorContains(t, err, "retained cleanup inventory unavailable")
	_, err = repo.GetTask(ctx, "missing-retain-inspector")
	require.NoError(t, err)
	retained, err := repo.HasRetainedTaskWorkspace(ctx, "unrelated-task")
	require.NoError(t, err)
	require.False(t, retained)
}
