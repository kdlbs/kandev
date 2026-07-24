package service

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

// TestService_ArchivedSessionReconciliationRecoversStrandedSessions is the
// regression test for the residual gap Greptile flagged on finalizeCancelledSessions:
// its bounded in-line retry (3 attempts x 10s) can still be exhausted by
// sustained SQLite writer contention, leaving an archived task's session
// stuck active in the DB with no session.state_changed event ever
// delivered. It reuses flakyCancelSessionRepository (defined in
// service_test.go) to fail all 3 in-line attempts during ArchiveTask,
// simulating exhausted retries, then calls runArchivedSessionReconciliation
// directly (the sweep's single run, not the ticker loop) once the
// underlying repository would succeed again, and asserts the stranded
// session is recovered: CANCELLED in the DB and carrying a published
// session.state_changed event.
func TestService_ArchivedSessionReconciliationRecoversStrandedSessions(t *testing.T) {
	flaky := &flakyCancelSessionRepository{failuresLeft: 3}
	svc, eventBus, repo := createTestServiceWithSessionsRepo(t, func(repo *sqliterepo.Repository) repository.SessionRepository {
		flaky.Repository = repo
		return flaky
	})

	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "Workflow"}); err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "task-stranded", WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1",
		Title: "Test", Priority: "medium",
	}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-stranded", TaskID: "task-stranded", State: models.TaskSessionStateRunning,
		AgentProfileID: "agent-1", IsPrimary: true,
	}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}

	// Exhaust finalizeCancelledSessions's bounded in-line retry during
	// ArchiveTask. ArchiveTask must still succeed: the session-cancellation
	// failure is best-effort and never fails the archive itself.
	if err := svc.ArchiveTask(ctx, "task-stranded"); err != nil {
		t.Fatalf("ArchiveTask: %v", err)
	}
	if got, want := flaky.callCount(), 3; got != want {
		t.Fatalf("CancelActiveTaskSessionsByTaskID call count = %d, want %d (all 3 in-line attempts exhausted)", got, want)
	}

	session, err := repo.GetTaskSession(ctx, "session-stranded")
	if err != nil {
		t.Fatalf("GetTaskSession: %v", err)
	}
	if session.State != models.TaskSessionStateRunning {
		t.Fatalf("session state after ArchiveTask = %q, want RUNNING (retries should be exhausted, leaving the session stranded)", session.State)
	}

	// flaky.failuresLeft is now exhausted (0 remaining), so the sweep's call
	// to CancelActiveTaskSessionsByTaskID succeeds. Call the sweep's
	// single-run method directly, not the ticker loop.
	svc.runArchivedSessionReconciliation(ctx)

	session, err = repo.GetTaskSession(ctx, "session-stranded")
	if err != nil {
		t.Fatalf("GetTaskSession after reconciliation: %v", err)
	}
	if session.State != models.TaskSessionStateCancelled {
		t.Fatalf("session state after reconciliation = %q, want CANCELLED", session.State)
	}

	var found *bus.Event
	for _, evt := range eventBus.GetPublishedEvents() {
		if evt.Type != events.TaskSessionStateChanged {
			continue
		}
		data, ok := evt.Data.(map[string]interface{})
		if ok && data["session_id"] == "session-stranded" {
			found = evt
		}
	}
	if found == nil {
		t.Fatal("expected a session.state_changed event for session-stranded from the reconciliation sweep, got none")
	}
	data := found.Data.(map[string]interface{})
	if got := data["new_state"]; got != string(models.TaskSessionStateCancelled) {
		t.Errorf("new_state = %v, want CANCELLED", got)
	}
	if got := data["task_id"]; got != "task-stranded" {
		t.Errorf("task_id = %v, want task-stranded", got)
	}
}
