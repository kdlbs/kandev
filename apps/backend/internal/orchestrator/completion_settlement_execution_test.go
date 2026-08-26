package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func TestReconcileDueCompletionIntentsDoesNotSettleReplacedExecution(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	now := time.Now().UTC().Truncate(time.Microsecond)
	if err := repo.CreateTurn(ctx, &models.Turn{ID: "turn-1", TaskID: "t1", TaskSessionID: "s1", StartedAt: now}); err != nil {
		t.Fatalf("CreateTurn: %v", err)
	}
	_, _, err := repo.CreateOrGetCompletionIntent(ctx, &models.CompletionIntent{ID: "intent-1", TaskID: "t1", SessionID: "s1", TurnID: "turn-1", WorkflowStepID: "step1", AgentExecutionID: "execution-old", State: models.CompletionIntentStatePending, RequestedAt: now, EligibleAt: now})
	if err != nil {
		t.Fatalf("CreateOrGetCompletionIntent: %v", err)
	}
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{ID: "running-1", TaskID: "t1", SessionID: "s1", AgentExecutionID: "execution-new", Status: "ready"}); err != nil {
		t.Fatalf("UpsertExecutorRunning: %v", err)
	}

	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.turnService = &repoTurnService{repo: repo}
	svc.reconcileDueCompletionIntents(ctx)

	intent, err := repo.GetCompletionIntent(ctx, "intent-1")
	if err != nil {
		t.Fatalf("GetCompletionIntent: %v", err)
	}
	if intent.State != models.CompletionIntentStateSuperseded {
		t.Fatalf("intent state = %q, want superseded", intent.State)
	}
	turn, err := repo.GetTurn(ctx, "turn-1")
	if err != nil {
		t.Fatalf("GetTurn: %v", err)
	}
	if turn.CompletedAt != nil {
		t.Fatal("a replaced execution must retain the captured turn")
	}
}
