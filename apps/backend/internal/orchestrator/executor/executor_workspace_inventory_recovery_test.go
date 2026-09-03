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

// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-004.9
func TestRepairReuseEnvironmentInventoryRetryAfterCommitReturnsStoredReceipt(t *testing.T) {
	repositoryPath, worktreePath := createExecutorPreservationFixture(t)
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
		taskEnvironmentRepos:       map[string][]*models.TaskEnvironmentRepo{env.ID: env.Repos},
		workspaceInventoryReceipts: map[string]*models.WorkspaceInventoryRecoveryReceipt{},
	}
	executor := &Executor{repo: mockRepo}
	task := &v1.Task{ID: "task", WorkspaceID: "workspace"}
	session := &models.TaskSession{ID: "session", TaskID: "task", TaskEnvironmentID: env.ID, State: models.TaskSessionStateFailed}
	req := &LaunchAgentRequest{TaskID: "task", WorkspaceID: "workspace", UseWorktree: true, WorkspaceReuseRequired: true,
		Repositories: []RepoSpec{{TaskRepositoryID: "task-repository", RepositoryID: "repository", BranchIdentitySlug: "main"}}}
	repositories := []*repoInfo{{
		TaskRepositoryID: "task-repository", TaskRepositoryUpdatedAt: now,
		RepositoryID: "repository", RepositoryPath: repositoryPath, Position: 0,
		Repository: &models.Repository{ID: "repository", SourceType: "github"},
	}}

	first, err := executor.repairReuseEnvironmentInventory(context.Background(), task, session, req, env, repositories, "retry-key")
	if err != nil {
		t.Fatalf("first repairReuseEnvironmentInventory: %v", err)
	}
	if first.ResultCode != models.WorkspaceInventoryRecoveryRepaired {
		t.Fatalf("first result code = %q", first.ResultCode)
	}
	if !first.PostRepairMatched || first.PostRepairEvidence == nil || first.PostRepairVerifiedAt == nil {
		t.Fatalf("first receipt missing post-repair attestation: %+v", first)
	}

	// Simulate the canonical-inventory reload that happens between calls in
	// production: the repaired row now matches, so a retry that reached
	// candidate selection would find zero unmatched slots and misreport a
	// conflict without the short-circuit this test proves.
	env.Repos[0].BranchSlug = "main"

	second, err := executor.repairReuseEnvironmentInventory(context.Background(), task, session, req, env, repositories, "retry-key")
	if err != nil {
		t.Fatalf("retry after commit returned error instead of stored receipt: %v", err)
	}
	if second.ID != first.ID || second.ResultCode != models.WorkspaceInventoryRecoveryDeduplicated {
		t.Fatalf("retry receipt = %+v, want deduplicated copy of %+v", second, first)
	}
}

// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-004.9
func TestRepairReuseEnvironmentInventoryRetryFromDifferentSessionConflicts(t *testing.T) {
	repositoryPath, worktreePath := createExecutorPreservationFixture(t)
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
		taskEnvironmentRepos:       map[string][]*models.TaskEnvironmentRepo{env.ID: env.Repos},
		workspaceInventoryReceipts: map[string]*models.WorkspaceInventoryRecoveryReceipt{},
	}
	executor := &Executor{repo: mockRepo}
	task := &v1.Task{ID: "task", WorkspaceID: "workspace"}
	req := &LaunchAgentRequest{TaskID: "task", WorkspaceID: "workspace", UseWorktree: true, WorkspaceReuseRequired: true,
		Repositories: []RepoSpec{{TaskRepositoryID: "task-repository", RepositoryID: "repository", BranchIdentitySlug: "main"}}}
	repositories := []*repoInfo{{
		TaskRepositoryID: "task-repository", TaskRepositoryUpdatedAt: now,
		RepositoryID: "repository", RepositoryPath: repositoryPath, Position: 0,
		Repository: &models.Repository{ID: "repository", SourceType: "github"},
	}}
	session := &models.TaskSession{ID: "session", TaskID: "task", TaskEnvironmentID: env.ID, State: models.TaskSessionStateFailed}

	if _, err := executor.repairReuseEnvironmentInventory(context.Background(), task, session, req, env, repositories, "reused-key"); err != nil {
		t.Fatalf("first repairReuseEnvironmentInventory: %v", err)
	}

	other := &models.TaskSession{ID: "other-session", TaskID: "task", TaskEnvironmentID: env.ID, State: models.TaskSessionStateFailed}
	_, err := executor.repairReuseEnvironmentInventory(context.Background(), task, other, req, env, repositories, "reused-key")
	if !errors.Is(err, models.ErrWorkspaceInventoryRecoveryIdempotencyConflict) {
		t.Fatalf("cross-session idempotency-key reuse error = %v", err)
	}
}

// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-004.9
//
// TestBuildResumeRequestRetryAfterCommittedRepairSucceeds proves the
// orchestrator-facing resume entry point (not just the low-level repair
// primitive) tolerates a caller retry with the same idempotency key once a
// repair has already committed: the first call repairs and resumes once, and
// a second call built from a freshly reloaded session/environment (mirroring
// a real caller retry) neither re-triggers a destructive repair nor surfaces
// ErrWorkspaceInventoryRecoveryIdempotencyConflict — it observes the already
// -corrected canonical row and proceeds.
func TestBuildResumeRequestRetryAfterCommittedRepairSucceeds(t *testing.T) {
	repositoryPath, worktreePath := createExecutorPreservationFixture(t)
	repo := newMockRepository()
	const taskID = "task-resume-retry"
	const sessionID = "session-resume-retry"
	seedWorktreeExecutor(repo)
	repo.repositories["repo-front"] = &models.Repository{
		ID: "repo-front", Name: "frontend", Provider: "github", LocalPath: repositoryPath,
	}
	repo.taskRepositories["tr-1"] = &models.TaskRepository{
		ID: "tr-1", TaskID: taskID, RepositoryID: "repo-front", Position: 0, BaseBranch: "main",
	}
	repo.tasks[taskID] = &models.Task{ID: taskID, WorkspaceID: "ws-1", Title: "Resume"}
	repo.taskEnvironments["env-retry"] = &models.TaskEnvironment{
		ID: "env-retry", TaskID: taskID, ExecutorType: string(models.ExecutorTypeWorktree),
		Status: models.TaskEnvironmentStatusReady, WorkspacePath: worktreePath,
		Repos: []*models.TaskEnvironmentRepo{{
			ID: "environment-repo-retry", TaskEnvironmentID: "env-retry",
			RepositoryID: "repo-front", BranchSlug: "stale",
			WorktreeID: "worktree-recovery", WorktreePath: worktreePath,
			WorktreeBranch: "feature/recovery", Position: 0, Status: "active",
		}},
	}
	repo.taskEnvironmentRepos["env-retry"] = repo.taskEnvironments["env-retry"].Repos
	repo.sessions[sessionID] = &models.TaskSession{
		ID: sessionID, TaskID: taskID, TaskEnvironmentID: "env-retry",
		RepositoryID: "repo-front", ExecutorID: models.ExecutorIDWorktree,
		AgentProfileID: "profile-123", State: models.TaskSessionStateFailed,
		StartedAt: time.Now(), UpdatedAt: time.Now(),
	}

	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	task := repo.tasks[taskID].ToAPI()
	options := ResumeOptions{RepairWorkspaceInventory: true, WorkspaceInventoryIdempotencyKey: "orchestrator-retry-key"}

	req1, _, _, _, _, err := exec.buildResumeRequestAtCredentialBoundaryWithOptions(
		context.Background(), task, repo.sessions[sessionID], false, nil, options)
	if err != nil {
		t.Fatalf("first buildResumeRequestAtCredentialBoundaryWithOptions: %v", err)
	}
	if req1.WorkspaceInventoryRecoveryReceipt == nil || req1.WorkspaceInventoryRecoveryReceipt.ResultCode != models.WorkspaceInventoryRecoveryRepaired {
		t.Fatalf("first receipt = %+v, want a committed repair", req1.WorkspaceInventoryRecoveryReceipt)
	}

	// A retry re-reads the session the same way a real caller would before
	// calling resume again, rather than reusing the first call's in-memory
	// request/session objects.
	freshSession := *repo.sessions[sessionID]
	req2, _, _, _, _, err := exec.buildResumeRequestAtCredentialBoundaryWithOptions(
		context.Background(), task, &freshSession, false, nil, options)
	if err != nil {
		t.Fatalf("retry after committed repair returned an error instead of succeeding: %v", err)
	}
	if req2.WorkspaceInventoryRecoveryReceipt != nil &&
		req2.WorkspaceInventoryRecoveryReceipt.ResultCode != models.WorkspaceInventoryRecoveryRepaired &&
		req2.WorkspaceInventoryRecoveryReceipt.ResultCode != models.WorkspaceInventoryRecoveryDeduplicated {
		t.Fatalf("retry receipt reported an unexpected result code: %+v", req2.WorkspaceInventoryRecoveryReceipt)
	}
	if len(repo.taskEnvironmentRepos["env-retry"]) != 1 {
		t.Fatalf("retry duplicated the canonical inventory row: %+v", repo.taskEnvironmentRepos["env-retry"])
	}
}

// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-004.9
//
// TestLaunchPreparedSessionAutoRepairsRecoverableInventoryMismatchAndLaunchesOnce
// proves guarded repair is wired into fresh/additional-session launch (not
// just resume): LaunchPreparedSession self-heals one provably stale canonical
// row automatically, launches the agent exactly once, and surfaces the
// resulting receipt on the returned TaskExecution.
func TestLaunchPreparedSessionAutoRepairsRecoverableInventoryMismatchAndLaunchesOnce(t *testing.T) {
	repositoryPath, worktreePath := createExecutorPreservationFixture(t)
	repo := newMockRepository()
	const taskID = "task-launch-auto-repair"
	const sessionID = "session-launch-auto-repair"
	seedWorktreeExecutor(repo)
	repo.repositories["repo-front"] = &models.Repository{
		ID: "repo-front", Name: "frontend", Provider: "github", LocalPath: repositoryPath,
	}
	repo.taskRepositories["tr-1"] = &models.TaskRepository{
		ID: "tr-1", TaskID: taskID, RepositoryID: "repo-front", Position: 0, BaseBranch: "main",
	}
	repo.tasks[taskID] = &models.Task{ID: taskID, WorkspaceID: "ws-1", Title: "Auto Repair"}
	repo.taskEnvironments["env-auto-repair"] = &models.TaskEnvironment{
		ID: "env-auto-repair", TaskID: taskID, ExecutorType: string(models.ExecutorTypeWorktree),
		Status: models.TaskEnvironmentStatusReady, WorkspacePath: worktreePath,
		Repos: []*models.TaskEnvironmentRepo{{
			ID: "environment-repo-auto-repair", TaskEnvironmentID: "env-auto-repair",
			RepositoryID: "repo-front", BranchSlug: "stale",
			WorktreeID: "worktree-recovery", WorktreePath: worktreePath,
			WorktreeBranch: "feature/recovery", Position: 0, Status: "active",
		}},
	}
	repo.taskEnvironmentRepos["env-auto-repair"] = repo.taskEnvironments["env-auto-repair"].Repos
	repo.sessions[sessionID] = &models.TaskSession{
		ID: sessionID, TaskID: taskID, TaskEnvironmentID: "env-auto-repair",
		AgentProfileID: "profile-123", ExecutorID: models.ExecutorIDWorktree,
		State: models.TaskSessionStateCreated, StartedAt: time.Now(), UpdatedAt: time.Now(),
	}

	manager := &mockAgentManager{}
	exec := newTestExecutor(t, manager, repo)
	execution, err := exec.LaunchPreparedSession(context.Background(),
		&v1.Task{ID: taskID, WorkspaceID: "ws-1", Title: "Auto Repair"}, sessionID,
		LaunchOptions{AgentProfileID: "profile-123", ExecutorID: models.ExecutorIDWorktree, StartAgent: true})
	if err != nil {
		t.Fatalf("LaunchPreparedSession: %v", err)
	}
	if manager.launchAgentCallCount != 1 {
		t.Fatalf("LaunchAgent calls = %d, want exactly 1", manager.launchAgentCallCount)
	}
	if execution.WorkspaceInventoryRecoveryReceipt == nil ||
		execution.WorkspaceInventoryRecoveryReceipt.ResultCode != models.WorkspaceInventoryRecoveryRepaired {
		t.Fatalf("execution receipt = %+v, want a committed repair", execution.WorkspaceInventoryRecoveryReceipt)
	}
	rows := repo.taskEnvironmentRepos["env-auto-repair"]
	if len(rows) != 1 || rows[0].BranchSlug != "main" {
		t.Fatalf("canonical inventory not repaired in place: %+v", rows)
	}
}

// TestLaunchPreparedSessionFailedAutoRepairPreservesFailClosedErrorAndLeavesNoOrphanState
// proves that when automatic repair cannot resolve a mismatch (no provable
// single reciprocal identity, e.g. a repository entirely missing from
// inventory), LaunchPreparedSession still fails closed with the original
// ErrWorkspaceReuseUnsafe admission error, never calls LaunchAgent, and never
// promotes the session into a bad primary/orphan STARTING or RUNNING state.
func TestLaunchPreparedSessionFailedAutoRepairPreservesFailClosedErrorAndLeavesNoOrphanState(t *testing.T) {
	repo := newMockRepository()
	taskID := "task-launch-unrepairable"
	sessionID := "session-launch-unrepairable"
	seedMultiRepoTask(t, repo, taskID)
	seedWorktreeExecutor(repo)

	environmentRepos := []*models.TaskEnvironmentRepo{{
		TaskEnvironmentID: "env-unrepairable",
		RepositoryID:      "repo-front",
		BranchSlug:        "main",
		WorktreeID:        "wt-front",
		Status:            "active",
	}}
	environment := &models.TaskEnvironment{
		ID:           "env-unrepairable",
		TaskID:       taskID,
		ExecutorType: string(models.ExecutorTypeWorktree),
		Status:       models.TaskEnvironmentStatusReady,
		Repos:        environmentRepos,
	}
	repo.taskEnvironments[environment.ID] = environment
	repo.taskEnvironmentRepos[environment.ID] = environmentRepos
	repo.sessions[sessionID] = &models.TaskSession{
		ID: sessionID, TaskID: taskID, TaskEnvironmentID: environment.ID,
		AgentProfileID: "profile-123", ExecutorID: models.ExecutorIDWorktree,
		State: models.TaskSessionStateCreated, StartedAt: time.Now(), UpdatedAt: time.Now(),
	}

	manager := &mockAgentManager{}
	exec := newTestExecutor(t, manager, repo)
	_, err := exec.LaunchPreparedSession(context.Background(),
		&v1.Task{ID: taskID, WorkspaceID: "ws-1", Title: "Multi"}, sessionID,
		LaunchOptions{AgentProfileID: "profile-123", ExecutorID: models.ExecutorIDWorktree})
	if !errors.Is(err, models.ErrWorkspaceReuseUnsafe) {
		t.Fatalf("LaunchPreparedSession error = %v, want ErrWorkspaceReuseUnsafe preserved after failed auto-repair", err)
	}
	if manager.launchAgentCallCount != 0 {
		t.Fatalf("LaunchAgent calls = %d, want 0", manager.launchAgentCallCount)
	}
	if got := repo.sessions[sessionID].State; got != models.TaskSessionStateCreated {
		t.Fatalf("session state = %q, want unchanged CREATED (no orphan STARTING/RUNNING state)", got)
	}
	if len(repo.taskEnvironmentRepos[environment.ID]) != 1 {
		t.Fatalf("failed repair mutated canonical inventory: %+v", repo.taskEnvironmentRepos[environment.ID])
	}
}
