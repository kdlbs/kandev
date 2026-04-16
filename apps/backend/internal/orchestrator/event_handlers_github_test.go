package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
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

	// Review PR reservation tracking.
	reserveCalls   int
	reserveReturn  bool // what ReserveReviewPRTask returns (default false; tests set true to let flow proceed)
	reserveErr     error
	assignCalls    int
	assignedTaskID string
	releaseCalls   int
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
	return m.prWatch, nil
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

		svc.detectPushAndAssociatePR(ctx, "s1", "t1", "feature-branch")

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

		svc.detectPushAndAssociatePR(ctx, "s1", "t1", "feature-branch")

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

		svc.detectPushAndAssociatePR(ctx, "s1", "t1", "feature-branch")

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
		svc.detectPushAndAssociatePR(ctx, "s1", "t1", "new-branch")

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

		svc.detectPushAndAssociatePR(ctx, "s1", "t1", "feature-branch")

		if ghSvc.updateBranchCalls != 0 {
			t.Errorf("expected no UpdatePRWatchBranch calls when branch matches, got %d", ghSvc.updateBranchCalls)
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
}

// countingReviewTaskCreator records how many times CreateReviewTask was called.
type countingReviewTaskCreator struct {
	calls  int
	err    error
	taskID string
}

func (c *countingReviewTaskCreator) CreateReviewTask(_ context.Context, _ *ReviewTaskRequest) (*models.Task, error) {
	c.calls++
	if c.err != nil {
		return nil, c.err
	}
	id := c.taskID
	if id == "" {
		id = "task-created"
	}
	return &models.Task{ID: id}, nil
}

// newReviewEvent builds a NewReviewPREvent for createReviewTask tests.
func newReviewEvent() *github.NewReviewPREvent {
	return &github.NewReviewPREvent{
		ReviewWatchID:  "w1",
		WorkspaceID:    "ws1",
		WorkflowID:     "wf1",
		WorkflowStepID: "step1",
		PR: &github.PR{
			Number: 42, Title: "Some PR", HTMLURL: "https://gh/acme/widget/pull/42",
			RepoOwner: "acme", RepoName: "widget",
		},
	}
}

// setupReviewTaskTest builds a Service with a seeded workflow step (needed
// because createReviewTask runs shouldAutoStartStep after task creation).
func setupReviewTaskTest(t *testing.T) (*Service, *mockStepGetter) {
	t.Helper()
	repo := setupTestRepo(t)
	stepGetter := newMockStepGetter()
	stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
		ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
		Events: wfmodels.StepEvents{}, // no auto-start action
	}
	return createTestService(repo, stepGetter, newMockTaskRepo()), stepGetter
}

// TestCreateReviewTask_SkipsWhenAlreadyReserved is the regression test for the
// duplicate review-task bug: if another handler has already reserved the
// dedup slot for this PR, createReviewTask must NOT call CreateReviewTask.
func TestCreateReviewTask_SkipsWhenAlreadyReserved(t *testing.T) {
	svc, _ := setupReviewTaskTest(t)
	ghSvc := &mockGitHubService{reserveReturn: false} // reservation lost to concurrent handler
	svc.SetGitHubService(ghSvc)
	creator := &countingReviewTaskCreator{}
	svc.SetReviewTaskCreator(creator)

	svc.createReviewTask(context.Background(), newReviewEvent())

	if ghSvc.reserveCalls != 1 {
		t.Errorf("expected 1 Reserve call, got %d", ghSvc.reserveCalls)
	}
	if creator.calls != 0 {
		t.Errorf("expected CreateReviewTask NOT to be called when reservation lost, got %d calls", creator.calls)
	}
	if ghSvc.releaseCalls != 0 {
		t.Errorf("expected no Release calls when reservation was never held, got %d", ghSvc.releaseCalls)
	}
}

// TestCreateReviewTask_ReservesThenAssignsTaskID verifies the happy path:
// Reserve -> CreateReviewTask -> AssignReviewPRTaskID.
func TestCreateReviewTask_ReservesThenAssignsTaskID(t *testing.T) {
	svc, _ := setupReviewTaskTest(t)
	ghSvc := &mockGitHubService{reserveReturn: true}
	svc.SetGitHubService(ghSvc)
	creator := &countingReviewTaskCreator{taskID: "task-999"}
	svc.SetReviewTaskCreator(creator)

	svc.createReviewTask(context.Background(), newReviewEvent())

	if ghSvc.reserveCalls != 1 {
		t.Errorf("expected 1 Reserve call, got %d", ghSvc.reserveCalls)
	}
	if creator.calls != 1 {
		t.Errorf("expected 1 CreateReviewTask call, got %d", creator.calls)
	}
	if ghSvc.assignCalls != 1 {
		t.Errorf("expected 1 AssignReviewPRTaskID call, got %d", ghSvc.assignCalls)
	}
	if ghSvc.assignedTaskID != "task-999" {
		t.Errorf("AssignReviewPRTaskID got taskID=%q, want %q", ghSvc.assignedTaskID, "task-999")
	}
	if ghSvc.releaseCalls != 0 {
		t.Errorf("expected no Release calls on happy path, got %d", ghSvc.releaseCalls)
	}
}

// TestCreateReviewTask_ReleasesOnTaskCreationFailure verifies that a failed
// CreateReviewTask triggers a Release so the slot can be retried on the next
// poll instead of being permanently blocked by an orphan reservation.
func TestCreateReviewTask_ReleasesOnTaskCreationFailure(t *testing.T) {
	svc, _ := setupReviewTaskTest(t)
	ghSvc := &mockGitHubService{reserveReturn: true}
	svc.SetGitHubService(ghSvc)
	creator := &countingReviewTaskCreator{err: assertErrTaskCreate}
	svc.SetReviewTaskCreator(creator)

	svc.createReviewTask(context.Background(), newReviewEvent())

	if ghSvc.reserveCalls != 1 {
		t.Errorf("expected 1 Reserve call, got %d", ghSvc.reserveCalls)
	}
	if creator.calls != 1 {
		t.Errorf("expected 1 CreateReviewTask call, got %d", creator.calls)
	}
	if ghSvc.releaseCalls != 1 {
		t.Errorf("expected 1 Release call after task-create failure, got %d", ghSvc.releaseCalls)
	}
	if ghSvc.assignCalls != 0 {
		t.Errorf("expected no Assign call after failure, got %d", ghSvc.assignCalls)
	}
}

var assertErrTaskCreate = &taskCreateErr{"simulated task creation failure"}

type taskCreateErr struct{ msg string }

func (e *taskCreateErr) Error() string { return e.msg }
