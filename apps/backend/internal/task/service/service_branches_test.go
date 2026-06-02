package service

import (
	"context"
	"strings"
	"testing"
	"time"

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
		Provider: "github", ProviderOwner: "acme", ProviderName: "widgets",
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
