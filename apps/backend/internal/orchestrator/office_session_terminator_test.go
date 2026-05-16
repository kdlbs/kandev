package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// seedTaskAndOfficeSession is a tiny helper that creates a minimal task +
// agent-bound session so the terminator has something to flip.
func seedTaskAndOfficeSession(t *testing.T, repo officeSeedRepo, taskID, sessionID, agentID string, state models.TaskSessionState) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-term", Name: "term", CreatedAt: now, UpdatedAt: now})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-term", WorkspaceID: "ws-term", Name: "wf", CreatedAt: now, UpdatedAt: now})
	stepID := "wfs-" + taskID
	_ = seedWorkflowStep(t, repo, stepID)
	if err := repo.CreateTask(ctx, &models.Task{
		ID: taskID, WorkspaceID: "ws-term", WorkflowID: "wf-term", WorkflowStepID: stepID,
		Title: "T", State: v1.TaskStateInProgress,
		ProjectID: "proj-1", AssigneeAgentProfileID: agentID,
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: sessionID, TaskID: taskID, AgentProfileID: agentID,
		State: state, StartedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}
}

func TestTerminateOfficeSession_FlipsToCompleted(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndOfficeSession(t, repo, "t1", "s1", "agent-a", models.TaskSessionStateRunning)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	term := svc.OfficeSessionTerminator()
	if err := term.TerminateOfficeSession(ctx, "t1", "agent-a", "test"); err != nil {
		t.Fatalf("terminate: %v", err)
	}

	got, err := repo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.State != models.TaskSessionStateCompleted {
		t.Errorf("state: got %q want COMPLETED", got.State)
	}
}

func TestTerminateOfficeSession_IdempotentOnTerminal(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndOfficeSession(t, repo, "t1", "s1", "agent-a", models.TaskSessionStateCompleted)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	if err := svc.OfficeSessionTerminator().TerminateOfficeSession(ctx, "t1", "agent-a", "test"); err != nil {
		t.Fatalf("terminate (terminal): %v", err)
	}
	// Already-terminal state: no-op, no error.
	got, _ := repo.GetTaskSession(ctx, "s1")
	if got.State != models.TaskSessionStateCompleted {
		t.Errorf("state mutated: got %q", got.State)
	}
}

func TestTerminateOfficeSession_MissingPair_NoOp(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	if err := svc.OfficeSessionTerminator().TerminateOfficeSession(ctx, "t-nonexistent", "agent-x", "test"); err != nil {
		t.Errorf("expected no error for missing pair, got %v", err)
	}
}

func TestTerminateAllForAgent_CascadesAcrossTasks(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndOfficeSession(t, repo, "t1", "s1", "agent-a", models.TaskSessionStateRunning)
	// Second task with an IDLE session for the same agent.
	now := time.Now().UTC()
	_ = seedWorkflowStep(t, repo, "wfs-t2")
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "t2", WorkspaceID: "ws-term", WorkflowID: "wf-term", WorkflowStepID: "wfs-t2",
		Title: "T2", State: v1.TaskStateInProgress,
		ProjectID: "proj-1", AssigneeAgentProfileID: "agent-a",
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("seed t2: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "s2", TaskID: "t2", AgentProfileID: "agent-a",
		State: models.TaskSessionStateIdle, StartedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("seed s2: %v", err)
	}
	// Different agent on the same task — should NOT be touched.
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "s-other", TaskID: "t1", AgentProfileID: "agent-b",
		State: models.TaskSessionStateRunning, StartedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("seed s-other: %v", err)
	}

	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	if err := svc.OfficeSessionTerminator().TerminateAllForAgent(ctx, "agent-a", "deletion"); err != nil {
		t.Fatalf("cascade: %v", err)
	}

	for _, sid := range []string{"s1", "s2"} {
		got, _ := repo.GetTaskSession(ctx, sid)
		if got.State != models.TaskSessionStateCompleted {
			t.Errorf("session %q: got %q want COMPLETED", sid, got.State)
		}
	}
	other, _ := repo.GetTaskSession(ctx, "s-other")
	if other.State != models.TaskSessionStateRunning {
		t.Errorf("other agent's session must not be touched, got %q", other.State)
	}
}
