package service

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
)

// TestService_GetExecutorRunningBySessionID_DelegatesToRepository proves the
// service-layer accessor (added for the Host data API's acp_session_id
// fallback, ADR 0043) reaches the real executor repository rather than
// returning a stub, and surfaces the same not-found sentinel repository
// callers already rely on.
func TestService_GetExecutorRunningBySessionID_DelegatesToRepository(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "Workflow"})
	_ = repo.CreateTask(ctx, &models.Task{ID: "task-1", WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1", Title: "Test", Priority: "medium"})
	_ = repo.CreateTaskSession(ctx, &models.TaskSession{ID: "session-1", TaskID: "task-1", State: models.TaskSessionStateRunning})
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID:          "session-1",
		SessionID:   "session-1",
		TaskID:      "task-1",
		ExecutorID:  "executor-1",
		Runtime:     agentruntime.RuntimeStandalone,
		Status:      models.ExecutorRunningStatusRunning,
		ResumeToken: "acp-resume-token",
	}); err != nil {
		t.Fatalf("seed executor running: %v", err)
	}

	got, err := svc.GetExecutorRunningBySessionID(ctx, "session-1")
	if err != nil {
		t.Fatalf("GetExecutorRunningBySessionID() unexpected error: %v", err)
	}
	if got.ResumeToken != "acp-resume-token" {
		t.Fatalf("ResumeToken = %q, want %q", got.ResumeToken, "acp-resume-token")
	}

	if _, err := svc.GetExecutorRunningBySessionID(ctx, "no-such-session"); !errors.Is(err, models.ErrExecutorRunningNotFound) {
		t.Fatalf("GetExecutorRunningBySessionID() for missing session = %v, want %v", err, models.ErrExecutorRunningNotFound)
	}
}
