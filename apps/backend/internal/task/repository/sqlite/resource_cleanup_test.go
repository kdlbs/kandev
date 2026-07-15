package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

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

func TestTaskResourceCleanupJobStaleClaimCannotOverwriteCancellation(t *testing.T) {
	ctx := context.Background()
	repo := newRepoForHealTests(t)
	job := &models.TaskResourceCleanupJob{
		ID: "job-cancel-race", OperationID: "archive:cancel-race", TaskID: "task-cancel-race",
		Trigger: models.TaskResourceCleanupTriggerArchive,
		State:   models.TaskResourceCleanupStatePending, ResourceSnapshot: `{"worktree":"preserve-me"}`,
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
		t.Fatalf("GetTaskResourceCleanupJob after claim: %v", err)
	}
	if err := repo.CancelArchiveTaskResourceCleanupJobs(ctx, job.TaskID); err != nil {
		t.Fatalf("CancelArchiveTaskResourceCleanupJobs: %v", err)
	}
	updated, err := repo.CompleteClaimedTaskResourceCleanupJob(
		ctx, job.ID, claimedJob.Attempts, models.TaskResourceCleanupStateSucceeded, "", nil,
	)
	if err != nil {
		t.Fatalf("CompleteClaimedTaskResourceCleanupJob: %v", err)
	}
	if updated {
		t.Fatal("stale claimed completion overwrote cancellation")
	}
	got, err := repo.GetTaskResourceCleanupJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("GetTaskResourceCleanupJob: %v", err)
	}
	if got.State != models.TaskResourceCleanupStateCancelled || got.Attempts != claimedJob.Attempts {
		t.Fatalf("job = state %q attempts %d, want cancelled generation %d", got.State, got.Attempts, claimedJob.Attempts)
	}
	if got.ResourceSnapshot != job.ResourceSnapshot || got.CompletedAt == nil {
		t.Fatalf("cancelled job lost historical metadata: %#v", got)
	}
}
