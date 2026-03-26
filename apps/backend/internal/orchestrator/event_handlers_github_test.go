package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/task/models"
)

// mockGitHubService implements GitHubService for testing CheckSessionPR.
type mockGitHubService struct {
	client            github.Client
	taskPR            *github.TaskPR
	taskPRErr         error
	ensureWatchCalls  int
	createWatchCalls  int
	associateCalls    int
	ensureWatchBranch string
	createWatchBranch string
}

func (m *mockGitHubService) Client() github.Client { return m.client }
func (m *mockGitHubService) GetTaskPR(_ context.Context, _ string) (*github.TaskPR, error) {
	return m.taskPR, m.taskPRErr
}
func (m *mockGitHubService) EnsurePRWatch(_ context.Context, _, _, _, _, branch string) (*github.PRWatch, error) {
	m.ensureWatchCalls++
	m.ensureWatchBranch = branch
	return &github.PRWatch{}, nil
}
func (m *mockGitHubService) GetPRWatchBySession(_ context.Context, _ string) (*github.PRWatch, error) {
	return nil, nil
}
func (m *mockGitHubService) CreatePRWatch(_ context.Context, _, _, _, _ string, _ int, branch string) (*github.PRWatch, error) {
	m.createWatchCalls++
	m.createWatchBranch = branch
	return &github.PRWatch{}, nil
}
func (m *mockGitHubService) AssociatePRWithTask(_ context.Context, _ string, _ *github.PR) (*github.TaskPR, error) {
	m.associateCalls++
	return &github.TaskPR{}, nil
}
func (m *mockGitHubService) RecordReviewPRTask(context.Context, string, string, string, int, string, string) error {
	return nil
}

func TestInterpolateReviewPrompt(t *testing.T) {
	pr := &github.PR{
		Number:      42,
		Title:       "Add feature X",
		HTMLURL:     "https://github.com/myorg/myrepo/pull/42",
		AuthorLogin: "alice",
		RepoOwner:   "myorg",
		RepoName:    "myrepo",
		HeadBranch:  "feature-x",
		BaseBranch:  "main",
	}

	tests := []struct {
		name     string
		template string
		want     string
	}{
		{
			"empty template uses embedded default",
			"",
			"Review Pull Request #42: Add feature X\nRepository: myorg/myrepo\nPR: https://github.com/myorg/myrepo/pull/42\nAuthor: alice\nBranch: feature-x → main\n\nTo see ONLY the PR changes, use:\n- git diff origin/main...HEAD (three-dot = only changes on the PR branch)\n- git log --oneline origin/main..HEAD (list PR commits)\nDo NOT review files outside this diff.",
		},
		{
			"all placeholders",
			"Review {{pr.link}} (#{{pr.number}}) by {{pr.author}} in {{pr.repo}} on {{pr.branch}} -> {{pr.base_branch}}: {{pr.title}}",
			"Review https://github.com/myorg/myrepo/pull/42 (#42) by alice in myorg/myrepo on feature-x -> main: Add feature X",
		},
		{
			"no placeholders",
			"Please review this PR",
			"Please review this PR",
		},
		{
			"partial placeholders",
			"Check {{pr.link}}",
			"Check https://github.com/myorg/myrepo/pull/42",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := interpolateReviewPrompt(tt.template, pr)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCheckSessionPR(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()

	testPR := &github.PR{
		Number:      10,
		Title:       "Test PR",
		HTMLURL:     "https://github.com/myorg/myrepo/pull/10",
		AuthorLogin: "alice",
		RepoOwner:   "myorg",
		RepoName:    "myrepo",
		HeadBranch:  "feature-branch",
		BaseBranch:  "main",
	}

	// seedWithRepo creates task + session + repository + task-repository + worktree
	// so that resolveTaskRepo and GetTaskSession succeed.
	seedWithRepo := func(t *testing.T, branch, checkoutBranch string) *Service {
		t.Helper()
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		// Create a GitHub-backed repository record
		repoObj := &models.Repository{
			ID:            "repo1",
			WorkspaceID:   "ws1",
			Name:          "myrepo",
			SourceType:    "provider",
			Provider:      "github",
			ProviderOwner: "myorg",
			ProviderName:  "myrepo",
			CreatedAt:     now,
			UpdatedAt:     now,
		}
		if err := repo.CreateRepository(ctx, repoObj); err != nil {
			t.Fatalf("failed to create repository: %v", err)
		}

		// Link task to repository
		taskRepo := &models.TaskRepository{
			ID:             "tr1",
			TaskID:         "t1",
			RepositoryID:   "repo1",
			CheckoutBranch: checkoutBranch,
			Position:       0,
			CreatedAt:      now,
			UpdatedAt:      now,
		}
		if err := repo.CreateTaskRepository(ctx, taskRepo); err != nil {
			t.Fatalf("failed to create task repository: %v", err)
		}

		// Add worktree with branch to the session
		wt := &models.TaskSessionWorktree{
			ID:             "wt1",
			SessionID:      "s1",
			WorktreeID:     "wtree1",
			RepositoryID:   "repo1",
			WorktreeBranch: branch,
			CreatedAt:      now,
		}
		if err := repo.CreateTaskSessionWorktree(ctx, wt); err != nil {
			t.Fatalf("failed to create worktree: %v", err)
		}

		svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
		return svc
	}

	t.Run("returns false when github service is nil", func(t *testing.T) {
		repo := setupTestRepo(t)
		svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
		// githubService is nil by default

		found, err := svc.CheckSessionPR(ctx, "t1", "s1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if found {
			t.Error("expected found=false when githubService is nil")
		}
	})

	t.Run("returns true when PR already associated", func(t *testing.T) {
		repo := setupTestRepo(t)
		svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
		ghSvc := &mockGitHubService{
			taskPR: &github.TaskPR{ID: "tpr1", TaskID: "t1", PRNumber: 10},
		}
		svc.SetGitHubService(ghSvc)

		found, err := svc.CheckSessionPR(ctx, "t1", "s1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !found {
			t.Error("expected found=true when PR already exists")
		}
		if ghSvc.ensureWatchCalls != 0 {
			t.Errorf("expected no EnsurePRWatch calls, got %d", ghSvc.ensureWatchCalls)
		}
	})

	t.Run("returns false when task has no repository", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")
		svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
		ghSvc := &mockGitHubService{taskPRErr: nil, taskPR: nil}
		svc.SetGitHubService(ghSvc)

		found, err := svc.CheckSessionPR(ctx, "t1", "s1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if found {
			t.Error("expected found=false when task has no repository")
		}
	})

	t.Run("returns false when session has no worktree branch", func(t *testing.T) {
		svc := seedWithRepo(t, "", "") // empty branch
		ghSvc := &mockGitHubService{taskPRErr: nil, taskPR: nil}
		svc.SetGitHubService(ghSvc)

		found, err := svc.CheckSessionPR(ctx, "t1", "s1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if found {
			t.Error("expected found=false when no branch on worktree")
		}
	})

	t.Run("returns false when no PR exists on branch", func(t *testing.T) {
		svc := seedWithRepo(t, "feature-branch", "")
		mockClient := github.NewMockClient()
		// No PR added to mock client
		ghSvc := &mockGitHubService{client: mockClient}
		svc.SetGitHubService(ghSvc)

		found, err := svc.CheckSessionPR(ctx, "t1", "s1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if found {
			t.Error("expected found=false when no PR on branch")
		}
		if ghSvc.ensureWatchCalls != 1 {
			t.Errorf("expected 1 EnsurePRWatch call, got %d", ghSvc.ensureWatchCalls)
		}
		if ghSvc.ensureWatchBranch != "feature-branch" {
			t.Errorf("expected EnsurePRWatch branch %q, got %q", "feature-branch", ghSvc.ensureWatchBranch)
		}
	})

	t.Run("finds PR and associates it", func(t *testing.T) {
		svc := seedWithRepo(t, "feature-branch", "")
		mockClient := github.NewMockClient()
		mockClient.AddPR(testPR)
		ghSvc := &mockGitHubService{client: mockClient}
		svc.SetGitHubService(ghSvc)

		found, err := svc.CheckSessionPR(ctx, "t1", "s1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !found {
			t.Error("expected found=true when PR exists on branch")
		}
		if ghSvc.ensureWatchCalls != 1 {
			t.Errorf("expected 1 EnsurePRWatch call, got %d", ghSvc.ensureWatchCalls)
		}
		if ghSvc.createWatchCalls != 1 {
			t.Errorf("expected 1 CreatePRWatch call (from associatePRFromPush), got %d", ghSvc.createWatchCalls)
		}
		if ghSvc.ensureWatchBranch != "feature-branch" {
			t.Errorf("expected EnsurePRWatch branch %q, got %q", "feature-branch", ghSvc.ensureWatchBranch)
		}
		if ghSvc.createWatchBranch != "feature-branch" {
			t.Errorf("expected CreatePRWatch branch %q, got %q", "feature-branch", ghSvc.createWatchBranch)
		}
		if ghSvc.associateCalls != 1 {
			t.Errorf("expected 1 AssociatePRWithTask call, got %d", ghSvc.associateCalls)
		}
	})

	t.Run("prefers checkout branch over synthetic worktree branch", func(t *testing.T) {
		svc := seedWithRepo(t, "kandev/pr-review-abc", "feature-branch")
		mockClient := github.NewMockClient()
		mockClient.AddPR(testPR)
		ghSvc := &mockGitHubService{client: mockClient}
		svc.SetGitHubService(ghSvc)

		found, err := svc.CheckSessionPR(ctx, "t1", "s1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !found {
			t.Error("expected found=true when PR exists on checkout branch")
		}
		if ghSvc.ensureWatchBranch != "feature-branch" {
			t.Errorf("expected EnsurePRWatch branch %q, got %q", "feature-branch", ghSvc.ensureWatchBranch)
		}
		if ghSvc.createWatchBranch != "feature-branch" {
			t.Errorf("expected CreatePRWatch branch %q, got %q", "feature-branch", ghSvc.createWatchBranch)
		}
	})

	t.Run("ensureSessionPRWatch prefers checkout branch", func(t *testing.T) {
		svc := seedWithRepo(t, "kandev/pr-review-abc", "feature-branch")
		ghSvc := &mockGitHubService{}
		svc.SetGitHubService(ghSvc)

		svc.ensureSessionPRWatch(ctx, "t1", "s1", "kandev/pr-review-abc")

		if ghSvc.ensureWatchCalls != 1 {
			t.Errorf("expected 1 EnsurePRWatch call, got %d", ghSvc.ensureWatchCalls)
		}
		if ghSvc.ensureWatchBranch != "feature-branch" {
			t.Errorf("expected EnsurePRWatch branch %q, got %q", "feature-branch", ghSvc.ensureWatchBranch)
		}
	})
}
