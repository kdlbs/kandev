package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
)

// TestAddBranchToTask_HappyPath attaches a second branch to a task that
// already has one TaskRepository row on the same repository and verifies a
// fresh row was created with the right position.
func TestAddBranchToTask_HappyPath(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "WS"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "WF"})
	_ = repo.CreateRepository(ctx, &models.Repository{ID: "repo-1", WorkspaceID: "ws-1", Name: "frontend", DefaultBranch: "main"})

	req := &CreateTaskRequest{
		WorkspaceID:    "ws-1",
		WorkflowID:     "wf-1",
		WorkflowStepID: "step-1",
		Title:          "Multi-branch",
		Repositories: []TaskRepositoryInput{
			{RepositoryID: "repo-1", BaseBranch: "main", CheckoutBranch: "feature/a"},
		},
	}
	task, err := svc.CreateTask(ctx, req)
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	added, err := svc.AddBranchToTask(ctx, AddBranchToTaskRequest{
		TaskID:         task.ID,
		RepositoryID:   "repo-1",
		BaseBranch:     "main",
		CheckoutBranch: "feature/b",
	})
	if err != nil {
		t.Fatalf("AddBranchToTask: %v", err)
	}
	if added.RepositoryID != "repo-1" || added.CheckoutBranch != "feature/b" {
		t.Errorf("unexpected row: %+v", added)
	}
	if added.Position == 0 {
		t.Errorf("expected position > 0, got %d", added.Position)
	}

	rows, err := repo.ListTaskRepositories(ctx, task.ID)
	if err != nil {
		t.Fatalf("ListTaskRepositories: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	gotBranches := map[string]bool{}
	for _, r := range rows {
		if r.RepositoryID != "repo-1" {
			t.Errorf("unexpected repo id %q", r.RepositoryID)
		}
		gotBranches[r.CheckoutBranch] = true
	}
	if !gotBranches["feature/a"] || !gotBranches["feature/b"] {
		t.Errorf("missing branches in rows: %+v", gotBranches)
	}
}

// TestAddBranchToTask_RejectsDuplicate ensures the same (repo, branch) pair
// cannot be attached twice — the second call returns an error rather than
// silently no-op'ing.
func TestAddBranchToTask_RejectsDuplicate(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "WS"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "WF"})
	_ = repo.CreateRepository(ctx, &models.Repository{ID: "repo-1", WorkspaceID: "ws-1", Name: "frontend"})

	task, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID:    "ws-1",
		WorkflowID:     "wf-1",
		WorkflowStepID: "step-1",
		Title:          "Dup",
		Repositories: []TaskRepositoryInput{
			{RepositoryID: "repo-1", BaseBranch: "main", CheckoutBranch: "feature/a"},
		},
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	_, err = svc.AddBranchToTask(ctx, AddBranchToTaskRequest{
		TaskID:         task.ID,
		RepositoryID:   "repo-1",
		BaseBranch:     "main",
		CheckoutBranch: "feature/a",
	})
	if err == nil {
		t.Fatal("expected error for duplicate (repo, branch)")
	}
	if !strings.Contains(err.Error(), "already attached") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestCreateTask_AllowsSameRepoDifferentBranches verifies that the relaxed
// unique constraint lets a task be born multi-branch (two rows sharing
// repository_id, distinguished by checkout_branch).
func TestCreateTask_AllowsSameRepoDifferentBranches(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "WS"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "WF"})
	_ = repo.CreateRepository(ctx, &models.Repository{ID: "repo-1", WorkspaceID: "ws-1", Name: "frontend"})

	task, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID:    "ws-1",
		WorkflowID:     "wf-1",
		WorkflowStepID: "step-1",
		Title:          "Born multi-branch",
		Repositories: []TaskRepositoryInput{
			{RepositoryID: "repo-1", BaseBranch: "main", CheckoutBranch: "feature/a"},
			{RepositoryID: "repo-1", BaseBranch: "main", CheckoutBranch: "feature/b"},
		},
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	rows, err := repo.ListTaskRepositories(ctx, task.ID)
	if err != nil {
		t.Fatalf("ListTaskRepositories: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
}

// TestAddBranchToTask_RejectsNonWorktreeExecutor guards against using the
// MCP tool on tasks whose execution environment is docker / sprites /
// local — sibling worktrees are a git-worktree layout and other executors
// would silently swallow the new branch. The service must reject before
// inserting a task_repositories row so the DB stays consistent with disk.
func TestAddBranchToTask_RejectsNonWorktreeExecutor(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "WS"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "WF"})
	_ = repo.CreateRepository(ctx, &models.Repository{ID: "repo-1", WorkspaceID: "ws-1", Name: "frontend"})

	task, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID:    "ws-1",
		WorkflowID:     "wf-1",
		WorkflowStepID: "step-1",
		Title:          "Docker task",
		Repositories: []TaskRepositoryInput{
			{RepositoryID: "repo-1", BaseBranch: "main"},
		},
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	// Seed a task_environments row with a containerized executor type.
	now := time.Now().UTC()
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID:           "env-1",
		TaskID:       task.ID,
		ExecutorType: string(models.ExecutorTypeLocalDocker),
		Status:       "ready",
		CreatedAt:    now,
		UpdatedAt:    now,
	}); err != nil {
		t.Fatalf("CreateTaskEnvironment: %v", err)
	}

	_, err = svc.AddBranchToTask(ctx, AddBranchToTaskRequest{
		TaskID:         task.ID,
		CheckoutBranch: "feature/x",
	})
	if err == nil {
		t.Fatal("expected error rejecting non-worktree executor")
	}
	if !strings.Contains(err.Error(), "worktree executor") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestAddBranchToTask_AllowsWorktreeExecutor guards the happy path: a task
// running on the worktree executor accepts add_branch normally.
func TestAddBranchToTask_AllowsWorktreeExecutor(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "WS"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "WF"})
	_ = repo.CreateRepository(ctx, &models.Repository{ID: "repo-1", WorkspaceID: "ws-1", Name: "frontend", DefaultBranch: "main"})

	task, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID:    "ws-1",
		WorkflowID:     "wf-1",
		WorkflowStepID: "step-1",
		Title:          "Worktree task",
		Repositories: []TaskRepositoryInput{
			{RepositoryID: "repo-1", BaseBranch: "main"},
		},
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	now := time.Now().UTC()
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID:            "env-1",
		TaskID:        task.ID,
		ExecutorType:  string(models.ExecutorTypeWorktree),
		WorkspacePath: "/tmp/task-env",
		Status:        "ready",
		CreatedAt:     now,
		UpdatedAt:     now,
	}); err != nil {
		t.Fatalf("CreateTaskEnvironment: %v", err)
	}

	if _, err := svc.AddBranchToTask(ctx, AddBranchToTaskRequest{
		TaskID:         task.ID,
		CheckoutBranch: "feature/x",
	}); err != nil {
		t.Fatalf("AddBranchToTask on worktree executor: %v", err)
	}
}

// TestAddBranchToTask_AllowsBeforeLaunch verifies the pre-launch escape
// hatch: when a task hasn't created its task_environments row yet (no
// executor type fixed), add_branch is allowed so users can stage multiple
// branches before starting the agent.
func TestAddBranchToTask_AllowsBeforeLaunch(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "WS"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "WF"})
	_ = repo.CreateRepository(ctx, &models.Repository{ID: "repo-1", WorkspaceID: "ws-1", Name: "frontend", DefaultBranch: "main"})

	task, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID:    "ws-1",
		WorkflowID:     "wf-1",
		WorkflowStepID: "step-1",
		Title:          "Unlaunched task",
		Repositories: []TaskRepositoryInput{
			{RepositoryID: "repo-1", BaseBranch: "main"},
		},
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	// No task_environments row inserted — simulates a task created but
	// never started.
	if _, err := svc.AddBranchToTask(ctx, AddBranchToTaskRequest{
		TaskID:         task.ID,
		CheckoutBranch: "feature/x",
	}); err != nil {
		t.Fatalf("AddBranchToTask before launch: %v", err)
	}
}

// TestAddBranchToTask_AutoGeneratesNameWhenWouldCollide exercises the
// no-args agent flow: "add a branch" with no checkout_branch should produce
// a fresh row instead of erroring out because the primary already occupies
// the (repo, base, "") triple. Generated name follows `branch-<random>` so
// concurrent adders don't clash on a predictable counter.
func TestAddBranchToTask_AutoGeneratesNameWhenWouldCollide(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "WS"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "WF"})
	_ = repo.CreateRepository(ctx, &models.Repository{ID: "repo-1", WorkspaceID: "ws-1", Name: "frontend", DefaultBranch: "main"})

	task, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID:    "ws-1",
		WorkflowID:     "wf-1",
		WorkflowStepID: "step-1",
		Title:          "Single-repo primary",
		Repositories: []TaskRepositoryInput{
			{RepositoryID: "repo-1", BaseBranch: "main"},
		},
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	added, err := svc.AddBranchToTask(ctx, AddBranchToTaskRequest{
		TaskID: task.ID,
	})
	if err != nil {
		t.Fatalf("AddBranchToTask: %v", err)
	}
	if !strings.HasPrefix(added.CheckoutBranch, "branch-") || len(added.CheckoutBranch) != len("branch-")+3 {
		t.Errorf("expected auto-name branch-<3 random chars>, got %q", added.CheckoutBranch)
	}

	added2, err := svc.AddBranchToTask(ctx, AddBranchToTaskRequest{
		TaskID: task.ID,
	})
	if err != nil {
		t.Fatalf("AddBranchToTask (second auto): %v", err)
	}
	if !strings.HasPrefix(added2.CheckoutBranch, "branch-") || len(added2.CheckoutBranch) != len("branch-")+3 {
		t.Errorf("expected auto-name branch-<3 random chars>, got %q", added2.CheckoutBranch)
	}
	if added2.CheckoutBranch == added.CheckoutBranch {
		t.Errorf("expected distinct auto-names, both got %q", added.CheckoutBranch)
	}
}

// TestAddBranchToTask_DefaultsRepositoryWhenSingleRepoTask exercises the
// agent-friendly path: omitting repository_id on a single-repo task
// auto-resolves to that repo. Agents inside a task only have task-mode MCP
// tools (no list_repositories_kandev), so requiring the UUID would force
// them to hit the DB directly.
func TestAddBranchToTask_DefaultsRepositoryWhenSingleRepoTask(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "WS"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "WF"})
	_ = repo.CreateRepository(ctx, &models.Repository{ID: "repo-1", WorkspaceID: "ws-1", Name: "frontend", DefaultBranch: "main"})

	task, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID:    "ws-1",
		WorkflowID:     "wf-1",
		WorkflowStepID: "step-1",
		Title:          "Single repo task",
		Repositories: []TaskRepositoryInput{
			{RepositoryID: "repo-1", BaseBranch: "main"},
		},
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	added, err := svc.AddBranchToTask(ctx, AddBranchToTaskRequest{
		TaskID:         task.ID,
		CheckoutBranch: "feature/x",
	})
	if err != nil {
		t.Fatalf("AddBranchToTask without repository_id: %v", err)
	}
	if added.RepositoryID != "repo-1" {
		t.Errorf("expected auto-resolved repository_id=repo-1, got %q", added.RepositoryID)
	}
}

// TestAddBranchToTask_RejectsMissingRepoOnMultiRepoTask guards the
// ambiguous case: multi-repo tasks must force the agent to disambiguate
// rather than silently picking one repo.
func TestAddBranchToTask_RejectsMissingRepoOnMultiRepoTask(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "WS"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "WF"})
	_ = repo.CreateRepository(ctx, &models.Repository{ID: "repo-1", WorkspaceID: "ws-1", Name: "frontend"})
	_ = repo.CreateRepository(ctx, &models.Repository{ID: "repo-2", WorkspaceID: "ws-1", Name: "backend"})

	task, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID:    "ws-1",
		WorkflowID:     "wf-1",
		WorkflowStepID: "step-1",
		Title:          "Multi-repo task",
		Repositories: []TaskRepositoryInput{
			{RepositoryID: "repo-1", BaseBranch: "main"},
			{RepositoryID: "repo-2", BaseBranch: "main"},
		},
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	_, err = svc.AddBranchToTask(ctx, AddBranchToTaskRequest{
		TaskID:         task.ID,
		CheckoutBranch: "feature/x",
	})
	if err == nil {
		t.Fatal("expected error when repository_id is missing on a multi-repo task")
	}
	if !strings.Contains(err.Error(), "repository_id is required") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestCreateTask_AllowsSameRepoDifferentBaseBranches mirrors the worktree-
// executor flow where the branch lives in base_branch and checkout_branch is
// empty. Two rows differing only in base_branch must both be accepted —
// previously they collided because the constraint did not include base_branch.
func TestCreateTask_AllowsSameRepoDifferentBaseBranches(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "WS"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "WF"})
	_ = repo.CreateRepository(ctx, &models.Repository{ID: "repo-1", WorkspaceID: "ws-1", Name: "frontend"})

	task, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID:    "ws-1",
		WorkflowID:     "wf-1",
		WorkflowStepID: "step-1",
		Title:          "Worktree multi-branch",
		Repositories: []TaskRepositoryInput{
			{RepositoryID: "repo-1", BaseBranch: "feature/a"},
			{RepositoryID: "repo-1", BaseBranch: "feature/b"},
		},
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	rows, err := repo.ListTaskRepositories(ctx, task.ID)
	if err != nil {
		t.Fatalf("ListTaskRepositories: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	gotBases := map[string]bool{}
	for _, r := range rows {
		gotBases[r.BaseBranch] = true
	}
	if !gotBases["feature/a"] || !gotBases["feature/b"] {
		t.Errorf("missing base_branch values in rows: %+v", gotBases)
	}
}

// TestAddBranchToTask_ResolvesGitHubURL exercises the agent-ergonomic path
// where the caller knows the GitHub URL but not the repository UUID. The
// service must find-or-create the repository in the task's workspace and
// attach the new branch row against the resolved id.
func TestAddBranchToTask_ResolvesGitHubURL(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "WS"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "WF"})
	// Seed the target repo with provider info so FindOrCreateRepository
	// returns the existing row instead of creating a duplicate.
	_ = repo.CreateRepository(ctx, &models.Repository{
		ID: "repo-target", WorkspaceID: "ws-1", Name: "acme/widgets",
		Provider: "github", ProviderHost: "https://github.com", ProviderOwner: "acme", ProviderName: "widgets",
		DefaultBranch: "main",
	})
	_ = repo.CreateRepository(ctx, &models.Repository{
		ID: "repo-primary", WorkspaceID: "ws-1", Name: "primary", DefaultBranch: "main",
	})

	// Multi-repo task — auto-default would fail, so URL resolution must
	// supply the repository_id.
	task, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID:    "ws-1",
		WorkflowID:     "wf-1",
		WorkflowStepID: "step-1",
		Title:          "Multi-repo task",
		Repositories: []TaskRepositoryInput{
			{RepositoryID: "repo-primary", BaseBranch: "main"},
		},
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	added, err := svc.AddBranchToTask(ctx, AddBranchToTaskRequest{
		TaskID:         task.ID,
		GitHubURL:      "https://github.com/acme/widgets",
		CheckoutBranch: "feature/x",
	})
	if err != nil {
		t.Fatalf("AddBranchToTask with github_url: %v", err)
	}
	if added.RepositoryID != "repo-target" {
		t.Errorf("expected resolved repository_id=repo-target, got %q", added.RepositoryID)
	}
	if added.BaseBranch != "main" {
		t.Errorf("expected base_branch defaulted to repo default (main), got %q", added.BaseBranch)
	}
}

// TestAddBranchToTask_ResolvesLocalPath exercises the local-worktree flow:
// caller supplies the on-disk folder path, the service finds the matching
// repository row in the task's workspace and attaches against it.
func TestAddBranchToTask_ResolvesLocalPath(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "WS"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "WF"})
	_ = repo.CreateRepository(ctx, &models.Repository{
		ID: "repo-primary", WorkspaceID: "ws-1", Name: "primary", DefaultBranch: "main",
	})
	_ = repo.CreateRepository(ctx, &models.Repository{
		ID: "repo-by-path", WorkspaceID: "ws-1", Name: "sibling",
		LocalPath: "/tmp/sibling", DefaultBranch: "develop",
	})

	task, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID:    "ws-1",
		WorkflowID:     "wf-1",
		WorkflowStepID: "step-1",
		Title:          "Multi-repo task",
		Repositories: []TaskRepositoryInput{
			{RepositoryID: "repo-primary", BaseBranch: "main"},
		},
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	added, err := svc.AddBranchToTask(ctx, AddBranchToTaskRequest{
		TaskID:         task.ID,
		LocalPath:      "/tmp/sibling",
		CheckoutBranch: "feature/y",
	})
	if err != nil {
		t.Fatalf("AddBranchToTask with local_path: %v", err)
	}
	if added.RepositoryID != "repo-by-path" {
		t.Errorf("expected resolved repository_id=repo-by-path, got %q", added.RepositoryID)
	}
	if added.BaseBranch != "develop" {
		t.Errorf("expected base_branch defaulted to repo default (develop), got %q", added.BaseBranch)
	}
}

// TestAddBranchToTask_RejectsNonWorktreeExecutor_NoOrphanRepo pins the
// ordering guarantee documented on requireWorktreeExecutorForBranchAdd:
// the executor gate must fire BEFORE ResolveRepositoryRef so a rejected
// add_branch call on a non-worktree task never leaks a freshly-created
// Repository row into the workspace via the github_url / local_path paths.
//
// Without this ordering, FindOrCreateRepository (github_url) and
// CreateRepository (local_path) write before the executor check rejects,
// leaving an orphan repository the user never asked for.
func TestAddBranchToTask_RejectsNonWorktreeExecutor_NoOrphanRepo(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "WS"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "WF"})
	_ = repo.CreateRepository(ctx, &models.Repository{ID: "repo-primary", WorkspaceID: "ws-1", Name: "primary", DefaultBranch: "main"})

	task, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID:    "ws-1",
		WorkflowID:     "wf-1",
		WorkflowStepID: "step-1",
		Title:          "Docker task",
		Repositories: []TaskRepositoryInput{
			{RepositoryID: "repo-primary", BaseBranch: "main"},
		},
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	// Seed a docker executor — add_branch must reject.
	now := time.Now().UTC()
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID:           "env-1",
		TaskID:       task.ID,
		ExecutorType: string(models.ExecutorTypeLocalDocker),
		Status:       "ready",
		CreatedAt:    now,
		UpdatedAt:    now,
	}); err != nil {
		t.Fatalf("CreateTaskEnvironment: %v", err)
	}

	before, err := repo.ListRepositories(ctx, "ws-1")
	if err != nil {
		t.Fatalf("ListRepositories before: %v", err)
	}
	beforeCount := len(before)

	// github_url points at a repo NOT yet in the workspace — would trigger
	// FindOrCreateRepository if resolution ran. The executor check must
	// reject first.
	_, err = svc.AddBranchToTask(ctx, AddBranchToTaskRequest{
		TaskID:    task.ID,
		GitHubURL: "https://github.com/acme/never-created",
	})
	if err == nil {
		t.Fatal("expected error rejecting non-worktree executor")
	}
	if !strings.Contains(err.Error(), "worktree executor") {
		t.Errorf("unexpected error: %v", err)
	}

	after, err := repo.ListRepositories(ctx, "ws-1")
	if err != nil {
		t.Fatalf("ListRepositories after: %v", err)
	}
	if len(after) != beforeCount {
		t.Errorf("expected no new repository rows, got %d before vs %d after", beforeCount, len(after))
	}
	for _, r := range after {
		if r.ProviderOwner == "acme" && r.ProviderName == "never-created" {
			t.Errorf("orphan repository row leaked into workspace: %+v", r)
		}
	}
}

// TestCreateTask_RejectsSameRepoSameBranch verifies the dedup guard still
// fires when the exact (repo, branch) pair is duplicated in the create
// payload — the migration-layer constraint mirrors the in-memory guard.
func TestCreateTask_RejectsSameRepoSameBranch(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "WS"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "WF"})
	_ = repo.CreateRepository(ctx, &models.Repository{ID: "repo-1", WorkspaceID: "ws-1", Name: "frontend"})

	_, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID:    "ws-1",
		WorkflowID:     "wf-1",
		WorkflowStepID: "step-1",
		Title:          "Bad dup",
		Repositories: []TaskRepositoryInput{
			{RepositoryID: "repo-1", BaseBranch: "main", CheckoutBranch: "feature/a"},
			{RepositoryID: "repo-1", BaseBranch: "main", CheckoutBranch: "feature/a"},
		},
	})
	if err == nil {
		t.Fatal("expected error for duplicate (repo, branch)")
	}
	if !strings.Contains(err.Error(), "more than once") {
		t.Errorf("unexpected error: %v", err)
	}
}

// stubProber implements ProviderDefaultBranchProber for tests; returns the
// preset branch (which may be empty to simulate probe failure) and records
// the call so tests can assert it ran.
type stubProber struct {
	branch          string
	err             error
	calls           int
	lastProviderArg string
	lastOwner       string
	lastName        string
}

func (p *stubProber) ProbeDefaultBranch(_ context.Context, provider, owner, name string) (string, error) {
	p.calls++
	p.lastProviderArg = provider
	p.lastOwner = owner
	p.lastName = name
	return p.branch, p.err
}

// stubMaterializer implements BranchMaterializer for tests; the err field
// controls whether the materialize step succeeds.
type stubMaterializer struct {
	err   error
	calls int
}

func (m *stubMaterializer) MaterializeBranch(_ context.Context, _ string, _ string) error {
	m.calls++
	return m.err
}

// seedWorktreeTaskEnv attaches a worktree-executor task_environments row so
// requireWorktreeExecutorForBranchAdd permits the call and taskAlreadyLaunched
// reports the task as live.
func seedWorktreeTaskEnv(t *testing.T, repo interface {
	CreateTaskEnvironment(ctx context.Context, env *models.TaskEnvironment) error
}, taskID, id string,
) {
	t.Helper()
	now := time.Now().UTC()
	if err := repo.CreateTaskEnvironment(context.Background(), &models.TaskEnvironment{
		ID:            id,
		TaskID:        taskID,
		ExecutorType:  string(models.ExecutorTypeWorktree),
		WorkspacePath: "/tmp/task-env",
		Status:        "ready",
		CreatedAt:     now,
		UpdatedAt:     now,
	}); err != nil {
		t.Fatalf("CreateTaskEnvironment: %v", err)
	}
}

// TestAddBranchToTask_RejectsProviderURLWithUnresolvableBaseBranch pins the
// bug fix: an add_branch call with repository_url for a repo that has no
// resolvable default branch (probe returns empty, no existing repo row in
// workspace) must return a "cannot resolve base_branch" error and leave no
// orphan rows behind — neither task_repositories nor a freshly-created
// Repository row.
func TestAddBranchToTask_RejectsProviderURLWithUnresolvableBaseBranch(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "WS"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "WF"})
	_ = repo.CreateRepository(ctx, &models.Repository{ID: "repo-1", WorkspaceID: "ws-1", Name: "frontend", DefaultBranch: "main"})

	task, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID:    "ws-1",
		WorkflowID:     "wf-1",
		WorkflowStepID: "step-1",
		Title:          "Provider URL",
		Repositories: []TaskRepositoryInput{
			{RepositoryID: "repo-1", BaseBranch: "main"},
		},
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	seedWorktreeTaskEnv(t, repo, task.ID, "env-1")
	svc.SetProviderDefaultBranchProber(&stubProber{branch: ""}) // probe fails

	beforeRepos, _ := repo.ListRepositories(ctx, "ws-1")
	beforeRows, _ := repo.ListTaskRepositories(ctx, task.ID)

	_, err = svc.AddBranchToTask(ctx, AddBranchToTaskRequest{
		TaskID:    task.ID,
		GitHubURL: "https://github.com/acme/never-seen",
	})
	if err == nil {
		t.Fatal("expected error for unresolvable base_branch")
	}
	if !strings.Contains(err.Error(), "cannot resolve base_branch") {
		t.Errorf("unexpected error: %v", err)
	}

	afterRepos, _ := repo.ListRepositories(ctx, "ws-1")
	if len(afterRepos) != len(beforeRepos) {
		t.Errorf("expected no orphan repositories row; before=%d after=%d", len(beforeRepos), len(afterRepos))
	}
	afterRows, _ := repo.ListTaskRepositories(ctx, task.ID)
	if len(afterRows) != len(beforeRows) {
		t.Errorf("expected no task_repositories row insert; before=%d after=%d", len(beforeRows), len(afterRows))
	}

	// Repository.created + repository.deleted must be symmetric on the
	// rollback path so WS subscribers / frontend caches don't keep a
	// phantom row after the resolve-then-fail sequence.
	var created, deleted int
	for _, evt := range eventBus.GetPublishedEvents() {
		switch evt.Type {
		case events.RepositoryCreated:
			created++
		case events.RepositoryDeleted:
			deleted++
		}
	}
	if created != deleted {
		t.Errorf("repository create/delete events not symmetric: created=%d deleted=%d", created, deleted)
	}
}

// TestAddBranchToTask_ProviderURLResolvesDefaultBranchAndPersists exercises
// the happy provider-URL path: the synchronous probe returns "main", which
// must populate both the new Repository's default_branch and the
// task_repositories row's base_branch.
func TestAddBranchToTask_ProviderURLResolvesDefaultBranchAndPersists(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "WS"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "WF"})
	_ = repo.CreateRepository(ctx, &models.Repository{ID: "repo-1", WorkspaceID: "ws-1", Name: "frontend", DefaultBranch: "main"})

	task, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID:    "ws-1",
		WorkflowID:     "wf-1",
		WorkflowStepID: "step-1",
		Title:          "Provider URL happy",
		Repositories: []TaskRepositoryInput{
			{RepositoryID: "repo-1", BaseBranch: "main"},
		},
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	// No task_environments row — exercises the pre-launch path so we don't
	// also need a materializer for the happy persist assertion.
	prober := &stubProber{branch: "main"}
	svc.SetProviderDefaultBranchProber(prober)

	added, err := svc.AddBranchToTask(ctx, AddBranchToTaskRequest{
		TaskID:    task.ID,
		GitHubURL: "https://github.com/acme/widgets",
	})
	if err != nil {
		t.Fatalf("AddBranchToTask: %v", err)
	}
	if prober.calls != 1 {
		t.Errorf("expected probe to run once, got %d calls", prober.calls)
	}
	if prober.lastProviderArg != "github" || prober.lastOwner != "acme" || prober.lastName != "widgets" {
		t.Errorf("unexpected probe args: %+v", prober)
	}
	if added.BaseBranch != "main" {
		t.Errorf("expected task_repositories.base_branch=main, got %q", added.BaseBranch)
	}
	created, err := svc.GetRepository(ctx, added.RepositoryID)
	if err != nil || created == nil {
		t.Fatalf("GetRepository: %v", err)
	}
	if created.DefaultBranch != "main" {
		t.Errorf("expected repositories.default_branch=main, got %q", created.DefaultBranch)
	}
	if created.ProviderOwner != "acme" || created.ProviderName != "widgets" {
		t.Errorf("unexpected provider info: %+v", created)
	}
}

// TestAddBranchToTask_MaterializeFailureRollsBackOnLiveTask covers the
// materialize-failure path on an already-launched worktree task: the
// task_repositories row must be deleted and the error must propagate to the
// caller rather than being swallowed. Also pins that no task.updated event
// is published on the rollback path — emitting one would push a phantom row
// to WS clients and require a second event to undo it.
func TestAddBranchToTask_MaterializeFailureRollsBackOnLiveTask(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "WS"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "WF"})
	_ = repo.CreateRepository(ctx, &models.Repository{ID: "repo-1", WorkspaceID: "ws-1", Name: "frontend", DefaultBranch: "main"})

	task, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID:    "ws-1",
		WorkflowID:     "wf-1",
		WorkflowStepID: "step-1",
		Title:          "Live task",
		Repositories: []TaskRepositoryInput{
			{RepositoryID: "repo-1", BaseBranch: "main"},
		},
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	seedWorktreeTaskEnv(t, repo, task.ID, "env-1")
	mat := &stubMaterializer{err: fmt.Errorf("simulated git failure")}
	svc.SetBranchMaterializer(mat)

	beforeRows, _ := repo.ListTaskRepositories(ctx, task.ID)
	eventBus.ClearEvents()
	_, err = svc.AddBranchToTask(ctx, AddBranchToTaskRequest{
		TaskID:         task.ID,
		RepositoryID:   "repo-1",
		BaseBranch:     "main",
		CheckoutBranch: "feature/x",
	})
	if err == nil {
		t.Fatal("expected materialize failure to propagate")
	}
	if !strings.Contains(err.Error(), "simulated git failure") {
		t.Errorf("unexpected error: %v", err)
	}
	if mat.calls != 1 {
		t.Errorf("expected materializer to be called once, got %d", mat.calls)
	}
	afterRows, _ := repo.ListTaskRepositories(ctx, task.ID)
	if len(afterRows) != len(beforeRows) {
		t.Errorf("expected task_repositories row rolled back; before=%d after=%d", len(beforeRows), len(afterRows))
	}
	for _, evt := range eventBus.GetPublishedEvents() {
		if evt.Type == events.TaskUpdated {
			t.Errorf("did not expect task.updated on rollback path; got %+v", evt)
		}
	}
}

// TestAddBranchToTask_MaterializeSkippedPreLaunchStillSucceeds preserves the
// legacy best-effort behaviour for pre-launch tasks: materializer failure on
// a task with no task_environments row is logged-and-continued, and the
// task_repositories row stays for the launcher to pick up.
func TestAddBranchToTask_MaterializeSkippedPreLaunchStillSucceeds(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "WS"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "WF"})
	_ = repo.CreateRepository(ctx, &models.Repository{ID: "repo-1", WorkspaceID: "ws-1", Name: "frontend", DefaultBranch: "main"})

	task, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID:    "ws-1",
		WorkflowID:     "wf-1",
		WorkflowStepID: "step-1",
		Title:          "Pre-launch",
		Repositories: []TaskRepositoryInput{
			{RepositoryID: "repo-1", BaseBranch: "main"},
		},
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	// No task_environments row → taskAlreadyLaunched reports false.
	mat := &stubMaterializer{err: fmt.Errorf("ignored on pre-launch tasks")}
	svc.SetBranchMaterializer(mat)

	added, err := svc.AddBranchToTask(ctx, AddBranchToTaskRequest{
		TaskID:         task.ID,
		RepositoryID:   "repo-1",
		BaseBranch:     "main",
		CheckoutBranch: "feature/x",
	})
	if err != nil {
		t.Fatalf("AddBranchToTask should not surface materialize error pre-launch: %v", err)
	}
	if added.CheckoutBranch != "feature/x" {
		t.Errorf("unexpected row: %+v", added)
	}
	rows, _ := repo.ListTaskRepositories(ctx, task.ID)
	if len(rows) != 2 {
		t.Errorf("expected row to remain after pre-launch materialize failure; got %d rows", len(rows))
	}
}

func TestAddBranchToTask_RejectsActiveTaskBeforePersisting(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-add-idle", Name: "WS"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-add-idle", WorkspaceID: "ws-add-idle", Name: "WF"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateRepository(ctx, &models.Repository{ID: "repo-add-idle", WorkspaceID: "ws-add-idle", Name: "app", DefaultBranch: "main"}); err != nil {
		t.Fatal(err)
	}
	task, err := svc.CreateTask(ctx, &CreateTaskRequest{WorkspaceID: "ws-add-idle", WorkflowID: "wf-add-idle", WorkflowStepID: "step", Title: "Task", Repositories: []TaskRepositoryInput{{RepositoryID: "repo-add-idle", BaseBranch: "main"}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: "session-add-idle", TaskID: task.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.StartTurn(ctx, "session-add-idle"); err != nil {
		t.Fatal(err)
	}

	_, err = svc.AddBranchToTask(ctx, AddBranchToTaskRequest{TaskID: task.ID, RepositoryID: "repo-add-idle", BaseBranch: "main", CheckoutBranch: "feature/source"})
	if !errors.Is(err, ErrWorkspaceSourceActive) {
		t.Fatalf("error = %v, want active-task rejection", err)
	}
	rows, err := repo.ListTaskRepositories(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("task repositories = %#v, want no added branch", rows)
	}
}
