package orchestrator

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// seedOfficeSession creates a task tagged as office (project_id set) with a
// session that carries the agent_profile_id pair. Used by handleOfficeTurnComplete
// tests below. When agentExecutionID is non-empty, an executors_running row is
// also seeded — agent_execution_id no longer lives on task_sessions; runtime
// reads it from the executors_running JOIN.
func seedOfficeSession(t *testing.T, repo officeSeedRepo, taskID, sessionID, agentExecutionID string) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-office", Name: "Office", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-office", WorkspaceID: "ws-office", Name: "Office WF", CreatedAt: now, UpdatedAt: now}); err != nil {
		_ = err
	}
	// ADR 0005 Wave F — assignee is now a runner participant keyed by
	// (workflow_step_id, task_id). Seed a step so the runner write
	// during CreateTask lands.
	stepID := "wfs-" + taskID
	if err := seedWorkflowStep(t, repo, stepID); err != nil {
		t.Fatalf("seed workflow step: %v", err)
	}
	task := &models.Task{
		ID:                     taskID,
		WorkspaceID:            "ws-office",
		WorkflowID:             "wf-office",
		WorkflowStepID:         stepID,
		Title:                  "Office task",
		State:                  v1.TaskStateInProgress,
		AssigneeAgentProfileID: "agent-1", // marks the task as office-style (per-(task, agent) session)
		CreatedAt:              now,
		UpdatedAt:              now,
	}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatalf("seed task: %v", err)
	}
	session := &models.TaskSession{
		ID:               sessionID,
		TaskID:           taskID,
		AgentProfileID:   "agent-1",
		AgentExecutionID: agentExecutionID,
		State:            models.TaskSessionStateRunning,
		StartedAt:        now,
		UpdatedAt:        now,
	}
	if err := repo.CreateTaskSession(ctx, session); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	if agentExecutionID != "" {
		if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
			ID:               sessionID,
			SessionID:        sessionID,
			TaskID:           taskID,
			AgentExecutionID: agentExecutionID,
			Status:           "ready",
		}); err != nil {
			t.Fatalf("seed executors_running: %v", err)
		}
	}
}

// officeSeedRepo is the minimal subset of repo methods seedOfficeSession needs.
// Implemented by *sqliterepo.Repository (via setupTestRepo) without requiring
// the whole sessionExecutorStore.
type officeSeedRepo interface {
	CreateWorkspace(ctx context.Context, ws *models.Workspace) error
	CreateWorkflow(ctx context.Context, wf *models.Workflow) error
	CreateTask(ctx context.Context, t *models.Task) error
	CreateTaskSession(ctx context.Context, s *models.TaskSession) error
	UpsertExecutorRunning(ctx context.Context, er *models.ExecutorRunning) error
	DB() *sql.DB
}

// seedWorkflowStep inserts a stub workflow_steps row so a CreateTask
// call can attach a runner participant keyed by (step_id, task_id).
// ADR 0005 Wave F: assignee lives in workflow_step_participants and
// requires a real step row to satisfy the natural key.
func seedWorkflowStep(_ *testing.T, repo officeSeedRepo, stepID string) error {
	_, err := repo.DB().Exec(`INSERT INTO workflow_steps (id, workflow_id, name, position, agent_profile_id, created_at, updated_at)
		VALUES (?, '', '', 0, '', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`, stepID)
	return err
}

func TestHandleOfficeTurnComplete_SetsIdleAndStopsAgent(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedOfficeSession(t, repo, "t-office", "s-office", "exec-42")
	mgr := &mockAgentManager{}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), mgr)

	session, err := repo.GetTaskSession(ctx, "s-office")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	handled := svc.handleOfficeTurnComplete(ctx, "t-office", "s-office", session)
	if !handled {
		t.Fatal("expected office turn complete to claim the event")
	}

	got, err := repo.GetTaskSession(ctx, "s-office")
	if err != nil {
		t.Fatalf("re-read session: %v", err)
	}
	if got.State != models.TaskSessionStateIdle {
		t.Errorf("state: got %q want IDLE", got.State)
	}

	if len(mgr.stopAgentArgs) != 1 {
		t.Fatalf("expected 1 StopAgent call, got %d", len(mgr.stopAgentArgs))
	}
	if mgr.stopAgentArgs[0].ExecutionID != "exec-42" {
		t.Errorf("StopAgent execution_id: got %q want exec-42", mgr.stopAgentArgs[0].ExecutionID)
	}
}

// TestHandleOfficeTurnComplete_KanbanFallthrough verifies kanban / quick-chat
// sessions (no agent_profile_id) are NOT claimed and StopAgent is not called.
func TestHandleOfficeTurnComplete_KanbanFallthrough(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-kanban", "sess-kanban", "step1")
	mgr := &mockAgentManager{}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), mgr)

	session, _ := repo.GetTaskSession(ctx, "sess-kanban")
	handled := svc.handleOfficeTurnComplete(ctx, "task-kanban", "sess-kanban", session)
	if handled {
		t.Error("kanban session should not be claimed by office turn-complete")
	}
	if len(mgr.stopAgentArgs) != 0 {
		t.Errorf("expected no StopAgent calls, got %d", len(mgr.stopAgentArgs))
	}
}

// TestHandleOfficeTurnComplete_StopAgentFailureStillIdle confirms a StopAgent
// error doesn't propagate or block the IDLE state flip.
func TestHandleOfficeTurnComplete_StopAgentFailureStillIdle(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedOfficeSession(t, repo, "t-stop-fail", "s-stop-fail", "exec-99")
	mgr := &mockAgentManager{stopAgentErr: errors.New("teardown failed")}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), mgr)

	session, _ := repo.GetTaskSession(ctx, "s-stop-fail")
	if !svc.handleOfficeTurnComplete(ctx, "t-stop-fail", "s-stop-fail", session) {
		t.Fatal("expected handler to claim the event")
	}

	got, _ := repo.GetTaskSession(ctx, "s-stop-fail")
	if got.State != models.TaskSessionStateIdle {
		t.Errorf("state still expected IDLE despite StopAgent failure, got %q", got.State)
	}
	if len(mgr.stopAgentArgs) != 1 {
		t.Errorf("StopAgent should have been called once, got %d", len(mgr.stopAgentArgs))
	}
}

// TestHandleOfficeTurnComplete_NoExecutionStillIdle ensures that an office
// session with no AgentExecutionID (e.g. crash-recovered row) still goes IDLE
// without panicking — StopAgent is simply skipped.
func TestHandleOfficeTurnComplete_NoExecutionStillIdle(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedOfficeSession(t, repo, "t-no-exec", "s-no-exec", "" /* no exec id */)
	mgr := &mockAgentManager{}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), mgr)

	session, _ := repo.GetTaskSession(ctx, "s-no-exec")
	svc.handleOfficeTurnComplete(ctx, "t-no-exec", "s-no-exec", session)

	got, _ := repo.GetTaskSession(ctx, "s-no-exec")
	if got.State != models.TaskSessionStateIdle {
		t.Errorf("state: got %q want IDLE", got.State)
	}
	if len(mgr.stopAgentArgs) != 0 {
		t.Errorf("expected no StopAgent (no execution_id), got %d", len(mgr.stopAgentArgs))
	}
}
