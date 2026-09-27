package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type guardedTTYServiceStub struct {
	request streams.GuardedTTYExecRequest
	receipt *streams.GuardedTTYExecReceipt
	err     error
}

func (s *guardedTTYServiceStub) ExecuteGuardedTTY(
	_ context.Context,
	request streams.GuardedTTYExecRequest,
) (*streams.GuardedTTYExecReceipt, error) {
	s.request = request
	return s.receipt, s.err
}

func TestHandleGuardedTTYExecUsesTrustedExecutionIdentity(t *testing.T) {
	completedAt := time.Date(2026, 8, 30, 12, 0, 1, 0, time.UTC)
	service := &guardedTTYServiceStub{receipt: &streams.GuardedTTYExecReceipt{
		AttestationID: "attestation-1",
		ExecutionID:   "execution-1",
		TaskID:        "task-1",
		SessionID:     "session-1",
		Method:        "command/exec",
		RequestedTTY:  true,
		DispatchedTTY: true,
		ExitCode:      0,
		CompletedAt:   completedAt,
	}}
	h := &Handlers{guardedTTYService: service, logger: testLogger(t).WithFields()}
	ctx := guardedTTYContext("execution-1", "task-1", "session-1")

	resp, err := h.handleGuardedTTYExec(ctx, makeWSMessage(t, ws.ActionMCPGuardedTTYExec, map[string]interface{}{
		"task_id":    "task-1",
		"session_id": "session-1",
		"argv":       []string{"sh", "-lc", "test -t 0 && test -t 1"},
	}))

	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, resp.Type)
	assert.Equal(t, streams.GuardedTTYExecRequest{
		Execution: streams.MCPExecutionContext{ExecutionID: "execution-1", TaskID: "task-1", SessionID: "session-1"},
		Argv:      []string{"sh", "-lc", "test -t 0 && test -t 1"},
	}, service.request)
	var got streams.GuardedTTYExecReceipt
	require.NoError(t, json.Unmarshal(resp.Payload, &got))
	assert.Equal(t, "attestation-1", got.AttestationID)
	assert.True(t, got.DispatchedTTY)
}

func TestHandleGuardedTTYExecDeniesUnboundOrMismatchedIdentity(t *testing.T) {
	tests := []struct {
		name    string
		ctx     context.Context
		payload map[string]interface{}
	}{
		{
			name: "no trusted execution",
			ctx: mcpscope.WithPrincipal(context.Background(), mcpscope.Principal{
				CallerTaskID: "task-1", CallerSessionID: "session-1", Surface: mcpprofile.SurfaceKanbanTask,
			}),
			payload: map[string]interface{}{"task_id": "task-1", "session_id": "session-1", "argv": []string{"stty"}},
		},
		{
			name:    "payload task mismatch",
			ctx:     guardedTTYContext("execution-1", "task-1", "session-1"),
			payload: map[string]interface{}{"task_id": "task-other", "session_id": "session-1", "argv": []string{"stty"}},
		},
		{
			name: "principal session mismatch",
			ctx: streams.WithMCPExecutionContext(
				mcpscope.WithPrincipal(context.Background(), mcpscope.Principal{
					CallerTaskID: "task-1", CallerSessionID: "session-other", Surface: mcpprofile.SurfaceKanbanTask,
				}),
				streams.MCPExecutionContext{ExecutionID: "execution-1", TaskID: "task-1", SessionID: "session-1"},
			),
			payload: map[string]interface{}{"task_id": "task-1", "session_id": "session-1", "argv": []string{"stty"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := &guardedTTYServiceStub{}
			h := &Handlers{guardedTTYService: service, logger: testLogger(t).WithFields()}

			resp, err := h.handleGuardedTTYExec(tt.ctx, makeWSMessage(t, ws.ActionMCPGuardedTTYExec, tt.payload))

			require.NoError(t, err)
			assert.Equal(t, ws.MessageTypeError, resp.Type)
			assert.Contains(t, string(resp.Payload), ws.ErrorCodeForbidden)
			assert.Empty(t, service.request.Execution.ExecutionID, "denied request must not dispatch")
		})
	}
}

func TestHandleGuardedTTYExecHidesServiceError(t *testing.T) {
	service := &guardedTTYServiceStub{err: errors.New("provider leaked SECRET_CANARY")}
	h := &Handlers{guardedTTYService: service, logger: testLogger(t).WithFields()}

	resp, err := h.handleGuardedTTYExec(
		guardedTTYContext("execution-1", "task-1", "session-1"),
		makeWSMessage(t, ws.ActionMCPGuardedTTYExec, map[string]interface{}{
			"task_id": "task-1", "session_id": "session-1", "argv": []string{"stty"},
		}),
	)

	require.NoError(t, err)
	assert.Equal(t, ws.MessageTypeError, resp.Type)
	assert.NotContains(t, string(resp.Payload), "SECRET_CANARY")
}

func TestHandleGuardedTTYExecRejectsUnboundedOrSecurityFields(t *testing.T) {
	tooMany := make([]string, streams.GuardedTTYMaxArgCount+1)
	for i := range tooMany {
		tooMany[i] = "x"
	}
	tests := []struct {
		name    string
		payload map[string]interface{}
	}{
		{name: "empty argv", payload: map[string]interface{}{
			"task_id": "task-1", "session_id": "session-1", "argv": []string{},
		}},
		{name: "empty element", payload: map[string]interface{}{
			"task_id": "task-1", "session_id": "session-1", "argv": []string{"stty", ""},
		}},
		{name: "too many elements", payload: map[string]interface{}{
			"task_id": "task-1", "session_id": "session-1", "argv": tooMany,
		}},
		{name: "element too large", payload: map[string]interface{}{
			"task_id": "task-1", "session_id": "session-1", "argv": []string{string(make([]byte, streams.GuardedTTYMaxSingleArgBytes+1))},
		}},
		{name: "security field", payload: map[string]interface{}{
			"task_id": "task-1", "session_id": "session-1", "argv": []string{"stty"}, "cwd": "/host",
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := &guardedTTYServiceStub{}
			h := &Handlers{guardedTTYService: service, logger: testLogger(t).WithFields()}

			resp, err := h.handleGuardedTTYExec(
				guardedTTYContext("execution-1", "task-1", "session-1"),
				makeWSMessage(t, ws.ActionMCPGuardedTTYExec, tt.payload),
			)

			require.NoError(t, err)
			assert.Equal(t, ws.MessageTypeError, resp.Type)
			assert.Empty(t, service.request.Execution.ExecutionID, "invalid request must not dispatch")
		})
	}
}

func guardedTTYContext(executionID, taskID, sessionID string) context.Context {
	ctx := mcpscope.WithPrincipal(context.Background(), mcpscope.Principal{
		WorkspaceID: "workspace-1", CallerTaskID: taskID, CallerSessionID: sessionID, Surface: mcpprofile.SurfaceKanbanTask,
	})
	return streams.WithMCPExecutionContext(ctx, streams.MCPExecutionContext{
		ExecutionID: executionID, TaskID: taskID, SessionID: sessionID,
	})
}
