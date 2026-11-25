package service

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/worktree"
	"github.com/stretchr/testify/require"
)

func TestArchiveInspectionFailurePersistsRemovedRoots(t *testing.T) {
	svc, repo := setupOfficeTest(t)
	svc.StopTaskResourceCleanupWorker()
	fixture := newMissingWorktreeCleanupFixture(t, repo, "archive-roots")
	svc.SetWorktreeCleanup(fixture.manager)
	svc.SetEnvironmentDestroyer(&managerEnvironmentDestroyer{mgr: fixture.manager})
	require.NoError(t, os.WriteFile(fixture.worktreeA.Path, []byte("keep"), 0600))
	require.NoError(t, svc.ArchiveTask(t.Context(), fixture.taskID))
	job := latestCleanupJob(t, repo, fixture.taskID, models.TaskResourceCleanupTriggerArchive)
	require.Error(t, svc.processTaskResourceCleanupJob(t.Context(), job.ID))
	require.NoDirExists(t, fixture.worktreeB.Path)
	saved, err := repo.GetTaskResourceCleanupJob(t.Context(), job.ID)
	require.NoError(t, err)
	var snapshot taskResourceCleanupSnapshot
	require.NoError(t, json.Unmarshal([]byte(saved.ResourceSnapshot), &snapshot))
	require.Contains(t, snapshot.OrphanReapRoots, resolveOrphanReapPathBestEffort(fixture.worktreeB.Path))
	require.NotContains(t, snapshot.OrphanReapRoots, resolveOrphanReapPathBestEffort(fixture.worktreeA.Path))
}

func TestArchiveInspectionFailureDeletesStoppedRuntimeRows(t *testing.T) {
	for _, failedStop := range []bool{false, true} {
		t.Run(map[bool]string{false: "all-stopped", true: "mixed-stops"}[failedStop], func(t *testing.T) {
			svc, _, repo := createTestService(t)
			svc.StopTaskResourceCleanupWorker()
			seedTaskDeletePreflightTask(t, repo, "archived", "", false)
			require.NoError(t, repo.ArchiveTask(t.Context(), "archived"))
			for _, session := range []string{"stopped", "other"} {
				require.NoError(t, repo.UpsertExecutorRunning(t.Context(), &models.ExecutorRunning{
					ID: session, TaskID: "archived", SessionID: session, ExecutorID: "executor",
					Runtime: "standalone", Status: models.ExecutorRunningStatusRunning,
				}))
			}
			stopper := sessionSelectiveFailingStopper{}
			if failedStop {
				stopper.failSessionID = "other"
			}
			svc.executionStopper = stopper
			snapshot := taskResourceCleanupSnapshot{
				Worktrees:                []*worktree.Worktree{{ID: "broken", TaskID: "archived"}},
				CleanupPreparationErrors: map[string]string{"broken": "physical_identity_unavailable"},
			}
			payload, err := json.Marshal(snapshot)
			require.NoError(t, err)
			job := &models.TaskResourceCleanupJob{ID: "blocked", TaskID: "archived", OperationID: "blocked", Trigger: models.TaskResourceCleanupTriggerArchive, State: models.TaskResourceCleanupStateRunning, Attempts: 1, ResourceSnapshot: string(payload)}
			require.NoError(t, repo.CreateTaskResourceCleanupJob(t.Context(), job))
			cleanupErr := svc.executeTaskResourceCleanupJob(t.Context(), job, &snapshot)
			require.Error(t, cleanupErr)
			if failedStop {
				require.ErrorContains(t, cleanupErr, "runtime stop operations failed")
			} else {
				require.ErrorContains(t, cleanupErr, "original workspace identity is unavailable")
			}
			_, err = repo.GetExecutorRunningBySessionID(t.Context(), "stopped")
			require.ErrorIs(t, err, models.ErrExecutorRunningNotFound)
			_, err = repo.GetExecutorRunningBySessionID(t.Context(), "other")
			if failedStop {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, models.ErrExecutorRunningNotFound)
			}
		})
	}
}

func TestArchiveInspectionFailureManifestSkipsNilWorktrees(t *testing.T) {
	svc, _, repo := createTestService(t)
	svc.StopTaskResourceCleanupWorker()
	seedTaskDeletePreflightTask(t, repo, "archived", "", false)
	require.NoError(t, repo.ArchiveTask(t.Context(), "archived"))
	svc.SetWorktreeCleanup(&archiveMixedCleanup{})
	snapshot := taskResourceCleanupSnapshot{Worktrees: []*worktree.Worktree{nil, {ID: "healthy", TaskID: "archived"}}}
	payload, err := json.Marshal(snapshot)
	require.NoError(t, err)
	job := &models.TaskResourceCleanupJob{ID: "nil-archive", TaskID: "archived", OperationID: "nil-archive", Trigger: models.TaskResourceCleanupTriggerArchive, State: models.TaskResourceCleanupStateRunning, Attempts: 1, ResourceSnapshot: string(payload)}
	require.NoError(t, repo.CreateTaskResourceCleanupJob(t.Context(), job))
	require.NoError(t, svc.cleanupArchiveWorktrees(t.Context(), job, &snapshot, taskRuntimeStopOutcome{}))
	require.True(t, snapshot.ArchiveSourceManifestCaptured)
}

func TestArchiveInspectionFailureReclaimRequiresOwnedManifest(t *testing.T) {
	for _, mode := range []string{"missing", "captured", "legacy"} {
		t.Run(mode, func(t *testing.T) {
			svc, _, repo, _, _, wt := createArchiveReclaimFixture(t)
			job := seedRetainedArchiveWorktree(t, svc, repo, wt.TaskID)
			// Use the wire shape so absent legacy markers remain covered.
			snapshot := map[string]any{"worktrees": []*worktree.Worktree{wt}}
			if mode != "legacy" {
				snapshot["archive_source_manifest_required"] = true
			}
			if mode == "captured" {
				snapshot["archive_source_manifest"] = []ArchiveSourceManifest{{TaskID: wt.TaskID, CleanupJobID: "original", WorktreeID: wt.ID, RepositoryID: wt.RepositoryID}}
			}
			payload, err := json.Marshal(snapshot)
			require.NoError(t, err)
			require.NoError(t, repo.CreateTaskResourceCleanupJob(t.Context(), &models.TaskResourceCleanupJob{
				ID: "original", TaskID: wt.TaskID, OperationID: "original", Trigger: models.TaskResourceCleanupTriggerArchive,
				State: models.TaskResourceCleanupStatePending, ResourceSnapshot: string(payload),
			}))
			err = svc.processTaskResourceCleanupJob(t.Context(), job.ID)
			if mode == "missing" {
				require.ErrorContains(t, err, "archive source manifest is unavailable")
				require.DirExists(t, wt.Path)
			} else {
				require.NoError(t, err)
				require.NoDirExists(t, wt.Path)
			}
		})
	}
}
