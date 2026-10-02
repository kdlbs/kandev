package executor

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-008.6
// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-008.10
//
// TestAdmitLaunchWorkspaceInventoryBlocksWhenAnotherRepairedSlotIsUnattested
// proves automatic multi-repository launch validates durable post-repair
// attestation for every requested canonical slot after it repairs a mismatch.
// The secondary repair commits before its attestation write fails; a later
// primary repair must not let that stale receipt bypass launch admission.
func TestAdmitLaunchWorkspaceInventoryBlocksWhenAnotherRepairedSlotIsUnattested(t *testing.T) {
	taskRoot := t.TempDir()
	frontRepositoryPath, frontWorktreePath := createNestedExecutorPreservationFixture(t, taskRoot, "frontend")
	backRepositoryPath, backWorktreePath := createNestedExecutorPreservationFixture(t, taskRoot, "backend")

	const taskID = "task-all-slot-attestation"
	const sessionID = "session-all-slot-attestation"
	const environmentID = "environment-all-slot-attestation"
	const unattestedKey = "secondary-unattested-repair"

	repo := newMockRepository()
	seedWorktreeExecutor(repo)
	repo.repositories["repo-front"] = &models.Repository{
		ID: "repo-front", Name: "frontend", Provider: "github", LocalPath: frontRepositoryPath,
	}
	repo.repositories["repo-back"] = &models.Repository{
		ID: "repo-back", Name: "backend", Provider: "github", LocalPath: backRepositoryPath,
	}
	repo.taskRepositories["tr-front"] = &models.TaskRepository{
		ID: "tr-front", TaskID: taskID, RepositoryID: "repo-front", Position: 0, BaseBranch: "main",
	}
	repo.taskRepositories["tr-back"] = &models.TaskRepository{
		ID: "tr-back", TaskID: taskID, RepositoryID: "repo-back", Position: 1, BaseBranch: "main",
	}
	repo.tasks[taskID] = &models.Task{ID: taskID, WorkspaceID: "workspace-all-slot", Title: "All slot attestation"}
	repo.sessions[sessionID] = &models.TaskSession{
		ID: sessionID, TaskID: taskID, TaskEnvironmentID: environmentID,
		ExecutorID: models.ExecutorIDWorktree, State: models.TaskSessionStateCreated,
		StartedAt: time.Now(), UpdatedAt: time.Now(),
	}

	environment := &models.TaskEnvironment{
		ID: environmentID, TaskID: taskID, ExecutorType: string(models.ExecutorTypeWorktree),
		Status: models.TaskEnvironmentStatusReady, WorkspacePath: taskRoot, TaskDirName: "task-all-slot",
		Repos: []*models.TaskEnvironmentRepo{
			{
				ID: "environment-repo-front", TaskEnvironmentID: environmentID, RepositoryID: "repo-front",
				BranchSlug: "stale", WorktreeID: "worktree-frontend", WorktreePath: frontWorktreePath,
				WorktreeBranch: "feature/recovery", Position: 0, Status: "active",
			},
			{
				ID: "environment-repo-back", TaskEnvironmentID: environmentID, RepositoryID: "repo-back",
				BranchSlug: "stale", WorktreeID: "worktree-backend", WorktreePath: backWorktreePath,
				WorktreeBranch: "feature/recovery", Position: 1, Status: "active",
			},
		},
	}
	repo.taskEnvironments[environmentID] = environment
	repo.taskEnvironmentRepos[environmentID] = environment.Repos

	frontInfo := &repoInfo{TaskRepositoryID: "tr-front", RepositoryID: "repo-front", RepositoryPath: frontRepositoryPath, Position: 0, Repository: repo.repositories["repo-front"]}
	backInfo := &repoInfo{TaskRepositoryID: "tr-back", RepositoryID: "repo-back", RepositoryPath: backRepositoryPath, Position: 1, Repository: repo.repositories["repo-back"]}
	allRepositories := []*repoInfo{frontInfo, backInfo}
	secondaryRequest := workspaceInventoryLaunchRequest(taskID, environmentID, []RepoSpec{{
		TaskRepositoryID: "tr-back", RepositoryID: "repo-back", BaseBranch: "main", BranchIdentitySlug: "main",
	}})

	repo.recordWorkspaceInventoryPostRepairAttestationFunc = func(
		_ context.Context, receivedTaskID, key string, evidence *models.WorkspaceInventoryPreservation, matched bool, verifiedAt time.Time,
	) error {
		if key == unattestedKey {
			return errors.New("secondary attestation write interrupted")
		}
		existing := repo.workspaceInventoryReceipts[receivedTaskID+"\x00"+key]
		if existing == nil {
			return models.ErrWorkspaceInventoryRecoveryInvalid
		}
		updated := *existing
		updated.PostRepairEvidence = evidence
		updated.PostRepairMatched = matched
		updated.PostRepairVerifiedAt = &verifiedAt
		repo.workspaceInventoryReceipts[receivedTaskID+"\x00"+key] = &updated
		return nil
	}
	executor := newTestExecutor(t, &mockAgentManager{}, repo)
	task := repo.tasks[taskID].ToAPI()

	if _, err := executor.repairReuseEnvironmentInventory(
		context.Background(), task, repo.sessions[sessionID], secondaryRequest, environment, []*repoInfo{backInfo}, unattestedKey,
	); !errors.Is(err, models.ErrWorkspaceInventoryRecoveryConflict) {
		t.Fatalf("secondary repair without durable attestation error = %v, want recovery conflict", err)
	}
	if got := repo.taskEnvironmentRepos[environmentID][1].BranchSlug; got != "main" {
		t.Fatalf("secondary repair did not commit canonical row before attestation failure: branch_slug=%q", got)
	}
	if receipt := repo.workspaceInventoryReceipts[taskID+"\x00"+unattestedKey]; receipt == nil || receipt.PostRepairVerifiedAt != nil {
		t.Fatalf("secondary receipt should be committed without durable attestation: %+v", receipt)
	}

	launchRequest := workspaceInventoryLaunchRequest(taskID, environmentID, []RepoSpec{
		{TaskRepositoryID: "tr-front", RepositoryID: "repo-front", BaseBranch: "main", BranchIdentitySlug: "main"},
		{TaskRepositoryID: "tr-back", RepositoryID: "repo-back", BaseBranch: "main", BranchIdentitySlug: "main"},
	})
	if err := executor.admitLaunchWorkspaceInventory(
		context.Background(), task, repo.sessions[sessionID], launchRequest, environment, allRepositories,
	); !errors.Is(err, models.ErrWorkspaceReuseUnsafe) {
		t.Fatalf("automatic launch with another unattested repaired slot error = %v, want fail-closed reuse error", err)
	}
	if got := repo.taskEnvironmentRepos[environmentID][0].BranchSlug; got != "main" {
		t.Fatalf("automatic launch did not repair primary slot before all-slot attestation gate: branch_slug=%q", got)
	}

	// Once attestation storage recovers, the next admission completes the
	// secondary receipt and permits launch only after both slots are positive.
	repo.recordWorkspaceInventoryPostRepairAttestationFunc = nil
	if err := executor.admitLaunchWorkspaceInventory(
		context.Background(), task, repo.sessions[sessionID], launchRequest, environment, allRepositories,
	); err != nil {
		t.Fatalf("automatic launch after all-slot attestation recovery: %v", err)
	}
	if receipt := repo.workspaceInventoryReceipts[taskID+"\x00"+unattestedKey]; receipt == nil ||
		!receipt.PostRepairMatched || receipt.PostRepairVerifiedAt == nil {
		t.Fatalf("secondary receipt was not durably attested after retry: %+v", receipt)
	}
}

func workspaceInventoryLaunchRequest(taskID, environmentID string, repositories []RepoSpec) *LaunchAgentRequest {
	return &LaunchAgentRequest{
		TaskID: taskID, TaskEnvironmentID: environmentID,
		WorkspaceReuseRequired: true, UseWorktree: true, Repositories: repositories,
	}
}

func createNestedExecutorPreservationFixture(t *testing.T, taskRoot, name string) (string, string) {
	t.Helper()
	repositoryPath := filepath.Join(taskRoot, name+"-repository")
	worktreePath := filepath.Join(taskRoot, name)
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
