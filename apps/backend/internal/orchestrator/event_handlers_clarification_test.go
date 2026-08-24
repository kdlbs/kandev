package orchestrator

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/clarification"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type zeroClarificationCanceller struct {
	sessions []string
}

func (c *zeroClarificationCanceller) DetachSessionAndNotify(_ context.Context, sessionID string) (int, error) {
	c.sessions = append(c.sessions, sessionID)
	return 0, nil
}

func (c *zeroClarificationCanceller) ExpireSessionAndNotify(context.Context, string) (int, error) {
	return 0, nil
}

type failingDetachClarificationCanceller struct {
	err error
}

func (c *failingDetachClarificationCanceller) DetachSessionAndNotify(context.Context, string) (int, error) {
	return 0, c.err
}

func (c *failingDetachClarificationCanceller) ExpireSessionAndNotify(context.Context, string) (int, error) {
	return 0, nil
}

func TestClarificationInputPhaseContextStartsFreshAfterPriorCancellation(t *testing.T) {
	prior, cancelPrior := context.WithCancel(context.Background())
	cancelPrior()

	phase, cancelPhase := clarificationInputPhaseContext(prior)
	defer cancelPhase()
	if err := phase.Err(); err != nil {
		t.Fatalf("fresh clarification phase inherited prior cancellation: %v", err)
	}
	if _, ok := phase.Deadline(); !ok {
		t.Fatal("fresh clarification phase has no bounded deadline")
	}
}

type failingStartTurnService struct {
	TurnService
	err error
}

func (s *failingStartTurnService) StartTurn(context.Context, string) (*models.Turn, error) {
	return nil, s.err
}

func (s *failingStartTurnService) ReserveTurn(
	context.Context,
	string,
	*models.PromptDispatchRecovery,
) (*models.Turn, error) {
	return nil, s.err
}

type blockingClarificationCanceller struct {
	entered chan struct{}
	release chan struct{}
}

func (c *blockingClarificationCanceller) DetachSessionAndNotify(context.Context, string) (int, error) {
	close(c.entered)
	<-c.release
	return 1, nil
}

func (c *blockingClarificationCanceller) ExpireSessionAndNotify(context.Context, string) (int, error) {
	return 0, nil
}

func TestHandleClarificationAnswered(t *testing.T) {
	ctx := context.Background()

	t.Run("resumes agent with answered prompt", func(t *testing.T) {
		repo := setupTestRepo(t)
		agentMgr := &mockAgentManager{isAgentRunning: true}
		svc := createTestServiceWithScheduler(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
		svc.eventBus = &recordingEventBus{}

		seedTaskAndSession(t, repo, "t1", "s1", models.TaskSessionStateCompleted)

		event := bus.NewEvent("clarification.answered", "test", map[string]any{
			"session_id":  "s1",
			"task_id":     "t1",
			"question":    "Which database?",
			"answer_text": "User selected: PostgreSQL",
			"rejected":    false,
		})

		// PromptTask will fail (no running execution) but the handler should not return an error.
		err := svc.handleClarificationAnswered(ctx, event)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("returns nil on missing session_id", func(t *testing.T) {
		svc := &Service{logger: testLogger()}

		event := bus.NewEvent("clarification.answered", "test", map[string]any{
			"task_id":     "t1",
			"answer_text": "some answer",
		})

		err := svc.handleClarificationAnswered(ctx, event)
		if err != nil {
			t.Fatalf("expected nil error, got: %v", err)
		}
	})

	t.Run("returns nil on missing task_id", func(t *testing.T) {
		svc := &Service{logger: testLogger()}

		event := bus.NewEvent("clarification.answered", "test", map[string]any{
			"session_id":  "s1",
			"answer_text": "some answer",
		})

		err := svc.handleClarificationAnswered(ctx, event)
		if err != nil {
			t.Fatalf("expected nil error, got: %v", err)
		}
	})

	t.Run("returns nil on invalid event data", func(t *testing.T) {
		svc := &Service{logger: testLogger()}

		event := bus.NewEvent("clarification.answered", "test", "not-a-map")

		err := svc.handleClarificationAnswered(ctx, event)
		if err != nil {
			t.Fatalf("expected nil error, got: %v", err)
		}
	})
}

func TestResumeDetachedClarificationReportsPromptFailure(t *testing.T) {
	repo := setupTestRepo(t)
	agentMgr := &mockAgentManager{isAgentRunning: true}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.eventBus = &recordingEventBus{}
	seedTaskAndSession(t, repo, "t1", "s1", models.TaskSessionStateCompleted)

	err := svc.ResumeDetachedClarification(context.Background(), clarification.DetachedClarificationResume{
		TaskID:     "t1",
		SessionID:  "s1",
		PendingID:  "pending-1",
		Question:   "Which database?",
		AnswerText: "User selected: PostgreSQL",
	})
	if err == nil {
		t.Fatal("detached resume reported success after PromptTask failed")
	}
}

func TestResumeDetachedClarificationDispatchFailureLeavesBundleRestorable(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-retry", "session-retry", "step-1")
	seedExecutorRunning(t, repo, "session-retry", "task-retry", "exec-retry")
	if err := repo.UpdateTaskSessionState(
		ctx,
		"session-retry",
		models.TaskSessionStateWaitingForInput,
		"",
	); err != nil {
		t.Fatalf("set session waiting: %v", err)
	}

	startedAt := time.Now().UTC().Add(-time.Minute)
	completedAt := startedAt.Add(time.Second)
	if err := repo.CreateTurn(ctx, &models.Turn{
		ID:            "turn-clarification",
		TaskSessionID: "session-retry",
		TaskID:        "task-retry",
		StartedAt:     startedAt,
		CompletedAt:   &completedAt,
	}); err != nil {
		t.Fatalf("create clarification turn: %v", err)
	}
	if err := repo.CreateMessage(ctx, &models.Message{
		ID:            "message-clarification",
		TaskSessionID: "session-retry",
		TaskID:        "task-retry",
		TurnID:        "turn-clarification",
		AuthorType:    models.MessageAuthorAgent,
		Type:          models.MessageTypeClarificationRequest,
		Metadata: map[string]interface{}{
			"pending_id":  "pending-retry",
			"question_id": "q1",
			"status":      "pending",
		},
		CreatedAt: startedAt,
	}); err != nil {
		t.Fatalf("create clarification message: %v", err)
	}
	claimedMessages, claimed, err := repo.CompleteActiveClarificationBundle(
		ctx,
		"pending-retry",
		"answered",
		map[string]interface{}{"q1": map[string]interface{}{"question_id": "q1"}},
	)
	if err != nil || !claimed {
		t.Fatalf("claim clarification bundle: claimed=%v err=%v", claimed, err)
	}

	dispatchErr := errors.New("prompt dispatch failed")
	agentMgr := &mockAgentManager{
		isAgentRunning:         true,
		repoForExecutionLookup: repo,
		promptErr:              dispatchErr,
	}
	svc := createEngineService(t, repo, newMockStepGetter(), agentMgr)
	svc.turnService = &repoBackedTurnService{repo: repo}

	err = svc.ResumeDetachedClarification(ctx, clarification.DetachedClarificationResume{
		TaskID:     "task-retry",
		SessionID:  "session-retry",
		PendingID:  "pending-retry",
		Question:   "Continue?",
		AnswerText: "Continue",
	})
	if !errors.Is(err, dispatchErr) {
		t.Fatalf("resume error = %v, want %v", err, dispatchErr)
	}

	_, restored, err := repo.RestoreActiveClarificationBundle(
		ctx,
		"pending-retry",
		"answered",
		claimedMessages,
	)
	if err != nil || !restored {
		t.Fatalf("restore after failed dispatch: restored=%v err=%v", restored, err)
	}
	turns, err := repo.ListTurnsBySession(ctx, "session-retry")
	if err != nil {
		t.Fatalf("list turns: %v", err)
	}
	if len(turns) != 1 || turns[0].ID != "turn-clarification" {
		t.Fatalf("turns after failed dispatch = %#v, want only original clarification turn", turns)
	}

	_, claimed, err = repo.CompleteActiveClarificationBundle(
		ctx,
		"pending-retry",
		"answered",
		map[string]interface{}{"q1": map[string]interface{}{"question_id": "q1"}},
	)
	if err != nil || !claimed {
		t.Fatalf("reclaim clarification bundle: claimed=%v err=%v", claimed, err)
	}
	agentMgr.mu.Lock()
	agentMgr.promptErr = nil
	agentMgr.mu.Unlock()
	if err := svc.ResumeDetachedClarification(ctx, clarification.DetachedClarificationResume{
		TaskID:     "task-retry",
		SessionID:  "session-retry",
		PendingID:  "pending-retry",
		Question:   "Continue?",
		AnswerText: "Continue",
	}); err != nil {
		t.Fatalf("retry detached resume: %v", err)
	}
	turns, err = repo.ListTurnsBySession(ctx, "session-retry")
	if err != nil {
		t.Fatalf("list turns after retry: %v", err)
	}
	if len(turns) != 2 || turns[1].ID != "turn-session-retry" {
		t.Fatalf("turns after accepted retry = %#v, want one successor turn", turns)
	}
}

func TestResumeDetachedClarificationReportsAcceptedPublicationFailure(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-publish-failure", "session-publish-failure", "step-1")
	seedExecutorRunning(t, repo, "session-publish-failure", "task-publish-failure", "exec-publish-failure")
	if err := repo.UpdateTaskSessionState(
		ctx,
		"session-publish-failure",
		models.TaskSessionStateWaitingForInput,
		"",
	); err != nil {
		t.Fatalf("set session waiting: %v", err)
	}

	publicationErr := errors.New("publish accepted turn")
	agentMgr := &mockAgentManager{isAgentRunning: true, repoForExecutionLookup: repo}
	svc := createEngineService(t, repo, newMockStepGetter(), agentMgr)
	svc.turnService = failingReservedTurnPublisher{
		TurnService: &repoBackedTurnService{repo: repo},
		err:         publicationErr,
	}

	err := svc.ResumeDetachedClarification(ctx, clarification.DetachedClarificationResume{
		TaskID:     "task-publish-failure",
		SessionID:  "session-publish-failure",
		PendingID:  "pending-publish-failure",
		Question:   "Continue?",
		AnswerText: "Continue",
	})
	if err == nil {
		t.Fatal("resume reported success without durable dispatch publication")
	}
	var accepted interface{ DetachedResumeAccepted() bool }
	if !errors.As(err, &accepted) || !accepted.DetachedResumeAccepted() {
		t.Fatalf("resume error = %v, want accepted-dispatch marker", err)
	}
	if !errors.Is(err, publicationErr) {
		t.Fatalf("resume error = %v, want publication error %v", err, publicationErr)
	}
	if pending := svc.reservedPromptTurnID("session-publish-failure"); pending != "" {
		t.Fatalf("accepted turn remained rollback-eligible: %q", pending)
	}
	active, ok := svc.activeTurns.Load("session-publish-failure")
	if !ok || active == "" {
		t.Fatalf("accepted turn active ownership = %v, %v", active, ok)
	}
}

func TestResumeDetachedClarificationCleansUpAcceptedDispatchFailure(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-accepted-failure", "session-accepted-failure", "step-1")
	seedExecutorRunning(t, repo, "session-accepted-failure", "task-accepted-failure", "exec-accepted-failure")
	if err := repo.UpdateTaskSessionState(
		ctx,
		"session-accepted-failure",
		models.TaskSessionStateWaitingForInput,
		"",
	); err != nil {
		t.Fatalf("set session waiting: %v", err)
	}

	dispatchErr := errors.New("stream failed after prompt acceptance")
	publicationErr := errors.New("publish accepted turn")
	agentMgr := &mockAgentManager{
		isAgentRunning:         true,
		repoForExecutionLookup: repo,
		promptErr:              dispatchErr,
		promptAcceptedOnError:  true,
	}
	svc := createEngineService(t, repo, newMockStepGetter(), agentMgr)
	svc.turnService = failingReservedTurnPublisher{
		TurnService: &repoBackedTurnService{repo: repo},
		err:         publicationErr,
	}

	err := svc.ResumeDetachedClarification(ctx, clarification.DetachedClarificationResume{
		TaskID:     "task-accepted-failure",
		SessionID:  "session-accepted-failure",
		PendingID:  "pending-accepted-failure",
		Question:   "Continue?",
		AnswerText: "Continue",
	})
	if !errors.Is(err, dispatchErr) || !errors.Is(err, publicationErr) {
		t.Fatalf("resume error = %v, want dispatch %v and publication %v", err, dispatchErr, publicationErr)
	}
	var accepted interface{ DetachedResumeAccepted() bool }
	if !errors.As(err, &accepted) || !accepted.DetachedResumeAccepted() {
		t.Fatalf("resume error = %v, want accepted-dispatch marker", err)
	}
	session, getErr := repo.GetTaskSession(ctx, "session-accepted-failure")
	if getErr != nil {
		t.Fatalf("load session after failure: %v", getErr)
	}
	if session.State != models.TaskSessionStateWaitingForInput {
		t.Fatalf("session state after accepted failure = %q, want %q", session.State, models.TaskSessionStateWaitingForInput)
	}
}

func TestResumeDetachedClarificationRejectsBeforeDispatchWhenTurnPersistenceFails(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-turn-failure", "session-turn-failure", "step-1")
	seedExecutorRunning(t, repo, "session-turn-failure", "task-turn-failure", "exec-turn-failure")
	if err := repo.UpdateTaskSessionState(
		ctx,
		"session-turn-failure",
		models.TaskSessionStateWaitingForInput,
		"",
	); err != nil {
		t.Fatalf("set session waiting: %v", err)
	}

	persistenceErr := errors.New("persist successor turn")
	agentMgr := &mockAgentManager{isAgentRunning: true, repoForExecutionLookup: repo}
	svc := createEngineService(t, repo, newMockStepGetter(), agentMgr)
	svc.turnService = &failingStartTurnService{
		TurnService: &repoBackedTurnService{repo: repo},
		err:         persistenceErr,
	}

	err := svc.ResumeDetachedClarification(ctx, clarification.DetachedClarificationResume{
		TaskID:     "task-turn-failure",
		SessionID:  "session-turn-failure",
		PendingID:  "pending-turn-failure",
		Question:   "Continue?",
		AnswerText: "Continue",
	})
	if !errors.Is(err, persistenceErr) {
		t.Fatalf("resume error = %v, want %v", err, persistenceErr)
	}
	agentMgr.mu.Lock()
	promptCalls := len(agentMgr.capturedPromptCalls)
	agentMgr.mu.Unlock()
	if promptCalls != 0 {
		t.Fatalf("prompt dispatch calls = %d, want none before successor persistence", promptCalls)
	}
	session, err := repo.GetTaskSession(ctx, "session-turn-failure")
	if err != nil {
		t.Fatalf("load session after persistence failure: %v", err)
	}
	if session.State != models.TaskSessionStateWaitingForInput {
		t.Fatalf("session state after persistence failure = %q, want waiting", session.State)
	}
}

func TestResumeDetachedClarificationRejectsBeforeDispatchWhenAttemptMarkerFails(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-marker-failure", "session-marker-failure", "step-1")
	seedExecutorRunning(t, repo, "session-marker-failure", "task-marker-failure", "exec-marker-failure")
	if err := repo.UpdateTaskSessionState(
		ctx,
		"session-marker-failure",
		models.TaskSessionStateWaitingForInput,
		"",
	); err != nil {
		t.Fatalf("set session waiting: %v", err)
	}

	persistenceErr := errors.New("persist dispatch attempt marker")
	agentMgr := &mockAgentManager{isAgentRunning: true, repoForExecutionLookup: repo}
	svc := createEngineService(t, repo, newMockStepGetter(), agentMgr)
	svc.turnService = failingReservedTurnAttemptMarker{
		TurnService: &repoBackedTurnService{repo: repo},
		err:         persistenceErr,
	}

	err := svc.ResumeDetachedClarification(ctx, clarification.DetachedClarificationResume{
		TaskID:     "task-marker-failure",
		SessionID:  "session-marker-failure",
		PendingID:  "pending-marker-failure",
		Question:   "Continue?",
		AnswerText: "Continue",
	})
	if !errors.Is(err, persistenceErr) {
		t.Fatalf("resume error = %v, want %v", err, persistenceErr)
	}
	agentMgr.mu.Lock()
	promptCalls := len(agentMgr.capturedPromptCalls)
	agentMgr.mu.Unlock()
	if promptCalls != 0 {
		t.Fatalf("prompt dispatch calls = %d, want none before attempt marker persistence", promptCalls)
	}
	if pending := svc.reservedPromptTurnID("session-marker-failure"); pending != "" {
		t.Fatalf("private reservation = %q, want rollback after marker failure", pending)
	}
	turns, err := repo.ListTurnsBySession(ctx, "session-marker-failure")
	if err != nil {
		t.Fatalf("list turns after marker failure: %v", err)
	}
	if len(turns) != 0 {
		t.Fatalf("turns after marker failure = %#v, want rolled-back reservation", turns)
	}
	session, err := repo.GetTaskSession(ctx, "session-marker-failure")
	if err != nil {
		t.Fatalf("load session after marker failure: %v", err)
	}
	if session.State != models.TaskSessionStateWaitingForInput {
		t.Fatalf("session state after marker failure = %q, want waiting", session.State)
	}
}

func TestResumeDetachedClarificationUsesBoundedDispatchOnlyPrompt(t *testing.T) {
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-bounded", "session-bounded", "step-1")
	seedExecutorRunning(t, repo, "session-bounded", "task-bounded", "exec-bounded")
	if err := repo.UpdateTaskSessionState(
		context.Background(),
		"session-bounded",
		models.TaskSessionStateWaitingForInput,
		"",
	); err != nil {
		t.Fatalf("set session waiting: %v", err)
	}
	type promptObservation struct {
		dispatchOnly bool
		hasDeadline  bool
	}
	observed := make(chan promptObservation, 1)
	release := make(chan struct{})
	defer close(release)
	agentMgr := &mockAgentManager{isAgentRunning: true, repoForExecutionLookup: repo}
	agentMgr.promptAgentFunc = func(
		ctx context.Context,
		_ string,
		_ string,
		_ []v1.MessageAttachment,
		dispatchOnly bool,
	) (*executor.PromptResult, error) {
		_, hasDeadline := ctx.Deadline()
		observed <- promptObservation{dispatchOnly: dispatchOnly, hasDeadline: hasDeadline}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-release:
			return &executor.PromptResult{}, nil
		}
	}
	svc := createEngineService(t, repo, newMockStepGetter(), agentMgr)
	resumeCtx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	done := make(chan error, 1)
	go func() {
		done <- svc.ResumeDetachedClarification(resumeCtx, clarification.DetachedClarificationResume{
			TaskID:     "task-bounded",
			SessionID:  "session-bounded",
			PendingID:  "pending-bounded",
			Question:   "Continue?",
			AnswerText: "Continue",
		})
	}()

	var observation promptObservation
	select {
	case observation = <-observed:
	case err := <-done:
		t.Fatalf("resume returned before prompt dispatch: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for prompt dispatch")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("resume error = %v, want context cancellation", err)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("detached resume ignored cancellation before prompt acknowledgment")
	}
	if !observation.dispatchOnly || !observation.hasDeadline {
		t.Fatalf("prompt observation = %+v, want dispatch-only with deadline", observation)
	}
}

func TestResumeDetachedClarificationDoesNotTreatAsyncRecoveryAsAcknowledgment(t *testing.T) {
	repo := setupTestRepo(t)
	promptEntered := make(chan struct{}, 1)
	promptRelease := make(chan struct{})
	t.Cleanup(func() { close(promptRelease) })
	agentMgr := &mockAgentManager{isAgentRunning: true, repoForExecutionLookup: repo}
	agentMgr.promptAgentFunc = func(
		context.Context,
		string,
		string,
		[]v1.MessageAttachment,
		bool,
	) (*executor.PromptResult, error) {
		promptEntered <- struct{}{}
		<-promptRelease
		return &executor.PromptResult{}, nil
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
	seedTaskAndSession(t, repo, "task-busy", "session-busy", models.TaskSessionStateRunning)
	seedExecutorRunning(t, repo, "session-busy", "task-busy", "exec-busy")

	err := svc.ResumeDetachedClarification(context.Background(), clarification.DetachedClarificationResume{
		TaskID:     "task-busy",
		SessionID:  "session-busy",
		PendingID:  "pending-busy",
		Question:   "Continue?",
		AnswerText: "Continue",
	})
	if err == nil {
		t.Fatal("detached resume reported acknowledgment after only an asynchronous retry handoff")
	}
	select {
	case <-promptEntered:
		t.Fatal("bounded detached resume must not hand an unacknowledged retry to the async queue")
	default:
	}
}

func TestHandleClarificationStaleDismissed(t *testing.T) {
	ctx := context.Background()

	t.Run("returns nil on missing session_id", func(t *testing.T) {
		svc := &Service{logger: testLogger()}
		event := bus.NewEvent("clarification.stale_dismissed", "test", map[string]any{
			"task_id": "t1",
		})
		if err := svc.handleClarificationStaleDismissed(ctx, event); err != nil {
			t.Fatalf("expected nil error, got: %v", err)
		}
	})

	t.Run("skips on_turn_complete while clarification still pending", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		stepGetter := newMockStepGetter()
		stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
			ID: "step1", WorkflowID: "wf1", Name: "Plan", Position: 0,
			Events: wfmodels.StepEvents{
				OnTurnComplete: []wfmodels.OnTurnCompleteAction{
					{Type: wfmodels.OnTurnCompleteMoveToNext},
				},
			},
		}
		stepGetter.steps["step2"] = &wfmodels.WorkflowStep{
			ID: "step2", WorkflowID: "wf1", Name: "Implement", Position: 1,
		}
		svc := createEngineService(t, repo, stepGetter, &mockAgentManager{})

		session, err := repo.GetTaskSession(ctx, "s1")
		if err != nil {
			t.Fatalf("get session: %v", err)
		}
		session.State = models.TaskSessionStateWaitingForInput
		if err := repo.UpdateTaskSession(ctx, session); err != nil {
			t.Fatalf("set session waiting: %v", err)
		}

		now := time.Now().UTC()
		requireNoError(t, repo.CreateTurn(ctx, &models.Turn{ID: "turn-1", TaskSessionID: "s1", TaskID: "t1", StartedAt: now}))
		requireNoError(t, repo.CreateMessage(ctx, &models.Message{
			ID: "clarify-1", TaskSessionID: "s1", TaskID: "t1", TurnID: "turn-1",
			AuthorType: models.MessageAuthorAgent, Type: "clarification_request", Content: "Q?",
			CreatedAt: now, Metadata: map[string]interface{}{"pending_id": "pending-1", "status": "pending"},
		}))

		event := bus.NewEvent("clarification.stale_dismissed", "test", map[string]any{
			"session_id": "s1",
			"task_id":    "t1",
			"pending_id": "pending-1",
		})
		if err := svc.handleClarificationStaleDismissed(ctx, event); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		task, err := repo.GetTask(ctx, "t1")
		if err != nil {
			t.Fatalf("get task: %v", err)
		}
		if task.WorkflowStepID != "step1" {
			t.Fatalf("expected step to remain step1 while clarification pending, got %q", task.WorkflowStepID)
		}
	})

	t.Run("skips cleanup for terminal session state", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		stepGetter := newMockStepGetter()
		stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
			ID: "step1", WorkflowID: "wf1", Name: "Plan", Position: 0,
			Events: wfmodels.StepEvents{
				OnTurnComplete: []wfmodels.OnTurnCompleteAction{
					{Type: wfmodels.OnTurnCompleteMoveToNext},
				},
			},
		}
		stepGetter.steps["step2"] = &wfmodels.WorkflowStep{
			ID: "step2", WorkflowID: "wf1", Name: "Implement", Position: 1,
		}
		svc := createEngineService(t, repo, stepGetter, &mockAgentManager{})

		session, err := repo.GetTaskSession(ctx, "s1")
		if err != nil {
			t.Fatalf("get session: %v", err)
		}
		session.State = models.TaskSessionStateCancelled
		if err := repo.UpdateTaskSession(ctx, session); err != nil {
			t.Fatalf("set session cancelled: %v", err)
		}

		event := bus.NewEvent("clarification.stale_dismissed", "test", map[string]any{
			"session_id": "s1",
			"task_id":    "t1",
			"pending_id": "pending-1",
		})
		if err := svc.handleClarificationStaleDismissed(ctx, event); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		task, err := repo.GetTask(ctx, "t1")
		if err != nil {
			t.Fatalf("get task: %v", err)
		}
		if task.WorkflowStepID != "step1" {
			t.Fatalf("expected step to remain step1 for terminal session, got %q", task.WorkflowStepID)
		}
	})

	t.Run("advances workflow when no clarification is pending after dismiss", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		stepGetter := newMockStepGetter()
		stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
			ID: "step1", WorkflowID: "wf1", Name: "Plan", Position: 0,
			Events: wfmodels.StepEvents{
				OnTurnComplete: []wfmodels.OnTurnCompleteAction{
					{Type: wfmodels.OnTurnCompleteMoveToNext},
				},
			},
		}
		stepGetter.steps["step2"] = &wfmodels.WorkflowStep{
			ID: "step2", WorkflowID: "wf1", Name: "Implement", Position: 1,
		}
		svc := createEngineService(t, repo, stepGetter, &mockAgentManager{})

		session, err := repo.GetTaskSession(ctx, "s1")
		if err != nil {
			t.Fatalf("get session: %v", err)
		}
		session.State = models.TaskSessionStateWaitingForInput
		if err := repo.UpdateTaskSession(ctx, session); err != nil {
			t.Fatalf("set session waiting: %v", err)
		}

		event := bus.NewEvent("clarification.stale_dismissed", "test", map[string]any{
			"session_id": "s1",
			"task_id":    "t1",
			"pending_id": "pending-1",
		})
		if err := svc.handleClarificationStaleDismissed(ctx, event); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		task, err := repo.GetTask(ctx, "t1")
		if err != nil {
			t.Fatalf("get task: %v", err)
		}
		if task.WorkflowStepID != "step2" {
			t.Fatalf("expected workflow step step2 after deferred on_turn_complete, got %q", task.WorkflowStepID)
		}
	})

	t.Run("moves task to REVIEW when no workflow transition fires", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		stepGetter := newMockStepGetter()
		stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
			ID: "step1", WorkflowID: "wf1", Name: "Plan", Position: 0,
		}
		taskRepo := newMockTaskRepo()
		seedMockTaskState(taskRepo, "t1", v1.TaskStateInProgress)
		svc := createEngineService(t, repo, stepGetter, &mockAgentManager{})
		svc.taskRepo = taskRepo

		session, err := repo.GetTaskSession(ctx, "s1")
		if err != nil {
			t.Fatalf("get session: %v", err)
		}
		session.State = models.TaskSessionStateWaitingForInput
		if err := repo.UpdateTaskSession(ctx, session); err != nil {
			t.Fatalf("set session waiting: %v", err)
		}

		event := bus.NewEvent("clarification.stale_dismissed", "test", map[string]any{
			"session_id": "s1",
			"task_id":    "t1",
			"pending_id": "pending-1",
		})
		if err := svc.handleClarificationStaleDismissed(ctx, event); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if state, ok := taskRepo.updatedStates["t1"]; !ok || state != v1.TaskStateReview {
			t.Fatalf("expected task state %q, got %q (ok=%v)", v1.TaskStateReview, state, ok)
		}
	})

	t.Run("coordinator cancellation wins while stale-dismiss event waits", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")
		requireNoError(t, repo.UpdateTaskSessionState(
			ctx,
			"s1",
			models.TaskSessionStateWaitingForInput,
			"",
		))
		stepGetter := newMockStepGetter()
		stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
			ID: "step1", WorkflowID: "wf1", Name: "Plan", Position: 0,
			Events: wfmodels.StepEvents{OnTurnComplete: []wfmodels.OnTurnCompleteAction{{
				Type: wfmodels.OnTurnCompleteMoveToNext,
			}}},
		}
		stepGetter.steps["step2"] = &wfmodels.WorkflowStep{
			ID: "step2", WorkflowID: "wf1", Name: "Implement", Position: 1,
		}
		svc := createEngineService(t, repo, stepGetter, &mockAgentManager{})
		event := bus.NewEvent("clarification.stale_dismissed", "test", map[string]any{
			"session_id": "s1",
			"task_id":    "t1",
			"pending_id": "pending-1",
		})

		guard, release := svc.acquireCancelInFlightGuard("s1")
		guard.Lock()
		done := make(chan error, 1)
		go func() { done <- svc.handleClarificationStaleDismissed(ctx, event) }()
		coordinatorStopWaitForGuardRefs(t, svc, "s1", 2)
		changed, _, err := repo.CancelActiveTaskSession(ctx, "s1", coordinatorMCPStopReason)
		requireNoError(t, err)
		if !changed {
			t.Fatal("coordinator cancellation did not change the waiting session")
		}
		guard.Unlock()
		release()
		select {
		case handlerErr := <-done:
			requireNoError(t, handlerErr)
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for stale-dismiss handler")
		}

		session, err := repo.GetTaskSession(ctx, "s1")
		requireNoError(t, err)
		if session.State != models.TaskSessionStateCancelled {
			t.Fatalf("expected cancelled session, got %q", session.State)
		}
		task, err := repo.GetTask(ctx, "t1")
		requireNoError(t, err)
		if task.WorkflowStepID != "step1" {
			t.Fatalf("stale-dismiss advanced workflow after cancellation: %q", task.WorkflowStepID)
		}
	})
}

func TestPauseForClarificationInput_SilentlyCancelsTurnWithoutWorkflowTransition(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	seedExecutorRunning(t, repo, "s1", "t1", "exec-1")
	seedPendingClarificationMessage(t, repo, "t1", "s1")

	stepGetter := newMockStepGetter()
	stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
		ID: "step1", WorkflowID: "wf1", Name: "Plan", Position: 0,
		Events: wfmodels.StepEvents{
			OnTurnComplete: []wfmodels.OnTurnCompleteAction{
				{Type: wfmodels.OnTurnCompleteMoveToNext},
			},
		},
	}
	stepGetter.steps["step2"] = &wfmodels.WorkflowStep{
		ID: "step2", WorkflowID: "wf1", Name: "Implement", Position: 1,
	}

	agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
	canceller := &recordingClarificationCanceller{}
	svc := createEngineService(t, repo, stepGetter, agentMgr)
	svc.SetClarificationCanceller(canceller)
	svc.turnService = &repoBackedTurnService{repo: repo}

	detached, err := svc.PauseForClarificationInput(ctx, "s1")
	if err != nil {
		t.Fatalf("pause clarification input: %v", err)
	}
	if detached != 1 {
		t.Fatalf("expected one detached clarification bundle, got %d", detached)
	}

	if got := agentMgr.cancelAgentCalls.Load(); got != 1 {
		t.Fatalf("expected silent cancel call, got %d", got)
	}
	if len(canceller.sessions) == 0 || canceller.sessions[0] != "s1" {
		t.Fatalf("expected clarification detach for s1, got %#v", canceller.sessions)
	}
	task, err := repo.GetTask(ctx, "t1")
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if task.WorkflowStepID != "step1" {
		t.Fatalf("timeout pause must not run on_turn_complete; got step %q", task.WorkflowStepID)
	}
	session, err := repo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if session.State != models.TaskSessionStateWaitingForInput {
		t.Fatalf("expected session waiting for input, got %q", session.State)
	}
	if turn, err := repo.GetActiveTurnBySessionID(ctx, "s1"); err != nil && !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("get active turn: %v", err)
	} else if turn != nil {
		t.Fatalf("expected active turn to be completed, got %#v", turn)
	}
}

func TestPauseForClarificationInput_ContinuesHardPauseAfterDetachFailure(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	seedExecutorRunning(t, repo, "s1", "t1", "exec-1")
	seedPendingClarificationMessage(t, repo, "t1", "s1")

	wantErr := errors.New("detach failed")
	agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
	svc := createEngineService(t, repo, newMockStepGetter(), agentMgr)
	svc.SetClarificationCanceller(&failingDetachClarificationCanceller{err: wantErr})
	svc.turnService = &repoBackedTurnService{repo: repo}
	if _, err := svc.messageQueue.QueueMessageWithMetadata(
		ctx, "s1", "t1", "peer-during-detach-failure", "user-1", "user-1", false, nil, map[string]interface{}{},
	); err != nil {
		t.Fatalf("queue peer: %v", err)
	}

	detached, err := svc.PauseForClarificationInput(ctx, "s1")
	if detached != 0 || !errors.Is(err, wantErr) {
		t.Fatalf("PauseForClarificationInput = %d, %v; want 0 and detach error", detached, err)
	}
	if got := agentMgr.cancelAgentCalls.Load(); got != 1 {
		t.Fatalf("detach failure must still hard-pause the active turn, got %d cancel calls", got)
	}
	session, err := repo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if session.State != models.TaskSessionStateWaitingForInput {
		t.Fatalf("expected session waiting for input after detach failure, got %q", session.State)
	}
	if got := svc.messageQueue.GetStatus(ctx, "s1").Count; got != 1 {
		t.Fatalf("detach failure drained queued peer message: count=%d, want 1", got)
	}
}

func TestPauseForClarificationInput_RetriesHardPauseAfterSuccessfulDetach(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	seedExecutorRunning(t, repo, "s1", "t1", "exec-1")
	seedPendingClarificationMessage(t, repo, "t1", "s1")
	requireNoError(t, repo.UpdateTaskSessionState(
		ctx,
		"s1",
		models.TaskSessionStateWaitingForInput,
		"",
	))

	wantErr := errors.New("cancel temporarily unavailable")
	agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
	agentMgr.cancelAgentFunc = func(context.Context, string) error {
		if agentMgr.cancelAgentCalls.Load() == 1 {
			return wantErr
		}
		return nil
	}
	store := clarification.NewStore(time.Minute)
	store.CreateRequest(&clarification.Request{
		PendingID: "pending-s1",
		SessionID: "s1",
	})
	svc := createEngineService(t, repo, newMockStepGetter(), agentMgr)
	svc.SetClarificationCanceller(clarification.NewCanceller(store, repo, nil, testLogger()))
	svc.turnService = &repoBackedTurnService{repo: repo}

	detached, err := svc.PauseForClarificationInput(ctx, "s1")
	if detached != 1 || !errors.Is(err, wantErr) {
		t.Fatalf("first PauseForClarificationInput = %d, %v; want 1 and cancel error", detached, err)
	}
	pending, err := repo.FindActiveClarificationMessagesBySessionID(ctx, "s1")
	if err != nil || len(pending) != 1 || pending[0].Metadata["agent_disconnected"] != true {
		t.Fatalf("detached clarification authority = %#v, %v; want one pending detached row", pending, err)
	}
	detached, err = svc.PauseForClarificationInput(ctx, "s1")
	if err != nil {
		t.Fatalf("retry PauseForClarificationInput: %v", err)
	}
	if detached != 0 {
		t.Fatalf("retry detached bundles = %d, want 0 for already-detached row", detached)
	}
	if got := agentMgr.cancelAgentCalls.Load(); got != 2 {
		t.Fatalf("hard-pause cancel calls = %d, want retry after successful detach", got)
	}
	if turn, turnErr := repo.GetActiveTurnBySessionID(ctx, "s1"); turnErr != nil && !errors.Is(turnErr, sql.ErrNoRows) {
		t.Fatalf("get active turn: %v", turnErr)
	} else if turn != nil {
		t.Fatalf("retry left clarification turn active: %#v", turn)
	}
}

func TestPauseForClarificationInput_CancelsWhileSessionAlreadyWaiting(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	seedExecutorRunning(t, repo, "s1", "t1", "exec-1")
	seedPendingClarificationMessage(t, repo, "t1", "s1")
	if err := repo.UpdateTaskSessionState(ctx, "s1", models.TaskSessionStateWaitingForInput, ""); err != nil {
		t.Fatalf("set waiting state: %v", err)
	}

	agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
	canceller := &zeroClarificationCanceller{}
	svc := createEngineService(t, repo, newMockStepGetter(), agentMgr)
	svc.SetClarificationCanceller(canceller)
	svc.turnService = &repoBackedTurnService{repo: repo}

	detached, err := svc.PauseForClarificationInput(ctx, "s1")
	if err != nil {
		t.Fatalf("pause clarification input: %v", err)
	}
	if detached != 0 {
		t.Fatalf("expected zero detached clarification bundles from zero canceller, got %d", detached)
	}
	if len(canceller.sessions) != 1 || canceller.sessions[0] != "s1" {
		t.Fatalf("expected clarification detach for s1, got %#v", canceller.sessions)
	}
	if got := agentMgr.cancelAgentCalls.Load(); got != 1 {
		t.Fatalf("waiting ask session must still cancel active agent, got %d calls", got)
	}
}

func TestPauseForClarificationInput_DoesNotCancelSuccessorCreatedDuringDetach(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	seedExecutorRunning(t, repo, "s1", "t1", "exec-1")
	seedPendingClarificationMessage(t, repo, "t1", "s1")

	agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
	canceller := &blockingClarificationCanceller{
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}
	var releaseOnce sync.Once
	releaseDetach := func() { releaseOnce.Do(func() { close(canceller.release) }) }
	t.Cleanup(releaseDetach)
	svc := createEngineService(t, repo, newMockStepGetter(), agentMgr)
	svc.SetClarificationCanceller(canceller)
	svc.turnService = &repoBackedTurnService{repo: repo}

	done := make(chan error, 1)
	go func() {
		_, err := svc.PauseForClarificationInput(ctx, "s1")
		done <- err
	}()
	select {
	case <-canceller.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for clarification detach")
	}
	if _, err := svc.turnService.StartTurn(ctx, "s1"); err != nil {
		t.Fatalf("start successor turn: %v", err)
	}
	releaseDetach()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("pause after successor turn: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for clarification pause")
	}
	if got := agentMgr.cancelAgentCalls.Load(); got != 0 {
		t.Fatalf("stale clarification pause cancelled successor: %d calls", got)
	}
}

func TestPauseForClarificationInput_DoesNotCancelFirstTurnCreatedDuringDetach(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	seedExecutorRunning(t, repo, "s1", "t1", "exec-1")

	agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
	canceller := &blockingClarificationCanceller{
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}
	var releaseOnce sync.Once
	releaseDetach := func() { releaseOnce.Do(func() { close(canceller.release) }) }
	t.Cleanup(releaseDetach)
	svc := createEngineService(t, repo, newMockStepGetter(), agentMgr)
	svc.SetClarificationCanceller(canceller)
	svc.turnService = &repoBackedTurnService{repo: repo}

	done := make(chan error, 1)
	go func() {
		_, err := svc.PauseForClarificationInput(ctx, "s1")
		done <- err
	}()
	select {
	case <-canceller.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for clarification detach")
	}
	if _, err := svc.turnService.StartTurn(ctx, "s1"); err != nil {
		t.Fatalf("start first turn during detach: %v", err)
	}
	releaseDetach()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("pause after first turn: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for clarification pause")
	}
	if got := agentMgr.cancelAgentCalls.Load(); got != 0 {
		t.Fatalf("stale clarification pause cancelled first turn: %d calls", got)
	}
}

func TestPauseForClarificationInput_IgnoresStaleTimeoutWithoutPendingClarification(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	seedExecutorRunning(t, repo, "s1", "t1", "exec-1")
	if err := repo.SetSessionMetadataKey(ctx, "s1", models.SessionMetaKeyPendingStepCompletion, models.PendingStepCompletionSignal{
		StepID:     "step1",
		Source:     "agent",
		Summary:    "ready",
		SignaledAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("seed pending step signal: %v", err)
	}

	agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
	canceller := &zeroClarificationCanceller{}
	svc := createEngineService(t, repo, newMockStepGetter(), agentMgr)
	svc.SetClarificationCanceller(canceller)
	svc.turnService = &repoBackedTurnService{repo: repo}

	detached, err := svc.PauseForClarificationInput(ctx, "s1")
	if err != nil {
		t.Fatalf("pause stale clarification input: %v", err)
	}
	if detached != 0 {
		t.Fatalf("expected no detached clarifications, got %d", detached)
	}
	if len(canceller.sessions) != 1 || canceller.sessions[0] != "s1" {
		t.Fatalf("expected stale timeout to probe clarification detach for s1, got %#v", canceller.sessions)
	}
	if got := agentMgr.cancelAgentCalls.Load(); got != 0 {
		t.Fatalf("stale timeout must not cancel a later turn, got %d calls", got)
	}
	session, err := repo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if _, has := models.LoadPendingStepSignal(session.Metadata); !has {
		t.Fatal("stale timeout must not clear pending step signal from later turn")
	}
}

func TestHandleClarificationAnswered_SkipsOnTurnStart(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	seedExecutorRunning(t, repo, "s1", "t1", "exec-1")
	if err := repo.UpdateTaskSessionState(ctx, "s1", models.TaskSessionStateWaitingForInput, ""); err != nil {
		t.Fatalf("set session waiting: %v", err)
	}

	stepGetter := newMockStepGetter()
	stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
		ID: "step1", WorkflowID: "wf1", Name: "Plan", Position: 0,
		Events: wfmodels.StepEvents{
			OnTurnStart: []wfmodels.OnTurnStartAction{
				{Type: wfmodels.OnTurnStartMoveToNext},
			},
		},
	}
	stepGetter.steps["step2"] = &wfmodels.WorkflowStep{
		ID: "step2", WorkflowID: "wf1", Name: "Implement", Position: 1,
	}

	agentMgr := &mockAgentManager{isAgentRunning: true, repoForExecutionLookup: repo}
	svc := createEngineService(t, repo, stepGetter, agentMgr)
	event := bus.NewEvent("clarification.answered", "test", map[string]any{
		"session_id":  "s1",
		"task_id":     "t1",
		"pending_id":  "pending-1",
		"question":    "Which database?",
		"answer_text": "User selected: PostgreSQL",
		"rejected":    false,
	})

	if err := svc.handleClarificationAnswered(ctx, event); err != nil {
		t.Fatalf("handle clarification answered: %v", err)
	}

	task, err := repo.GetTask(ctx, "t1")
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if task.WorkflowStepID != "step1" {
		t.Fatalf("clarification continuation must not run on_turn_start; got step %q", task.WorkflowStepID)
	}
	if len(agentMgr.capturedPrompts) != 1 {
		t.Fatalf("expected one clarification answer prompt, got %d", len(agentMgr.capturedPrompts))
	}
	if !strings.Contains(agentMgr.capturedPrompts[0], "User selected: PostgreSQL") {
		t.Fatalf("clarification answer prompt missing answer: %q", agentMgr.capturedPrompts[0])
	}
}

func TestBuildClarificationPrompt(t *testing.T) {
	t.Run("builds accepted prompt with question and answer", func(t *testing.T) {
		data := clarificationAnsweredData{
			Question:   "Which database?",
			AnswerText: "User selected: PostgreSQL",
			Rejected:   false,
		}

		prompt := buildClarificationPrompt(data)

		if !strings.Contains(prompt, "Which database?") {
			t.Error("prompt should contain the question")
		}
		if !strings.Contains(prompt, "PostgreSQL") {
			t.Error("prompt should contain the answer")
		}
		if !strings.Contains(prompt, "continue with this information") {
			t.Error("prompt should instruct agent to continue")
		}
	})

	t.Run("builds rejected prompt with reason", func(t *testing.T) {
		data := clarificationAnsweredData{
			Question:     "Which database?",
			Rejected:     true,
			RejectReason: "Not relevant",
		}

		prompt := buildClarificationPrompt(data)

		if !strings.Contains(prompt, "declined") {
			t.Error("prompt should mention declined")
		}
		if !strings.Contains(prompt, "Not relevant") {
			t.Error("prompt should contain the reason")
		}
	})

	t.Run("builds rejected prompt without reason", func(t *testing.T) {
		data := clarificationAnsweredData{
			Question: "Which database?",
			Rejected: true,
		}

		prompt := buildClarificationPrompt(data)

		if !strings.Contains(prompt, "No reason provided") {
			t.Error("prompt should contain fallback reason")
		}
	})
}

func TestHandleClarificationPrimaryAnswered_SchedulesWatchdog(t *testing.T) {
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "t1", "s1", models.TaskSessionStateRunning)
	taskRepo := newMockTaskRepo()
	svc := createTestServiceWithScheduler(
		repo,
		newMockStepGetter(),
		taskRepo,
		&mockAgentManager{isAgentRunning: true},
	)
	svc.clarificationWatchdogTimeout = 500 * time.Millisecond
	t.Cleanup(func() { svc.cancelAllClarificationWatchdogs() })
	if err := repo.UpdateTaskState(context.Background(), "t1", v1.TaskStateReview); err != nil {
		t.Fatalf("set task review state: %v", err)
	}

	event := bus.NewEvent("clarification.primary_answered", "test", map[string]any{
		"session_id":  "s1",
		"task_id":     "t1",
		"pending_id":  "p1",
		"question":    "Which approach?",
		"answer_text": "User selected: Option A",
	})

	if err := svc.handleClarificationPrimaryAnswered(context.Background(), event); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := countClarificationWatchdogs(svc); got != 1 {
		t.Fatalf("expected 1 active watchdog, got %d", got)
	}
	if got := taskRepo.updatedStates["t1"]; got != v1.TaskStateInProgress {
		t.Fatalf("task state = %q, want %q", got, v1.TaskStateInProgress)
	}
}

func TestHandleAgentStreamEvent_CancelsClarificationWatchdogs(t *testing.T) {
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "t1", "s1", models.TaskSessionStateRunning)
	svc := createTestServiceWithScheduler(
		repo,
		newMockStepGetter(),
		newMockTaskRepo(),
		&mockAgentManager{isAgentRunning: true},
	)
	svc.clarificationWatchdogTimeout = time.Second
	t.Cleanup(func() { svc.cancelAllClarificationWatchdogs() })

	event := bus.NewEvent("clarification.primary_answered", "test", map[string]any{
		"session_id":  "s1",
		"task_id":     "t1",
		"pending_id":  "p1",
		"question":    "Which approach?",
		"answer_text": "User selected: Option A",
	})
	if err := svc.handleClarificationPrimaryAnswered(context.Background(), event); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	svc.handleAgentStreamEvent(context.Background(), &lifecycle.AgentStreamEventPayload{
		TaskID:    "t1",
		SessionID: "s1",
		Data: &lifecycle.AgentStreamEventData{
			Type: "session_mode",
		},
	})

	if got := countClarificationWatchdogs(svc); got != 0 {
		t.Fatalf("expected watchdogs to be cancelled, got %d", got)
	}
}

func TestClarificationWatchdog_MatchingTurnDispatchesAndClearsEntry(t *testing.T) {
	repo := setupTestRepo(t)
	promptDone := make(chan struct{})
	agentMgr := &mockAgentManager{
		isAgentRunning:         true,
		repoForExecutionLookup: repo,
		promptDone:             promptDone,
	}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.clarificationWatchdogTimeout = 20 * time.Millisecond
	t.Cleanup(func() { svc.cancelAllClarificationWatchdogs() })

	seedTaskAndSession(t, repo, "t1", "s1", models.TaskSessionStateWaitingForInput)
	seedExecutorRunning(t, repo, "s1", "t1", "exec-watchdog-positive")
	turnService := &repoTurnService{repo: repo}
	svc.turnService = turnService
	clarificationTurn, err := turnService.StartTurn(context.Background(), "s1")
	if err != nil {
		t.Fatalf("start clarification turn: %v", err)
	}

	event := bus.NewEvent("clarification.primary_answered", "test", map[string]any{
		"session_id":            "s1",
		"task_id":               "t1",
		"pending_id":            "p1",
		"clarification_turn_id": clarificationTurn.ID,
		"question":              "Which approach?",
		"answer_text":           "User selected: Option A",
	})
	if err := svc.handleClarificationPrimaryAnswered(context.Background(), event); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	select {
	case <-promptDone:
	case <-time.After(time.Second):
		t.Fatal("matching clarification watchdog did not dispatch a prompt")
	}

	deadline := time.Now().Add(time.Second)
	for countClarificationWatchdogs(svc) != 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if got := countClarificationWatchdogs(svc); got != 0 {
		t.Fatalf("expected watchdog map to be empty after timeout, got %d", got)
	}
}

func TestClarificationWatchdogExpirationPreservesSuccessorEntry(t *testing.T) {
	svc := &Service{}
	key := svc.clarificationWatchdogKey("session-1", "pending-1")
	expired := &clarificationWatchdogEntry{}
	successor := &clarificationWatchdogEntry{}
	svc.clarificationWatchdogs.Store(key, successor)
	t.Cleanup(func() { svc.clarificationWatchdogs.Delete(key) })

	svc.runClarificationWatchdog(
		context.Background(), key, expired, clarificationAnsweredData{}, 0,
	)

	current, ok := svc.clarificationWatchdogs.Load(key)
	if !ok || current != successor {
		t.Fatalf("successor watchdog = %v, %v; want preserved successor %p", current, ok, successor)
	}
}

type blockingClarificationTurnLookup struct {
	*repoTurnService
	entered chan context.Context
	release chan struct{}
}

func (s *blockingClarificationTurnLookup) GetActiveTurn(
	ctx context.Context,
	_ string,
) (*models.Turn, error) {
	s.entered <- ctx
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-s.release:
		return nil, nil
	}
}

func TestClarificationWatchdogCancellationInterruptsFallbackLookup(t *testing.T) {
	repo := setupTestRepo(t)
	svc := createTestServiceWithScheduler(
		repo,
		newMockStepGetter(),
		newMockTaskRepo(),
		&mockAgentManager{isAgentRunning: true},
	)
	lookup := &blockingClarificationTurnLookup{
		repoTurnService: &repoTurnService{repo: repo},
		entered:         make(chan context.Context, 1),
		release:         make(chan struct{}),
	}
	svc.turnService = lookup
	svc.clarificationWatchdogTimeout = time.Millisecond
	defer close(lookup.release)

	svc.scheduleClarificationWatchdog(clarificationAnsweredData{
		TaskID: "task-watchdog-shutdown", SessionID: "session-watchdog-shutdown",
		PendingID: "pending-watchdog-shutdown", ClarificationTurnID: "turn-watchdog-shutdown",
	})

	var lookupCtx context.Context
	select {
	case lookupCtx = <-lookup.entered:
	case <-time.After(time.Second):
		t.Fatal("watchdog fallback did not enter turn lookup")
	}
	svc.cancelAllClarificationWatchdogs()
	select {
	case <-lookupCtx.Done():
	case <-time.After(100 * time.Millisecond):
		t.Fatal("watchdog cancellation did not interrupt fallback turn lookup")
	}
}

func TestClarificationWatchdogRecoveryIgnoresOwnCancelActivity(t *testing.T) {
	ctx := context.Background()
	const (
		taskID    = "task-watchdog-own-cancel"
		sessionID = "session-watchdog-own-cancel"
		execution = "exec-watchdog-own-cancel"
	)
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateRunning)
	seedExecutorRunning(t, repo, sessionID, taskID, execution)

	promptDone := make(chan struct{})
	var svc *Service
	agentMgr := &mockAgentManager{
		isAgentRunning: true,
		promptDone:     promptDone,
	}
	agentMgr.cancelAgentFunc = func(cancelCtx context.Context, _ string) error {
		svc.handleAgentStreamEvent(cancelCtx, &lifecycle.AgentStreamEventPayload{
			TaskID:      taskID,
			SessionID:   sessionID,
			ExecutionID: execution,
			Data: &lifecycle.AgentStreamEventData{
				Type:             "session_info",
				PromptGeneration: 1,
			},
		})
		return nil
	}
	agentMgr.currentPromptExecutionID = execution
	agentMgr.currentPromptGeneration.Store(1)
	svc = createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
	svc.turnService = &repoTurnService{repo: repo}
	clarificationTurn, err := svc.turnService.StartTurn(ctx, sessionID)
	if err != nil {
		t.Fatalf("start clarification turn: %v", err)
	}

	watchCtx, cancelWatchdog := context.WithCancel(context.Background())
	defer cancelWatchdog()
	pendingID := "pending-watchdog-own-cancel"
	entry := &clarificationWatchdogEntry{cancel: cancelWatchdog}
	key := svc.clarificationWatchdogKey(sessionID, pendingID)
	svc.clarificationWatchdogs.Store(key, entry)
	t.Cleanup(func() { svc.cancelAllClarificationWatchdogs() })

	recovered := svc.retryClarificationAfterCancel(
		watchCtx,
		clarificationAnsweredData{
			TaskID: taskID, SessionID: sessionID, PendingID: pendingID,
			ClarificationTurnID: clarificationTurn.ID,
		},
		"the clarification answer",
		fmt.Errorf("wrapped: %w", ErrAgentPromptInProgress),
	)
	if !recovered {
		t.Fatal("clarification watchdog recovery did not complete after its own cancellation activity")
	}
	select {
	case <-promptDone:
	case <-time.After(2 * time.Second):
		t.Fatal("clarification answer was not handed off after recovery")
	}
	if err := watchCtx.Err(); err != nil {
		t.Fatalf("watchdog recovery context was cancelled by its own stream activity: %v", err)
	}
	if got := len(agentMgr.capturedPrompts); got != 1 {
		t.Fatalf("clarification replacement prompts = %d, want exactly 1", got)
	}
}

func TestClarificationWatchdogRecoveryCancelsOnIndependentActivity(t *testing.T) {
	ctx := context.Background()
	const (
		taskID    = "task-watchdog-independent-activity"
		sessionID = "session-watchdog-independent-activity"
		execution = "exec-watchdog-independent-activity"
	)
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateRunning)
	seedExecutorRunning(t, repo, sessionID, taskID, execution)

	cancelEntered := make(chan struct{}, 1)
	releaseCancel := make(chan struct{})
	var releaseCancelOnce sync.Once
	release := func() {
		releaseCancelOnce.Do(func() { close(releaseCancel) })
	}
	promptDone := make(chan struct{})
	agentMgr := &mockAgentManager{
		isAgentRunning:           true,
		promptDone:               promptDone,
		cancelAgentBlock:         releaseCancel,
		cancelAgentEntered:       cancelEntered,
		currentPromptExecutionID: execution,
	}
	agentMgr.currentPromptGeneration.Store(1)
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.messageCreator = &mockMessageCreator{}
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
	svc.turnService = &repoTurnService{repo: repo}
	clarificationTurn, err := svc.turnService.StartTurn(ctx, sessionID)
	if err != nil {
		t.Fatalf("start clarification turn: %v", err)
	}

	watchCtx, cancelWatchdog := context.WithCancel(context.Background())
	defer cancelWatchdog()
	pendingID := "pending-watchdog-independent-activity"
	entry := &clarificationWatchdogEntry{cancel: cancelWatchdog}
	key := svc.clarificationWatchdogKey(sessionID, pendingID)
	svc.clarificationWatchdogs.Store(key, entry)
	t.Cleanup(func() {
		release()
		svc.cancelAllClarificationWatchdogs()
	})

	recoveryDone := make(chan bool, 1)
	go func() {
		recoveryDone <- svc.retryClarificationAfterCancel(
			watchCtx,
			clarificationAnsweredData{
				TaskID: taskID, SessionID: sessionID, PendingID: pendingID,
				ClarificationTurnID: clarificationTurn.ID,
			},
			"the clarification answer",
			fmt.Errorf("wrapped: %w", ErrAgentPromptInProgress),
		)
	}()

	select {
	case <-cancelEntered:
	case <-time.After(time.Second):
		t.Fatal("silent cancellation did not reach the agent")
	}

	// This frame is from a newer prompt generation on the same execution. It
	// must remain authoritative even while the fallback cancellation is blocked.
	svc.handleAgentStreamEvent(ctx, &lifecycle.AgentStreamEventPayload{
		TaskID:      taskID,
		SessionID:   sessionID,
		ExecutionID: execution,
		Data: &lifecycle.AgentStreamEventData{
			Type:             "session_info",
			PromptGeneration: 2,
		},
	})
	if watchCtx.Err() == nil {
		t.Fatal("independent stream activity did not cancel the watchdog")
	}

	release()
	select {
	case <-recoveryDone:
	case <-time.After(2 * time.Second):
		t.Fatal("silent cancellation did not settle")
	}
}

func TestClarificationWatchdogRecoveryCancelsOnSameExecutionMessageActivity(t *testing.T) {
	ctx := context.Background()
	const (
		taskID    = "task-watchdog-same-execution-message"
		sessionID = "session-watchdog-same-execution-message"
		execution = "exec-watchdog-same-execution-message"
	)
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateRunning)
	seedExecutorRunning(t, repo, sessionID, taskID, execution)

	cancelEntered := make(chan struct{}, 1)
	releaseCancel := make(chan struct{})
	var releaseCancelOnce sync.Once
	release := func() {
		releaseCancelOnce.Do(func() { close(releaseCancel) })
	}
	agentMgr := &mockAgentManager{
		isAgentRunning:           true,
		cancelAgentBlock:         releaseCancel,
		cancelAgentEntered:       cancelEntered,
		currentPromptExecutionID: execution,
	}
	agentMgr.currentPromptGeneration.Store(1)
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.messageCreator = &mockMessageCreator{}
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
	svc.turnService = &repoTurnService{repo: repo}
	clarificationTurn, err := svc.turnService.StartTurn(ctx, sessionID)
	if err != nil {
		t.Fatalf("start clarification turn: %v", err)
	}

	watchCtx, cancelWatchdog := context.WithCancel(context.Background())
	defer cancelWatchdog()
	pendingID := "pending-watchdog-same-execution-message"
	entry := &clarificationWatchdogEntry{cancel: cancelWatchdog}
	key := svc.clarificationWatchdogKey(sessionID, pendingID)
	svc.clarificationWatchdogs.Store(key, entry)
	t.Cleanup(func() {
		release()
		svc.cancelAllClarificationWatchdogs()
	})

	recoveryDone := make(chan bool, 1)
	go func() {
		recoveryDone <- svc.retryClarificationAfterCancel(
			watchCtx,
			clarificationAnsweredData{
				TaskID: taskID, SessionID: sessionID, PendingID: pendingID,
				ClarificationTurnID: clarificationTurn.ID,
			},
			"the clarification answer",
			fmt.Errorf("wrapped: %w", ErrAgentPromptInProgress),
		)
	}()

	select {
	case <-cancelEntered:
	case <-time.After(time.Second):
		t.Fatal("silent cancellation did not reach the agent")
	}

	// A same-execution message frame is still independent activity. Matching the
	// cancellation's execution and prompt generation does not prove that a
	// message frame came from the cancellation, so it must cancel recovery.
	svc.handleAgentStreamEvent(ctx, &lifecycle.AgentStreamEventPayload{
		TaskID:      taskID,
		SessionID:   sessionID,
		ExecutionID: execution,
		Data: &lifecycle.AgentStreamEventData{
			Type:             "message_streaming",
			PromptGeneration: 1,
		},
	})
	if watchCtx.Err() == nil {
		t.Fatal("same-execution message activity did not cancel the watchdog")
	}

	release()
	select {
	case <-recoveryDone:
	case <-time.After(2 * time.Second):
		t.Fatal("silent cancellation did not settle")
	}
}

func TestClarificationRecoveryCancellationFrameRejectsNormalAgentActivity(t *testing.T) {
	for _, eventType := range []string{"message_streaming", "thinking_streaming", agentEventToolCall, agentEventToolUpdate} {
		t.Run(eventType, func(t *testing.T) {
			if clarificationRecoveryCancellationFrame(&lifecycle.AgentStreamEventPayload{
				Data: &lifecycle.AgentStreamEventData{Type: eventType},
			}) {
				t.Fatalf("%q was classified as recovery cancellation activity", eventType)
			}
		})
	}

	if !clarificationRecoveryCancellationFrame(&lifecycle.AgentStreamEventPayload{
		Data: &lifecycle.AgentStreamEventData{Type: "session_info"},
	}) {
		t.Fatal("session_info was not classified as recovery cancellation activity")
	}
	if !clarificationRecoveryCancellationFrame(&lifecycle.AgentStreamEventPayload{
		Data: &lifecycle.AgentStreamEventData{
			Type: agentEventComplete,
			Data: map[string]interface{}{"stop_reason": "cancelled"},
		},
	}) {
		t.Fatal("cancelled complete was not classified as recovery cancellation activity")
	}
}

// TestRetryClarificationAfterCancel_DoesNotStarveUserCancel is the regression
// test for the production hang where a clarification-timeout recovery left a
// session permanently unstoppable. retryClarificationAfterCancel used to send
// its retry prompt inline while holding the per-session cancelInFlight guard.
// executor.Prompt blocks until a jammed agent accepts the prompt (observed:
// minutes, stuck in an MCP call), so the guard stayed held the whole time —
// and every user Cancel-button click TryLocks that same guard, so it was
// starved and silently no-op'd ("cancel already in flight; skipping
// duplicate"), leaving the session stuck RUNNING forever.
//
// This pins the fix: the retry prompt is dispatched on a background goroutine
// off the guard, so even while that prompt is blocked in-flight, a concurrent
// user CancelAgent still acquires the guard and reaches agentManager.CancelAgent.
func TestRetryClarificationAfterCancel_DoesNotStarveUserCancel(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	retryPromptBlock := make(chan struct{})
	retryPromptEntered := make(chan struct{})
	agentMgr := &mockAgentManager{
		isAgentRunning:         true,
		repoForExecutionLookup: repo,
	}
	taskRepo := newMockTaskRepo()
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), taskRepo, agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})

	// The retry prompt (the only prompt this test dispatches) blocks in-flight,
	// standing in for a jammed agent that never accepts the resume prompt.
	var enteredOnce sync.Once
	agentMgr.promptAgentFunc = func(context.Context, string, string, []v1.MessageAttachment, bool) (*executor.PromptResult, error) {
		enteredOnce.Do(func() { close(retryPromptEntered) })
		<-retryPromptBlock
		return &executor.PromptResult{}, nil
	}
	t.Cleanup(func() { close(retryPromptBlock) })

	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateRunning)
	seedExecutorRunning(t, repo, "session1", "task1", "exec-1")
	turnService := &repoTurnService{repo: repo}
	svc.turnService = turnService
	clarificationTurn, err := turnService.StartTurn(ctx, "session1")
	if err != nil {
		t.Fatalf("start clarification turn: %v", err)
	}

	// Kick off the clarification-timeout recovery. Its silent cancel succeeds
	// (mock CancelAgent returns nil), then it hands the retry prompt to the
	// async path and returns — releasing the guard — even though that prompt is
	// still blocked inside PromptAgent.
	recoveryDone := make(chan struct{})
	go func() {
		svc.retryClarificationAfterCancel(ctx, clarificationAnsweredData{
			TaskID: "task1", SessionID: "session1", ClarificationTurnID: clarificationTurn.ID,
		}, "the clarification answer", fmt.Errorf("wrap: %w", ErrAgentPromptInProgress))
		close(recoveryDone)
	}()

	select {
	case <-retryPromptEntered:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the async retry prompt to reach the agent")
	}
	select {
	case <-recoveryDone:
	case <-time.After(2 * time.Second):
		t.Fatal("retryClarificationAfterCancel blocked on the in-flight retry prompt instead of releasing the guard")
	}

	// The recovery's silent cancel was the first agent-cancel call.
	if got := agentMgr.cancelAgentCalls.Load(); got != 1 {
		t.Fatalf("expected exactly 1 agent cancel from recovery's silent cancel, got %d", got)
	}

	// The user clicks Cancel while the retry prompt is still blocked in-flight.
	// Before the fix this returned immediately as a starved no-op; now it must
	// acquire the guard and actually reach agentManager.CancelAgent.
	if err := svc.CancelAgent(ctx, "session1"); err != nil {
		t.Fatalf("user CancelAgent returned error: %v", err)
	}
	if got := agentMgr.cancelAgentCalls.Load(); got != 2 {
		t.Fatalf("user cancel was starved by a leaked guard: expected 2 agent cancel calls, got %d", got)
	}
}

func TestRetryClarificationAfterCancel_CoordinatorCancellationWinsWhileRetryWaits(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-retry-stop", "session-retry-stop", models.TaskSessionStateRunning)
	manager := &mockAgentManager{isAgentRunning: true, repoForExecutionLookup: repo}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), manager)
	svc.executor = executor.NewExecutor(manager, repo, testLogger(), executor.ExecutorConfig{})

	guard, release := svc.acquireCancelInFlightGuard("session-retry-stop")
	guard.Lock()
	done := make(chan bool, 1)
	go func() {
		done <- svc.retryClarificationAfterCancel(
			ctx,
			clarificationAnsweredData{TaskID: "task-retry-stop", SessionID: "session-retry-stop"},
			"clarification answer",
			fmt.Errorf("wrapped: %w", ErrAgentPromptInProgress),
		)
	}()
	coordinatorStopWaitForGuardRefs(t, svc, "session-retry-stop", 2)
	changed, _, err := repo.CancelActiveTaskSession(
		ctx,
		"session-retry-stop",
		coordinatorMCPStopReason,
	)
	if err != nil || !changed {
		t.Fatalf("cancel running session: changed=%v err=%v", changed, err)
	}
	guard.Unlock()
	release()

	select {
	case recovered := <-done:
		if recovered {
			t.Fatal("clarification retry reported recovery after coordinator cancellation")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for guarded clarification retry")
	}
	if got := manager.cancelAgentCalls.Load(); got != 0 {
		t.Fatalf("clarification retry cancelled agent after stop won: %d calls", got)
	}
	if got := svc.messageQueue.GetStatus(ctx, "session-retry-stop").Count; got != 0 {
		t.Fatalf("clarification retry queued replacement after stop won: %d messages", got)
	}
	session, err := repo.GetTaskSession(ctx, "session-retry-stop")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if session.State != models.TaskSessionStateCancelled {
		t.Fatalf("expected cancelled session, got %q", session.State)
	}
}

// TestDispatchClarificationResumeLocked_ReturnPaths pins the two outcomes that
// are deterministically reachable at this seam — a genuine error (nil queue)
// and an immediate dispatch (nil) — so the caller can tell a real failure from
// success and log accordingly (bot review on PR #1680: the old bool return
// logged an alarming "failed to resume agent" error even when the answer was
// safely queued).
//
// The third outcome, errClarificationResumeQueuedForDrain, is not unit-testable
// here: takeAndDispatchEntryLocked always finds and dispatches the entry this
// function just queued, so the sentinel only arises when a concurrent take
// removes that entry first (targeted take misses) AND a rival dispatch is
// already settling (drainQueuedMessageForPromptableSessionLocked backs off on
// isQueuedDispatchInFlight). That race is driven end-to-end by
// TestQueueAndInterruptForPeerMessage_RacesClarificationTimeoutRecovery, which
// asserts the answer stays queued for the recovered turn's own natural drain.
func TestDispatchClarificationResumeLocked_ReturnPaths(t *testing.T) {
	ctx := context.Background()
	data := clarificationAnsweredData{TaskID: "t1", SessionID: "s1"}

	t.Run("nil message queue is a genuine error", func(t *testing.T) {
		repo := setupTestRepo(t)
		svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), &mockAgentManager{})
		svc.messageQueue = nil

		err := svc.dispatchClarificationResumeLocked(ctx, data, "answer")
		if err == nil {
			t.Fatal("expected an error when the message queue is not configured")
		}
		if errors.Is(err, errClarificationResumeQueuedForDrain) {
			t.Fatalf("nil queue must not be reported as the benign queued-for-drain case: %v", err)
		}
	})

	t.Run("immediate dispatch returns nil", func(t *testing.T) {
		repo := setupTestRepo(t)
		agentMgr := &mockAgentManager{isAgentRunning: true, repoForExecutionLookup: repo}
		svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
		svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
		seedTaskAndSession(t, repo, "t1", "s1", models.TaskSessionStateWaitingForInput)
		seedExecutorRunning(t, repo, "s1", "t1", "exec-1")

		if err := svc.dispatchClarificationResumeLocked(ctx, data, "answer"); err != nil {
			t.Fatalf("expected nil on immediate dispatch, got %v", err)
		}
	})
}

func countClarificationWatchdogs(svc *Service) int {
	count := 0
	svc.clarificationWatchdogs.Range(func(_, _ interface{}) bool {
		count++
		return true
	})
	return count
}
