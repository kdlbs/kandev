package executor

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/worktree"
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
