package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
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

func TestReconcileDueCompletionIntentsDoesNotSettleReplacedPromptGeneration(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	now := time.Now().UTC().Truncate(time.Microsecond)
	if err := repo.CreateTurn(ctx, &models.Turn{ID: "turn-1", TaskID: "t1", TaskSessionID: "s1", StartedAt: now}); err != nil {
		t.Fatalf("CreateTurn: %v", err)
	}
	_, _, err := repo.CreateOrGetCompletionIntent(ctx, &models.CompletionIntent{ID: "intent-1", TaskID: "t1", SessionID: "s1", TurnID: "turn-1", WorkflowStepID: "step1", AgentExecutionID: "execution-1", PromptGeneration: 1, State: models.CompletionIntentStatePending, RequestedAt: now, EligibleAt: now})
	if err != nil {
		t.Fatalf("CreateOrGetCompletionIntent: %v", err)
	}
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{ID: "running-1", TaskID: "t1", SessionID: "s1", AgentExecutionID: "execution-1", Status: "ready"}); err != nil {
		t.Fatalf("UpsertExecutorRunning: %v", err)
	}
	agentMgr := &mockAgentManager{currentPromptExecutionID: "execution-1"}
	agentMgr.currentPromptGeneration.Store(2)
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
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
		t.Fatal("a replaced prompt generation must retain the captured turn")
	}
}

func TestReconcileDueCompletionIntentsRetriesUnknownRecoveredPromptGeneration(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	now := time.Now().UTC().Truncate(time.Microsecond)
	if err := repo.CreateTurn(ctx, &models.Turn{ID: "turn-1", TaskID: "t1", TaskSessionID: "s1", StartedAt: now}); err != nil {
		t.Fatalf("CreateTurn: %v", err)
	}
	_, _, err := repo.CreateOrGetCompletionIntent(ctx, &models.CompletionIntent{
		ID: "intent-1", TaskID: "t1", SessionID: "s1", TurnID: "turn-1", WorkflowStepID: "step1",
		AgentExecutionID: "execution-1", PromptGeneration: 7, State: models.CompletionIntentStatePending,
		RequestedAt: now, EligibleAt: now,
	})
	if err != nil {
		t.Fatalf("CreateOrGetCompletionIntent: %v", err)
	}
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{ID: "running-1", TaskID: "t1", SessionID: "s1", AgentExecutionID: "execution-1", Status: "ready"}); err != nil {
		t.Fatalf("UpsertExecutorRunning: %v", err)
	}
	agentMgr := &mockAgentManager{currentPromptExecutionID: "execution-1"}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.turnService = &repoTurnService{repo: repo}

	svc.reconcileDueCompletionIntents(ctx)

	intent, err := repo.GetCompletionIntent(ctx, "intent-1")
	if err != nil {
		t.Fatalf("GetCompletionIntent: %v", err)
	}
	if intent.State != models.CompletionIntentStatePending {
		t.Fatalf("intent state = %q, want pending retry while recovered generation is unknown", intent.State)
	}
	turn, err := repo.GetTurn(ctx, "turn-1")
	if err != nil {
		t.Fatalf("GetTurn: %v", err)
	}
	if turn.CompletedAt != nil {
		t.Fatal("unknown recovered generation must keep the captured turn open")
	}
}

func TestReconcileDueCompletionIntentsDoesNotSettleAfterQueuedUserWork(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	now := time.Now().UTC().Truncate(time.Microsecond)
	if err := repo.CreateTurn(ctx, &models.Turn{ID: "turn-1", TaskID: "t1", TaskSessionID: "s1", StartedAt: now}); err != nil {
		t.Fatalf("CreateTurn: %v", err)
	}
	_, _, err := repo.CreateOrGetCompletionIntent(ctx, &models.CompletionIntent{
		ID: "intent-1", TaskID: "t1", SessionID: "s1", TurnID: "turn-1", WorkflowStepID: "step1",
		AgentExecutionID: "execution-1", PromptGeneration: 1, State: models.CompletionIntentStatePending,
		RequestedAt: now.Add(-time.Second), EligibleAt: now,
	})
	if err != nil {
		t.Fatalf("CreateOrGetCompletionIntent: %v", err)
	}
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{ID: "running-1", TaskID: "t1", SessionID: "s1", AgentExecutionID: "execution-1", Status: "ready"}); err != nil {
		t.Fatalf("UpsertExecutorRunning: %v", err)
	}
	agentMgr := &mockAgentManager{currentPromptExecutionID: "execution-1"}
	agentMgr.currentPromptGeneration.Store(1)
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.turnService = &repoTurnService{repo: repo}
	if _, err := svc.messageQueue.QueueMessage(ctx, "s1", "t1", "continue working", "", messagequeue.QueuedByUser, false, nil); err != nil {
		t.Fatalf("QueueMessage: %v", err)
	}

	svc.reconcileDueCompletionIntents(ctx)

	intent, err := repo.GetCompletionIntent(ctx, "intent-1")
	if err != nil {
		t.Fatalf("GetCompletionIntent: %v", err)
	}
	if intent.State != models.CompletionIntentStateReopened {
		t.Fatalf("intent state = %q, want reopened", intent.State)
	}
	turn, err := repo.GetTurn(ctx, "turn-1")
	if err != nil {
		t.Fatalf("GetTurn: %v", err)
	}
	if turn.CompletedAt != nil {
		t.Fatal("queued user work must keep the captured turn open")
	}
}

func TestQueuedUserWorkAdmissionUsesCompletionSettlementGuard(t *testing.T) {
	type queuedUserWorkAdmitter interface {
		AdmitQueuedUserWork(
			context.Context, string, string,
			func(context.Context) (*messagequeue.QueuedMessage, error),
		) (*messagequeue.QueuedMessage, error)
	}

	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	now := time.Now().UTC().Truncate(time.Microsecond)
	if err := repo.CreateTurn(ctx, &models.Turn{ID: "turn-admission", TaskID: "t1", TaskSessionID: "s1", StartedAt: now}); err != nil {
		t.Fatalf("CreateTurn: %v", err)
	}
	_, _, err := repo.CreateOrGetCompletionIntent(ctx, &models.CompletionIntent{
		ID: "intent-admission", TaskID: "t1", SessionID: "s1", TurnID: "turn-admission", WorkflowStepID: "step1",
		State: models.CompletionIntentStatePending, RequestedAt: now, EligibleAt: now.Add(time.Second),
	})
	if err != nil {
		t.Fatalf("CreateOrGetCompletionIntent: %v", err)
	}
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.turnService = &repoTurnService{repo: repo}
	admitter, ok := interface{}(svc).(queuedUserWorkAdmitter)
	if !ok {
		t.Fatal("orchestrator service does not guard queued user-work admission")
	}
	guard, release := svc.acquireCancelInFlightGuard("s1")
	guard.Lock()
	defer release()

	callbackStarted := make(chan struct{})
	result := make(chan error, 1)
	go func() {
		_, err := admitter.AdmitQueuedUserWork(ctx, "t1", "s1", func(admittedCtx context.Context) (*messagequeue.QueuedMessage, error) {
			close(callbackStarted)
			return svc.messageQueue.QueueMessage(admittedCtx, "s1", "t1", "continue working", "", messagequeue.QueuedByUser, false, nil)
		})
		result <- err
	}()

	select {
	case <-callbackStarted:
		t.Fatal("queue admission started before the completion-settlement guard was released")
	case <-time.After(25 * time.Millisecond):
	}
	guard.Unlock()
	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("AdmitQueuedUserWork: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for guarded queue admission")
	}
	intent, err := repo.GetCompletionIntent(ctx, "intent-admission")
	if err != nil {
		t.Fatalf("GetCompletionIntent: %v", err)
	}
	if intent.State != models.CompletionIntentStateReopened {
		t.Fatalf("intent state after queued work = %q, want reopened", intent.State)
	}
}
