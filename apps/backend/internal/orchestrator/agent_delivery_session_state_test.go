package orchestrator

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
)

type settledSessionStateFixture struct {
	service     *Service
	repo        *tasksqlite.Repository
	sessionID   string
	taskID      string
	incarnation string
	executionID string
	streamID    string
	submission  string
	turnID      string
}

func seedOrchestratorSettledSessionState(t *testing.T, suffix string, completeEffect bool) settledSessionStateFixture {
	t.Helper()
	service, repo := newServiceWithRealRepo(t)
	ctx := context.Background()
	fixture := settledSessionStateFixture{
		service: service, repo: repo,
		taskID:      "task-delivery-state-" + suffix,
		sessionID:   "session-delivery-state-" + suffix,
		incarnation: "incarnation-delivery-state-" + suffix,
		executionID: "execution-delivery-state-" + suffix,
		streamID:    "stream-delivery-state-" + suffix,
		submission:  "submission-delivery-state-" + suffix,
		turnID:      "turn-delivery-state-" + suffix,
	}
	now := time.Now().UTC()
	if err := repo.CreateTask(ctx, &models.Task{ID: fixture.taskID, Title: "Settled delivery state"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: fixture.sessionID, TaskID: fixture.taskID, QueueIncarnationID: fixture.incarnation,
		State: models.TaskSessionStateRunning, StartedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: "running-delivery-state-" + suffix, SessionID: fixture.sessionID, TaskID: fixture.taskID,
		ExecutorID: "executor-delivery-state-" + suffix, AgentExecutionID: fixture.executionID,
		Status: "ready",
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateHarnessSessionGeneration(ctx, &models.HarnessSessionGeneration{
		SessionID: fixture.sessionID, IncarnationID: fixture.incarnation, Generation: 1,
		NativeSessionID: "native-delivery-state-" + suffix, CreationReason: "test",
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTurn(ctx, &models.Turn{
		ID: fixture.turnID, TaskID: fixture.taskID, TaskSessionID: fixture.sessionID,
		StartedAt: now, CompletedAt: &now, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.PrepareAgentDeliverySubmission(ctx, &models.AgentDeliverySubmission{
		ID: fixture.submission, SessionID: fixture.sessionID, IncarnationID: fixture.incarnation,
		HarnessGeneration: 1, OwnerGeneration: 1, PayloadHash: "hash-" + suffix,
		Payload: []byte("prompt"), State: models.DeliverySubmissionDispatching,
	}); err != nil {
		t.Fatal(err)
	}
	event := persistedTerminalEvent(fixture.sessionID, fixture.incarnation, 1,
		fixture.submission, fixture.streamID, "cancelled")
	event.Payload = []byte(`{"type":"cancelled","turn_id":"` + fixture.turnID + `"}`)
	projectOrchestratorDeliveryEvent(t, repo, ctx, event)
	if settled, err := repo.SettleAgentDeliveryTerminal(
		ctx, event.StreamID, event.Sequence, models.DeliverySubmissionCancelled, now,
	); err != nil || !settled {
		t.Fatalf("settle terminal = %v, %v", settled, err)
	}
	recordSettlementWorkflowOutcome(t, repo, ctx, event)
	if completeEffect {
		if completed, err := repo.CompleteAgentDeliveryTerminalSettlement(
			ctx, "agent_delivery.terminal:"+fixture.streamID+":1", now,
		); err != nil || !completed {
			t.Fatalf("complete terminal effect = %v, %v", completed, err)
		}
	}
	service.agentManager = &mockAgentManager{
		getExecutionIDForSessionFunc: func(context.Context, string) (string, error) {
			return fixture.executionID, nil
		},
		isAgentReadyFn: func(context.Context, string) bool { return true },
	}
	return fixture
}

func requireDeliverySessionState(t *testing.T, repo *tasksqlite.Repository, sessionID string, want models.TaskSessionState) {
	t.Helper()
	session, err := repo.GetTaskSession(context.Background(), sessionID)
	if err != nil || session == nil {
		t.Fatalf("load session = %+v, %v", session, err)
	}
	if session.State != want {
		t.Fatalf("session state = %s, want %s", session.State, want)
	}
}

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-005.2
// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-005.3
// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.5
func TestDeliverySessionStateConvergesAfterSettledCancellation(t *testing.T) {
	fixture := seedOrchestratorSettledSessionState(t, "converge", true)
	requireDeliverySessionState(t, fixture.repo, fixture.sessionID, models.TaskSessionStateRunning)
	if err := fixture.service.reconcileAgentDeliverySettlements(context.Background(), fixture.sessionID); err != nil {
		t.Fatal(err)
	}
	requireDeliverySessionState(t, fixture.repo, fixture.sessionID, models.TaskSessionStateWaitingForInput)
	if err := fixture.service.reconcileAgentDeliverySettlements(context.Background(), fixture.sessionID); err != nil {
		t.Fatal(err)
	}
	requireDeliverySessionState(t, fixture.repo, fixture.sessionID, models.TaskSessionStateWaitingForInput)
}

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.5
func TestDeliverySessionStateLeavesLiveSuccessorRunning(t *testing.T) {
	fixture := seedOrchestratorSettledSessionState(t, "successor", true)
	if _, err := fixture.repo.PrepareAgentDeliverySubmission(context.Background(), &models.AgentDeliverySubmission{
		ID: "submission-delivery-state-successor-live", SessionID: fixture.sessionID,
		IncarnationID: fixture.incarnation, HarnessGeneration: 1, OwnerGeneration: 1,
		PayloadHash: "hash-delivery-state-successor-live", Payload: []byte("next prompt"),
		State: models.DeliverySubmissionPrepared,
	}); err != nil {
		t.Fatal(err)
	}
	if err := fixture.service.reconcileAgentDeliverySettlements(context.Background(), fixture.sessionID); err != nil {
		t.Fatal(err)
	}
	requireDeliverySessionState(t, fixture.repo, fixture.sessionID, models.TaskSessionStateRunning)
}

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.5
func TestDeliverySessionStateRequiresIdleCurrentRuntime(t *testing.T) {
	fixture := seedOrchestratorSettledSessionState(t, "runtime", true)
	fixture.service.agentManager = &mockAgentManager{
		getExecutionIDForSessionFunc: func(context.Context, string) (string, error) {
			return "replacement-execution", nil
		},
		isAgentReadyFn: func(context.Context, string) bool { return true },
	}
	if err := fixture.service.reconcileAgentDeliverySettlements(context.Background(), fixture.sessionID); err != nil {
		t.Fatal(err)
	}
	requireDeliverySessionState(t, fixture.repo, fixture.sessionID, models.TaskSessionStateRunning)
	fixture.service.agentManager = &mockAgentManager{
		getExecutionIDForSessionFunc: func(context.Context, string) (string, error) {
			return fixture.executionID, nil
		},
		isAgentReadyFn: func(context.Context, string) bool { return false },
	}
	if err := fixture.service.reconcileAgentDeliverySettlements(context.Background(), fixture.sessionID); err != nil {
		t.Fatal(err)
	}
	requireDeliverySessionState(t, fixture.repo, fixture.sessionID, models.TaskSessionStateRunning)
}

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.5
func TestDeliverySessionStateRejectsReservedSuccessorAfterIdleProof(t *testing.T) {
	fixture := seedOrchestratorSettledSessionState(t, "reserved-successor", true)
	entered := make(chan struct{})
	release := make(chan struct{})
	fixture.service.deliverySessionStateTestHook = func() {
		close(entered)
		<-release
	}
	done := make(chan error, 1)
	go func() {
		done <- fixture.service.reconcileAgentDeliverySettlements(context.Background(), fixture.sessionID)
	}()
	<-entered
	now := time.Now().UTC()
	if err := fixture.repo.CreateTurn(context.Background(), &models.Turn{
		ID: "turn-delivery-state-reserved-successor-next", TaskID: fixture.taskID,
		TaskSessionID: fixture.sessionID, StartedAt: now, CreatedAt: now, UpdatedAt: now,
		Metadata: map[string]interface{}{models.TurnMetaKeyPromptDispatchPending: true},
	}); err != nil {
		t.Fatalf("create reserved successor turn: %v", err)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	requireDeliverySessionState(t, fixture.repo, fixture.sessionID, models.TaskSessionStateRunning)
	open, err := fixture.repo.GetTurn(context.Background(), "turn-delivery-state-reserved-successor-next")
	if err != nil || open == nil || open.CompletedAt != nil {
		t.Fatalf("reserved successor turn = %+v, %v", open, err)
	}
}

type settledSessionReadFailureStore struct {
	sessionExecutorStore
	repository.AgentDeliverySettlementRepository
	repository.AgentDeliveryRepository
	mu     sync.Mutex
	failAt int
	calls  int
}

func (r *settledSessionReadFailureStore) GetTaskSession(ctx context.Context, id string) (*models.TaskSession, error) {
	r.mu.Lock()
	r.calls++
	fail := r.failAt > 0 && r.calls == r.failAt
	r.mu.Unlock()
	if fail {
		return nil, errors.New("injected session read failure")
	}
	return r.sessionExecutorStore.GetTaskSession(ctx, id)
}

func (r *settledSessionReadFailureStore) setFailAt(failAt int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.failAt = failAt
	r.calls = 0
}

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-005.3
func TestDeliverySessionStateReadFailureKeepsTerminalSettlementRetryable(t *testing.T) {
	for _, failAt := range []int{1, 2} {
		t.Run("read failure "+string(rune('0'+failAt)), func(t *testing.T) {
			fixture := seedOrchestratorSettledSessionState(t, "read-failure-"+string(rune('0'+failAt)), false)
			failing := &settledSessionReadFailureStore{
				sessionExecutorStore:              fixture.service.repo,
				AgentDeliverySettlementRepository: fixture.repo,
				AgentDeliveryRepository:           fixture.repo,
			}
			fixture.service.repo = failing
			failing.setFailAt(failAt)
			if err := fixture.service.reconcileAgentDeliverySettlements(context.Background(), fixture.sessionID); err == nil {
				t.Fatal("injected session read failure was swallowed")
			}
			effect, err := fixture.repo.GetAgentDeliveryEffect(
				context.Background(), "agent_delivery.terminal:"+fixture.streamID+":1",
			)
			if err != nil || effect == nil || effect.State == models.DeliveryEffectCompleted {
				t.Fatalf("failed correction completed terminal outbox: %+v, %v", effect, err)
			}
			requireDeliverySessionState(t, fixture.repo, fixture.sessionID, models.TaskSessionStateRunning)
			failing.setFailAt(0)
			if err := fixture.service.reconcileAgentDeliverySettlements(context.Background(), fixture.sessionID); err != nil {
				t.Fatal(err)
			}
			requireDeliverySessionState(t, fixture.repo, fixture.sessionID, models.TaskSessionStateWaitingForInput)
			effect, err = fixture.repo.GetAgentDeliveryEffect(
				context.Background(), "agent_delivery.terminal:"+fixture.streamID+":1",
			)
			if err != nil || effect == nil || effect.State != models.DeliveryEffectCompleted {
				t.Fatalf("retry did not complete terminal outbox: %+v, %v", effect, err)
			}
		})
	}
}

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-005.3
func TestDeliverySessionStateRepairsCompletedOutboxAtStartup(t *testing.T) {
	fixture := seedOrchestratorSettledSessionState(t, "startup-repair", true)
	if err := fixture.service.reconcileAgentDeliverySettlements(context.Background(), ""); err != nil {
		t.Fatal(err)
	}
	requireDeliverySessionState(t, fixture.repo, fixture.sessionID, models.TaskSessionStateWaitingForInput)
}
