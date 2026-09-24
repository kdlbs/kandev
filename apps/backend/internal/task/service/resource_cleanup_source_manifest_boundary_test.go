package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

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
	stopper       *manifestBoundaryStopper
	captureErr    error
	captureCount  int
	cleanupErr    error
	cleanupCalled bool
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
	_ context.Context, worktrees []*worktree.Worktree,
) (map[string]worktree.ArchiveSourceManifest, error) {
	c.captureCount++
	if !c.stopper.stopped {
		return nil, errors.New("capture ran before runtime stop")
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
