package executor

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// helper: a task v1 used by the office tests below.
func officeTestTask() *v1.Task {
	return &v1.Task{
		ID:          "task-office",
		WorkspaceID: "ws-office",
		Title:       "Office task",
	}
}

func TestEnsureSessionForAgent_CreatesWhenMissing(t *testing.T) {
	repo := newMockRepository()
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	task := officeTestTask()
	ctx := context.Background()

	got, err := exec.EnsureSessionForAgent(ctx, task, "agent-1", "profile-1", "exec-1", "")
	if err != nil {
		t.Fatalf("EnsureSessionForAgent: %v", err)
	}
	if got == nil || got.ID == "" {
		t.Fatal("expected new session")
	}
	if got.AgentProfileID != "agent-1" {
		t.Errorf("agent_profile_id: got %q want agent-1", got.AgentProfileID)
	}
	if got.State != models.TaskSessionStateCreated {
		t.Errorf("state: got %q want CREATED", got.State)
	}
	if len(repo.createTaskSessionCalls) != 1 {
		t.Fatalf("expected 1 CreateTaskSession call, got %d", len(repo.createTaskSessionCalls))
	}
}

// TestEnsureSessionForAgent_ReusesIdleAndFlipsRunning covers the canonical
// reuse path: the second run for the same (task, agent) reuses the existing
// row and flips IDLE → RUNNING. No new row is inserted.
func TestEnsureSessionForAgent_ReusesIdleAndFlipsRunning(t *testing.T) {
	repo := newMockRepository()
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	ctx := context.Background()

	existing := &models.TaskSession{
		ID:             "sess-existing",
		TaskID:         "task-office",
		AgentProfileID: "agent-1",
		State:          models.TaskSessionStateIdle,
		StartedAt:      time.Now().UTC(),
	}
	repo.sessions[existing.ID] = existing

	got, err := exec.EnsureSessionForAgent(ctx, officeTestTask(), "agent-1", "profile-1", "exec-1", "")
	if err != nil {
		t.Fatalf("EnsureSessionForAgent: %v", err)
	}
	if got.ID != existing.ID {
		t.Errorf("expected reuse of %q, got %q", existing.ID, got.ID)
	}
	if got.State != models.TaskSessionStateRunning {
		t.Errorf("state: got %q want RUNNING", got.State)
	}
	if len(repo.createTaskSessionCalls) != 0 {
		t.Errorf("expected zero create calls, got %d", len(repo.createTaskSessionCalls))
	}
}

// TestEnsureSessionForAgent_ReusesActiveStates covers RUNNING / STARTING /
// CREATED / WAITING_FOR_INPUT — each is returned as-is, idempotent.
func TestEnsureSessionForAgent_ReusesActiveStates(t *testing.T) {
	for _, st := range []models.TaskSessionState{
		models.TaskSessionStateCreated,
		models.TaskSessionStateStarting,
		models.TaskSessionStateRunning,
		models.TaskSessionStateWaitingForInput,
	} {
		t.Run(string(st), func(t *testing.T) {
			repo := newMockRepository()
			exec := newTestExecutor(t, &mockAgentManager{}, repo)

			existing := &models.TaskSession{
				ID:             "sess-" + string(st),
				TaskID:         "task-office",
				AgentProfileID: "agent-1",
				State:          st,
				StartedAt:      time.Now().UTC(),
			}
			repo.sessions[existing.ID] = existing

			got, err := exec.EnsureSessionForAgent(context.Background(), officeTestTask(), "agent-1", "profile-1", "", "")
			if err != nil {
				t.Fatalf("EnsureSessionForAgent: %v", err)
			}
			if got.ID != existing.ID {
				t.Errorf("expected reuse of %q, got %q", existing.ID, got.ID)
			}
			if got.State != st {
				t.Errorf("state mutated: got %q want %q", got.State, st)
			}
			if len(repo.createTaskSessionCalls) != 0 {
				t.Errorf("expected zero create calls, got %d", len(repo.createTaskSessionCalls))
			}
		})
	}
}

// TestEnsureSessionForAgent_TerminalRowsCreateFresh covers the "agent was
// removed and re-added" case: a prior COMPLETED / FAILED / CANCELLED row is
// preserved and a new row is created on the next run.
func TestEnsureSessionForAgent_TerminalRowsCreateFresh(t *testing.T) {
	for _, st := range []models.TaskSessionState{
		models.TaskSessionStateCompleted,
		models.TaskSessionStateFailed,
		models.TaskSessionStateCancelled,
	} {
		t.Run(string(st), func(t *testing.T) {
			repo := newMockRepository()
			exec := newTestExecutor(t, &mockAgentManager{}, repo)

			existing := &models.TaskSession{
				ID:             "sess-old-" + string(st),
				TaskID:         "task-office",
				AgentProfileID: "agent-1",
				State:          st,
				StartedAt:      time.Now().UTC(),
			}
			repo.sessions[existing.ID] = existing

			got, err := exec.EnsureSessionForAgent(context.Background(), officeTestTask(), "agent-1", "profile-1", "", "")
			if err != nil {
				t.Fatalf("EnsureSessionForAgent: %v", err)
			}
			if got.ID == existing.ID {
				t.Errorf("expected fresh row, got reuse of %q", existing.ID)
			}
			if got.State != models.TaskSessionStateCreated {
				t.Errorf("state: got %q want CREATED", got.State)
			}
			if len(repo.createTaskSessionCalls) != 1 {
				t.Errorf("expected 1 CreateTaskSession call, got %d", len(repo.createTaskSessionCalls))
			}
		})
	}
}

// TestEnsureSessionForAgent_RejectsMissingAgentID reports an error rather
// than silently inserting a row with an empty agent_profile_id (which would
// defeat the partial unique index).
func TestEnsureSessionForAgent_RejectsMissingAgentID(t *testing.T) {
	repo := newMockRepository()
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	if _, err := exec.EnsureSessionForAgent(context.Background(), officeTestTask(), "", "profile-1", "", ""); err == nil {
		t.Error("expected error when agent_profile_id is empty")
	}
}

// TestEnsureSessionForAgent_RejectsMissingProfile mirrors PrepareSession's
// ErrNoAgentProfileID guard.
func TestEnsureSessionForAgent_RejectsMissingProfile(t *testing.T) {
	repo := newMockRepository()
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	if _, err := exec.EnsureSessionForAgent(context.Background(), officeTestTask(), "agent-1", "", "", ""); err == nil {
		t.Error("expected ErrNoAgentProfileID, got nil")
	}
}
