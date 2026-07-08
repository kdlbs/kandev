package service

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	"github.com/kandev/kandev/internal/worktree"
)

// --- isCleanableSessionState ---

func TestIsCleanableSessionState(t *testing.T) {
	cleanable := []models.TaskSessionState{
		models.TaskSessionStateCancelled,
		models.TaskSessionStateCompleted,
		models.TaskSessionStateFailed,
		models.TaskSessionStateIdle,
	}
	for _, s := range cleanable {
		if !isCleanableSessionState(s) {
			t.Errorf("expected %q to be cleanable", s)
		}
	}

	nonCleanable := []models.TaskSessionState{
		models.TaskSessionStateRunning,
		models.TaskSessionStateCreated,
		models.TaskSessionStateStarting,
	}
	for _, s := range nonCleanable {
		if isCleanableSessionState(s) {
			t.Errorf("expected %q to NOT be cleanable", s)
		}
	}
}

// TestIsCleanableSessionState_IdleIncluded guards against a future change that
// accidentally excludes IDLE (the orchestrator's same-named function does NOT
// include IDLE, but in the cleanup path IDLE sessions have no live execution).
func TestIsCleanableSessionState_IdleIncluded(t *testing.T) {
	if !isCleanableSessionState(models.TaskSessionStateIdle) {
		t.Error("IDLE must be cleanable: an idle session has no live execution and StopSession will return ErrExecutionNotFound")
	}
}

// --- buildStopTargets ---

// stubExecutors is a minimal repository.ExecutorRepository implementation for tests.
type stubExecutors struct {
	repository.ExecutorRepository
	runningByTaskID  []*models.ExecutorRunning
	runningByTaskErr error
	runningBySession *models.ExecutorRunning
	runningBySessErr error
}

func (s *stubExecutors) ListExecutorsRunningByTaskID(_ context.Context, _ string) ([]*models.ExecutorRunning, error) {
	return s.runningByTaskID, s.runningByTaskErr
}

func (s *stubExecutors) GetExecutorRunningBySessionID(_ context.Context, _ string) (*models.ExecutorRunning, error) {
	return s.runningBySession, s.runningBySessErr
}

func (s *stubExecutors) DeleteExecutorRunningBySessionID(_ context.Context, _ string) error {
	return nil
}

func (s *stubExecutors) HasExecutorRunningRow(_ context.Context, _ string) (bool, error) {
	return s.runningBySession != nil, nil
}

func TestBuildStopTargets_TerminalExecutorRow(t *testing.T) {
	svc, _, _ := createTestService(t)
	svc.executors = &stubExecutors{
		runningByTaskID: []*models.ExecutorRunning{
			{SessionID: "sess-1", AgentExecutionID: "exec-1"},
		},
	}

	// Session is CANCELLED — stop target must be marked terminal.
	sessions := []*models.TaskSession{
		{ID: "sess-1", State: models.TaskSessionStateCancelled, AgentExecutionID: "exec-1"},
	}

	targets, err := svc.buildStopTargets(context.Background(), "task-1", sessions)
	if err != nil {
		t.Fatalf("buildStopTargets error: %v", err)
	}
	if len(targets) != 1 {
		t.Fatalf("expected 1 target, got %d", len(targets))
	}
	if !targets[0].terminal {
		t.Error("expected target to be marked terminal for a CANCELLED session")
	}
}

func TestBuildStopTargets_NonTerminalExecutorRow(t *testing.T) {
	svc, _, _ := createTestService(t)
	svc.executors = &stubExecutors{
		runningByTaskID: []*models.ExecutorRunning{
			{SessionID: "sess-2", AgentExecutionID: "exec-2"},
		},
	}

	// Session is RUNNING — stop target must NOT be terminal.
	sessions := []*models.TaskSession{
		{ID: "sess-2", State: models.TaskSessionStateRunning, AgentExecutionID: "exec-2"},
	}

	targets, err := svc.buildStopTargets(context.Background(), "task-2", sessions)
	if err != nil {
		t.Fatalf("buildStopTargets error: %v", err)
	}
	if len(targets) != 1 {
		t.Fatalf("expected 1 target, got %d", len(targets))
	}
	if targets[0].terminal {
		t.Error("expected target to NOT be terminal for a RUNNING session")
	}
}

func TestBuildStopTargets_TerminalSessionWithoutExecutorRow(t *testing.T) {
	svc, _, _ := createTestService(t)
	svc.executors = &stubExecutors{
		runningByTaskID: nil, // no executor_running rows
	}

	// Session is COMPLETED with no executor_running row → no stop target created.
	sessions := []*models.TaskSession{
		{ID: "sess-3", State: models.TaskSessionStateCompleted, AgentExecutionID: "exec-3"},
	}

	targets, err := svc.buildStopTargets(context.Background(), "task-3", sessions)
	if err != nil {
		t.Fatalf("buildStopTargets error: %v", err)
	}
	if len(targets) != 0 {
		t.Errorf("expected 0 targets for terminal session without executor row, got %d", len(targets))
	}
}

// --- stopTaskRuntimeTargets ---

// stubStopper is a minimal TaskExecutionStopper for tests.
type stubStopper struct {
	stopSessionErr error
}

func (s *stubStopper) StopTask(_ context.Context, _, _ string, _ bool) error      { return nil }
func (s *stubStopper) StopExecution(_ context.Context, _, _ string, _ bool) error { return nil }
func (s *stubStopper) StopSession(_ context.Context, _, _ string, _ bool) error {
	return s.stopSessionErr
}

func TestStopTaskRuntimeTargets_TerminalStopFailureDoesNotBlockCleanup(t *testing.T) {
	svc, _, _ := createTestService(t)
	svc.executors = &stubExecutors{}
	svc.executionStopper = &stubStopper{stopSessionErr: errors.New("ErrExecutionNotFound")}

	targets := []taskStopTarget{
		{sessionID: "sess-cancelled", terminal: true},
	}

	failed := svc.stopTaskRuntimeTargets(context.Background(), "task-a", targets, "archive", "stop failed")
	if len(failed) != 0 {
		t.Errorf("terminal stop failure must not add to failedStops; got %v", failed)
	}
}

func TestStopTaskRuntimeTargets_NonTerminalStopFailureBlocksCleanup(t *testing.T) {
	svc, _, _ := createTestService(t)
	svc.executors = &stubExecutors{}
	svc.executionStopper = &stubStopper{stopSessionErr: errors.New("unexpected error")}

	targets := []taskStopTarget{
		{sessionID: "sess-running", terminal: false},
	}

	failed := svc.stopTaskRuntimeTargets(context.Background(), "task-b", targets, "archive", "stop failed")
	if _, ok := failed["sess-running"]; !ok {
		t.Error("non-terminal stop failure must add session to failedStops")
	}
}

// --- CleanupTaskResources cascade regression tests ---
//
// These tests reproduce the archive cleanup regression: ArchiveTaskTree calls
// cancelActiveRuns (sessions → CANCELLED) before CleanupTaskResources, leaving
// executor_running rows whose StopExecution returns ErrExecutionNotFound. The
// stop failure must not block environment teardown for terminal sessions.

func seedCascadeFixtures(t *testing.T, repo interface {
	CreateWorkspace(context.Context, *models.Workspace) error
	CreateWorkflow(context.Context, *models.Workflow) error
	CreateTask(context.Context, *models.Task) error
	CreateTaskSession(context.Context, *models.TaskSession) error
	UpsertExecutorRunning(context.Context, *models.ExecutorRunning) error
}, wsID, wfID, taskID, sessID, execID string, state models.TaskSessionState) {
	t.Helper()
	ctx := context.Background()
	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: wsID, Name: "WS"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: wfID, WorkspaceID: wsID, Name: "WF"})
	_ = repo.CreateTask(ctx, &models.Task{ID: taskID, WorkspaceID: wsID, WorkflowID: wfID, WorkflowStepID: "step-1", Title: "T", Priority: "medium"})
	_ = repo.CreateTaskSession(ctx, &models.TaskSession{
		ID:               sessID,
		TaskID:           taskID,
		State:            state,
		AgentExecutionID: execID,
	})
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID:               sessID,
		SessionID:        sessID,
		TaskID:           taskID,
		ExecutorID:       "executor-1",
		AgentExecutionID: execID,
		Runtime:          agentruntime.RuntimeStandalone,
		Status:           models.ExecutorRunningStatusStarting,
	}); err != nil {
		t.Fatalf("seed executor_running: %v", err)
	}
}

// TestCleanupTaskResources_TerminalSessionStopFailureDoesNotBlockCleanup is a
// regression test for the archive cleanup bug: a CANCELLED session with a stale
// executor_running row must have its row removed even when StopExecution fails.
func TestCleanupTaskResources_TerminalSessionStopFailureDoesNotBlockCleanup(t *testing.T) {
	svc, _, repo := createTestService(t)

	stopper := newRecordingTaskExecutionStopper()
	stopper.stopExecutionErr = errors.New("execution not found")
	svc.SetExecutionStopper(stopper)
	svc.setCleanupDoneForTestHook(make(chan struct{}, 1))

	seedCascadeFixtures(t, repo, "ws-c1", "wf-c1", "task-c1", "sess-cancelled", "exec-cancelled", models.TaskSessionStateCancelled)

	svc.CleanupTaskResources(context.Background(), "task-c1", false)
	waitForCleanupDone(t, svc)

	_, err := repo.GetExecutorRunningBySessionID(context.Background(), "sess-cancelled")
	if err == nil {
		t.Error("executor_running row must be removed after cleanup of terminal (CANCELLED) session — stop failure must not block teardown")
	}
}

// TestCleanupTaskResources_NonTerminalSessionStopFailureBlocksCleanup is the
// companion case: a RUNNING session whose StopExecution fails must keep its
// executor_running row so the runtime is not torn down unexpectedly.
func TestCleanupTaskResources_NonTerminalSessionStopFailureBlocksCleanup(t *testing.T) {
	svc, _, repo := createTestService(t)

	stopper := newRecordingTaskExecutionStopper()
	stopper.stopExecutionErr = errors.New("stop failed")
	svc.SetExecutionStopper(stopper)
	svc.setCleanupDoneForTestHook(make(chan struct{}, 1))

	seedCascadeFixtures(t, repo, "ws-c2", "wf-c2", "task-c2", "sess-running", "exec-running", models.TaskSessionStateRunning)

	svc.CleanupTaskResources(context.Background(), "task-c2", false)
	waitForCleanupDone(t, svc)

	if _, err := repo.GetExecutorRunningBySessionID(context.Background(), "sess-running"); err != nil {
		t.Error("executor_running row must be preserved when stop fails for a non-terminal (RUNNING) session")
	}
}

func TestCleanupTaskResources_SkipsBorrowedInheritedWorktree(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	seedParentChildWorkspace(t, repo, "ws-inherited", "wf-inherited", "parent-task", "child-task")
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID:           "env-parent",
		TaskID:       "parent-task",
		WorktreeID:   "wt-parent",
		WorktreePath: "/tmp/parent-worktree",
		Status:       models.TaskEnvironmentStatusReady,
	}); err != nil {
		t.Fatalf("create parent environment: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID:                "session-child",
		TaskID:            "child-task",
		State:             models.TaskSessionStateCancelled,
		TaskEnvironmentID: "env-parent",
	}); err != nil {
		t.Fatalf("create child session: %v", err)
	}

	cleanup := &recordingWorktreeCleanup{
		worktreesByTaskID: map[string][]*worktree.Worktree{
			"child-task": {{
				ID:        "wt-parent",
				TaskID:    "child-task",
				SessionID: "session-child",
				Path:      "/tmp/parent-worktree",
			}},
		},
	}
	svc.SetWorktreeCleanup(cleanup)
	svc.setCleanupDoneForTestHook(make(chan struct{}, 1))

	svc.CleanupTaskResources(ctx, "child-task", true)
	waitForCleanupDone(t, svc)

	if cleanedIDs := cleanup.cleanedIDs(); len(cleanedIDs) != 0 {
		t.Fatalf("child cleanup must not clean inherited parent worktrees, got %#v", cleanedIDs)
	}
}

func TestCleanupTaskResources_PreservesOwnedEnvironmentWithActiveInheritedChild(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	seedParentChildWorkspace(t, repo, "ws-shared", "wf-shared", "parent-task", "child-task")
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID:           "env-parent",
		TaskID:       "parent-task",
		WorktreeID:   "wt-parent",
		WorktreePath: "/tmp/parent-worktree",
		Status:       models.TaskEnvironmentStatusReady,
	}); err != nil {
		t.Fatalf("create parent environment: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID:                "session-parent",
		TaskID:            "parent-task",
		State:             models.TaskSessionStateCancelled,
		TaskEnvironmentID: "env-parent",
	}); err != nil {
		t.Fatalf("create parent session: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID:                "session-child",
		TaskID:            "child-task",
		State:             models.TaskSessionStateRunning,
		TaskEnvironmentID: "env-parent",
	}); err != nil {
		t.Fatalf("create child session: %v", err)
	}

	destroyer := &stubDestroyer{}
	svc.SetEnvironmentDestroyer(destroyer)
	cleanup := &recordingWorktreeCleanup{
		worktreesByTaskID: map[string][]*worktree.Worktree{
			"parent-task": {{
				ID:        "wt-parent",
				TaskID:    "parent-task",
				SessionID: "session-parent",
				Path:      "/tmp/parent-worktree",
			}},
		},
	}
	svc.SetWorktreeCleanup(cleanup)
	svc.setCleanupDoneForTestHook(make(chan struct{}, 1))

	svc.CleanupTaskResources(ctx, "parent-task", false)
	waitForCleanupDone(t, svc)

	if len(destroyer.worktreeCalls) != 0 {
		t.Fatalf("parent cleanup must not destroy a worktree while an active child inherits it, got %#v", destroyer.worktreeCalls)
	}
	if cleanedIDs := cleanup.cleanedIDs(); len(cleanedIDs) != 0 {
		t.Fatalf("parent cleanup must not batch-clean a shared inherited worktree, got %#v", cleanedIDs)
	}
}

func seedParentChildWorkspace(t *testing.T, repo interface {
	CreateWorkspace(context.Context, *models.Workspace) error
	CreateWorkflow(context.Context, *models.Workflow) error
	CreateTask(context.Context, *models.Task) error
}, wsID, wfID, parentID, childID string) {
	t.Helper()
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: wsID, Name: "Workspace"}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: wfID, WorkspaceID: wsID, Name: "Workflow"}); err != nil {
		t.Fatalf("create workflow: %v", err)
	}
	if err := repo.CreateTask(ctx, &models.Task{ID: parentID, WorkspaceID: wsID, WorkflowID: wfID, WorkflowStepID: "step-1", Title: "Parent", Priority: "medium"}); err != nil {
		t.Fatalf("create parent task: %v", err)
	}
	if err := repo.CreateTask(ctx, &models.Task{ID: childID, WorkspaceID: wsID, WorkflowID: wfID, WorkflowStepID: "step-1", ParentID: parentID, Title: "Child", Priority: "medium"}); err != nil {
		t.Fatalf("create child task: %v", err)
	}
}
