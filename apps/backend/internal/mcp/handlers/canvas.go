package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// CanvasAgentContext is derived from the stream that carried the MCP call.
// None of these fields are accepted from a canvas tool payload.
type CanvasAgentContext struct {
	ExecutionID string
	TaskID      string
	SessionID   string
}

type CanvasListRequest struct {
	Agent CanvasAgentContext
}

type CanvasReadSkillRequest struct {
	Agent CanvasAgentContext
	Path  string
}

type CanvasCreateRequest struct {
	Agent   CanvasAgentContext
	Title   string
	Summary string
}

type CanvasGetRequest struct {
	Agent    CanvasAgentContext
	CanvasID string
}

type CanvasPublishRequest struct {
	Agent      CanvasAgentContext
	CanvasID   string
	SourcePath string
}

type CanvasGetStateRequest struct {
	Agent    CanvasAgentContext
	CanvasID string
	Key      string
}

type CanvasSetStateRequest struct {
	Agent            CanvasAgentContext
	CanvasID         string
	Key              string
	Value            json.RawMessage
	ExpectedRevision *int64
}

// CanvasAuthoringService is the narrow backend boundary for task-local canvas
// operations. Implementations must resolve workspace, owner, edit target,
// source root, release validation, and state authorization from Agent and
// server-side records. The MCP payload is never an authorization boundary.
type CanvasAuthoringService interface {
	ListCanvases(context.Context, CanvasListRequest) (any, error)
	ReadCanvasAuthoringSkill(context.Context, CanvasReadSkillRequest) (any, error)
	CreateCanvas(context.Context, CanvasCreateRequest) (any, error)
	GetCanvas(context.Context, CanvasGetRequest) (any, error)
	PublishCanvas(context.Context, CanvasPublishRequest) (any, error)
	GetCanvasState(context.Context, CanvasGetStateRequest) (any, error)
	SetCanvasState(context.Context, CanvasSetStateRequest) (any, error)
}

// CanvasAuthoringError lets the task-06 canvas service return a stable safe
// error code without exposing implementation details through the MCP bridge.
type CanvasAuthoringError struct {
	Code    string
	Message string
	Details map[string]interface{}
}

func (e *CanvasAuthoringError) Error() string {
	if e == nil {
		return "canvas operation failed"
	}
	return e.Message
}

func (h *Handlers) registerCanvasHandlers(d mcpActionRegistrar) {
	if h.canvasAuthoringSvc == nil {
		return
	}
	d.RegisterFunc(ws.ActionMCPListCanvases, h.handleListCanvases)
	d.RegisterFunc(ws.ActionMCPReadCanvasAuthoringSkill, h.handleReadCanvasAuthoringSkill)
	d.RegisterFunc(ws.ActionMCPCreateCanvas, h.handleCreateCanvas)
	d.RegisterFunc(ws.ActionMCPGetCanvas, h.handleGetCanvas)
	d.RegisterFunc(ws.ActionMCPPublishCanvas, h.handlePublishCanvas)
	d.RegisterFunc(ws.ActionMCPGetCanvasState, h.handleGetCanvasState)
	d.RegisterFunc(ws.ActionMCPSetCanvasState, h.handleSetCanvasState)
}

func (h *Handlers) handleListCanvases(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	_, agent, response, err := h.canvasRequestContext(ctx, msg)
	if response != nil || err != nil {
		return response, err
	}
	result, err := h.canvasAuthoringSvc.ListCanvases(ctx, CanvasListRequest{Agent: agent})
	return h.canvasServiceResponse(msg, result, err)
}

func (h *Handlers) handleReadCanvasAuthoringSkill(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	fields, agent, response, err := h.canvasRequestContext(ctx, msg)
	if response != nil || err != nil {
		return response, err
	}
	path, err := optionalCanvasString(fields, "path")
	if err != nil {
		return canvasBadRequest(msg, err.Error())
	}
	result, err := h.canvasAuthoringSvc.ReadCanvasAuthoringSkill(ctx, CanvasReadSkillRequest{Agent: agent, Path: path})
	return h.canvasServiceResponse(msg, result, err)
}

func (h *Handlers) handleCreateCanvas(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	fields, agent, response, err := h.canvasRequestContext(ctx, msg)
	if response != nil || err != nil {
		return response, err
	}
	title, err := requiredCanvasString(fields, "title")
	if err != nil {
		return canvasBadRequest(msg, err.Error())
	}
	summary, err := requiredCanvasString(fields, "summary")
	if err != nil {
		return canvasBadRequest(msg, err.Error())
	}
	result, err := h.canvasAuthoringSvc.CreateCanvas(ctx, CanvasCreateRequest{
		Agent: agent, Title: title, Summary: summary,
	})
	return h.canvasServiceResponse(msg, result, err)
}

func (h *Handlers) handleGetCanvas(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	fields, agent, response, err := h.canvasRequestContext(ctx, msg)
	if response != nil || err != nil {
		return response, err
	}
	canvasID, err := requiredCanvasString(fields, "canvas_id")
	if err != nil {
		return canvasBadRequest(msg, err.Error())
	}
	result, err := h.canvasAuthoringSvc.GetCanvas(ctx, CanvasGetRequest{Agent: agent, CanvasID: canvasID})
	return h.canvasServiceResponse(msg, result, err)
}

func (h *Handlers) handlePublishCanvas(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	fields, agent, response, err := h.canvasRequestContext(ctx, msg)
	if response != nil || err != nil {
		return response, err
	}
	canvasID, err := requiredCanvasString(fields, "canvas_id")
	if err != nil {
		return canvasBadRequest(msg, err.Error())
	}
	sourcePath, err := requiredCanvasString(fields, "source_path")
	if err != nil {
		return canvasBadRequest(msg, err.Error())
	}
	result, err := h.canvasAuthoringSvc.PublishCanvas(ctx, CanvasPublishRequest{
		Agent: agent, CanvasID: canvasID, SourcePath: sourcePath,
	})
	return h.canvasServiceResponse(msg, result, err)
}

func (h *Handlers) handleGetCanvasState(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	fields, agent, response, err := h.canvasRequestContext(ctx, msg)
	if response != nil || err != nil {
		return response, err
	}
	canvasID, err := requiredCanvasString(fields, "canvas_id")
	if err != nil {
		return canvasBadRequest(msg, err.Error())
	}
	key, err := optionalCanvasString(fields, "key")
	if err != nil {
		return canvasBadRequest(msg, err.Error())
	}
	result, err := h.canvasAuthoringSvc.GetCanvasState(ctx, CanvasGetStateRequest{
		Agent: agent, CanvasID: canvasID, Key: key,
	})
	return h.canvasServiceResponse(msg, result, err)
}

func (h *Handlers) handleSetCanvasState(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	fields, agent, response, err := h.canvasRequestContext(ctx, msg)
	if response != nil || err != nil {
		return response, err
	}
	canvasID, err := requiredCanvasString(fields, "canvas_id")
	if err != nil {
		return canvasBadRequest(msg, err.Error())
	}
	key, err := requiredCanvasString(fields, "key")
	if err != nil {
		return canvasBadRequest(msg, err.Error())
	}
	value, ok := fields["value"]
	if !ok {
		return canvasBadRequest(msg, "value is required")
	}
	revision, err := optionalCanvasRevision(fields)
	if err != nil {
		return canvasBadRequest(msg, err.Error())
	}
	result, err := h.canvasAuthoringSvc.SetCanvasState(ctx, CanvasSetStateRequest{
		Agent: agent, CanvasID: canvasID, Key: key, Value: value, ExpectedRevision: revision,
	})
	return h.canvasServiceResponse(msg, result, err)
}

func (h *Handlers) canvasRequestContext(ctx context.Context, msg *ws.Message) (map[string]json.RawMessage, CanvasAgentContext, *ws.Message, error) {
	fields, response, err := canvasPayloadFields(msg)
	if response != nil || err != nil {
		return nil, CanvasAgentContext{}, response, err
	}
	if err := rejectCanvasIdentityFields(fields); err != nil {
		response, responseErr := canvasBadRequest(msg, err.Error())
		return nil, CanvasAgentContext{}, response, responseErr
	}
	agent, response, err := h.canvasAgentContext(ctx, msg)
	return fields, agent, response, err
}

func canvasPayloadFields(msg *ws.Message) (map[string]json.RawMessage, *ws.Message, error) {
	fields := make(map[string]json.RawMessage)
	if len(msg.Payload) == 0 || string(msg.Payload) == "null" {
		return fields, nil, nil
	}
	if err := json.Unmarshal(msg.Payload, &fields); err != nil {
		response, responseErr := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "invalid request: "+err.Error(), nil)
		return nil, response, responseErr
	}
	if fields == nil {
		fields = make(map[string]json.RawMessage)
	}
	return fields, nil, nil
}

func rejectCanvasIdentityFields(fields map[string]json.RawMessage) error {
	for _, field := range []string{"task_id", "session_id", "workspace_id", "user_id", "owner_id", "execution_id"} {
		if _, present := fields[field]; present {
			return fmt.Errorf("%s must be derived from the running execution", field)
		}
	}
	return nil
}

func requiredCanvasString(fields map[string]json.RawMessage, name string) (string, error) {
	value, err := optionalCanvasString(fields, name)
	if err != nil {
		return "", err
	}
	if value == "" {
		return "", fmt.Errorf("%s is required", name)
	}
	return value, nil
}

func optionalCanvasString(fields map[string]json.RawMessage, name string) (string, error) {
	raw, ok := fields[name]
	if !ok {
		return "", nil
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", fmt.Errorf("%s must be a string", name)
	}
	return value, nil
}

func optionalCanvasRevision(fields map[string]json.RawMessage) (*int64, error) {
	raw, ok := fields["expected_revision"]
	if !ok || string(raw) == "null" {
		return nil, nil
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value interface{}
	if err := decoder.Decode(&value); err != nil {
		return nil, errors.New("expected_revision must be a non-negative integer")
	}
	number, ok := value.(json.Number)
	if !ok {
		return nil, errors.New("expected_revision must be a non-negative integer")
	}
	revision, err := number.Int64()
	if err != nil || revision < 0 {
		return nil, errors.New("expected_revision must be a non-negative integer")
	}
	return &revision, nil
}

func (h *Handlers) canvasAgentContext(ctx context.Context, msg *ws.Message) (CanvasAgentContext, *ws.Message, error) {
	execution, ok := streams.MCPExecutionContextFromContext(ctx)
	if !ok {
		response, err := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeUnauthorized,
			"canvas authoring requires a bound task execution", nil)
		return CanvasAgentContext{}, response, err
	}
	return CanvasAgentContext{
		ExecutionID: execution.ExecutionID,
		TaskID:      execution.TaskID,
		SessionID:   execution.SessionID,
	}, nil, nil
}

func canvasBadRequest(msg *ws.Message, message string) (*ws.Message, error) {
	return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, message, nil)
}

func (h *Handlers) canvasServiceResponse(msg *ws.Message, result any, operationErr error) (*ws.Message, error) {
	if operationErr != nil {
		var canvasErr *CanvasAuthoringError
		if errors.As(operationErr, &canvasErr) && canvasErr != nil {
			code := canvasErr.Code
			if code == "" {
				code = ws.ErrorCodeInternalError
			}
			return ws.NewError(msg.ID, msg.Action, code, canvasErr.Message, canvasErr.Details)
		}
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError,
			"canvas operation failed: "+operationErr.Error(), nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, result)
}
