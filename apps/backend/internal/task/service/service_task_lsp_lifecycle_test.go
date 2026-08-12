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

func TestService_TerminalMutationBlocksBorrowerAdmissionOnPhysicalEnvironment(t *testing.T) {
	for _, action := range []struct {
		name string
		run  func(*Service) error
	}{
		{name: "archive", run: func(svc *Service) error {
			return svc.ArchiveTask(context.Background(), "parent-task")
		}},
		{name: "delete", run: func(svc *Service) error {
			return svc.DeleteTask(context.Background(), "parent-task")
		}},
	} {
		t.Run(action.name, func(t *testing.T) {
			svc, _, repo := createTestService(t)
			seedParentChildWorkspace(t, repo, "ws-physical", "wf-physical", "parent-task", "child-task")
			if err := repo.CreateTaskEnvironment(context.Background(), &models.TaskEnvironment{
				ID: "env-physical", TaskID: "parent-task", ExecutorType: string(models.ExecutorTypeLocal),
				Status: models.TaskEnvironmentStatusReady,
			}); err != nil {
				t.Fatal(err)
			}
			if err := repo.CreateTaskSession(context.Background(), &models.TaskSession{
				ID: "session-child", TaskID: "child-task", State: models.TaskSessionStateCompleted,
				TaskEnvironmentID: "env-physical",
			}); err != nil {
				t.Fatal(err)
			}
			cleanupEntered := make(chan struct{})
			cleanupRelease := make(chan struct{})
			svc.SetTaskLSPLifecycle(&recordingTaskLSPLifecycle{
				onCleanup: func(context.Context, string) {
					close(cleanupEntered)
					<-cleanupRelease
				},
			})

			done := make(chan error, 1)
			go func() { done <- action.run(svc) }()
			<-cleanupEntered
			assertTaskLSPAdmissionBlocked(t, svc, "child-task")
			close(cleanupRelease)
			if err := <-done; err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestService_BorrowerTerminalMutationBlocksOwnerAdmissionOnPhysicalEnvironment(t *testing.T) {
	for _, action := range []struct {
		name string
		run  func(*Service) error
	}{
		{name: "archive", run: func(svc *Service) error {
			return svc.ArchiveTask(context.Background(), "child-task")
		}},
		{name: "delete", run: func(svc *Service) error {
			return svc.DeleteTask(context.Background(), "child-task")
		}},
	} {
		t.Run(action.name, func(t *testing.T) {
			svc, _, repo := createTestService(t)
			seedParentChildWorkspace(t, repo, "ws-borrower", "wf-borrower", "parent-task", "child-task")
			if err := repo.CreateTaskEnvironment(context.Background(), &models.TaskEnvironment{
				ID: "env-borrower", TaskID: "parent-task", ExecutorType: string(models.ExecutorTypeLocal),
				Status: models.TaskEnvironmentStatusReady,
			}); err != nil {
				t.Fatal(err)
			}
			if err := repo.CreateTaskSession(context.Background(), &models.TaskSession{
				ID: "session-child", TaskID: "child-task", State: models.TaskSessionStateCompleted,
				TaskEnvironmentID: "env-borrower",
			}); err != nil {
				t.Fatal(err)
			}
			cleanupEntered := make(chan struct{})
			cleanupRelease := make(chan struct{})
			svc.SetTaskLSPLifecycle(&recordingTaskLSPLifecycle{
				onCleanup: func(context.Context, string) {
					close(cleanupEntered)
					<-cleanupRelease
				},
			})

			done := make(chan error, 1)
			go func() { done <- action.run(svc) }()
			<-cleanupEntered
			assertTaskLSPAdmissionBlocked(t, svc, "parent-task")
			close(cleanupRelease)
			if err := <-done; err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestTaskTreeTerminalMutationPreservesWarmBorrowerAndBlocksAdmission(t *testing.T) {
	for _, action := range []struct {
		name string
		run  func(context.Context, *HandoffService) error
	}{
		{name: "archive", run: func(ctx context.Context, handoff *HandoffService) error {
			_, err := handoff.ArchiveTaskTree(ctx, "parent-task", false)
			return err
		}},
		{name: "delete", run: func(ctx context.Context, handoff *HandoffService) error {
			_, err := handoff.DeleteTaskTree(ctx, "parent-task", false)
			return err
		}},
	} {
		t.Run(action.name, func(t *testing.T) {
			svc, _, repo := createTestService(t)
			ctx := context.Background()
			seedParentChildWorkspace(t, repo, "ws-tree-shared", "wf-tree-shared", "parent-task", "child-task")
			if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
				ID: "env-tree-shared", TaskID: "parent-task", ExecutorType: string(models.ExecutorTypeLocal),
				Status: models.TaskEnvironmentStatusReady,
			}); err != nil {
				t.Fatal(err)
			}
			if err := repo.CreateTaskSession(ctx, &models.TaskSession{
				ID: "session-tree-child", TaskID: "child-task", State: models.TaskSessionStateCompleted,
				TaskEnvironmentID: "env-tree-shared",
			}); err != nil {
				t.Fatal(err)
			}
			cleanupEntered := make(chan struct{})
			cleanupRelease := make(chan struct{})
			svc.SetTaskLSPLifecycle(&recordingTaskLSPLifecycle{
				onCleanup: func(context.Context, string) {
					close(cleanupEntered)
					<-cleanupRelease
				},
			})
			handoff := NewHandoffService(repo, repo, nil, nil, nil, nil)
			handoff.SetTaskResourceCleaner(svc)

			done := make(chan error, 1)
			go func() { done <- action.run(ctx, handoff) }()
			<-cleanupEntered
			assertTaskLSPAdmissionBlocked(t, svc, "child-task")
			environment, err := repo.GetTaskEnvironment(ctx, "env-tree-shared")
			if err != nil || environment.TaskID != "child-task" {
				close(cleanupRelease)
				<-done
				t.Fatalf("environment ownership during %s = %#v, err=%v", action.name, environment, err)
			}
			close(cleanupRelease)
			if err := <-done; err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestPreserveTaskEnvironmentsForTerminalMutationRollsBackPartialTransfer(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	seedParentChildWorkspace(t, repo, "ws-partial", "wf-partial", "owner-a", "owner-b")
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "borrower", WorkspaceID: "ws-partial", WorkflowID: "wf-partial",
		WorkflowStepID: "step-1", Title: "Borrower", Priority: "medium",
	}); err != nil {
		t.Fatal(err)
	}
	for _, environment := range []*models.TaskEnvironment{
		{ID: "env-a", TaskID: "owner-a", ExecutorType: string(models.ExecutorTypeLocal)},
		{ID: "env-b", TaskID: "owner-b", ExecutorType: string(models.ExecutorTypeLocal)},
	} {
		if err := repo.CreateTaskEnvironment(ctx, environment); err != nil {
			t.Fatal(err)
		}
	}
	for _, session := range []*models.TaskSession{
		{ID: "borrow-a", TaskID: "borrower", State: models.TaskSessionStateCompleted, TaskEnvironmentID: "env-a"},
		{ID: "borrow-b", TaskID: "borrower", State: models.TaskSessionStateCompleted, TaskEnvironmentID: "env-b"},
	} {
		if err := repo.CreateTaskSession(ctx, session); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := svc.preserveTaskEnvironmentsForTerminalMutation(ctx, []string{"owner-a", "owner-b"}); err == nil {
		t.Fatal("preserving incompatible environments unexpectedly succeeded")
	}
	for environmentID, ownerTaskID := range map[string]string{"env-a": "owner-a", "env-b": "owner-b"} {
		environment, err := repo.GetTaskEnvironment(ctx, environmentID)
		if err != nil || environment.TaskID != ownerTaskID {
			t.Fatalf("environment %s after rollback = %#v, err=%v", environmentID, environment, err)
		}
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

func TestService_StopTaskLSPPreservesWarmBorrowerEnvironment(t *testing.T) {
	svc, _, repo := createTestService(t)
	seedParentChildWorkspace(t, repo, "ws-stop-shared", "wf-stop-shared", "parent-task", "child-task")
	if err := repo.CreateTaskEnvironment(context.Background(), &models.TaskEnvironment{
		ID: "env-stop-shared", TaskID: "parent-task", ExecutorType: string(models.ExecutorTypeLocal),
		Status: models.TaskEnvironmentStatusReady,
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTaskSession(context.Background(), &models.TaskSession{
		ID: "session-child", TaskID: "child-task", State: models.TaskSessionStateCompleted,
		TaskEnvironmentID: "env-stop-shared",
	}); err != nil {
		t.Fatal(err)
	}
	cleanupEntered := make(chan struct{})
	cleanupRelease := make(chan struct{})
	lifecycle := &recordingTaskLSPLifecycle{
		onCleanup: func(context.Context, string) {
			close(cleanupEntered)
			<-cleanupRelease
		},
	}
	svc.SetTaskLSPLifecycle(lifecycle)

	done := make(chan error, 1)
	go func() { done <- svc.StopTaskLSP(context.Background(), "parent-task", "user_stop") }()
	<-cleanupEntered
	assertTaskLSPAdmissionBlocked(t, svc, "child-task")
	close(cleanupRelease)
	if err := <-done; err != nil {
		t.Fatal(err)
	}

	environment, err := repo.GetTaskEnvironment(context.Background(), "env-stop-shared")
	if err != nil || environment == nil {
		t.Fatalf("shared environment after owner stop = %#v, err=%v", environment, err)
	}
	if environment.TaskID != "child-task" || environment.Status != models.TaskEnvironmentStatusReady {
		t.Fatalf("shared environment after owner stop = %#v, want ready child ownership", environment)
	}
	if got := lifecycle.cleanupCalls; len(got) != 1 || got[0] != "parent-task:user_stop" {
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

func TestService_TaskMutationRestoresSharedEnvironmentOwnershipWhenLSPCleanupFails(t *testing.T) {
	for _, action := range []struct {
		name string
		run  func(*Service) error
	}{
		{name: "archive", run: func(svc *Service) error {
			return svc.ArchiveTask(context.Background(), "parent-task")
		}},
		{name: "delete", run: func(svc *Service) error {
			return svc.DeleteTask(context.Background(), "parent-task")
		}},
	} {
		t.Run(action.name, func(t *testing.T) {
			svc, _, repo := createTestService(t)
			seedParentChildWorkspace(t, repo, "ws-rollback", "wf-rollback", "parent-task", "child-task")
			if err := repo.CreateTaskEnvironment(context.Background(), &models.TaskEnvironment{
				ID: "env-rollback", TaskID: "parent-task", ExecutorType: string(models.ExecutorTypeLocal),
				Status: models.TaskEnvironmentStatusReady,
			}); err != nil {
				t.Fatal(err)
			}
			if err := repo.CreateTaskSession(context.Background(), &models.TaskSession{
				ID: "session-child", TaskID: "child-task", State: models.TaskSessionStateCompleted,
				TaskEnvironmentID: "env-rollback",
			}); err != nil {
				t.Fatal(err)
			}
			svc.SetTaskLSPLifecycle(&recordingTaskLSPLifecycle{cleanupErr: errors.New("stop failed")})

			if err := action.run(svc); err == nil {
				t.Fatal("terminal mutation succeeded despite LSP cleanup failure")
			}
			environment, err := repo.GetTaskEnvironment(context.Background(), "env-rollback")
			if err != nil || environment.TaskID != "parent-task" {
				t.Fatalf("environment ownership after failed %s = %#v, err=%v", action.name, environment, err)
			}
		})
	}
}
