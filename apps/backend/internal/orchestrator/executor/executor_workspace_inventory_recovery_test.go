package executor

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/worktree"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-004.5
func TestSelectWorkspaceInventoryRepairTargetRequiresExactlyOneCanonicalMismatch(t *testing.T) {
	infoA := &repoInfo{TaskRepositoryID: "task-repo-a", RepositoryID: "repo-a", Position: 0}
	infoB := &repoInfo{TaskRepositoryID: "task-repo-b", RepositoryID: "repo-b", Position: 1}
	req := &LaunchAgentRequest{UseWorktree: true, Repositories: []RepoSpec{
		{TaskRepositoryID: infoA.TaskRepositoryID, RepositoryID: infoA.RepositoryID, BranchIdentitySlug: "main"},
		{TaskRepositoryID: infoB.TaskRepositoryID, RepositoryID: infoB.RepositoryID, BranchIdentitySlug: "main"},
	}}
	env := &models.TaskEnvironment{Repos: []*models.TaskEnvironmentRepo{
		{ID: "stale-a", RepositoryID: "repo-a", Position: 0, BranchSlug: "wrong", WorktreeID: "wt-a", WorktreePath: "/synthetic/a", WorktreeBranch: "feature/a"},
		{ID: "stale-b", RepositoryID: "repo-b", Position: 1, BranchSlug: "wrong", WorktreeID: "wt-b", WorktreePath: "/synthetic/b", WorktreeBranch: "feature/b"},
	}}

	_, _, _, err := selectWorkspaceInventoryRepairTarget(req, env, &models.TaskSession{}, []*repoInfo{infoA, infoB})
	if !errors.Is(err, models.ErrWorkspaceInventoryRecoveryConflict) {
		t.Fatalf("ambiguous repair error = %v", err)
	}
}

// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-004.2
func TestWorkspaceInventoryRepairSessionUsesOnlyMatchingServerRuntimeIdentity(t *testing.T) {
	mockRepo := &mockRepository{executorsRunning: map[string]*models.ExecutorRunning{
		"session": {
			SessionID: "session", TaskID: "task", WorktreeID: "worktree",
			WorktreePath: "/synthetic/worktree", WorktreeBranch: "feature/recovery",
		},
	}}
	executor := &Executor{repo: mockRepo}
	session, err := executor.workspaceInventoryRepairSession(
		context.Background(),
		&LaunchAgentRequest{Repositories: []RepoSpec{{TaskRepositoryID: "task-repo", RepositoryID: "repo"}}},
		&models.TaskEnvironment{ID: "environment"},
		&models.TaskSession{ID: "session", TaskID: "task"},
		[]*repoInfo{{TaskRepositoryID: "task-repo", RepositoryID: "repo", Position: 4}},
	)
	if err != nil {
		t.Fatalf("workspaceInventoryRepairSession: %v", err)
	}
	if len(session.Worktrees) != 1 || session.Worktrees[0].RepositoryID != "repo" ||
		session.Worktrees[0].Position != 4 || session.Worktrees[0].WorktreeID != "worktree" {
		t.Fatalf("fallback identity = %+v", session.Worktrees)
	}

	mockRepo.executorsRunning["session"].TaskID = "other-task"
	_, err = executor.workspaceInventoryRepairSession(
		context.Background(),
		&LaunchAgentRequest{Repositories: []RepoSpec{{TaskRepositoryID: "task-repo", RepositoryID: "repo"}}},
		&models.TaskEnvironment{ID: "environment"},
		&models.TaskSession{ID: "session", TaskID: "task"},
		[]*repoInfo{{TaskRepositoryID: "task-repo", RepositoryID: "repo", Position: 4}},
	)
	if !errors.Is(err, models.ErrWorkspaceInventoryRecoveryConflict) {
		t.Fatalf("cross-task runtime error = %v", err)
	}
}

// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-004.2
// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-004.3
func TestRepairReuseEnvironmentInventoryPreservesDirtyCheckout(t *testing.T) {
	repositoryPath, worktreePath := createExecutorPreservationFixture(t)
	if err := os.WriteFile(filepath.Join(worktreePath, "README.md"), []byte("dirty\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(worktreePath, "untracked.txt"), []byte("keep me\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	before, err := worktree.InspectPreservedCheckout(context.Background(), worktree.PreservationRequest{
		RepositoryPath: repositoryPath, WorktreePath: worktreePath,
		ExpectedBranch: "feature/recovery", WorktreeID: "worktree-recovery",
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	row := &models.TaskEnvironmentRepo{
		ID: "environment-repo", TaskEnvironmentID: "environment", RepositoryID: "repository",
		BranchSlug: "stale", WorktreeID: "worktree-recovery", WorktreePath: worktreePath,
		WorktreeBranch: "feature/recovery", Position: 0, Status: "active", UpdatedAt: now,
	}
	env := &models.TaskEnvironment{
		ID: "environment", TaskID: "task", WorkspacePath: worktreePath,
		Status: models.TaskEnvironmentStatusReady, UpdatedAt: now,
		Repos: []*models.TaskEnvironmentRepo{row},
	}
	mockRepo := &mockRepository{
		taskEnvironmentRepos: map[string][]*models.TaskEnvironmentRepo{env.ID: env.Repos},
	}
	mockRepo.repairWorkspaceInventoryFunc = func(_ context.Context, repair *models.WorkspaceInventoryRepair) (*models.WorkspaceInventoryRecoveryReceipt, error) {
		if repair.TaskID != "task" || repair.WorkspaceID != "workspace" ||
			repair.TaskEnvironmentID != env.ID || repair.EnvironmentRepoID != row.ID ||
			repair.BranchSlug != "main" || repair.WorktreePath != worktreePath {
			t.Fatalf("repair escaped proven slot: %+v", repair)
		}
		row.BranchSlug = repair.BranchSlug
		return &models.WorkspaceInventoryRecoveryReceipt{
			ID: "receipt", ResultCode: models.WorkspaceInventoryRecoveryRepaired,
			Preservation: repair.Preservation,
		}, nil
	}
	executor := &Executor{repo: mockRepo}
	receipt, err := executor.repairReuseEnvironmentInventory(
		context.Background(),
		&v1.Task{ID: "task", WorkspaceID: "workspace"},
		&models.TaskSession{ID: "session", TaskID: "task", TaskEnvironmentID: env.ID, State: models.TaskSessionStateFailed},
		&LaunchAgentRequest{TaskID: "task", WorkspaceID: "workspace", UseWorktree: true, WorkspaceReuseRequired: true,
			Repositories: []RepoSpec{{TaskRepositoryID: "task-repository", RepositoryID: "repository", BranchIdentitySlug: "main"}}},
		env,
		[]*repoInfo{{
			TaskRepositoryID: "task-repository", TaskRepositoryUpdatedAt: now,
			RepositoryID: "repository", RepositoryPath: repositoryPath, Position: 0,
			Repository: &models.Repository{ID: "repository", SourceType: "github"},
		}},
		"repair-once",
	)
	if err != nil {
		t.Fatalf("repairReuseEnvironmentInventory: %v", err)
	}
	if receipt.ResultCode != models.WorkspaceInventoryRecoveryRepaired || receipt.Preservation.PathHash == worktreePath {
		t.Fatalf("unsafe receipt = %+v", receipt)
	}
	after, err := worktree.InspectPreservedCheckout(context.Background(), worktree.PreservationRequest{
		RepositoryPath: repositoryPath, WorktreePath: worktreePath,
		ExpectedBranch: "feature/recovery", WorktreeID: "worktree-recovery",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !samePreservationEvidence(before, after) || string(mustReadFile(t, filepath.Join(worktreePath, "untracked.txt"))) != "keep me\n" {
		t.Fatalf("checkout changed: before=%+v after=%+v", before, after)
	}
}

func createExecutorPreservationFixture(t *testing.T) (string, string) {
	t.Helper()
	repositoryPath := filepath.Join(t.TempDir(), "repository")
	worktreePath := filepath.Join(t.TempDir(), "preserved")
	if err := os.Mkdir(repositoryPath, 0o755); err != nil {
		t.Fatal(err)
	}
	runExecutorGit(t, repositoryPath, "init", "-b", "main")
	runExecutorGit(t, repositoryPath, "config", "user.email", "fixture@example.com")
	runExecutorGit(t, repositoryPath, "config", "user.name", "Fixture")
	if err := os.WriteFile(filepath.Join(repositoryPath, "README.md"), []byte("clean\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runExecutorGit(t, repositoryPath, "add", "README.md")
	runExecutorGit(t, repositoryPath, "commit", "-m", "fixture")
	runExecutorGit(t, repositoryPath, "branch", "feature/recovery")
	runExecutorGit(t, repositoryPath, "worktree", "add", worktreePath, "feature/recovery")
	return repositoryPath, worktreePath
}

func runExecutorGit(t *testing.T, directory string, args ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", directory}, args...)...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, output)
	}
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return contents
}

// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-004.3
func TestSelectWorkspaceInventoryRepairTargetKeepsOtherRepositoryCanonical(t *testing.T) {
	infoA := &repoInfo{TaskRepositoryID: "task-repo-a", RepositoryID: "repo-a", Position: 0}
	infoB := &repoInfo{TaskRepositoryID: "task-repo-b", RepositoryID: "repo-b", Position: 1}
	req := &LaunchAgentRequest{UseWorktree: true, Repositories: []RepoSpec{
		{TaskRepositoryID: infoA.TaskRepositoryID, RepositoryID: infoA.RepositoryID, BranchIdentitySlug: "main"},
		{TaskRepositoryID: infoB.TaskRepositoryID, RepositoryID: infoB.RepositoryID, BranchIdentitySlug: "main"},
	}}
	env := &models.TaskEnvironment{Repos: []*models.TaskEnvironmentRepo{
		{ID: "canonical-a", RepositoryID: "repo-a", Position: 0, BranchSlug: "main", WorktreeID: "wt-a", WorktreePath: "/synthetic/a", WorktreeBranch: "feature/a"},
		{ID: "stale-b", RepositoryID: "repo-b", Position: 1, BranchSlug: "wrong", WorktreeID: "wt-b", WorktreePath: "/synthetic/b", WorktreeBranch: "feature/b"},
	}}

	spec, info, candidate, err := selectWorkspaceInventoryRepairTarget(req, env, &models.TaskSession{}, []*repoInfo{infoA, infoB})
	if err != nil {
		t.Fatalf("select repair target: %v", err)
	}
	if spec.RepositoryID != "repo-b" || info != infoB || candidate.ID != "stale-b" {
		t.Fatalf("selected spec=%+v info=%+v candidate=%+v", spec, info, candidate)
	}
}
