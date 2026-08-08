package orchestrator

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	agentctlclient "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/require"
)

type titleBranchRuntimeStub struct {
	calls        []titleBranchRuntimeCall
	primaryCalls []bool
	err          error
}

type titleBranchRuntimeCall struct {
	sessionID string
	newName   string
	repo      string
}

type failingTitleBranchSnapshotStore struct {
	*sqliterepo.Repository
	err error
}

func (s *failingTitleBranchSnapshotStore) UpdateTaskSessionWorktreeBranchByWorktree(context.Context, string, string, string) error {
	return s.err
}

func (s *failingTitleBranchSnapshotStore) UpdateTaskEnvironmentRepo(context.Context, *models.TaskEnvironmentRepo) error {
	return s.err
}

func (s *titleBranchRuntimeStub) RenameBranchForSession(_ context.Context, sessionID, newName, repo string) (*agentctlclient.GitOperationResult, error) {
	s.calls = append(s.calls, titleBranchRuntimeCall{sessionID: sessionID, newName: newName, repo: repo})
	if s.err != nil {
		return nil, s.err
	}
	return &agentctlclient.GitOperationResult{Success: true, Operation: "rename_branch"}, nil
}

func (s *titleBranchRuntimeStub) RenameBranchForSessionWithPrimary(ctx context.Context, sessionID, newName, repo string, primary bool) (*agentctlclient.GitOperationResult, error) {
	s.primaryCalls = append(s.primaryCalls, primary)
	return s.RenameBranchForSession(ctx, sessionID, newName, repo)
}

func TestRenameGeneratedBranchesForTaskTitleUsesFinalTitleAndPersistsSnapshots(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-title-branch", "session-title-branch", "step1")
	taskRoot := t.TempDir()
	now := time.Now().UTC()
	require.NoError(t, repo.CreateRepository(ctx, &models.Repository{
		ID: "repo-title-branch", WorkspaceID: "ws1", Name: "backend", DefaultBranch: "main",
		WorktreeBranchTemplate: "feature/{title}", CreatedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, repo.CreateTaskRepository(ctx, &models.TaskRepository{
		ID: "task-repo-title-branch", TaskID: "task-title-branch", RepositoryID: "repo-title-branch",
		BaseBranch: "main", Position: 0,
	}))
	require.NoError(t, repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: "env-title-branch", TaskID: "task-title-branch",
		ExecutorType: string(models.ExecutorTypeWorktree), WorkspacePath: taskRoot,
		Status: models.TaskEnvironmentStatusReady,
		Repos: []*models.TaskEnvironmentRepo{{
			ID: "env-repo-title-branch", RepositoryID: "repo-title-branch", BranchSlug: "main",
			WorktreeID: "wt-title-branch", WorktreePath: filepath.Join(taskRoot, "backend"),
			WorktreeBranch: "feature/provisional", Position: 0,
		}},
	}))
	session, err := repo.GetTaskSession(ctx, "session-title-branch")
	require.NoError(t, err)
	session.TaskEnvironmentID = "env-title-branch"
	require.NoError(t, repo.UpdateTaskSession(ctx, session))

	service := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	runtime := &titleBranchRuntimeStub{}
	service.SetTitleBranchRuntime(runtime)

	result, err := service.RenameGeneratedBranchesForTaskTitle(ctx, "task-title-branch", "session-title-branch", "Final title")
	require.NoError(t, err)
	require.Equal(t, TitleBranchStatusRenamed, result.Status)
	require.Len(t, result.Renamed, 1)
	require.Equal(t, "feature/final-title", result.Renamed[0].To)
	require.Equal(t, []titleBranchRuntimeCall{{
		sessionID: "session-title-branch", newName: "feature/final-title", repo: "",
	}}, runtime.calls)
	require.Equal(t, []bool{true}, runtime.primaryCalls)

	worktrees, err := repo.ListTaskSessionWorktrees(ctx, "session-title-branch")
	require.NoError(t, err)
	require.Equal(t, "feature/final-title", worktrees[0].WorktreeBranch)
	env, err := repo.GetTaskEnvironment(ctx, "env-title-branch")
	require.NoError(t, err)
	require.Equal(t, "feature/final-title", env.Repos[0].WorktreeBranch)
}

func TestRenameGeneratedBranchesForTaskTitleUsesStableWorktreeSuffix(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-title-stable-suffix", "session-title-stable-suffix", "step1")
	taskRoot := t.TempDir()
	now := time.Now().UTC()
	require.NoError(t, repo.CreateRepository(ctx, &models.Repository{
		ID: "repo-title-stable-suffix", WorkspaceID: "ws1", Name: "backend", DefaultBranch: "main",
		WorktreeBranchTemplate: "feature/{title}-{suffix}", CreatedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, repo.CreateTaskRepository(ctx, &models.TaskRepository{
		ID: "task-repo-title-stable-suffix", TaskID: "task-title-stable-suffix", RepositoryID: "repo-title-stable-suffix",
		BaseBranch: "main", Position: 0,
	}))
	require.NoError(t, repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: "env-title-stable-suffix", TaskID: "task-title-stable-suffix",
		ExecutorType: string(models.ExecutorTypeWorktree), WorkspacePath: taskRoot, Status: models.TaskEnvironmentStatusReady,
		Repos: []*models.TaskEnvironmentRepo{{
			ID: "env-repo-title-stable-suffix", RepositoryID: "repo-title-stable-suffix", BranchSlug: "main",
			WorktreeID: "wt-title-stable-suffix", WorktreePath: filepath.Join(taskRoot, "backend"),
			WorktreeBranch: "feature/provisional-abc", Position: 0,
		}},
	}))
	session, err := repo.GetTaskSession(ctx, "session-title-stable-suffix")
	require.NoError(t, err)
	session.TaskEnvironmentID = "env-title-stable-suffix"
	require.NoError(t, repo.UpdateTaskSession(ctx, session))

	service := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	runtime := &titleBranchRuntimeStub{}
	service.SetTitleBranchRuntime(runtime)
	first, err := service.RenameGeneratedBranchesForTaskTitle(ctx, "task-title-stable-suffix", "session-title-stable-suffix", "Final title")
	require.NoError(t, err)
	require.Len(t, first.Renamed, 1)
	second, err := service.RenameGeneratedBranchesForTaskTitle(ctx, "task-title-stable-suffix", "session-title-stable-suffix", "Final title")
	require.NoError(t, err)
	require.Len(t, second.Renamed, 1)
	require.Equal(t, first.Renamed[0].To, second.Renamed[0].To)
	require.Len(t, runtime.calls, 1, "the second identical handoff should be idempotent")
}

func TestRenameGeneratedBranchesForTaskTitlePreservesManuallySelectedBranch(t *testing.T) {
	runtime := &titleBranchRuntimeStub{}
	service := &Service{titleBranchRuntime: runtime}
	result := TitleBranchRenameResult{}
	service.renameTitleBranchBinding(
		context.Background(),
		&models.Task{ID: "task-manual-branch", Title: "Final title"},
		"Final title",
		models.ExecutorTypeWorktree,
		&models.TaskEnvironment{Repos: []*models.TaskEnvironmentRepo{{WorktreeBranch: "feature/provisional"}}},
		false,
		titleBranchBinding{
			sessionID:       "session-manual-branch",
			taskRepository:  &models.TaskRepository{RepositoryID: "repo-manual-branch"},
			repository:      &models.Repository{ID: "repo-manual-branch", WorktreeBranchTemplate: "feature/{title}"},
			worktree:        &models.TaskEnvironmentRepo{RepositoryID: "repo-manual-branch", WorktreeBranch: "feature/user-selected"},
			environmentRepo: &models.TaskEnvironmentRepo{RepositoryID: "repo-manual-branch", WorktreeBranch: "feature/provisional"},
		},
		&result,
	)
	require.Equal(t, TitleBranchStatusPreserved, aggregateTitleBranchRenameStatus(result.Renamed, result.Preserved, result.Failed))
	require.Equal(t, "switched_branch", result.Preserved[0].Reason)
	require.Empty(t, runtime.calls)
}

func TestRenameGeneratedBranchesForTaskTitlePreservesRemoteCheckout(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-remote-branch", "session-remote-branch", "step1")
	taskRoot := t.TempDir()
	now := time.Now().UTC()
	require.NoError(t, repo.CreateRepository(ctx, &models.Repository{
		ID: "repo-remote-branch", WorkspaceID: "ws1", Name: "backend", DefaultBranch: "main",
		WorktreeBranchTemplate: "feature/{title}", CreatedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, repo.CreateTaskRepository(ctx, &models.TaskRepository{
		ID: "task-repo-remote-branch", TaskID: "task-remote-branch", RepositoryID: "repo-remote-branch",
		BaseBranch: "main", CheckoutBranch: "pr-42", Position: 0,
	}))
	require.NoError(t, repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: "env-remote-branch", TaskID: "task-remote-branch",
		ExecutorType: string(models.ExecutorTypeWorktree), WorkspacePath: taskRoot,
		Status: models.TaskEnvironmentStatusReady,
		Repos: []*models.TaskEnvironmentRepo{{
			ID: "env-repo-remote-branch", RepositoryID: "repo-remote-branch", BranchSlug: "pr-42",
			WorktreeID: "wt-remote-branch", WorktreePath: filepath.Join(taskRoot, "backend"),
			WorktreeBranch: "pr-42", Position: 0,
		}},
	}))
	session, err := repo.GetTaskSession(ctx, "session-remote-branch")
	require.NoError(t, err)
	session.TaskEnvironmentID = "env-remote-branch"
	require.NoError(t, repo.UpdateTaskSession(ctx, session))

	service := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	runtime := &titleBranchRuntimeStub{}
	service.SetTitleBranchRuntime(runtime)

	result, err := service.RenameGeneratedBranchesForTaskTitle(ctx, "task-remote-branch", "session-remote-branch", "Final title")
	require.NoError(t, err)
	require.Equal(t, TitleBranchStatusPreserved, result.Status)
	require.Len(t, result.Preserved, 1)
	require.Equal(t, "remote_checkout", result.Preserved[0].Reason)
	require.Empty(t, runtime.calls)
}

func TestRenameGeneratedBranchesForTaskTitleScopesMixedRepositories(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-mixed-branches", "session-mixed-branches", "step1")
	taskRoot := t.TempDir()
	now := time.Now().UTC()
	for _, repository := range []*models.Repository{
		{ID: "repo-mixed-backend", WorkspaceID: "ws1", Name: "backend", DefaultBranch: "main", WorktreeBranchTemplate: "feature/{title}", CreatedAt: now, UpdatedAt: now},
		{ID: "repo-mixed-frontend", WorkspaceID: "ws1", Name: "frontend", DefaultBranch: "main", WorktreeBranchTemplate: "feature/{title}", CreatedAt: now, UpdatedAt: now},
	} {
		require.NoError(t, repo.CreateRepository(ctx, repository))
	}
	require.NoError(t, repo.CreateTaskRepository(ctx, &models.TaskRepository{
		ID: "task-repo-mixed-backend", TaskID: "task-mixed-branches", RepositoryID: "repo-mixed-backend", BaseBranch: "main", Position: 0,
	}))
	require.NoError(t, repo.CreateTaskRepository(ctx, &models.TaskRepository{
		ID: "task-repo-mixed-frontend", TaskID: "task-mixed-branches", RepositoryID: "repo-mixed-frontend", BaseBranch: "main", CheckoutBranch: "pr-42", Position: 1,
	}))
	require.NoError(t, repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: "env-mixed-branches", TaskID: "task-mixed-branches", ExecutorType: string(models.ExecutorTypeWorktree),
		WorkspacePath: taskRoot, Status: models.TaskEnvironmentStatusReady,
		Repos: []*models.TaskEnvironmentRepo{
			{ID: "env-repo-mixed-backend", RepositoryID: "repo-mixed-backend", BranchSlug: "main", WorktreeID: "wt-mixed-backend", WorktreePath: filepath.Join(taskRoot, "backend"), WorktreeBranch: "feature/provisional-backend", Position: 0},
			{ID: "env-repo-mixed-frontend", RepositoryID: "repo-mixed-frontend", BranchSlug: "pr-42", WorktreeID: "wt-mixed-frontend", WorktreePath: filepath.Join(taskRoot, "frontend"), WorktreeBranch: "pr-42", Position: 1},
		},
	}))
	session, err := repo.GetTaskSession(ctx, "session-mixed-branches")
	require.NoError(t, err)
	session.TaskEnvironmentID = "env-mixed-branches"
	require.NoError(t, repo.UpdateTaskSession(ctx, session))

	service := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	runtime := &titleBranchRuntimeStub{}
	service.SetTitleBranchRuntime(runtime)
	result, err := service.RenameGeneratedBranchesForTaskTitle(ctx, "task-mixed-branches", "session-mixed-branches", "Final title")
	require.NoError(t, err)
	require.Equal(t, TitleBranchStatusRenamed, result.Status)
	require.Len(t, result.Renamed, 1)
	require.Equal(t, "repo-mixed-backend", result.Renamed[0].RepositoryID)
	require.Len(t, result.Preserved, 1)
	require.Equal(t, "repo-mixed-frontend", result.Preserved[0].RepositoryID)
	require.Equal(t, []titleBranchRuntimeCall{{sessionID: "session-mixed-branches", newName: "feature/final-title", repo: "backend"}}, runtime.calls)
	require.Equal(t, []bool{true}, runtime.primaryCalls)
}

func TestRenameGeneratedBranchesForTaskTitleScopesOwnerSession(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-owner-scope", "session-owner-scope", "step1")
	now := time.Now().UTC()
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-non-owner-scope", TaskID: "task-owner-scope", State: models.TaskSessionStateRunning,
		StartedAt: now, UpdatedAt: now,
	}))
	taskRoot := t.TempDir()
	require.NoError(t, repo.CreateRepository(ctx, &models.Repository{
		ID: "repo-owner-scope", WorkspaceID: "ws1", Name: "backend", DefaultBranch: "main",
		WorktreeBranchTemplate: "feature/{title}", CreatedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, repo.CreateTaskRepository(ctx, &models.TaskRepository{
		ID: "task-repo-owner-scope", TaskID: "task-owner-scope", RepositoryID: "repo-owner-scope",
		BaseBranch: "main", Position: 0,
	}))
	require.NoError(t, repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: "env-owner-scope", TaskID: "task-owner-scope",
		ExecutorType: string(models.ExecutorTypeWorktree), WorkspacePath: taskRoot,
		Status: models.TaskEnvironmentStatusReady,
		Repos: []*models.TaskEnvironmentRepo{{
			ID: "env-repo-owner-scope", RepositoryID: "repo-owner-scope", BranchSlug: "main",
			WorktreeID: "worktree-owner-scope", WorktreePath: filepath.Join(taskRoot, "backend"),
			WorktreeBranch: "feature/owner-provisional", Position: 0,
		}},
	}))
	ownerSession, err := repo.GetTaskSession(ctx, "session-owner-scope")
	require.NoError(t, err)
	ownerSession.TaskEnvironmentID = "env-owner-scope"
	require.NoError(t, repo.UpdateTaskSession(ctx, ownerSession))
	nonOwnerSession, err := repo.GetTaskSession(ctx, "session-non-owner-scope")
	require.NoError(t, err)
	nonOwnerSession.TaskEnvironmentID = "env-owner-scope"
	require.NoError(t, repo.UpdateTaskSession(ctx, nonOwnerSession))

	service := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	runtime := &titleBranchRuntimeStub{}
	service.SetTitleBranchRuntime(runtime)
	result, err := service.RenameGeneratedBranchesForTaskTitle(ctx, "task-owner-scope", "session-owner-scope", "Final title")
	require.NoError(t, err)
	require.Equal(t, TitleBranchStatusRenamed, result.Status)
	require.Equal(t, []titleBranchRuntimeCall{{sessionID: "session-owner-scope", newName: "feature/final-title", repo: ""}}, runtime.calls)

	// Both sessions share the task environment, so both observe the renamed
	// branch — the rename is scoped to the environment, not per session.
	ownerWorktrees, err := repo.ListTaskSessionWorktrees(ctx, "session-owner-scope")
	require.NoError(t, err)
	require.Len(t, ownerWorktrees, 1)
	require.Equal(t, "feature/final-title", ownerWorktrees[0].WorktreeBranch)
	nonOwnerWorktrees, err := repo.ListTaskSessionWorktrees(ctx, "session-non-owner-scope")
	require.NoError(t, err)
	require.Len(t, nonOwnerWorktrees, 1)
	require.Equal(t, "feature/final-title", nonOwnerWorktrees[0].WorktreeBranch)
}

func TestRenameGeneratedBranchesForTaskTitlePreservesLocalExecutor(t *testing.T) {
	runtime := &titleBranchRuntimeStub{}
	service := &Service{titleBranchRuntime: runtime}
	result := TitleBranchRenameResult{}
	service.renameTitleBranchBinding(
		context.Background(),
		&models.Task{ID: "task-local", Title: "Final title"},
		"Final title",
		models.ExecutorTypeLocal,
		nil,
		false,
		titleBranchBinding{
			taskRepository: &models.TaskRepository{RepositoryID: "repo-local"},
			repository:     &models.Repository{ID: "repo-local", WorktreeBranchTemplate: "feature/{title}"},
			worktree:       &models.TaskEnvironmentRepo{RepositoryID: "repo-local", WorktreeBranch: "user-branch"},
		},
		&result,
	)
	require.Equal(t, TitleBranchStatusPreserved, aggregateTitleBranchRenameStatus(result.Renamed, result.Preserved, result.Failed))
	require.Equal(t, "local_executor", result.Preserved[0].Reason)
	require.Empty(t, runtime.calls)
}

func TestRenameGeneratedBranchesForTaskTitleReportsGitRenameFailure(t *testing.T) {
	runtime := &titleBranchRuntimeStub{err: errors.New("git branch rename failed")}
	service := &Service{titleBranchRuntime: runtime}
	result := TitleBranchRenameResult{}
	service.renameTitleBranchBinding(
		context.Background(),
		&models.Task{ID: "task-snapshot-failure", Title: "Final title"},
		"Final title",
		models.ExecutorTypeWorktree,
		nil,
		false,
		titleBranchBinding{
			taskRepository:  &models.TaskRepository{RepositoryID: "repo-snapshot-failure", Position: 0},
			repository:      &models.Repository{ID: "repo-snapshot-failure", WorktreeBranchTemplate: "feature/{title}"},
			worktree:        &models.TaskEnvironmentRepo{RepositoryID: "repo-snapshot-failure", WorktreeBranch: "feature/provisional"},
			environmentRepo: &models.TaskEnvironmentRepo{RepositoryID: "repo-snapshot-failure", WorktreeBranch: "feature/provisional"},
		},
		&result,
	)

	require.Equal(t, TitleBranchStatusFailed, aggregateTitleBranchRenameStatus(result.Renamed, result.Preserved, result.Failed))
	require.Len(t, result.Failed, 1)
	require.Equal(t, "repo-snapshot-failure", result.Failed[0].RepositoryID)
	require.Equal(t, "feature/provisional", result.Failed[0].Branch)
	require.Contains(t, result.Failed[0].Message, "git branch rename failed")
}

func TestRenameGeneratedBranchesForTaskTitleReportsSnapshotFailure(t *testing.T) {
	store := &failingTitleBranchSnapshotStore{
		Repository: setupTestRepo(t),
		err:        errors.New("branch snapshot persistence failed"),
	}
	service := &Service{
		repo:               store,
		titleBranchRuntime: &titleBranchRuntimeStub{},
	}
	result := TitleBranchRenameResult{}
	service.renameTitleBranchBinding(
		context.Background(),
		&models.Task{ID: "task-snapshot-failure", Title: "Final title"},
		"Final title",
		models.ExecutorTypeWorktree,
		nil,
		false,
		titleBranchBinding{
			taskRepository:  &models.TaskRepository{RepositoryID: "repo-snapshot-failure", Position: 0},
			repository:      &models.Repository{ID: "repo-snapshot-failure", WorktreeBranchTemplate: "feature/{title}"},
			worktree:        &models.TaskEnvironmentRepo{WorktreeID: "worktree-snapshot-failure", RepositoryID: "repo-snapshot-failure", WorktreeBranch: "feature/provisional"},
			environmentRepo: &models.TaskEnvironmentRepo{RepositoryID: "repo-snapshot-failure", WorktreeBranch: "feature/provisional"},
		},
		&result,
	)

	require.Equal(t, TitleBranchStatusFailed, aggregateTitleBranchRenameStatus(result.Renamed, result.Preserved, result.Failed))
	require.Len(t, result.Failed, 1)
	require.Equal(t, "repo-snapshot-failure", result.Failed[0].RepositoryID)
	require.Equal(t, "feature/final-title", result.Failed[0].Branch)
	require.Contains(t, result.Failed[0].Message, "branch snapshot persistence failed")
}
