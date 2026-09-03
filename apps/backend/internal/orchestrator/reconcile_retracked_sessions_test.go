package orchestrator

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// countingTurnService is a minimal TurnService stub that only needs to
// record AbandonOpenTurns calls; every other method panics if a test
// exercises a path that was not supposed to reach it.
type countingTurnService struct {
	mu                   sync.Mutex
	abandonOpenTurnsCall int
}

func (c *countingTurnService) AbandonOpenTurns(context.Context, string) error {
	c.mu.Lock()
	c.abandonOpenTurnsCall++
	c.mu.Unlock()
	return nil
}
func (c *countingTurnService) calls() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.abandonOpenTurnsCall
}
func (*countingTurnService) StartTurn(context.Context, string) (*models.Turn, error) {
	panic("countingTurnService: StartTurn should not be called")
}
func (*countingTurnService) ReserveTurn(context.Context, string, *models.PromptDispatchRecovery) (*models.Turn, error) {
	panic("countingTurnService: ReserveTurn should not be called")
}
func (*countingTurnService) MarkReservedTurnDispatchAttempted(context.Context, *models.Turn) error {
	panic("countingTurnService: MarkReservedTurnDispatchAttempted should not be called")
}
func (*countingTurnService) PublishReservedTurn(context.Context, *models.Turn) error {
	panic("countingTurnService: PublishReservedTurn should not be called")
}
func (*countingTurnService) RollbackReservedTurn(context.Context, string, string) (bool, error) {
	panic("countingTurnService: RollbackReservedTurn should not be called")
}
func (*countingTurnService) ReconcileUnpublishedPromptTurns(context.Context) (int, error) {
	return 0, nil
}
func (*countingTurnService) CompleteTurn(context.Context, string) error { return nil }
func (*countingTurnService) GetTurn(context.Context, string) (*models.Turn, error) {
	return nil, nil
}
func (*countingTurnService) GetActiveTurn(context.Context, string) (*models.Turn, error) {
	return nil, nil
}
func (*countingTurnService) UpdateTurn(context.Context, *models.Turn) error { return nil }
func (*countingTurnService) PatchTurnMetadata(context.Context, string, string, map[string]interface{}) error {
	return nil
}

// TestReconcileSessionsOnStartupSkipsReconciliationWhenRetracked pins
// AC-EXECUTORS-SURVIVAL-003.2/.3/.4/.5: a session the lifecycle manager
// reports as re-tracked keeps its RUNNING state, its task stays IN_PROGRESS,
// its task is never marked interrupted, and its open turns are never
// abandoned -- because its agent actually survived the restart, none of
// today's "backend died mid-turn" cleanup applies to it.
func TestReconcileSessionsOnStartupSkipsReconciliationWhenRetracked(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()
	now := time.Now().UTC()

	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateRunning)

	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID:        "er1",
		SessionID: "session1",
		TaskID:    "task1",
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("failed to upsert executor running: %v", err)
	}

	taskRepo := newMockTaskRepo()
	taskRepo.tasks["task1"] = &v1.Task{ID: "task1", State: v1.TaskStateInProgress}

	turns := &countingTurnService{}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), taskRepo, &mockAgentManager{})
	svc.turnService = turns
	svc.SetRetrackedSessionChecker(func(sessionID string) bool { return sessionID == "session1" })

	svc.reconcileSessionsOnStartup(ctx)

	session, err := repo.GetTaskSession(ctx, "session1")
	if err != nil {
		t.Fatalf("failed to load session: %v", err)
	}
	if session.State != models.TaskSessionStateRunning {
		t.Fatalf("session state = %q, want %q (AC-EXECUTORS-SURVIVAL-003.3)", session.State, models.TaskSessionStateRunning)
	}

	if state, ok := taskRepo.updatedStates["task1"]; ok {
		t.Fatalf("expected task state left untouched (AC-EXECUTORS-SURVIVAL-003.5), got write to %q", state)
	}

	task, err := repo.GetTask(ctx, "task1")
	if err != nil {
		t.Fatalf("failed to load task: %v", err)
	}
	if _, marked := task.Metadata[models.MetaKeyInterruptedAt]; marked {
		t.Fatal("expected task NOT to be marked interrupted (AC-EXECUTORS-SURVIVAL-003.2)")
	}

	if got := turns.calls(); got != 0 {
		t.Fatalf("AbandonOpenTurns called %d times, want 0 (AC-EXECUTORS-SURVIVAL-003.4)", got)
	}
}

// TestReconcileSessionsOnStartupAppliesFullReconciliationWhenNotRetracked is
// the regression pin for the same fixture as above, but with the checker
// reporting the session as NOT re-tracked -- today's full "backend died
// mid-turn" reconciliation must still apply exactly as before Layer 6.6.
func TestReconcileSessionsOnStartupAppliesFullReconciliationWhenNotRetracked(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()
	now := time.Now().UTC()

	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateRunning)

	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID:        "er1",
		SessionID: "session1",
		TaskID:    "task1",
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("failed to upsert executor running: %v", err)
	}

	taskRepo := newMockTaskRepo()
	taskRepo.tasks["task1"] = &v1.Task{ID: "task1", State: v1.TaskStateInProgress}

	turns := &countingTurnService{}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), taskRepo, &mockAgentManager{})
	svc.turnService = turns
	svc.SetRetrackedSessionChecker(func(sessionID string) bool { return false })

	svc.reconcileSessionsOnStartup(ctx)

	session, err := repo.GetTaskSession(ctx, "session1")
	if err != nil {
		t.Fatalf("failed to load session: %v", err)
	}
	if session.State != models.TaskSessionStateWaitingForInput {
		t.Fatalf("session state = %q, want %q", session.State, models.TaskSessionStateWaitingForInput)
	}

	if state, ok := taskRepo.updatedStates["task1"]; !ok || state != v1.TaskStateReview {
		t.Fatalf("expected task moved to REVIEW, updatedStates[task1] = %q (ok=%v)", state, ok)
	}

	task, err := repo.GetTask(ctx, "task1")
	if err != nil {
		t.Fatalf("failed to load task: %v", err)
	}
	if _, marked := task.Metadata[models.MetaKeyInterruptedAt]; !marked {
		t.Fatal("expected task to be marked interrupted")
	}

	if got := turns.calls(); got != 1 {
		t.Fatalf("AbandonOpenTurns called %d times, want 1", got)
	}
}

// TestReconcileSessionsOnStartupUnwiredCheckerTreatsEverySessionAsNotRetracked
// pins the nil-checker default: when SetRetrackedSessionChecker is never
// called (agent survival disabled, or backendapp wiring absent), every
// active session is reconciled exactly as before Layer 6.6.
func TestReconcileSessionsOnStartupUnwiredCheckerTreatsEverySessionAsNotRetracked(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()
	now := time.Now().UTC()

	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateRunning)

	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID:        "er1",
		SessionID: "session1",
		TaskID:    "task1",
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("failed to upsert executor running: %v", err)
	}

	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), &mockAgentManager{})
	svc.reconcileSessionsOnStartup(ctx)

	session, err := repo.GetTaskSession(ctx, "session1")
	if err != nil {
		t.Fatalf("failed to load session: %v", err)
	}
	if session.State != models.TaskSessionStateWaitingForInput {
		t.Fatalf("session state = %q, want %q", session.State, models.TaskSessionStateWaitingForInput)
	}
}
