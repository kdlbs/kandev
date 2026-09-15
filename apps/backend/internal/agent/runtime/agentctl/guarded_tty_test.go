package client

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGuardedTTYExecSendsExactBoundRequest(t *testing.T) {
	completedAt := time.Date(2026, 8, 30, 12, 0, 1, 0, time.UTC)
	var received streams.GuardedTTYAgentRequest
	client, server := newTestClientWithStream(t, func(msg ws.Message) *ws.Message {
		assert.Equal(t, streams.GuardedTTYAgentAction, msg.Action)
		require.NoError(t, msg.ParsePayload(&received))
		response, err := ws.NewResponse(msg.ID, msg.Action, streams.GuardedTTYExecReceipt{
			AttestationID:   "attestation-1",
			ContractVersion: 1,
			BridgeVersion:   "1",
			ExecutionID:     "execution-1",
			TaskID:          "task-1",
			SessionID:       "session-1",
			Method:          streams.GuardedTTYExecMethod,
			RequestedTTY:    true,
			DispatchedTTY:   true,
			ProcessID:       "process-1",
			CWD:             "/workspace/task",
			Output:          "ready\n",
			ExitCode:        0,
			Outcome:         "succeeded",
			CompletionCount: 1,
			CompletedAt:     completedAt,
		})
		require.NoError(t, err)
		return response
	})
	defer server.Close()
	defer client.Close()

	request := streams.GuardedTTYAgentRequest{
		AttestationID: "attestation-1",
		ExecutionID:   "execution-1",
		TaskID:        "task-1",
		SessionID:     "session-1",
		Argv:          []string{"sh", "-lc", "test -t 0"},
	}
	receipt, err := client.GuardedTTYExec(context.Background(), request)

	require.NoError(t, err)
	require.NotNil(t, receipt)
	assert.Equal(t, request, received)
	assert.Equal(t, "process-1", receipt.ProcessID)
	assert.Equal(t, completedAt, receipt.CompletedAt)
}

func TestGuardedTTYExecFailsClosedOnAgentctlError(t *testing.T) {
	client, server := newTestClientWithStream(t, func(msg ws.Message) *ws.Message {
		response, err := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeConflict, "guarded TTY unavailable", nil)
		require.NoError(t, err)
		return response
	})
	defer server.Close()
	defer client.Close()

	receipt, err := client.GuardedTTYExec(context.Background(), streams.GuardedTTYAgentRequest{
		AttestationID: "attestation-1",
		ExecutionID:   "execution-1",
		TaskID:        "task-1",
		SessionID:     "session-1",
		Argv:          []string{"stty"},
	})

	require.Error(t, err)
	assert.Nil(t, receipt)
	assert.False(t, errors.Is(err, ErrAgentStreamNotConnected))
}
