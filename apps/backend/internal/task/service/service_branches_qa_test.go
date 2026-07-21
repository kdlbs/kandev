package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

// TestAddBranchToTask_QA_ProberErrorFallsThrough exercises the
// error-from-probe path (not just empty-result): the rejection contract
// must hold the same way and leave no orphan rows.
func TestAddBranchToTask_QA_ProberErrorFallsThrough(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "WS"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "WF"})
	_ = repo.CreateRepository(ctx, &models.Repository{ID: "repo-1", WorkspaceID: "ws-1", Name: "frontend", DefaultBranch: "main"})

	task, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID:    "ws-1",
		WorkflowID:     "wf-1",
		WorkflowStepID: "step-1",
		Title:          "Probe error",
		Repositories:   []TaskRepositoryInput{{RepositoryID: "repo-1", BaseBranch: "main"}},
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	seedWorktreeTaskEnv(t, repo, task.ID, "env-1")
	svc.SetProviderDefaultBranchProber(&stubProber{err: errors.New("network: timeout")})

	beforeRepos, _ := repo.ListRepositories(ctx, "ws-1")
	_, err = svc.AddBranchToTask(ctx, AddBranchToTaskRequest{
		TaskID:    task.ID,
		GitHubURL: "https://github.com/acme/never-seen",
	})
	if err == nil {
		t.Fatal("expected probe error path to reject")
	}
	if !strings.Contains(err.Error(), "cannot resolve base_branch") {
		t.Errorf("unexpected error: %v", err)
	}
	afterRepos, _ := repo.ListRepositories(ctx, "ws-1")
	if len(afterRepos) != len(beforeRepos) {
		t.Errorf("orphan repo leaked on probe-error path: before=%d after=%d", len(beforeRepos), len(afterRepos))
	}
}

// TestAddBranchToTask_QA_BadGitHubURLRejected pins the upstream URL
// validation. parseGitHubRepoURL must surface bad URLs before any DB
// write so the workspace stays clean. Plain validation surface.
func TestAddBranchToTask_QA_BadGitHubURLRejected(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "WS"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "WF"})
	_ = repo.CreateRepository(ctx, &models.Repository{ID: "repo-1", WorkspaceID: "ws-1", Name: "frontend", DefaultBranch: "main"})

	task, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID:    "ws-1",
		WorkflowID:     "wf-1",
		WorkflowStepID: "step-1",
		Title:          "Bad URL",
		Repositories:   []TaskRepositoryInput{{RepositoryID: "repo-1", BaseBranch: "main"}},
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	seedWorktreeTaskEnv(t, repo, task.ID, "env-1")

	beforeRepos, _ := repo.ListRepositories(ctx, "ws-1")
	_, err = svc.AddBranchToTask(ctx, AddBranchToTaskRequest{
		TaskID:    task.ID,
		GitHubURL: "https://gitlab.com/acme/widgets", // not github
	})
	if err == nil {
		t.Fatal("expected rejection of non-github URL")
	}
	afterRepos, _ := repo.ListRepositories(ctx, "ws-1")
	if len(afterRepos) != len(beforeRepos) {
		t.Errorf("URL parse error path leaked an orphan: before=%d after=%d", len(beforeRepos), len(afterRepos))
	}
}

// TestAddBranchToTask_QA_RegisteredProviderRepoSkipsProbe pins the
// happy path when the workspace already has the provider repo
// registered (with a non-empty default_branch): the probe is NOT called
// (existing row's stored default wins) and the call succeeds.
func TestAddBranchToTask_QA_RegisteredProviderRepoSkipsProbe(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "WS"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "WF"})
	_ = repo.CreateRepository(ctx, &models.Repository{
		ID: "repo-existing", WorkspaceID: "ws-1", Name: "acme/widgets",
		Provider: "github", ProviderHost: "https://github.com", ProviderOwner: "acme", ProviderName: "widgets",
		DefaultBranch: "trunk", SourceType: "provider",
	})
	_ = repo.CreateRepository(ctx, &models.Repository{ID: "repo-1", WorkspaceID: "ws-1", Name: "frontend", DefaultBranch: "main"})

	task, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID:    "ws-1",
		WorkflowID:     "wf-1",
		WorkflowStepID: "step-1",
		Title:          "Skip probe",
		Repositories:   []TaskRepositoryInput{{RepositoryID: "repo-1", BaseBranch: "main"}},
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	// Pre-launch (no env) so we don't need a materializer for this assertion.
	prober := &stubProber{branch: "shouldnt-be-used"}
	svc.SetProviderDefaultBranchProber(prober)

	added, err := svc.AddBranchToTask(ctx, AddBranchToTaskRequest{
		TaskID:    task.ID,
		GitHubURL: "https://github.com/acme/widgets",
	})
	if err != nil {
		t.Fatalf("AddBranchToTask: %v", err)
	}
	if prober.calls != 0 {
		t.Errorf("probe should not run when existing repo carries default_branch; calls=%d", prober.calls)
	}
	if added.BaseBranch != "trunk" {
		t.Errorf("expected existing repo's stored default_branch=trunk on row, got %q", added.BaseBranch)
	}
	if added.RepositoryID != "repo-existing" {
		t.Errorf("expected to reuse existing repo id, got %q", added.RepositoryID)
	}
}

// TestAddBranchToTask_QA_BackfillsExistingEmptyDefaultBranch covers the
// FindOrCreateRepository backfill side effect: a workspace repo with an
// empty default_branch (e.g. orphan row from a prior bug-affected call)
// gets its default_branch populated when the probe returns a real value.
func TestAddBranchToTask_QA_BackfillsExistingEmptyDefaultBranch(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "WS"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "WF"})
	_ = repo.CreateRepository(ctx, &models.Repository{
		ID: "repo-empty", WorkspaceID: "ws-1", Name: "acme/widgets",
		Provider: "github", ProviderHost: "https://github.com", ProviderOwner: "acme", ProviderName: "widgets",
		DefaultBranch: "", SourceType: "provider", // legacy orphan with empty default
	})
	_ = repo.CreateRepository(ctx, &models.Repository{ID: "repo-1", WorkspaceID: "ws-1", Name: "frontend", DefaultBranch: "main"})

	task, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID:    "ws-1",
		WorkflowID:     "wf-1",
		WorkflowStepID: "step-1",
		Title:          "Backfill",
		Repositories:   []TaskRepositoryInput{{RepositoryID: "repo-1", BaseBranch: "main"}},
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	prober := &stubProber{branch: "main"}
	svc.SetProviderDefaultBranchProber(prober)

	added, err := svc.AddBranchToTask(ctx, AddBranchToTaskRequest{
		TaskID:    task.ID,
		GitHubURL: "https://github.com/acme/widgets",
	})
	if err != nil {
		t.Fatalf("AddBranchToTask: %v", err)
	}
	if added.BaseBranch != "main" {
		t.Errorf("expected base_branch=main from probe, got %q", added.BaseBranch)
	}
	reloaded, err := svc.GetRepository(ctx, "repo-empty")
	if err != nil || reloaded == nil {
		t.Fatalf("GetRepository: %v", err)
	}
	if reloaded.DefaultBranch != "main" {
		t.Errorf("expected existing repo default_branch backfilled to main, got %q", reloaded.DefaultBranch)
	}
}

// TestAddBranchToTask_QA_RepositoryIDFastPathIgnoresProbe pins that the
// repository_id fast path never invokes the probe — the worktree path
// for already-known repos must stay zero-cost.
func TestAddBranchToTask_QA_RepositoryIDFastPathIgnoresProbe(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "WS"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "WF"})
	_ = repo.CreateRepository(ctx, &models.Repository{ID: "repo-1", WorkspaceID: "ws-1", Name: "frontend", DefaultBranch: "main"})

	task, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID:    "ws-1",
		WorkflowID:     "wf-1",
		WorkflowStepID: "step-1",
		Title:          "Fast path",
		Repositories:   []TaskRepositoryInput{{RepositoryID: "repo-1", BaseBranch: "main"}},
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	prober := &stubProber{branch: "should-not-fire"}
	svc.SetProviderDefaultBranchProber(prober)

	_, err = svc.AddBranchToTask(ctx, AddBranchToTaskRequest{
		TaskID:         task.ID,
		RepositoryID:   "repo-1",
		BaseBranch:     "main",
		CheckoutBranch: "feature/fast-path",
	})
	if err != nil {
		t.Fatalf("AddBranchToTask: %v", err)
	}
	if prober.calls != 0 {
		t.Errorf("probe must not run on repository_id fast path; calls=%d", prober.calls)
	}
}
