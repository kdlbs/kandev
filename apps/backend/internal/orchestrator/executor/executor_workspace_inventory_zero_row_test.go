package executor

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-004.2
// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-004.9
func TestLaunchPreparedSessionAutoRepairsZeroInventoryFromPriorEnvironmentRuntime(t *testing.T) {
	repositoryPath, worktreePath := createExecutorPreservationFixture(t)
	repo := newMockRepository()
	const (
		taskID         = "task-zero-inventory"
		environmentID  = "env-zero-inventory"
		priorSessionID = "session-materialized"
		freshSessionID = "session-fresh"
	)
	seedWorktreeExecutor(repo)
	repo.repositories["repo-front"] = &models.Repository{
		ID: "repo-front", Name: "frontend", Provider: "github", LocalPath: repositoryPath,
	}
	repo.taskRepositories["tr-1"] = &models.TaskRepository{
		ID: "tr-1", TaskID: taskID, RepositoryID: "repo-front", Position: 0, BaseBranch: "staging-py3",
	}
	repo.tasks[taskID] = &models.Task{ID: taskID, WorkspaceID: "ws-1", Title: "Zero inventory"}
	repo.taskEnvironments[environmentID] = &models.TaskEnvironment{
		ID: environmentID, TaskID: taskID, ExecutorType: string(models.ExecutorTypeWorktree),
		Status: models.TaskEnvironmentStatusReady, WorkspacePath: worktreePath,
	}
	repo.taskEnvironmentRepos[environmentID] = nil
	repo.sessions[priorSessionID] = &models.TaskSession{
		ID: priorSessionID, TaskID: taskID, TaskEnvironmentID: environmentID,
		AgentProfileID: "profile-123", ExecutorID: models.ExecutorIDWorktree,
		State: models.TaskSessionStateFailed, StartedAt: time.Now(), UpdatedAt: time.Now(),
	}
	repo.sessions[freshSessionID] = &models.TaskSession{
		ID: freshSessionID, TaskID: taskID, TaskEnvironmentID: environmentID,
		AgentProfileID: "profile-123", ExecutorID: models.ExecutorIDWorktree,
		State: models.TaskSessionStateCreated, StartedAt: time.Now(), UpdatedAt: time.Now(),
	}
	repo.executorsRunning[priorSessionID] = &models.ExecutorRunning{
		ID: "runtime-prior", SessionID: priorSessionID, TaskID: taskID,
		ExecutorID: models.ExecutorIDWorktree, Status: models.ExecutorRunningStatusStopped,
		WorktreeID: "worktree-recovery", WorktreePath: worktreePath, WorktreeBranch: "feature/recovery",
	}
	repo.getExecutorRunningFunc = func(_ context.Context, sessionID string) (*models.ExecutorRunning, error) {
		if sessionID == freshSessionID {
			return nil, models.ErrExecutorRunningNotFound
		}
		return repo.executorsRunning[sessionID], nil
	}

	manager := &mockAgentManager{}
	exec := newTestExecutor(t, manager, repo)
	execution, err := exec.LaunchPreparedSession(
		context.Background(),
		&v1.Task{ID: taskID, WorkspaceID: "ws-1", Title: "Zero inventory"},
		freshSessionID,
		LaunchOptions{AgentProfileID: "profile-123", ExecutorID: models.ExecutorIDWorktree, StartAgent: true},
	)
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
	preservation := execution.WorkspaceInventoryRecoveryReceipt.Preservation
	if preservation.ExecutorID != models.ExecutorIDWorktree ||
		preservation.ExecutorStatus != models.ExecutorRunningStatusStopped {
		t.Fatalf("receipt omitted authoritative prior runtime evidence: %+v", preservation)
	}
	rows := repo.taskEnvironmentRepos[environmentID]
	if len(rows) != 1 || rows[0].RepositoryID != "repo-front" || rows[0].BranchSlug != "staging-py3" ||
		rows[0].WorktreeID != "worktree-recovery" || rows[0].WorktreePath != worktreePath {
		t.Fatalf("zero inventory was not repaired from the prior environment runtime: %+v", rows)
	}
}

func TestWorkspaceInventoryRuntimeEvidenceTreatsMissingRowAsProvenAbsence(t *testing.T) {
	repo := newMockRepository()
	repo.getExecutorRunningFunc = func(context.Context, string) (*models.ExecutorRunning, error) {
		return nil, models.ErrExecutorRunningNotFound
	}
	exec := newTestExecutor(t, &mockAgentManager{}, repo)

	running, err := exec.workspaceInventoryRuntimeEvidence(context.Background(), "task-1", "session-new")
	if err != nil {
		t.Fatalf("workspaceInventoryRuntimeEvidence: %v", err)
	}
	if running != nil {
		t.Fatalf("runtime evidence = %+v, want nil for an absent row", running)
	}
}

func TestWorkspaceInventoryRepairSessionRejectsPriorRuntimeFromDifferentEnvironment(t *testing.T) {
	repo := newMockRepository()
	const taskID = "task-1"
	repo.sessions["session-prior"] = &models.TaskSession{
		ID: "session-prior", TaskID: taskID, TaskEnvironmentID: "env-other",
	}
	repo.executorsRunning["session-prior"] = &models.ExecutorRunning{
		SessionID: "session-prior", TaskID: taskID, Status: models.ExecutorRunningStatusStopped,
		WorktreeID: "worktree-other", WorktreePath: "/task/other", WorktreeBranch: "feature/other",
	}
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	req := &LaunchAgentRequest{Repositories: []RepoSpec{{TaskRepositoryID: "tr-1", RepositoryID: "repo-1"}}}
	env := &models.TaskEnvironment{ID: "env-target", TaskID: taskID}
	session := &models.TaskSession{ID: "session-new", TaskID: taskID, TaskEnvironmentID: env.ID}
	repositories := []*repoInfo{{TaskRepositoryID: "tr-1", RepositoryID: "repo-1", Position: 0}}

	_, _, err := exec.workspaceInventoryRepairSession(context.Background(), req, env, session, repositories)
	if !errors.Is(err, models.ErrWorkspaceInventoryRecoveryConflict) {
		t.Fatalf("cross-environment runtime error = %v, want recovery conflict", err)
	}
}

func TestWorkspaceInventoryRepairSessionRejectsMultiplePriorCheckoutIdentities(t *testing.T) {
	repo := newMockRepository()
	const taskID = "task-1"
	for _, sessionID := range []string{"session-a", "session-b"} {
		repo.sessions[sessionID] = &models.TaskSession{
			ID: sessionID, TaskID: taskID, TaskEnvironmentID: "env-target",
		}
		repo.executorsRunning[sessionID] = &models.ExecutorRunning{
			SessionID: sessionID, TaskID: taskID, Status: models.ExecutorRunningStatusStopped,
			WorktreeID: "worktree-" + sessionID, WorktreePath: "/task/" + sessionID,
			WorktreeBranch: "feature/" + sessionID,
		}
	}
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	req := &LaunchAgentRequest{Repositories: []RepoSpec{{TaskRepositoryID: "tr-1", RepositoryID: "repo-1"}}}
	env := &models.TaskEnvironment{ID: "env-target", TaskID: taskID}
	session := &models.TaskSession{ID: "session-new", TaskID: taskID, TaskEnvironmentID: env.ID}
	repositories := []*repoInfo{{TaskRepositoryID: "tr-1", RepositoryID: "repo-1", Position: 0}}

	_, _, err := exec.workspaceInventoryRepairSession(context.Background(), req, env, session, repositories)
	if !errors.Is(err, models.ErrWorkspaceInventoryRecoveryConflict) {
		t.Fatalf("multiple prior checkout identities error = %v, want recovery conflict", err)
	}
}
