package service

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	"github.com/kandev/kandev/internal/worktree"
	"github.com/stretchr/testify/require"
)

// @covers AC-TASKS-DIRTY-WORKTREE-ARCHIVE-005.1
func TestArchiveInspectionFailureCommitsTree(t *testing.T) {
	for _, cascade := range []bool{false, true} {
		t.Run(map[bool]string{false: "direct", true: "cascade"}[cascade], func(t *testing.T) {
			svc, _, repo := createTestService(t)
			svc.StopTaskResourceCleanupWorker()
			seedTaskDeletePreflightTask(t, repo, "root", "", false)
			seedTaskDeletePreflightTask(t, repo, "damaged", "root", false)
			svc.SetWorktreeCleanup(&unavailableIdentityCleanup{taskDeletePreflightCleanup: taskDeletePreflightCleanup{worktreesByTaskID: map[string][]*worktree.Worktree{"damaged": {{ID: "broken", TaskID: "damaged"}, {ID: "healthy", TaskID: "damaged"}}}}})
			ctx := context.Background()
			if cascade {
				h := NewHandoffService(repo, nil, nil, nil, nil, nil)
				h.SetTaskResourceCleaner(svc)
				_, err := h.ArchiveTaskTree(ctx, "root", true)
				require.NoError(t, err)
			} else {
				require.NoError(t, svc.ArchiveTask(ctx, "damaged"))
			}
			task, err := repo.GetTask(ctx, "damaged")
			require.NoError(t, err)
			require.NotNil(t, task.ArchivedAt)
			jobs, err := repo.ListTaskResourceCleanupJobs(ctx, "damaged")
			require.NoError(t, err)
			require.NotEmpty(t, jobs)
			var snapshot taskResourceCleanupSnapshot
			require.NoError(t, json.Unmarshal([]byte(jobs[0].ResourceSnapshot), &snapshot))
			require.True(t, snapshot.ArchiveSourceManifestRequired)
			require.Contains(t, snapshot.CleanupPreparationErrors, "broken")
			require.NotEmpty(t, snapshot.WorktreeHeadOIDs["healthy"])
			require.False(t, snapshot.AllowUnverifiedWorkspaceChanges)
		})
	}
}

type archiveInspectionCleaner struct {
	unavailableIdentityCleanup
	err error
}

func (c *archiveInspectionCleaner) CaptureCleanupHeadOIDs(context.Context, []*worktree.Worktree) (map[string]string, error) {
	return nil, c.err
}

// @covers AC-TASKS-DIRTY-WORKTREE-ARCHIVE-005.4
func TestArchiveInspectionFailureKeepsAdmissionErrorsFatal(t *testing.T) {
	for _, cause := range []error{errors.New("database unavailable"), context.Canceled} {
		t.Run(cause.Error(), func(t *testing.T) {
			svc, _, repo := createTestService(t)
			svc.StopTaskResourceCleanupWorker()
			seedTaskDeletePreflightTask(t, repo, "damaged", "", false)
			svc.SetWorktreeCleanup(&archiveInspectionCleaner{unavailableIdentityCleanup: unavailableIdentityCleanup{taskDeletePreflightCleanup: taskDeletePreflightCleanup{worktreesByTaskID: map[string][]*worktree.Worktree{"damaged": {{ID: "broken", TaskID: "damaged"}}}}}, err: cause})
			require.ErrorIs(t, svc.ArchiveTask(context.Background(), "damaged"), cause)
			task, err := repo.GetTask(context.Background(), "damaged")
			require.NoError(t, err)
			require.Nil(t, task.ArchivedAt)
		})
	}
}

// @covers AC-TASKS-DIRTY-WORKTREE-ARCHIVE-005.2
// @covers AC-TASKS-DIRTY-WORKTREE-ARCHIVE-005.3
func TestArchiveInspectionFailurePreservesResources(t *testing.T) {
	svc, _, repo := createTestService(t)
	svc.StopTaskResourceCleanupWorker()
	seedTaskDeletePreflightTask(t, repo, "archived", "", false)
	require.NoError(t, repo.ArchiveTask(t.Context(), "archived"))
	cleaner := &archiveMixedCleanup{}
	svc.SetWorktreeCleanup(cleaner)
	snapshot := taskResourceCleanupSnapshot{Worktrees: []*worktree.Worktree{{ID: "broken", TaskID: "archived"}, {ID: "healthy", TaskID: "archived"}}, WorktreeHeadOIDs: map[string]string{"healthy": "original"}, CleanupPreparationErrors: map[string]string{"broken": "physical_identity_unavailable"}}
	payload, err := json.Marshal(snapshot)
	require.NoError(t, err)
	job := &models.TaskResourceCleanupJob{ID: "archive-inspection", TaskID: "archived", OperationID: "archive-inspection", Trigger: models.TaskResourceCleanupTriggerArchive, State: models.TaskResourceCleanupStateRunning, Attempts: 1, ResourceSnapshot: string(payload)}
	require.NoError(t, repo.CreateTaskResourceCleanupJob(context.Background(), job))
	require.ErrorContains(t, svc.executeTaskResourceCleanupJob(context.Background(), job, &snapshot), "original workspace identity is unavailable")
	require.Equal(t, []string{"healthy"}, cleaner.cleanedIDs())
	saved, err := repo.GetTaskResourceCleanupJob(context.Background(), job.ID)
	require.NoError(t, err)
	snapshot = taskResourceCleanupSnapshot{}
	require.NoError(t, json.Unmarshal([]byte(saved.ResourceSnapshot), &snapshot))
	cleaner.repaired = true
	require.ErrorContains(t, svc.executeTaskResourceCleanupJob(context.Background(), job, &snapshot), "original workspace identity is unavailable")
	require.Equal(t, []string{"healthy"}, cleaner.cleanedIDs(), "retry must not substitute current HEAD for unknown original identity")
}

type archiveMixedCleanup struct{ mixedUnavailableCleanup }

func (c *archiveMixedCleanup) CleanupWorktreesPreservingBranches(ctx context.Context, trees []*worktree.Worktree) error {
	return c.CleanupWorktrees(ctx, trees)
}

// @covers AC-TASKS-DIRTY-WORKTREE-ARCHIVE-005.2
func TestArchiveInspectionFailureReclaimCannotReplaceOriginalIdentity(t *testing.T) {
	svc, _, repo, _, _, wt := createArchiveReclaimFixture(t)
	job := seedRetainedArchiveWorktree(t, svc, repo, wt.TaskID)
	snapshot, err := json.Marshal(taskResourceCleanupSnapshot{CleanupPreparationErrors: map[string]string{wt.ID: "physical_identity_unavailable"}})
	require.NoError(t, err)
	require.NoError(t, repo.CreateTaskResourceCleanupJob(t.Context(), &models.TaskResourceCleanupJob{ID: "blocked-original", TaskID: wt.TaskID, OperationID: "blocked-original", Trigger: models.TaskResourceCleanupTriggerArchive, State: models.TaskResourceCleanupStatePending, ResourceSnapshot: string(snapshot)}))
	require.ErrorContains(t, svc.processTaskResourceCleanupJob(t.Context(), job.ID), "original workspace identity is unavailable")
	require.DirExists(t, wt.Path)
}

// @covers AC-TASKS-DIRTY-WORKTREE-ARCHIVE-005.2
// @covers AC-TASKS-DIRTY-WORKTREE-ARCHIVE-005.3
func TestArchiveInspectionFailureKeepsDirectory(t *testing.T) {
	svc, repo := setupOfficeTest(t)
	svc.StopTaskResourceCleanupWorker()
	fixture := newMissingWorktreeCleanupFixture(t, repo, "archive-damaged")
	svc.SetWorktreeCleanup(fixture.manager)
	svc.SetEnvironmentDestroyer(&managerEnvironmentDestroyer{mgr: fixture.manager})
	require.NoError(t, os.WriteFile(fixture.worktreeA.Path, []byte("keep"), 0600))
	require.NoError(t, svc.ArchiveTask(t.Context(), fixture.taskID))
	job := latestCleanupJob(t, repo, fixture.taskID, models.TaskResourceCleanupTriggerArchive)
	require.Error(t, svc.processTaskResourceCleanupJob(t.Context(), job.ID))
	require.FileExists(t, fixture.worktreeA.Path)
	require.DirExists(t, filepath.Dir(fixture.worktreeA.Path))
	require.NoDirExists(t, fixture.worktreeB.Path)
	restarted := newServiceOverRepository(t, repo)
	restarted.SetWorktreeCleanup(fixture.manager)
	require.Error(t, restarted.processTaskResourceCleanupJob(t.Context(), job.ID))
	content, err := os.ReadFile(fixture.worktreeA.Path)
	require.NoError(t, err)
	require.Equal(t, "keep", string(content))
}

func TestArchiveInspectionFailureSnapshotFailure(t *testing.T) {
	svc, _, repo := createTestService(t)
	svc.StopTaskResourceCleanupWorker()
	seedTaskDeletePreflightTask(t, repo, "damaged", "", false)
	svc.SetWorktreeCleanup(&unavailableIdentityCleanup{taskDeletePreflightCleanup: taskDeletePreflightCleanup{worktreesByTaskID: map[string][]*worktree.Worktree{"damaged": {{ID: "broken", TaskID: "damaged"}}}}})
	svc.resourceCleanups = &archiveSnapshotFailure{TaskResourceCleanupRepository: repo}
	require.ErrorContains(t, svc.ArchiveTask(t.Context(), "damaged"), "snapshot write failed")
	task, err := repo.GetTask(t.Context(), "damaged")
	require.NoError(t, err)
	require.Nil(t, task.ArchivedAt)
}

type archiveSnapshotFailure struct {
	repository.TaskResourceCleanupRepository
}

func (*archiveSnapshotFailure) CreateTaskResourceCleanupJob(context.Context, *models.TaskResourceCleanupJob) error {
	return errors.New("snapshot write failed")
}

func TestArchiveInspectionFailureParentDeadline(t *testing.T) {
	svc, _, repo := createTestService(t)
	svc.StopTaskResourceCleanupWorker()
	seedTaskDeletePreflightTask(t, repo, "damaged", "", false)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.Error(t, svc.ArchiveTask(ctx, "damaged"))
	task, err := repo.GetTask(t.Context(), "damaged")
	require.NoError(t, err)
	require.Nil(t, task.ArchivedAt)
}

func TestArchiveInspectionFailureUnarchiveFencesCleanup(t *testing.T) {
	svc, _, repo := createTestService(t)
	svc.StopTaskResourceCleanupWorker()
	seedTaskDeletePreflightTask(t, repo, "restored", "", false)
	require.NoError(t, repo.ArchiveTask(t.Context(), "restored"))
	cleaner := &archiveMixedCleanup{}
	svc.SetWorktreeCleanup(cleaner)
	payload := `{"worktrees":[{"id":"healthy","task_id":"restored"}],"worktree_head_oids":{"healthy":"original"}}`
	job := &models.TaskResourceCleanupJob{ID: "cancelled-archive", OperationID: "cancelled-archive", TaskID: "restored", Trigger: models.TaskResourceCleanupTriggerArchive, State: models.TaskResourceCleanupStatePending, ResourceSnapshot: payload}
	require.NoError(t, repo.CreateTaskResourceCleanupJob(t.Context(), job))
	h := NewHandoffService(repo, nil, nil, nil, nil, nil)
	h.SetTaskResourceCleaner(svc)
	_, err := h.UnarchiveTaskTree(t.Context(), "restored")
	require.NoError(t, err)
	require.NoError(t, svc.processTaskResourceCleanupJob(t.Context(), job.ID))
	require.Empty(t, cleaner.cleanedIDs())
	saved, err := repo.GetTaskResourceCleanupJob(t.Context(), job.ID)
	require.NoError(t, err)
	require.Equal(t, models.TaskResourceCleanupStateCancelled, saved.State)
}

func TestArchiveInspectionFailureSharedWorktreeIsNotCompleted(t *testing.T) {
	svc, _, repo := createTestService(t)
	svc.StopTaskResourceCleanupWorker()
	seedTaskDeletePreflightTask(t, repo, "archived", "", false)
	require.NoError(t, repo.ArchiveTask(t.Context(), "archived"))
	cleaner := &archiveMixedCleanup{}
	cleaner.referenceCounts = map[string]int{"shared": 1}
	svc.SetWorktreeCleanup(cleaner)
	snapshot := taskResourceCleanupSnapshot{Worktrees: []*worktree.Worktree{{ID: "shared", TaskID: "archived"}}, WorktreeHeadOIDs: map[string]string{"shared": "original"}}
	payload, err := json.Marshal(snapshot)
	require.NoError(t, err)
	job := &models.TaskResourceCleanupJob{ID: "shared-archive", TaskID: "archived", OperationID: "shared-archive", Trigger: models.TaskResourceCleanupTriggerArchive, State: models.TaskResourceCleanupStateRunning, Attempts: 1, ResourceSnapshot: string(payload)}
	require.NoError(t, repo.CreateTaskResourceCleanupJob(t.Context(), job))
	require.Error(t, svc.executeTaskResourceCleanupJob(t.Context(), job, &snapshot))
	require.Empty(t, cleaner.cleanedIDs())
	require.False(t, snapshot.CompletedWorktrees["shared"])
}

func TestArchiveInspectionFailureManifestRetry(t *testing.T) {
	svc, _, repo := createTestService(t)
	svc.StopTaskResourceCleanupWorker()
	seedTaskDeletePreflightTask(t, repo, "archived", "", false)
	require.NoError(t, repo.ArchiveTask(t.Context(), "archived"))
	cleaner := &archiveMixedCleanup{}
	svc.SetWorktreeCleanup(cleaner)
	snapshot := taskResourceCleanupSnapshot{Worktrees: []*worktree.Worktree{{ID: "broken", TaskID: "archived"}, {ID: "healthy", TaskID: "archived"}}, WorktreeHeadOIDs: map[string]string{"broken": "original", "healthy": "original"}}
	payload, err := json.Marshal(snapshot)
	require.NoError(t, err)
	job := &models.TaskResourceCleanupJob{ID: "manifest-retry", TaskID: "archived", OperationID: "manifest-retry", Trigger: models.TaskResourceCleanupTriggerArchive, State: models.TaskResourceCleanupStateRunning, Attempts: 1, ResourceSnapshot: string(payload)}
	require.NoError(t, repo.CreateTaskResourceCleanupJob(t.Context(), job))
	require.Error(t, svc.executeTaskResourceCleanupJob(t.Context(), job, &snapshot))
	require.Equal(t, []string{"healthy"}, cleaner.cleanedIDs())
	require.False(t, snapshot.ArchiveSourceManifestCaptured)
	saved, err := repo.GetTaskResourceCleanupJob(t.Context(), job.ID)
	require.NoError(t, err)
	snapshot = taskResourceCleanupSnapshot{}
	require.NoError(t, json.Unmarshal([]byte(saved.ResourceSnapshot), &snapshot))
	cleaner.repaired = true
	require.NoError(t, svc.executeTaskResourceCleanupJob(t.Context(), job, &snapshot))
	require.ElementsMatch(t, []string{"healthy", "broken"}, cleaner.cleanedIDs())
}
