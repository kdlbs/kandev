package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// seedOfficeTaskAndSessions seeds an office task with the assignee's session
// plus an extra reviewer session. Both rows carry agent_profile_id; the
// assignee row is the one advanced-mode resume should pick by default.
func seedOfficeTaskAndSessions(t *testing.T, repo officeSeedRepo) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-r", Name: "r", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("ws: %v", err)
	}
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-r", WorkspaceID: "ws-r", Name: "wf", CreatedAt: now, UpdatedAt: now})
	if err := seedWorkflowStep(t, repo, "wfs-office"); err != nil {
		t.Fatalf("workflow step: %v", err)
	}
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "t-office", WorkspaceID: "ws-r", WorkflowID: "wf-r", WorkflowStepID: "wfs-office",
		Title: "Office", State: v1.TaskStateInProgress,
		ProjectID: "p-1", AssigneeAgentProfileID: "agent-assignee",
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("task: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "s-assignee", TaskID: "t-office", AgentProfileID: "agent-assignee",
		State: models.TaskSessionStateIdle, StartedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("assignee sess: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "s-reviewer", TaskID: "t-office", AgentProfileID: "agent-reviewer",
		State: models.TaskSessionStateIdle, StartedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("reviewer sess: %v", err)
	}
}

func TestFindExistingSession_OfficeTaskResolvesToAssignee(t *testing.T) {
	repo := setupTestRepo(t)
	seedOfficeTaskAndSessions(t, repo)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	resp := svc.findExistingSession(context.Background(), "t-office")
	if resp == nil {
		t.Fatal("expected resume target, got nil")
	}
	if resp.SessionID != "s-assignee" {
		t.Errorf("session_id: got %q want s-assignee", resp.SessionID)
	}
	if resp.Source != "existing_office_agent" {
		t.Errorf("source: got %q want existing_office_agent", resp.Source)
	}
}

// TestFindExistingSession_OfficeTaskWithViewerAgent confirms the agent
// context overrides the assignee fallback.
func TestFindExistingSession_OfficeTaskWithViewerAgent(t *testing.T) {
	repo := setupTestRepo(t)
	seedOfficeTaskAndSessions(t, repo)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	ctx := WithViewerAgent(context.Background(), "agent-reviewer")
	resp := svc.findExistingSession(ctx, "t-office")
	if resp == nil {
		t.Fatal("expected resume target, got nil")
	}
	if resp.SessionID != "s-reviewer" {
		t.Errorf("session_id: got %q want s-reviewer", resp.SessionID)
	}
}

// TestFindExistingSession_KanbanTask_UsesPrimary keeps the kanban-side
// is_primary lookup intact under the new gating.
func TestFindExistingSession_KanbanTask_UsesPrimary(t *testing.T) {
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-k", "sess-k", "step1")
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	resp := svc.findExistingSession(context.Background(), "task-k")
	if resp == nil {
		t.Fatal("expected resume target, got nil")
	}
	// Kanban / quick-chat sessions don't go through the office branch.
	if resp.Source == "existing_office_agent" {
		t.Errorf("kanban session must not surface as office")
	}
}

// TestFindExistingSession_OfficeTaskNoSessionYet returns nil so the caller
// (EnsureSession) falls through to the create branch — which routes through
// prepareSessionForStart → EnsureSessionForAgent for office tasks.
func TestFindExistingSession_OfficeTaskNoSessionYet(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()
	now := time.Now().UTC()
	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-z", Name: "z", CreatedAt: now, UpdatedAt: now})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-z", WorkspaceID: "ws-z", Name: "z", CreatedAt: now, UpdatedAt: now})
	_ = seedWorkflowStep(t, repo, "wfs-empty")
	_ = repo.CreateTask(ctx, &models.Task{
		ID: "t-empty", WorkspaceID: "ws-z", WorkflowID: "wf-z", WorkflowStepID: "wfs-empty",
		Title: "empty", State: v1.TaskStateInProgress,
		ProjectID: "p", AssigneeAgentProfileID: "agent-z",
		CreatedAt: now, UpdatedAt: now,
	})
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	if got := svc.findExistingSession(ctx, "t-empty"); got != nil {
		t.Errorf("expected nil for office task with no session yet, got %+v", got)
	}
}
