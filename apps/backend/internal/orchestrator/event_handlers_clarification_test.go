package orchestrator

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/events/bus"
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

func countClarificationWatchdogs(svc *Service) int {
	count := 0
	svc.clarificationWatchdogs.Range(func(_, _ interface{}) bool {
		count++
		return true
	})
	return count
}
