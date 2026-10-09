package codexappserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	agenttypes "github.com/kandev/kandev/internal/agentctl/types"
	protocol "github.com/kandev/kandev/pkg/codexappserver"
)

const maxNativeInputQuestions = 4

func (a *Adapter) handleUserInputRequest(ctx context.Context, raw json.RawMessage) (any, error) {
	var params protocol.ToolRequestUserInputParams
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, &protocol.RPCError{Code: -32602, Message: "invalid Codex user input request"}
	}
	if strings.TrimSpace(params.ThreadID) == "" || strings.TrimSpace(params.TurnID) == "" || strings.TrimSpace(params.ItemID) == "" {
		return nil, &protocol.RPCError{Code: -32602, Message: "Codex user input request is missing its thread, turn, or item ID"}
	}
	a.mu.RLock()
	activeThreadID := a.threadID
	_, isOwnedChildThread := a.children[params.ThreadID]
	handler := a.userInputRequest
	a.mu.RUnlock()
	if activeThreadID == "" || (params.ThreadID != activeThreadID && !isOwnedChildThread) {
		return nil, &protocol.RPCError{Code: -32602, Message: "Codex user input request belongs to an inactive thread"}
	}
	request, err := normalizeUserInputRequest(params)
	if err != nil {
		return nil, &protocol.RPCError{Code: -32602, Message: err.Error()}
	}
	if handler == nil {
		return nil, &protocol.RPCError{Code: -32601, Message: "Codex user input request handler is unavailable"}
	}
	response, err := handler(ctx, request)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, &protocol.RPCError{Code: -32603, Message: "Kandev clarification request failed"}
	}
	if response == nil {
		return nil, &protocol.RPCError{Code: -32603, Message: "Kandev clarification returned no response"}
	}
	return makeUserInputResponse(params.Questions, request.Questions, response)
}

func normalizeUserInputRequest(params protocol.ToolRequestUserInputParams) (*agenttypes.UserInputRequest, error) {
	if len(params.Questions) == 0 || len(params.Questions) > maxNativeInputQuestions {
		return nil, fmt.Errorf("codex user input request must contain 1 to %d questions", maxNativeInputQuestions)
	}
	request := &agenttypes.UserInputRequest{
		ThreadID: params.ThreadID,
		TurnID:   params.TurnID,
		ItemID:   params.ItemID,
	}
	seen := make(map[string]struct{}, len(params.Questions))
	request.Questions = make([]agenttypes.UserInputQuestion, 0, len(params.Questions))
	for questionIndex, question := range params.Questions {
		normalized, err := normalizeUserInputQuestion(question, questionIndex, seen)
		if err != nil {
			return nil, err
		}
		request.Questions = append(request.Questions, normalized)
	}
	return request, nil
}

func normalizeUserInputQuestion(
	question protocol.ToolRequestUserInputQuestion,
	questionIndex int,
	seen map[string]struct{},
) (agenttypes.UserInputQuestion, error) {
	if strings.TrimSpace(question.ID) == "" || strings.TrimSpace(question.Question) == "" {
		return agenttypes.UserInputQuestion{}, fmt.Errorf("codex user input question %d is missing its ID or prompt", questionIndex+1)
	}
	if _, exists := seen[question.ID]; exists {
		return agenttypes.UserInputQuestion{}, fmt.Errorf("codex user input request contains duplicate question ID %q", question.ID)
	}
	seen[question.ID] = struct{}{}
	if question.IsSecret {
		return agenttypes.UserInputQuestion{}, errors.New("secret codex user input questions are unsupported by Kandev clarification controls")
	}
	if len(question.Options) != 0 && (len(question.Options) < 2 || len(question.Options) > 6) {
		return agenttypes.UserInputQuestion{}, fmt.Errorf("codex user input question %q must offer 2 to 6 options", question.ID)
	}
	if len(question.Options) == 0 && !question.IsOther {
		return agenttypes.UserInputQuestion{}, fmt.Errorf("codex user input question %q has no answerable options", question.ID)
	}
	options, err := normalizeUserInputOptions(question.ID, question.Options)
	if err != nil {
		return agenttypes.UserInputQuestion{}, err
	}
	return agenttypes.UserInputQuestion{
		ID:       question.ID,
		Header:   question.Header,
		Prompt:   question.Question,
		IsOther:  question.IsOther,
		IsSecret: question.IsSecret,
		Options:  options,
	}, nil
}

func normalizeUserInputOptions(
	questionID string,
	options []protocol.ToolRequestUserInputOption,
) ([]agenttypes.UserInputOption, error) {
	normalized := make([]agenttypes.UserInputOption, 0, len(options))
	for optionIndex, option := range options {
		if strings.TrimSpace(option.Label) == "" {
			return nil, fmt.Errorf("codex user input question %q contains an option without a label", questionID)
		}
		normalized = append(normalized, agenttypes.UserInputOption{
			OptionID:    fmt.Sprintf("codex-option-%d", optionIndex),
			Label:       option.Label,
			Description: option.Description,
		})
	}
	return normalized, nil
}

func makeUserInputResponse(
	providerQuestions []protocol.ToolRequestUserInputQuestion,
	kandevQuestions []agenttypes.UserInputQuestion,
	response *agenttypes.UserInputResponse,
) (any, error) {
	answers := emptyUserInputAnswers(providerQuestions)
	if response.Rejected {
		return protocol.ToolRequestUserInputResponse{Answers: answers}, nil
	}
	if len(response.Answers) != len(providerQuestions) {
		return nil, &protocol.RPCError{Code: -32602, Message: "clarification response must answer every Codex question exactly once"}
	}
	for questionIndex, providerQuestion := range providerQuestions {
		kandevQuestion := kandevQuestions[questionIndex]
		answer, exists := response.Answers[providerQuestion.ID]
		if !exists {
			return nil, &protocol.RPCError{Code: -32602, Message: "clarification response is missing a Codex question answer"}
		}
		values, err := userInputAnswerValues(kandevQuestion, answer)
		if err != nil {
			return nil, err
		}
		answers[providerQuestion.ID] = protocol.ToolRequestUserInputAnswer{Answers: values}
	}
	if err := rejectUnknownUserInputAnswers(response.Answers, answers); err != nil {
		return nil, err
	}
	return protocol.ToolRequestUserInputResponse{Answers: answers}, nil
}

func emptyUserInputAnswers(
	questions []protocol.ToolRequestUserInputQuestion,
) map[string]protocol.ToolRequestUserInputAnswer {
	answers := make(map[string]protocol.ToolRequestUserInputAnswer, len(questions))
	for _, question := range questions {
		answers[question.ID] = protocol.ToolRequestUserInputAnswer{Answers: []string{}}
	}
	return answers
}

func userInputAnswerValues(
	question agenttypes.UserInputQuestion,
	answer agenttypes.UserInputAnswer,
) ([]string, error) {
	if len(answer.OptionIDs) > 1 {
		return nil, &protocol.RPCError{Code: -32602, Message: "clarification response selected multiple options for one Codex question"}
	}
	if answer.CustomText != "" && (!question.IsOther || len(answer.OptionIDs) != 0) {
		return nil, &protocol.RPCError{Code: -32602, Message: "clarification response contains unsupported custom text"}
	}
	values := make([]string, 0, 1)
	if answer.CustomText != "" {
		if strings.TrimSpace(answer.CustomText) == "" {
			return nil, &protocol.RPCError{Code: -32602, Message: "clarification response contains empty custom text"}
		}
		values = append(values, answer.CustomText)
	}
	if len(answer.OptionIDs) == 1 {
		label, ok := userInputOptionLabel(question.Options, answer.OptionIDs[0])
		if !ok {
			return nil, &protocol.RPCError{Code: -32602, Message: "clarification response selected an option not offered by Codex"}
		}
		values = append(values, label)
	}
	if len(values) == 0 {
		return nil, &protocol.RPCError{Code: -32602, Message: "clarification response contains an unanswered Codex question"}
	}
	return values, nil
}

func rejectUnknownUserInputAnswers(
	response map[string]agenttypes.UserInputAnswer,
	providerAnswers map[string]protocol.ToolRequestUserInputAnswer,
) error {
	for questionID := range response {
		if _, ok := providerAnswers[questionID]; !ok {
			return &protocol.RPCError{Code: -32602, Message: "clarification response contains an unknown Codex question ID"}
		}
	}
	return nil
}

func userInputOptionLabel(options []agenttypes.UserInputOption, selectedID string) (string, bool) {
	for _, option := range options {
		if option.OptionID == selectedID {
			return option.Label, true
		}
	}
	return "", false
}
