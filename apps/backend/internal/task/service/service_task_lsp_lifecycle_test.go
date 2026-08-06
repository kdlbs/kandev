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
