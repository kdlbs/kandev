package executor

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/worktree"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func seedSelectedWorktreeRecoveryEnvironment(
	repo *mockRepository,
	taskID, sessionID string,
	sessionState models.TaskSessionState,
) {
	repo.repositories["repo-recovery"] = &models.Repository{
		ID:                   "repo-recovery",
		Name:                 "recovery",
		LocalPath:            "/repos/recovery",
		WorktreeBranchPrefix: "feature/",
	}
	repo.taskRepositories["task-repo-recovery"] = &models.TaskRepository{
		ID: "task-repo-recovery", TaskID: taskID, RepositoryID: "repo-recovery", Position: 0, BaseBranch: "main",
	}
	repo.executors[models.ExecutorIDWorktree] = &models.Executor{
		ID: models.ExecutorIDWorktree, Type: models.ExecutorTypeWorktree, Status: models.ExecutorStatusActive,
	}
	environmentRepos := []*models.TaskEnvironmentRepo{{
		ID:                "environment-repo-recovery",
		TaskEnvironmentID: "environment-recovery",
		RepositoryID:      "repo-recovery",
		BranchSlug:        "main",
		WorktreeID:        "worktree-recovery",
		WorktreePath:      "/tasks/recovery/recovery",
		WorktreeBranch:    "feature/recovery",
		Status:            "active",
		Position:          0,
	}}
	repo.taskEnvironments["environment-recovery"] = &models.TaskEnvironment{
		ID:                  "environment-recovery",
		TaskID:              taskID,
		OwnershipGeneration: 1,
		ExecutorType:        string(models.ExecutorTypeWorktree),
		Status:              models.TaskEnvironmentStatusReady,
		WorkspacePath:       "/tasks/recovery/recovery",
		TaskDirName:         "recovery_abc",
		Repos:               environmentRepos,
	}
	repo.taskEnvironmentRepos["environment-recovery"] = environmentRepos
	repo.sessions[sessionID] = &models.TaskSession{
		ID:                sessionID,
		TaskID:            taskID,
		TaskEnvironmentID: "environment-recovery",
		AgentProfileID:    "profile-recovery",
		ExecutorID:        models.ExecutorIDWorktree,
		RepositoryID:      "repo-recovery",
		BaseBranch:        "main",
		State:             sessionState,
		StartedAt:         time.Now().UTC(),
		UpdatedAt:         time.Now().UTC(),
	}
}

func TestWorktreeRecoveryLaunchIntegration(t *testing.T) {
	const taskID = "task-recovery-launch"
	const sessionID = "session-recovery-launch"
	repo := newMockRepository()
	seedSelectedWorktreeRecoveryEnvironment(repo, taskID, sessionID, models.TaskSessionStateCreated)

	var admissionRequest worktree.RecoveryAdmissionRequest
	admissionCalls := 0
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	exec.SetSelectedWorktreeRecoveryAdmission(func(_ context.Context, req worktree.RecoveryAdmissionRequest) (*worktree.RecoveryAdmission, error) {
		admissionCalls++
		admissionRequest = req
		return nil, nil
	})

	_, err := exec.LaunchPreparedSession(context.Background(), &v1.Task{
		ID: taskID, WorkspaceID: "workspace-recovery", Title: "Recovery launch",
	}, sessionID, LaunchOptions{
		AgentProfileID: "profile-recovery",
		ExecutorID:     models.ExecutorIDWorktree,
		StartAgent:     false,
	})
	if err != nil {
		t.Fatalf("LaunchPreparedSession: %v", err)
	}
	if admissionCalls != 1 {
		t.Fatalf("selected recovery admission calls = %d, want 1", admissionCalls)
	}
	assertSelectedWorktreeRecoveryRequest(t, admissionRequest, taskID, sessionID)
}

func TestWorktreeRecoveryResumeIntegration(t *testing.T) {
	const taskID = "task-recovery-resume"
	const sessionID = "session-recovery-resume"
	repo := newMockRepository()
	seedSelectedWorktreeRecoveryEnvironment(repo, taskID, sessionID, models.TaskSessionStateCancelled)
	repo.tasks[taskID] = &models.Task{ID: taskID, WorkspaceID: "workspace-recovery", Title: "Recovery resume"}

	var admissionRequest worktree.RecoveryAdmissionRequest
	admissionCalls := 0
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	exec.SetSelectedWorktreeRecoveryAdmission(func(_ context.Context, req worktree.RecoveryAdmissionRequest) (*worktree.RecoveryAdmission, error) {
		admissionCalls++
		admissionRequest = req
		return nil, nil
	})

	_, err := exec.ResumeSession(context.Background(), repo.sessions[sessionID], false)
	if err != nil {
		t.Fatalf("ResumeSession: %v", err)
	}
	if admissionCalls != 1 {
		t.Fatalf("selected recovery admission calls = %d, want 1", admissionCalls)
	}
	assertSelectedWorktreeRecoveryRequest(t, admissionRequest, taskID, sessionID)
}

func assertSelectedWorktreeRecoveryRequest(
	t *testing.T,
	req worktree.RecoveryAdmissionRequest,
	taskID, sessionID string,
) {
	t.Helper()
	if req.TaskID != taskID || req.SessionID != sessionID {
		t.Fatalf("admission identity = task %q/session %q, want %q/%q", req.TaskID, req.SessionID, taskID, sessionID)
	}
	if req.TaskEnvironmentID != "environment-recovery" || req.OwnerTaskID != taskID {
		t.Fatalf("admission environment = %q, owner %q, want environment-recovery/%q", req.TaskEnvironmentID, req.OwnerTaskID, taskID)
	}
	if req.OwnershipGeneration != 1 || req.ExecutorType != string(models.ExecutorTypeWorktree) {
		t.Fatalf("admission authority = generation %d/executor %q, want 1/worktree", req.OwnershipGeneration, req.ExecutorType)
	}
	if len(req.Slots) != 1 {
		t.Fatalf("admission slots = %d, want 1", len(req.Slots))
	}
	slot := req.Slots[0]
	if slot.WorktreeID != "worktree-recovery" || slot.RepositoryID != "repo-recovery" || slot.BranchSlug != "main" {
		t.Fatalf("admission slot = %+v, want selected canonical worktree", slot)
	}
}
