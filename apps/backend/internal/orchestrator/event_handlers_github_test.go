package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// mockGitHubService implements GitHubService for testing.
type mockGitHubService struct {
	client              github.Client
	taskPR              *github.TaskPR
	taskPRErr           error
	prWatch             *github.PRWatch // returned by GetPRWatchBySession (nil = no watch)
	ensureWatchCalls    int
	createWatchCalls    int
	associateCalls      int
	updateBranchCalls   int
	updatePRNumberCalls int
	resetWatchCalls     int
	resetWatchBranch    string
	ensureWatchBranch   string
	createWatchBranch   string
	updatedBranch       string
	updatedPRNumber     int
	// repository_id captured by the most recent CreatePRWatch /
	// AssociatePRWithTask call. Used by the multi-repo push tests to assert
	// the per-repo scoping (an empty value indicates the legacy single-repo
	// path landed on primary by accident).
	lastCreateWatchRepositoryID string
	lastAssociateRepositoryID   string

	// Review PR reservation tracking.
	reserveCalls   int
	reserveReturn  bool // what ReserveReviewPRTask returns (default false; tests set true to let flow proceed)
	reserveErr     error
	assignCalls    int
	assignedTaskID string
	releaseCalls   int

	// Issue reservation tracking.
	issueReserveCalls  int
	issueReserveReturn bool
	issueAssignCalls   int
	issueAssignedID    string
	issueReleaseCalls  int
}

func (m *mockGitHubService) Client() github.Client { return m.client }
func (m *mockGitHubService) GetTaskPR(_ context.Context, _ string) (*github.TaskPR, error) {
	return m.taskPR, m.taskPRErr
}
func (m *mockGitHubService) EnsurePRWatch(_ context.Context, _, _, _, _, _, branch string) (*github.PRWatch, error) {
	m.ensureWatchCalls++
	m.ensureWatchBranch = branch
	return &github.PRWatch{}, nil
}
func (m *mockGitHubService) GetPRWatchBySession(_ context.Context, _ string) (*github.PRWatch, error) {
	return m.prWatch, nil
}
func (m *mockGitHubService) GetPRWatchBySessionAndRepo(_ context.Context, _, _ string) (*github.PRWatch, error) {
	return m.prWatch, nil
}
func (m *mockGitHubService) CreatePRWatch(_ context.Context, _, _, repositoryID, _, _ string, _ int, branch string) (*github.PRWatch, error) {
	m.createWatchCalls++
	m.createWatchBranch = branch
	m.lastCreateWatchRepositoryID = repositoryID
	return &github.PRWatch{}, nil
}
func (m *mockGitHubService) AssociatePRWithTask(_ context.Context, _, repositoryID string, _ *github.PR) (*github.TaskPR, error) {
	m.associateCalls++
	m.lastAssociateRepositoryID = repositoryID
	return &github.TaskPR{}, nil
}
func (m *mockGitHubService) UpdatePRWatchBranchIfSearching(_ context.Context, _, branch string) error {
	m.updateBranchCalls++
	m.updatedBranch = branch
	return nil
}
func (m *mockGitHubService) UpdatePRWatchPRNumber(_ context.Context, _ string, prNumber int) error {
	m.updatePRNumberCalls++
	m.updatedPRNumber = prNumber
	return nil
}
func (m *mockGitHubService) ResetPRWatch(_ context.Context, _, branch string) error {
	m.resetWatchCalls++
	m.resetWatchBranch = branch
	return nil
}
func (m *mockGitHubService) ListActivePRWatches(context.Context) ([]*github.PRWatch, error) {
	return nil, nil
}
func (m *mockGitHubService) ReserveReviewPRTask(_ context.Context, _, _, _ string, _ int, _ string) (bool, error) {
	m.reserveCalls++
	return m.reserveReturn, m.reserveErr
}
func (m *mockGitHubService) AssignReviewPRTaskID(_ context.Context, _, _, _ string, _ int, taskID string) error {
	m.assignCalls++
	m.assignedTaskID = taskID
	return nil
}
func (m *mockGitHubService) ReleaseReviewPRTask(_ context.Context, _, _, _ string, _ int) error {
	m.releaseCalls++
	return nil
}
func (m *mockGitHubService) ReserveIssueWatchTask(_ context.Context, _, _, _ string, _ int, _ string) (bool, error) {
	m.issueReserveCalls++
	return m.issueReserveReturn, nil
}
func (m *mockGitHubService) AssignIssueWatchTaskID(_ context.Context, _, _, _ string, _ int, taskID string) error {
	m.issueAssignCalls++
	m.issueAssignedID = taskID
	return nil
}
func (m *mockGitHubService) ReleaseIssueWatchTask(_ context.Context, _, _, _ string, _ int) error {
	m.issueReleaseCalls++
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

// TestDetectPushAndAssociatePR tests the push detection flow for PR association.
func TestDetectPushAndAssociatePR(t *testing.T) {
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

	// seedSessionWithRepo creates a session linked to a GitHub repository
	// for testing detectPushAndAssociatePR which uses resolveSessionRepo.
	seedSessionWithRepo := func(t *testing.T) *Service {
		t.Helper()
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		repoObj := &models.Repository{
			ID: "repo1", WorkspaceID: "ws1", Name: "myrepo",
			SourceType: "provider", Provider: "github",
			ProviderOwner: "myorg", ProviderName: "myrepo",
			CreatedAt: now, UpdatedAt: now,
		}
		if err := repo.CreateRepository(ctx, repoObj); err != nil {
			t.Fatalf("failed to create repository: %v", err)
		}

		session, _ := repo.GetTaskSession(ctx, "s1")
		session.RepositoryID = "repo1"
		if err := repo.UpdateTaskSession(ctx, session); err != nil {
			t.Fatalf("failed to update session: %v", err)
		}

		return createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	}

	t.Run("searches and finds PR when no watch exists", func(t *testing.T) {
		svc := seedSessionWithRepo(t)
		mockClient := github.NewMockClient()
		mockClient.AddPR(testPR)
		ghSvc := &mockGitHubService{client: mockClient}
		svc.SetGitHubService(ghSvc)

		svc.detectPushAndAssociatePR(ctx, "s1", "t1", "", "feature-branch")

		if ghSvc.associateCalls != 1 {
			t.Errorf("expected 1 AssociatePRWithTask call, got %d", ghSvc.associateCalls)
		}
		if ghSvc.createWatchCalls != 1 {
			t.Errorf("expected 1 CreatePRWatch call, got %d", ghSvc.createWatchCalls)
		}
	})

	t.Run("skips when watch has pr_number > 0", func(t *testing.T) {
		svc := seedSessionWithRepo(t)
		ghSvc := &mockGitHubService{
			prWatch: &github.PRWatch{
				ID: "w1", SessionID: "s1", TaskID: "t1",
				Owner: "myorg", Repo: "myrepo",
				PRNumber: 10, Branch: "feature-branch",
			},
		}
		svc.SetGitHubService(ghSvc)

		svc.detectPushAndAssociatePR(ctx, "s1", "t1", "", "feature-branch")

		if ghSvc.associateCalls != 0 {
			t.Errorf("expected no AssociatePRWithTask calls when PR already found, got %d", ghSvc.associateCalls)
		}
	})

	t.Run("searches immediately when watch has pr_number=0", func(t *testing.T) {
		svc := seedSessionWithRepo(t)
		mockClient := github.NewMockClient()
		mockClient.AddPR(testPR)
		ghSvc := &mockGitHubService{
			client: mockClient,
			prWatch: &github.PRWatch{
				ID: "w1", SessionID: "s1", TaskID: "t1",
				Owner: "myorg", Repo: "myrepo",
				PRNumber: 0, Branch: "feature-branch",
			},
		}
		svc.SetGitHubService(ghSvc)

		svc.detectPushAndAssociatePR(ctx, "s1", "t1", "", "feature-branch")

		if ghSvc.associateCalls != 1 {
			t.Errorf("expected 1 AssociatePRWithTask call when pr_number=0, got %d", ghSvc.associateCalls)
		}
		// Should update existing watch's PR number, not create a new watch
		if ghSvc.updatePRNumberCalls != 1 {
			t.Errorf("expected 1 UpdatePRWatchPRNumber call, got %d", ghSvc.updatePRNumberCalls)
		}
		if ghSvc.createWatchCalls != 0 {
			t.Errorf("expected no CreatePRWatch calls (watch already exists), got %d", ghSvc.createWatchCalls)
		}
	})

	t.Run("updates watch branch when agent pushes from different branch", func(t *testing.T) {
		svc := seedSessionWithRepo(t)
		mockClient := github.NewMockClient()
		// PR is on the NEW branch, not the old one
		prOnNewBranch := &github.PR{
			Number: 10, Title: "Test PR",
			HTMLURL:     "https://github.com/myorg/myrepo/pull/10",
			AuthorLogin: "alice", RepoOwner: "myorg", RepoName: "myrepo",
			HeadBranch: "new-branch", BaseBranch: "main",
		}
		mockClient.AddPR(prOnNewBranch)
		ghSvc := &mockGitHubService{
			client: mockClient,
			prWatch: &github.PRWatch{
				ID: "w1", SessionID: "s1", TaskID: "t1",
				Owner: "myorg", Repo: "myrepo",
				PRNumber: 0, Branch: "old-branch", // watch created with original branch
			},
		}
		svc.SetGitHubService(ghSvc)

		// Agent pushed from "new-branch" (different from watch's "old-branch")
		svc.detectPushAndAssociatePR(ctx, "s1", "t1", "", "new-branch")

		if ghSvc.updateBranchCalls != 1 {
			t.Errorf("expected 1 UpdatePRWatchBranch call, got %d", ghSvc.updateBranchCalls)
		}
		if ghSvc.updatedBranch != "new-branch" {
			t.Errorf("expected updated branch %q, got %q", "new-branch", ghSvc.updatedBranch)
		}
		if ghSvc.associateCalls != 1 {
			t.Errorf("expected 1 AssociatePRWithTask call, got %d", ghSvc.associateCalls)
		}
	})

	t.Run("does not update branch when same as watch", func(t *testing.T) {
		svc := seedSessionWithRepo(t)
		mockClient := github.NewMockClient()
		ghSvc := &mockGitHubService{
			client: mockClient,
			prWatch: &github.PRWatch{
				ID: "w1", SessionID: "s1", TaskID: "t1",
				Owner: "myorg", Repo: "myrepo",
				PRNumber: 0, Branch: "feature-branch",
			},
		}
		svc.SetGitHubService(ghSvc)

		svc.detectPushAndAssociatePR(ctx, "s1", "t1", "", "feature-branch")

		if ghSvc.updateBranchCalls != 0 {
			t.Errorf("expected no UpdatePRWatchBranch calls when branch matches, got %d", ghSvc.updateBranchCalls)
		}
	})

	// Multi-repo: when the push event carries a repository_name, the resolver
	// must look up that specific task_repository (not the primary one) and
	// associate the PR under its repository_id. Without this, a multi-repo
	// task only ever gets one PR detected — the second repo's push silently
	// drops or lands under the wrong repository_id.
	t.Run("resolvePushRepo multi-repo: scopes to the named repository", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		// Two repos linked to the task; agent pushes to the secondary one.
		primary := &models.Repository{
			ID: "repo-front", WorkspaceID: "ws1", Name: "frontend",
			SourceType: "provider", Provider: "github",
			ProviderOwner: "myorg", ProviderName: "frontend",
			CreatedAt: now, UpdatedAt: now,
		}
		secondary := &models.Repository{
			ID: "repo-back", WorkspaceID: "ws1", Name: "backend",
			SourceType: "provider", Provider: "github",
			ProviderOwner: "myorg", ProviderName: "backend",
			CreatedAt: now, UpdatedAt: now,
		}
		if err := repo.CreateRepository(ctx, primary); err != nil {
			t.Fatalf("create primary: %v", err)
		}
		if err := repo.CreateRepository(ctx, secondary); err != nil {
			t.Fatalf("create secondary: %v", err)
		}
		if err := repo.CreateTaskRepository(ctx, &models.TaskRepository{
			ID: "tr-1", TaskID: "t1", RepositoryID: "repo-front", Position: 0, CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			t.Fatalf("link primary: %v", err)
		}
		if err := repo.CreateTaskRepository(ctx, &models.TaskRepository{
			ID: "tr-2", TaskID: "t1", RepositoryID: "repo-back", Position: 1, CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			t.Fatalf("link secondary: %v", err)
		}

		svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
		mockClient := github.NewMockClient()
		mockClient.AddPR(&github.PR{
			Number: 99, Title: "Backend PR",
			HTMLURL:     "https://github.com/myorg/backend/pull/99",
			AuthorLogin: "alice",
			RepoOwner:   "myorg", RepoName: "backend",
			HeadBranch: "feature-x", BaseBranch: "main",
		})
		ghSvc := &mockGitHubService{client: mockClient}
		svc.SetGitHubService(ghSvc)

		// Push event came from the "backend" repo subdir — must associate
		// under repo-back, NOT repo-front.
		svc.detectPushAndAssociatePR(ctx, "s1", "t1", "backend", "feature-x")

		if ghSvc.associateCalls != 1 {
			t.Fatalf("expected 1 AssociatePRWithTask call, got %d", ghSvc.associateCalls)
		}
		if ghSvc.lastAssociateRepositoryID != "repo-back" {
			t.Errorf("expected association under repo-back, got %q", ghSvc.lastAssociateRepositoryID)
		}
		if ghSvc.createWatchCalls != 1 {
			t.Errorf("expected 1 CreatePRWatch call, got %d", ghSvc.createWatchCalls)
		}
		if ghSvc.lastCreateWatchRepositoryID != "repo-back" {
			t.Errorf("expected watch under repo-back, got %q", ghSvc.lastCreateWatchRepositoryID)
		}
	})

	// Symmetric guard: an unknown repository_name (not linked to the task)
	// must not fall back to the primary — silently mis-associating a PR is
	// worse than dropping the event.
	t.Run("resolvePushRepo multi-repo: drops events for unknown repository_name", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")
		if err := repo.CreateRepository(ctx, &models.Repository{
			ID: "repo-front", WorkspaceID: "ws1", Name: "frontend",
			SourceType: "provider", Provider: "github",
			ProviderOwner: "myorg", ProviderName: "frontend",
			CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			t.Fatalf("create: %v", err)
		}
		if err := repo.CreateTaskRepository(ctx, &models.TaskRepository{
			ID: "tr-1", TaskID: "t1", RepositoryID: "repo-front", Position: 0, CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			t.Fatalf("link: %v", err)
		}

		svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
		ghSvc := &mockGitHubService{client: github.NewMockClient()}
		svc.SetGitHubService(ghSvc)

		svc.detectPushAndAssociatePR(ctx, "s1", "t1", "ghost-repo", "feature-x")

		if ghSvc.associateCalls != 0 {
			t.Errorf("expected 0 AssociatePRWithTask calls for unknown repo, got %d", ghSvc.associateCalls)
		}
		if ghSvc.createWatchCalls != 0 {
			t.Errorf("expected 0 CreatePRWatch calls for unknown repo, got %d", ghSvc.createWatchCalls)
		}
	})
}

func TestListTasksNeedingPRWatch(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()

	// seedFullSession creates a task, session, repository, task-repo, and worktree.
	seedFullSession := func(t *testing.T, repo interface {
		CreateRepository(ctx context.Context, r *models.Repository) error
		CreateTaskRepository(ctx context.Context, tr *models.TaskRepository) error
		CreateTaskSessionWorktree(ctx context.Context, wt *models.TaskSessionWorktree) error
	}, taskID, sessionID, branch, repoID string) {
		t.Helper()
		rObj := &models.Repository{
			ID: repoID, WorkspaceID: "ws1", Name: "myrepo",
			SourceType: "provider", Provider: "github",
			ProviderOwner: "myorg", ProviderName: "myrepo",
			CreatedAt: now, UpdatedAt: now,
		}
		// Ignore duplicate errors for shared repos.
		_ = repo.CreateRepository(ctx, rObj)

		tr := &models.TaskRepository{
			ID: "tr-" + sessionID, TaskID: taskID, RepositoryID: repoID,
			Position: 0, CreatedAt: now, UpdatedAt: now,
		}
		_ = repo.CreateTaskRepository(ctx, tr)

		wt := &models.TaskSessionWorktree{
			ID: "wt-" + sessionID, SessionID: sessionID,
			WorktreeID: "wtree-" + sessionID, RepositoryID: repoID,
			WorktreeBranch: branch, CreatedAt: now,
		}
		if err := repo.CreateTaskSessionWorktree(ctx, wt); err != nil {
			t.Fatalf("failed to create worktree: %v", err)
		}
	}

	// seedTask creates a second task and session in an already-seeded repo.
	seedTask := func(t *testing.T, repo *sqliterepo.Repository, taskID, sessionID string) {
		t.Helper()
		task := &models.Task{
			ID: taskID, WorkflowID: "wf1", WorkflowStepID: "step1",
			Title: "Test Task", Description: "Test",
			State: v1.TaskStateInProgress, CreatedAt: now, UpdatedAt: now,
		}
		if err := repo.CreateTask(ctx, task); err != nil {
			t.Fatalf("failed to create task: %v", err)
		}
		session := &models.TaskSession{
			ID: sessionID, TaskID: taskID,
			State: models.TaskSessionStateRunning, StartedAt: now, UpdatedAt: now,
		}
		if err := repo.CreateTaskSession(ctx, session); err != nil {
			t.Fatalf("failed to create session: %v", err)
		}
	}

	t.Run("returns sessions with branches but no watch", func(t *testing.T) {
		testRepo := setupTestRepo(t)
		seedSession(t, testRepo, "t1", "s1", "step1")
		seedTask(t, testRepo, "t2", "s2")
		seedFullSession(t, testRepo, "t1", "s1", "feature-a", "repo1")
		seedFullSession(t, testRepo, "t2", "s2", "feature-b", "repo1")

		svc := createTestService(testRepo, newMockStepGetter(), newMockTaskRepo())
		ghSvc := &mockGitHubService{
			// s1 has a watch, s2 does not.
			prWatch: nil,
		}
		svc.SetGitHubService(ghSvc)

		tasks, err := svc.ListTasksNeedingPRWatch(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Both sessions have branches and no watches (mock returns nil for all).
		if len(tasks) < 1 {
			t.Fatalf("expected at least 1 task, got %d", len(tasks))
		}

		// Verify results contain expected fields.
		found := map[string]bool{}
		for _, ti := range tasks {
			found[ti.SessionID] = true
			if ti.Owner != "myorg" {
				t.Errorf("expected owner %q, got %q", "myorg", ti.Owner)
			}
		}
		if !found["s1"] && !found["s2"] {
			t.Error("expected at least one of s1 or s2 in results")
		}
	})

	t.Run("excludes sessions on archived tasks", func(t *testing.T) {
		testRepo := setupTestRepo(t)
		seedSession(t, testRepo, "t1", "s1", "step1")
		seedFullSession(t, testRepo, "t1", "s1", "feature-a", "repo1")

		// Archive the task.
		if err := testRepo.ArchiveTask(ctx, "t1"); err != nil {
			t.Fatalf("failed to archive task: %v", err)
		}

		svc := createTestService(testRepo, newMockStepGetter(), newMockTaskRepo())
		ghSvc := &mockGitHubService{}
		svc.SetGitHubService(ghSvc)

		tasks, err := svc.ListTasksNeedingPRWatch(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(tasks) != 0 {
			t.Errorf("expected no tasks for archived task, got %d", len(tasks))
		}
	})

	t.Run("returns nil when github service is nil", func(t *testing.T) {
		testRepo := setupTestRepo(t)
		svc := createTestService(testRepo, newMockStepGetter(), newMockTaskRepo())

		tasks, err := svc.ListTasksNeedingPRWatch(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(tasks) != 0 {
			t.Errorf("expected empty result when github service is nil, got %d", len(tasks))
		}
	})

	// Regression for task a7a59bd8: a multi-repo task only ever had a watch
	// for its primary repo, so the poller never searched GitHub for the
	// non-primary repo's PR. The fix iterates per (session, repo).
	t.Run("multi-repo: emits one entry per repository", func(t *testing.T) {
		testRepo := setupTestRepo(t)
		seedSession(t, testRepo, "t1", "s1", "step1")

		// Two repos linked to one task, one worktree per repo with its own branch.
		repos := []*models.Repository{
			{
				ID: "repo-front", WorkspaceID: "ws1", Name: "frontend",
				SourceType: "provider", Provider: "github",
				ProviderOwner: "myorg", ProviderName: "frontend",
				CreatedAt: now, UpdatedAt: now,
			},
			{
				ID: "repo-back", WorkspaceID: "ws1", Name: "backend",
				SourceType: "provider", Provider: "github",
				ProviderOwner: "myorg", ProviderName: "backend",
				CreatedAt: now, UpdatedAt: now,
			},
		}
		for _, r := range repos {
			if err := testRepo.CreateRepository(ctx, r); err != nil {
				t.Fatalf("create repo %s: %v", r.ID, err)
			}
		}
		links := []*models.TaskRepository{
			{ID: "tr-1", TaskID: "t1", RepositoryID: "repo-front", Position: 0, CreatedAt: now, UpdatedAt: now},
			{ID: "tr-2", TaskID: "t1", RepositoryID: "repo-back", Position: 1, CreatedAt: now, UpdatedAt: now},
		}
		for _, l := range links {
			if err := testRepo.CreateTaskRepository(ctx, l); err != nil {
				t.Fatalf("link %s: %v", l.ID, err)
			}
		}
		worktrees := []*models.TaskSessionWorktree{
			{ID: "wt-1", SessionID: "s1", WorktreeID: "wtree-1", RepositoryID: "repo-front", WorktreeBranch: "feat/frontend", CreatedAt: now},
			{ID: "wt-2", SessionID: "s1", WorktreeID: "wtree-2", RepositoryID: "repo-back", WorktreeBranch: "feat/backend", CreatedAt: now},
		}
		for _, wt := range worktrees {
			if err := testRepo.CreateTaskSessionWorktree(ctx, wt); err != nil {
				t.Fatalf("worktree %s: %v", wt.ID, err)
			}
		}

		svc := createTestService(testRepo, newMockStepGetter(), newMockTaskRepo())
		// Mock the watch list as if frontend already has a watch (the
		// primary repo). Pre-fix this would have suppressed both repos —
		// fix: secondary repo still emits.
		svc.SetGitHubService(&mockGitHubService{})

		tasks, err := svc.ListTasksNeedingPRWatch(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Expect both repos to be emitted (no existing watches in the mock).
		byRepo := map[string]github.TaskBranchInfo{}
		for _, ti := range tasks {
			byRepo[ti.RepositoryID] = ti
		}
		front, ok := byRepo["repo-front"]
		if !ok {
			t.Fatal("expected primary repo entry, missing")
		}
		if front.Owner != "myorg" || front.Repo != "frontend" || front.Branch != "feat/frontend" {
			t.Errorf("primary entry mis-resolved: %+v", front)
		}
		back, ok := byRepo["repo-back"]
		if !ok {
			t.Fatal("expected secondary repo entry, missing — this was the regression")
		}
		if back.Owner != "myorg" || back.Repo != "backend" || back.Branch != "feat/backend" {
			t.Errorf("secondary entry mis-resolved: %+v", back)
		}
	})
}

// TestEnsureSessionPRWatch_MultiRepo guards the same multi-repo gap on the
// session-start path: ensureSessionPRWatch (called when a session is launched
// or resumed) must create a watch row for every repo on the task, not only
// the primary one. Pre-fix only one EnsurePRWatch call landed regardless of
// repo count, so secondary repos waited indefinitely for the never-firing
// poller and never associated their PRs.
func TestEnsureSessionPRWatch_MultiRepo(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()

	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")

	for _, r := range []*models.Repository{
		{
			ID: "repo-front", WorkspaceID: "ws1", Name: "frontend",
			SourceType: "provider", Provider: "github",
			ProviderOwner: "myorg", ProviderName: "frontend",
			CreatedAt: now, UpdatedAt: now,
		},
		{
			ID: "repo-back", WorkspaceID: "ws1", Name: "backend",
			SourceType: "provider", Provider: "github",
			ProviderOwner: "myorg", ProviderName: "backend",
			CreatedAt: now, UpdatedAt: now,
		},
	} {
		if err := repo.CreateRepository(ctx, r); err != nil {
			t.Fatalf("create repo: %v", err)
		}
	}
	for _, l := range []*models.TaskRepository{
		{ID: "tr-1", TaskID: "t1", RepositoryID: "repo-front", Position: 0, CreatedAt: now, UpdatedAt: now},
		{ID: "tr-2", TaskID: "t1", RepositoryID: "repo-back", Position: 1, CreatedAt: now, UpdatedAt: now},
	} {
		if err := repo.CreateTaskRepository(ctx, l); err != nil {
			t.Fatalf("link: %v", err)
		}
	}
	for _, wt := range []*models.TaskSessionWorktree{
		{ID: "wt-1", SessionID: "s1", WorktreeID: "wtree-1", RepositoryID: "repo-front", WorktreeBranch: "feat/frontend", CreatedAt: now},
		{ID: "wt-2", SessionID: "s1", WorktreeID: "wtree-2", RepositoryID: "repo-back", WorktreeBranch: "feat/backend", CreatedAt: now},
	} {
		if err := repo.CreateTaskSessionWorktree(ctx, wt); err != nil {
			t.Fatalf("worktree: %v", err)
		}
	}

	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	ghSvc := &mockGitHubService{}
	svc.SetGitHubService(ghSvc)

	svc.ensureSessionPRWatch(ctx, "t1", "s1", "ignored-fallback")

	if ghSvc.ensureWatchCalls != 2 {
		t.Errorf("expected 2 EnsurePRWatch calls (one per repo), got %d", ghSvc.ensureWatchCalls)
	}
}
