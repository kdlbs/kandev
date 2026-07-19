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
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type zeroClarificationCanceller struct {
	sessions []string
}

func (c *zeroClarificationCanceller) DetachSessionAndNotify(_ context.Context, sessionID string) int {
	c.sessions = append(c.sessions, sessionID)
	return 0
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
	svc := &Service{
		logger:                       testLogger(),
		clarificationWatchdogTimeout: 500 * time.Millisecond,
	}
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

	if got := countClarificationWatchdogs(svc); got != 1 {
		t.Fatalf("expected 1 active watchdog, got %d", got)
	}
}

func TestHandleAgentStreamEvent_CancelsClarificationWatchdogs(t *testing.T) {
	svc := &Service{
		logger:                       testLogger(),
		clarificationWatchdogTimeout: time.Second,
	}
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

func TestClarificationWatchdog_ExpiresAndClearsEntry(t *testing.T) {
	repo := setupTestRepo(t)
	agentMgr := &mockAgentManager{isAgentRunning: true}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.clarificationWatchdogTimeout = 20 * time.Millisecond
	t.Cleanup(func() { svc.cancelAllClarificationWatchdogs() })

	seedTaskAndSession(t, repo, "t1", "s1", models.TaskSessionStateCompleted)

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

	time.Sleep(100 * time.Millisecond)

	if got := countClarificationWatchdogs(svc); got != 0 {
		t.Fatalf("expected watchdog map to be empty after timeout, got %d", got)
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

	// Kick off the clarification-timeout recovery. Its silent cancel succeeds
	// (mock CancelAgent returns nil), then it hands the retry prompt to the
	// async path and returns — releasing the guard — even though that prompt is
	// still blocked inside PromptAgent.
	recoveryDone := make(chan struct{})
	go func() {
		svc.retryClarificationAfterCancel(ctx, clarificationAnsweredData{
			TaskID: "task1", SessionID: "session1",
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
