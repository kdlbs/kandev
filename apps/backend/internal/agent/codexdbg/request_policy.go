package codexdbg

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/kandev/kandev/pkg/codexappserver"
)

var requestDefaults = map[string]json.RawMessage{
	codexappserver.ServerRequestCommandExecutionApproval: json.RawMessage(`{"decision":"decline"}`),
	codexappserver.ServerRequestFileChangeApproval:       json.RawMessage(`{"decision":"decline"}`),
	codexappserver.ServerRequestToolUserInput:            json.RawMessage(`{"answers":{}}`),
	codexappserver.ServerRequestMCPElicitation:           json.RawMessage(`{"action":"cancel","content":null,"_meta":null}`),
	codexappserver.ServerRequestPermissionsApproval:      json.RawMessage(`{"permissions":{},"scope":"turn"}`),
	codexappserver.ServerRequestApplyPatchApproval:       json.RawMessage(`{"decision":{"denied":{"rejection":"Codex debugger declined the request."}}}`),
	codexappserver.ServerRequestExecCommandApproval:      json.RawMessage(`{"decision":{"denied":{"rejection":"Codex debugger declined the request."}}}`),
}

var requestRejections = map[string]struct{}{
	codexappserver.ServerRequestDynamicToolCall:     {},
	codexappserver.ServerRequestAuthTokensRefresh:   {},
	codexappserver.ServerRequestAttestationGenerate: {},
}

// RequestPolicy gives app-server requests a safe default and only accepts
// answer-file responses for methods in the pinned protocol.
type RequestPolicy struct {
	answers map[string]json.RawMessage
}

func NewRequestPolicy(answers map[string]json.RawMessage) *RequestPolicy {
	copyAnswers := make(map[string]json.RawMessage, len(answers))
	for method, answer := range answers {
		copyAnswers[method] = append(json.RawMessage(nil), answer...)
	}
	return &RequestPolicy{answers: copyAnswers}
}

// Handle implements codexappserver.RequestHandler.
func (p *RequestPolicy) Handle(ctx context.Context, request codexappserver.ServerRequest) (any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	method := request.Method
	if _, known := requestDefaults[method]; !known {
		if _, rejected := requestRejections[method]; rejected {
			return nil, &codexappserver.RPCError{Code: -32601, Message: "unsupported server request: " + method}
		}
		return nil, &codexappserver.RPCError{Code: -32601, Message: "method not found: " + method}
	}
	if answer, ok := p.answers[method]; ok {
		if !json.Valid(answer) {
			return nil, fmt.Errorf("answer for %s is invalid JSON", method)
		}
		if method == codexappserver.ServerRequestToolUserInput {
			if err := validateToolUserInputAnswer(request.Params, answer); err != nil {
				return nil, fmt.Errorf("answer for %s: %w", method, err)
			}
		}
		return answer, nil
	}
	return requestDefaults[method], nil
}

type toolUserInputQuestion struct {
	ID      string `json:"id"`
	IsOther bool   `json:"isOther"`
	Options []struct {
		Label string `json:"label"`
	} `json:"options"`
}

type toolUserInputAnswer struct {
	Answers []string `json:"answers"`
}

func validateToolUserInputAnswer(rawRequest, rawAnswer json.RawMessage) error {
	questions, err := decodeToolUserInputQuestions(rawRequest)
	if err != nil {
		return err
	}
	answers, err := decodeToolUserInputAnswers(rawAnswer)
	if err != nil {
		return err
	}
	if len(answers) != len(questions) {
		return errors.New("response must answer every request question exactly once")
	}
	for questionID, answer := range answers {
		if err := validateToolUserInputResponse(questions, questionID, answer); err != nil {
			return err
		}
	}
	return nil
}

func decodeToolUserInputQuestions(rawRequest json.RawMessage) (map[string]toolUserInputQuestion, error) {
	var request struct {
		Questions []toolUserInputQuestion `json:"questions"`
	}
	if err := json.Unmarshal(rawRequest, &request); err != nil || len(request.Questions) == 0 {
		return nil, errors.New("request contains no valid questions")
	}
	questions := make(map[string]toolUserInputQuestion, len(request.Questions))
	for _, question := range request.Questions {
		if question.ID == "" {
			return nil, errors.New("request question has an empty ID")
		}
		if _, exists := questions[question.ID]; exists {
			return nil, fmt.Errorf("request contains duplicate question ID %q", question.ID)
		}
		questions[question.ID] = question
	}
	return questions, nil
}

func decodeToolUserInputAnswers(rawAnswer json.RawMessage) (map[string]toolUserInputAnswer, error) {
	var response struct {
		Answers map[string]toolUserInputAnswer `json:"answers"`
	}
	decoder := json.NewDecoder(bytes.NewReader(rawAnswer))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&response); err != nil || response.Answers == nil {
		return nil, errors.New("response must contain an answers object")
	}
	return response.Answers, nil
}

func validateToolUserInputResponse(questions map[string]toolUserInputQuestion, questionID string, answer toolUserInputAnswer) error {
	question, exists := questions[questionID]
	if !exists {
		return fmt.Errorf("response contains unknown question ID %q", questionID)
	}
	if len(answer.Answers) != 1 || strings.TrimSpace(answer.Answers[0]) == "" {
		return fmt.Errorf("response for question %q must contain one non-empty answer", questionID)
	}
	if len(question.Options) == 0 || question.IsOther {
		return nil
	}
	if !toolUserInputOptionOffered(question, answer.Answers[0]) {
		return fmt.Errorf("response for question %q does not select an offered option", questionID)
	}
	return nil
}

func toolUserInputOptionOffered(question toolUserInputQuestion, answer string) bool {
	for _, option := range question.Options {
		if option.Label == answer {
			return true
		}
	}
	return false
}

// LoadAnswerFile reads explicit method-specific JSON responses. Unknown
// methods are rejected so an answer cannot grant an unreviewed request.
func LoadAnswerFile(path string) (map[string]json.RawMessage, error) {
	if path == "" {
		return nil, errors.New("answer-file path is required")
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read answer file: %w", err)
	}
	var answers map[string]json.RawMessage
	if err := json.Unmarshal(content, &answers); err != nil {
		return nil, fmt.Errorf("decode answer file: %w", err)
	}
	for method, answer := range answers {
		if _, known := requestDefaults[method]; !known {
			return nil, fmt.Errorf("answer file names unsupported server request %q", method)
		}
		if !json.Valid(answer) {
			return nil, fmt.Errorf("answer for %s is not valid JSON", method)
		}
	}
	return answers, nil
}
