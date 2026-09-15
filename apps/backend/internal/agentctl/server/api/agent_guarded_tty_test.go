package api

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type guardedTTYAdapter struct {
	bareAgentAdapter
	request   streams.GuardedTTYBridgeRequest
	receipt   *streams.GuardedTTYExecReceipt
	err       error
	calls     int
	available bool
}

func (a *guardedTTYAdapter) GuardedTTYAvailable() bool { return a.available }

func (a *guardedTTYAdapter) ExecuteGuardedTTY(
	_ context.Context,
	request streams.GuardedTTYBridgeRequest,
) (*streams.GuardedTTYExecReceipt, error) {
	a.calls++
	a.request = request
	return a.receipt, a.err
}

func TestHandleWSGuardedTTYExecBindsLaunchIdentityBeforeAdapter(t *testing.T) {
	server := newTestServer(t)
	server.cfg.InstanceID = "execution-1"
	server.cfg.TaskID = "task-1"
	server.cfg.SessionID = "session-1"
	completedAt := time.Date(2026, 8, 30, 12, 0, 1, 0, time.UTC)
	agentAdapter := &guardedTTYAdapter{bareAgentAdapter: bareAgentAdapter{sessionID: "acp-session-1"}, available: true, receipt: &streams.GuardedTTYExecReceipt{
		ContractVersion: 1,
		BridgeVersion:   "1",
		ACPSessionID:    "acp-session-1",
		Method:          streams.GuardedTTYExecMethod,
		RequestedTTY:    true,
		DispatchedTTY:   true,
		ProcessID:       "process-1",
		CWD:             server.cfg.WorkDir,
		Output:          "ready\n",
		ExitCode:        0,
		Outcome:         "succeeded",
		CompletionCount: 1,
		CompletedAt:     completedAt,
	}}
	server.procMgr.SetAdapterForTest(agentAdapter)
	request := streams.GuardedTTYAgentRequest{
		AttestationID: "attestation-1",
		ExecutionID:   "execution-1",
		TaskID:        "task-1",
		SessionID:     "session-1",
		Argv:          []string{"stty", "-a"},
	}
	msg, err := ws.NewRequest("request-1", streams.GuardedTTYAgentAction, request)
	require.NoError(t, err)

	response := server.handleAgentStreamRequest(context.Background(), msg)

	require.Equal(t, ws.MessageTypeResponse, response.Type, string(response.Payload))
	var receipt streams.GuardedTTYExecReceipt
	require.NoError(t, response.ParsePayload(&receipt))
	assert.Equal(t, 1, agentAdapter.calls)
	assert.Equal(t, streams.GuardedTTYBridgeRequest{
		SessionID: "acp-session-1",
		Argv:      []string{"stty", "-a"},
	}, agentAdapter.request)
	assert.Equal(t, "attestation-1", receipt.AttestationID)
	assert.Equal(t, "execution-1", receipt.ExecutionID)
	assert.Equal(t, "task-1", receipt.TaskID)
	assert.Equal(t, "session-1", receipt.SessionID)
}

func TestHandleWSNewSessionPublishesNegotiatedGuardedTTYAvailability(t *testing.T) {
	server := newTestServerWithMCP(t)
	server.mcpServer.SetProfile(mcpprofile.New(
		mcpprofile.SurfaceKanbanTask,
		[]mcpprofile.Capability{mcpprofile.CapabilityGuardedTTYExec},
		nil,
	))
	agentAdapter := &guardedTTYAdapter{
		bareAgentAdapter: bareAgentAdapter{sessionID: "acp-session-1"},
		available:        true,
	}
	server.procMgr.SetAdapterForTest(agentAdapter)
	msg, err := ws.NewRequest("request-new", "agent.session.new", NewSessionRequest{})
	require.NoError(t, err)

	response := server.handleAgentStreamRequest(context.Background(), msg)

	require.Equal(t, ws.MessageTypeResponse, response.Type, string(response.Payload))
	assert.True(t, server.mcpServer.GuardedTTYAvailable())
}

func TestHandleWSGuardedTTYExecRejectsIdentityMismatchWithoutDispatch(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*streams.GuardedTTYAgentRequest)
	}{
		{name: "attestation missing", mutate: func(r *streams.GuardedTTYAgentRequest) { r.AttestationID = "" }},
		{name: "execution", mutate: func(r *streams.GuardedTTYAgentRequest) { r.ExecutionID = "other" }},
		{name: "task", mutate: func(r *streams.GuardedTTYAgentRequest) { r.TaskID = "other" }},
		{name: "session", mutate: func(r *streams.GuardedTTYAgentRequest) { r.SessionID = "other" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := newTestServer(t)
			server.cfg.InstanceID = "execution-1"
			server.cfg.TaskID = "task-1"
			server.cfg.SessionID = "session-1"
			agentAdapter := &guardedTTYAdapter{}
			server.procMgr.SetAdapterForTest(agentAdapter)
			request := streams.GuardedTTYAgentRequest{
				AttestationID: "attestation-1", ExecutionID: "execution-1",
				TaskID: "task-1", SessionID: "session-1", Argv: []string{"stty"},
			}
			test.mutate(&request)
			msg, err := ws.NewRequest("request-1", streams.GuardedTTYAgentAction, request)
			require.NoError(t, err)

			response := server.handleAgentStreamRequest(context.Background(), msg)

			assert.Equal(t, ws.MessageTypeError, response.Type)
			assert.Equal(t, 0, agentAdapter.calls)
		})
	}
}

func TestHandleWSGuardedTTYExecRejectsUnsupportedAndUnknownFields(t *testing.T) {
	t.Run("unsupported adapter", func(t *testing.T) {
		server := newGuardedTTYTestServer(t)
		server.procMgr.SetAdapterForTest(&bareAgentAdapter{})
		msg, err := ws.NewRequest("request-1", streams.GuardedTTYAgentAction, guardedTTYTestRequest())
		require.NoError(t, err)
		assert.Equal(t, ws.MessageTypeError, server.handleAgentStreamRequest(context.Background(), msg).Type)
	})

	t.Run("unnegotiated adapter", func(t *testing.T) {
		server := newGuardedTTYTestServer(t)
		agentAdapter := &guardedTTYAdapter{bareAgentAdapter: bareAgentAdapter{sessionID: "acp-session-1"}}
		server.procMgr.SetAdapterForTest(agentAdapter)
		msg, err := ws.NewRequest("request-1", streams.GuardedTTYAgentAction, guardedTTYTestRequest())
		require.NoError(t, err)
		assert.Equal(t, ws.MessageTypeError, server.handleAgentStreamRequest(context.Background(), msg).Type)
		assert.Equal(t, 0, agentAdapter.calls)
	})

	t.Run("unknown security field", func(t *testing.T) {
		server := newGuardedTTYTestServer(t)
		agentAdapter := &guardedTTYAdapter{}
		server.procMgr.SetAdapterForTest(agentAdapter)
		msg, err := ws.NewRequest("request-1", streams.GuardedTTYAgentAction, map[string]any{
			"attestation_id": "attestation-1", "execution_id": "execution-1",
			"task_id": "task-1", "session_id": "session-1", "argv": []string{"stty"},
			"cwd": "/tmp/escape",
		})
		require.NoError(t, err)
		assert.Equal(t, ws.MessageTypeError, server.handleAgentStreamRequest(context.Background(), msg).Type)
		assert.Equal(t, 0, agentAdapter.calls)
	})
}

func TestHandleWSGuardedTTYExecHidesAdapterError(t *testing.T) {
	server := newGuardedTTYTestServer(t)
	agentAdapter := &guardedTTYAdapter{
		bareAgentAdapter: bareAgentAdapter{sessionID: "acp-session-1"},
		available:        true,
		err:              errors.New("SECRET_CANARY"),
	}
	server.procMgr.SetAdapterForTest(agentAdapter)
	msg, err := ws.NewRequest("request-1", streams.GuardedTTYAgentAction, guardedTTYTestRequest())
	require.NoError(t, err)

	response := server.handleAgentStreamRequest(context.Background(), msg)

	payload := errorPayload(t, response)
	assert.NotContains(t, payload.Message, "SECRET_CANARY")
	assert.Equal(t, 1, agentAdapter.calls)
}

func newGuardedTTYTestServer(t *testing.T) *Server {
	t.Helper()
	server := newTestServer(t)
	server.cfg.InstanceID = "execution-1"
	server.cfg.TaskID = "task-1"
	server.cfg.SessionID = "session-1"
	return server
}

func guardedTTYTestRequest() streams.GuardedTTYAgentRequest {
	return streams.GuardedTTYAgentRequest{
		AttestationID: "attestation-1", ExecutionID: "execution-1",
		TaskID: "task-1", SessionID: "session-1", Argv: []string{"stty"},
	}
}
