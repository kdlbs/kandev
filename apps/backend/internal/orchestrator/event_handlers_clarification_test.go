package orchestrator

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/lifecycle"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
)

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
