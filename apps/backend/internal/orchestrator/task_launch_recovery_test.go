package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	taskservice "github.com/kandev/kandev/internal/task/service"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	"github.com/kandev/kandev/internal/worktree"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type taskLaunchRecoveryServiceFake struct {
	branches    []taskservice.Branch
	branchErr   error
	updated     []taskservice.UpdateRepositoryBaseBranchRequest
	moved       []taskservice.MoveTaskOptions
	moveErr     error
	moveTaskIDs []string
}

func (f *taskLaunchRecoveryServiceFake) UpdateRepositoryBaseBranch(_ context.Context, req taskservice.UpdateRepositoryBaseBranchRequest) (*models.TaskRepository, error) {
	f.updated = append(f.updated, req)
	return &models.TaskRepository{ID: req.TaskRepositoryID, TaskID: req.TaskID, BaseBranch: req.BaseBranch}, nil
}

func (f *taskLaunchRecoveryServiceFake) ListBranches(context.Context, string, string) ([]taskservice.Branch, error) {
	return f.branches, f.branchErr
}

func (f *taskLaunchRecoveryServiceFake) MoveTaskWithOptions(_ context.Context, taskID, workflowID, workflowStepID string, position int, opts taskservice.MoveTaskOptions) (*taskservice.MoveTaskResult, error) {
	f.moveTaskIDs = append(f.moveTaskIDs, taskID)
	f.moved = append(f.moved, opts)
	if f.moveErr != nil {
		return nil, f.moveErr
	}
	return &taskservice.MoveTaskResult{Task: &models.Task{ID: taskID, WorkflowID: workflowID, WorkflowStepID: workflowStepID, Position: position}}, nil
}

type taskLaunchRecoveryWorktreeFake struct {
	branch string
	err    error
}

func (f taskLaunchRecoveryWorktreeFake) ResolveRemoteDefaultBranch(context.Context, string) (string, error) {
	return f.branch, f.err
}

type blockingTaskLaunchRecoveryWorktree struct {
	started chan<- struct{}
	release <-chan struct{}
	err     error
}

func (f blockingTaskLaunchRecoveryWorktree) ResolveRemoteDefaultBranch(context.Context, string) (string, error) {
	f.started <- struct{}{}
	<-f.release
	return "", f.err
}

func seedTaskLaunchRecoveryFixture(t *testing.T, repo *sqliterepo.Repository, taskID, taskRepositoryID string, action string) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-recovery", Name: "Recovery", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-recovery", WorkspaceID: "ws-recovery", Name: "Recovery workflow", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}
	if err := repo.CreateTask(ctx, &models.Task{
		ID: taskID, WorkspaceID: "ws-recovery", WorkflowID: "wf-recovery", WorkflowStepID: "review-recovery",
		Title: "Recovery task", State: v1.TaskStateFailed, Metadata: map[string]interface{}{
			models.MetaKeyLastLaunchError: models.TaskLaunchError{
				Message:          "launch failed",
				OccurredAt:       now,
				Code:             models.LaunchErrorCategoryGenericLaunchFailure,
				RecoveryActions:  []string{action},
				TaskRepositoryID: taskRepositoryID,
				StampValue:       "recovery-stamp",
			},
		}, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if err := repo.CreateRepository(ctx, &models.Repository{
		ID: "repo-recovery", WorkspaceID: "ws-recovery", Name: "recovery-repo", LocalPath: t.TempDir(), DefaultBranch: "main", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("CreateRepository: %v", err)
	}
	if err := repo.CreateTaskRepository(ctx, &models.TaskRepository{
		ID: taskRepositoryID, TaskID: taskID, RepositoryID: "repo-recovery", BaseBranch: "main", CheckoutBranch: "feature/recovery", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("CreateTaskRepository: %v", err)
	}
}

func recoveryFixtureService(t *testing.T, repo *sqliterepo.Repository, fake *taskLaunchRecoveryServiceFake) *Service {
	t.Helper()
	steps := newMockStepGetter()
	steps.steps["review-recovery"] = &wfmodels.WorkflowStep{ID: "review-recovery", WorkflowID: "wf-recovery", Position: 0, Name: "Review"}
	steps.steps["done-recovery"] = &wfmodels.WorkflowStep{ID: "done-recovery", WorkflowID: "wf-recovery", Position: 1, Name: "Done"}
	svc := createTestService(repo, steps, newMockTaskRepo())
	svc.taskLaunchRecoveryRepo = repo
	svc.taskLaunchRecoveryTasks = fake
	return svc
}

func TestRecoverTaskLaunchRejectsStaleStampBeforeMutation(t *testing.T) {
	repo := setupTestRepo(t)
	seedTaskLaunchRecoveryFixture(t, repo, "task-stale", "task-repo-stale", models.RecoveryActionPickBaseBranch)
	fake := &taskLaunchRecoveryServiceFake{branches: []taskservice.Branch{{Name: "main", Type: "remote"}}}
	svc := recoveryFixtureService(t, repo, fake)

	_, err := svc.RecoverTaskLaunch(context.Background(), &TaskLaunchRecoveryRequest{
		TaskID: "task-stale", TaskRepositoryID: "task-repo-stale", Action: models.RecoveryActionPickBaseBranch,
		BaseBranch: "main", ErrorStamp: "old-stamp",
	})
	if !errors.Is(err, ErrTaskLaunchRecoveryStale) {
		t.Fatalf("RecoverTaskLaunch error = %v, want stale error", err)
	}
	if len(fake.updated) != 0 {
		t.Fatalf("stale recovery updated repositories: %#v", fake.updated)
	}
}

func TestRecoverTaskLaunchRejectsOlderSessionErrorWhenTaskErrorIsNewer(t *testing.T) {
	repo := setupTestRepo(t)
	const taskID = "task-newer-task-error"
	const taskRepositoryID = "task-repo-newer-task-error"
	seedTaskLaunchRecoveryFixture(t, repo, taskID, taskRepositoryID, models.RecoveryActionPickBaseBranch)

	now := time.Now().UTC()
	if err := repo.CreateTaskSession(context.Background(), &models.TaskSession{
		ID:        "session-older-error",
		TaskID:    taskID,
		State:     models.TaskSessionStateFailed,
		StartedAt: now.Add(-time.Hour),
		UpdatedAt: now.Add(-time.Hour),
	}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}
	if err := repo.SetSessionMetadataKey(context.Background(), "session-older-error", models.SessionMetaKeyLastAgentError, models.LastAgentError{
		Message:          "older session launch error",
		OccurredAt:       now.Add(-time.Hour),
		Code:             models.LaunchErrorCategoryBaseBranchMissing,
		RecoveryActions:  []string{models.RecoveryActionPickBaseBranch},
		TaskRepositoryID: taskRepositoryID,
		StampValue:       "older-session-stamp",
	}); err != nil {
		t.Fatalf("SetSessionMetadataKey: %v", err)
	}

	fake := &taskLaunchRecoveryServiceFake{branches: []taskservice.Branch{{Name: "main", Type: "remote"}}}
	svc := recoveryFixtureService(t, repo, fake)
	_, err := svc.RecoverTaskLaunch(context.Background(), &TaskLaunchRecoveryRequest{
		TaskID: taskID, SessionID: "session-older-error", TaskRepositoryID: taskRepositoryID,
		Action: models.RecoveryActionPickBaseBranch, BaseBranch: "main", ErrorStamp: "older-session-stamp",
	})
	if !errors.Is(err, ErrTaskLaunchRecoveryStale) {
		t.Fatalf("RecoverTaskLaunch error = %v, want stale error", err)
	}
	if len(fake.updated) != 0 {
		t.Fatalf("older session recovery updated repositories: %#v", fake.updated)
	}
}

func TestRecoverTaskLaunchRejectsForeignTaskRepository(t *testing.T) {
	repo := setupTestRepo(t)
	seedTaskLaunchRecoveryFixture(t, repo, "task-foreign-row", "task-repo-owned", models.RecoveryActionPickBaseBranch)
	fake := &taskLaunchRecoveryServiceFake{}
	svc := recoveryFixtureService(t, repo, fake)

	_, err := svc.RecoverTaskLaunch(context.Background(), &TaskLaunchRecoveryRequest{
		TaskID: "task-foreign-row", TaskRepositoryID: "task-repo-foreign", Action: models.RecoveryActionPickBaseBranch,
		BaseBranch: "main", ErrorStamp: "recovery-stamp",
	})
	if err == nil {
		t.Fatal("foreign task repository was accepted")
	}
	if len(fake.updated) != 0 {
		t.Fatalf("foreign repository recovery updated repositories: %#v", fake.updated)
	}
}

func TestRecoverTaskLaunchPickBranchValidatesBeforeWriting(t *testing.T) {
	repo := setupTestRepo(t)
	seedTaskLaunchRecoveryFixture(t, repo, "task-invalid-branch", "task-repo-invalid-branch", models.RecoveryActionPickBaseBranch)
	fake := &taskLaunchRecoveryServiceFake{branches: []taskservice.Branch{{Name: "main", Type: "remote"}}}
	svc := recoveryFixtureService(t, repo, fake)

	_, err := svc.RecoverTaskLaunch(context.Background(), &TaskLaunchRecoveryRequest{
		TaskID: "task-invalid-branch", TaskRepositoryID: "task-repo-invalid-branch", Action: models.RecoveryActionPickBaseBranch,
		BaseBranch: "does-not-exist", ErrorStamp: "recovery-stamp",
	})
	if !errors.Is(err, worktree.ErrInvalidBaseBranch) {
		t.Fatalf("invalid branch error = %v, want ErrInvalidBaseBranch", err)
	}
	if len(fake.updated) != 0 {
		t.Fatalf("invalid branch updated repositories: %#v", fake.updated)
	}
}

func TestValidateSelectedRecoveryBranchRejectsLocalOnlyBranch(t *testing.T) {
	repo := setupTestRepo(t)
	fake := &taskLaunchRecoveryServiceFake{branches: []taskservice.Branch{{Name: "main", Type: "local"}}}
	svc := recoveryFixtureService(t, repo, fake)

	err := svc.validateSelectedRecoveryBranch(context.Background(), &models.Repository{ID: "repo-recovery", LocalPath: t.TempDir()}, "main")
	if !errors.Is(err, worktree.ErrInvalidBaseBranch) {
		t.Fatalf("validateSelectedRecoveryBranch(local) = %v, want ErrInvalidBaseBranch", err)
	}
}

func TestRecoverTaskLaunchRecordsUnresolvedDefaultAndDoesNotLaunch(t *testing.T) {
	repo := setupTestRepo(t)
	seedTaskLaunchRecoveryFixture(t, repo, "task-default-unresolved", "task-repo-default-unresolved", models.RecoveryActionRetryDefault)
	fake := &taskLaunchRecoveryServiceFake{}
	svc := recoveryFixtureService(t, repo, fake)
	svc.taskLaunchRecoveryWorktree = taskLaunchRecoveryWorktreeFake{
		err: fmt.Errorf("token=ghp_abcdefghijklmnopqrstuvwxyz1234567890AB: %w", worktree.ErrRemoteDefaultUnresolved),
	}

	_, err := svc.RecoverTaskLaunch(context.Background(), &TaskLaunchRecoveryRequest{
		TaskID: "task-default-unresolved", TaskRepositoryID: "task-repo-default-unresolved", Action: models.RecoveryActionRetryDefault,
		ErrorStamp: "recovery-stamp",
	})
	if !errors.Is(err, worktree.ErrRemoteDefaultUnresolved) {
		t.Fatalf("unresolved default error = %v, want unresolved error", err)
	}
	if len(fake.updated) != 0 || len(fake.moveTaskIDs) != 0 {
		t.Fatalf("unresolved default mutated recovery collaborators: updated=%#v moved=%#v", fake.updated, fake.moveTaskIDs)
	}
	task, err := repo.GetTask(context.Background(), "task-default-unresolved")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	launchError, ok := models.LoadTaskLaunchError(task.Metadata)
	if !ok || launchError.Code != models.LaunchErrorCategoryDefaultBranchUnresolved || !containsRecoveryAction(launchError.RecoveryActions, models.RecoveryActionPickBaseBranch) {
		t.Fatalf("stored launch error = %#v, want unresolved default with branch picker", launchError)
	}
	if launchError.Details == "" || strings.Contains(launchError.Details, "abcdefghijklmnopqrstuvwxyz") {
		t.Fatalf("task launch details were not sanitized: %q", launchError.Details)
	}
}

func TestRecoverTaskLaunchDoesNotOverwriteFailureDuringBlockedDefaultResolution(t *testing.T) {
	repo := setupTestRepo(t)
	const taskID = "task-default-unresolved-stale"
	const taskRepositoryID = "task-repo-default-unresolved-stale"
	seedTaskLaunchRecoveryFixture(t, repo, taskID, taskRepositoryID, models.RecoveryActionRetryDefault)
	fake := &taskLaunchRecoveryServiceFake{}
	svc := recoveryFixtureService(t, repo, fake)
	started := make(chan struct{})
	release := make(chan struct{})
	svc.taskLaunchRecoveryWorktree = blockingTaskLaunchRecoveryWorktree{
		started: started,
		release: release,
		err:     worktree.ErrRemoteDefaultUnresolved,
	}

	result := make(chan error, 1)
	go func() {
		_, err := svc.RecoverTaskLaunch(context.Background(), &TaskLaunchRecoveryRequest{
			TaskID: taskID, TaskRepositoryID: taskRepositoryID,
			Action: models.RecoveryActionRetryDefault, ErrorStamp: "recovery-stamp",
		})
		result <- err
	}()
	<-started
	if err := repo.SetTaskMetadataKey(context.Background(), taskID, models.MetaKeyLastLaunchError, models.TaskLaunchError{
		Message:          "new provider failure",
		OccurredAt:       time.Now().UTC(),
		Code:             models.LaunchErrorCategoryGenericLaunchFailure,
		RecoveryActions:  []string{models.RecoveryActionRetryDefault},
		TaskRepositoryID: taskRepositoryID,
		StampValue:       "newer-failure-stamp",
	}); err != nil {
		t.Fatalf("store newer failure: %v", err)
	}
	close(release)

	if err := <-result; !errors.Is(err, ErrTaskLaunchRecoveryStale) {
		t.Fatalf("blocked recovery error = %v, want stale error", err)
	}
	task, err := repo.GetTask(context.Background(), taskID)
	if err != nil {
		t.Fatalf("load task after stale recovery: %v", err)
	}
	launchError, ok := models.LoadTaskLaunchError(task.Metadata)
	if !ok || launchError.Stamp() != "newer-failure-stamp" {
		t.Fatalf("task launch error = %#v, want newer failure", launchError)
	}
}

func TestRecoverTaskLaunchRecordsUnresolvedDefaultForSessionSource(t *testing.T) {
	repo := setupTestRepo(t)
	const (
		taskID           = "task-session-default-unresolved"
		taskRepositoryID = "task-repo-session-default-unresolved"
		sessionID        = "session-default-unresolved"
	)
	seedTaskLaunchRecoveryFixture(t, repo, taskID, taskRepositoryID, models.RecoveryActionRetryDefault)
	if removed, err := repo.RemoveTaskMetadataKeyIfStamp(context.Background(), taskID, models.MetaKeyLastLaunchError, "recovery-stamp"); err != nil || !removed {
		t.Fatalf("remove task launch error: removed=%v err=%v", removed, err)
	}
	if err := repo.CreateTaskSession(context.Background(), &models.TaskSession{
		ID: sessionID, TaskID: taskID, State: models.TaskSessionStateFailed,
		Metadata: map[string]interface{}{
			models.SessionMetaKeyLastAgentError: models.LastAgentError{
				Message:          "session launch failed",
				OccurredAt:       time.Now().UTC(),
				Code:             models.LaunchErrorCategoryBaseBranchMissing,
				RecoveryActions:  []string{models.RecoveryActionRetryDefault},
				TaskRepositoryID: taskRepositoryID,
				StampValue:       "session-recovery-stamp",
			},
		},
	}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}

	fake := &taskLaunchRecoveryServiceFake{}
	svc := recoveryFixtureService(t, repo, fake)
	svc.taskLaunchRecoveryWorktree = taskLaunchRecoveryWorktreeFake{
		err: fmt.Errorf("token=ghp_abcdefghijklmnopqrstuvwxyz1234567890AB: %w", worktree.ErrRemoteDefaultUnresolved),
	}
	_, err := svc.RecoverTaskLaunch(context.Background(), &TaskLaunchRecoveryRequest{
		TaskID: taskID, SessionID: sessionID, TaskRepositoryID: taskRepositoryID,
		Action: models.RecoveryActionRetryDefault, ErrorStamp: "session-recovery-stamp",
	})
	if !errors.Is(err, worktree.ErrRemoteDefaultUnresolved) {
		t.Fatalf("session unresolved default error = %v, want unresolved error", err)
	}
	updated, err := repo.GetTaskSession(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("GetTaskSession: %v", err)
	}
	lastError, ok := models.LoadLastAgentError(updated.Metadata)
	if !ok || lastError.Code != models.LaunchErrorCategoryDefaultBranchUnresolved {
		t.Fatalf("stored session launch error = %#v, want unresolved default", lastError)
	}
	if containsRecoveryAction(lastError.RecoveryActions, models.RecoveryActionRetryDefault) {
		t.Fatal("unresolved session error still offers retry_default")
	}
	if !containsRecoveryAction(lastError.RecoveryActions, models.RecoveryActionPickBaseBranch) {
		t.Fatal("unresolved session error does not offer branch selection")
	}
	if len(lastError.Details) == 0 || lastError.Details == "token=ghp_abcdefghijklmnopqrstuvwxyz1234567890AB" {
		t.Fatalf("session launch details were not sanitized: %q", lastError.Details)
	}
}

func TestRecoverTaskLaunchMarksReviewDoneAfterTerminalPRRecheck(t *testing.T) {
	repo := setupTestRepo(t)
	seedTaskLaunchRecoveryFixture(t, repo, "task-mark-done", "task-repo-mark-done", models.RecoveryActionMarkReviewDone)
	fake := &taskLaunchRecoveryServiceFake{}
	svc := recoveryFixtureService(t, repo, fake)
	svc.SetGitHubService(&mockGitHubService{taskPRs: []*github.TaskPR{{TaskID: "task-mark-done", RepositoryID: "repo-recovery", PRNumber: 42, State: githubPRStateMerged}}})
	if err := repo.UpdateTaskRepository(context.Background(), &models.TaskRepository{
		ID: "task-repo-mark-done", TaskID: "task-mark-done", RepositoryID: "repo-recovery", BaseBranch: "main", CheckoutBranch: "feature/recovery",
		Metadata: map[string]interface{}{"pr_number": float64(42)},
	}); err != nil {
		t.Fatalf("UpdateTaskRepository: %v", err)
	}

	response, err := svc.RecoverTaskLaunch(context.Background(), &TaskLaunchRecoveryRequest{
		TaskID: "task-mark-done", Action: models.RecoveryActionMarkReviewDone, ErrorStamp: "recovery-stamp",
	})
	if err != nil {
		t.Fatalf("RecoverTaskLaunch: %v", err)
	}
	if !response.OK || response.TaskID != "task-mark-done" {
		t.Fatalf("response = %#v, want successful task response", response)
	}
	if len(fake.moveTaskIDs) != 1 || fake.moveTaskIDs[0] != "task-mark-done" {
		t.Fatalf("move calls = %#v, want one terminal move", fake.moveTaskIDs)
	}
	if len(fake.moved) != 1 || !fake.moved[0].AllowFailedToCompletedRecovery {
		t.Fatalf("move options = %#v, want recovery-only completion", fake.moved)
	}
	task, err := repo.GetTask(context.Background(), "task-mark-done")
	if err != nil {
		t.Fatalf("GetTask after mark done: %v", err)
	}
	if _, ok := models.LoadTaskLaunchError(task.Metadata); ok {
		t.Fatal("mark_review_done did not clear the stamped task error")
	}
}

func TestRecoverTaskLaunchRejectsOpenPRWithoutMutation(t *testing.T) {
	repo := setupTestRepo(t)
	seedTaskLaunchRecoveryFixture(t, repo, "task-open-pr", "task-repo-open-pr", models.RecoveryActionMarkReviewDone)
	fake := &taskLaunchRecoveryServiceFake{}
	svc := recoveryFixtureService(t, repo, fake)
	svc.SetGitHubService(&mockGitHubService{taskPRs: []*github.TaskPR{{TaskID: "task-open-pr", RepositoryID: "repo-recovery", PRNumber: 42, State: githubPRStateOpen}}})
	if err := repo.UpdateTaskRepository(context.Background(), &models.TaskRepository{
		ID: "task-repo-open-pr", TaskID: "task-open-pr", RepositoryID: "repo-recovery", BaseBranch: "main", CheckoutBranch: "feature/recovery",
		Metadata: map[string]interface{}{"pr_number": float64(42)},
	}); err != nil {
		t.Fatalf("UpdateTaskRepository: %v", err)
	}

	_, err := svc.RecoverTaskLaunch(context.Background(), &TaskLaunchRecoveryRequest{
		TaskID: "task-open-pr", Action: models.RecoveryActionMarkReviewDone, ErrorStamp: "recovery-stamp",
	})
	if err == nil {
		t.Fatal("mark_review_done accepted an open PR")
	}
	if len(fake.moveTaskIDs) != 0 {
		t.Fatalf("open PR caused a move: %#v", fake.moveTaskIDs)
	}
}
