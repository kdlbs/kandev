package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type queuedStreamBoundaryAgentManager struct {
	*mockAgentManager
	service *Service
	prepare func(context.Context) error
}

func (m *queuedStreamBoundaryAgentManager) PromptAgentWithAdmissionCallback(
	ctx context.Context,
	executionID, prompt string,
	attachments []v1.MessageAttachment,
	dispatchOnly bool,
	beforeAdmission func() error,
	onDispatched func(),
) (*executor.PromptResult, error) {
	// This synchronous event takes the same session guard as the production
	// stream handler. Lifecycle performs this publication while preparing the
	// previous stream boundary, before its final admission callback.
	m.service.handleAgentStreamEvent(ctx, &lifecycle.AgentStreamEventPayload{
		TaskID: taskIDFromSession(ctx), SessionID: sessionIDFromContext(ctx),
		ExecutionID: executionID,
		Data:        &lifecycle.AgentStreamEventData{Type: "stream_boundary_test"},
	})
	if m.prepare != nil {
		if err := m.prepare(ctx); err != nil {
			return nil, err
		}
	}
	if beforeAdmission != nil {
		if err := beforeAdmission(); err != nil {
			return nil, err
		}
	}
	return m.PromptAgentWithDispatchCallback(
		ctx, executionID, prompt, attachments, dispatchOnly, onDispatched,
	)
}

type promptBoundaryContextKey string

const (
	promptBoundaryTaskContextKey    promptBoundaryContextKey = "prompt-boundary-task"
	promptBoundarySessionContextKey promptBoundaryContextKey = "prompt-boundary-session"
)

func taskIDFromSession(ctx context.Context) string {
	taskID, _ := ctx.Value(promptBoundaryTaskContextKey).(string)
	return taskID
}

func sessionIDFromContext(ctx context.Context) string {
	sessionID, _ := ctx.Value(promptBoundarySessionContextKey).(string)
	return sessionID
}

func TestPromptTask_QueuedStreamBoundaryDoesNotDeadlock(t *testing.T) {
	ctx := context.Background()
	const taskID, sessionID, executionID = "task-stream-boundary", "session-stream-boundary", "execution-stream-boundary"
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateWaitingForInput)
	seedExecutorRunning(t, repo, sessionID, taskID, executionID)
	baseAgent := &mockAgentManager{isAgentRunning: true, repoForExecutionLookup: repo}
	agent := &queuedStreamBoundaryAgentManager{mockAgentManager: baseAgent}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agent)
	agent.service = svc
	svc.executor = executor.NewExecutor(agent, repo, testLogger(), executor.ExecutorConfig{})
	svc.messageQueue = newAuthoritativeMemoryQueue(repo, testLogger())
	identity, err := svc.messageQueue.ResolveSessionIdentity(ctx, taskID, sessionID)
	if err != nil {
		t.Fatalf("resolve session identity: %v", err)
	}
	if err := svc.messageQueue.SetAutoRun(ctx, sessionID, true); err != nil {
		t.Fatalf("enable auto-run: %v", err)
	}
	if _, err := svc.messageQueue.QueueMessageWithMetadata(
		ctx, sessionID, taskID, "queued prompt", "", messagequeue.QueuedByUser, false, nil, nil,
	); err != nil {
		t.Fatalf("queue prompt: %v", err)
	}
	queued, ok, _, err := svc.messageQueue.ReserveQueuedWithAutoRunForSession(ctx, identity)
	if err != nil || !ok || queued == nil {
		t.Fatalf("reserve queued prompt: queued=%+v ok=%v err=%v", queued, ok, err)
	}
	reservation := svc.markQueuedDispatchInFlightWithIdentityLocked(identity, queued.ID, queued)
	if reservation == nil {
		t.Fatal("mark queued dispatch in flight returned nil")
	}
	if claimed, err := svc.claimQueuedDispatchForExecution(sessionID, queued.ID, reservation); err != nil || !claimed {
		t.Fatalf("claim queued dispatch for execution: claimed=%v err=%v", claimed, err)
	}
	ctx = context.WithValue(ctx, promptBoundaryTaskContextKey, taskID)
	ctx = context.WithValue(ctx, promptBoundarySessionContextKey, sessionID)

	promptDone := make(chan error, 1)
	go func() {
		_, promptErr := svc.promptTask(
			ctx, taskID, sessionID, "queued prompt", "", false, nil, true,
			launchOriginAutomatic,
			promptTaskOptions{claimEntryID: queued.ID, expectedSessionIdentity: &identity},
		)
		promptDone <- promptErr
	}()
	select {
	case promptErr := <-promptDone:
		if promptErr != nil {
			t.Fatalf("queued prompt dispatch: %v", promptErr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("queued prompt did not complete stream preparation and admission")
	}
	if got := capturedPromptCount(baseAgent); got != 1 {
		t.Fatalf("provider prompt calls = %d, want 1", got)
	}

	cancelDone := make(chan error, 1)
	go func() { cancelDone <- svc.CancelAgent(ctx, sessionID) }()
	select {
	case cancelErr := <-cancelDone:
		if cancelErr != nil {
			t.Fatalf("CancelAgent after queued dispatch: %v", cancelErr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("CancelAgent did not complete after stream-boundary dispatch")
	}
	if got := baseAgent.cancelAgentCalls.Load(); got != 1 {
		t.Fatalf("runtime cancel calls = %d, want 1", got)
	}
	execution, err := repo.GetExecutorRunningBySessionID(ctx, sessionID)
	if err != nil {
		t.Fatalf("read accepted execution after pause: %v", err)
	}
	if execution.AgentExecutionID != executionID || !isHealthyPromptExecutionStatus(execution.Status) {
		t.Fatalf("accepted execution after pause = id:%s status:%s, want same healthy execution %s", execution.AgentExecutionID, execution.Status, executionID)
	}
	if pending := svc.CancellationPending(sessionID); pending {
		t.Fatal("cancellation pending projection remained after settlement")
	}
	assertResumeSessionState(t, repo, sessionID, models.TaskSessionStateWaitingForInput)
	if _, err := svc.PromptTask(ctx, taskID, sessionID, "follow-up", "", false, nil, false); err != nil {
		t.Fatalf("follow-up prompt on accepted execution: %v", err)
	}
	if got := capturedPromptCount(baseAgent); got != 2 {
		t.Fatalf("provider prompt calls after follow-up = %d, want 2", got)
	}
	execution, err = repo.GetExecutorRunningBySessionID(ctx, sessionID)
	if err != nil {
		t.Fatalf("read accepted execution after follow-up: %v", err)
	}
	if execution.AgentExecutionID != executionID || !isHealthyPromptExecutionStatus(execution.Status) {
		t.Fatalf("accepted execution after follow-up = id:%s status:%s, want same healthy execution %s", execution.AgentExecutionID, execution.Status, executionID)
	}
}

func isHealthyPromptExecutionStatus(status string) bool {
	return status == models.ExecutorRunningStatusReady || status == models.ExecutorRunningStatusRunning
}

func TestPromptTask_PreparedDispatchRevalidatesOwnership(t *testing.T) {
	t.Run("cancellation", func(t *testing.T) {
		fixture := newPreparedPromptFixture(t, "cancel", false)
		var operation *cancelOperation
		fixture.agent.prepare = func(context.Context) error {
			var owner bool
			operation, owner = fixture.svc.claimCancellation(fixture.sessionID, cancellationKindExplicit)
			if !owner {
				return errors.New("claim cancellation owner")
			}
			return nil
		}
		_, err := fixture.runPrompt(false, "")
		if !errors.Is(err, ErrAgentPromptInProgress) {
			t.Fatalf("prepared dispatch error = %v, want cancellation rejection", err)
		}
		fixture.svc.finishCancellation(fixture.sessionID, operation, nil)
		fixture.assertNoProviderPrompt(t)
	})

	t.Run("task archived", func(t *testing.T) {
		fixture := newPreparedPromptFixture(t, "archive", false)
		fixture.agent.prepare = func(ctx context.Context) error {
			return fixture.repo.ArchiveTask(ctx, fixture.taskID)
		}
		_, err := fixture.runPrompt(false, "")
		if !errors.Is(err, executor.ErrTaskArchived) {
			t.Fatalf("prepared dispatch error = %v, want archived-task rejection", err)
		}
		fixture.assertNoProviderPrompt(t)
	})

	t.Run("queued reservation replaced", func(t *testing.T) {
		fixture := newPreparedPromptFixture(t, "replacement", false)
		var successor *queuedDispatchReservation
		var successorTurnID string
		fixture.agent.prepare = func(context.Context) error {
			guard := fixture.svc.lockCancelInFlightGuard(fixture.sessionID)
			defer guard.release()
			successor = fixture.svc.markQueuedDispatchInFlightWithIdentityLocked(
				fixture.identity, fixture.queued.ID+"-successor", nil,
			)
			activeTurn, err := fixture.turnService.GetActiveTurn(context.Background(), fixture.sessionID)
			if err != nil || activeTurn == nil {
				return errors.New("active predecessor turn is missing")
			}
			if err := fixture.turnService.CompleteTurn(context.Background(), activeTurn.ID); err != nil {
				return err
			}
			successorTurn, err := fixture.turnService.StartTurn(context.Background(), fixture.sessionID)
			if err != nil {
				return err
			}
			successorTurnID = successorTurn.ID
			return nil
		}
		_, err := fixture.runPrompt(false, "")
		if !errors.Is(err, errQueuedDispatchSuperseded) {
			t.Fatalf("prepared dispatch error = %v, want reservation superseded", err)
		}
		fixture.assertNoProviderPrompt(t)
		if current := fixture.svc.pendingQueuedDispatch(fixture.sessionID); current != successor {
			t.Fatalf("replacement reservation was not preserved: got=%p want=%p", current, successor)
		}
		activeTurn, err := fixture.turnService.GetActiveTurn(context.Background(), fixture.sessionID)
		if err != nil || activeTurn == nil || activeTurn.ID != successorTurnID {
			t.Fatalf("successor turn was changed by stale rollback: active=%+v want=%s err=%v", activeTurn, successorTurnID, err)
		}
	})

	t.Run("lifecycle route reselected", func(t *testing.T) {
		fixture := newPreparedPromptFixture(t, "route", true)
		var successorTurnID string
		fixture.agent.prepare = func(ctx context.Context) error {
			guard := fixture.svc.lockCancelInFlightGuard(fixture.sessionID)
			defer guard.release()
			activeTurn, err := fixture.turnService.GetActiveTurn(ctx, fixture.sessionID)
			if err != nil || activeTurn == nil {
				return errors.New("active lifecycle turn is missing")
			}
			if err := fixture.turnService.CompleteTurn(ctx, activeTurn.ID); err != nil {
				return err
			}
			successorTurn, err := fixture.turnService.StartTurn(ctx, fixture.sessionID)
			if err != nil {
				return err
			}
			successorTurnID = successorTurn.ID
			current, err := fixture.repo.GetTaskSession(ctx, fixture.sessionID)
			if err != nil {
				return err
			}
			current.State = models.TaskSessionStateCompleted
			return fixture.repo.UpdateTaskSession(ctx, current)
		}
		_, err := fixture.runPrompt(true, "")
		var reselected *lifecyclePromptReselectedError
		if !errors.As(err, &reselected) {
			t.Fatalf("prepared dispatch error = %v, want lifecycle route re-selection", err)
		}
		fixture.assertNoProviderPrompt(t)
		activeTurn, err := fixture.turnService.GetActiveTurn(context.Background(), fixture.sessionID)
		if err != nil || activeTurn == nil || activeTurn.ID != successorTurnID {
			t.Fatalf("successor turn was changed by stale lifecycle prompt: active=%+v want=%s err=%v", activeTurn, successorTurnID, err)
		}
	})

	t.Run("model switch execution replaced", func(t *testing.T) {
		fixture := newPreparedPromptFixture(t, "model-switch", false)
		fixture.baseAgent.setSessionModelSupported = true
		current, err := fixture.repo.GetTaskSession(context.Background(), fixture.sessionID)
		if err != nil {
			t.Fatal(err)
		}
		current.AgentProfileSnapshot = map[string]interface{}{"model": "old"}
		if err := fixture.repo.UpdateTaskSession(context.Background(), current); err != nil {
			t.Fatal(err)
		}
		fixture.agent.prepare = func(context.Context) error {
			seedExecutorRunning(t, fixture.repo, fixture.sessionID, fixture.taskID, "execution-model-switch-successor")
			return nil
		}
		_, err = fixture.runPrompt(false, "new")
		if !errors.Is(err, executor.ErrExecutionNotFound) {
			t.Fatalf("prepared dispatch error = %v, want replacement execution rejection", err)
		}
		fixture.assertNoProviderPrompt(t)
	})

	t.Run("resumed attempt cancelled", func(t *testing.T) {
		fixture := newPreparedPromptFixture(t, "resumed", false)
		current, err := fixture.repo.GetTaskSession(context.Background(), fixture.sessionID)
		if err != nil {
			t.Fatal(err)
		}
		current.State = models.TaskSessionStateRunning
		if err := fixture.repo.UpdateTaskSession(context.Background(), current); err != nil {
			t.Fatal(err)
		}
		attempt, owner := fixture.svc.resumeAttemptStore().begin(fixture.ctx, fixture.taskID, fixture.sessionID)
		if !owner {
			t.Fatal("create resume attempt")
		}
		guard := fixture.svc.lockCancelInFlightGuard(fixture.sessionID)
		rollback := promptClaimRollback{sessionIdentity: fixture.identity, dispatchGuard: guard}
		options := promptTaskOptions{claimEntryID: fixture.queued.ID, expectedSessionIdentity: &fixture.identity}
		beforeAdmission := fixture.svc.preparePromptAdmissionCallback(
			fixture.ctx, fixture.taskID, fixture.sessionID, current, rollback, options, attempt,
		)
		fixture.svc.resumeAttemptStore().invalidate(fixture.sessionID)
		if err := beforeAdmission(); !errors.Is(err, ErrResumeAttemptCancelled) {
			t.Fatalf("prepared dispatch error = %v, want cancelled resume attempt", err)
		}
		guard.release()
		fixture.assertNoProviderPrompt(t)
	})
}

type preparedPromptFixture struct {
	taskID, sessionID, executionID string
	repo                           *sqliterepo.Repository
	svc                            *Service
	baseAgent                      *mockAgentManager
	agent                          *queuedStreamBoundaryAgentManager
	turnService                    *repoTurnService
	identity                       messagequeue.QueueSessionIdentity
	queued                         *messagequeue.QueuedMessage
	ctx                            context.Context
}

func newPreparedPromptFixture(t *testing.T, suffix string, alternateSession bool) *preparedPromptFixture {
	t.Helper()
	ctx := context.Background()
	taskID, sessionID, executionID := "task-prepared-"+suffix, "session-prepared-"+suffix, "execution-prepared-"+suffix
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateWaitingForInput)
	current, err := repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	current.IsPrimary = true
	current.AgentExecutionID = executionID
	if err := repo.UpdateTaskSession(ctx, current); err != nil {
		t.Fatal(err)
	}
	seedExecutorRunning(t, repo, sessionID, taskID, executionID)
	if alternateSession {
		if err := repo.CreateTaskSession(ctx, &models.TaskSession{
			ID: sessionID + "-alternate", TaskID: taskID,
			State:     models.TaskSessionStateWaitingForInput,
			StartedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
		}); err != nil {
			t.Fatal(err)
		}
	}
	baseAgent := &mockAgentManager{isAgentRunning: true, repoForExecutionLookup: repo}
	agent := &queuedStreamBoundaryAgentManager{mockAgentManager: baseAgent}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agent)
	agent.service = svc
	svc.executor = executor.NewExecutor(agent, repo, testLogger(), executor.ExecutorConfig{})
	turnService := &repoTurnService{repo: repo}
	svc.turnService = turnService
	identity, err := svc.messageQueue.ResolveSessionIdentity(ctx, taskID, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.messageQueue.SetAutoRun(ctx, sessionID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.messageQueue.QueueMessageWithMetadata(
		ctx, sessionID, taskID, "queued prompt", "", messagequeue.QueuedByUser, false, nil, nil,
	); err != nil {
		t.Fatal(err)
	}
	queued, ok, _, err := svc.messageQueue.ReserveQueuedWithAutoRunForSession(ctx, identity)
	if err != nil || !ok || queued == nil {
		t.Fatalf("reserve queued prompt: queued=%+v ok=%v err=%v", queued, ok, err)
	}
	reservation := svc.markQueuedDispatchInFlightWithIdentityLocked(identity, queued.ID, queued)
	if reservation == nil {
		t.Fatal("mark queued dispatch in flight returned nil")
	}
	if claimed, err := svc.claimQueuedDispatchForExecution(sessionID, queued.ID, reservation); err != nil || !claimed {
		t.Fatalf("claim queued dispatch: claimed=%v err=%v", claimed, err)
	}
	promptCtx := context.WithValue(ctx, promptBoundaryTaskContextKey, taskID)
	promptCtx = context.WithValue(promptCtx, promptBoundarySessionContextKey, sessionID)
	return &preparedPromptFixture{
		taskID: taskID, sessionID: sessionID, executionID: executionID,
		repo: repo, svc: svc, baseAgent: baseAgent, agent: agent, turnService: turnService,
		identity: identity, queued: queued, ctx: promptCtx,
	}
}

func (fixture *preparedPromptFixture) runPrompt(lifecyclePrompt bool, model string) (*PromptResult, error) {
	return fixture.svc.promptTask(
		fixture.ctx, fixture.taskID, fixture.sessionID, "queued prompt", model, false, nil, true,
		launchOriginAutomatic,
		promptTaskOptions{
			claimEntryID: fixture.queued.ID, expectedSessionIdentity: &fixture.identity,
			lifecyclePrompt: lifecyclePrompt,
		},
	)
}

func (fixture *preparedPromptFixture) assertNoProviderPrompt(t *testing.T) {
	t.Helper()
	if count := capturedPromptCount(fixture.baseAgent); count != 0 {
		t.Fatalf("provider prompt calls = %d, want no stale dispatch", count)
	}
}

func capturedPromptCount(agent *mockAgentManager) int {
	agent.mu.Lock()
	defer agent.mu.Unlock()
	return len(agent.capturedPromptCalls)
}
