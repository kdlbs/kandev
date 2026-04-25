package orchestrator

import (
	"context"
	"testing"
	"time"

	v1 "github.com/kandev/kandev/pkg/api/v1"

	"github.com/kandev/kandev/internal/task/models"
)

func TestInheritFromParentSession_InheritsFromPrimary(t *testing.T) {
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	ctx := context.Background()
	now := time.Now().UTC()

	ws := &models.Workspace{ID: "ws1", Name: "Test", CreatedAt: now, UpdatedAt: now}
	_ = repo.CreateWorkspace(ctx, ws)
	wf := &models.Workflow{ID: "wf1", WorkspaceID: "ws1", Name: "WF", CreatedAt: now, UpdatedAt: now}
	_ = repo.CreateWorkflow(ctx, wf)
	parent := &models.Task{ID: "parent1", WorkflowID: "wf1", Title: "Parent", State: v1.TaskStateInProgress, CreatedAt: now, UpdatedAt: now}
	_ = repo.CreateTask(ctx, parent)

	parentSession := &models.TaskSession{
		ID: "ps1", TaskID: "parent1", State: models.TaskSessionStateRunning,
		IsPrimary: true, AgentProfileID: "agent-prof-1", ExecutorProfileID: "exec-prof-1",
		StartedAt: now, UpdatedAt: now,
	}
	_ = repo.CreateTaskSession(ctx, parentSession)

	agentID, execProfID, execID := svc.inheritFromParentSession(ctx, "parent1", "", "", "")

	if agentID != "agent-prof-1" {
		t.Errorf("expected agentProfileID=%q, got %q", "agent-prof-1", agentID)
	}
	if execProfID != "exec-prof-1" {
		t.Errorf("expected executorProfileID=%q, got %q", "exec-prof-1", execProfID)
	}
	if execID != "" {
		t.Errorf("expected executorID=%q (profile takes precedence), got %q", "", execID)
	}
}

func TestInheritFromParentSession_DoesNotOverrideExplicit(t *testing.T) {
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	ctx := context.Background()
	now := time.Now().UTC()

	ws := &models.Workspace{ID: "ws1", Name: "Test", CreatedAt: now, UpdatedAt: now}
	_ = repo.CreateWorkspace(ctx, ws)
	wf := &models.Workflow{ID: "wf1", WorkspaceID: "ws1", Name: "WF", CreatedAt: now, UpdatedAt: now}
	_ = repo.CreateWorkflow(ctx, wf)
	parent := &models.Task{ID: "parent2", WorkflowID: "wf1", Title: "Parent", State: v1.TaskStateInProgress, CreatedAt: now, UpdatedAt: now}
	_ = repo.CreateTask(ctx, parent)

	parentSession := &models.TaskSession{
		ID: "ps2", TaskID: "parent2", State: models.TaskSessionStateRunning,
		IsPrimary: true, AgentProfileID: "parent-agent", ExecutorProfileID: "parent-exec",
		StartedAt: now, UpdatedAt: now,
	}
	_ = repo.CreateTaskSession(ctx, parentSession)

	agentID, execProfID, _ := svc.inheritFromParentSession(ctx, "parent2", "my-agent", "", "")

	if agentID != "my-agent" {
		t.Errorf("expected caller-provided agentProfileID=%q, got %q", "my-agent", agentID)
	}
	if execProfID != "parent-exec" {
		t.Errorf("expected inherited executorProfileID=%q, got %q", "parent-exec", execProfID)
	}
}

func TestInheritFromParentSession_NoPrimarySession(t *testing.T) {
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	ctx := context.Background()
	now := time.Now().UTC()

	ws := &models.Workspace{ID: "ws1", Name: "Test", CreatedAt: now, UpdatedAt: now}
	_ = repo.CreateWorkspace(ctx, ws)
	wf := &models.Workflow{ID: "wf1", WorkspaceID: "ws1", Name: "WF", CreatedAt: now, UpdatedAt: now}
	_ = repo.CreateWorkflow(ctx, wf)
	parent := &models.Task{ID: "parent3", WorkflowID: "wf1", Title: "Parent", State: v1.TaskStateInProgress, CreatedAt: now, UpdatedAt: now}
	_ = repo.CreateTask(ctx, parent)

	nonPrimary := &models.TaskSession{
		ID: "ps3", TaskID: "parent3", State: models.TaskSessionStateRunning,
		IsPrimary: false, AgentProfileID: "should-not-use",
		StartedAt: now, UpdatedAt: now,
	}
	_ = repo.CreateTaskSession(ctx, nonPrimary)

	agentID, execProfID, execID := svc.inheritFromParentSession(ctx, "parent3", "", "", "")

	if agentID != "" || execProfID != "" || execID != "" {
		t.Errorf("expected all empty when no primary session, got agent=%q execProf=%q exec=%q",
			agentID, execProfID, execID)
	}
}

func TestInheritFromParentSession_NoParentSessions(t *testing.T) {
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	ctx := context.Background()

	agentID, execProfID, execID := svc.inheritFromParentSession(ctx, "nonexistent", "", "", "")

	if agentID != "" || execProfID != "" || execID != "" {
		t.Errorf("expected all empty on error, got agent=%q execProf=%q exec=%q",
			agentID, execProfID, execID)
	}
}

func TestInheritFromParentSession_InheritsExecutorIDWhenNoProfile(t *testing.T) {
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	ctx := context.Background()
	now := time.Now().UTC()

	ws := &models.Workspace{ID: "ws1", Name: "Test", CreatedAt: now, UpdatedAt: now}
	_ = repo.CreateWorkspace(ctx, ws)
	wf := &models.Workflow{ID: "wf1", WorkspaceID: "ws1", Name: "WF", CreatedAt: now, UpdatedAt: now}
	_ = repo.CreateWorkflow(ctx, wf)
	parent := &models.Task{ID: "parent4", WorkflowID: "wf1", Title: "Parent", State: v1.TaskStateInProgress, CreatedAt: now, UpdatedAt: now}
	_ = repo.CreateTask(ctx, parent)

	parentSession := &models.TaskSession{
		ID: "ps4", TaskID: "parent4", State: models.TaskSessionStateRunning,
		IsPrimary: true, AgentProfileID: "agent-1", ExecutorID: "exec-docker",
		StartedAt: now, UpdatedAt: now,
	}
	_ = repo.CreateTaskSession(ctx, parentSession)

	_, _, execID := svc.inheritFromParentSession(ctx, "parent4", "", "", "")

	if execID != "exec-docker" {
		t.Errorf("expected inherited executorID=%q, got %q", "exec-docker", execID)
	}
}

func TestFindPrimarySession(t *testing.T) {
	sessions := []*models.TaskSession{
		{ID: "s1", IsPrimary: false},
		{ID: "s2", IsPrimary: true},
		{ID: "s3", IsPrimary: false},
	}
	ps := findPrimarySession(sessions)
	if ps == nil || ps.ID != "s2" {
		t.Errorf("expected primary session s2, got %v", ps)
	}
}

func TestFindPrimarySession_NoneFound(t *testing.T) {
	sessions := []*models.TaskSession{
		{ID: "s1", IsPrimary: false},
	}
	ps := findPrimarySession(sessions)
	if ps != nil {
		t.Errorf("expected nil, got %v", ps)
	}
}

func TestFindPrimarySession_Empty(t *testing.T) {
	ps := findPrimarySession(nil)
	if ps != nil {
		t.Errorf("expected nil for empty slice, got %v", ps)
	}
}
