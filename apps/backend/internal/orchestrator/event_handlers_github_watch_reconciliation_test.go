package orchestrator

import (
	"context"
	"fmt"
	"testing"

	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

// @covers AC-INTEGRATIONS-GITHUB-PR-DISCOVERY-001.4
func TestResolvePRWatchBranchForWatch(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	seedGitHubPushAssocFixture(t, repo, "primary")
	if err := repo.CreateRepository(ctx, &models.Repository{ID: "repo2", WorkspaceID: "ws1", Name: "secondary", Provider: "github", ProviderOwner: "myorg", ProviderName: "secondary"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTaskRepository(ctx, &models.TaskRepository{ID: "tr2", TaskID: "t1", RepositoryID: "repo2", CheckoutBranch: "secondary", Position: 1}); err != nil {
		t.Fatal(err)
	}
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	watch := &github.PRWatch{TaskID: "t1", SessionID: "s1", RepositoryID: "repo2", Branch: "secondary"}
	if got := svc.ResolveBranchForWatch(ctx, nil); got != "" {
		t.Fatalf("nil watch resolved to %q", got)
	}
	if got := svc.ResolveBranchForWatch(ctx, &github.PRWatch{RepositoryID: "repo1"}); got != "" {
		t.Fatalf("missing session resolved to %q", got)
	}
	got := svc.ResolveBranchForWatch(ctx, watch)
	if got != watch.Branch {
		t.Fatalf("secondary watch resolved to %q, want %q", got, watch.Branch)
	}
}

// @covers AC-INTEGRATIONS-GITHUB-PR-DISCOVERY-001.4
func TestResolvePRWatchBranchForWatch_SourceIdentity(t *testing.T) {
	for _, tt := range []struct {
		name, session, repository, branch, want string
		branches                                []string
		cancelled                               bool
	}{
		{name: "configured checkout wins", session: "s1", repository: "repo1", branch: "old", want: "primary", branches: []string{"synthetic-review"}},
		{name: "exact sibling survives", session: "s1", repository: "repo1", branch: "secondary", want: "secondary", branches: []string{"primary", "secondary"}},
		{name: "ambiguous rename", session: "s1", repository: "repo1", branch: "old", branches: []string{"first", "second"}},
		{name: "empty sibling is not candidate", session: "s1", repository: "repo1", branch: "old", want: "valid", branches: []string{"", "valid"}},
		{name: "duplicate branches are one candidate", session: "s1", repository: "repo1", branch: "old", want: "valid", branches: []string{"valid", "valid"}},
		{name: "missing repository", session: "s1", repository: "missing", branch: "old", branches: []string{"primary"}},
		{name: "missing session", session: "missing", repository: "repo1", branch: "old"},
		{name: "missing repository identity", session: "s1", branch: "old"},
		{name: "cancelled read", session: "s1", repository: "repo1", branch: "old", cancelled: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			repo := setupTestRepo(t)
			seedSession(t, repo, "t1", "s1", "step1")
			seedGitHubPushAssocFixture(t, repo, "primary")
			owner, err := repo.GetTask(ctx, "t1")
			if err != nil {
				t.Fatal(err)
			}
			owner.ID = "attributed-group-owner"
			if err := repo.CreateTask(ctx, owner); err != nil {
				t.Fatal(err)
			}
			if err := repo.CreateTaskRepository(ctx, &models.TaskRepository{ID: "owner-tr", TaskID: "attributed-group-owner", RepositoryID: "repo1", CheckoutBranch: "owner-primary"}); err != nil {
				t.Fatal(err)
			}
			repos := make([]*models.TaskEnvironmentRepo, 0, len(tt.branches))
			for i, branch := range tt.branches {
				repos = append(repos, &models.TaskEnvironmentRepo{ID: fmt.Sprintf("wt%d", i), RepositoryID: "repo1", WorktreeID: fmt.Sprintf("tree%d", i), BranchSlug: fmt.Sprintf("branch%d", i), WorktreeBranch: branch, Status: "active", Position: i})
			}
			if len(repos) > 0 {
				attachWatchTestEnvironment(t, repo, repos)
			}
			svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
			if tt.cancelled {
				cancelled, cancel := context.WithCancel(ctx)
				cancel()
				ctx = cancelled
			}
			watch := &github.PRWatch{TaskID: "attributed-group-owner", SessionID: tt.session, RepositoryID: tt.repository, Branch: tt.branch}
			got := svc.ResolveBranchForWatch(ctx, watch)
			if got != tt.want {
				t.Fatalf("branch = %q, want %q", got, tt.want)
			}
		})
	}
}

func attachWatchTestEnvironment(t *testing.T, repo *sqliterepo.Repository, repos []*models.TaskEnvironmentRepo) {
	t.Helper()
	ctx := context.Background()
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{ID: "env1", TaskID: "t1", ExecutorType: "worktree", WorkspacePath: t.TempDir(), Status: models.TaskEnvironmentStatusReady, Repos: repos}); err != nil {
		t.Fatal(err)
	}
	session, err := repo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatal(err)
	}
	session.TaskEnvironmentID = "env1"
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatal(err)
	}
}

// @covers AC-INTEGRATIONS-GITHUB-PR-DISCOVERY-001.4
func TestResolvePRWatchBranchForWatch_CreationUsesSameTargets(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	seedGitHubPushAssocFixture(t, repo, "")
	if err := repo.CreateRepository(ctx, &models.Repository{ID: "repo2", WorkspaceID: "ws1", Name: "secondary", Provider: "github", ProviderOwner: "myorg", ProviderName: "secondary"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTaskRepository(ctx, &models.TaskRepository{ID: "tr2", TaskID: "t1", RepositoryID: "repo2", Position: 1}); err != nil {
		t.Fatal(err)
	}
	attachWatchTestEnvironment(t, repo, []*models.TaskEnvironmentRepo{
		{ID: "wt1", RepositoryID: "repo1", WorktreeID: "tree1", WorktreeBranch: "primary", Status: "active"},
		{ID: "wt2", RepositoryID: "repo2", WorktreeID: "tree2", WorktreeBranch: "secondary", Status: "active", Position: 1},
	})
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.SetGitHubService(&mockGitHubService{})
	targets, err := svc.ListTasksNeedingPRWatch(ctx)
	if err != nil {
		t.Fatal(err)
	}
	seen := make(map[string]bool)
	for _, target := range targets {
		want := map[string]string{"repo1": "primary", "repo2": "secondary"}[target.RepositoryID]
		if target.Branch != want {
			t.Fatalf("creation target = %+v, want branch %q", target, want)
		}
		key := target.SessionID + "/" + target.RepositoryID + "/" + target.Branch
		if seen[key] {
			t.Fatalf("duplicate creation target %s", key)
		}
		seen[key] = true
		watch := &github.PRWatch{SessionID: target.SessionID, TaskID: target.TaskID, RepositoryID: target.RepositoryID, Branch: target.Branch}
		if got := svc.ResolveBranchForWatch(ctx, watch); got != want {
			t.Fatalf("refresh = %q, want %q", got, want)
		}
	}
	if len(targets) != 2 {
		t.Fatalf("targets = %d, want 2", len(targets))
	}
}
