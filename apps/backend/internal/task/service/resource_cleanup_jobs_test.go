package service

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/activity"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	"github.com/kandev/kandev/internal/worktree"
)

type cancellableCleanupBarrier struct {
	started chan struct{}
	stopped chan struct{}
	release chan struct{}
	once    sync.Once
}

type joinCleanupBarrier struct {
	started   chan struct{}
	cancelled chan struct{}
	release   chan struct{}
	stopped   chan struct{}
}

func newJoinCleanupBarrier() *joinCleanupBarrier {
	return &joinCleanupBarrier{
		started: make(chan struct{}), cancelled: make(chan struct{}),
		release: make(chan struct{}), stopped: make(chan struct{}),
	}
}

func (b *joinCleanupBarrier) OnTaskDeleted(context.Context, string) error { return nil }
func (b *joinCleanupBarrier) GetAllByTaskID(context.Context, string) ([]*worktree.Worktree, error) {
	return nil, nil
}
func (b *joinCleanupBarrier) CleanupWorktrees(ctx context.Context, _ []*worktree.Worktree) error {
	close(b.started)
	<-ctx.Done()
	close(b.cancelled)
	<-b.release
	close(b.stopped)
	return ctx.Err()
}

type recordingLegacyCleanup struct {
	calls int
}

func (c *recordingLegacyCleanup) OnTaskDeleted(context.Context, string) error {
	c.calls++
	return nil
}

type activityCleanupStopper struct {
	coordinator *activity.Coordinator
}

type coordinatorCleanupActivityGate struct {
	coordinator *activity.Coordinator
}

func (g coordinatorCleanupActivityGate) AcquireTaskResourceCleanup(
	ctx context.Context,
) (TaskResourceCleanupActivityLease, error) {
	return g.coordinator.AcquireTask(ctx, activity.KindCleanupScript)
}

func (s *activityCleanupStopper) StopTask(context.Context, string, string, bool) error { return nil }
func (s *activityCleanupStopper) RegisterExecutionStopOwner(string, string, bool)      {}
func (s *activityCleanupStopper) StopSession(context.Context, string, string, bool) error {
	return nil
}
func (s *activityCleanupStopper) StopExecution(ctx context.Context, _ string, _ string, _ bool) error {
	lease, err := s.coordinator.AcquireTask(ctx, activity.KindExecutionStopping)
	if err != nil {
		return err
	}
	lease.Release()
	return nil
}

type activityCleanupBarrier struct {
	coordinator    *activity.Coordinator
	started        chan struct{}
	release        chan struct{}
	maintenanceErr chan error
}

type blockingResumeCleanupRepository struct {
	repository.TaskResourceCleanupRepository
	entered chan struct{}
	release chan struct{}
}

func (r *blockingResumeCleanupRepository) ResetRunningTaskResourceCleanupJobs(context.Context) error {
	close(r.entered)
	<-r.release
	return nil
}

func (b *activityCleanupBarrier) OnTaskDeleted(context.Context, string) error { return nil }
func (b *activityCleanupBarrier) GetAllByTaskID(context.Context, string) ([]*worktree.Worktree, error) {
	return nil, nil
}
func (b *activityCleanupBarrier) CleanupWorktrees(ctx context.Context, _ []*worktree.Worktree) error {
	close(b.started)
	lease, _, err := b.coordinator.TryAcquireMaintenance(context.Background(), 0)
	if lease != nil {
		lease.Release()
	}
	b.maintenanceErr <- err
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-b.release:
		return nil
	}
}

func TestClaimedCleanupDrainsMaintenanceAndHoldsActivityThroughDestructiveWork(t *testing.T) {
	taskSvc, repo := setupOfficeTest(t)
	coordinator := activity.NewCoordinator(activity.Options{})
	maintenance, _, err := coordinator.TryAcquireMaintenance(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	barrier := &activityCleanupBarrier{
		coordinator: coordinator, started: make(chan struct{}), release: make(chan struct{}),
		maintenanceErr: make(chan error, 1),
	}
	taskSvc.SetExecutionStopper(&activityCleanupStopper{coordinator: coordinator})
	taskSvc.SetTaskResourceCleanupActivityGate(coordinatorCleanupActivityGate{coordinator: coordinator})
	taskSvc.SetWorktreeCleanup(barrier)
	snapshot, err := json.Marshal(taskResourceCleanupSnapshot{
		StopTargets: []persistedTaskStopTarget{{SessionID: "session-1", ExecutionID: "execution-1"}},
		Worktrees:   []*worktree.Worktree{{ID: "worktree-1", TaskID: "task-1"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	job := &models.TaskResourceCleanupJob{
		ID: "activity-cleanup", OperationID: "delete:activity-cleanup", TaskID: "task-1",
		Trigger: models.TaskResourceCleanupTriggerDelete,
		State:   models.TaskResourceCleanupStatePending, ResourceSnapshot: string(snapshot),
	}
	if err := repo.CreateTaskResourceCleanupJob(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	processDone := make(chan error, 1)
	go func() { processDone <- taskSvc.processTaskResourceCleanupJob(context.Background(), job.ID) }()
	select {
	case <-maintenance.Context().Done():
	case <-time.After(time.Second):
		t.Fatal("claimed cleanup did not cancel active maintenance")
	}
	select {
	case <-barrier.started:
		t.Fatal("destructive cleanup started before maintenance drained")
	default:
	}
	maintenance.Release()
	select {
	case <-barrier.started:
	case <-time.After(time.Second):
		t.Fatal("destructive cleanup did not start after maintenance drained")
	}
	if err := <-barrier.maintenanceErr; !errors.Is(err, activity.ErrBusy) {
		t.Fatalf("maintenance during destructive cleanup error = %v, want ErrBusy", err)
	}
	close(barrier.release)
	if err := <-processDone; err != nil {
		t.Fatalf("processTaskResourceCleanupJob: %v", err)
	}
	lease, _, err := coordinator.TryAcquireMaintenance(context.Background(), 0)
	if err != nil {
		t.Fatalf("cleanup activity lease was not released: %v", err)
	}
	lease.Release()
}

func newCancellableCleanupBarrier() *cancellableCleanupBarrier {
	return &cancellableCleanupBarrier{
		started: make(chan struct{}),
		stopped: make(chan struct{}),
		release: make(chan struct{}),
	}
}

func (c *cancellableCleanupBarrier) OnTaskDeleted(context.Context, string) error { return nil }

func (c *cancellableCleanupBarrier) GetAllByTaskID(context.Context, string) ([]*worktree.Worktree, error) {
	return nil, nil
}

func (c *cancellableCleanupBarrier) CleanupWorktrees(ctx context.Context, _ []*worktree.Worktree) error {
	c.once.Do(func() { close(c.started) })
	defer close(c.stopped)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-c.release:
		return nil
	}
}

func TestUnarchiveCancelsAndJoinsClaimedArchiveCleanup(t *testing.T) {
	taskSvc, repo := setupOfficeTest(t)
	ctx := context.Background()
	coordinator := activity.NewCoordinator(activity.Options{})
	taskSvc.SetTaskResourceCleanupActivityGate(coordinatorCleanupActivityGate{coordinator: coordinator})
	task, err := taskSvc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-1", Title: "Archived task", ProjectID: "proj-1",
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if err := repo.ArchiveTask(ctx, task.ID); err != nil {
		t.Fatalf("ArchiveTask: %v", err)
	}
	snapshot, err := json.Marshal(taskResourceCleanupSnapshot{
		Worktrees: []*worktree.Worktree{{ID: "worktree-claimed", TaskID: task.ID}},
	})
	if err != nil {
		t.Fatalf("marshal cleanup snapshot: %v", err)
	}
	job := &models.TaskResourceCleanupJob{
		ID: "archive-job-claimed", OperationID: "archive:" + task.ID,
		TaskID: task.ID, Trigger: models.TaskResourceCleanupTriggerArchive,
		State: models.TaskResourceCleanupStatePending, ResourceSnapshot: string(snapshot),
	}
	if err := repo.CreateTaskResourceCleanupJob(ctx, job); err != nil {
		t.Fatalf("CreateTaskResourceCleanupJob: %v", err)
	}
	barrier := newJoinCleanupBarrier()
	taskSvc.SetWorktreeCleanup(barrier)
	processDone := make(chan error, 1)
	go func() { processDone <- taskSvc.processTaskResourceCleanupJob(ctx, job.ID) }()
	select {
	case <-barrier.started:
	case <-time.After(time.Second):
		t.Fatal("cleanup did not reach the post-claim barrier")
	}

	handoff := NewHandoffService(repo, repo, nil, nil, nil, nil)
	handoff.SetTaskResourceCleaner(taskSvc)
	type unarchiveResult struct {
		outcome *CascadeOutcome
		err     error
	}
	unarchiveDone := make(chan unarchiveResult, 1)
	go func() {
		outcome, err := handoff.UnarchiveTaskTree(ctx, task.ID)
		unarchiveDone <- unarchiveResult{outcome: outcome, err: err}
	}()
	select {
	case <-barrier.cancelled:
	case <-time.After(time.Second):
		t.Fatal("cleanup did not observe unarchive cancellation")
	}
	select {
	case result := <-unarchiveDone:
		t.Fatalf("unarchive returned before claimed cleanup joined: outcome=%#v err=%v", result.outcome, result.err)
	case <-time.After(100 * time.Millisecond):
	}
	close(barrier.release)
	select {
	case <-barrier.stopped:
	case <-time.After(time.Second):
		t.Fatal("cleanup did not stop after release")
	}
	var unarchiveErr error
	select {
	case result := <-unarchiveDone:
		unarchiveErr = result.err
	case <-time.After(time.Second):
		t.Fatal("unarchive did not return after cleanup joined")
	}
	if unarchiveErr != nil {
		t.Fatalf("UnarchiveTaskTree: %v", unarchiveErr)
	}
	var processErr error
	select {
	case processErr = <-processDone:
	case <-time.After(time.Second):
		t.Fatal("cleanup processor did not return after release")
	}
	if processErr != nil && !errors.Is(processErr, context.Canceled) {
		t.Fatalf("cleanup worker error = %v, want nil or context cancellation", processErr)
	}
	got, err := repo.GetTaskResourceCleanupJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("GetTaskResourceCleanupJob: %v", err)
	}
	if got.State != models.TaskResourceCleanupStateCancelled {
		t.Fatalf("cleanup state = %q, want cancelled", got.State)
	}
	if got.Attempts != 1 {
		t.Fatalf("cleanup attempts = %d, want claimed generation 1 preserved", got.Attempts)
	}
	lease, _, err := coordinator.TryAcquireMaintenance(ctx, 0)
	if err != nil {
		t.Fatalf("cancelled cleanup activity lease was not released: %v", err)
	}
	lease.Release()
}

func TestUnarchiveCancellationPreservesCleanupResourcesAfterBlockedCleaner(t *testing.T) {
	taskSvc, repo := setupOfficeTest(t)
	ctx := context.Background()
	task, err := taskSvc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-1", Title: "Archived task", ProjectID: "proj-1",
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	const sessionID = "session-cancel-boundary"
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: sessionID, TaskID: task.ID, State: models.TaskSessionStateCompleted,
	}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: sessionID, SessionID: sessionID, TaskID: task.ID, ExecutorID: "executor-1",
		Runtime: agentruntime.RuntimeStandalone, Status: models.ExecutorRunningStatusStarting,
	}); err != nil {
		t.Fatalf("UpsertExecutorRunning: %v", err)
	}
	quickChatRoot := t.TempDir()
	taskSvc.SetQuickChatDir(quickChatRoot)
	sessionDir := filepath.Join(quickChatRoot, sessionID)
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatalf("create quick-chat session directory: %v", err)
	}
	if err := repo.ArchiveTask(ctx, task.ID); err != nil {
		t.Fatalf("ArchiveTask: %v", err)
	}
	snapshot, err := json.Marshal(taskResourceCleanupSnapshot{
		Sessions:  []*models.TaskSession{{ID: sessionID, TaskID: task.ID}},
		Worktrees: []*worktree.Worktree{{ID: "worktree-cancel-boundary", TaskID: task.ID}},
	})
	if err != nil {
		t.Fatalf("marshal cleanup snapshot: %v", err)
	}
	job := &models.TaskResourceCleanupJob{
		ID: "archive-job-cancel-boundary", OperationID: "archive:" + task.ID,
		TaskID: task.ID, Trigger: models.TaskResourceCleanupTriggerArchive,
		State: models.TaskResourceCleanupStatePending, ResourceSnapshot: string(snapshot),
	}
	if err := repo.CreateTaskResourceCleanupJob(ctx, job); err != nil {
		t.Fatalf("CreateTaskResourceCleanupJob: %v", err)
	}
	barrier := newCancellableCleanupBarrier()
	taskSvc.SetWorktreeCleanup(barrier)
	processDone := make(chan error, 1)
	go func() { processDone <- taskSvc.processTaskResourceCleanupJob(ctx, job.ID) }()
	select {
	case <-barrier.started:
	case <-time.After(time.Second):
		t.Fatal("cleanup did not reach the blocked worktree cleaner")
	}

	handoff := NewHandoffService(repo, repo, nil, nil, nil, nil)
	handoff.SetTaskResourceCleaner(taskSvc)
	if _, err := handoff.UnarchiveTaskTree(ctx, task.ID); err != nil {
		t.Fatalf("UnarchiveTaskTree: %v", err)
	}
	if err := <-processDone; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("cleanup worker error = %v, want nil or context cancellation", err)
	}
	if _, err := os.Stat(sessionDir); err != nil {
		t.Fatalf("quick-chat session directory removed after cancellation: %v", err)
	}
	if _, err := repo.GetExecutorRunningBySessionID(ctx, sessionID); err != nil {
		t.Fatalf("executor runtime row removed after cancellation: %v", err)
	}
	got, err := repo.GetTaskResourceCleanupJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("GetTaskResourceCleanupJob: %v", err)
	}
	if got.State != models.TaskResourceCleanupStateCancelled {
		t.Fatalf("cleanup state = %q, want cancelled", got.State)
	}
}

func TestExecuteTaskResourceCleanupJob_CancellationSkipsLegacyCleanup(t *testing.T) {
	taskSvc, _, _ := createTestService(t)
	legacy := &recordingLegacyCleanup{}
	taskSvc.SetWorktreeCleanup(legacy)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := taskSvc.executeTaskResourceCleanupJob(ctx, &models.TaskResourceCleanupJob{
		ID: "delete-job-cancelled", TaskID: "task-cancelled",
		Trigger: models.TaskResourceCleanupTriggerDelete,
	}, taskResourceCleanupSnapshot{LegacyWorktreeCleanup: true})

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cleanup error = %v, want context cancellation", err)
	}
	if legacy.calls != 0 {
		t.Fatalf("legacy cleanup calls after cancellation = %d, want 0", legacy.calls)
	}
}

func TestPerformTaskCleanup_CancellationStopsBeforeWorktreeCleanup(t *testing.T) {
	taskSvc, _, _ := createTestService(t)
	ctx, cancel := context.WithCancel(context.Background())
	taskSvc.SetEnvironmentDestroyer(&stubDestroyer{cancelAfterContainer: cancel})
	barrier := newCancellableCleanupBarrier()
	taskSvc.SetWorktreeCleanup(barrier)

	errs := taskSvc.performTaskCleanup(ctx, "task-cancelled", nil,
		[]*worktree.Worktree{{ID: "worktree-after-environment", TaskID: "task-cancelled"}}, nil,
		taskEnvironmentCleanup{env: &models.TaskEnvironment{ContainerID: "container-1"}}, nil)

	if !errors.Is(errors.Join(errs...), context.Canceled) {
		t.Fatalf("cleanup errors = %v, want context cancellation", errs)
	}
	select {
	case <-barrier.started:
		t.Fatal("worktree cleanup started after environment teardown cancelled")
	default:
	}
}

func TestUnarchiveTaskTreeCancelsPendingArchiveCleanup(t *testing.T) {
	taskSvc, repo := setupOfficeTest(t)
	ctx := context.Background()
	task, err := taskSvc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-1", Title: "Archived task", ProjectID: "proj-1",
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if err := repo.ArchiveTask(ctx, task.ID); err != nil {
		t.Fatalf("ArchiveTask: %v", err)
	}
	job := &models.TaskResourceCleanupJob{
		ID: "archive-job", OperationID: "archive:" + task.ID, TaskID: task.ID,
		Trigger: models.TaskResourceCleanupTriggerArchive,
		State:   models.TaskResourceCleanupStatePending, ResourceSnapshot: `{}`,
	}
	if err := repo.CreateTaskResourceCleanupJob(ctx, job); err != nil {
		t.Fatalf("CreateTaskResourceCleanupJob: %v", err)
	}

	handoff := NewHandoffService(repo, repo, nil, nil, nil, nil)
	handoff.SetTaskResourceCleaner(taskSvc)
	if _, err := handoff.UnarchiveTaskTree(ctx, task.ID); err != nil {
		t.Fatalf("UnarchiveTaskTree: %v", err)
	}
	got, err := repo.GetTaskResourceCleanupJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("GetTaskResourceCleanupJob: %v", err)
	}
	if got.State != models.TaskResourceCleanupStateCancelled {
		t.Fatalf("cleanup state = %q, want cancelled", got.State)
	}
}

func TestResumeTaskResourceCleanupJobsReconstructsInterruptedJob(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	job := &models.TaskResourceCleanupJob{
		ID: "interrupted-job", OperationID: "delete:interrupted", TaskID: "deleted-task",
		Trigger: models.TaskResourceCleanupTriggerDelete,
		State:   models.TaskResourceCleanupStateRunning, ResourceSnapshot: `{}`,
	}
	if err := repo.CreateTaskResourceCleanupJob(ctx, job); err != nil {
		t.Fatalf("CreateTaskResourceCleanupJob: %v", err)
	}
	if err := svc.ResumeTaskResourceCleanupJobs(ctx); err != nil {
		t.Fatalf("ResumeTaskResourceCleanupJobs: %v", err)
	}
	got, err := repo.GetTaskResourceCleanupJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("GetTaskResourceCleanupJob: %v", err)
	}
	if got.State != models.TaskResourceCleanupStateSucceeded || got.Attempts != 1 {
		t.Fatalf("reconstructed job = state %q attempts %d, want succeeded/1", got.State, got.Attempts)
	}
}

func TestDeleteTaskCleanupSnapshotSurvivesSessionCascade(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	seedCleanupTaskAndSession(t, repo, "task-delete-snapshot", "session-delete-snapshot")
	if err := svc.DeleteTask(ctx, "task-delete-snapshot"); err != nil {
		t.Fatalf("DeleteTask: %v", err)
	}
	var encoded string
	if err := repo.DB().QueryRowContext(ctx, `
		SELECT resource_snapshot FROM task_resource_cleanup_jobs
		WHERE task_id = ? AND trigger = 'delete'
	`, "task-delete-snapshot").Scan(&encoded); err != nil {
		t.Fatalf("load delete snapshot: %v", err)
	}
	var snapshot taskResourceCleanupSnapshot
	if err := json.Unmarshal([]byte(encoded), &snapshot); err != nil {
		t.Fatalf("decode snapshot: %v", err)
	}
	if len(snapshot.Sessions) != 1 || snapshot.Sessions[0].ID != "session-delete-snapshot" {
		t.Fatalf("snapshot sessions = %#v, want deleted session handle", snapshot.Sessions)
	}
}

func TestArchiveCleanupPreservesHistoricalWorktreeBranchMetadata(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	seedCleanupTaskAndSession(t, repo, "task-history", "session-history")
	if err := repo.CreateTaskSessionWorktree(ctx, &models.TaskSessionWorktree{
		ID: "session-worktree-history", SessionID: "session-history", WorktreeID: "worktree-history",
		RepositoryID: "repo-history", WorktreePath: "/tmp/history", WorktreeBranch: "feature/history",
	}); err != nil {
		t.Fatalf("CreateTaskSessionWorktree: %v", err)
	}
	svc.setCleanupDoneForTestHook(make(chan struct{}, 1))
	if err := svc.ArchiveTask(ctx, "task-history"); err != nil {
		t.Fatalf("ArchiveTask: %v", err)
	}
	waitForCleanupDone(t, svc)
	rows, err := repo.ListTaskSessionWorktrees(ctx, "session-history")
	if err != nil {
		t.Fatalf("ListTaskSessionWorktrees: %v", err)
	}
	if len(rows) != 1 || rows[0].WorktreeBranch != "feature/history" {
		t.Fatalf("historical worktrees = %#v, want preserved branch metadata", rows)
	}
}

func TestArchiveFailsBeforeMutationWhenCleanupIntentCannotPersist(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	seedCleanupTaskAndSession(t, repo, "task-persist-fail", "session-persist-fail")
	if _, err := repo.DB().Exec(`
		CREATE TRIGGER reject_cleanup_intent BEFORE INSERT ON task_resource_cleanup_jobs
		BEGIN SELECT RAISE(ABORT, 'cleanup persistence unavailable'); END
	`); err != nil {
		t.Fatalf("create trigger: %v", err)
	}
	if err := svc.ArchiveTask(ctx, "task-persist-fail"); err == nil {
		t.Fatal("ArchiveTask succeeded without durable cleanup intent")
	}
	task, err := repo.GetTask(ctx, "task-persist-fail")
	if err != nil || task.ArchivedAt != nil {
		t.Fatalf("task mutated after cleanup persistence failure: task=%#v err=%v", task, err)
	}
}

func TestCascadeArchivePersistsCleanupBeforeTaskMutation(t *testing.T) {
	taskSvc, repo := setupOfficeTest(t)
	ctx := context.Background()
	task, err := taskSvc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-1", Title: "Cascade archive", ProjectID: "proj-1",
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if _, err := repo.DB().Exec(`
		CREATE TRIGGER require_cascade_archive_cleanup
		BEFORE UPDATE OF archived_at ON tasks
		WHEN NEW.archived_at IS NOT NULL AND NOT EXISTS (
			SELECT 1 FROM task_resource_cleanup_jobs
			WHERE task_id = NEW.id AND trigger = 'cascade_archive'
		)
		BEGIN SELECT RAISE(ABORT, 'missing cascade archive cleanup'); END
	`); err != nil {
		t.Fatalf("create trigger: %v", err)
	}
	handoff := NewHandoffService(repo, repo, nil, nil, nil, nil)
	handoff.SetTaskResourceCleaner(taskSvc)
	if _, err := handoff.ArchiveTaskTree(ctx, task.ID, false); err != nil {
		t.Fatalf("ArchiveTaskTree: %v", err)
	}
}

func TestWorkspaceDeletePersistsCleanupBeforeTaskCascade(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	seedCleanupTaskAndSession(t, repo, "task-workspace-delete", "session-workspace-delete")
	if _, err := repo.DB().Exec(`
		CREATE TRIGGER require_workspace_delete_cleanup
		BEFORE DELETE ON tasks
		WHEN NOT EXISTS (
			SELECT 1 FROM task_resource_cleanup_jobs
			WHERE task_id = OLD.id AND trigger = 'workspace_delete'
		)
		BEGIN SELECT RAISE(ABORT, 'missing workspace delete cleanup'); END
	`); err != nil {
		t.Fatalf("create trigger: %v", err)
	}
	if err := svc.DeleteWorkspace(ctx, "ws-task-workspace-delete"); err != nil {
		t.Fatalf("DeleteWorkspace: %v", err)
	}
}

func TestCascadeDeletePersistsCleanupBeforeTaskMutation(t *testing.T) {
	taskSvc, repo := setupOfficeTest(t)
	ctx := context.Background()
	task, err := taskSvc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-1", Title: "Cascade delete", ProjectID: "proj-1",
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if _, err := repo.DB().Exec(`
		CREATE TRIGGER require_cascade_delete_cleanup
		BEFORE DELETE ON tasks
		WHEN NOT EXISTS (
			SELECT 1 FROM task_resource_cleanup_jobs
			WHERE task_id = OLD.id AND trigger = 'cascade_delete'
		)
		BEGIN SELECT RAISE(ABORT, 'missing cascade delete cleanup'); END
	`); err != nil {
		t.Fatalf("create trigger: %v", err)
	}
	handoff := NewHandoffService(repo, repo, nil, nil, nil, nil)
	handoff.SetTaskResourceCleaner(taskSvc)
	if _, err := handoff.DeleteTaskTree(ctx, task.ID, false); err != nil {
		t.Fatalf("DeleteTaskTree: %v", err)
	}
}

func TestPreparedCleanupIsNotRunnableUntilStarted(t *testing.T) {
	taskSvc, repo := setupOfficeTest(t)
	ctx := context.Background()
	seedCleanupTaskAndSession(t, repo, "task-prepared", "session-prepared")
	const operationID = "cascade_delete:cascade-1:task-prepared"

	if err := taskSvc.PrepareTaskResourceCleanup(ctx, "task-prepared",
		models.TaskResourceCleanupTriggerCascadeDelete, operationID, true); err != nil {
		t.Fatalf("PrepareTaskResourceCleanup: %v", err)
	}
	prepared, err := repo.GetTaskResourceCleanupJobByOperationID(ctx, operationID)
	if err != nil {
		t.Fatalf("GetTaskResourceCleanupJobByOperationID: %v", err)
	}
	if prepared.State != models.TaskResourceCleanupState("prepared") {
		t.Fatalf("prepared state = %q, want prepared", prepared.State)
	}
	due, err := repo.ListDueTaskResourceCleanupJobs(ctx, time.Now().UTC(), 10)
	if err != nil {
		t.Fatalf("ListDueTaskResourceCleanupJobs: %v", err)
	}
	if len(due) != 0 {
		t.Fatalf("prepared cleanup was runnable: %#v", due)
	}

	taskSvc.cleanupWorkerMu.Lock()
	taskSvc.cleanupWorkerWake = make(chan struct{}, 1)
	taskSvc.cleanupWorkerMu.Unlock()
	if err := taskSvc.StartPreparedTaskResourceCleanup(ctx, operationID); err != nil {
		t.Fatalf("StartPreparedTaskResourceCleanup: %v", err)
	}
	started, err := repo.GetTaskResourceCleanupJobByOperationID(ctx, operationID)
	if err != nil {
		t.Fatalf("reload started cleanup: %v", err)
	}
	if started.State != models.TaskResourceCleanupStatePending {
		t.Fatalf("started state = %q, want pending", started.State)
	}
}

func TestRetryTaskResourceCleanupJobPersistsAfterRunContextCancellation(t *testing.T) {
	taskSvc, repo := setupOfficeTest(t)
	ctx := context.Background()
	job := &models.TaskResourceCleanupJob{
		ID: "timed-out-job", OperationID: "delete:timed-out", TaskID: "task-timed-out",
		Trigger: models.TaskResourceCleanupTriggerDelete,
		State:   models.TaskResourceCleanupStatePending, ResourceSnapshot: `{}`,
	}
	if err := repo.CreateTaskResourceCleanupJob(ctx, job); err != nil {
		t.Fatalf("CreateTaskResourceCleanupJob: %v", err)
	}
	claimed, err := repo.MarkTaskResourceCleanupJobRunning(ctx, job.ID)
	if err != nil || !claimed {
		t.Fatalf("MarkTaskResourceCleanupJobRunning = %v, %v", claimed, err)
	}
	job, err = repo.GetTaskResourceCleanupJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("GetTaskResourceCleanupJob: %v", err)
	}
	runCtx, cancel := context.WithCancel(ctx)
	cancel()
	cleanupErr := errors.New("cleanup deadline exceeded")
	if err := taskSvc.retryTaskResourceCleanupJob(runCtx, job, cleanupErr); !errors.Is(err, cleanupErr) {
		t.Fatalf("retryTaskResourceCleanupJob error = %v, want cleanup error", err)
	}
	got, err := repo.GetTaskResourceCleanupJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("reload cleanup job: %v", err)
	}
	if got.State != models.TaskResourceCleanupStateRetryWait {
		t.Fatalf("cleanup state = %q, want retry_wait", got.State)
	}
}

func TestCancelWorkspaceDeleteCleanupUsesDetachedContext(t *testing.T) {
	taskSvc, repo := setupOfficeTest(t)
	ctx := context.Background()
	job := &models.TaskResourceCleanupJob{
		ID: "workspace-delete-cancel", OperationID: "workspace_delete:cancel", TaskID: "task-cancel",
		Trigger: models.TaskResourceCleanupTriggerWorkspaceDelete,
		State:   models.TaskResourceCleanupState("prepared"), ResourceSnapshot: `{}`,
	}
	if err := repo.CreateTaskResourceCleanupJob(ctx, job); err != nil {
		t.Fatalf("CreateTaskResourceCleanupJob: %v", err)
	}
	cancelledCtx, cancel := context.WithCancel(ctx)
	cancel()
	taskSvc.cancelWorkspaceDeleteTaskCleanupJobs(cancelledCtx, []workspaceDeleteTaskCleanup{{cleanupJob: job}})
	got, err := repo.GetTaskResourceCleanupJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("GetTaskResourceCleanupJob: %v", err)
	}
	if got.State != models.TaskResourceCleanupStateCancelled {
		t.Fatalf("cleanup state = %q, want cancelled", got.State)
	}
}

func TestStopTaskResourceCleanupWorkerJoinsStartupResume(t *testing.T) {
	taskSvc, _ := setupOfficeTest(t)
	taskSvc.StopTaskResourceCleanupWorker()
	blocking := &blockingResumeCleanupRepository{
		TaskResourceCleanupRepository: taskSvc.resourceCleanups,
		entered:                       make(chan struct{}), release: make(chan struct{}),
	}
	taskSvc.resourceCleanups = blocking
	startDone := make(chan error, 1)
	go func() { startDone <- taskSvc.StartTaskResourceCleanupWorker(context.Background()) }()
	<-blocking.entered
	stopDone := make(chan struct{})
	go func() {
		taskSvc.StopTaskResourceCleanupWorker()
		close(stopDone)
	}()
	select {
	case <-stopDone:
		t.Fatal("StopTaskResourceCleanupWorker returned before startup resume drained")
	case <-time.After(100 * time.Millisecond):
	}
	close(blocking.release)
	select {
	case <-stopDone:
	case <-time.After(time.Second):
		t.Fatal("StopTaskResourceCleanupWorker did not return after startup resume drained")
	}
	if err := <-startDone; err == nil {
		t.Fatal("StartTaskResourceCleanupWorker succeeded after concurrent stop")
	}
}

func TestStartTaskResourceCleanupLeavesPendingJobForOwnedWorker(t *testing.T) {
	taskSvc, repo := setupOfficeTest(t)
	taskSvc.StopTaskResourceCleanupWorker()
	ctx := context.Background()
	snapshot, err := json.Marshal(taskResourceCleanupSnapshot{
		Worktrees: []*worktree.Worktree{{ID: "worktree-owned-worker", TaskID: "task-owned-worker"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	job := &models.TaskResourceCleanupJob{
		ID: "job-owned-worker", OperationID: "delete:owned-worker", TaskID: "task-owned-worker",
		Trigger: models.TaskResourceCleanupTriggerDelete,
		State:   models.TaskResourceCleanupStatePending, ResourceSnapshot: string(snapshot),
	}
	if err := repo.CreateTaskResourceCleanupJob(ctx, job); err != nil {
		t.Fatalf("CreateTaskResourceCleanupJob: %v", err)
	}
	barrier := newCancellableCleanupBarrier()
	taskSvc.SetWorktreeCleanup(barrier)

	taskSvc.startTaskResourceCleanup(job)
	select {
	case <-barrier.started:
		close(barrier.release)
		<-barrier.stopped
		t.Fatal("cleanup ran in an unowned fallback goroutine")
	case <-time.After(100 * time.Millisecond):
	}

	startDone := make(chan error, 1)
	go func() { startDone <- taskSvc.StartTaskResourceCleanupWorker(ctx) }()
	select {
	case <-barrier.started:
	case <-time.After(time.Second):
		t.Fatal("owned cleanup worker did not process pending job")
	}
	close(barrier.release)
	select {
	case <-barrier.stopped:
	case <-time.After(time.Second):
		t.Fatal("owned cleanup worker did not finish job")
	}
	select {
	case err := <-startDone:
		if err != nil {
			t.Fatalf("StartTaskResourceCleanupWorker: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("StartTaskResourceCleanupWorker did not return")
	}
	taskSvc.StopTaskResourceCleanupWorker()
}

func seedCleanupTaskAndSession(t *testing.T, repo interface {
	CreateWorkspace(context.Context, *models.Workspace) error
	CreateWorkflow(context.Context, *models.Workflow) error
	CreateTask(context.Context, *models.Task) error
	CreateTaskSession(context.Context, *models.TaskSession) error
}, taskID, sessionID string) {
	t.Helper()
	ctx := context.Background()
	workspaceID := "ws-" + taskID
	workflowID := "wf-" + taskID
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: workspaceID, Name: "Workspace"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: workflowID, WorkspaceID: workspaceID, Name: "Workflow"}); err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}
	if err := repo.CreateTask(ctx, &models.Task{ID: taskID, WorkspaceID: workspaceID, WorkflowID: workflowID, WorkflowStepID: "step", Title: "Task", Priority: "medium"}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: sessionID, TaskID: taskID, State: models.TaskSessionStateCompleted}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}
}
