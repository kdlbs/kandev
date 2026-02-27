package service

import (
	"context"
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
		ID:             "session-1",
		TaskID:         "task-123",
		AgentProfileID: "profile-1",
		State:          models.TaskSessionStateCompleted,
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
