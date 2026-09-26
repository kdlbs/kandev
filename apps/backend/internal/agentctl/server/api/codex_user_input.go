package api

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/agentctl/server/adapter"
	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/clarification"
	"github.com/kandev/kandev/internal/common/logger"
	mcp "github.com/kandev/kandev/internal/mcp/server"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

func newCodexUserInputRequestHandler(
	cfg *config.InstanceConfig,
	backend *mcp.ChannelBackendClient,
	log *logger.Logger,
) adapter.UserInputRequestHandler {
	return func(ctx context.Context, request *adapter.UserInputRequest) (*adapter.UserInputResponse, error) {
		questions, err := clarificationQuestions(request.Questions)
		if err != nil {
			return nil, err
		}
		payload := struct {
			SessionID         string                   `json:"session_id"`
			TaskID            string                   `json:"task_id"`
			Questions         []clarification.Question `json:"questions"`
			AllowFreeTextOnly bool                     `json:"allow_free_text_only,omitempty"`
		}{
			SessionID:         cfg.SessionID,
			TaskID:            cfg.TaskID,
			Questions:         questions,
			AllowFreeTextOnly: true,
		}
		var result clarification.Response
		if err := backend.RequestPayload(ctx, ws.ActionMCPAskUserQuestion, payload, &result); err != nil {
			if ctx.Err() != nil {
				go notifyCodexQuestionCancellation(backend, cfg.SessionID, log)
			}
			return nil, err
		}
		return normalizeClarificationResponse(request.Questions, result)
	}
}

func clarificationQuestions(questions []adapter.UserInputQuestion) ([]clarification.Question, error) {
	if len(questions) == 0 {
		return nil, errors.New("codex user input request has no questions")
	}
	result := make([]clarification.Question, 0, len(questions))
	for _, question := range questions {
		if question.IsSecret {
			return nil, fmt.Errorf("secret Codex user input question %q cannot be stored in Kandev chat", question.ID)
		}
		allowCustomText := question.IsOther
		converted := clarification.Question{
			ID:              question.ID,
			Title:           question.Header,
			Prompt:          question.Prompt,
			AllowCustomText: &allowCustomText,
			Options:         make([]clarification.Option, 0, len(question.Options)),
		}
		for _, option := range question.Options {
			converted.Options = append(converted.Options, clarification.Option{
				ID:          option.OptionID,
				Label:       option.Label,
				Description: option.Description,
			})
		}
		result = append(result, converted)
	}
	return result, nil
}

func normalizeClarificationResponse(
	questions []adapter.UserInputQuestion,
	response clarification.Response,
) (*adapter.UserInputResponse, error) {
	if response.Rejected {
		return &adapter.UserInputResponse{Rejected: true}, nil
	}
	if len(response.Answers) != len(questions) {
		return nil, errors.New("clarification response must answer every Codex question exactly once")
	}
	questionsByID := make(map[string]adapter.UserInputQuestion, len(questions))
	for _, question := range questions {
		questionsByID[question.ID] = question
	}
	answers := make(map[string]adapter.UserInputAnswer, len(response.Answers))
	for _, answer := range response.Answers {
		question, exists := questionsByID[answer.QuestionID]
		if !exists {
			return nil, fmt.Errorf("clarification response contains unknown Codex question ID %q", answer.QuestionID)
		}
		if _, duplicate := answers[answer.QuestionID]; duplicate {
			return nil, fmt.Errorf("clarification response repeats Codex question ID %q", answer.QuestionID)
		}
		normalized, err := normalizeClarificationAnswer(question, answer)
		if err != nil {
			return nil, err
		}
		answers[answer.QuestionID] = normalized
	}
	if len(answers) != len(questionsByID) {
		return nil, errors.New("clarification response is missing a Codex question answer")
	}
	return &adapter.UserInputResponse{Answers: answers}, nil
}

func normalizeClarificationAnswer(
	question adapter.UserInputQuestion,
	answer clarification.Answer,
) (adapter.UserInputAnswer, error) {
	if len(answer.SelectedOptions) > 1 {
		return adapter.UserInputAnswer{}, fmt.Errorf("clarification response selected multiple options for Codex question %q", answer.QuestionID)
	}
	if answer.CustomText != "" && (!question.IsOther || len(answer.SelectedOptions) != 0) {
		return adapter.UserInputAnswer{}, fmt.Errorf("clarification response contains unsupported custom text for Codex question %q", answer.QuestionID)
	}
	if len(answer.SelectedOptions) == 0 && strings.TrimSpace(answer.CustomText) == "" {
		return adapter.UserInputAnswer{}, fmt.Errorf("clarification response left Codex question %q unanswered", answer.QuestionID)
	}
	if len(answer.SelectedOptions) == 1 && !userInputOfferedOption(question, answer.SelectedOptions[0]) {
		return adapter.UserInputAnswer{}, fmt.Errorf("clarification response selected an unoffered option for Codex question %q", answer.QuestionID)
	}
	return adapter.UserInputAnswer{
		OptionIDs:  append([]string(nil), answer.SelectedOptions...),
		CustomText: answer.CustomText,
	}, nil
}

func userInputOfferedOption(question adapter.UserInputQuestion, selectedID string) bool {
	for _, option := range question.Options {
		if option.OptionID == selectedID {
			return true
		}
	}
	return false
}

func notifyCodexQuestionCancellation(backend *mcp.ChannelBackendClient, sessionID string, log *logger.Logger) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := backend.RequestPayload(ctx, ws.ActionMCPClarificationTimeout, map[string]string{"session_id": sessionID}, nil); err != nil {
		log.Warn("failed to cancel Codex clarification after provider resolution",
			zap.String("session_id", sessionID),
			zap.Error(err))
	}
}
