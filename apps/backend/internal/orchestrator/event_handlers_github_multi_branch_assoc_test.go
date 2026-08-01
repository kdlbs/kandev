package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

// seedGitHubPushAssocFixture creates the repository, task_repository
// (checked out at checkoutBranch), and session wiring shared by
// TestGitHubMultiBranchAssociationsCoexist and
// TestGitHubPushAssociationPassesRepositoryID.
func seedGitHubPushAssocFixture(t *testing.T, repo *sqliterepo.Repository, checkoutBranch string) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()

	repoObj := &models.Repository{
		ID: "repo1", WorkspaceID: "ws1", Name: "kandev",
		SourceType: "provider", Provider: "github",
		ProviderOwner: "myorg", ProviderName: "kandev",
		CreatedAt: now, UpdatedAt: now,
	}
	if err := repo.CreateRepository(ctx, repoObj); err != nil {
		t.Fatalf("create repository: %v", err)
	}
	if err := repo.CreateTaskRepository(ctx, &models.TaskRepository{
		ID: "tr1", TaskID: "t1", RepositoryID: "repo1", CheckoutBranch: checkoutBranch,
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create task repository: %v", err)
	}
	session, _ := repo.GetTaskSession(ctx, "s1")
	session.RepositoryID = "repo1"
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("update session: %v", err)
	}
}

// TestGitHubMultiBranchAssociationsCoexist covers the gap flagged in
// event_handlers_github_multi_branch_test.go: TestIsMultiBranchSubdir only
// pins the pure string-matching helper, not the end-to-end behavior it
// exists to support. Two branches of ONE repository in ONE session (the
// `<repo>-<branch-slug>` sibling-worktree naming) must each get their own
// watch and association — the second branch's push must not be dropped, and
// must not clobber the first branch's already-created rows.
func TestGitHubMultiBranchAssociationsCoexist(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	seedGitHubPushAssocFixture(t, repo, "main")

	mockClient := github.NewMockClient()
	mockClient.AddPR(&github.PR{
		Number: 1, Title: "Primary", HTMLURL: "https://github.com/myorg/kandev/pull/1",
		RepoOwner: "myorg", RepoName: "kandev", HeadBranch: "main", BaseBranch: "main",
	})
	mockClient.AddPR(&github.PR{
		Number: 2, Title: "Secondary", HTMLURL: "https://github.com/myorg/kandev/pull/2",
		RepoOwner: "myorg", RepoName: "kandev", HeadBranch: "feature-x", BaseBranch: "main",
	})
	ghSvc := &mockGitHubService{client: mockClient}
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.SetGitHubService(ghSvc)

	// Primary branch push: repositoryName empty (legacy single-repo tag).
	svc.detectPushAndAssociatePR(ctx, "s1", "t1", "", "main")
	// Secondary branch push: repositoryName tagged with the multi-branch
	// sibling-worktree subdir naming ("<repo.Name>-<branch-slug>").
	svc.detectPushAndAssociatePR(ctx, "s1", "t1", "kandev-feature-x", "feature-x")

	if ghSvc.createWatchCalls != 2 {
		t.Fatalf("expected 2 CreatePRWatch calls (one per branch), got %d", ghSvc.createWatchCalls)
	}
	if ghSvc.associateCalls != 2 {
		t.Fatalf("expected 2 AssociatePRWithTask calls (one per branch), got %d", ghSvc.associateCalls)
	}
	if len(ghSvc.createWatchLog) != 2 || len(ghSvc.associateLog) != 2 {
		t.Fatalf("call logs incomplete: createWatch=%+v associate=%+v", ghSvc.createWatchLog, ghSvc.associateLog)
	}

	branches := map[string]bool{}
	for _, c := range ghSvc.createWatchLog {
		branches[c.Branch] = true
		// AC17: the multi-repo/multi-branch path must pass a non-empty
		// repositoryID on every call — an empty value here means the push
		// silently fell back to whichever watch already existed instead of
		// creating its own, the exact regression the event_handlers_github.go
		// comment near associatePRFromPushScoped warns about.
		if c.RepositoryID == "" {
			t.Fatalf("CreatePRWatch call for branch %q had empty repositoryID", c.Branch)
		}
	}
	if !branches["main"] || !branches["feature-x"] {
		t.Fatalf("expected watches for both main and feature-x, got %+v", ghSvc.createWatchLog)
	}

	for _, c := range ghSvc.associateLog {
		if c.RepositoryID == "" {
			t.Fatalf("AssociatePRWithTask call for branch %q had empty repositoryID", c.Branch)
		}
	}
}

// TestGitHubPushAssociationPassesRepositoryID pins the AC17 invariant on the
// single-repo path too: even when there is only one repository, push
// detection must resolve and pass its repositoryID rather than an empty
// string, since AssociatePRWithTaskForWorkspace and CreatePRWatchForWorkspace
// use it to scope the row.
func TestGitHubPushAssociationPassesRepositoryID(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	seedGitHubPushAssocFixture(t, repo, "main")

	mockClient := github.NewMockClient()
	mockClient.AddPR(&github.PR{
		Number: 1, Title: "PR", HTMLURL: "https://github.com/myorg/kandev/pull/1",
		RepoOwner: "myorg", RepoName: "kandev", HeadBranch: "main", BaseBranch: "main",
	})
	ghSvc := &mockGitHubService{client: mockClient}
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.SetGitHubService(ghSvc)

	svc.detectPushAndAssociatePR(ctx, "s1", "t1", "", "main")

	if ghSvc.lastCreateWatchRepositoryID != "repo1" {
		t.Fatalf("CreatePRWatch repositoryID = %q, want repo1", ghSvc.lastCreateWatchRepositoryID)
	}
	if ghSvc.lastAssociateRepositoryID != "repo1" {
		t.Fatalf("AssociatePRWithTask repositoryID = %q, want repo1", ghSvc.lastAssociateRepositoryID)
	}
}
