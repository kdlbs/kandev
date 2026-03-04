package orchestrator

import (
	"context"
	"strings"
	"testing"

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
