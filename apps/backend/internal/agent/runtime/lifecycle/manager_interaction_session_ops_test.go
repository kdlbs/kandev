package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"

	agentctlclient "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// sessionOpsHarness wires a Manager to a live agentctl agent stream backed by
// the mock WS server, recording every request the manager makes so tests can
// assert the exact action and payload that reached agentctl rather than merely
// that the call returned nil.
type sessionOpsHarness struct {
	mgr      *Manager
	exec     *AgentExecution
	mu       sync.Mutex
	requests []recordedStreamRequest
}

type recordedStreamRequest struct {
	action  string
	payload map[string]interface{}
	raw     []byte
}

// decode unmarshals the captured wire payload into v. Needed for payloads whose
// fields carry `omitempty` (permission responses), where the typed shape — not
// the raw map — is what agentctl actually observes.
func (r recordedStreamRequest) decode(t *testing.T, v interface{}) {
	t.Helper()
	require.NoError(t, json.Unmarshal(r.raw, v))
}

func (h *sessionOpsHarness) record(msg ws.Message) {
	payload := map[string]interface{}{}
	raw := append([]byte(nil), msg.Payload...)
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &payload)
	}
	h.mu.Lock()
	h.requests = append(h.requests, recordedStreamRequest{action: msg.Action, payload: payload, raw: raw})
	h.mu.Unlock()
}

// only returns the single recorded request, failing when the manager made a
// different number of agentctl calls than expected.
func (h *sessionOpsHarness) only(t *testing.T) recordedStreamRequest {
	t.Helper()
	h.mu.Lock()
	defer h.mu.Unlock()
	require.Len(t, h.requests, 1, "expected exactly one agentctl stream request, got %+v", h.requests)
	return h.requests[0]
}

func (h *sessionOpsHarness) count() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.requests)
}

// newSessionOpsHarness builds a manager holding one execution whose agentctl
// client has a connected agent stream. The execution is registered in the
// store under both its execution ID and session ID.
func newSessionOpsHarness(t *testing.T) *sessionOpsHarness {
	t.Helper()

	h := &sessionOpsHarness{}
	mock := newMockAgentServer(t)
	t.Cleanup(mock.Close)
	mock.handler = func(msg ws.Message) *ws.Message {
		h.record(msg)
		resp, _ := ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{"success": true})
		return resp
	}

	client := createTestClient(t, mock.server.URL)
	t.Cleanup(client.Close)
	streamCtx, streamCancel := context.WithCancel(context.Background())
	t.Cleanup(streamCancel)
	require.NoError(t, client.StreamUpdates(streamCtx, func(agentctlclient.AgentEvent) {}, nil, nil))
	select {
	case <-mock.wsConnected:
	case <-time.After(2 * time.Second):
		t.Fatal("mock agentctl server never saw the agent stream connect")
	}

	h.mgr = newTestManager(t)
	h.exec = &AgentExecution{
		ID:                 "exec-ops",
		TaskID:             "task-ops",
		SessionID:          "session-ops",
		ACPSessionID:       "acp-live",
		Status:             v1.AgentStatusReady,
		sessionInitialized: true,
		agentctl:           client,
	}
	require.NoError(t, h.mgr.executionStore.Add(h.exec))
	return h
}

// TestSetSessionModeUsesLiveACPSessionID pins the documented invariant on
// SetSessionMode: the ACP session ID sent to agentctl always comes from the
// execution, never from the caller's (possibly stale) copy.
func TestSetSessionModeUsesLiveACPSessionID(t *testing.T) {
	h := newSessionOpsHarness(t)

	require.NoError(t, h.mgr.SetSessionMode(context.Background(), "exec-ops", "acp-stale-from-caller", "plan"))

	req := h.only(t)
	require.Equal(t, "agent.session.set_mode", req.action)
	require.Equal(t, "acp-live", req.payload["session_id"],
		"SetSessionMode must send the execution's current ACP session ID, not the caller's argument")
	require.Equal(t, "plan", req.payload["mode_id"])
}

func TestSetSessionModeBySessionIDResolvesExecution(t *testing.T) {
	h := newSessionOpsHarness(t)

	require.NoError(t, h.mgr.SetSessionModeBySessionID(context.Background(), "session-ops", "accept-edits"))

	req := h.only(t)
	require.Equal(t, "agent.session.set_mode", req.action)
	require.Equal(t, "acp-live", req.payload["session_id"])
	require.Equal(t, "accept-edits", req.payload["mode_id"])
}

func TestSetSessionModeGuardsRejectBeforeReachingAgentctl(t *testing.T) {
	ctx := context.Background()

	t.Run("unknown execution", func(t *testing.T) {
		h := newSessionOpsHarness(t)
		err := h.mgr.SetSessionMode(ctx, "missing", "", "plan")
		require.ErrorContains(t, err, `execution "missing" not found`)
		require.Zero(t, h.count(), "no agentctl call may be made for an unknown execution")
	})

	t.Run("no agentctl client", func(t *testing.T) {
		h := newSessionOpsHarness(t)
		require.NoError(t, h.mgr.executionStore.Add(&AgentExecution{
			ID: "exec-bare", SessionID: "session-bare", ACPSessionID: "acp", sessionInitialized: true,
		}))
		err := h.mgr.SetSessionMode(ctx, "exec-bare", "", "plan")
		require.ErrorContains(t, err, "has no agentctl client")
		require.Zero(t, h.count())
	})

	t.Run("acp session not initialized", func(t *testing.T) {
		h := newSessionOpsHarness(t)
		h.exec.sessionInitialized = false
		err := h.mgr.SetSessionMode(ctx, "exec-ops", "", "plan")
		require.ErrorContains(t, err, "ACP session is not ready")
		require.Zero(t, h.count())
	})

	t.Run("acp session id empty", func(t *testing.T) {
		h := newSessionOpsHarness(t)
		h.exec.ACPSessionID = ""
		err := h.mgr.SetSessionMode(ctx, "exec-ops", "", "plan")
		require.ErrorContains(t, err, "ACP session is not ready")
		require.Zero(t, h.count())
	})

	t.Run("unknown session id", func(t *testing.T) {
		h := newSessionOpsHarness(t)
		err := h.mgr.SetSessionModeBySessionID(ctx, "nope", "plan")
		require.ErrorContains(t, err, `no agent running for session "nope"`)
		require.Zero(t, h.count())
	})
}

func TestSetSessionModelSendsModelToAgentctl(t *testing.T) {
	h := newSessionOpsHarness(t)

	require.NoError(t, h.mgr.SetSessionModelBySessionID(context.Background(), "session-ops", "claude-opus-5"))

	req := h.only(t)
	require.Equal(t, "agent.session.set_model", req.action)
	require.Equal(t, "claude-opus-5", req.payload["model_id"])
	require.Empty(t, h.exec.metadataString(MetadataKeyModelOverride),
		"an ACP session swaps the model in-place and must not persist a relaunch override")
}

// TestSetSessionModelPassthroughPersistsOverrideAndRestarts pins the
// passthrough branch: a TUI agent has no live channel for the model, so the
// override is persisted and the process is relaunched instead. The restart is
// expected to fail here (no interactive runner is wired), which is what proves
// RestartAgentProcess was reached — but the override must already be stored.
func TestSetSessionModelPassthroughPersistsOverrideAndRestarts(t *testing.T) {
	h := newSessionOpsHarness(t)
	h.exec.PassthroughProcessID = "pty-1"

	err := h.mgr.SetSessionModel(context.Background(), "exec-ops", "gpt-5-codex")

	require.Error(t, err, "restart must surface the missing interactive runner")
	require.ErrorContains(t, err, "interactive runner not available")
	require.Equal(t, "gpt-5-codex", h.exec.metadataString(MetadataKeyModelOverride),
		"passthrough model change must persist the override for the next launch")
	require.Zero(t, h.count(), "passthrough agents have no ACP channel for set_model")
}

func TestSetSessionModelGuards(t *testing.T) {
	ctx := context.Background()

	t.Run("unknown execution", func(t *testing.T) {
		h := newSessionOpsHarness(t)
		require.ErrorContains(t, h.mgr.SetSessionModel(ctx, "missing", "m"), `execution "missing" not found`)
		require.Zero(t, h.count())
	})

	t.Run("no agentctl client", func(t *testing.T) {
		h := newSessionOpsHarness(t)
		require.NoError(t, h.mgr.executionStore.Add(&AgentExecution{ID: "exec-bare", SessionID: "session-bare"}))
		require.ErrorContains(t, h.mgr.SetSessionModel(ctx, "exec-bare", "m"), "has no agentctl client")
		require.Zero(t, h.count())
	})

	t.Run("unknown session id", func(t *testing.T) {
		h := newSessionOpsHarness(t)
		require.ErrorContains(t, h.mgr.SetSessionModelBySessionID(ctx, "nope", "m"), `no agent running for session "nope"`)
		require.Zero(t, h.count())
	})
}

func TestSetSessionConfigOptionSendsIDAndValue(t *testing.T) {
	h := newSessionOpsHarness(t)

	require.NoError(t, h.mgr.SetSessionConfigOptionBySessionID(context.Background(), "session-ops", "reasoning", "high"))

	req := h.only(t)
	require.Equal(t, "agent.session.set_config_option", req.action)
	require.Equal(t, "reasoning", req.payload["config_id"])
	require.Equal(t, "high", req.payload["value"])
	require.NotContains(t, req.payload, "include_configuration_result",
		"error-only lifecycle calls must preserve strict setter behavior")
}

func TestSetSessionConfigOptionErrorOnlyMethodsDoNotRequestResult(t *testing.T) {
	tests := []struct {
		name string
		call func(*Manager) error
	}{
		{
			name: "execution ID",
			call: func(mgr *Manager) error {
				return mgr.SetSessionConfigOption(context.Background(), "exec-ops", "codex_mode", "plan")
			},
		},
		{
			name: "session ID",
			call: func(mgr *Manager) error {
				return mgr.SetSessionConfigOptionBySessionID(context.Background(), "session-ops", "codex_mode", "plan")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newSessionOpsHarness(t)
			require.NoError(t, tt.call(h.mgr))
			req := h.only(t)
			require.Equal(t, "agent.session.set_config_option", req.action)
			require.NotContains(t, req.payload, "include_configuration_result",
				"error-only lifecycle calls must preserve strict setter behavior")
		})
	}
}

func TestSetSessionConfigOptionWithResultRequestsPickerEvidence(t *testing.T) {
	h := newSessionOpsHarness(t)

	result, err := h.mgr.SetSessionConfigOptionWithResult(context.Background(), "exec-ops", "codex_mode", "plan")
	require.NoError(t, err)
	require.Equal(t, "exec-ops", result.ExecutionID)
	require.Equal(t, "codex_mode", result.ConfigID)
	require.Equal(t, "plan", result.Requested)
	require.False(t, result.Confirmed)

	req := h.only(t)
	require.Equal(t, true, req.payload["include_configuration_result"],
		"picker result calls must explicitly request correlated configuration evidence")
}

func TestExpectedModelConfigOptionSerializesAgainstModelChange(t *testing.T) {
	client, requests := newConcurrentSessionConfigurationClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	t.Cleanup(cancel)

	mgr := newTestManager(t)
	execution := &AgentExecution{
		ID: "exec-serialized-config", SessionID: "session-serialized-config", ACPSessionID: "acp-live",
		Status: v1.AgentStatusReady, sessionInitialized: true, agentctl: client,
	}
	execution.SetModelState(&CachedModelState{CurrentModelID: "model-b"})
	require.NoError(t, mgr.executionStore.Add(execution))

	configResult := make(chan error, 1)
	go func() {
		_, err := mgr.SetSessionConfigOptionBySessionIDWithExpectedModelWithResult(
			ctx, execution.SessionID, "thinking", "high", "model-b",
		)
		configResult <- err
	}()
	var configRequest concurrentSessionConfigurationRequest
	select {
	case configRequest = <-requests:
	case <-time.After(time.Second):
		t.Fatal("expected-model option request was not sent")
	}
	require.Equal(t, "agent.session.set_config_option", configRequest.message.Action)

	modelStarted := make(chan struct{})
	modelResult := make(chan error, 1)
	go func() {
		close(modelStarted)
		_, err := mgr.SetSessionModelWithResult(ctx, execution.ID, "model-c")
		modelResult <- err
	}()
	<-modelStarted

	modelChangedBeforeOptionCompleted := false
	select {
	case modelRequest := <-requests:
		modelChangedBeforeOptionCompleted = true
		require.Equal(t, "agent.session.set_model", modelRequest.message.Action)
		respondConcurrentSessionConfigurationRequest(t, modelRequest, map[string]any{
			"requested": "model-c", "effective": "model-c", "confirmed": true,
		})
	case <-time.After(100 * time.Millisecond):
	}
	respondConcurrentSessionConfigurationRequest(t, configRequest, map[string]any{
		"requested": "high", "effective": "high", "confirmed": true,
	})
	require.NoError(t, <-configResult)
	if !modelChangedBeforeOptionCompleted {
		select {
		case modelRequest := <-requests:
			require.Equal(t, "agent.session.set_model", modelRequest.message.Action)
			respondConcurrentSessionConfigurationRequest(t, modelRequest, map[string]any{
				"requested": "model-c", "effective": "model-c", "confirmed": true,
			})
		case <-time.After(time.Second):
			t.Fatal("model change did not resume after the option request completed")
		}
	}
	require.NoError(t, <-modelResult)
	require.False(t, modelChangedBeforeOptionCompleted,
		"a model change must not cross the expected-model guard and option write")
}

type concurrentSessionConfigurationRequest struct {
	message ws.Message
	conn    *websocket.Conn
}

func newConcurrentSessionConfigurationClient(t *testing.T) (*agentctlclient.Client, <-chan concurrentSessionConfigurationRequest) {
	t.Helper()
	requests := make(chan concurrentSessionConfigurationRequest, 4)
	connected := make(chan struct{})
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		close(connected)
		for {
			_, raw, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var message ws.Message
			if json.Unmarshal(raw, &message) == nil && message.Type == ws.MessageTypeRequest {
				requests <- concurrentSessionConfigurationRequest{message: message, conn: conn}
			}
		}
	}))
	t.Cleanup(server.Close)
	client := createTestClient(t, server.URL)
	t.Cleanup(client.Close)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	require.NoError(t, client.StreamUpdates(ctx, func(agentctlclient.AgentEvent) {}, nil, nil))
	select {
	case <-connected:
	case <-time.After(time.Second):
		t.Fatal("agentctl stream did not connect")
	}
	return client, requests
}

func respondConcurrentSessionConfigurationRequest(
	t *testing.T,
	request concurrentSessionConfigurationRequest,
	payload map[string]any,
) {
	t.Helper()
	response, err := ws.NewResponse(request.message.ID, request.message.Action, payload)
	require.NoError(t, err)
	require.NoError(t, request.conn.WriteJSON(response))
}

func TestSetSessionConfigOptionGuards(t *testing.T) {
	ctx := context.Background()

	t.Run("unknown execution", func(t *testing.T) {
		h := newSessionOpsHarness(t)
		require.ErrorContains(t, h.mgr.SetSessionConfigOption(ctx, "missing", "c", "v"), `execution "missing" not found`)
		require.Zero(t, h.count())
	})

	t.Run("acp session not ready", func(t *testing.T) {
		h := newSessionOpsHarness(t)
		h.exec.ACPSessionID = ""
		require.ErrorContains(t, h.mgr.SetSessionConfigOption(ctx, "exec-ops", "c", "v"), "ACP session is not ready")
		require.Zero(t, h.count())
	})

	t.Run("unknown session id", func(t *testing.T) {
		h := newSessionOpsHarness(t)
		require.ErrorContains(t, h.mgr.SetSessionConfigOptionBySessionID(ctx, "nope", "c", "v"),
			`no agent running for session "nope"`)
		require.Zero(t, h.count())
	})
}

func TestAuthenticateBySessionIDSendsMethodID(t *testing.T) {
	h := newSessionOpsHarness(t)

	require.NoError(t, h.mgr.AuthenticateBySessionID(context.Background(), "session-ops", "claude-auth-login"))

	req := h.only(t)
	require.Equal(t, "agent.session.authenticate", req.action)
	require.Equal(t, "claude-auth-login", req.payload["method_id"])
}

func TestAuthenticateBySessionIDGuards(t *testing.T) {
	ctx := context.Background()

	t.Run("unknown session", func(t *testing.T) {
		h := newSessionOpsHarness(t)
		require.ErrorContains(t, h.mgr.AuthenticateBySessionID(ctx, "nope", "m"), `no agent running for session "nope"`)
		require.Zero(t, h.count())
	})

	t.Run("no agentctl client", func(t *testing.T) {
		h := newSessionOpsHarness(t)
		require.NoError(t, h.mgr.executionStore.Add(&AgentExecution{ID: "exec-bare", SessionID: "session-bare"}))
		require.ErrorContains(t, h.mgr.AuthenticateBySessionID(ctx, "session-bare", "m"), "has no agentctl client")
		require.Zero(t, h.count())
	})
}

func TestRespondToPermissionForwardsDecision(t *testing.T) {
	for _, tc := range []struct {
		name      string
		optionID  string
		cancelled bool
	}{
		{name: "approve", optionID: "allow", cancelled: false},
		{name: "deny", optionID: "deny", cancelled: false},
		{name: "dialog cancelled", optionID: "", cancelled: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newSessionOpsHarness(t)

			require.NoError(t, h.mgr.RespondToPermissionBySessionID("session-ops", "pending-7", tc.optionID, tc.cancelled))

			req := h.only(t)
			require.Equal(t, "agent.permissions.respond", req.action)
			var got struct {
				PendingID string `json:"pending_id"`
				OptionID  string `json:"option_id"`
				Cancelled bool   `json:"cancelled"`
			}
			req.decode(t, &got)
			require.Equal(t, "pending-7", got.PendingID)
			require.Equal(t, tc.optionID, got.OptionID)
			require.Equal(t, tc.cancelled, got.Cancelled)
		})
	}
}

func TestRespondToPermissionGuards(t *testing.T) {
	t.Run("unknown execution", func(t *testing.T) {
		h := newSessionOpsHarness(t)
		require.ErrorContains(t, h.mgr.RespondToPermission("missing", "p", "allow", false),
			"agent execution not found: missing")
		require.Zero(t, h.count())
	})

	t.Run("no agentctl client", func(t *testing.T) {
		h := newSessionOpsHarness(t)
		require.NoError(t, h.mgr.executionStore.Add(&AgentExecution{ID: "exec-bare", SessionID: "session-bare"}))
		require.ErrorContains(t, h.mgr.RespondToPermission("exec-bare", "p", "allow", false),
			"agent execution has no agentctl client")
		require.Zero(t, h.count())
	})

	t.Run("unknown session", func(t *testing.T) {
		h := newSessionOpsHarness(t)
		require.ErrorContains(t, h.mgr.RespondToPermissionBySessionID("nope", "p", "allow", false),
			"no agent execution found for session: nope")
		require.Zero(t, h.count())
	})
}

// TestCancelAgentBySessionIDPassthroughWritesCtrlC pins that a passthrough
// session is cancelled with a PTY Ctrl-C rather than an ACP agent.cancel, and
// that a failed write stays non-fatal so the caller still reconciles DB state.
func TestCancelAgentBySessionIDPassthroughSendsCtrlCNotACPCancel(t *testing.T) {
	h := newSessionOpsHarness(t)
	h.exec.PassthroughProcessID = "pty-1"

	// No interactive runner is wired, so WritePassthroughStdin fails — the
	// documented non-fatal path.
	require.NoError(t, h.mgr.CancelAgentBySessionID(context.Background(), "session-ops"),
		"a failed PTY write must not surface as a cancel error")
	require.Zero(t, h.count(), "passthrough cancel must not send agent.cancel over ACP")
}

func TestCancelAgentBySessionIDACPPathSendsCancel(t *testing.T) {
	h := newSessionOpsHarness(t)

	require.NoError(t, h.mgr.CancelAgentBySessionID(context.Background(), "session-ops"))

	req := h.only(t)
	require.Equal(t, "agent.cancel", req.action)
}

func TestCancelAgentBySessionIDUnknownSession(t *testing.T) {
	h := newSessionOpsHarness(t)

	err := h.mgr.CancelAgentBySessionID(context.Background(), "nope")

	require.ErrorIs(t, err, ErrNoExecutionForSession)
	require.Zero(t, h.count())
}

func TestCancelAgentRejectsExecutionWithoutAgentctl(t *testing.T) {
	mgr := newTestManager(t)
	require.NoError(t, mgr.executionStore.Add(&AgentExecution{ID: "exec-bare", SessionID: "session-bare"}))

	err := mgr.CancelAgent(context.Background(), "exec-bare")

	require.ErrorContains(t, err, "has no agentctl client")
}

func TestCancelAgentUnknownExecution(t *testing.T) {
	mgr := newTestManager(t)

	err := mgr.CancelAgent(context.Background(), "missing")

	require.ErrorContains(t, err, `execution "missing" not found`)
}

// TestSetMcpProvidersForSessionSendsProviders exercises the HTTP-backed MCP
// provider refresh and its documented no-op-on-missing-session contract.
func TestSetMcpProvidersForSessionRequiresSessionID(t *testing.T) {
	mgr := newTestManager(t)

	require.ErrorContains(t, mgr.SetMcpProvidersForSession(context.Background(), "", nil), "session_id is required")
}

func TestSetMcpProvidersForSessionMissingExecutionIsNoOp(t *testing.T) {
	mgr := newTestManager(t)

	require.NoError(t, mgr.SetMcpProvidersForSession(context.Background(), "session-absent", []string{"github"}),
		"an absent execution is a successful no-op: the next launch re-derives providers")
}

func TestSetMcpProvidersForSessionRequiresAgentctl(t *testing.T) {
	mgr := newTestManager(t)
	require.NoError(t, mgr.executionStore.Add(&AgentExecution{ID: "exec-bare", SessionID: "session-bare"}))

	err := mgr.SetMcpProvidersForSession(context.Background(), "session-bare", []string{"github"})

	require.ErrorContains(t, err, "has no agentctl client")
}

func TestSetMcpModeGuards(t *testing.T) {
	mgr := newTestManager(t)
	require.NoError(t, mgr.executionStore.Add(&AgentExecution{ID: "exec-bare", SessionID: "session-bare"}))

	require.ErrorContains(t, mgr.SetMcpMode(context.Background(), "missing", "config"), `execution "missing" not found`)
	require.ErrorContains(t, mgr.SetMcpMode(context.Background(), "exec-bare", "config"), "has no agentctl client")
}

// TestCancelAgentEscalatesWhenStreamDisconnected covers the branch where the
// stream is torn down before the cancel lands: the manager escalates locally
// rather than surfacing the transport error.
func TestCancelAgentEscalatesWhenStreamDisconnected(t *testing.T) {
	prevEsc := cancelEscalationTimeout
	cancelEscalationTimeout = 50 * time.Millisecond
	t.Cleanup(func() { cancelEscalationTimeout = prevEsc })

	mock := newMockAgentServer(t)
	t.Cleanup(mock.Close)
	client := createTestClient(t, mock.server.URL)
	t.Cleanup(client.Close)

	mgr := newTestManager(t)
	promptFinished := make(chan struct{})
	exec := &AgentExecution{
		ID:             "exec-disc",
		SessionID:      "session-disc",
		Status:         v1.AgentStatusRunning,
		agentctl:       client,
		promptFinished: promptFinished,
		promptDoneCh:   make(chan PromptCompletionSignal, 1),
	}
	require.NoError(t, mgr.executionStore.Add(exec))

	// The agent stream was never connected, so Cancel returns
	// ErrAgentStreamNotConnected — the teardown-race path.
	err := mgr.CancelAgent(context.Background(), "exec-disc")

	require.True(t, errors.Is(err, ErrCancelEscalated),
		"a disconnected stream must escalate locally, got %v", err)
	require.Equal(t, v1.AgentStatusReady, exec.Status,
		"escalation must leave the execution ready so the workflow can advance")
}
