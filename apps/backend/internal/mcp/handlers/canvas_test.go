package handlers

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

type canvasAuthoringServiceFake struct {
	create CanvasCreateRequest
	read   []CanvasReadSkillRequest
	called bool
}

func (f *canvasAuthoringServiceFake) ListCanvases(context.Context, CanvasListRequest) (any, error) {
	return map[string]any{"canvases": []any{}}, nil
}

func (f *canvasAuthoringServiceFake) ReadCanvasAuthoringSkill(_ context.Context, request CanvasReadSkillRequest) (any, error) {
	f.read = append(f.read, request)
	return map[string]any{"path": "SKILL.md"}, nil
}

func (f *canvasAuthoringServiceFake) CreateCanvas(_ context.Context, request CanvasCreateRequest) (any, error) {
	f.called = true
	f.create = request
	return map[string]any{"canvas_id": "canvas-1"}, nil
}

func (f *canvasAuthoringServiceFake) GetCanvas(context.Context, CanvasGetRequest) (any, error) {
	return map[string]any{"canvas_id": "canvas-1"}, nil
}

func (f *canvasAuthoringServiceFake) PublishCanvas(context.Context, CanvasPublishRequest) (any, error) {
	return map[string]any{"published": true}, nil
}

func (f *canvasAuthoringServiceFake) GetCanvasState(context.Context, CanvasGetStateRequest) (any, error) {
	return map[string]any{"revision": 1}, nil
}

func (f *canvasAuthoringServiceFake) SetCanvasState(context.Context, CanvasSetStateRequest) (any, error) {
	return map[string]any{"revision": 2}, nil
}

func newCanvasHandlersForTest(t *testing.T, service CanvasAuthoringService) (*Handlers, *ws.Dispatcher) {
	t.Helper()
	h := NewHandlers(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, testLogger(t))
	h.SetCanvasAuthoringService(service)
	d := ws.NewDispatcher()
	h.RegisterHandlers(d)
	return h, d
}

func canvasExecutionContext() context.Context {
	return streams.WithMCPExecutionContext(context.Background(), streams.MCPExecutionContext{
		ExecutionID: "execution-1",
		TaskID:      "task-1",
		SessionID:   "session-1",
	})
}

func TestCanvasHandlers_AreAbsentWithoutService(t *testing.T) {
	_, dispatcher := newCanvasHandlersForTest(t, nil)

	require.False(t, dispatcher.HasHandler(ws.ActionMCPCreateCanvas))
	require.False(t, dispatcher.HasHandler(ws.ActionMCPPublishCanvas))
}

func TestCanvasHandlers_UseTrustedExecutionContext(t *testing.T) {
	fake := &canvasAuthoringServiceFake{}
	_, dispatcher := newCanvasHandlersForTest(t, fake)
	message := makeWSMessage(t, ws.ActionMCPCreateCanvas, map[string]any{
		"title":   "Release dashboard",
		"summary": "Shows deployment health.",
	})

	response, err := dispatcher.Dispatch(canvasExecutionContext(), message)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, response.Type)
	require.True(t, fake.called)
	require.Equal(t, "task-1", fake.create.Agent.TaskID)
	require.Equal(t, "session-1", fake.create.Agent.SessionID)
	require.Equal(t, "execution-1", fake.create.Agent.ExecutionID)
	require.Equal(t, "Release dashboard", fake.create.Title)
	require.Equal(t, "Shows deployment health.", fake.create.Summary)
}

func TestCanvasHandlers_RejectCallerSuppliedExecutionIdentity(t *testing.T) {
	fake := &canvasAuthoringServiceFake{}
	_, dispatcher := newCanvasHandlersForTest(t, fake)
	message := makeWSMessage(t, ws.ActionMCPCreateCanvas, map[string]any{
		"title":      "Release dashboard",
		"summary":    "Shows deployment health.",
		"task_id":    "task-foreign",
		"session_id": "session-foreign",
	})

	response, err := dispatcher.Dispatch(canvasExecutionContext(), message)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeError, response.Type)
	var payload ws.ErrorPayload
	require.NoError(t, json.Unmarshal(response.Payload, &payload))
	require.Equal(t, ws.ErrorCodeBadRequest, payload.Code)
	require.False(t, fake.called)
}

func TestCanvasHandlers_RequireBoundExecution(t *testing.T) {
	fake := &canvasAuthoringServiceFake{}
	_, dispatcher := newCanvasHandlersForTest(t, fake)
	message := makeWSMessage(t, ws.ActionMCPListCanvases, map[string]any{})

	response, err := dispatcher.Dispatch(context.Background(), message)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeError, response.Type)
	var payload ws.ErrorPayload
	require.NoError(t, json.Unmarshal(response.Payload, &payload))
	require.Equal(t, ws.ErrorCodeUnauthorized, payload.Code)
}

func TestCanvasHandlers_ReadAuthoringSkillUsesOnePathlessCoreCall(t *testing.T) {
	fake := &canvasAuthoringServiceFake{}
	_, dispatcher := newCanvasHandlersForTest(t, fake)
	message := makeWSMessage(t, ws.ActionMCPReadCanvasAuthoringSkill, map[string]any{})

	response, err := dispatcher.Dispatch(canvasExecutionContext(), message)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, response.Type)
	require.Len(t, fake.read, 1)
	require.Empty(t, fake.read[0].Path)
}
