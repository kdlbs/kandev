package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

// TestPostgresCompletionIntentLeaseAndCompareAndSet is the PostgreSQL parity
// coverage for the exact-turn completion state machine. The environment-gated
// test proves conflict identity, due scans, and compare-and-set settlement do
// not rely on SQLite serialization.
func TestPostgresCompletionIntentLeaseAndCompareAndSet(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("NewWithDB: %v", err)
	}
	ctx := context.Background()
	now := time.Date(2026, time.August, 20, 12, 0, 0, 0, time.UTC)
	if err := repo.CreateTask(ctx, &models.Task{ID: "task-pg-intent", Title: "Task"}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: "session-pg-intent", TaskID: "task-pg-intent"}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}
	if err := repo.CreateTurn(ctx, &models.Turn{ID: "turn-pg-intent", TaskID: "task-pg-intent", TaskSessionID: "session-pg-intent", StartedAt: now}); err != nil {
		t.Fatalf("CreateTurn: %v", err)
	}
	intent := &models.CompletionIntent{
		ID: "intent-pg", TaskID: "task-pg-intent", SessionID: "session-pg-intent", TurnID: "turn-pg-intent",
		WorkflowStepID: "work", State: models.CompletionIntentStatePending, RequestedAt: now, EligibleAt: now,
	}
	created, stored, err := repo.CreateOrGetCompletionIntent(ctx, intent)
	if err != nil || !created || stored.ID != intent.ID {
		t.Fatalf("CreateOrGetCompletionIntent = (%v, %+v, %v)", created, stored, err)
	}
	created, duplicate, err := repo.CreateOrGetCompletionIntent(ctx, intent)
	if err != nil || created || duplicate.ID != intent.ID {
		t.Fatalf("duplicate CreateOrGetCompletionIntent = (%v, %+v, %v)", created, duplicate, err)
	}
	due, err := repo.ListDueCompletionIntents(ctx, now, 2)
	if err != nil || len(due) != 1 || due[0].ID != intent.ID {
		t.Fatalf("ListDueCompletionIntents = (%+v, %v)", due, err)
	}
	claimed, err := repo.TransitionCompletionIntent(ctx, intent.ID, models.CompletionIntentStatePending, models.CompletionIntentStateSettling, now)
	if err != nil || !claimed {
		t.Fatalf("pending->settling = (%v, %v)", claimed, err)
	}
	claimed, err = repo.TransitionCompletionIntent(ctx, intent.ID, models.CompletionIntentStatePending, models.CompletionIntentStateSettling, now)
	if err != nil || claimed {
		t.Fatalf("duplicate pending->settling = (%v, %v)", claimed, err)
	}
	count, err := repo.CountPendingCompletionIntents(ctx)
	if err != nil || count != 0 {
		t.Fatalf("CountPendingCompletionIntents = (%d, %v), want 0", count, err)
	}
}
