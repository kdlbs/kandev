package executor

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
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

type officeRebindRaceRepository struct {
	*mockRepository
	beforeGuardedUpdate func()
}

func (r *officeRebindRaceRepository) UpdateTaskSessionIfCurrentState(
	ctx context.Context,
	session *models.TaskSession,
	expected models.TaskSessionState,
) (bool, error) {
	if r.beforeGuardedUpdate != nil {
		hook := r.beforeGuardedUpdate
		r.beforeGuardedUpdate = nil
		hook()
	}
	return r.mockRepository.UpdateTaskSessionIfCurrentState(ctx, session, expected)
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
	if got.ExecutionProfileID != "profile-1" {
		t.Errorf("execution_profile_id: got %q want profile-1", got.ExecutionProfileID)
	}
	if got.State != models.TaskSessionStateCreated {
		t.Errorf("state: got %q want CREATED", got.State)
	}
	if len(repo.createTaskSessionCalls) != 1 {
		t.Fatalf("expected 1 CreateTaskSession call, got %d", len(repo.createTaskSessionCalls))
	}
}

func TestEnsureSessionForAgent_RebindsExecutionProfileOnReuse(t *testing.T) {
	repo := newMockRepository()
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	existing := &models.TaskSession{
		ID:                 "sess-existing",
		TaskID:             "task-office",
		AgentProfileID:     "agent-1",
		ExecutionProfileID: "codex-profile",
		State:              models.TaskSessionStateIdle,
		StartedAt:          time.Now().UTC(),
	}
	repo.sessions[existing.ID] = existing

	got, err := exec.EnsureSessionForAgent(
		context.Background(), officeTestTask(), "agent-1", "claude-profile", "exec-1", "",
	)
	if err != nil {
		t.Fatalf("EnsureSessionForAgent: %v", err)
	}
	if got.AgentProfileID != "agent-1" {
		t.Fatalf("office identity changed: %q", got.AgentProfileID)
	}
	if got.ExecutionProfileID != "claude-profile" {
		t.Fatalf("execution profile = %q, want claude-profile", got.ExecutionProfileID)
	}
}

func TestEnsureSessionForAgent_RebindDoesNotOverwriteConcurrentCancellation(t *testing.T) {
	baseRepo := newMockRepository()
	existing := &models.TaskSession{
		ID:                 "sess-existing",
		TaskID:             "task-office",
		AgentProfileID:     "agent-1",
		ExecutionProfileID: "codex-profile",
		State:              models.TaskSessionStateIdle,
		StartedAt:          time.Now().UTC(),
	}
	baseRepo.sessions[existing.ID] = existing
	raceRepo := &officeRebindRaceRepository{mockRepository: baseRepo}
	raceRepo.beforeGuardedUpdate = func() {
		baseRepo.mu.Lock()
		cancelled := *baseRepo.sessions[existing.ID]
		cancelled.State = models.TaskSessionStateCancelled
		cancelled.ErrorMessage = "stopped by parent task via MCP"
		baseRepo.sessions[existing.ID] = &cancelled
		baseRepo.mu.Unlock()
	}
	exec := newTestExecutor(t, &mockAgentManager{}, baseRepo)
	exec.repo = raceRepo

	got, err := exec.EnsureSessionForAgent(
		context.Background(), officeTestTask(), "agent-1", "claude-profile", "exec-1", "",
	)
	if err != nil {
		t.Fatalf("EnsureSessionForAgent: %v", err)
	}
	if got.ID == existing.ID {
		t.Fatalf("cancelled session %q was reused", existing.ID)
	}

	baseRepo.mu.Lock()
	stored := baseRepo.sessions[existing.ID]
	baseRepo.mu.Unlock()
	if stored.State != models.TaskSessionStateCancelled {
		t.Fatalf("existing session state = %q, want CANCELLED", stored.State)
	}
	if stored.ErrorMessage != "stopped by parent task via MCP" {
		t.Fatalf("existing session error = %q, want coordinator stop reason", stored.ErrorMessage)
	}
	if stored.ExecutionProfileID != "codex-profile" {
		t.Fatalf("cancelled session profile = %q, want original profile", stored.ExecutionProfileID)
	}
}

func TestEnsureSessionForAgent_RebindsExecutionProfileAfterCreateRace(t *testing.T) {
	repo := newMockRepository()
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	winner := &models.TaskSession{
		ID:                 "sess-winner",
		TaskID:             "task-office",
		AgentProfileID:     "agent-1",
		ExecutionProfileID: "codex-profile",
		State:              models.TaskSessionStateIdle,
		StartedAt:          time.Now().UTC(),
	}
	repo.createTaskSessionFunc = func(_ context.Context, _ *models.TaskSession) error {
		repo.mu.Lock()
		repo.sessions[winner.ID] = winner
		repo.mu.Unlock()
		return fmt.Errorf("%w: concurrent insert", taskrepo.ErrOfficeSessionRaceConflict)
	}

	got, err := exec.EnsureSessionForAgent(
		context.Background(), officeTestTask(), "agent-1", "claude-profile", "exec-1", "",
	)
	if err != nil {
		t.Fatalf("EnsureSessionForAgent: %v", err)
	}
	if got.ID != winner.ID {
		t.Fatalf("session = %q, want race winner %q", got.ID, winner.ID)
	}
	if got.AgentProfileID != "agent-1" {
		t.Fatalf("office identity changed: %q", got.AgentProfileID)
	}
	if got.ExecutionProfileID != "claude-profile" {
		t.Fatalf("execution profile = %q, want claude-profile", got.ExecutionProfileID)
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
