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

// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-004.1
// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-004.9
func TestLaunchPreparedSessionRepairsInventoryBeforePreparedWorkspaceFastPath(t *testing.T) {
	repositoryPath, worktreePath := createExecutorPreservationFixture(t)
	repo := newMockRepository()
	const taskID = "task-prepared-fast-path-repair"
	const sessionID = "session-prepared-fast-path-repair"
	seedWorktreeExecutor(repo)
	repo.repositories["repo-front"] = &models.Repository{
		ID: "repo-front", Name: "frontend", Provider: "github", LocalPath: repositoryPath,
	}
	repo.taskRepositories["tr-1"] = &models.TaskRepository{
		ID: "tr-1", TaskID: taskID, RepositoryID: "repo-front", Position: 0, BaseBranch: "main",
	}
	repo.tasks[taskID] = &models.Task{ID: taskID, WorkspaceID: "ws-1", Title: "Prepared Fast Path"}
	repo.taskEnvironments["env-prepared-fast-path"] = &models.TaskEnvironment{
		ID: "env-prepared-fast-path", TaskID: taskID, ExecutorType: string(models.ExecutorTypeWorktree),
		Status: models.TaskEnvironmentStatusReady, WorkspacePath: worktreePath,
		Repos: []*models.TaskEnvironmentRepo{{
			ID: "environment-repo-prepared-fast-path", TaskEnvironmentID: "env-prepared-fast-path",
			RepositoryID: "repo-front", BranchSlug: "stale",
			WorktreeID: "worktree-recovery", WorktreePath: worktreePath,
			WorktreeBranch: "feature/recovery", Position: 0, Status: "active",
		}},
	}
	repo.taskEnvironmentRepos["env-prepared-fast-path"] = repo.taskEnvironments["env-prepared-fast-path"].Repos
	repo.sessions[sessionID] = &models.TaskSession{
		ID: sessionID, TaskID: taskID, TaskEnvironmentID: "env-prepared-fast-path",
		AgentProfileID: "profile-123", ExecutorID: models.ExecutorIDWorktree,
		State: models.TaskSessionStateCreated, StartedAt: time.Now(), UpdatedAt: time.Now(),
	}
	repo.executorsRunning[sessionID] = &models.ExecutorRunning{
		ID: "runtime-prepared-fast-path", SessionID: sessionID, TaskID: taskID,
		AgentExecutionID: "execution-prepared-fast-path", ExecutorID: models.ExecutorIDWorktree,
		Status: models.ExecutorRunningStatusPrepared, WorktreeID: "worktree-recovery",
		WorktreePath: worktreePath, WorktreeBranch: "feature/recovery",
	}

	manager := &mockAgentManager{
		getExecutionIDForSessionFunc: func(context.Context, string) (string, error) {
			return "execution-prepared-fast-path", nil
		},
	}
	exec := newTestExecutor(t, manager, repo)
	execution, err := exec.LaunchPreparedSession(
		context.Background(),
		&v1.Task{ID: taskID, WorkspaceID: "ws-1", Title: "Prepared Fast Path"},
		sessionID,
		LaunchOptions{AgentProfileID: "profile-123", ExecutorID: models.ExecutorIDWorktree, StartAgent: true},
	)
	if err != nil {
		t.Fatalf("LaunchPreparedSession: %v", err)
	}
	if execution.WorkspaceInventoryRecoveryReceipt == nil ||
		execution.WorkspaceInventoryRecoveryReceipt.ResultCode != models.WorkspaceInventoryRecoveryRepaired {
		t.Fatalf("prepared fast-path receipt = %+v, want committed repair", execution.WorkspaceInventoryRecoveryReceipt)
	}
	rows := repo.taskEnvironmentRepos["env-prepared-fast-path"]
	if len(rows) != 1 || rows[0].BranchSlug != "main" {
		t.Fatalf("prepared fast path bypassed canonical inventory repair: %+v", rows)
	}
	if manager.launchAgentCallCount != 0 {
		t.Fatalf("prepared fast path invoked full LaunchAgent %d times", manager.launchAgentCallCount)
	}
}

// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-004.7
// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-004.9
func TestLaunchPreparedSessionPreparedWorkspaceFastPathBlocksUnattestedRepair(t *testing.T) {
	repositoryPath, worktreePath := createExecutorPreservationFixture(t)
	repo := newMockRepository()
	const taskID = "task-prepared-fast-path-attestation"
	const sessionAID = "session-prepared-fast-path-attestation-a"
	const sessionBID = "session-prepared-fast-path-attestation-b"
	seedWorktreeExecutor(repo)
	repo.repositories["repo-front"] = &models.Repository{
		ID: "repo-front", Name: "frontend", Provider: "github", LocalPath: repositoryPath,
	}
	repo.taskRepositories["tr-1"] = &models.TaskRepository{
		ID: "tr-1", TaskID: taskID, RepositoryID: "repo-front", Position: 0, BaseBranch: "main",
	}
	repo.tasks[taskID] = &models.Task{ID: taskID, WorkspaceID: "ws-1", Title: "Prepared Attestation"}
	repo.taskEnvironments["env-prepared-attestation"] = &models.TaskEnvironment{
		ID: "env-prepared-attestation", TaskID: taskID, ExecutorType: string(models.ExecutorTypeWorktree),
		Status: models.TaskEnvironmentStatusReady, WorkspacePath: worktreePath,
		Repos: []*models.TaskEnvironmentRepo{{
			ID: "environment-repo-prepared-attestation", TaskEnvironmentID: "env-prepared-attestation",
			RepositoryID: "repo-front", BranchSlug: "stale",
			WorktreeID: "worktree-recovery", WorktreePath: worktreePath,
			WorktreeBranch: "feature/recovery", Position: 0, Status: "active",
		}},
	}
	repo.taskEnvironmentRepos["env-prepared-attestation"] = repo.taskEnvironments["env-prepared-attestation"].Repos
	repo.sessions[sessionAID] = &models.TaskSession{
		ID: sessionAID, TaskID: taskID, TaskEnvironmentID: "env-prepared-attestation",
		AgentProfileID: "profile-123", ExecutorID: models.ExecutorIDWorktree,
		State: models.TaskSessionStateCreated, StartedAt: time.Now(), UpdatedAt: time.Now(),
	}
	repo.sessions[sessionBID] = &models.TaskSession{
		ID: sessionBID, TaskID: taskID, TaskEnvironmentID: "env-prepared-attestation",
		AgentProfileID: "profile-123", ExecutorID: models.ExecutorIDWorktree,
		State: models.TaskSessionStateCreated, StartedAt: time.Now(), UpdatedAt: time.Now(),
	}
	repo.recordWorkspaceInventoryPostRepairAttestationFunc = func(
		context.Context, string, string, *models.WorkspaceInventoryPreservation, bool, time.Time,
	) error {
		return errors.New("attestation store unavailable")
	}

	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	task := repo.tasks[taskID].ToAPI()
	if _, err := exec.LaunchPreparedSession(context.Background(), task, sessionAID,
		LaunchOptions{AgentProfileID: "profile-123", ExecutorID: models.ExecutorIDWorktree, StartAgent: true},
	); !errors.Is(err, models.ErrWorkspaceReuseUnsafe) {
		t.Fatalf("session A repair error = %v, want fail-closed reuse error", err)
	}
	repo.executorsRunning[sessionBID] = &models.ExecutorRunning{
		ID: "runtime-prepared-attestation", SessionID: sessionBID, TaskID: taskID,
		AgentExecutionID: "execution-prepared-attestation", ExecutorID: models.ExecutorIDWorktree,
		Status: models.ExecutorRunningStatusPrepared, WorktreeID: "worktree-recovery",
		WorktreePath: worktreePath, WorktreeBranch: "feature/recovery",
	}
	manager := &mockAgentManager{
		getExecutionIDForSessionFunc: func(context.Context, string) (string, error) {
			return "execution-prepared-attestation", nil
		},
	}
	exec = newTestExecutor(t, manager, repo)

	if _, err := exec.LaunchPreparedSession(context.Background(), task, sessionBID,
		LaunchOptions{AgentProfileID: "profile-123", ExecutorID: models.ExecutorIDWorktree, StartAgent: true},
	); !errors.Is(err, models.ErrWorkspaceReuseUnsafe) {
		t.Fatalf("prepared fast path against unattested row error = %v, want fail-closed reuse error", err)
	}
	if manager.launchAgentCallCount != 0 {
		t.Fatalf("prepared fast path against unattested row invoked LaunchAgent %d times", manager.launchAgentCallCount)
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

// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-004.2
// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-004.9
//
// TestResumeSessionWithOptionsAutoRepairsExactDevIncidentAndLaunchesOnce
// reproduces the separate logged incident for task
// 24cab57c-8e2c-44cc-8214-c0600d559391 / branch "dev" through the production
// resume entry point. The incident evidence did not include a session ID, so
// the session identity here is synthetic while the task and branch identities
// remain exact.
func TestResumeSessionWithOptionsAutoRepairsExactDevIncidentAndLaunchesOnce(t *testing.T) {
	repositoryPath, worktreePath := createExecutorPreservationFixture(t)
	repo := newMockRepository()
	const taskID = "24cab57c-8e2c-44cc-8214-c0600d559391"
	const sessionID = "session-dev-inventory-recovery"
	const branch = "dev"
	seedWorktreeExecutor(repo)
	repo.repositories["repo-front"] = &models.Repository{
		ID: "repo-front", Name: "frontend", Provider: "github", LocalPath: repositoryPath,
	}
	repo.taskRepositories["tr-1"] = &models.TaskRepository{
		ID: "tr-1", TaskID: taskID, RepositoryID: "repo-front", Position: 0, BaseBranch: branch,
	}
	repo.tasks[taskID] = &models.Task{ID: taskID, WorkspaceID: "ws-1", Title: "Dev"}
	repo.taskEnvironments["env-dev"] = &models.TaskEnvironment{
		ID: "env-dev", TaskID: taskID, ExecutorType: string(models.ExecutorTypeWorktree),
		Status: models.TaskEnvironmentStatusReady, WorkspacePath: worktreePath,
		Repos: []*models.TaskEnvironmentRepo{{
			ID: "environment-repo-dev", TaskEnvironmentID: "env-dev",
			RepositoryID: "repo-front", BranchSlug: "stale",
			WorktreeID: "worktree-recovery", WorktreePath: worktreePath,
			WorktreeBranch: "feature/recovery", Position: 0, Status: "active",
		}},
	}
	repo.taskEnvironmentRepos["env-dev"] = repo.taskEnvironments["env-dev"].Repos
	repo.sessions[sessionID] = &models.TaskSession{
		ID: sessionID, TaskID: taskID, TaskEnvironmentID: "env-dev",
		RepositoryID: "repo-front", AgentProfileID: "profile-123",
		ExecutorID: models.ExecutorIDWorktree, State: models.TaskSessionStateFailed,
		StartedAt: time.Now(), UpdatedAt: time.Now(),
	}

	manager := &mockAgentManager{}
	exec := newTestExecutor(t, manager, repo)
	execution, err := exec.ResumeSessionWithOptions(
		context.Background(),
		repo.sessions[sessionID],
		true,
		ResumeOptions{
			RepairWorkspaceInventory:         true,
			WorkspaceInventoryIdempotencyKey: "exact-dev-incident",
		},
	)
	if err != nil {
		t.Fatalf("ResumeSessionWithOptions (dev reproduction): %v", err)
	}
	if manager.launchAgentCallCount != 1 {
		t.Fatalf("LaunchAgent calls = %d, want exactly 1", manager.launchAgentCallCount)
	}
	if execution.WorkspaceInventoryRecoveryReceipt == nil ||
		execution.WorkspaceInventoryRecoveryReceipt.ResultCode != models.WorkspaceInventoryRecoveryRepaired {
		t.Fatalf("execution receipt = %+v, want a committed repair", execution.WorkspaceInventoryRecoveryReceipt)
	}
	rows := repo.taskEnvironmentRepos["env-dev"]
	if len(rows) != 1 || rows[0].BranchSlug != branch {
		t.Fatalf("canonical inventory not repaired in place: %+v", rows)
	}
}

// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-004.7
// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-004.9
//
// TestLaunchPreparedSessionCrossSessionRetryCompletesUnattestedCommitBeforeLaunch
// proves the fresh/additional-session launch path (executor_execute.go) gates
// launch on durable post-repair attestation across genuinely DIFFERENT
// sessions, not merely across repeated calls that happen to reuse the same
// session (and therefore the same session-derived idempotency key). Session
// A's launch repairs the canonical row but its own attestation write
// crashes; a completely different session B then observes the
// now-canonical row. validateReuseEnvironmentInventory passes for session B
// immediately (the row already matches), so session B reaches the
// already-valid launch branch (attestedWorkspaceInventoryRowsReceipt)
// instead of the repair branch session A used, and a session-scoped receipt
// lookup would never find session A's receipt at all. This proves the
// row-scoped lookup still finds it and still blocks launch until durable
// positive attestation exists.
func TestLaunchPreparedSessionCrossSessionRetryCompletesUnattestedCommitBeforeLaunch(t *testing.T) {
	repositoryPath, worktreePath := createExecutorPreservationFixture(t)
	repo := newMockRepository()
	const taskID = "task-launch-cross-session"
	const sessionAID = "session-launch-cross-session-a"
	const sessionBID = "session-launch-cross-session-b"
	seedWorktreeExecutor(repo)
	repo.repositories["repo-front"] = &models.Repository{
		ID: "repo-front", Name: "frontend", Provider: "github", LocalPath: repositoryPath,
	}
	repo.taskRepositories["tr-1"] = &models.TaskRepository{
		ID: "tr-1", TaskID: taskID, RepositoryID: "repo-front", Position: 0, BaseBranch: "main",
	}
	repo.tasks[taskID] = &models.Task{ID: taskID, WorkspaceID: "ws-1", Title: "Cross Session"}
	repo.taskEnvironments["env-cross-session"] = &models.TaskEnvironment{
		ID: "env-cross-session", TaskID: taskID, ExecutorType: string(models.ExecutorTypeWorktree),
		Status: models.TaskEnvironmentStatusReady, WorkspacePath: worktreePath,
		Repos: []*models.TaskEnvironmentRepo{{
			ID: "environment-repo-cross-session", TaskEnvironmentID: "env-cross-session",
			RepositoryID: "repo-front", BranchSlug: "stale",
			WorktreeID: "worktree-recovery", WorktreePath: worktreePath,
			WorktreeBranch: "feature/recovery", Position: 0, Status: "active",
		}},
	}
	repo.taskEnvironmentRepos["env-cross-session"] = repo.taskEnvironments["env-cross-session"].Repos
	repo.sessions[sessionAID] = &models.TaskSession{
		ID: sessionAID, TaskID: taskID, TaskEnvironmentID: "env-cross-session",
		AgentProfileID: "profile-123", ExecutorID: models.ExecutorIDWorktree,
		State: models.TaskSessionStateCreated, StartedAt: time.Now(), UpdatedAt: time.Now(),
	}
	repo.sessions[sessionBID] = &models.TaskSession{
		ID: sessionBID, TaskID: taskID, TaskEnvironmentID: "env-cross-session",
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
	task := &v1.Task{ID: taskID, WorkspaceID: "ws-1", Title: "Cross Session"}

	// Session A's launch repairs the canonical row (branch_slug -> "main")
	// but its own attestation write fails, leaving a committed, unattested
	// receipt behind.
	if _, err := exec.LaunchPreparedSession(context.Background(), task, sessionAID,
		LaunchOptions{AgentProfileID: "profile-123", ExecutorID: models.ExecutorIDWorktree, StartAgent: true},
	); !errors.Is(err, models.ErrWorkspaceReuseUnsafe) {
		t.Fatalf("session A initial repair error = %v, want fail-closed reuse error", err)
	}
	if manager.launchAgentCallCount != 0 {
		t.Fatalf("session A unattested repair launched agent %d times", manager.launchAgentCallCount)
	}
	if got := repo.taskEnvironmentRepos["env-cross-session"][0].BranchSlug; got != "main" {
		t.Fatalf("test setup did not commit the repair row before attestation failure: branch_slug=%q", got)
	}

	// Session B is a completely different session first observing the
	// already-canonical row. It must still be blocked while session A's
	// repair remains unattested.
	if _, err := exec.LaunchPreparedSession(context.Background(), task, sessionBID,
		LaunchOptions{AgentProfileID: "profile-123", ExecutorID: models.ExecutorIDWorktree, StartAgent: true},
	); !errors.Is(err, models.ErrWorkspaceReuseUnsafe) {
		t.Fatalf("session B launch against unattested cross-session row error = %v, want fail-closed reuse error", err)
	}
	if manager.launchAgentCallCount != 0 {
		t.Fatalf("session B launch against unattested cross-session row launched agent %d times", manager.launchAgentCallCount)
	}

	// Once the attestation store recovers, session B's retry completes the
	// attestation session A's repair never durably recorded, and launches
	// exactly once.
	repo.recordWorkspaceInventoryPostRepairAttestationFunc = nil
	execution, err := exec.LaunchPreparedSession(context.Background(), task, sessionBID,
		LaunchOptions{AgentProfileID: "profile-123", ExecutorID: models.ExecutorIDWorktree, StartAgent: true},
	)
	if err != nil {
		t.Fatalf("session B retry after attestation store recovery: %v", err)
	}
	if manager.launchAgentCallCount != 1 {
		t.Fatalf("session B retry after attestation store recovery launched agent %d times, want 1", manager.launchAgentCallCount)
	}
	if execution.WorkspaceInventoryRecoveryReceipt == nil ||
		!execution.WorkspaceInventoryRecoveryReceipt.PostRepairMatched ||
		execution.WorkspaceInventoryRecoveryReceipt.PostRepairVerifiedAt == nil {
		t.Fatalf("session B retry did not complete session A's unattested repair: %+v", execution.WorkspaceInventoryRecoveryReceipt)
	}
	if execution.WorkspaceInventoryRecoveryReceipt.SessionID != sessionAID {
		t.Fatalf("completed receipt should remain owned by session A's original repair, got session_id=%q",
			execution.WorkspaceInventoryRecoveryReceipt.SessionID)
	}
}

// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-004.7
// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-004.9
//
// TestResumeCrossSessionRetryCompletesUnattestedLaunchCommitBeforeLaunch proves
// the resume path (executor_resume.go) is gated by the same cross-session
// attestation requirement, when the unattested commit originated from the
// OTHER call site: a fresh/additional-session launch (session A, via
// LaunchPreparedSession) repairs the canonical row but crashes before its
// attestation write lands, and a later RESUME of a DIFFERENT session C (a
// realistic sequence: an auto-start launch crashes, and the task is later
// resumed) must not be admitted until that repair is durably attested.
func TestResumeCrossSessionRetryCompletesUnattestedLaunchCommitBeforeLaunch(t *testing.T) {
	repositoryPath, worktreePath := createExecutorPreservationFixture(t)
	repo := newMockRepository()
	const taskID = "task-resume-cross-session"
	const sessionAID = "session-resume-cross-session-a"
	const sessionCID = "session-resume-cross-session-c"
	seedWorktreeExecutor(repo)
	repo.repositories["repo-front"] = &models.Repository{
		ID: "repo-front", Name: "frontend", Provider: "github", LocalPath: repositoryPath,
	}
	repo.taskRepositories["tr-1"] = &models.TaskRepository{
		ID: "tr-1", TaskID: taskID, RepositoryID: "repo-front", Position: 0, BaseBranch: "main",
	}
	repo.tasks[taskID] = &models.Task{ID: taskID, WorkspaceID: "ws-1", Title: "Resume Cross Session"}
	repo.taskEnvironments["env-resume-cross-session"] = &models.TaskEnvironment{
		ID: "env-resume-cross-session", TaskID: taskID, ExecutorType: string(models.ExecutorTypeWorktree),
		Status: models.TaskEnvironmentStatusReady, WorkspacePath: worktreePath,
		Repos: []*models.TaskEnvironmentRepo{{
			ID: "environment-repo-resume-cross-session", TaskEnvironmentID: "env-resume-cross-session",
			RepositoryID: "repo-front", BranchSlug: "stale",
			WorktreeID: "worktree-recovery", WorktreePath: worktreePath,
			WorktreeBranch: "feature/recovery", Position: 0, Status: "active",
		}},
	}
	repo.taskEnvironmentRepos["env-resume-cross-session"] = repo.taskEnvironments["env-resume-cross-session"].Repos
	repo.sessions[sessionAID] = &models.TaskSession{
		ID: sessionAID, TaskID: taskID, TaskEnvironmentID: "env-resume-cross-session",
		AgentProfileID: "profile-123", ExecutorID: models.ExecutorIDWorktree,
		State: models.TaskSessionStateCreated, StartedAt: time.Now(), UpdatedAt: time.Now(),
	}
	repo.sessions[sessionCID] = &models.TaskSession{
		ID: sessionCID, TaskID: taskID, TaskEnvironmentID: "env-resume-cross-session",
		RepositoryID: "repo-front", ExecutorID: models.ExecutorIDWorktree,
		AgentProfileID: "profile-123", State: models.TaskSessionStateFailed,
		StartedAt: time.Now(), UpdatedAt: time.Now(),
	}
	repo.recordWorkspaceInventoryPostRepairAttestationFunc = func(
		context.Context, string, string, *models.WorkspaceInventoryPreservation, bool, time.Time,
	) error {
		return errors.New("attestation store unavailable")
	}

	manager := &mockAgentManager{}
	exec := newTestExecutor(t, manager, repo)
	task := repo.tasks[taskID].ToAPI()

	// Session A's fresh launch repairs the canonical row but its own
	// attestation write fails, leaving a committed, unattested receipt.
	if _, err := exec.LaunchPreparedSession(context.Background(), task, sessionAID,
		LaunchOptions{AgentProfileID: "profile-123", ExecutorID: models.ExecutorIDWorktree, StartAgent: true},
	); !errors.Is(err, models.ErrWorkspaceReuseUnsafe) {
		t.Fatalf("session A initial repair error = %v, want fail-closed reuse error", err)
	}
	if manager.launchAgentCallCount != 0 {
		t.Fatalf("session A unattested repair launched agent %d times", manager.launchAgentCallCount)
	}
	if got := repo.taskEnvironmentRepos["env-resume-cross-session"][0].BranchSlug; got != "main" {
		t.Fatalf("test setup did not commit the repair row before attestation failure: branch_slug=%q", got)
	}

	// Session C is a completely different session resuming the same task.
	// validateReuseEnvironmentInventory already passes for it (the row is
	// canonical), so resume reaches the already-valid branch
	// (attestedWorkspaceInventoryRowsReceipt) rather than the repair branch
	// session A used. It must still be blocked while session A's repair
	// remains unattested.
	options := ResumeOptions{RepairWorkspaceInventory: true, WorkspaceInventoryIdempotencyKey: "resume-cross-session-c-key"}
	if _, _, _, _, _, err := exec.buildResumeRequestAtCredentialBoundaryWithOptions(
		context.Background(), task, repo.sessions[sessionCID], false, nil, options,
	); !errors.Is(err, models.ErrWorkspaceInventoryRecoveryConflict) {
		t.Fatalf("session C resume against unattested cross-session row error = %v, want recovery conflict", err)
	}

	// Once the attestation store recovers, session C's resume completes the
	// attestation session A's repair never durably recorded.
	repo.recordWorkspaceInventoryPostRepairAttestationFunc = nil
	freshSessionC := *repo.sessions[sessionCID]
	req, _, _, _, _, err := exec.buildResumeRequestAtCredentialBoundaryWithOptions(
		context.Background(), task, &freshSessionC, false, nil, options,
	)
	if err != nil {
		t.Fatalf("session C resume after attestation store recovery: %v", err)
	}
	if req.WorkspaceInventoryRecoveryReceipt == nil ||
		!req.WorkspaceInventoryRecoveryReceipt.PostRepairMatched ||
		req.WorkspaceInventoryRecoveryReceipt.PostRepairVerifiedAt == nil {
		t.Fatalf("session C resume did not complete session A's unattested repair: %+v", req.WorkspaceInventoryRecoveryReceipt)
	}
	if req.WorkspaceInventoryRecoveryReceipt.SessionID != sessionAID {
		t.Fatalf("completed receipt should remain owned by session A's original repair, got session_id=%q",
			req.WorkspaceInventoryRecoveryReceipt.SessionID)
	}
}

// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-004.7
// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-004.9
//
// TestNormalResumeCrossSessionUnattestedLaunchCommitBlocksBeforeLaunch proves
// the ordinary resume path, not just the explicit repair action, gates an
// already-valid row that another session repaired before crashing or failing
// before durable post-repair attestation. A normal resume has no repair option
// set, but it must still refuse to launch from a repaired row until that row's
// receipt is positively attested.
func TestNormalResumeCrossSessionUnattestedLaunchCommitBlocksBeforeLaunch(t *testing.T) {
	repositoryPath, worktreePath := createExecutorPreservationFixture(t)
	repo := newMockRepository()
	const taskID = "task-normal-resume-cross-session"
	const sessionAID = "session-normal-resume-cross-session-a"
	const sessionCID = "session-normal-resume-cross-session-c"
	seedWorktreeExecutor(repo)
	repo.repositories["repo-front"] = &models.Repository{
		ID: "repo-front", Name: "frontend", Provider: "github", LocalPath: repositoryPath,
	}
	repo.taskRepositories["tr-1"] = &models.TaskRepository{
		ID: "tr-1", TaskID: taskID, RepositoryID: "repo-front", Position: 0, BaseBranch: "main",
	}
	repo.tasks[taskID] = &models.Task{ID: taskID, WorkspaceID: "ws-1", Title: "Normal Resume Cross Session"}
	repo.taskEnvironments["env-normal-resume-cross-session"] = &models.TaskEnvironment{
		ID: "env-normal-resume-cross-session", TaskID: taskID, ExecutorType: string(models.ExecutorTypeWorktree),
		Status: models.TaskEnvironmentStatusReady, WorkspacePath: worktreePath,
		Repos: []*models.TaskEnvironmentRepo{{
			ID: "environment-repo-normal-resume-cross-session", TaskEnvironmentID: "env-normal-resume-cross-session",
			RepositoryID: "repo-front", BranchSlug: "stale",
			WorktreeID: "worktree-recovery", WorktreePath: worktreePath,
			WorktreeBranch: "feature/recovery", Position: 0, Status: "active",
		}},
	}
	repo.taskEnvironmentRepos["env-normal-resume-cross-session"] = repo.taskEnvironments["env-normal-resume-cross-session"].Repos
	repo.sessions[sessionAID] = &models.TaskSession{
		ID: sessionAID, TaskID: taskID, TaskEnvironmentID: "env-normal-resume-cross-session",
		AgentProfileID: "profile-123", ExecutorID: models.ExecutorIDWorktree,
		State: models.TaskSessionStateCreated, StartedAt: time.Now(), UpdatedAt: time.Now(),
	}
	repo.sessions[sessionCID] = &models.TaskSession{
		ID: sessionCID, TaskID: taskID, TaskEnvironmentID: "env-normal-resume-cross-session",
		RepositoryID: "repo-front", ExecutorID: models.ExecutorIDWorktree,
		AgentProfileID: "profile-123", State: models.TaskSessionStateFailed,
		StartedAt: time.Now(), UpdatedAt: time.Now(),
	}
	repo.recordWorkspaceInventoryPostRepairAttestationFunc = func(
		context.Context, string, string, *models.WorkspaceInventoryPreservation, bool, time.Time,
	) error {
		return errors.New("attestation store unavailable")
	}

	manager := &mockAgentManager{}
	exec := newTestExecutor(t, manager, repo)
	task := repo.tasks[taskID].ToAPI()
	if _, err := exec.LaunchPreparedSession(context.Background(), task, sessionAID,
		LaunchOptions{AgentProfileID: "profile-123", ExecutorID: models.ExecutorIDWorktree, StartAgent: true},
	); !errors.Is(err, models.ErrWorkspaceReuseUnsafe) {
		t.Fatalf("session A initial repair error = %v, want fail-closed reuse error", err)
	}
	if got := repo.taskEnvironmentRepos["env-normal-resume-cross-session"][0].BranchSlug; got != "main" {
		t.Fatalf("test setup did not commit the repair row before attestation failure: branch_slug=%q", got)
	}

	_, err := exec.ResumeSession(context.Background(), repo.sessions[sessionCID], true)
	if !errors.Is(err, models.ErrWorkspaceInventoryRecoveryConflict) {
		t.Fatalf("normal resume against unattested cross-session row error = %v, want recovery conflict", err)
	}
	if manager.launchAgentCallCount != 0 {
		t.Fatalf("normal resume against unattested cross-session row launched agent %d times", manager.launchAgentCallCount)
	}

	repo.recordWorkspaceInventoryPostRepairAttestationFunc = nil
	execution, err := exec.ResumeSession(context.Background(), repo.sessions[sessionCID], true)
	if err != nil {
		t.Fatalf("normal resume after attestation store recovery: %v", err)
	}
	if manager.launchAgentCallCount != 1 {
		t.Fatalf("normal resume after attestation store recovery launched agent %d times, want 1", manager.launchAgentCallCount)
	}
	if execution.WorkspaceInventoryRecoveryReceipt == nil ||
		!execution.WorkspaceInventoryRecoveryReceipt.PostRepairMatched ||
		execution.WorkspaceInventoryRecoveryReceipt.PostRepairVerifiedAt == nil {
		t.Fatalf("normal resume did not complete durable post-repair attestation: %+v", execution.WorkspaceInventoryRecoveryReceipt)
	}
	if execution.WorkspaceInventoryRecoveryReceipt.SessionID != sessionAID {
		t.Fatalf("completed receipt should remain owned by session A's original repair, got session_id=%q",
			execution.WorkspaceInventoryRecoveryReceipt.SessionID)
	}
}
