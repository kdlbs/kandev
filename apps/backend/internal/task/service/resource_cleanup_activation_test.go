package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
)

type transientStartCleanupRepository struct {
	repository.TaskResourceCleanupRepository
	mu       sync.Mutex
	failures int
	calls    int
}

type failOnceResetCleanupRepository struct {
	repository.TaskResourceCleanupRepository
	mu        sync.Mutex
	calls     int
	dueListed chan struct{}
	dueOnce   sync.Once
}

func (r *failOnceResetCleanupRepository) ResetRunningTaskResourceCleanupJobs(ctx context.Context) error {
	r.mu.Lock()
	r.calls++
	calls := r.calls
	r.mu.Unlock()
	if calls == 1 {
		return errors.New("transient reset running cleanup failure")
	}
	return r.TaskResourceCleanupRepository.ResetRunningTaskResourceCleanupJobs(ctx)
}

func (r *failOnceResetCleanupRepository) ListDueTaskResourceCleanupJobs(
	ctx context.Context,
	now time.Time,
	limit int,
) ([]*models.TaskResourceCleanupJob, error) {
	jobs, err := r.TaskResourceCleanupRepository.ListDueTaskResourceCleanupJobs(ctx, now, limit)
	if r.dueListed != nil {
		r.dueOnce.Do(func() { close(r.dueListed) })
	}
	return jobs, err
}

func (r *transientStartCleanupRepository) StartPreparedTaskResourceCleanupJob(
	ctx context.Context,
	id string,
) (bool, error) {
	r.mu.Lock()
	r.calls++
	if r.failures > 0 {
		r.failures--
		r.mu.Unlock()
		return false, errors.New("transient prepared cleanup start failure")
	}
	r.mu.Unlock()
	return r.TaskResourceCleanupRepository.StartPreparedTaskResourceCleanupJob(ctx, id)
}

type blockingTaskMutationRepository struct {
	repository.TaskRepository
	archiveEntered chan struct{}
	deleteEntered  chan struct{}
	release        <-chan struct{}
}

type failPostArchiveReadRepository struct {
	repository.TaskRepository
	archived bool
}

func (r *failPostArchiveReadRepository) ArchiveTask(ctx context.Context, id string) error {
	if err := r.TaskRepository.ArchiveTask(ctx, id); err != nil {
		return err
	}
	r.archived = true
	return nil
}

func (r *failPostArchiveReadRepository) GetTask(ctx context.Context, id string) (*models.Task, error) {
	if r.archived {
		return nil, errors.New("post-archive task read failed")
	}
	return r.TaskRepository.GetTask(ctx, id)
}

func (r *blockingTaskMutationRepository) ArchiveTask(ctx context.Context, id string) error {
	if r.archiveEntered != nil {
		close(r.archiveEntered)
		select {
		case <-r.release:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return r.TaskRepository.ArchiveTask(ctx, id)
}

func (r *blockingTaskMutationRepository) DeleteTask(ctx context.Context, id string) error {
	if r.deleteEntered != nil {
		close(r.deleteEntered)
		select {
		case <-r.release:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return r.TaskRepository.DeleteTask(ctx, id)
}

func TestStartPreparedCleanupUsesDetachedContextAndRetriesTransientFailure(t *testing.T) {
	taskSvc, repo := setupOfficeTest(t)
	ctx := context.Background()
	seedCleanupTaskAndSession(t, repo, "task-start-retry", "session-start-retry")
	const operationID = "delete:start-retry"
	if err := taskSvc.PrepareTaskResourceCleanup(
		ctx, "task-start-retry", models.TaskResourceCleanupTriggerDelete, operationID, true,
	); err != nil {
		t.Fatalf("PrepareTaskResourceCleanup: %v", err)
	}
	transient := &transientStartCleanupRepository{
		TaskResourceCleanupRepository: taskSvc.resourceCleanups,
		failures:                      1,
	}
	taskSvc.resourceCleanups = transient
	cancelledCtx, cancel := context.WithCancel(ctx)
	cancel()

	if err := taskSvc.StartPreparedTaskResourceCleanup(cancelledCtx, operationID); err != nil {
		t.Fatalf("StartPreparedTaskResourceCleanup: %v", err)
	}
	transient.mu.Lock()
	calls := transient.calls
	transient.mu.Unlock()
	if calls != 2 {
		t.Fatalf("start attempts = %d, want 2", calls)
	}
	job, err := repo.GetTaskResourceCleanupJobByOperationID(ctx, operationID)
	if err != nil {
		t.Fatal(err)
	}
	if job.State == models.TaskResourceCleanupStatePrepared {
		t.Fatal("cleanup remained prepared after committed lifecycle mutation")
	}
}

func TestArchiveAndDeleteCleanupRemainPreparedUntilMutationCommits(t *testing.T) {
	tests := []struct {
		name    string
		trigger models.TaskResourceCleanupTrigger
		mutate  func(context.Context, *Service, string) error
	}{
		{
			name: "archive", trigger: models.TaskResourceCleanupTriggerArchive,
			mutate: func(ctx context.Context, svc *Service, taskID string) error {
				return svc.ArchiveTask(ctx, taskID)
			},
		},
		{
			name: "delete", trigger: models.TaskResourceCleanupTriggerDelete,
			mutate: func(ctx context.Context, svc *Service, taskID string) error {
				return svc.DeleteTask(ctx, taskID)
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			taskSvc, repo := setupOfficeTest(t)
			ctx := context.Background()
			taskID := "task-two-phase-" + test.name
			seedCleanupTaskAndSession(t, repo, taskID, "session-two-phase-"+test.name)
			entered := make(chan struct{})
			release := make(chan struct{})
			var releaseOnce sync.Once
			t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })
			blocked := &blockingTaskMutationRepository{TaskRepository: taskSvc.tasks, release: release}
			if test.name == "archive" {
				blocked.archiveEntered = entered
			} else {
				blocked.deleteEntered = entered
			}
			taskSvc.tasks = blocked
			done := make(chan error, 1)
			go func() { done <- test.mutate(ctx, taskSvc, taskID) }()
			select {
			case <-entered:
			case <-time.After(time.Second):
				t.Fatal("task mutation did not reach commit barrier")
			}

			var state models.TaskResourceCleanupState
			if err := repo.DB().QueryRowContext(ctx, `
				SELECT state FROM task_resource_cleanup_jobs WHERE task_id = ? AND trigger = ?
			`, taskID, test.trigger).Scan(&state); err != nil {
				t.Fatalf("load cleanup state: %v", err)
			}
			if state != models.TaskResourceCleanupStatePrepared {
				t.Fatalf("cleanup state before task mutation commit = %q, want prepared", state)
			}
			if err := taskSvc.processDueTaskResourceCleanupJobs(ctx); err != nil {
				t.Fatalf("periodic cleanup during task mutation: %v", err)
			}
			if err := repo.DB().QueryRowContext(ctx, `
				SELECT state FROM task_resource_cleanup_jobs WHERE task_id = ? AND trigger = ?
			`, taskID, test.trigger).Scan(&state); err != nil {
				t.Fatalf("reload cleanup state: %v", err)
			}
			if state != models.TaskResourceCleanupStatePrepared {
				t.Fatalf("periodic reconciliation changed in-flight cleanup to %q", state)
			}

			releaseOnce.Do(func() { close(release) })
			select {
			case err := <-done:
				if err != nil {
					t.Fatalf("%s task: %v", test.name, err)
				}
			case <-time.After(time.Second):
				t.Fatalf("%s did not finish after mutation commit", test.name)
			}
		})
	}
}

func TestRestartReconcilesCommittedPreparedCleanup(t *testing.T) {
	tests := []struct {
		name    string
		trigger models.TaskResourceCleanupTrigger
		commit  func(context.Context, repository.TaskRepository, string) error
	}{
		{
			name: "archive", trigger: models.TaskResourceCleanupTriggerArchive,
			commit: func(ctx context.Context, tasks repository.TaskRepository, taskID string) error {
				return tasks.ArchiveTask(ctx, taskID)
			},
		},
		{
			name: "delete", trigger: models.TaskResourceCleanupTriggerDelete,
			commit: func(ctx context.Context, tasks repository.TaskRepository, taskID string) error {
				return tasks.DeleteTask(ctx, taskID)
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			taskSvc, repo := setupOfficeTest(t)
			taskSvc.StopTaskResourceCleanupWorker()
			ctx := context.Background()
			taskID := "task-restart-" + test.name
			seedCleanupTaskAndSession(t, repo, taskID, "session-restart-"+test.name)
			operationID := test.name + ":restart"
			if err := taskSvc.PrepareTaskResourceCleanup(ctx, taskID, test.trigger, operationID, true); err != nil {
				t.Fatalf("PrepareTaskResourceCleanup: %v", err)
			}
			if err := test.commit(ctx, repo, taskID); err != nil {
				t.Fatalf("commit lifecycle mutation: %v", err)
			}

			if err := taskSvc.StartTaskResourceCleanupWorker(ctx); err != nil {
				t.Fatalf("restart cleanup worker: %v", err)
			}
			job, err := repo.GetTaskResourceCleanupJobByOperationID(ctx, operationID)
			if err != nil {
				t.Fatal(err)
			}
			if job.State == models.TaskResourceCleanupStatePrepared {
				t.Fatal("committed cleanup remained prepared after worker restart")
			}
		})
	}
}

func TestPreparedCleanupReconciliationFailsClosedBeforeMutationCommit(t *testing.T) {
	tests := []struct {
		name    string
		trigger models.TaskResourceCleanupTrigger
	}{
		{name: "archive", trigger: models.TaskResourceCleanupTriggerArchive},
		{name: "delete", trigger: models.TaskResourceCleanupTriggerDelete},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			taskSvc, repo := setupOfficeTest(t)
			taskSvc.StopTaskResourceCleanupWorker()
			ctx := context.Background()
			taskID := "task-uncommitted-" + test.name
			seedCleanupTaskAndSession(t, repo, taskID, "session-uncommitted-"+test.name)
			operationID := test.name + ":uncommitted"
			if err := taskSvc.PrepareTaskResourceCleanup(ctx, taskID, test.trigger, operationID, true); err != nil {
				t.Fatalf("PrepareTaskResourceCleanup: %v", err)
			}

			if err := taskSvc.StartTaskResourceCleanupWorker(ctx); err != nil {
				t.Fatalf("restart cleanup worker: %v", err)
			}
			job, err := repo.GetTaskResourceCleanupJobByOperationID(ctx, operationID)
			if err != nil {
				t.Fatal(err)
			}
			if job.State != models.TaskResourceCleanupStateCancelled {
				t.Fatalf("uncommitted cleanup state = %q, want cancelled", job.State)
			}
		})
	}
}

func TestPeriodicReconciliationRecoversPreparedCleanupAfterActivationOutage(t *testing.T) {
	taskSvc, repo := setupOfficeTest(t)
	taskSvc.StopTaskResourceCleanupWorker()
	ctx := context.Background()
	seedCleanupTaskAndSession(t, repo, "task-activation-outage", "session-activation-outage")
	const operationID = "delete:activation-outage"
	if err := taskSvc.PrepareTaskResourceCleanup(
		ctx, "task-activation-outage", models.TaskResourceCleanupTriggerDelete, operationID, true,
	); err != nil {
		t.Fatalf("PrepareTaskResourceCleanup: %v", err)
	}
	if err := repo.DeleteTask(ctx, "task-activation-outage"); err != nil {
		t.Fatalf("commit delete: %v", err)
	}

	if err := taskSvc.processDueTaskResourceCleanupJobs(ctx); err != nil {
		t.Fatalf("periodic cleanup reconciliation: %v", err)
	}
	job, err := repo.GetTaskResourceCleanupJobByOperationID(ctx, operationID)
	if err != nil {
		t.Fatal(err)
	}
	if job.State == models.TaskResourceCleanupStatePrepared {
		t.Fatal("cleanup remained prepared after activation service recovered")
	}
}

func TestPeriodicReconciliationRecoversAfterActivationRetryWindowExpires(t *testing.T) {
	taskSvc, repo := setupOfficeTest(t)
	taskSvc.StopTaskResourceCleanupWorker()
	ctx := context.Background()
	seedCleanupTaskAndSession(t, repo, "task-persistent-outage", "session-persistent-outage")
	const operationID = "delete:persistent-outage"
	if err := taskSvc.PrepareTaskResourceCleanup(
		ctx, "task-persistent-outage", models.TaskResourceCleanupTriggerDelete, operationID, true,
	); err != nil {
		t.Fatalf("PrepareTaskResourceCleanup: %v", err)
	}
	if err := repo.DeleteTask(ctx, "task-persistent-outage"); err != nil {
		t.Fatalf("commit delete: %v", err)
	}
	persistentFailure := &transientStartCleanupRepository{
		TaskResourceCleanupRepository: taskSvc.resourceCleanups,
		failures:                      1 << 30,
	}
	taskSvc.resourceCleanups = persistentFailure
	if err := taskSvc.StartPreparedTaskResourceCleanup(ctx, operationID); err == nil {
		t.Fatal("activation unexpectedly succeeded during persistent repository outage")
	}
	job, err := repo.GetTaskResourceCleanupJobByOperationID(ctx, operationID)
	if err != nil {
		t.Fatal(err)
	}
	if job.State != models.TaskResourceCleanupStatePrepared {
		t.Fatalf("cleanup state after activation outage = %q, want prepared", job.State)
	}

	taskSvc.resourceCleanups = repo
	if err := taskSvc.processDueTaskResourceCleanupJobs(ctx); err != nil {
		t.Fatalf("periodic cleanup reconciliation after recovery: %v", err)
	}
	job, err = repo.GetTaskResourceCleanupJobByOperationID(ctx, operationID)
	if err != nil {
		t.Fatal(err)
	}
	if job.State == models.TaskResourceCleanupStatePrepared {
		t.Fatal("cleanup remained prepared after persistent activation outage recovered")
	}
}

func TestPreparedActivationFailureDoesNotStarveRunnableCleanup(t *testing.T) {
	taskSvc, repo := setupOfficeTest(t)
	taskSvc.StopTaskResourceCleanupWorker()
	ctx := context.Background()
	seedCleanupTaskAndSession(t, repo, "task-starved-prepared", "session-starved-prepared")
	if err := taskSvc.PrepareTaskResourceCleanup(
		ctx, "task-starved-prepared", models.TaskResourceCleanupTriggerDelete, "delete:starved-prepared", true,
	); err != nil {
		t.Fatalf("PrepareTaskResourceCleanup: %v", err)
	}
	if err := repo.DeleteTask(ctx, "task-starved-prepared"); err != nil {
		t.Fatalf("commit prepared delete: %v", err)
	}
	pending := &models.TaskResourceCleanupJob{
		ID: "job-runnable", OperationID: "delete:runnable", TaskID: "task-runnable",
		Trigger: models.TaskResourceCleanupTriggerDelete, State: models.TaskResourceCleanupStatePending,
		ResourceSnapshot: `{}`,
	}
	if err := repo.CreateTaskResourceCleanupJob(ctx, pending); err != nil {
		t.Fatalf("CreateTaskResourceCleanupJob: %v", err)
	}
	taskSvc.resourceCleanups = &transientStartCleanupRepository{
		TaskResourceCleanupRepository: repo,
		failures:                      1 << 30,
	}

	if err := taskSvc.processDueTaskResourceCleanupJobs(ctx); err == nil {
		t.Fatal("periodic cleanup unexpectedly ignored prepared activation failure")
	}
	job, err := repo.GetTaskResourceCleanupJob(ctx, pending.ID)
	if err != nil {
		t.Fatal(err)
	}
	if job.State != models.TaskResourceCleanupStateSucceeded {
		t.Fatalf("runnable cleanup state = %q, want succeeded despite prepared activation failure", job.State)
	}
}

func TestStartupActivationFailureKeepsWorkerRunningForRecovery(t *testing.T) {
	taskSvc, repo := setupOfficeTest(t)
	taskSvc.StopTaskResourceCleanupWorker()
	ctx := context.Background()
	seedCleanupTaskAndSession(t, repo, "task-startup-outage", "session-startup-outage")
	const operationID = "delete:startup-outage"
	if err := taskSvc.PrepareTaskResourceCleanup(
		ctx, "task-startup-outage", models.TaskResourceCleanupTriggerDelete, operationID, true,
	); err != nil {
		t.Fatalf("PrepareTaskResourceCleanup: %v", err)
	}
	if err := repo.DeleteTask(ctx, "task-startup-outage"); err != nil {
		t.Fatalf("commit delete: %v", err)
	}
	taskSvc.resourceCleanups = &transientStartCleanupRepository{
		TaskResourceCleanupRepository: repo,
		failures:                      1,
	}
	taskSvc.setCleanupDoneForTestHook(make(chan struct{}, 1))
	if err := taskSvc.StartTaskResourceCleanupWorker(ctx); err == nil {
		t.Fatal("worker startup unexpectedly hid initial activation failure")
	}
	taskSvc.startTaskResourceCleanup(&models.TaskResourceCleanupJob{ID: "wake-after-recovery"})
	waitForCleanupDone(t, taskSvc)
	job, err := repo.GetTaskResourceCleanupJobByOperationID(ctx, operationID)
	if err != nil {
		t.Fatal(err)
	}
	if job.State != models.TaskResourceCleanupStateSucceeded {
		t.Fatalf("cleanup state after startup recovery = %q, want succeeded", job.State)
	}
}

func TestWorkerRetriesFullResumeAfterStartupResetFailure(t *testing.T) {
	taskSvc, repo := setupOfficeTest(t)
	taskSvc.StopTaskResourceCleanupWorker()
	ctx := context.Background()
	running := &models.TaskResourceCleanupJob{
		ID: "job-interrupted-running", OperationID: "delete:interrupted-running", TaskID: "task-running-missing",
		Trigger: models.TaskResourceCleanupTriggerDelete, State: models.TaskResourceCleanupStatePending,
		ResourceSnapshot: `{}`,
	}
	prepared := &models.TaskResourceCleanupJob{
		ID: "job-committed-prepared", OperationID: "delete:committed-prepared", TaskID: "task-prepared-missing",
		Trigger: models.TaskResourceCleanupTriggerDelete, State: models.TaskResourceCleanupStatePrepared,
		ResourceSnapshot: `{}`,
	}
	for _, job := range []*models.TaskResourceCleanupJob{running, prepared} {
		if err := repo.CreateTaskResourceCleanupJob(ctx, job); err != nil {
			t.Fatalf("CreateTaskResourceCleanupJob(%s): %v", job.ID, err)
		}
	}
	claimed, err := repo.MarkTaskResourceCleanupJobRunning(ctx, running.ID)
	if err != nil || !claimed {
		t.Fatalf("MarkTaskResourceCleanupJobRunning = %v, %v", claimed, err)
	}
	failOnce := &failOnceResetCleanupRepository{TaskResourceCleanupRepository: repo}
	taskSvc.resourceCleanups = failOnce
	taskSvc.setCleanupDoneForTestHook(make(chan struct{}, 2))

	if err := taskSvc.StartTaskResourceCleanupWorker(ctx); err == nil {
		t.Fatal("worker startup unexpectedly hid reset failure")
	}
	taskSvc.startTaskResourceCleanup(&models.TaskResourceCleanupJob{ID: "retry-full-resume"})
	waitForCleanupDone(t, taskSvc)
	waitForCleanupDone(t, taskSvc)
	for _, id := range []string{running.ID, prepared.ID} {
		job, err := repo.GetTaskResourceCleanupJob(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if job.State != models.TaskResourceCleanupStateSucceeded {
			t.Fatalf("cleanup %s state = %q, want succeeded", id, job.State)
		}
	}
}

func TestWorkerResumeRetryPreservesPreparedCleanupCreatedAfterStartup(t *testing.T) {
	taskSvc, repo := setupOfficeTest(t)
	taskSvc.StopTaskResourceCleanupWorker()
	ctx := context.Background()
	seedCleanupTaskAndSession(t, repo, "task-live-mutation", "session-live-mutation")
	const operationID = "delete:live-mutation"
	dueListed := make(chan struct{})
	failOnce := &failOnceResetCleanupRepository{
		TaskResourceCleanupRepository: repo,
		dueListed:                     dueListed,
	}
	taskSvc.resourceCleanups = failOnce

	if err := taskSvc.StartTaskResourceCleanupWorker(ctx); err == nil {
		t.Fatal("worker startup unexpectedly hid reset failure")
	}
	if err := taskSvc.PrepareTaskResourceCleanup(
		ctx, "task-live-mutation", models.TaskResourceCleanupTriggerDelete, operationID, true,
	); err != nil {
		t.Fatalf("PrepareTaskResourceCleanup: %v", err)
	}
	taskSvc.startTaskResourceCleanup(&models.TaskResourceCleanupJob{ID: "retry-during-live-mutation"})
	select {
	case <-dueListed:
	case <-time.After(time.Second):
		t.Fatal("worker did not finish startup resume retry")
	}

	job, err := repo.GetTaskResourceCleanupJobByOperationID(ctx, operationID)
	if err != nil {
		t.Fatal(err)
	}
	if job.State != models.TaskResourceCleanupStatePrepared {
		t.Fatalf("live cleanup state after startup retry = %q, want prepared", job.State)
	}

	taskSvc.setCleanupDoneForTestHook(make(chan struct{}, 1))
	if err := repo.DeleteTask(ctx, "task-live-mutation"); err != nil {
		t.Fatalf("commit live mutation: %v", err)
	}
	if err := taskSvc.StartPreparedTaskResourceCleanup(ctx, operationID); err != nil {
		t.Fatalf("StartPreparedTaskResourceCleanup: %v", err)
	}
	waitForCleanupDone(t, taskSvc)
	job, err = repo.GetTaskResourceCleanupJobByOperationID(ctx, operationID)
	if err != nil {
		t.Fatal(err)
	}
	if job.State != models.TaskResourceCleanupStateSucceeded {
		t.Fatalf("live cleanup state after commit = %q, want succeeded", job.State)
	}
}

func TestArchiveReadFailureLeavesRecoverablePreparedCleanup(t *testing.T) {
	taskSvc, repo := setupOfficeTest(t)
	taskSvc.StopTaskResourceCleanupWorker()
	ctx := context.Background()
	seedCleanupTaskAndSession(t, repo, "task-archive-reread", "session-archive-reread")
	failingTasks := &failPostArchiveReadRepository{TaskRepository: taskSvc.tasks}
	taskSvc.tasks = failingTasks

	if err := taskSvc.ArchiveTask(ctx, "task-archive-reread"); err == nil {
		t.Fatal("ArchiveTask succeeded despite forced post-commit read failure")
	}
	var operationID string
	if err := repo.DB().QueryRowContext(ctx, `
		SELECT operation_id FROM task_resource_cleanup_jobs
		WHERE task_id = ? AND trigger = ?
	`, "task-archive-reread", models.TaskResourceCleanupTriggerArchive).Scan(&operationID); err != nil {
		t.Fatalf("load prepared cleanup: %v", err)
	}
	taskSvc.tasks = repo
	if err := taskSvc.ResumeTaskResourceCleanupJobs(ctx); err != nil {
		t.Fatalf("ResumeTaskResourceCleanupJobs: %v", err)
	}
	job, err := repo.GetTaskResourceCleanupJobByOperationID(ctx, operationID)
	if err != nil {
		t.Fatal(err)
	}
	if job.State == models.TaskResourceCleanupStatePrepared {
		t.Fatal("post-commit archive cleanup remained prepared after reconciliation")
	}
}
