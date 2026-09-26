package handlers

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/agent/controller"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func TestAgentHandlersRegisterEveryAction(t *testing.T) {
	h := NewHandlers(nil, newTestLogger())
	dispatcher := ws.NewDispatcher()
	h.RegisterHandlers(dispatcher)
	for _, action := range []string{
		ws.ActionAgentList,
		ws.ActionAgentLaunch,
		ws.ActionAgentStatus,
		ws.ActionAgentLogs,
		ws.ActionAgentStop,
		ws.ActionAgentTypes,
	} {
		if !dispatcher.HasHandler(action) {
			t.Fatalf("handler %s was not registered", action)
		}
	}
}

func TestAgentHandlersListStatusLogsAndTypes(t *testing.T) {
	mgr := newTestManager()
	execution := &lifecycle.AgentExecution{
		ID:             "agent-1",
		TaskID:         "task-1",
		AgentProfileID: "profile-1",
		Status:         v1.AgentStatusRunning,
	}
	if err := mgr.ExecutionStoreForTesting().Add(execution); err != nil {
		t.Fatalf("add execution: %v", err)
	}
	h := NewHandlers(controller.NewController(mgr, newTestRegistry()), newTestLogger())

	list, err := h.wsListAgents(context.Background(), agentHandlerMessage(t, ws.ActionAgentList, map[string]any{}))
	if err != nil {
		t.Fatalf("wsListAgents: %v", err)
	}
	var listPayload struct {
		Total  int `json:"total"`
		Agents []struct {
			ID     string `json:"id"`
			TaskID string `json:"task_id"`
		} `json:"agents"`
	}
	decodeHandlerPayload(t, list, &listPayload)
	if listPayload.Total != 1 || len(listPayload.Agents) != 1 || listPayload.Agents[0].ID != "agent-1" || listPayload.Agents[0].TaskID != "task-1" {
		t.Fatalf("list payload = %#v", listPayload)
	}

	status, err := h.wsGetAgentStatus(context.Background(), agentHandlerMessage(t, ws.ActionAgentStatus, map[string]any{"agent_id": "agent-1"}))
	if err != nil {
		t.Fatalf("wsGetAgentStatus: %v", err)
	}
	var statusPayload struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	decodeHandlerPayload(t, status, &statusPayload)
	if statusPayload.ID != "agent-1" || statusPayload.Status != string(v1.AgentStatusRunning) {
		t.Fatalf("status payload = %#v", statusPayload)
	}

	logs, err := h.wsGetAgentLogs(context.Background(), agentHandlerMessage(t, ws.ActionAgentLogs, map[string]any{"agent_id": "agent-1"}))
	if err != nil {
		t.Fatalf("wsGetAgentLogs: %v", err)
	}
	var logsPayload struct {
		AgentID string   `json:"agent_id"`
		Logs    []string `json:"logs"`
	}
	decodeHandlerPayload(t, logs, &logsPayload)
	if logsPayload.AgentID != "agent-1" || len(logsPayload.Logs) != 0 {
		t.Fatalf("logs payload = %#v", logsPayload)
	}

	types, err := h.wsListAgentTypes(context.Background(), agentHandlerMessage(t, ws.ActionAgentTypes, map[string]any{}))
	if err != nil {
		t.Fatalf("wsListAgentTypes: %v", err)
	}
	var typesPayload struct {
		Types []any `json:"types"`
	}
	decodeHandlerPayload(t, types, &typesPayload)
	if len(typesPayload.Types) == 0 {
		t.Fatal("expected registered agent types")
	}
}

func TestAgentHandlersValidation(t *testing.T) {
	h := NewHandlers(nil, newTestLogger())
	tests := []struct {
		name   string
		action string
		invoke func(context.Context, *ws.Message) (*ws.Message, error)
		body   any
		code   string
	}{
		{name: "launch malformed", action: ws.ActionAgentLaunch, invoke: h.wsLaunchAgent, body: json.RawMessage(`{invalid`), code: ws.ErrorCodeBadRequest},
		{name: "launch task", action: ws.ActionAgentLaunch, invoke: h.wsLaunchAgent, body: map[string]any{}, code: ws.ErrorCodeValidation},
		{name: "launch profile", action: ws.ActionAgentLaunch, invoke: h.wsLaunchAgent, body: map[string]any{"task_id": "task"}, code: ws.ErrorCodeValidation},
		{name: "launch workspace", action: ws.ActionAgentLaunch, invoke: h.wsLaunchAgent, body: map[string]any{"task_id": "task", "agent_profile_id": "profile"}, code: ws.ErrorCodeValidation},
		{name: "status malformed", action: ws.ActionAgentStatus, invoke: h.wsGetAgentStatus, body: json.RawMessage(`{invalid`), code: ws.ErrorCodeBadRequest},
		{name: "status id", action: ws.ActionAgentStatus, invoke: h.wsGetAgentStatus, body: map[string]any{}, code: ws.ErrorCodeValidation},
		{name: "logs id", action: ws.ActionAgentLogs, invoke: h.wsGetAgentLogs, body: map[string]any{}, code: ws.ErrorCodeValidation},
		{name: "stop malformed", action: ws.ActionAgentStop, invoke: h.wsStopAgent, body: json.RawMessage(`{invalid`), code: ws.ErrorCodeBadRequest},
		{name: "stop id", action: ws.ActionAgentStop, invoke: h.wsStopAgent, body: map[string]any{}, code: ws.ErrorCodeValidation},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := agentHandlerMessage(t, tt.action, tt.body)
			response, err := tt.invoke(context.Background(), msg)
			if err != nil {
				t.Fatalf("handler error: %v", err)
			}
			assertWSErrorCode(t, response, tt.code)
		})
	}
}

func TestAgentHandlersMapControllerErrors(t *testing.T) {
	h := NewHandlers(controller.NewController(nil, nil), newTestLogger())
	tests := []struct {
		name   string
		action string
		invoke func(context.Context, *ws.Message) (*ws.Message, error)
		body   any
	}{
		{name: "list", action: ws.ActionAgentList, invoke: h.wsListAgents, body: map[string]any{}},
		{name: "launch", action: ws.ActionAgentLaunch, invoke: h.wsLaunchAgent, body: map[string]any{"task_id": "task", "agent_profile_id": "profile", "workspace_path": "/work"}},
		{name: "status", action: ws.ActionAgentStatus, invoke: h.wsGetAgentStatus, body: map[string]any{"agent_id": "missing"}},
		{name: "logs", action: ws.ActionAgentLogs, invoke: h.wsGetAgentLogs, body: map[string]any{"agent_id": "missing"}},
		{name: "stop", action: ws.ActionAgentStop, invoke: h.wsStopAgent, body: map[string]any{"agent_id": "missing"}},
		{name: "types", action: ws.ActionAgentTypes, invoke: h.wsListAgentTypes, body: map[string]any{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response, err := tt.invoke(context.Background(), agentHandlerMessage(t, tt.action, tt.body))
			if err != nil || response == nil {
				t.Fatalf("response = (%#v, %v)", response, err)
			}
			assertWSErrorCode(t, response, ws.ErrorCodeInternalError)
		})
	}
}

func TestAgentStatusAndLogsMapMissingAgentToNotFound(t *testing.T) {
	h := NewHandlers(controller.NewController(newTestManager(), newTestRegistry()), newTestLogger())
	for _, test := range []struct {
		action string
		invoke func(context.Context, *ws.Message) (*ws.Message, error)
	}{
		{action: ws.ActionAgentStatus, invoke: h.wsGetAgentStatus},
		{action: ws.ActionAgentLogs, invoke: h.wsGetAgentLogs},
	} {
		response, err := test.invoke(context.Background(), agentHandlerMessage(t, test.action, map[string]any{"agent_id": "missing"}))
		if err != nil {
			t.Fatalf("handler error: %v", err)
		}
		assertWSErrorCode(t, response, ws.ErrorCodeNotFound)
	}
}

func agentHandlerMessage(t *testing.T, action string, payload any) *ws.Message {
	t.Helper()
	if raw, ok := payload.(json.RawMessage); ok {
		return &ws.Message{ID: "id", Action: action, Payload: raw}
	}
	message, err := ws.NewRequest("id", action, payload)
	if err != nil {
		t.Fatal(err)
	}
	return message
}

func decodeHandlerPayload(t *testing.T, message *ws.Message, target any) {
	t.Helper()
	if err := json.Unmarshal(message.Payload, target); err != nil {
		t.Fatalf("decode response: %v", err)
	}
}
