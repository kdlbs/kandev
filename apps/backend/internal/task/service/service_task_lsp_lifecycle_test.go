package service

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

type recordingTaskLSPLifecycle struct {
	mu             sync.Mutex
	cleanupCalls   []string
	reconcileCalls []string
	cleanupErr     error
	onCleanup      func(context.Context, string)
}

func (l *recordingTaskLSPLifecycle) CleanupTask(ctx context.Context, taskID, reason string) error {
	l.mu.Lock()
	l.cleanupCalls = append(l.cleanupCalls, taskID+":"+reason)
	onCleanup := l.onCleanup
	err := l.cleanupErr
	l.mu.Unlock()
	if onCleanup != nil {
		onCleanup(ctx, taskID)
	}
	return err
}

func (l *recordingTaskLSPLifecycle) ReconcileTask(_ context.Context, taskID string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.reconcileCalls = append(l.reconcileCalls, taskID)
	return nil
}

func (l *recordingTaskLSPLifecycle) WorkspaceSourcesChanged(context.Context, string) error {
	return nil
}

func seedTaskForLSPLifecycleTest(t *testing.T, repo interface {
	CreateWorkspace(context.Context, *models.Workspace) error
	CreateWorkflow(context.Context, *models.Workflow) error
	CreateTask(context.Context, *models.Task) error
}) {
	t.Helper()
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-lsp", Name: "Workspace"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-lsp", WorkspaceID: "ws-lsp", Name: "Workflow"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "task-lsp", WorkspaceID: "ws-lsp", WorkflowID: "wf-lsp",
		WorkflowStepID: "step-lsp", Title: "LSP", Priority: "medium",
	}); err != nil {
		t.Fatal(err)
	}
}

func TestService_ArchiveTaskStopsTaskLSPBeforeMutation(t *testing.T) {
	svc, _, repo := createTestService(t)
	seedTaskForLSPLifecycleTest(t, repo)
	lifecycle := &recordingTaskLSPLifecycle{}
	lifecycle.onCleanup = func(ctx context.Context, taskID string) {
		task, err := repo.GetTask(ctx, taskID)
		if err != nil || task.ArchivedAt != nil {
			t.Errorf("LSP cleanup ran after archive: task=%v err=%v", task, err)
		}
	}
	svc.SetTaskLSPLifecycle(lifecycle)

	if err := svc.ArchiveTask(context.Background(), "task-lsp"); err != nil {
		t.Fatal(err)
	}
	if got := lifecycle.cleanupCalls; len(got) != 1 || got[0] != "task-lsp:task_archived" {
		t.Fatalf("cleanup calls = %v", got)
	}
}

func TestService_DeleteTaskStopsTaskLSPBeforeMutation(t *testing.T) {
	svc, _, repo := createTestService(t)
	seedTaskForLSPLifecycleTest(t, repo)
	lifecycle := &recordingTaskLSPLifecycle{}
	lifecycle.onCleanup = func(ctx context.Context, taskID string) {
		if _, err := repo.GetTask(ctx, taskID); err != nil {
			t.Errorf("LSP cleanup ran after delete: %v", err)
		}
	}
	svc.SetTaskLSPLifecycle(lifecycle)

	if err := svc.DeleteTask(context.Background(), "task-lsp"); err != nil {
		t.Fatal(err)
	}
	if got := lifecycle.cleanupCalls; len(got) != 1 || got[0] != "task-lsp:task_deleted" {
		t.Fatalf("cleanup calls = %v", got)
	}
}

func TestService_ArchiveTaskBlocksLSPAdmissionThroughMutation(t *testing.T) {
	svc, _, repo := createTestService(t)
	seedTaskForLSPLifecycleTest(t, repo)
	cleanupEntered := make(chan struct{})
	cleanupRelease := make(chan struct{})
	svc.SetTaskLSPLifecycle(&recordingTaskLSPLifecycle{
		onCleanup: func(context.Context, string) {
			close(cleanupEntered)
			<-cleanupRelease
		},
	})

	done := make(chan error, 1)
	go func() { done <- svc.ArchiveTask(context.Background(), "task-lsp") }()
	<-cleanupEntered
	assertTaskLSPAdmissionBlocked(t, svc, "task-lsp")
	close(cleanupRelease)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	task, err := repo.GetTask(context.Background(), "task-lsp")
	if err != nil || task.ArchivedAt == nil {
		t.Fatalf("task was not archived before admission reopened: task=%#v err=%v", task, err)
	}
}

func TestService_DeleteTaskBlocksLSPAdmissionThroughMutation(t *testing.T) {
	svc, _, repo := createTestService(t)
	seedTaskForLSPLifecycleTest(t, repo)
	cleanupEntered := make(chan struct{})
	cleanupRelease := make(chan struct{})
	svc.SetTaskLSPLifecycle(&recordingTaskLSPLifecycle{
		onCleanup: func(context.Context, string) {
			close(cleanupEntered)
			<-cleanupRelease
		},
	})

	done := make(chan error, 1)
	go func() { done <- svc.DeleteTask(context.Background(), "task-lsp") }()
	<-cleanupEntered
	assertTaskLSPAdmissionBlocked(t, svc, "task-lsp")
	close(cleanupRelease)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if _, err := repo.GetTask(context.Background(), "task-lsp"); !errors.Is(err, repoerrors.ErrTaskNotFound) {
		t.Fatalf("task remained after admission reopened: %v", err)
	}
}

func TestService_StopTaskLSPMarksEnvironmentStoppedBeforeCleanupAndBlocksAdmission(t *testing.T) {
	svc, _, repo := createTestService(t)
	seedTaskForLSPLifecycleTest(t, repo)
	if err := repo.CreateTaskEnvironment(context.Background(), &models.TaskEnvironment{
		ID:           "env-lsp-stop",
		TaskID:       "task-lsp",
		ExecutorType: string(models.ExecutorTypeLocal),
		Status:       models.TaskEnvironmentStatusReady,
	}); err != nil {
		t.Fatal(err)
	}

	cleanupEntered := make(chan struct{})
	cleanupRelease := make(chan struct{})
	lifecycle := &recordingTaskLSPLifecycle{
		onCleanup: func(ctx context.Context, taskID string) {
			environment, err := repo.GetTaskEnvironmentByTaskID(ctx, taskID)
			if err != nil || environment == nil || environment.Status != models.TaskEnvironmentStatusStopped {
				t.Errorf("environment was not stopped before LSP cleanup: environment=%#v err=%v", environment, err)
			}
			close(cleanupEntered)
			<-cleanupRelease
		},
	}
	svc.SetTaskLSPLifecycle(lifecycle)

	done := make(chan error, 1)
	go func() { done <- svc.StopTaskLSP(context.Background(), "task-lsp", "user_stop") }()
	<-cleanupEntered
	assertTaskLSPAdmissionBlocked(t, svc, "task-lsp")
	close(cleanupRelease)
	if err := <-done; err != nil {
		t.Fatal(err)
	}

	environment, err := repo.GetTaskEnvironmentByTaskID(context.Background(), "task-lsp")
	if err != nil || environment == nil || environment.Status != models.TaskEnvironmentStatusStopped {
		t.Fatalf("environment after task stop = %#v, err=%v", environment, err)
	}
	if got := lifecycle.cleanupCalls; len(got) != 1 || got[0] != "task-lsp:user_stop" {
		t.Fatalf("cleanup calls = %v", got)
	}
}

func assertTaskLSPAdmissionBlocked(t *testing.T, svc *Service, taskID string) {
	t.Helper()
	release, err := svc.AcquireTaskLSPAdmission(context.Background(), taskID)
	if release != nil {
		release()
	}
	if !errors.Is(err, ErrTaskLSPAdmissionBlocked) {
		t.Fatalf("LSP admission during terminal mutation = %v, want blocked", err)
	}
}

func TestService_TaskMutationFailsClosedWhenTaskLSPCleanupFails(t *testing.T) {
	svc, _, repo := createTestService(t)
	seedTaskForLSPLifecycleTest(t, repo)
	svc.SetTaskLSPLifecycle(&recordingTaskLSPLifecycle{cleanupErr: errors.New("stop failed")})

	if err := svc.ArchiveTask(context.Background(), "task-lsp"); err == nil {
		t.Fatal("archive succeeded despite LSP cleanup failure")
	}
	task, err := repo.GetTask(context.Background(), "task-lsp")
	if err != nil || task.ArchivedAt != nil {
		t.Fatalf("archive mutated task after cleanup failure: task=%v err=%v", task, err)
	}
	if err := svc.DeleteTask(context.Background(), "task-lsp"); err == nil {
		t.Fatal("delete succeeded despite LSP cleanup failure")
	}
	if _, err := repo.GetTask(context.Background(), "task-lsp"); errors.Is(err, repoerrors.ErrTaskNotFound) {
		t.Fatal("delete removed task after cleanup failure")
	}
}
