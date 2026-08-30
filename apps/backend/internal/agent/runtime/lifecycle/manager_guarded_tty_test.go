package lifecycle

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecuteGuardedTTYRoutesOnlyToExactReadyCodexExecution(t *testing.T) {
	var dispatches atomic.Int32
	client := newGuardedTTYLifecycleClient(t, func(msg ws.Message) *ws.Message {
		dispatches.Add(1)
		var request streams.GuardedTTYAgentRequest
		require.NoError(t, msg.ParsePayload(&request))
		response, err := ws.NewResponse(msg.ID, msg.Action, streams.GuardedTTYExecReceipt{
			AttestationID: request.AttestationID, ContractVersion: 1, BridgeVersion: "1",
			ExecutionID: request.ExecutionID, TaskID: request.TaskID, SessionID: request.SessionID,
			ACPSessionID: "acp-session-1",
			Method:       streams.GuardedTTYExecMethod, RequestedTTY: true, DispatchedTTY: true,
			ProcessID: "process-1", CWD: "/workspace/task", Output: "ready\n", ExitCode: 0,
			Outcome: "succeeded", CompletionCount: 1, CompletedAt: time.Now().UTC(),
		})
		require.NoError(t, err)
		return response
	})
	manager := &Manager{executionStore: NewExecutionStore()}
	execution := &AgentExecution{
		ID: "execution-1", TaskID: "task-1", SessionID: "session-1", AgentID: "codex-acp",
		ACPSessionID: "acp-session-1", Status: v1.AgentStatusReady, agentctl: client,
		sessionInitialized: true,
	}
	execution.MarkAgentctlReady()
	require.NoError(t, manager.executionStore.Add(execution))
	request := models.GuardedTTYDispatchRequest{
		AttestationID: "attestation-1",
		Execution:     streams.MCPExecutionContext{ExecutionID: "execution-1", TaskID: "task-1", SessionID: "session-1"},
		Argv:          []string{"stty"},
	}

	receipt, err := manager.ExecuteGuardedTTY(context.Background(), request)

	require.NoError(t, err)
	require.NotNil(t, receipt)
	assert.Equal(t, int32(1), dispatches.Load())
	assert.Equal(t, "attestation-1", receipt.AttestationID)
}

func TestExecuteGuardedTTYRejectsStaleOrIneligibleExecutionBeforeDispatch(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*AgentExecution, *models.GuardedTTYDispatchRequest)
	}{
		{name: "wrong task", mutate: func(_ *AgentExecution, r *models.GuardedTTYDispatchRequest) { r.Execution.TaskID = "other" }},
		{name: "wrong session", mutate: func(_ *AgentExecution, r *models.GuardedTTYDispatchRequest) { r.Execution.SessionID = "other" }},
		{name: "non codex", mutate: func(e *AgentExecution, _ *models.GuardedTTYDispatchRequest) { e.AgentID = "claude-acp" }},
		{name: "not ready", mutate: func(e *AgentExecution, _ *models.GuardedTTYDispatchRequest) { e.Status = v1.AgentStatusRunning }},
		{name: "session not initialized", mutate: func(e *AgentExecution, _ *models.GuardedTTYDispatchRequest) { e.sessionInitialized = false }},
		{name: "agentctl not ready", mutate: func(e *AgentExecution, _ *models.GuardedTTYDispatchRequest) { e.agentctlReady.Store(false) }},
		{name: "acp session missing", mutate: func(e *AgentExecution, _ *models.GuardedTTYDispatchRequest) { e.ACPSessionID = "" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var dispatches atomic.Int32
			client := newGuardedTTYLifecycleClient(t, func(msg ws.Message) *ws.Message {
				dispatches.Add(1)
				response, _ := ws.NewResponse(msg.ID, msg.Action, streams.GuardedTTYExecReceipt{})
				return response
			})
			manager := &Manager{executionStore: NewExecutionStore()}
			execution := &AgentExecution{
				ID: "execution-1", TaskID: "task-1", SessionID: "session-1", AgentID: "codex-acp",
				ACPSessionID: "acp-session-1", Status: v1.AgentStatusReady, agentctl: client,
				sessionInitialized: true,
			}
			execution.MarkAgentctlReady()
			require.NoError(t, manager.executionStore.Add(execution))
			request := models.GuardedTTYDispatchRequest{
				AttestationID: "attestation-1",
				Execution:     streams.MCPExecutionContext{ExecutionID: "execution-1", TaskID: "task-1", SessionID: "session-1"},
				Argv:          []string{"stty"},
			}
			test.mutate(execution, &request)

			receipt, err := manager.ExecuteGuardedTTY(context.Background(), request)

			require.Error(t, err)
			assert.Nil(t, receipt)
			assert.Equal(t, int32(0), dispatches.Load())
		})
	}
}

func TestExecuteGuardedTTYRejectsReceiptForDifferentACPSession(t *testing.T) {
	client := newGuardedTTYLifecycleClient(t, func(msg ws.Message) *ws.Message {
		var request streams.GuardedTTYAgentRequest
		require.NoError(t, msg.ParsePayload(&request))
		response, err := ws.NewResponse(msg.ID, msg.Action, streams.GuardedTTYExecReceipt{
			AttestationID: request.AttestationID, ExecutionID: request.ExecutionID,
			TaskID: request.TaskID, SessionID: request.SessionID, ACPSessionID: "replaced-acp-session",
		})
		require.NoError(t, err)
		return response
	})
	manager := &Manager{executionStore: NewExecutionStore()}
	execution := &AgentExecution{
		ID: "execution-1", TaskID: "task-1", SessionID: "session-1", AgentID: "codex-acp",
		ACPSessionID: "acp-session-1", Status: v1.AgentStatusReady, agentctl: client,
		sessionInitialized: true,
	}
	execution.MarkAgentctlReady()
	require.NoError(t, manager.executionStore.Add(execution))

	receipt, err := manager.ExecuteGuardedTTY(context.Background(), models.GuardedTTYDispatchRequest{
		AttestationID: "attestation-1",
		Execution:     streams.MCPExecutionContext{ExecutionID: "execution-1", TaskID: "task-1", SessionID: "session-1"},
		Argv:          []string{"stty"},
	})

	require.Error(t, err)
	assert.Nil(t, receipt)
}

func TestExecuteGuardedTTYSerializesAgainstExecutionReplacement(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	client := newGuardedTTYLifecycleClient(t, func(msg ws.Message) *ws.Message {
		var request streams.GuardedTTYAgentRequest
		require.NoError(t, msg.ParsePayload(&request))
		close(entered)
		<-release
		response, err := ws.NewResponse(msg.ID, msg.Action, streams.GuardedTTYExecReceipt{
			AttestationID: request.AttestationID, ExecutionID: request.ExecutionID,
			TaskID: request.TaskID, SessionID: request.SessionID, ACPSessionID: "acp-session-1",
		})
		require.NoError(t, err)
		return response
	})
	manager := &Manager{executionStore: NewExecutionStore()}
	execution := &AgentExecution{
		ID: "execution-1", TaskID: "task-1", SessionID: "session-1", AgentID: "codex-acp",
		ACPSessionID: "acp-session-1", Status: v1.AgentStatusReady, agentctl: client,
		sessionInitialized: true,
	}
	execution.MarkAgentctlReady()
	require.NoError(t, manager.executionStore.Add(execution))
	result := make(chan error, 1)
	go func() {
		_, err := manager.ExecuteGuardedTTY(context.Background(), models.GuardedTTYDispatchRequest{
			AttestationID: "attestation-1",
			Execution:     streams.MCPExecutionContext{ExecutionID: "execution-1", TaskID: "task-1", SessionID: "session-1"},
			Argv:          []string{"stty"},
		})
		result <- err
	}()
	<-entered
	assert.False(t, execution.guardedTTYMu.TryLock(), "replacement lock acquired during guarded dispatch")
	close(release)
	require.NoError(t, <-result)
	require.True(t, execution.guardedTTYMu.TryLock(), "replacement lock remained held after completion")
	execution.guardedTTYMu.Unlock()
}

func newGuardedTTYLifecycleClient(t *testing.T, handler func(ws.Message) *ws.Message) *agentctl.Client {
	t.Helper()
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		connection, err := upgrader.Upgrade(writer, request, nil)
		if err != nil {
			return
		}
		defer func() { _ = connection.Close() }()
		for {
			_, data, readErr := connection.ReadMessage()
			if readErr != nil {
				return
			}
			var message ws.Message
			if json.Unmarshal(data, &message) != nil {
				continue
			}
			response := handler(message)
			if response == nil {
				continue
			}
			encoded, _ := json.Marshal(response)
			_ = connection.WriteMessage(websocket.TextMessage, encoded)
		}
	}))
	t.Cleanup(server.Close)
	client := createTestClient(t, server.URL)
	require.NoError(t, client.StreamUpdates(context.Background(), func(agentctl.AgentEvent) {}, nil, nil))
	t.Cleanup(client.Close)
	return client
}
