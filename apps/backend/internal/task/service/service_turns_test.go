package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func TestGetWorkspaceInfoForSession_BasicFields(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	setupTestTask(t, repo)
	now := time.Now().UTC()

	session := &models.TaskSession{
		ID:                "session-1",
		TaskID:            "task-123",
		TaskEnvironmentID: "env-123",
		AgentProfileID:    "profile-1",
		State:             models.TaskSessionStateCompleted,
		AgentProfileSnapshot: map[string]interface{}{
			"agent_name": "auggie",
		},
		Metadata: map[string]interface{}{
			"acp_session_id": "acp-123",
		},
		StartedAt: now,
		UpdatedAt: now,
	}
	if err := repo.CreateTaskSession(ctx, session); err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	// Add a worktree to the session
	if err := repo.CreateTaskSessionWorktree(ctx, &models.TaskSessionWorktree{
		ID:             "wt1",
		SessionID:      "session-1",
		WorktreeID:     "wid1",
		RepositoryID:   "repo1",
		WorktreePath:   "/tmp/worktrees/session-1",
		WorktreeBranch: "feature/test",
		CreatedAt:      now,
	}); err != nil {
		t.Fatalf("failed to create worktree: %v", err)
	}

	info, err := svc.GetWorkspaceInfoForSession(ctx, "task-123", "session-1")
	if err != nil {
		t.Fatalf("GetWorkspaceInfoForSession returned error: %v", err)
	}

	if info.TaskID != "task-123" {
		t.Errorf("expected TaskID 'task-123', got %q", info.TaskID)
	}
	if info.SessionID != "session-1" {
		t.Errorf("expected SessionID 'session-1', got %q", info.SessionID)
	}
	if info.TaskEnvironmentID != "env-123" {
		t.Errorf("expected TaskEnvironmentID 'env-123', got %q", info.TaskEnvironmentID)
	}
	if info.WorkspacePath != "/tmp/worktrees/session-1" {
		t.Errorf("expected WorkspacePath '/tmp/worktrees/session-1', got %q", info.WorkspacePath)
	}
	if info.AgentProfileID != "profile-1" {
		t.Errorf("expected AgentProfileID 'profile-1', got %q", info.AgentProfileID)
	}
	if info.AgentID != "auggie" {
		t.Errorf("expected AgentID 'auggie', got %q", info.AgentID)
	}
	if info.ACPSessionID != "acp-123" {
		t.Errorf("expected ACPSessionID 'acp-123', got %q", info.ACPSessionID)
	}
}

func TestGetWorkspaceInfoForSession_InfersTaskID(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	setupTestTask(t, repo)
	now := time.Now().UTC()

	session := &models.TaskSession{
		ID:        "session-1",
		TaskID:    "task-123",
		State:     models.TaskSessionStateCompleted,
		StartedAt: now,
		UpdatedAt: now,
	}
	if err := repo.CreateTaskSession(ctx, session); err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	// Pass empty taskID - should be inferred from the session
	info, err := svc.GetWorkspaceInfoForSession(ctx, "", "session-1")
	if err != nil {
		t.Fatalf("GetWorkspaceInfoForSession returned error: %v", err)
	}
	if info.TaskID != "task-123" {
		t.Errorf("expected TaskID 'task-123' inferred from session, got %q", info.TaskID)
	}
}

func TestGetWorkspaceInfoForSession_ExecutorInfo(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	setupTestTask(t, repo)
	now := time.Now().UTC()

	// Create executor
	exec := &models.Executor{
		ID:        "exec-1",
		Name:      "My Sprites Executor",
		Type:      models.ExecutorTypeSprites,
		Status:    "active",
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := repo.CreateExecutor(ctx, exec); err != nil {
		t.Fatalf("failed to create executor: %v", err)
	}

	// Create session with executor reference
	session := &models.TaskSession{
		ID:         "session-1",
		TaskID:     "task-123",
		ExecutorID: "exec-1",
		State:      models.TaskSessionStateCompleted,
		AgentProfileSnapshot: map[string]interface{}{
			"agent_name": "auggie",
		},
		StartedAt: now,
		UpdatedAt: now,
	}
	if err := repo.CreateTaskSession(ctx, session); err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	// Create executor running record
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID:               "er-1",
		SessionID:        "session-1",
		TaskID:           "task-123",
		ExecutorID:       "exec-1",
		Runtime:          "sprites",
		AgentExecutionID: "agent-exec-abc123",
		CreatedAt:        now,
		UpdatedAt:        now,
	}); err != nil {
		t.Fatalf("failed to upsert executor running: %v", err)
	}

	info, err := svc.GetWorkspaceInfoForSession(ctx, "task-123", "session-1")
	if err != nil {
		t.Fatalf("GetWorkspaceInfoForSession returned error: %v", err)
	}

	if info.ExecutorType != "sprites" {
		t.Errorf("expected ExecutorType 'sprites', got %q", info.ExecutorType)
	}
	if info.RuntimeName != "sprites" {
		t.Errorf("expected RuntimeName 'sprites', got %q", info.RuntimeName)
	}
	if info.AgentExecutionID != "agent-exec-abc123" {
		t.Errorf("expected AgentExecutionID 'agent-exec-abc123', got %q", info.AgentExecutionID)
	}
}

func TestGetWorkspaceInfoForSession_NoExecutorRunning(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	setupTestTask(t, repo)
	now := time.Now().UTC()

	session := &models.TaskSession{
		ID:        "session-1",
		TaskID:    "task-123",
		State:     models.TaskSessionStateCompleted,
		StartedAt: now,
		UpdatedAt: now,
	}
	if err := repo.CreateTaskSession(ctx, session); err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	// No executor running record - should still succeed with empty executor fields
	info, err := svc.GetWorkspaceInfoForSession(ctx, "task-123", "session-1")
	if err != nil {
		t.Fatalf("GetWorkspaceInfoForSession returned error: %v", err)
	}
	if info.RuntimeName != "" {
		t.Errorf("expected empty RuntimeName, got %q", info.RuntimeName)
	}
	if info.AgentExecutionID != "" {
		t.Errorf("expected empty AgentExecutionID, got %q", info.AgentExecutionID)
	}
	if info.ExecutorType != "" {
		t.Errorf("expected empty ExecutorType, got %q", info.ExecutorType)
	}
}

func TestGetWorkspaceInfoForSession_SessionNotFound(t *testing.T) {
	svc, _, _ := createTestService(t)
	ctx := context.Background()

	_, err := svc.GetWorkspaceInfoForSession(ctx, "task-123", "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent session")
	}
}

func TestGetWorkspaceInfoForEnvironment(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	setupTestTask(t, repo)
	now := time.Now().UTC()
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID:        "env-123",
		TaskID:    "task-123",
		Status:    models.TaskEnvironmentStatusReady,
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("failed to create task environment: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID:                "session-1",
		TaskID:            "task-123",
		TaskEnvironmentID: "env-123",
		State:             models.TaskSessionStateCompleted,
		AgentProfileSnapshot: map[string]interface{}{
			"agent_name": "auggie",
		},
		StartedAt: now,
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("failed to create session: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID:                "session-2",
		TaskID:            "task-123",
		TaskEnvironmentID: "env-123",
		State:             models.TaskSessionStateCompleted,
		AgentProfileSnapshot: map[string]interface{}{
			"agent_name": "auggie",
		},
		StartedAt: now.Add(time.Minute),
		UpdatedAt: now.Add(time.Minute),
	}); err != nil {
		t.Fatalf("failed to create newer session: %v", err)
	}

	info, err := svc.GetWorkspaceInfoForEnvironment(ctx, "env-123")
	if err != nil {
		t.Fatalf("GetWorkspaceInfoForEnvironment returned error: %v", err)
	}
	if info.SessionID != "session-2" {
		t.Errorf("SessionID = %q, want session-2", info.SessionID)
	}
	if info.TaskEnvironmentID != "env-123" {
		t.Errorf("TaskEnvironmentID = %q, want env-123", info.TaskEnvironmentID)
	}
}

// Multi-repo: the workspace path agentctl boots with must be the task root
// (parent of every per-repo subdir) so its scanRepositorySubdirs detects all
// repos and starts a per-repo tracker for each. Returning a single repo's
// path would collapse fan-out into the legacy single-tracker mode and
// suppress the per-repo events the Changes panel needs to render headers.
func TestGetWorkspaceInfoForSession_MultiRepoReturnsTaskRoot(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	setupTestTask(t, repo)
	now := time.Now().UTC()

	session := &models.TaskSession{
		ID:        "session-multi",
		TaskID:    "task-123",
		State:     models.TaskSessionStateCompleted,
		StartedAt: now,
		UpdatedAt: now,
	}
	if err := repo.CreateTaskSession(ctx, session); err != nil {
		t.Fatalf("create session: %v", err)
	}
	for i, path := range []string{
		"/tmp/tasks/do-nothing_mvo/kandev",
		"/tmp/tasks/do-nothing_mvo/thm",
	} {
		if err := repo.CreateTaskSessionWorktree(ctx, &models.TaskSessionWorktree{
			ID:           fmt.Sprintf("wt%d", i),
			SessionID:    session.ID,
			WorktreeID:   fmt.Sprintf("wid%d", i),
			RepositoryID: fmt.Sprintf("repo%d", i),
			Position:     i,
			WorktreePath: path,
			CreatedAt:    now,
		}); err != nil {
			t.Fatalf("create worktree %d: %v", i, err)
		}
	}

	info, err := svc.GetWorkspaceInfoForSession(ctx, "task-123", session.ID)
	if err != nil {
		t.Fatalf("GetWorkspaceInfoForSession: %v", err)
	}
	if info.WorkspacePath != "/tmp/tasks/do-nothing_mvo" {
		t.Errorf("expected WorkspacePath '/tmp/tasks/do-nothing_mvo' (task root), got %q",
			info.WorkspacePath)
	}
}
