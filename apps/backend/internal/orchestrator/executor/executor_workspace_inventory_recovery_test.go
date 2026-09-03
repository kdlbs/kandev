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
	// This test's custom repairWorkspaceInventoryFunc bypasses the mock's
	// default receipt store, so the default post-repair attestation lookup
	// (keyed on that store) would find nothing and fail closed. Stub durable
	// attestation persistence directly: this test proves checkout
	// preservation, not attestation-store mechanics.
	mockRepo.recordWorkspaceInventoryPostRepairAttestationFunc = func(
		context.Context, string, string, *models.WorkspaceInventoryPreservation, bool, time.Time,
	) error {
		return nil
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

// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-004.7
// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-004.9
//
// TestRepairReuseEnvironmentInventoryRetryAfterUnattestedCommitSafelyCompletesAttestation
// proves the unsafe retry boundary is closed: a repair transaction can
// commit and then crash — or its post-repair attestation write can itself
// fail — leaving a committed receipt with no attestation durably recorded
// (PostRepairVerifiedAt == nil). A retry with the same idempotency key must
// not hand that receipt back as success; it must safely complete the
// attestation now, by re-inspecting the exact preserved checkout, before it
// can ever be reused, and it must never re-run the repair transaction or
// duplicate the canonical row.
func TestRepairReuseEnvironmentInventoryRetryAfterUnattestedCommitSafelyCompletesAttestation(t *testing.T) {
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
	attestationAttempts := 0
	mockRepo.recordWorkspaceInventoryPostRepairAttestationFunc = func(
		_ context.Context, taskID, idempotencyKey string,
		evidence *models.WorkspaceInventoryPreservation, matched bool, verifiedAt time.Time,
	) error {
		attestationAttempts++
		if attestationAttempts == 1 {
			// Simulate a crash (or a failed write) between the repair
			// transaction committing and its attestation persisting.
			return errors.New("simulated crash before attestation persists")
		}
		key := taskID + "\x00" + idempotencyKey
		existing, ok := mockRepo.workspaceInventoryReceipts[key]
		if !ok {
			return models.ErrWorkspaceInventoryRecoveryInvalid
		}
		updated := *existing
		updated.PostRepairEvidence = evidence
		updated.PostRepairMatched = matched
		verified := verifiedAt
		updated.PostRepairVerifiedAt = &verified
		mockRepo.workspaceInventoryReceipts[key] = &updated
		return nil
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

	first, err := executor.repairReuseEnvironmentInventory(context.Background(), task, session, req, env, repositories, "crash-key")
	if !errors.Is(err, models.ErrWorkspaceInventoryRecoveryConflict) {
		t.Fatalf("first repairReuseEnvironmentInventory error = %v, want retryable conflict", err)
	}
	if first != nil {
		t.Fatalf("first repair returned an in-memory success receipt despite failed durable attestation: %+v", first)
	}
	stored := mockRepo.workspaceInventoryReceipts["task\x00crash-key"]
	if stored == nil || stored.PostRepairVerifiedAt != nil {
		t.Fatalf("stored receipt should be unattested after simulated crash: %+v", stored)
	}

	// Simulate the canonical-inventory reload that happens between calls in
	// production: the repaired row now matches.
	env.Repos[0].BranchSlug = "main"

	second, err := executor.repairReuseEnvironmentInventory(context.Background(), task, session, req, env, repositories, "crash-key")
	if err != nil {
		t.Fatalf("retry after unattested commit returned an error instead of safely completing attestation: %v", err)
	}
	if second.ID != stored.ID {
		t.Fatalf("retry did not complete the existing receipt: %+v", second)
	}
	if !second.PostRepairMatched || second.PostRepairEvidence == nil || second.PostRepairVerifiedAt == nil {
		t.Fatalf("retry did not durably complete post-repair attestation: %+v", second)
	}
	if attestationAttempts != 2 {
		t.Fatalf("attestation attempts = %d, want exactly 2 (initial failed write + retry completion)", attestationAttempts)
	}
	if len(mockRepo.taskEnvironmentRepos[env.ID]) != 1 {
		t.Fatalf("retry duplicated the canonical inventory row: %+v", mockRepo.taskEnvironmentRepos[env.ID])
	}
}

// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-004.7
// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-004.9
//
// TestRepairReuseEnvironmentInventoryRetryWithStoredNegativeAttestationConflicts
// proves a receipt already carrying a negative/divergent post-repair
// attestation (a prior attempt detected the checkout changed during the
// metadata repair) is never retried as success. It must remain a stable,
// typed conflict on every subsequent retry, not flip back to success.
func TestRepairReuseEnvironmentInventoryRetryWithStoredNegativeAttestationConflicts(t *testing.T) {
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

	first, err := executor.repairReuseEnvironmentInventory(context.Background(), task, session, req, env, repositories, "divergent-key")
	if err != nil {
		t.Fatalf("first repairReuseEnvironmentInventory: %v", err)
	}
	if first.ResultCode != models.WorkspaceInventoryRecoveryRepaired {
		t.Fatalf("first result code = %q", first.ResultCode)
	}

	// Simulate a prior attempt whose post-repair inspection detected and
	// durably recorded a divergent checkout.
	key := "task\x00divergent-key"
	stored := *mockRepo.workspaceInventoryReceipts[key]
	stored.PostRepairMatched = false
	verifiedAt := time.Now().UTC()
	stored.PostRepairVerifiedAt = &verifiedAt
	mockRepo.workspaceInventoryReceipts[key] = &stored

	env.Repos[0].BranchSlug = "main"

	if _, err := executor.repairReuseEnvironmentInventory(context.Background(), task, session, req, env, repositories, "divergent-key"); !errors.Is(err, models.ErrWorkspaceInventoryRecoveryConflict) {
		t.Fatalf("retry with stored negative attestation error = %v, want conflict", err)
	}
	// A second retry must still be refused; a negative attestation never
	// self-heals into success.
	if _, err := executor.repairReuseEnvironmentInventory(context.Background(), task, session, req, env, repositories, "divergent-key"); !errors.Is(err, models.ErrWorkspaceInventoryRecoveryConflict) {
		t.Fatalf("second retry with stored negative attestation error = %v, want conflict", err)
	}
}

// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-004.2
//
// TestRepairReuseEnvironmentInventoryPreservationRecordsExecutorRuntimeEvidence
// proves the durable preservation evidence captures the authoritative
// executor/runtime record (models.ExecutorRunning) in effect for the
// session, not just the session's own lifecycle state string.
func TestRepairReuseEnvironmentInventoryPreservationRecordsExecutorRuntimeEvidence(t *testing.T) {
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
		executorsRunning: map[string]*models.ExecutorRunning{
			"session": {
				SessionID: "session", TaskID: "task", ExecutorID: "executor-worktree",
				Status: "running", AgentExecutionID: "agent-execution-1",
			},
		},
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

	receipt, err := executor.repairReuseEnvironmentInventory(context.Background(), task, session, req, env, repositories, "runtime-evidence-key")
	if err != nil {
		t.Fatalf("repairReuseEnvironmentInventory: %v", err)
	}
	if receipt.Preservation.ExecutorID != "executor-worktree" ||
		receipt.Preservation.ExecutorStatus != "running" ||
		receipt.Preservation.AgentExecutionID != "agent-execution-1" {
		t.Fatalf("preservation missing authoritative executor/runtime evidence: %+v", receipt.Preservation)
	}
	if receipt.PostRepairEvidence == nil || receipt.PostRepairEvidence.ExecutorID != "executor-worktree" {
		t.Fatalf("post-repair evidence missing authoritative executor/runtime evidence: %+v", receipt.PostRepairEvidence)
	}
}

// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-004.6
//
// TestRepairPathIsTaskScopedRejectsParentDirectorySymlinkEscape proves
// canonical task-root ownership resolves parent-directory symlinks rather
// than comparing lexical paths: a symlink placed in a directory *above* the
// candidate worktree path must not let a path outside the task workspace
// root lexically compare as scoped to it.
func TestRepairPathIsTaskScopedRejectsParentDirectorySymlinkEscape(t *testing.T) {
	outsideRoot := t.TempDir()
	outsideTarget := filepath.Join(outsideRoot, "outside-worktree")
	if err := os.Mkdir(outsideTarget, 0o755); err != nil {
		t.Fatal(err)
	}

	taskRoot := t.TempDir()
	// linkedParent lexically lives under the task root, but is a symlink to
	// a directory entirely outside it; escaped/worktree therefore resolves
	// outside taskRoot even though the lexical path starts with taskRoot.
	linkedParent := filepath.Join(taskRoot, "linked-parent")
	if err := os.Symlink(outsideRoot, linkedParent); err != nil {
		t.Fatal(err)
	}
	escapedWorktreePath := filepath.Join(taskRoot, "linked-parent", "outside-worktree")

	env := &models.TaskEnvironment{WorkspacePath: taskRoot, TaskDirName: "task-dir"}
	if repairPathIsTaskScoped(env, escapedWorktreePath) {
		t.Fatalf("parent-directory symlink escape was accepted as task-scoped: %q under %q", escapedWorktreePath, taskRoot)
	}

	// A real, non-symlinked worktree directly under the task root must still
	// be accepted.
	genuineWorktree := filepath.Join(taskRoot, "genuine-worktree")
	if err := os.Mkdir(genuineWorktree, 0o755); err != nil {
		t.Fatal(err)
	}
	if !repairPathIsTaskScoped(env, genuineWorktree) {
		t.Fatalf("genuine task-scoped worktree was rejected: %q under %q", genuineWorktree, taskRoot)
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
func TestRepairReuseEnvironmentInventoryRetryWithChangedBranchIdentityConflicts(t *testing.T) {
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

	if _, err := executor.repairReuseEnvironmentInventory(context.Background(), task, session, req, env, repositories, "reused-key"); err != nil {
		t.Fatalf("first repairReuseEnvironmentInventory: %v", err)
	}
	env.Repos[0].BranchSlug = "main"
	req.Repositories[0].BranchIdentitySlug = "renamed"

	_, err := executor.repairReuseEnvironmentInventory(context.Background(), task, session, req, env, repositories, "reused-key")
	if !errors.Is(err, models.ErrWorkspaceInventoryRecoveryIdempotencyConflict) {
		t.Fatalf("changed-payload idempotency-key reuse error = %v", err)
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
