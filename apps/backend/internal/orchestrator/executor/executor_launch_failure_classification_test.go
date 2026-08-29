package executor

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/worktree"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestClassifyLaunchFailureUsesTypedBaseBranchCategory(t *testing.T) {
	classification := classifyLaunchFailure(errors.Join(
		errors.New("environment preparation failed"), worktree.ErrInvalidBaseBranch,
	))
	if classification.code != models.LaunchErrorCategoryBaseBranchMissing {
		t.Fatalf("classification code = %q, want %q", classification.code, models.LaunchErrorCategoryBaseBranchMissing)
	}
	if classification.message == "" || classification.message == "environment preparation failed" {
		t.Fatalf("classification message = %q, want safe user message", classification.message)
	}
}

func TestWorktreeRecoveryFailureIsActionableWithoutRetryActions(t *testing.T) {
	repo := newMockRepository()
	repo.sessions["session-1"] = &models.TaskSession{
		ID: "session-1", TaskID: "task-1", State: models.TaskSessionStateCreated,
	}
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	recoveryErr := &worktree.WorktreeRecoveryError{
		TaskID: "task-1", Checkout: "/tasks/task-1/repo",
		Reason: `linked-worktree admin target "/repos/main/.git/worktrees/task-1" is missing`,
	}

	_, changed := exec.transitionLaunchFailure(
		context.Background(), "task-1", "session-1", "repo-1", "task-repo-1", recoveryErr,
	)
	if !changed {
		t.Fatal("transitionLaunchFailure did not transition the session")
	}
	session, err := repo.GetTaskSession(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("GetTaskSession: %v", err)
	}
	errorValue, found := models.LoadLastAgentError(session.Metadata)
	if !found {
		t.Fatal("typed launch error was not persisted")
	}
	if errorValue.Code != models.LaunchErrorCategoryGenericLaunchFailure {
		t.Fatalf("error code = %q, want generic launch failure", errorValue.Code)
	}
	if errorValue.Message == "" || !strings.Contains(errorValue.Message, recoveryErr.Checkout) || !strings.Contains(errorValue.Message, "task-1") {
		t.Fatalf("error message = %q, want actionable task and checkout details", errorValue.Message)
	}
	if len(errorValue.RecoveryActions) != 0 {
		t.Fatalf("recovery actions = %#v, want no retry actions", errorValue.RecoveryActions)
	}
}

func TestPrepareSessionBlocksWorktreeRecoveryBeforePersistingSession(t *testing.T) {
	repo := newMockRepository()
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	admission, ok := any(exec).(interface {
		SetWorktreeRecoveryAdmission(WorktreeRecoveryAdmissionFunc)
	})
	if !ok {
		t.Fatal("executor has no worktree recovery admission seam")
	}

	recoveryErr := &worktree.WorktreeRecoveryError{
		TaskID: "task-1", Checkout: "/tasks/task-1/repo", Reason: "recovery is active",
	}
	checks := 0
	admission.SetWorktreeRecoveryAdmission(WorktreeRecoveryAdmissionFunc(func(_ context.Context, taskID string) error {
		checks++
		if taskID != "task-1" {
			t.Fatalf("admission task ID = %q, want task-1", taskID)
		}
		return recoveryErr
	}))

	_, err := exec.PrepareSession(context.Background(), &v1.Task{ID: "task-1"}, "profile-1", "", "", "")
	if !errors.Is(err, worktree.ErrWorktreeCorrupted) {
		t.Fatalf("PrepareSession() error = %v, want worktree recovery error", err)
	}
	if checks != 1 {
		t.Fatalf("admission checks = %d, want 1", checks)
	}
	if len(repo.createTaskSessionCalls) != 0 {
		t.Fatalf("CreateTaskSession calls = %d, want 0", len(repo.createTaskSessionCalls))
	}
}

func TestTransitionLaunchFailurePersistsTypedErrorAndExactTaskRepository(t *testing.T) {
	repo := newMockRepository()
	repo.sessions["session-1"] = &models.TaskSession{
		ID: "session-1", TaskID: "task-1", State: models.TaskSessionStateCreated,
	}
	exec := newTestExecutor(t, &mockAgentManager{}, repo)

	_, changed := exec.transitionLaunchFailure(
		context.Background(), "task-1", "session-1", "repo-1", "task-repo-1",
		errors.Join(errors.New("prepare failed"), worktree.ErrInvalidBaseBranch),
	)
	if !changed {
		t.Fatal("transitionLaunchFailure did not transition the session")
	}
	session, err := repo.GetTaskSession(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("GetTaskSession: %v", err)
	}
	errorValue, found := models.LoadLastAgentError(session.Metadata)
	if !found {
		t.Fatal("typed launch error was not persisted")
	}
	if errorValue.Code != models.LaunchErrorCategoryBaseBranchMissing {
		t.Fatalf("error code = %q, want %q", errorValue.Code, models.LaunchErrorCategoryBaseBranchMissing)
	}
	if errorValue.TaskRepositoryID != "task-repo-1" {
		t.Fatalf("task repository id = %q, want task-repo-1", errorValue.TaskRepositoryID)
	}
	if len(errorValue.RecoveryActions) != 2 ||
		errorValue.RecoveryActions[0] != models.RecoveryActionRetryDefault ||
		errorValue.RecoveryActions[1] != models.RecoveryActionPickBaseBranch {
		t.Fatalf("recovery actions = %#v, want default retry and branch picker", errorValue.RecoveryActions)
	}
}

func TestLaunchFailureReviewActionRequiresSuccessfulEligibilityResolver(t *testing.T) {
	exec := &Executor{}
	exec.launchFailureReviewEligibility = func(context.Context, string) (bool, error) {
		return true, nil
	}
	errorValue := exec.buildLastAgentError(context.Background(), "task-1", "", errors.New("start failed"))
	if len(errorValue.RecoveryActions) != 1 || errorValue.RecoveryActions[0] != models.RecoveryActionMarkReviewDone {
		t.Fatalf("eligible recovery actions = %#v, want mark_review_done", errorValue.RecoveryActions)
	}

	exec.launchFailureReviewEligibility = func(context.Context, string) (bool, error) {
		return false, errors.New("lookup failed")
	}
	errorValue = exec.buildLastAgentError(context.Background(), "task-1", "", errors.New("start failed"))
	if len(errorValue.RecoveryActions) != 0 {
		t.Fatalf("failed eligibility lookup exposed recovery actions = %#v", errorValue.RecoveryActions)
	}
}

func TestBuildLastAgentErrorSanitizesRepositoryPreparationDetails(t *testing.T) {
	exec := &Executor{}
	launchErr := &lifecycle.RepositoryPreparationError{
		RepositoryID:   "repo-back",
		RepositoryName: "backend",
		Cause:          errors.New("fatal: https://user:ghp_abcdefghijklmnopqrstuvwxyz1234567890AB@example.com/repo.git"),
	}

	errorValue := exec.buildLastAgentError(context.Background(), "task-1", "task-repo-2", launchErr)
	if !strings.Contains(errorValue.Details, "repo-back") || !strings.Contains(errorValue.Details, "backend") {
		t.Fatalf("launch details = %q, want repository identity", errorValue.Details)
	}
	if strings.Contains(errorValue.Details, "ghp_abcdefghijklmnopqrstuvwxyz1234567890AB") ||
		strings.Contains(errorValue.Details, "user:") {
		t.Fatalf("launch details exposed credential-bearing URL: %q", errorValue.Details)
	}
}
