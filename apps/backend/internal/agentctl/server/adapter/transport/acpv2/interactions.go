package acpv2

import (
	"context"
	"encoding/json"
	"slices"

	acp "github.com/coder/acp-go-sdk"
	wire "github.com/coder/acp-go-sdk/experimental/v2"
	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

func (a *Adapter) requestPermission(ctx context.Context, request wire.RequestPermissionRequest) (wire.RequestPermissionResponse, error) {
	result := wire.RequestPermissionResponse{Outcome: wire.CancelledPermission()}
	a.mu.Lock()
	turn, handler := a.interactionTurn(request.SessionID), a.approval
	a.mu.Unlock()
	if turn == nil || handler == nil {
		return result, nil
	}
	normalized, err := permissionRequest(turn.session, request)
	if err != nil {
		return result, err
	}
	ctx, cancel := context.WithTimeout(ctx, a.cfg.GetPermissionTimeout())
	defer cancel()
	unlink := context.AfterFunc(turn.ctx, cancel)
	defer unlink()
	reply, err := handler(ctx, normalized)
	if err != nil || ctx.Err() != nil || turn.ctx.Err() != nil || reply == nil || reply.Cancelled {
		return result, nil
	}
	for _, option := range normalized.Options {
		if option.OptionID == reply.OptionID {
			result.Outcome = wire.SelectedPermission(wire.PermissionOptionID(reply.OptionID))
			break
		}
	}
	return result, nil
}

// interactionTurn validates native identity with mu held. Callbacks are
// linked to the captured turn and to the SDK's request context.
func (a *Adapter) interactionTurn(session wire.SessionID) *promptTurn {
	turn := a.turn
	if turn == nil || a.blocked || turn.terminal || a.ctx.Err() != nil {
		return nil
	}
	if session == turn.session || a.state.children[session].toolCallID != "" {
		return turn
	}
	return nil
}

func permissionRequest(root wire.SessionID, req wire.RequestPermissionRequest) (*types.PermissionRequest, error) {
	request := &types.PermissionRequest{SessionID: string(root), Title: req.Title}
	seen := make(map[wire.PermissionOptionID]bool)
	for _, option := range req.Options {
		if option.OptionID == "" || seen[option.OptionID] {
			return nil, acp.NewInvalidParams(nil)
		}
		seen[option.OptionID] = true
		switch option.Kind {
		case wire.PermissionAllowOnce, wire.PermissionAllowAlways, wire.PermissionRejectOnce, wire.PermissionRejectAlways:
			request.Options = append(request.Options, streams.PermissionOption{
				OptionID: string(option.OptionID), Name: option.Name, Kind: streams.PermissionOptionKind(option.Kind),
			})
		}
	}
	if len(request.Options) == 0 {
		return nil, acp.NewInvalidParams(nil)
	}
	if subject := req.Subject; subject != nil {
		switch subject.Type {
		case "tool_call":
			if tool := subject.ToolCall; tool != nil {
				request.ToolCallID = entityID(req.SessionID, tool.ToolCallID)
				if name, ok := tool.Name.Get(); ok && name != "" {
					request.ToolName = &name
				}
			}
		case "command":
			request.ActionType = string(streams.ActionTypeCommand)
			request.ActionDetails = map[string]any{"command": valueOrZero(subject.Command), "cwd": valueOrZero(subject.Cwd)}
			if subject.ToolCallID != nil {
				request.ToolCallID = entityID(req.SessionID, *subject.ToolCallID)
			}
		}
	}
	return request, nil
}

func (a *Adapter) createElicitation(ctx context.Context, req wire.CreateElicitationRequest) (wire.CreateElicitationResponse, error) {
	if req.Mode != wire.ElicitationModeForm || req.SessionID == nil || req.RequestedSchema == nil {
		return wire.CreateElicitationResponse{}, acp.NewInvalidParams(nil)
	}
	a.mu.Lock()
	turn, handler := a.interactionTurn(*req.SessionID), a.input
	a.mu.Unlock()
	if turn == nil || handler == nil {
		return wire.CreateElicitationResponse{Action: wire.ElicitationCancel}, nil
	}
	questions, ok := projectQuestions(req.Message, req.RequestedSchema)
	if !ok {
		return wire.CreateElicitationResponse{Action: wire.ElicitationDecline}, nil
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	unlink := context.AfterFunc(turn.ctx, cancel)
	defer unlink()
	answer, err := handler(ctx, &types.UserInputRequest{
		ThreadID: string(*req.SessionID), ItemID: uuid.NewString(), Questions: questions, ScopedCancellation: true,
	})
	if err != nil || ctx.Err() != nil || turn.ctx.Err() != nil || answer == nil {
		return wire.CreateElicitationResponse{Action: wire.ElicitationCancel}, nil
	}
	return projectAnswer(questions, answer), nil
}

func projectQuestions(prompt string, schema *wire.ElicitationSchema) ([]types.UserInputQuestion, bool) {
	if len(schema.Properties) == 0 || len(schema.Properties) > 4 {
		return nil, false
	}
	ids := make([]string, 0, len(schema.Properties))
	for id := range schema.Properties {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	questions := make([]types.UserInputQuestion, 0, len(ids))
	for _, id := range ids {
		field := schema.Properties[id]
		if field.Type != "string" {
			return nil, false
		}
		question, ok := projectStringQuestion(id, prompt, field.Raw)
		if !ok {
			return nil, false
		}
		questions = append(questions, question)
	}
	return questions, true
}

func projectStringQuestion(id, prompt string, raw json.RawMessage) (types.UserInputQuestion, bool) {
	var field struct {
		Title       string          `json:"title"`
		Description string          `json:"description"`
		Format      string          `json:"format"`
		Enum        []string        `json:"enum"`
		OneOf       json.RawMessage `json:"oneOf"`
		AnyOf       json.RawMessage `json:"anyOf"`
		MinLength   *int            `json:"minLength"`
		MaxLength   *int            `json:"maxLength"`
	}
	if id == "" || json.Unmarshal(raw, &field) != nil || field.Format != "" ||
		field.MinLength != nil || field.MaxLength != nil || len(field.OneOf) > 0 || len(field.AnyOf) > 0 ||
		len(field.Enum) == 1 || len(field.Enum) > 6 {
		return types.UserInputQuestion{}, false
	}
	question := types.UserInputQuestion{ID: id, Header: field.Title, Prompt: field.Description, IsOther: len(field.Enum) == 0}
	if question.Header == "" {
		question.Header = id
	}
	if question.Prompt == "" {
		question.Prompt = prompt
	}
	for _, option := range field.Enum {
		if option == "" || slices.ContainsFunc(question.Options, func(existing types.UserInputOption) bool { return existing.OptionID == option }) {
			return types.UserInputQuestion{}, false
		}
		question.Options = append(question.Options, types.UserInputOption{OptionID: option, Label: option})
	}
	return question, true
}

func projectAnswer(questions []types.UserInputQuestion, answer *types.UserInputResponse) wire.CreateElicitationResponse {
	cancel := wire.CreateElicitationResponse{Action: wire.ElicitationCancel}
	if answer.Rejected {
		return wire.CreateElicitationResponse{Action: wire.ElicitationDecline}
	}
	if len(answer.Answers) != len(questions) {
		return cancel
	}
	content := make(map[string]json.RawMessage, len(questions))
	for _, question := range questions {
		value, ok := answer.Answers[question.ID]
		if !ok || len(value.Attachments) > 0 {
			return cancel
		}
		text := value.CustomText
		if question.IsOther {
			if len(value.OptionIDs) != 0 || text == "" {
				return cancel
			}
		} else {
			if len(value.OptionIDs) != 1 || text != "" || !slices.ContainsFunc(question.Options, func(option types.UserInputOption) bool {
				return option.OptionID == value.OptionIDs[0]
			}) {
				return cancel
			}
			text = value.OptionIDs[0]
		}
		content[question.ID], _ = json.Marshal(text)
	}
	return wire.CreateElicitationResponse{Action: wire.ElicitationAccept, Content: content}
}
