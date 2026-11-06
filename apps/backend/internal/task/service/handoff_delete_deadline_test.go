package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	"github.com/stretchr/testify/require"
)

type heldArchiveDeleteRepo struct {
	*fakeDeleteRepo
	entered chan struct{}
	release chan struct{}
}

func (r *heldArchiveDeleteRepo) ArchiveTaskIfActiveWithVacatedStep(ctx context.Context, id, cascadeID string) (string, bool, error) {
	step, changed, err := r.fakeDeleteRepo.ArchiveTaskIfActiveWithVacatedStep(ctx, id, cascadeID)
	close(r.entered)
	<-r.release
	return step, changed, err
}

// @covers AC-TASKS-RUNTIME-CLEANUP-001.42
func TestDeleteTaskTreeLockDeadlineAndRetry(t *testing.T) {
	tasks := newFakeTaskRepo()
	tasks.addTask("root", "", "ws-1")
	repo := &heldArchiveDeleteRepo{
		fakeDeleteRepo: &fakeDeleteRepo{fakeCascadeRepo: newCascadeRepo(tasks)},
		entered:        make(chan struct{}), release: make(chan struct{}),
	}
	svc := NewHandoffService(repo, nil, nil, nil, nil, nil)
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(repo.release) }) }
	t.Cleanup(release)
	archiveDone := make(chan error, 1)
	go func() {
		_, err := svc.ArchiveTaskTree(context.Background(), "root", false)
		archiveDone <- err
	}()
	<-repo.entered
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	deleteDone := make(chan error, 1)
	go func() {
		_, err := svc.DeleteTaskTree(ctx, "root", false)
		deleteDone <- err
	}()
	<-ctx.Done()
	select {
	case err := <-deleteDone:
		require.ErrorIs(t, err, context.DeadlineExceeded)
	case <-time.After(time.Second):
		release()
		<-archiveDone
		<-deleteDone
		t.Fatal("delete waiter ignored its deadline while the root cascade remained locked")
	}
	release()
	require.NoError(t, <-archiveDone)
	task, err := tasks.GetTask(context.Background(), "root")
	require.NoError(t, err)
	require.NotNil(t, task, "the expired delete must never run after the root lock becomes available")
	_, err = svc.DeleteTaskTree(context.Background(), "root", false)
	require.NoError(t, err)
	task, err = tasks.GetTask(context.Background(), "root")
	require.NoError(t, err)
	require.Nil(t, task)
}

type failedDeleteStop struct {
	err      error
	deadline time.Time
}

type waitForDeleteDeadlineStop struct{}

func (waitForDeleteDeadlineStop) CancelTaskExecution(context.Context, string, string, bool) error {
	return errors.New("synchronous stop is required")
}

func (waitForDeleteDeadlineStop) CancelTaskExecutionSynchronously(ctx context.Context, _, _ string, _ bool) error {
	<-ctx.Done()
	return ctx.Err()
}

type contextAwareDeleteRepo struct {
	*fakeDeleteRepo
}

func (r *contextAwareDeleteRepo) RestoreTaskParentIfUnchanged(
	ctx context.Context, childID, expectedParentID, parentID string, workspaceMode string,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return r.fakeDeleteRepo.RestoreTaskParentIfUnchanged(ctx, childID, expectedParentID, parentID, workspaceMode)
}

func TestDeleteTaskTreeStopDeadlinePreservesNoCascadeChildren(t *testing.T) {
	tasks := newFakeTaskRepo()
	tasks.addTask("root", "", "ws-1")
	tasks.addTask("child", "root", "ws-1")
	base := &fakeDeleteRepo{fakeCascadeRepo: newCascadeRepo(tasks)}
	repo := &contextAwareDeleteRepo{fakeDeleteRepo: base}
	svc := NewHandoffService(repo, nil, nil, nil, nil, nil)
	svc.SetRunCanceller(waitForDeleteDeadlineStop{})
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := svc.DeleteTaskTree(ctx, "root", false)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	child, getErr := tasks.GetTask(context.Background(), "child")
	require.NoError(t, getErr)
	require.Equal(t, "root", child.ParentID,
		"the parent link must remain untouched when strict shutdown reaches the delete deadline")
}

func (s *failedDeleteStop) CancelTaskExecution(context.Context, string, string, bool) error {
	return errors.New("synchronous stop is required")
}

func (s *failedDeleteStop) CancelTaskExecutionSynchronously(ctx context.Context, _ string, _ string, _ bool) error {
	s.deadline, _ = ctx.Deadline()
	return s.err
}

type deadlineCleanupCoordinator struct {
	*recordingCleanupCoordinator
	cancellationDeadline time.Time
	deleteOptions        []DeleteTaskOptions
}

func (c *deadlineCleanupCoordinator) PrepareTaskResourceCleanupWithDeleteOptions(ctx context.Context, taskID string, trigger models.TaskResourceCleanupTrigger, operationID string, deleteEnvironmentRow bool, options DeleteTaskOptions) error {
	c.deleteOptions = append(c.deleteOptions, options)
	return c.PrepareTaskResourceCleanup(ctx, taskID, trigger, operationID, deleteEnvironmentRow)
}

func (*deadlineCleanupCoordinator) ValidateTaskDeleteWorktreesWithOptions(context.Context, []string, DeleteTaskOptions) error {
	return nil
}

func (c *deadlineCleanupCoordinator) CancelPreparedTaskResourceCleanupWithinDeadline(ctx context.Context, operationID string) error {
	c.cancellationDeadline, _ = ctx.Deadline()
	return c.CancelPreparedTaskResourceCleanup(ctx, operationID)
}

// @covers AC-TASKS-RUNTIME-CLEANUP-001.1, AC-TASKS-RUNTIME-CLEANUP-001.6, AC-TASKS-RUNTIME-CLEANUP-001.25
func TestDeleteTaskTreeStopFailurePreservesPreparedIntent(t *testing.T) {
	tasks := newFakeTaskRepo()
	tasks.addTask("root", "", "ws-1")
	coordinator := &deadlineCleanupCoordinator{recordingCleanupCoordinator: &recordingCleanupCoordinator{}}
	stop := &failedDeleteStop{err: context.DeadlineExceeded}
	svc := NewHandoffService(&fakeDeleteRepo{fakeCascadeRepo: newCascadeRepo(tasks)}, nil, nil, nil, nil, nil)
	svc.SetTaskResourceCleaner(coordinator)
	svc.SetRunCanceller(stop)

	out, err := svc.DeleteTaskTree(context.Background(), "root", false)
	require.ErrorIs(t, err, context.DeadlineExceeded, "a failed stop cannot become a successful task deletion")
	require.Empty(t, out.ArchivedTaskIDs)
	task, err := tasks.GetTask(context.Background(), "root")
	require.NoError(t, err)
	require.NotNil(t, task)
	coordinator.mu.Lock()
	require.Len(t, coordinator.prepared, 1)
	require.Equal(t, coordinator.prepared, coordinator.cancelled, "only this uncommitted preparation must be cancelled")
	require.Empty(t, coordinator.started, "physical cleanup must remain unauthorized")
	coordinator.mu.Unlock()
	require.False(t, coordinator.cancellationDeadline.IsZero(), "production compensation must use the bounded coordinator capability")
	require.Equal(t, stop.deadline, coordinator.cancellationDeadline, "compensation must retain the same absolute work deadline")

	stop.err = nil
	out, err = svc.DeleteTaskTree(context.Background(), "root", false)
	require.NoError(t, err)
	require.Equal(t, []string{"root"}, out.ArchivedTaskIDs)
}

func TestDeleteTaskRecoveryModesRetainUnconfirmedShutdown(t *testing.T) {
	for _, options := range []DeleteTaskOptions{
		{WorkspaceCleanup: models.WorkspaceCleanupRetain},
		{AllowUnverifiedWorkspaceChanges: true, DiscardWorktreeChanges: true},
	} {
		t.Run(string(options.WorkspaceCleanup), func(t *testing.T) {
			tasks := newFakeTaskRepo()
			tasks.addTask("root", "", "ws-1")
			coordinator := &deadlineCleanupCoordinator{recordingCleanupCoordinator: &recordingCleanupCoordinator{}}
			svc := NewHandoffService(&fakeDeleteRepo{fakeCascadeRepo: newCascadeRepo(tasks)}, nil, nil, nil, nil, nil)
			svc.SetTaskResourceCleaner(coordinator)
			svc.SetRunCanceller(&failedDeleteStop{err: errors.New("runtime shutdown is unconfirmed")})

			out, err := svc.DeleteTaskTreeWithOptions(context.Background(), "root", false, options)
			require.NoError(t, err, "explicit recovery modes retain their committed-deletion and pending-shutdown contract")
			require.Equal(t, []string{"root"}, out.ArchivedTaskIDs)
			require.Equal(t, []DeleteTaskOptions{options}, coordinator.deleteOptions)
			require.Equal(t, coordinator.prepared, coordinator.started, "shutdown obligations must remain runnable after deletion")
			require.Empty(t, coordinator.cancelled)
		})
	}
}

// @covers AC-TASKS-RUNTIME-CLEANUP-001.6, AC-TASKS-RUNTIME-CLEANUP-001.25
func TestDeleteTaskTreePartialFailureRetainsCommittedCleanup(t *testing.T) {
	tasks := newFakeTaskRepo()
	tasks.addTask("root", "", "ws-1")
	tasks.addTask("child", "root", "ws-1")
	mutationErr := errors.New("root deletion failed")
	repo := &partialFailureCascadeRepo{
		fakeDeleteRepo: &fakeDeleteRepo{fakeCascadeRepo: newCascadeRepo(tasks)},
		failTaskID:     "root",
		err:            mutationErr,
	}
	coordinator := &recordingCleanupCoordinator{}
	svc := NewHandoffService(repo, nil, nil, nil, nil, nil)
	svc.SetTaskResourceCleaner(coordinator)

	out, err := svc.DeleteTaskTree(context.Background(), "root", true)
	require.ErrorIs(t, err, mutationErr)
	require.Equal(t, []string{"child"}, out.ArchivedTaskIDs)
	root, err := tasks.GetTask(context.Background(), "root")
	require.NoError(t, err)
	require.NotNil(t, root)
	child, err := tasks.GetTask(context.Background(), "child")
	require.NoError(t, err)
	require.Nil(t, child)
	operationPrefix := string(models.TaskResourceCleanupTriggerCascadeDelete) + ":" + out.CascadeID + ":"
	require.ElementsMatch(t, []string{operationPrefix + "root", operationPrefix + "child"}, coordinator.prepared)
	require.Equal(t, []string{operationPrefix + "child"}, coordinator.started,
		"committed child cleanup must remain runnable")
	require.Equal(t, []string{operationPrefix + "root"}, coordinator.cancelled,
		"compensation must cancel only the uncommitted root preparation")
}

type deadlineCleanupRepository struct {
	repository.TaskResourceCleanupRepository
}

func (*deadlineCleanupRepository) GetTaskResourceCleanupJobByOperationID(ctx context.Context, _ string) (*models.TaskResourceCleanupJob, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

// @covers AC-TASKS-RUNTIME-CLEANUP-001.42
func TestCancelPreparedCleanupPreservesOriginalDeadline(t *testing.T) {
	for _, budget := range []time.Duration{time.Second, 10 * time.Second} {
		t.Run(budget.String(), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				svc := &Service{resourceCleanups: &deadlineCleanupRepository{}}
				ctx, cancel := context.WithTimeout(context.Background(), budget)
				defer cancel()
				start := time.Now()
				err := svc.CancelPreparedTaskResourceCleanupWithinDeadline(ctx, "prepared-operation")
				require.ErrorIs(t, err, context.DeadlineExceeded)
				require.Equal(t, min(budget, 5*time.Second), time.Since(start),
					"compensation must preserve the earlier caller deadline and the existing transition cap")
			})
		})
	}
}
