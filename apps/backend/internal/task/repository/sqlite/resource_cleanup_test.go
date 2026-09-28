package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

type archivedWorktreeReclaimLister interface {
	ListArchivedActiveWorktreeReclaimCandidates(
		context.Context, string, string, int,
	) ([]*models.TaskArchiveReclaimCandidate, error)
}

func TestListArchivedActiveWorktreeReclaimCandidatesUsesStableBoundedCursor(t *testing.T) {
	ctx := context.Background()
	repo := newRepoForEntityTests(t)
	const workspaceID = "workspace-archive-reclaim-candidates"
	seedWorkspace(t, repo, workspaceID)
	archiveAt := time.Date(2026, time.September, 24, 10, 30, 0, 0, time.UTC)
	for _, item := range []struct {
		taskID       string
		repositoryID string
		worktreeID   string
		archived     bool
		status       string
	}{
		{taskID: "task-archived-a", repositoryID: "repo-archive-a", worktreeID: "wt-a", archived: true, status: "active"},
		{taskID: "task-active", repositoryID: "repo-active", worktreeID: "wt-b", status: "active"},
		{taskID: "task-archived-c", repositoryID: "repo-archive-c", worktreeID: "wt-c", archived: true, status: "deleted"},
		{taskID: "task-archived-d", repositoryID: "repo-archive-d", worktreeID: "wt-d", archived: true, status: "active"},
	} {
		if err := repo.CreateTask(ctx, &models.Task{ID: item.taskID, WorkspaceID: workspaceID, Title: item.taskID}); err != nil {
			t.Fatalf("CreateTask(%s): %v", item.taskID, err)
		}
		if item.archived {
			if _, err := repo.db.ExecContext(ctx, `UPDATE tasks SET archived_at = ? WHERE id = ?`, archiveAt, item.taskID); err != nil {
				t.Fatalf("archive %s: %v", item.taskID, err)
			}
		}
		repositoryPath := filepath.Join(t.TempDir(), item.repositoryID)
		if err := repo.CreateRepository(ctx, &models.Repository{
			ID: item.repositoryID, WorkspaceID: workspaceID, Name: item.repositoryID, LocalPath: repositoryPath,
		}); err != nil {
			t.Fatalf("CreateRepository(%s): %v", item.repositoryID, err)
		}
		env := &models.TaskEnvironment{
			ID: "env-" + item.taskID, TaskID: item.taskID,
			ExecutorType: string(models.ExecutorTypeLocal), Status: models.TaskEnvironmentStatusReady,
		}
		if err := repo.CreateTaskEnvironment(ctx, env); err != nil {
			t.Fatalf("CreateTaskEnvironment(%s): %v", item.taskID, err)
		}
		link := &models.TaskEnvironmentRepo{
			ID: "link-" + item.worktreeID, TaskEnvironmentID: env.ID, RepositoryID: item.repositoryID,
			WorktreeID: item.worktreeID, WorktreePath: filepath.Join(repositoryPath, item.worktreeID), Status: item.status,
		}
		if item.status == "deleted" {
			deletedAt := archiveAt.Add(time.Hour)
			link.DeletedAt = &deletedAt
		}
		if err := repo.CreateTaskEnvironmentRepo(ctx, link); err != nil {
			t.Fatalf("CreateTaskEnvironmentRepo(%s): %v", item.worktreeID, err)
		}
	}

	lister, ok := any(repo).(archivedWorktreeReclaimLister)
	if !ok {
		t.Fatal("repository does not list archived active worktree reclaim candidates")
	}
	first, err := lister.ListArchivedActiveWorktreeReclaimCandidates(ctx, "", "", 1)
	if err != nil {
		t.Fatalf("first candidate page: %v", err)
	}
	if len(first) != 1 || first[0].WorktreeID != "wt-a" || first[0].TaskID != "task-archived-a" || !first[0].ArchivedAt.Equal(archiveAt) {
		t.Fatalf("first candidate page = %#v, want wt-a with its archive generation", first)
	}
	second, err := lister.ListArchivedActiveWorktreeReclaimCandidates(ctx, "", first[0].WorktreeID, 10)
	if err != nil {
		t.Fatalf("second candidate page: %v", err)
	}
	if len(second) != 1 || second[0].WorktreeID != "wt-d" || second[0].TaskID != "task-archived-d" {
		t.Fatalf("second candidate page = %#v, want only active archived wt-d", second)
	}
	if second[0].RepositoryPath == "" || second[0].WorktreePath == "" {
		t.Fatalf("candidate paths = repository %q worktree %q, want recorded paths", second[0].RepositoryPath, second[0].WorktreePath)
	}
}

func TestTaskResourceCleanupJobSurvivesTaskDeletion(t *testing.T) {
	ctx := context.Background()
	repo := newRepoForHealTests(t)
	seedExecutorRunningCleanupTask(t, repo, "task-cleanup")

	job := &models.TaskResourceCleanupJob{
		ID: "job-1", OperationID: "delete:task-cleanup", TaskID: "task-cleanup",
		Trigger:          models.TaskResourceCleanupTriggerDelete,
		State:            models.TaskResourceCleanupStatePending,
		ResourceSnapshot: `{"workspace_path":"/tmp/task-cleanup"}`,
	}
	if err := repo.CreateTaskResourceCleanupJob(ctx, job); err != nil {
		t.Fatalf("persist cleanup intent before task deletion: %v", err)
	}
	if err := repo.DeleteTask(ctx, "task-cleanup"); err != nil {
		t.Fatalf("DeleteTask: %v", err)
	}

	var taskID, snapshot string
	if err := repo.ro.QueryRowContext(ctx, `
		SELECT task_id, resource_snapshot
		FROM task_resource_cleanup_jobs
		WHERE operation_id = 'delete:task-cleanup'
	`).Scan(&taskID, &snapshot); err != nil {
		t.Fatalf("load cleanup job after task deletion: %v", err)
	}
	if taskID != "task-cleanup" || snapshot != `{"workspace_path":"/tmp/task-cleanup"}` {
		t.Fatalf("cleanup snapshot changed after task cascade: task_id=%q snapshot=%q", taskID, snapshot)
	}
}

func TestTaskResourceCleanupJobClaimAndRetry(t *testing.T) {
	ctx := context.Background()
	repo := newRepoForHealTests(t)
	job := &models.TaskResourceCleanupJob{
		ID: "job-retry", OperationID: "delete:retry", TaskID: "task-retry",
		Trigger: models.TaskResourceCleanupTriggerDelete,
		State:   models.TaskResourceCleanupStatePending, ResourceSnapshot: `{}`,
	}
	if err := repo.CreateTaskResourceCleanupJob(ctx, job); err != nil {
		t.Fatalf("CreateTaskResourceCleanupJob: %v", err)
	}
	claimed, err := repo.MarkTaskResourceCleanupJobRunning(ctx, job.ID)
	if err != nil || !claimed {
		t.Fatalf("first claim = %v, %v; want true", claimed, err)
	}
	claimed, err = repo.MarkTaskResourceCleanupJobRunning(ctx, job.ID)
	if err != nil || claimed {
		t.Fatalf("second claim = %v, %v; want false", claimed, err)
	}
	past := time.Now().UTC().Add(-time.Minute)
	if err := repo.CompleteTaskResourceCleanupJob(ctx, job.ID, models.TaskResourceCleanupStateRetryWait, "retry", &past); err != nil {
		t.Fatalf("mark retry: %v", err)
	}
	due, err := repo.ListDueTaskResourceCleanupJobs(ctx, time.Now().UTC(), 10)
	if err != nil || len(due) != 1 || due[0].ID != job.ID {
		t.Fatalf("due jobs = %#v, %v; want job-retry", due, err)
	}
}

func TestListDueTaskResourceCleanupJobsIncludesOnlyDueWaitingForClean(t *testing.T) {
	ctx := context.Background()
	repo := newRepoForHealTests(t)
	now := time.Now().UTC()
	dueAt := now.Add(-time.Minute)
	futureAt := now.Add(time.Hour)
	for _, job := range []*models.TaskResourceCleanupJob{
		{
			ID: "job-waiting-due", OperationID: "archive_reclaim:due", TaskID: "task-waiting-due",
			Trigger: models.TaskResourceCleanupTrigger("archive_reclaim"),
			State:   models.TaskResourceCleanupState("waiting_for_clean"), ResourceSnapshot: `{}`, NextAttemptAt: &dueAt,
		},
		{
			ID: "job-waiting-future", OperationID: "archive_reclaim:future", TaskID: "task-waiting-future",
			Trigger: models.TaskResourceCleanupTrigger("archive_reclaim"),
			State:   models.TaskResourceCleanupState("waiting_for_clean"), ResourceSnapshot: `{}`, NextAttemptAt: &futureAt,
		},
		{
			ID: "job-pending", OperationID: "archive_reclaim:pending", TaskID: "task-pending",
			Trigger: models.TaskResourceCleanupTrigger("archive_reclaim"),
			State:   models.TaskResourceCleanupStatePending, ResourceSnapshot: `{}`,
		},
	} {
		if err := repo.CreateTaskResourceCleanupJob(ctx, job); err != nil {
			t.Fatalf("CreateTaskResourceCleanupJob(%s): %v", job.ID, err)
		}
	}

	due, err := repo.ListDueTaskResourceCleanupJobs(ctx, now, 10)
	if err != nil {
		t.Fatalf("ListDueTaskResourceCleanupJobs: %v", err)
	}
	got := make(map[string]bool, len(due))
	for _, job := range due {
		got[job.ID] = true
	}
	if !got["job-waiting-due"] || !got["job-pending"] || got["job-waiting-future"] || len(got) != 2 {
		t.Fatalf("due job IDs = %#v, want due waiting and pending only", got)
	}
}

func TestCancelTaskResourceCleanupJobIfPendingDoesNotOverwriteRunningClaims(t *testing.T) {
	ctx := context.Background()
	repo := newRepoForHealTests(t)
	pending := &models.TaskResourceCleanupJob{
		ID: "job-cancel-pending", OperationID: "archive:pending", TaskID: "task-cancel-pending",
		Trigger: models.TaskResourceCleanupTriggerCascadeArchive,
		State:   models.TaskResourceCleanupStatePending, ResourceSnapshot: `{}`,
	}
	running := &models.TaskResourceCleanupJob{
		ID: "job-cancel-running", OperationID: "archive:running", TaskID: "task-cancel-running",
		Trigger: models.TaskResourceCleanupTriggerCascadeArchive,
		State:   models.TaskResourceCleanupStateRunning, ResourceSnapshot: `{}`,
	}
	for _, job := range []*models.TaskResourceCleanupJob{pending, running} {
		if err := repo.CreateTaskResourceCleanupJob(ctx, job); err != nil {
			t.Fatalf("CreateTaskResourceCleanupJob(%s): %v", job.ID, err)
		}
	}

	cancelled, err := repo.CancelTaskResourceCleanupJobIfPending(ctx, pending.ID)
	if err != nil || !cancelled {
		t.Fatalf("pending cancellation = %v, %v; want true", cancelled, err)
	}
	cancelled, err = repo.CancelTaskResourceCleanupJobIfPending(ctx, pending.ID)
	if err != nil || cancelled {
		t.Fatalf("second pending cancellation = %v, %v; want false", cancelled, err)
	}
	cancelled, err = repo.CancelTaskResourceCleanupJobIfPending(ctx, running.ID)
	if err != nil || cancelled {
		t.Fatalf("running cancellation = %v, %v; want false", cancelled, err)
	}
	got, err := repo.GetTaskResourceCleanupJob(ctx, running.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != models.TaskResourceCleanupStateRunning {
		t.Fatalf("running state = %q, want running", got.State)
	}
}

func TestRestoreCancelledTaskResourceCleanupJobIfUnchangedFencesNewerClaim(t *testing.T) {
	ctx := context.Background()
	repo := newRepoForHealTests(t)
	job := &models.TaskResourceCleanupJob{
		ID: "job-restore", OperationID: "archive:restore", TaskID: "task-restore",
		Trigger: models.TaskResourceCleanupTriggerCascadeArchive,
		State:   models.TaskResourceCleanupStateCancelled, Attempts: 3, ResourceSnapshot: `{}`,
	}
	if err := repo.CreateTaskResourceCleanupJob(ctx, job); err != nil {
		t.Fatalf("CreateTaskResourceCleanupJob: %v", err)
	}

	restored, err := repo.RestoreCancelledTaskResourceCleanupJobIfUnchanged(ctx, job.ID, 3, "unknown")
	if err != nil || !restored {
		t.Fatalf("first restore = %v, %v; want true", restored, err)
	}
	got, err := repo.GetTaskResourceCleanupJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("GetTaskResourceCleanupJob after restore: %v", err)
	}
	if got.State != models.TaskResourceCleanupStatePrepared || got.LastError != "unknown" || got.CompletedAt != nil {
		t.Fatalf("restored job = %+v, want prepared with open completion", got)
	}

	started, err := repo.StartPreparedTaskResourceCleanupJob(ctx, job.ID)
	if err != nil || !started {
		t.Fatalf("start restored job = %v, %v; want true", started, err)
	}
	claimed, err := repo.MarkTaskResourceCleanupJobRunning(ctx, job.ID)
	if err != nil || !claimed {
		t.Fatalf("claim restored job = %v, %v; want true", claimed, err)
	}

	restored, err = repo.RestoreCancelledTaskResourceCleanupJobIfUnchanged(ctx, job.ID, 3, "stale")
	if err != nil {
		t.Fatalf("stale restore: %v", err)
	}
	if restored {
		t.Fatal("stale restore succeeded after a newer claim")
	}
	got, err = repo.GetTaskResourceCleanupJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("GetTaskResourceCleanupJob after stale restore: %v", err)
	}
	if got.State != models.TaskResourceCleanupStateRunning || got.Attempts != 4 || got.LastError == "stale" {
		t.Fatalf("newer claim was overwritten: %+v", got)
	}
}

func TestCancelArchiveTaskResourceCleanupJobsLeavesRunningClaims(t *testing.T) {
	ctx := context.Background()
	repo := newRepoForHealTests(t)
	for _, job := range []*models.TaskResourceCleanupJob{
		{
			ID: "job-broad-pending", OperationID: "archive:broad-pending", TaskID: "task-broad",
			Trigger: models.TaskResourceCleanupTriggerCascadeArchive,
			State:   models.TaskResourceCleanupStatePending, ResourceSnapshot: `{}`,
		},
		{
			ID: "job-broad-running", OperationID: "archive:broad-running", TaskID: "task-broad",
			Trigger: models.TaskResourceCleanupTriggerCascadeArchive,
			State:   models.TaskResourceCleanupStateRunning, ResourceSnapshot: `{}`,
		},
		{
			ID: "job-broad-waiting", OperationID: "archive_reclaim:broad-waiting", TaskID: "task-broad",
			Trigger: models.TaskResourceCleanupTrigger("archive_reclaim"),
			State:   models.TaskResourceCleanupState("waiting_for_clean"), ResourceSnapshot: `{}`,
		},
	} {
		if err := repo.CreateTaskResourceCleanupJob(ctx, job); err != nil {
			t.Fatalf("CreateTaskResourceCleanupJob(%s): %v", job.ID, err)
		}
	}

	if err := repo.CancelArchiveTaskResourceCleanupJobs(ctx, "task-broad"); err != nil {
		t.Fatalf("CancelArchiveTaskResourceCleanupJobs: %v", err)
	}
	pending, err := repo.GetTaskResourceCleanupJob(ctx, "job-broad-pending")
	if err != nil {
		t.Fatal(err)
	}
	if pending.State != models.TaskResourceCleanupStateCancelled {
		t.Fatalf("pending state = %q, want cancelled", pending.State)
	}
	waiting, err := repo.GetTaskResourceCleanupJob(ctx, "job-broad-waiting")
	if err != nil {
		t.Fatal(err)
	}
	if waiting.State != models.TaskResourceCleanupStateCancelled {
		t.Fatalf("waiting state = %q, want cancelled", waiting.State)
	}
	running, err := repo.GetTaskResourceCleanupJob(ctx, "job-broad-running")
	if err != nil {
		t.Fatal(err)
	}
	if running.State != models.TaskResourceCleanupStateRunning {
		t.Fatalf("running state = %q, want running", running.State)
	}
}

func TestListPreparedTaskResourceCleanupJobsExcludesRunnableStates(t *testing.T) {
	ctx := context.Background()
	repo := newRepoForHealTests(t)
	for _, job := range []*models.TaskResourceCleanupJob{
		{
			ID: "job-prepared", OperationID: "delete:prepared", TaskID: "task-prepared",
			Trigger: models.TaskResourceCleanupTriggerDelete, State: models.TaskResourceCleanupStatePrepared,
		},
		{
			ID: "job-pending", OperationID: "delete:pending", TaskID: "task-pending",
			Trigger: models.TaskResourceCleanupTriggerDelete, State: models.TaskResourceCleanupStatePending,
		},
	} {
		if err := repo.CreateTaskResourceCleanupJob(ctx, job); err != nil {
			t.Fatalf("CreateTaskResourceCleanupJob(%s): %v", job.ID, err)
		}
	}

	jobs, err := repo.ListPreparedTaskResourceCleanupJobs(ctx)
	if err != nil {
		t.Fatalf("ListPreparedTaskResourceCleanupJobs: %v", err)
	}
	if len(jobs) != 1 || jobs[0].ID != "job-prepared" {
		t.Fatalf("prepared jobs = %#v, want only job-prepared", jobs)
	}
}

func TestHasActiveTaskResourceCleanupJobTracksAdmissionStates(t *testing.T) {
	ctx := context.Background()
	repo := newRepoForHealTests(t)
	states := []struct {
		state  models.TaskResourceCleanupState
		active bool
	}{
		{models.TaskResourceCleanupStatePrepared, true},
		{models.TaskResourceCleanupStatePending, true},
		{models.TaskResourceCleanupStateRunning, true},
		{models.TaskResourceCleanupStateRetryWait, true},
		{models.TaskResourceCleanupState("waiting_for_clean"), true},
		{models.TaskResourceCleanupStateSucceeded, false},
		{models.TaskResourceCleanupStateFailed, false},
		{models.TaskResourceCleanupStateCancelled, false},
	}
	for _, tt := range states {
		t.Run(string(tt.state), func(t *testing.T) {
			taskID := "task-" + string(tt.state)
			job := &models.TaskResourceCleanupJob{
				ID: "job-" + string(tt.state), OperationID: "delete:" + taskID, TaskID: taskID,
				Trigger: models.TaskResourceCleanupTriggerDelete, State: tt.state, ResourceSnapshot: `{}`,
			}
			if err := repo.CreateTaskResourceCleanupJob(ctx, job); err != nil {
				t.Fatalf("CreateTaskResourceCleanupJob: %v", err)
			}
			active, err := repo.HasActiveTaskResourceCleanupJob(ctx, taskID)
			if err != nil {
				t.Fatalf("HasActiveTaskResourceCleanupJob: %v", err)
			}
			if active != tt.active {
				t.Fatalf("active = %v, want %v", active, tt.active)
			}
		})
	}
}

func TestTaskResourceCleanupFailedJobIsTerminalAndNotDue(t *testing.T) {
	ctx := context.Background()
	repo := newRepoForHealTests(t)
	job := &models.TaskResourceCleanupJob{
		ID: "job-failed", OperationID: "delete:failed", TaskID: "task-failed",
		Trigger: models.TaskResourceCleanupTriggerDelete,
		State:   models.TaskResourceCleanupStatePending, ResourceSnapshot: `{}`,
	}
	if err := repo.CreateTaskResourceCleanupJob(ctx, job); err != nil {
		t.Fatalf("CreateTaskResourceCleanupJob: %v", err)
	}
	claimed, err := repo.MarkTaskResourceCleanupJobRunning(ctx, job.ID)
	if err != nil || !claimed {
		t.Fatalf("claim = %v, %v; want true", claimed, err)
	}
	claimedJob, err := repo.GetTaskResourceCleanupJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("GetTaskResourceCleanupJob: %v", err)
	}
	if _, err := repo.CompleteClaimedTaskResourceCleanupJob(
		ctx, job.ID, claimedJob.Attempts, models.TaskResourceCleanupState("failed"), "permanent failure", nil,
	); err != nil {
		t.Fatalf("complete failed cleanup: %v", err)
	}
	got, err := repo.GetTaskResourceCleanupJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("reload failed cleanup: %v", err)
	}
	if got.CompletedAt == nil {
		t.Fatal("failed cleanup has no completion timestamp")
	}
	due, err := repo.ListDueTaskResourceCleanupJobs(ctx, time.Now().UTC(), 10)
	if err != nil {
		t.Fatalf("ListDueTaskResourceCleanupJobs: %v", err)
	}
	if len(due) != 0 {
		t.Fatalf("failed cleanup is due: %#v", due)
	}
}

func TestTaskResourceCleanupFailedUnclaimedCompletionHasTimestamp(t *testing.T) {
	ctx := context.Background()
	repo := newRepoForHealTests(t)
	job := &models.TaskResourceCleanupJob{
		ID: "job-failed-unclaimed", OperationID: "delete:failed-unclaimed", TaskID: "task-failed-unclaimed",
		Trigger: models.TaskResourceCleanupTriggerDelete,
		State:   models.TaskResourceCleanupStatePending, ResourceSnapshot: `{}`,
	}
	if err := repo.CreateTaskResourceCleanupJob(ctx, job); err != nil {
		t.Fatalf("CreateTaskResourceCleanupJob: %v", err)
	}
	if err := repo.CompleteTaskResourceCleanupJob(
		ctx, job.ID, models.TaskResourceCleanupStateFailed, "permanent failure", nil,
	); err != nil {
		t.Fatalf("complete unclaimed failed cleanup: %v", err)
	}
	got, err := repo.GetTaskResourceCleanupJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("reload failed cleanup: %v", err)
	}
	if got.CompletedAt == nil {
		t.Fatal("unclaimed failed cleanup has no completion timestamp")
	}
}

func TestPreparedCleanupSnapshotStartAndRunningReset(t *testing.T) {
	ctx := context.Background()
	repo := newRepoForHealTests(t)
	job := &models.TaskResourceCleanupJob{
		ID: "job-prepared-lifecycle", OperationID: "delete:prepared-lifecycle", TaskID: "task-prepared-lifecycle",
		Trigger: models.TaskResourceCleanupTriggerDelete, State: models.TaskResourceCleanupStatePrepared,
	}
	if err := repo.CreateTaskResourceCleanupJob(ctx, job); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpdateTaskResourceCleanupSnapshot(ctx, job.OperationID, `{"worktrees":["one"]}`); err != nil {
		t.Fatalf("UpdateTaskResourceCleanupSnapshot: %v", err)
	}
	if err := repo.UpdateTaskResourceCleanupSnapshot(ctx, "missing", `{}`); err == nil {
		t.Fatal("missing prepared snapshot update returned nil")
	}
	started, err := repo.StartPreparedTaskResourceCleanupJob(ctx, job.ID)
	if err != nil || !started {
		t.Fatalf("StartPreparedTaskResourceCleanupJob = %v, %v", started, err)
	}
	started, err = repo.StartPreparedTaskResourceCleanupJob(ctx, job.ID)
	if err != nil || started {
		t.Fatalf("second StartPreparedTaskResourceCleanupJob = %v, %v", started, err)
	}
	claimed, err := repo.MarkTaskResourceCleanupJobRunning(ctx, job.ID)
	if err != nil || !claimed {
		t.Fatalf("MarkTaskResourceCleanupJobRunning = %v, %v", claimed, err)
	}
	if err := repo.ResetRunningTaskResourceCleanupJobs(ctx); err != nil {
		t.Fatalf("ResetRunningTaskResourceCleanupJobs: %v", err)
	}
	got, err := repo.GetTaskResourceCleanupJobByOperationID(ctx, job.OperationID)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != models.TaskResourceCleanupStateRetryWait || got.NextAttemptAt == nil || got.ResourceSnapshot != `{"worktrees":["one"]}` {
		t.Fatalf("reset job = %+v", got)
	}
	if _, err := repo.GetTaskResourceCleanupJobByOperationID(ctx, "missing"); err == nil {
		t.Fatal("missing operation lookup returned nil")
	}
}
