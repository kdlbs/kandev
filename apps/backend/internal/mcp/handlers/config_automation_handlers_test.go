package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/kandev/kandev/internal/automation"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestConfigAutomationUnavailable(t *testing.T) {
	h := &Handlers{logger: testLogger(t)}
	d := ws.NewDispatcher()
	h.RegisterHandlers(d)
	require.True(t, d.HasHandler("mcp.create_automation"))
	response, err := d.Dispatch(context.Background(), makeWSMessage(t, "mcp.create_automation", map[string]interface{}{"workspace_id": "ws", "name": "Daily"}))
	require.NoError(t, err)
	assertWSError(t, response, ws.ErrorCodeInternalError)
}

type automationCreatorStub struct {
	saved *automation.Automation
	err   error
	calls int
}

func (s *automationCreatorStub) CreateAutomation(context.Context, *automation.CreateAutomationRequest) (*automation.Automation, error) {
	s.calls++
	return s.saved, s.err
}

func TestConfigAutomationCreateErrors(t *testing.T) {
	for _, tc := range []struct {
		name    string
		payload json.RawMessage
		failure error
		code    string
		calls   int
	}{
		{name: "malformed payload", payload: json.RawMessage(`{"triggers":"bad"}`), code: ws.ErrorCodeBadRequest},
		{name: "service error", payload: json.RawMessage(`{"name":"daily"}`), failure: errors.New("name is invalid"), code: ws.ErrorCodeInternalError, calls: 1},
		{name: "missing saved record", payload: json.RawMessage(`{"name":"daily"}`), code: ws.ErrorCodeInternalError, calls: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			creator := &automationCreatorStub{err: tc.failure}
			h := &Handlers{logger: testLogger(t)}
			h.SetAutomationCreator(creator)
			d := ws.NewDispatcher()
			h.RegisterHandlers(d)
			response, err := d.Dispatch(context.Background(), &ws.Message{ID: "create", Action: ws.ActionMCPCreateAutomation, Payload: tc.payload})
			require.NoError(t, err)
			assertWSError(t, response, tc.code)
			assert.Equal(t, tc.calls, creator.calls)
		})
	}
}

// @covers AC-OFFICE-CONFIG-AUTOMATION-001.3
func TestAutomationCannotCreateAutomation(t *testing.T) {
	creator := &automationCreatorStub{saved: &automation.Automation{ID: "unexpected"}}
	h := &Handlers{logger: testLogger(t)}
	h.SetAutomationCreator(creator)
	d := ws.NewDispatcher()
	h.RegisterHandlers(d)
	ctx := mcpscope.WithPrincipal(context.Background(), mcpscope.Principal{
		AutomationID: "automation", WorkspaceID: "ws", CallerTaskID: "task", CallerSessionID: "session", Surface: mcpprofile.SurfaceAutomation,
	})
	response, err := d.Dispatch(ctx, makeWSMessage(t, ws.ActionMCPCreateAutomation, map[string]interface{}{"workspace_id": "ws", "name": "forbidden"}))
	require.NoError(t, err)
	assertWSError(t, response, ws.ErrorCodeUnknownAction)
	assert.Zero(t, creator.calls)
}

func TestConfigAutomationReturnsSavedState(t *testing.T) {
	creator := &automationCreatorStub{saved: &automation.Automation{ID: "saved", Name: "persisted", WebhookSecret: "one-time-secret"}}
	h := &Handlers{logger: testLogger(t)}
	h.SetAutomationCreator(creator)
	response, err := h.handleCreateAutomation(context.Background(), makeWSMessage(t, ws.ActionMCPCreateAutomation, map[string]interface{}{"workspace_id": "ws", "name": "requested"}))
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, response.Type)
	var saved map[string]interface{}
	require.NoError(t, json.Unmarshal(response.Payload, &saved))
	assert.Equal(t, "saved", saved["id"])
	assert.Equal(t, "persisted", saved["name"])
	assert.Equal(t, "one-time-secret", saved["webhook_secret"])
}
