package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/worktree"
)

type manifestBoundaryStopper struct {
	stopped bool
}

func (s *manifestBoundaryStopper) StopTask(context.Context, string, string, bool) error { return nil }
func (s *manifestBoundaryStopper) StopSession(context.Context, string, string, bool) error {
	return nil
}
func (s *manifestBoundaryStopper) StopExecution(context.Context, string, string, bool) error {
	s.stopped = true
	return nil
}
func (*manifestBoundaryStopper) RegisterExecutionStopOwner(string, string, bool) {}

type manifestBoundaryCleanup struct {
	WorktreeCleanup
	repo interface {
		GetTaskResourceCleanupJob(context.Context, string) (*models.TaskResourceCleanupJob, error)
	}
	stopper           *manifestBoundaryStopper
	captureErr        error
	captureCount      int
	captureStarted    chan struct{}
	waitCaptureCancel bool
	cleanupErr        error
	cleanupCalled     bool
}

func TestCaptureManifestDefersOnRuntimeStopFailureWithStableReason(t *testing.T) {
	svc, _ := setupOfficeTest(t)
	snapshot := &taskResourceCleanupSnapshot{}
	err := svc.captureAndPersistTaskSourceManifest(
		context.Background(),
		&models.TaskResourceCleanupJob{Trigger: models.TaskResourceCleanupTriggerDelete},
		snapshot,
		1,
	)
	if err == nil || !strings.Contains(err.Error(), "runtime stop operations failed") {
		t.Fatalf("capture error = %v, want stable runtime stop failure description", err)
	}
	if snapshot.ArchiveSourceManifestCaptured {
		t.Fatal("manifest marked captured after an incomplete runtime stop")
	}
}

func (c *manifestBoundaryCleanup) GetAllByTaskID(context.Context, string) ([]*worktree.Worktree, error) {
	return nil, nil
}

func (c *manifestBoundaryCleanup) CaptureArchiveSourceManifests(
	ctx context.Context, worktrees []*worktree.Worktree,
) (map[string]worktree.ArchiveSourceManifest, error) {
	c.captureCount++
	if !c.stopper.stopped {
		return nil, errors.New("capture ran before runtime stop")
	}
	if c.waitCaptureCancel {
		close(c.captureStarted)
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if c.captureErr != nil {
		return nil, c.captureErr
	}
	result := make(map[string]worktree.ArchiveSourceManifest, len(worktrees))
	for _, wt := range worktrees {
		result[wt.ID] = worktree.ArchiveSourceManifest{
			TaskID: wt.TaskID, WorktreeID: wt.ID, RepositoryID: wt.RepositoryID,
			TaskEnvironmentID: wt.TaskEnvironmentID, HeadOID: "head", IndexStateSHA256: "index",
		}
	}
	return result, nil
}

func (c *manifestBoundaryCleanup) CleanupWorktrees(context.Context, []*worktree.Worktree) error {
	c.cleanupCalled = true
	job, err := c.repo.GetTaskResourceCleanupJob(context.Background(), "manifest-boundary-job")
	if err != nil {
		return err
	}
	var snapshot taskResourceCleanupSnapshot
	if err := json.Unmarshal([]byte(job.ResourceSnapshot), &snapshot); err != nil {
		return err
	}
	if !snapshot.ArchiveSourceManifestCaptured || len(snapshot.ArchiveSourceManifest) != 1 {
		return errors.New("cleanup began before source manifest was durably persisted")
	}
	return c.cleanupErr
}

// @covers AC-TASKS-ARCHIVE-SOURCE-MANIFEST-001.1
func TestCleanupRetryReusesPersistedSourceManifest(t *testing.T) {
	ctx := context.Background()
	svc, repo := setupOfficeTest(t)
	svc.StopTaskResourceCleanupWorker()
	stopper := &manifestBoundaryStopper{}
	cleanup := &manifestBoundaryCleanup{repo: repo, stopper: stopper, cleanupErr: errors.New("partial cleanup")}
	svc.SetWorktreeCleanup(cleanup)
	svc.SetExecutionStopper(stopper)
	snapshot, err := json.Marshal(taskResourceCleanupSnapshot{
		Worktrees:   []*worktree.Worktree{{ID: "wt", TaskID: "task", RepositoryID: "repo"}},
		StopTargets: []persistedTaskStopTarget{{SessionID: "session", ExecutionID: "execution"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	job := &models.TaskResourceCleanupJob{
		ID: "manifest-boundary-job", OperationID: "delete:manifest-boundary",
		TaskID: "task", Trigger: models.TaskResourceCleanupTriggerDelete,
		State: models.TaskResourceCleanupStatePending, ResourceSnapshot: string(snapshot),
	}
	if err := repo.CreateTaskResourceCleanupJob(ctx, job); err != nil {
		t.Fatal(err)
	}
	if err := svc.processTaskResourceCleanupJob(ctx, job.ID); err == nil {
		t.Fatal("first cleanup attempt succeeded; expected partial cleanup failure")
	}
	cleanup.cleanupErr = nil
	if err := svc.processTaskResourceCleanupJob(ctx, job.ID); err != nil {
		t.Fatalf("retry cleanup: %v", err)
	}
	if cleanup.captureCount != 1 || !cleanup.cleanupCalled {
		t.Fatalf("capture count=%d cleanup called=%v, want one capture reused on retry", cleanup.captureCount, cleanup.cleanupCalled)
	}
}

// @covers AC-TASKS-ARCHIVE-SOURCE-MANIFEST-001.1
func TestCleanupPersistsSourceManifestAfterStopBeforeWorktreeRemoval(t *testing.T) {
	ctx := context.Background()
	svc, repo := setupOfficeTest(t)
	svc.StopTaskResourceCleanupWorker()
	stopper := &manifestBoundaryStopper{}
	cleanup := &manifestBoundaryCleanup{repo: repo, stopper: stopper}
	svc.SetWorktreeCleanup(cleanup)
	svc.SetExecutionStopper(stopper)

	snapshot, err := json.Marshal(taskResourceCleanupSnapshot{
		Worktrees:   []*worktree.Worktree{{ID: "wt", TaskID: "task", RepositoryID: "repo"}},
		StopTargets: []persistedTaskStopTarget{{SessionID: "session", ExecutionID: "execution"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	job := &models.TaskResourceCleanupJob{
		ID: "manifest-boundary-job", OperationID: "delete:manifest-boundary",
		TaskID: "task", Trigger: models.TaskResourceCleanupTriggerDelete,
		State: models.TaskResourceCleanupStatePending, ResourceSnapshot: string(snapshot),
	}
	if err := repo.CreateTaskResourceCleanupJob(ctx, job); err != nil {
		t.Fatal(err)
	}
	if err := svc.processTaskResourceCleanupJob(ctx, job.ID); err != nil {
		t.Fatalf("process cleanup: %v", err)
	}
	if !stopper.stopped || !cleanup.cleanupCalled {
		t.Fatalf("stop=%v cleanup=%v, want stop then cleanup", stopper.stopped, cleanup.cleanupCalled)
	}
}

// @covers AC-TASKS-ARCHIVE-SOURCE-MANIFEST-001.1
func TestDeleteCleanupTriggersPersistSourceManifestBeforeWorktreeRemoval(t *testing.T) {
	for _, trigger := range []models.TaskResourceCleanupTrigger{
		models.TaskResourceCleanupTriggerWorkspaceDelete,
		models.TaskResourceCleanupTriggerQuickChatExpire,
	} {
		t.Run(string(trigger), func(t *testing.T) {
			ctx := context.Background()
			svc, repo := setupOfficeTest(t)
			svc.StopTaskResourceCleanupWorker()
			stopper := &manifestBoundaryStopper{}
			cleanup := &manifestBoundaryCleanup{repo: repo, stopper: stopper}
			svc.SetWorktreeCleanup(cleanup)
			svc.SetExecutionStopper(stopper)

			snapshot, err := json.Marshal(taskResourceCleanupSnapshot{
				Worktrees:   []*worktree.Worktree{{ID: "wt", TaskID: "task", RepositoryID: "repo"}},
				StopTargets: []persistedTaskStopTarget{{SessionID: "session", ExecutionID: "execution"}},
			})
			if err != nil {
				t.Fatal(err)
			}
			job := &models.TaskResourceCleanupJob{
				ID: "manifest-boundary-job", OperationID: string(trigger) + ":manifest-boundary",
				TaskID: "task", Trigger: trigger,
				State: models.TaskResourceCleanupStatePending, ResourceSnapshot: string(snapshot),
			}
			if err := repo.CreateTaskResourceCleanupJob(ctx, job); err != nil {
				t.Fatal(err)
			}
			if err := svc.processTaskResourceCleanupJob(ctx, job.ID); err != nil {
				t.Fatalf("process %s cleanup: %v", trigger, err)
			}
			if !stopper.stopped || cleanup.captureCount != 1 || !cleanup.cleanupCalled {
				t.Fatalf("stop=%v captures=%d cleanup=%v, want manifest persisted before removal",
					stopper.stopped, cleanup.captureCount, cleanup.cleanupCalled)
			}
		})
	}
}

// @covers AC-TASKS-ARCHIVE-SOURCE-MANIFEST-001.4
func TestCleanupCaptureFailureBlocksWorktreeRemoval(t *testing.T) {
	ctx := context.Background()
	svc, repo := setupOfficeTest(t)
	svc.StopTaskResourceCleanupWorker()
	stopper := &manifestBoundaryStopper{}
	captureErr := errors.New("capture unavailable")
	cleanup := &manifestBoundaryCleanup{repo: repo, stopper: stopper, captureErr: captureErr}
	svc.SetWorktreeCleanup(cleanup)
	svc.SetExecutionStopper(stopper)

	snapshot, err := json.Marshal(taskResourceCleanupSnapshot{
		Worktrees: []*worktree.Worktree{{ID: "wt", TaskID: "task", RepositoryID: "repo"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	job := &models.TaskResourceCleanupJob{
		ID: "manifest-boundary-job", OperationID: "delete:manifest-boundary",
		TaskID: "task", Trigger: models.TaskResourceCleanupTriggerDelete,
		State: models.TaskResourceCleanupStatePending, ResourceSnapshot: string(snapshot),
	}
	if err := repo.CreateTaskResourceCleanupJob(ctx, job); err != nil {
		t.Fatal(err)
	}
	if err := svc.processTaskResourceCleanupJob(ctx, job.ID); err == nil {
		t.Fatal("cleanup succeeded after source capture failed")
	}
	if cleanup.cleanupCalled {
		t.Fatal("worktree cleanup ran after source capture failed")
	}
}

// @covers AC-TASKS-ARCHIVE-SOURCE-MANIFEST-001.8
func TestCleanupManifestCancellationBlocksRemoval(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	svc, repo := setupOfficeTest(t)
	svc.StopTaskResourceCleanupWorker()
	stopper := &manifestBoundaryStopper{}
	captureStarted := make(chan struct{})
	cleanup := &manifestBoundaryCleanup{
		repo: repo, stopper: stopper, captureStarted: captureStarted, waitCaptureCancel: true,
	}
	svc.SetWorktreeCleanup(cleanup)
	svc.SetExecutionStopper(stopper)

	snapshot, err := json.Marshal(taskResourceCleanupSnapshot{
		Worktrees:   []*worktree.Worktree{{ID: "wt", TaskID: "task", RepositoryID: "repo"}},
		StopTargets: []persistedTaskStopTarget{{SessionID: "session", ExecutionID: "execution"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	job := &models.TaskResourceCleanupJob{
		ID: "manifest-boundary-job", OperationID: "delete:manifest-cancel",
		TaskID: "task", Trigger: models.TaskResourceCleanupTriggerDelete,
		State: models.TaskResourceCleanupStatePending, ResourceSnapshot: string(snapshot),
	}
	if err := repo.CreateTaskResourceCleanupJob(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	processDone := make(chan error, 1)
	go func() { processDone <- svc.processTaskResourceCleanupJob(ctx, job.ID) }()
	select {
	case <-captureStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("cleanup did not reach source capture")
	}
	cancel()
	select {
	case err := <-processDone:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cleanup error = %v, want cancellation", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cleanup did not finish its canceled attempt")
	}
	if cleanup.cleanupCalled {
		t.Fatal("worktree removal ran after manifest capture was canceled")
	}
	job, err = repo.GetTaskResourceCleanupJob(context.Background(), job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if job.State != models.TaskResourceCleanupStateRetryWait || !strings.Contains(job.LastError, context.Canceled.Error()) {
		t.Fatalf("cleanup retry state=%q last_error=%q, want retained cancellation diagnostics", job.State, job.LastError)
	}
	var persisted taskResourceCleanupSnapshot
	if err := json.Unmarshal([]byte(job.ResourceSnapshot), &persisted); err != nil {
		t.Fatal(err)
	}
	if persisted.ArchiveSourceManifestCaptured || len(persisted.ArchiveSourceManifest) != 0 {
		t.Fatalf("canceled attempt persisted partial manifest: %+v", persisted)
	}
}

// @covers AC-TASKS-ARCHIVE-SOURCE-MANIFEST-001.8
func TestStopTaskResourceCleanupWorkerCancelsManifestCapture(t *testing.T) {
	ctx := context.Background()
	svc, repo := setupOfficeTest(t)
	svc.StopTaskResourceCleanupWorker()
	stopper := &manifestBoundaryStopper{}
	captureStarted := make(chan struct{})
	cleanup := &manifestBoundaryCleanup{
		repo: repo, stopper: stopper, captureStarted: captureStarted, waitCaptureCancel: true,
	}
	svc.SetWorktreeCleanup(cleanup)
	svc.SetExecutionStopper(stopper)
	snapshot, err := json.Marshal(taskResourceCleanupSnapshot{
		Worktrees:   []*worktree.Worktree{{ID: "wt", TaskID: "task", RepositoryID: "repo"}},
		StopTargets: []persistedTaskStopTarget{{SessionID: "session", ExecutionID: "execution"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	job := &models.TaskResourceCleanupJob{
		ID: "manifest-boundary-job", OperationID: "delete:manifest-stop-worker",
		TaskID: "task", Trigger: models.TaskResourceCleanupTriggerDelete,
		State: models.TaskResourceCleanupStatePending, ResourceSnapshot: string(snapshot),
	}
	if err := repo.CreateTaskResourceCleanupJob(ctx, job); err != nil {
		t.Fatal(err)
	}
	if err := svc.StartTaskResourceCleanupWorker(ctx); err != nil {
		t.Fatalf("StartTaskResourceCleanupWorker: %v", err)
	}
	select {
	case <-captureStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("cleanup worker did not reach manifest capture")
	}
	stopDone := make(chan struct{})
	go func() {
		svc.StopTaskResourceCleanupWorker()
		close(stopDone)
	}()
	select {
	case <-stopDone:
	case <-time.After(5 * time.Second):
		t.Fatal("StopTaskResourceCleanupWorker did not join canceled capture")
	}
	if cleanup.cleanupCalled {
		t.Fatal("worktree removal ran after worker canceled manifest capture")
	}
	job, err = repo.GetTaskResourceCleanupJob(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if job.State != models.TaskResourceCleanupStateRetryWait || !strings.Contains(job.LastError, context.Canceled.Error()) {
		t.Fatalf("cleanup retry state=%q last_error=%q, want retained cancellation diagnostics", job.State, job.LastError)
	}
	var persisted taskResourceCleanupSnapshot
	if err := json.Unmarshal([]byte(job.ResourceSnapshot), &persisted); err != nil {
		t.Fatal(err)
	}
	if persisted.ArchiveSourceManifestCaptured || len(persisted.ArchiveSourceManifest) != 0 {
		t.Fatalf("worker stop persisted a partial manifest: %+v", persisted)
	}
}
