package executor

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-004.7
// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-004.9
//
// TestBuildResumeRequestRetryAfterUnattestedCommitBlocksBeforeLaunch proves
// the orchestrator-facing resume path does not skip durable attestation once
// the previous repair already fixed the canonical inventory row. A crash or
// failed attestation write happens after the row commit, so the next retry
// can pass validateReuseEnvironmentInventory; it still must not proceed
// unless the existing receipt is durably attested.
func TestBuildResumeRequestRetryAfterUnattestedCommitBlocksBeforeLaunch(t *testing.T) {
	repositoryPath, worktreePath := createExecutorPreservationFixture(t)
	repo := newMockRepository()
	const taskID = "task-resume-unattested"
	const sessionID = "session-resume-unattested"
	seedWorktreeExecutor(repo)
	repo.repositories["repo-front"] = &models.Repository{
		ID: "repo-front", Name: "frontend", Provider: "github", LocalPath: repositoryPath,
	}
	repo.taskRepositories["tr-1"] = &models.TaskRepository{
		ID: "tr-1", TaskID: taskID, RepositoryID: "repo-front", Position: 0, BaseBranch: "main",
	}
	repo.tasks[taskID] = &models.Task{ID: taskID, WorkspaceID: "ws-1", Title: "Resume"}
	repo.taskEnvironments["env-unattested"] = &models.TaskEnvironment{
		ID: "env-unattested", TaskID: taskID, ExecutorType: string(models.ExecutorTypeWorktree),
		Status: models.TaskEnvironmentStatusReady, WorkspacePath: worktreePath,
		Repos: []*models.TaskEnvironmentRepo{{
			ID: "environment-repo-unattested", TaskEnvironmentID: "env-unattested",
			RepositoryID: "repo-front", BranchSlug: "stale",
			WorktreeID: "worktree-recovery", WorktreePath: worktreePath,
			WorktreeBranch: "feature/recovery", Position: 0, Status: "active",
		}},
	}
	repo.taskEnvironmentRepos["env-unattested"] = repo.taskEnvironments["env-unattested"].Repos
	repo.sessions[sessionID] = &models.TaskSession{
		ID: sessionID, TaskID: taskID, TaskEnvironmentID: "env-unattested",
		RepositoryID: "repo-front", ExecutorID: models.ExecutorIDWorktree,
		AgentProfileID: "profile-123", State: models.TaskSessionStateFailed,
		StartedAt: time.Now(), UpdatedAt: time.Now(),
	}
	repo.recordWorkspaceInventoryPostRepairAttestationFunc = func(
		context.Context, string, string, *models.WorkspaceInventoryPreservation, bool, time.Time,
	) error {
		return errors.New("attestation store unavailable")
	}

	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	task := repo.tasks[taskID].ToAPI()
	options := ResumeOptions{RepairWorkspaceInventory: true, WorkspaceInventoryIdempotencyKey: "resume-unattested-key"}

	if _, _, _, _, _, err := exec.buildResumeRequestAtCredentialBoundaryWithOptions(
		context.Background(), task, repo.sessions[sessionID], false, nil, options,
	); !errors.Is(err, models.ErrWorkspaceInventoryRecoveryConflict) {
		t.Fatalf("initial resume repair error = %v, want unattested repair conflict", err)
	}
	if got := repo.taskEnvironmentRepos["env-unattested"][0].BranchSlug; got != "main" {
		t.Fatalf("test setup did not commit the repair row before attestation failure: branch_slug=%q", got)
	}
	if stored := repo.workspaceInventoryReceipts[taskID+"\x00resume-unattested-key"]; stored == nil || stored.PostRepairVerifiedAt != nil {
		t.Fatalf("stored receipt should exist without durable attestation: %+v", stored)
	}

	freshSession := *repo.sessions[sessionID]
	if _, _, _, _, _, err := exec.buildResumeRequestAtCredentialBoundaryWithOptions(
		context.Background(), task, &freshSession, false, nil, options,
	); !errors.Is(err, models.ErrWorkspaceInventoryRecoveryConflict) {
		t.Fatalf("retry after unattested committed row error = %v, want recovery conflict", err)
	}

	repo.recordWorkspaceInventoryPostRepairAttestationFunc = nil
	freshSession = *repo.sessions[sessionID]
	req, _, _, _, _, err := exec.buildResumeRequestAtCredentialBoundaryWithOptions(
		context.Background(), task, &freshSession, false, nil, options,
	)
	if err != nil {
		t.Fatalf("retry after attestation store recovery: %v", err)
	}
	if req.WorkspaceInventoryRecoveryReceipt == nil ||
		!req.WorkspaceInventoryRecoveryReceipt.PostRepairMatched ||
		req.WorkspaceInventoryRecoveryReceipt.PostRepairVerifiedAt == nil {
		t.Fatalf("retry did not complete durable post-repair attestation: %+v", req.WorkspaceInventoryRecoveryReceipt)
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

// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-004.7
// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-004.9
//
// TestLaunchPreparedSessionRetryAfterUnattestedCommitDoesNotLaunch proves
// fresh/additional-session launch applies the same durable-attestation gate
// even when the prior failed repair attempt already corrected the canonical
// row enough for validateReuseEnvironmentInventory to pass on retry.
func TestLaunchPreparedSessionRetryAfterUnattestedCommitDoesNotLaunch(t *testing.T) {
	repositoryPath, worktreePath := createExecutorPreservationFixture(t)
	repo := newMockRepository()
	const taskID = "task-launch-unattested"
	const sessionID = "session-launch-unattested"
	seedWorktreeExecutor(repo)
	repo.repositories["repo-front"] = &models.Repository{
		ID: "repo-front", Name: "frontend", Provider: "github", LocalPath: repositoryPath,
	}
	repo.taskRepositories["tr-1"] = &models.TaskRepository{
		ID: "tr-1", TaskID: taskID, RepositoryID: "repo-front", Position: 0, BaseBranch: "main",
	}
	repo.tasks[taskID] = &models.Task{ID: taskID, WorkspaceID: "ws-1", Title: "Auto Repair"}
	repo.taskEnvironments["env-launch-unattested"] = &models.TaskEnvironment{
		ID: "env-launch-unattested", TaskID: taskID, ExecutorType: string(models.ExecutorTypeWorktree),
		Status: models.TaskEnvironmentStatusReady, WorkspacePath: worktreePath,
		Repos: []*models.TaskEnvironmentRepo{{
			ID: "environment-repo-launch-unattested", TaskEnvironmentID: "env-launch-unattested",
			RepositoryID: "repo-front", BranchSlug: "stale",
			WorktreeID: "worktree-recovery", WorktreePath: worktreePath,
			WorktreeBranch: "feature/recovery", Position: 0, Status: "active",
		}},
	}
	repo.taskEnvironmentRepos["env-launch-unattested"] = repo.taskEnvironments["env-launch-unattested"].Repos
	repo.sessions[sessionID] = &models.TaskSession{
		ID: sessionID, TaskID: taskID, TaskEnvironmentID: "env-launch-unattested",
		AgentProfileID: "profile-123", ExecutorID: models.ExecutorIDWorktree,
		State: models.TaskSessionStateCreated, StartedAt: time.Now(), UpdatedAt: time.Now(),
	}
	repo.recordWorkspaceInventoryPostRepairAttestationFunc = func(
		context.Context, string, string, *models.WorkspaceInventoryPreservation, bool, time.Time,
	) error {
		return errors.New("attestation store unavailable")
	}

	manager := &mockAgentManager{}
	exec := newTestExecutor(t, manager, repo)
	task := &v1.Task{ID: taskID, WorkspaceID: "ws-1", Title: "Auto Repair"}
	if _, err := exec.LaunchPreparedSession(context.Background(), task, sessionID,
		LaunchOptions{AgentProfileID: "profile-123", ExecutorID: models.ExecutorIDWorktree, StartAgent: true},
	); !errors.Is(err, models.ErrWorkspaceReuseUnsafe) {
		t.Fatalf("initial launch repair error = %v, want original fail-closed reuse error", err)
	}
	if manager.launchAgentCallCount != 0 {
		t.Fatalf("initial unattested repair launched agent %d times", manager.launchAgentCallCount)
	}
	if got := repo.taskEnvironmentRepos["env-launch-unattested"][0].BranchSlug; got != "main" {
		t.Fatalf("test setup did not commit the repair row before attestation failure: branch_slug=%q", got)
	}

	if _, err := exec.LaunchPreparedSession(context.Background(), task, sessionID,
		LaunchOptions{AgentProfileID: "profile-123", ExecutorID: models.ExecutorIDWorktree, StartAgent: true},
	); !errors.Is(err, models.ErrWorkspaceReuseUnsafe) {
		t.Fatalf("retry after unattested committed row error = %v, want fail-closed reuse error", err)
	}
	if manager.launchAgentCallCount != 0 {
		t.Fatalf("retry after unattested committed row launched agent %d times", manager.launchAgentCallCount)
	}

	repo.recordWorkspaceInventoryPostRepairAttestationFunc = nil
	execution, err := exec.LaunchPreparedSession(context.Background(), task, sessionID,
		LaunchOptions{AgentProfileID: "profile-123", ExecutorID: models.ExecutorIDWorktree, StartAgent: true},
	)
	if err != nil {
		t.Fatalf("retry after attestation store recovery: %v", err)
	}
	if manager.launchAgentCallCount != 1 {
		t.Fatalf("retry after attestation store recovery launched agent %d times, want 1", manager.launchAgentCallCount)
	}
	if execution.WorkspaceInventoryRecoveryReceipt == nil ||
		!execution.WorkspaceInventoryRecoveryReceipt.PostRepairMatched ||
		execution.WorkspaceInventoryRecoveryReceipt.PostRepairVerifiedAt == nil {
		t.Fatalf("retry did not complete durable post-repair attestation: %+v", execution.WorkspaceInventoryRecoveryReceipt)
	}
}

// TestLaunchPreparedSessionAutoRepairsExactStagingPy3IncidentAndLaunchesOnce
// reproduces the exact field incident using the real reported identifiers —
// task 96cfb14c-62f4-4048-bc03-813f1f123875, session
// be3a413d-0891-4982-a563-b631028c36c6, branch "staging-py3" — with a single
// provable stale canonical row. It proves the guarded recovery path repairs
// exactly that row and admits exactly one Human-QA auto-start launch, rather
// than repeating the originally logged
// "canonical workspace repository inventory has no matching entry" failure.
func TestLaunchPreparedSessionAutoRepairsExactStagingPy3IncidentAndLaunchesOnce(t *testing.T) {
	repositoryPath, worktreePath := createExecutorPreservationFixture(t)
	repo := newMockRepository()
	const taskID = "96cfb14c-62f4-4048-bc03-813f1f123875"
	const sessionID = "be3a413d-0891-4982-a563-b631028c36c6"
	const branch = "staging-py3"
	seedWorktreeExecutor(repo)
	repo.repositories["repo-front"] = &models.Repository{
		ID: "repo-front", Name: "frontend", Provider: "github", LocalPath: repositoryPath,
	}
	repo.taskRepositories["tr-1"] = &models.TaskRepository{
		ID: "tr-1", TaskID: taskID, RepositoryID: "repo-front", Position: 0, BaseBranch: branch,
	}
	repo.tasks[taskID] = &models.Task{ID: taskID, WorkspaceID: "ws-1", Title: "Staging Py3"}
	repo.taskEnvironments["env-staging-py3"] = &models.TaskEnvironment{
		ID: "env-staging-py3", TaskID: taskID, ExecutorType: string(models.ExecutorTypeWorktree),
		Status: models.TaskEnvironmentStatusReady, WorkspacePath: worktreePath,
		Repos: []*models.TaskEnvironmentRepo{{
			ID: "environment-repo-staging-py3", TaskEnvironmentID: "env-staging-py3",
			RepositoryID: "repo-front", BranchSlug: "stale",
			WorktreeID: "worktree-recovery", WorktreePath: worktreePath,
			WorktreeBranch: "feature/recovery", Position: 0, Status: "active",
		}},
	}
	repo.taskEnvironmentRepos["env-staging-py3"] = repo.taskEnvironments["env-staging-py3"].Repos
	repo.sessions[sessionID] = &models.TaskSession{
		ID: sessionID, TaskID: taskID, TaskEnvironmentID: "env-staging-py3",
		AgentProfileID: "profile-123", ExecutorID: models.ExecutorIDWorktree,
		State: models.TaskSessionStateCreated, StartedAt: time.Now(), UpdatedAt: time.Now(),
	}

	manager := &mockAgentManager{}
	exec := newTestExecutor(t, manager, repo)
	execution, err := exec.LaunchPreparedSession(context.Background(),
		&v1.Task{ID: taskID, WorkspaceID: "ws-1", Title: "Staging Py3"}, sessionID,
		LaunchOptions{AgentProfileID: "profile-123", ExecutorID: models.ExecutorIDWorktree, StartAgent: true})
	if err != nil {
		t.Fatalf("LaunchPreparedSession (staging-py3 reproduction): %v", err)
	}
	if manager.launchAgentCallCount != 1 {
		t.Fatalf("LaunchAgent calls = %d, want exactly 1", manager.launchAgentCallCount)
	}
	if execution.WorkspaceInventoryRecoveryReceipt == nil ||
		execution.WorkspaceInventoryRecoveryReceipt.ResultCode != models.WorkspaceInventoryRecoveryRepaired {
		t.Fatalf("execution receipt = %+v, want a committed repair", execution.WorkspaceInventoryRecoveryReceipt)
	}
	rows := repo.taskEnvironmentRepos["env-staging-py3"]
	if len(rows) != 1 || rows[0].BranchSlug != branch {
		t.Fatalf("canonical inventory not repaired in place: %+v", rows)
	}
}
