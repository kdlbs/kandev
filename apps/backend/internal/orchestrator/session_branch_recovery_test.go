package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/worktree"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

func TestBranchRecoveryErrorPreservesTypedCauseAndDetails(t *testing.T) {
	cause := fmt.Errorf("restore worktree: %w", &worktree.BranchUnrecoverableError{Branch: "feature/lost"})
	service := &Service{}

	err := service.branchRecoveryError(context.Background(), "task-1", "session-1", cause)

	var recoveryErr *BranchRecoveryError
	require.ErrorAs(t, err, &recoveryErr)
	require.ErrorIs(t, err, worktree.ErrBranchUnrecoverable)
	require.Equal(t, "session-1", recoveryErr.SessionID)
	require.Equal(t, "feature/lost", recoveryErr.OriginalBranch)
	require.Equal(t, map[string]interface{}{
		"kind":            "branch_unrecoverable",
		"recovery_action": "resume_new_branch",
		"original_branch": "feature/lost",
		"base_branch":     "",
		"repository_id":   "",
		"session_id":      "session-1",
	}, recoveryErr.Details())
	require.Contains(t, recoveryErr.Error(), "continue on a new branch")
}

func TestPersistBranchRecoveryWarningsIsIdempotent(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-branch-recovery", "session-branch-recovery", "step1")
	now := time.Now().UTC()
	require.NoError(t, repo.CreateRepository(ctx, &models.Repository{
		ID: "repo-branch-recovery", WorkspaceID: "ws1", Name: "backend", DefaultBranch: "main",
		CreatedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, repo.CreateTaskRepository(ctx, &models.TaskRepository{
		ID: "task-repo-branch-recovery", TaskID: "task-branch-recovery", RepositoryID: "repo-branch-recovery",
		BaseBranch: "main", Position: 0, CreatedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: "env-branch-recovery", TaskID: "task-branch-recovery",
		ExecutorType: string(models.ExecutorTypeWorktree), WorkspacePath: t.TempDir(), Status: models.TaskEnvironmentStatusReady,
		Repos: []*models.TaskEnvironmentRepo{{
			ID: "env-repo-branch-recovery", RepositoryID: "repo-branch-recovery", BranchSlug: "task-branch-recovery",
			WorktreeID: "worktree-branch-recovery", WorktreeBranch: "feature/lost", Position: 0,
			CreatedAt: now, UpdatedAt: now,
		}},
	}))
	session, err := repo.GetTaskSession(ctx, "session-branch-recovery")
	require.NoError(t, err)
	session.TaskEnvironmentID = "env-branch-recovery"
	require.NoError(t, repo.UpdateTaskSession(ctx, session))

	service := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	messages := &mockMessageCreator{}
	service.messageCreator = messages
	before, err := service.captureBranchRecoverySnapshot(ctx, "task-branch-recovery", "session-branch-recovery")
	require.NoError(t, err)

	row, err := repo.ListTaskEnvironmentRepos(ctx, "env-branch-recovery")
	require.NoError(t, err)
	require.Len(t, row, 1)
	row[0].WorktreeBranch = "feature/recreated"
	require.NoError(t, repo.UpdateTaskEnvironmentRepo(ctx, row[0]))

	service.persistBranchRecoveryWarnings(ctx, "task-branch-recovery", "session-branch-recovery", before)
	service.persistBranchRecoveryWarnings(ctx, "task-branch-recovery", "session-branch-recovery", before)

	require.Len(t, messages.sessionMessages, 1)
	warning := messages.sessionMessages[0]
	require.Equal(t, "branch_recreated", warning.content)
	require.Equal(t, string(v1.MessageTypeStatus), warning.messageType)
	require.Equal(t, "warning", warning.metadata["variant"])
	require.Equal(t, "branch_recreated", warning.metadata["kind"])
	require.Equal(t, "feature/lost", warning.metadata["original_branch"])
	require.Equal(t, "feature/recreated", warning.metadata["new_branch"])
	require.Equal(t, "main", warning.metadata["base_branch"])
	require.Equal(t, "session-branch-recovery", warning.metadata["session_id"])
	require.Equal(t, "repo-branch-recovery", warning.metadata["repository_id"])
}

func TestPersistBranchRecoveryWarningsReclaimsStaleClaim(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-stale-claim", "session-stale-claim", "step1")
	now := time.Now().UTC()
	require.NoError(t, repo.CreateRepository(ctx, &models.Repository{
		ID: "repo-stale-claim", WorkspaceID: "ws1", Name: "backend", DefaultBranch: "main",
		CreatedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, repo.CreateTaskRepository(ctx, &models.TaskRepository{
		ID: "task-repo-stale-claim", TaskID: "task-stale-claim", RepositoryID: "repo-stale-claim",
		BaseBranch: "main", Position: 0, CreatedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: "env-stale-claim", TaskID: "task-stale-claim",
		ExecutorType: string(models.ExecutorTypeWorktree), WorkspacePath: t.TempDir(), Status: models.TaskEnvironmentStatusReady,
		Repos: []*models.TaskEnvironmentRepo{{
			ID: "env-repo-stale-claim", RepositoryID: "repo-stale-claim", BranchSlug: "task-stale-claim",
			WorktreeID: "worktree-stale-claim", WorktreeBranch: "feature/lost", Position: 0,
			CreatedAt: now, UpdatedAt: now,
		}},
	}))
	session, err := repo.GetTaskSession(ctx, "session-stale-claim")
	require.NoError(t, err)
	session.TaskEnvironmentID = "env-stale-claim"
	require.NoError(t, repo.UpdateTaskSession(ctx, session))

	decisionID := branchRecoveryDecisionID(
		"task-stale-claim", "session-stale-claim", "repo-stale-claim", "feature/lost", "feature/recreated", "main",
	)
	claimed, err := repo.SetSessionMetadataKeyIfAbsent(
		ctx,
		"session-stale-claim",
		branchRecoveryWarningKeyPrefix+decisionID,
		branchRecoveryWarningClaim{ClaimedAt: time.Now().Add(-(branchRecoveryWarningClaimStaleAfter + time.Minute))},
	)
	require.NoError(t, err)
	require.True(t, claimed)

	service := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	messages := &mockMessageCreator{}
	service.messageCreator = messages
	before, err := service.captureBranchRecoverySnapshot(ctx, "task-stale-claim", "session-stale-claim")
	require.NoError(t, err)

	row, err := repo.ListTaskEnvironmentRepos(ctx, "env-stale-claim")
	require.NoError(t, err)
	require.Len(t, row, 1)
	row[0].WorktreeBranch = "feature/recreated"
	require.NoError(t, repo.UpdateTaskEnvironmentRepo(ctx, row[0]))

	service.persistBranchRecoveryWarnings(ctx, "task-stale-claim", "session-stale-claim", before)

	require.Len(t, messages.sessionMessages, 1)
	require.Equal(t, "branch_recreated", messages.sessionMessages[0].metadata["kind"])
}

func TestPersistBranchRecoveryWarningReleasesClaimWhenMessageWriteFails(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-warning-write-failure", "session-warning-write-failure", "step1")
	writeErr := errors.New("message write failed")
	messages := &mockMessageCreator{sessionMessageErr: writeErr}
	service := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	service.messageCreator = messages
	previous := branchRecoveryRepoSnapshot{
		RepositoryID:   "repo-warning-write-failure",
		BranchSlug:     "primary",
		WorktreeBranch: "feature/lost",
		BaseBranch:     "main",
	}
	current := previous
	current.WorktreeBranch = "feature/recreated"

	service.persistBranchRecoveryWarning(
		ctx,
		"task-warning-write-failure",
		"session-warning-write-failure",
		previous,
		current,
	)
	if len(messages.sessionMessages) != 0 {
		t.Fatalf("failed message write was recorded as a success: %+v", messages.sessionMessages)
	}

	messages.sessionMessageErr = nil
	service.persistBranchRecoveryWarning(
		ctx,
		"task-warning-write-failure",
		"session-warning-write-failure",
		previous,
		current,
	)
	if len(messages.sessionMessages) != 1 {
		t.Fatalf("warning messages = %d, want one retry after the initial write failure", len(messages.sessionMessages))
	}
	if messages.sessionMessageAttempts != 2 {
		t.Fatalf("message write attempts = %d, want one failed attempt and one retry", messages.sessionMessageAttempts)
	}
}

func TestBranchRecoveryErrorLeavesOrdinaryErrorsUntouched(t *testing.T) {
	cause := errors.New("ordinary resume failure")
	service := &Service{}

	require.Same(t, cause, service.branchRecoveryError(context.Background(), "task-1", "session-1", cause))
}
