package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func TestCompletionIntentCreateOrGetAndCompareAndSet(t *testing.T) {
	ctx := context.Background()
	repo := newRepoForSessionTests(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	if err := repo.CreateTask(ctx, &models.Task{ID: "task", Title: "Task"}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: "session", TaskID: "task"}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}
	if err := repo.CreateTurn(ctx, &models.Turn{ID: "turn", TaskID: "task", TaskSessionID: "session", StartedAt: now}); err != nil {
		t.Fatalf("CreateTurn: %v", err)
	}

	intent := &models.CompletionIntent{
		ID: "intent", TaskID: "task", SessionID: "session", TurnID: "turn", WorkflowStepID: "step",
		State: models.CompletionIntentStatePending, RequestedAt: now, EligibleAt: now,
	}
	created, got, err := repo.CreateOrGetCompletionIntent(ctx, intent)
	if err != nil || !created || got.ID != intent.ID {
		t.Fatalf("CreateOrGetCompletionIntent = (%v, %+v, %v), want created intent", created, got, err)
	}
	created, got, err = repo.CreateOrGetCompletionIntent(ctx, intent)
	if err != nil || created || got.ID != intent.ID {
		t.Fatalf("duplicate CreateOrGetCompletionIntent = (%v, %+v, %v)", created, got, err)
	}
	settled, err := repo.TransitionCompletionIntent(ctx, intent.ID, models.CompletionIntentStatePending, models.CompletionIntentStateSettling, now)
	if err != nil || !settled {
		t.Fatalf("TransitionCompletionIntent pending->settling = (%v, %v)", settled, err)
	}
	settled, err = repo.TransitionCompletionIntent(ctx, intent.ID, models.CompletionIntentStatePending, models.CompletionIntentStateSettling, now)
	if err != nil || settled {
		t.Fatalf("duplicate compare-and-set = (%v, %v), want false nil", settled, err)
	}
}
