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
	byTurn, err := repo.GetCompletionIntentForTurn(ctx, intent.SessionID, intent.TurnID)
	if err != nil || byTurn.ID != intent.ID {
		t.Fatalf("GetCompletionIntentForTurn = (%+v, %v), want %q", byTurn, err, intent.ID)
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

func TestCompletionIntentListDueOrdersAndLimitsPendingIntents(t *testing.T) {
	ctx := context.Background()
	repo := newRepoForSessionTests(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	for _, candidate := range []struct {
		id       string
		eligible time.Time
		state    models.CompletionIntentState
	}{
		{id: "first", eligible: now.Add(-2 * time.Minute), state: models.CompletionIntentStatePending},
		{id: "second", eligible: now.Add(-time.Minute), state: models.CompletionIntentStatePending},
		{id: "future", eligible: now.Add(time.Minute), state: models.CompletionIntentStatePending},
		{id: "settled", eligible: now.Add(-3 * time.Minute), state: models.CompletionIntentStateSettled},
	} {
		taskID := "task-" + candidate.id
		sessionID := "session-" + candidate.id
		turnID := "turn-" + candidate.id
		if err := repo.CreateTask(ctx, &models.Task{ID: taskID, Title: "Task"}); err != nil {
			t.Fatalf("CreateTask(%s): %v", candidate.id, err)
		}
		if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: sessionID, TaskID: taskID}); err != nil {
			t.Fatalf("CreateTaskSession(%s): %v", candidate.id, err)
		}
		if err := repo.CreateTurn(ctx, &models.Turn{ID: turnID, TaskID: taskID, TaskSessionID: sessionID, StartedAt: now}); err != nil {
			t.Fatalf("CreateTurn(%s): %v", candidate.id, err)
		}
		_, _, err := repo.CreateOrGetCompletionIntent(ctx, &models.CompletionIntent{
			ID: candidate.id, TaskID: taskID, SessionID: sessionID, TurnID: turnID, WorkflowStepID: "step",
			State: candidate.state, RequestedAt: now, EligibleAt: candidate.eligible,
		})
		if err != nil {
			t.Fatalf("CreateOrGetCompletionIntent(%s): %v", candidate.id, err)
		}
	}

	due, err := repo.ListDueCompletionIntents(ctx, now, 1)
	if err != nil {
		t.Fatalf("ListDueCompletionIntents: %v", err)
	}
	if len(due) != 1 || due[0].ID != "first" {
		t.Fatalf("ListDueCompletionIntents = %+v, want only first", due)
	}
}

func TestCompletionIntentRearmDefersOnlyPendingIntent(t *testing.T) {
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
	_, _, err := repo.CreateOrGetCompletionIntent(ctx, &models.CompletionIntent{
		ID: "intent", TaskID: "task", SessionID: "session", TurnID: "turn", WorkflowStepID: "step",
		State: models.CompletionIntentStatePending, RequestedAt: now, EligibleAt: now,
	})
	if err != nil {
		t.Fatalf("CreateOrGetCompletionIntent: %v", err)
	}

	activityAt := now.Add(time.Second)
	rearmed, err := repo.RearmCompletionIntent(ctx, "intent", activityAt, activityAt.Add(time.Minute))
	if err != nil || !rearmed {
		t.Fatalf("RearmCompletionIntent = (%v, %v), want true nil", rearmed, err)
	}
	due, err := repo.ListDueCompletionIntents(ctx, activityAt, 10)
	if err != nil {
		t.Fatalf("ListDueCompletionIntents: %v", err)
	}
	if len(due) != 0 {
		t.Fatalf("ListDueCompletionIntents after activity = %+v, want no due intents", due)
	}
}
