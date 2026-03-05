package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

// subscribeClarificationEvents subscribes to clarification-related events.
func (s *Service) subscribeClarificationEvents() {
	if s.eventBus == nil {
		return
	}
	if _, err := s.eventBus.Subscribe(events.ClarificationAnswered, s.handleClarificationAnswered); err != nil {
		s.logger.Error("failed to subscribe to clarification.answered events", zap.Error(err))
	}
}

// clarificationAnsweredData is the event payload for ClarificationAnswered events.
type clarificationAnsweredData struct {
	SessionID    string `json:"session_id"`
	TaskID       string `json:"task_id"`
	Question     string `json:"question"`
	AnswerText   string `json:"answer_text"`
	Rejected     bool   `json:"rejected"`
	RejectReason string `json:"reject_reason"`
}

// handleClarificationAnswered handles user responses to agent clarification questions.
// It constructs a follow-up prompt with the answer and sends it to the agent.
func (s *Service) handleClarificationAnswered(ctx context.Context, event *bus.Event) error {
	dataBytes, err := json.Marshal(event.Data)
	if err != nil {
		s.logger.Error("failed to marshal clarification event data", zap.Error(err))
		return nil
	}
	var data clarificationAnsweredData
	if err := json.Unmarshal(dataBytes, &data); err != nil {
		s.logger.Error("failed to parse clarification event data", zap.Error(err))
		return nil
	}

	if data.SessionID == "" || data.TaskID == "" {
		s.logger.Warn("clarification answered event missing session_id or task_id",
			zap.String("session_id", data.SessionID),
			zap.String("task_id", data.TaskID))
		return nil
	}

	prompt := buildClarificationPrompt(data)

	s.logger.Info("resuming agent with clarification answer",
		zap.String("task_id", data.TaskID),
		zap.String("session_id", data.SessionID),
		zap.Bool("rejected", data.Rejected))

	if _, err := s.PromptTask(ctx, data.TaskID, data.SessionID, prompt, "", false, nil); err != nil {
		s.logger.Error("failed to resume agent with clarification answer",
			zap.String("task_id", data.TaskID),
			zap.String("session_id", data.SessionID),
			zap.Error(err))
	}
	return nil
}

// buildClarificationPrompt constructs the resume prompt from a clarification answer.
func buildClarificationPrompt(data clarificationAnsweredData) string {
	if data.Rejected {
		reason := data.RejectReason
		if reason == "" {
			reason = "No reason provided"
		}
		return fmt.Sprintf("The user declined to answer your question: %q\nReason: %s\nPlease continue without this information.",
			data.Question, reason)
	}
	return fmt.Sprintf("You previously asked the user: %q\n%s\nPlease continue with this information.",
		data.Question, data.AnswerText)
}
