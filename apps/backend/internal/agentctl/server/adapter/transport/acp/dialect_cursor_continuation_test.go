package acp

import (
	"encoding/json"
	"fmt"
	acpsdk "github.com/coder/acp-go-sdk"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/stretchr/testify/require"
)

// @covers AC-PLATFORM-INTERRUPTION-CONTINUATION-001.1
func TestCursorContinuationEvidenceTerminalOutput(t *testing.T) {
	a, fake, conn := setupHandoffFakeAgent(t)
	a.agentID = cursorAgentID
	a.normalizer = NewNormalizer(cursorAgentID)
	a.dialect = newACPDialect(cursorAgentID)
	a.cfg.ProviderInterruptionContinuation = true
	require.NoError(t, a.Initialize(t.Context()))
	a.capabilities.LoadSession = true
	_, err := a.NewSession(t.Context(), nil)
	require.NoError(t, err)
	_ = drainEvents(a)
	done := make(chan error, 1)
	go func() { done <- a.Prompt(t.Context(), "test", nil, 7) }()
	select {
	case <-fake.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("prompt not accepted")
	}
	sendCapturedUpdate(t, conn, `{"sessionId":"session-handoff","update":{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"partial response"}}}`)
	sendCapturedUpdate(t, conn, `{"sessionId":"session-handoff","update":{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"`+cursorRetriableStreamResetChunk+`"}}}`)
	fake.releasePrompts()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("prompt did not settle")
	}
	for _, e := range drainEvents(a) {
		if e.Type != streams.EventTypeError {
			continue
		}
		raw, err := json.Marshal(e)
		require.NoError(t, err)
		var wire map[string]any
		require.NoError(t, json.Unmarshal(raw, &wire))
		require.Contains(t, wire, "continuation_safety", "terminal error must carry bounded safety evidence")
		var decoded AgentEvent
		require.NoError(t, json.Unmarshal(raw, &decoded))
		require.True(t, decoded.ContinuationSafety.SafeFor(7))
		return
	}
	t.Fatal("missing terminal error")
}

func TestCursorContinuationEvidenceRPCFailureRetiresAsyncCompletion(t *testing.T) {
	a, fake, _ := setupHandoffFakeAgent(t)
	a.agentID = mockAgentID
	a.dialect = newACPDialect(mockAgentID)
	a.cfg.ProviderInterruptionContinuation = true
	fake.promptFailure = &acpsdk.RequestError{Code: -32603, Message: "peer disconnected before response", Data: map[string]any{"kandevMock": map[string]any{"continuationInterruption": true}}}
	require.NoError(t, a.Initialize(t.Context()))
	a.capabilities.LoadSession = true
	_, err := a.NewSession(t.Context(), nil)
	require.NoError(t, err)
	done := make(chan error, 1)
	go func() { done <- a.Prompt(t.Context(), "test", nil, 7) }()
	select {
	case <-fake.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("prompt not accepted")
	}
	a.asyncTurnMu.Lock()
	a.asyncTurnFinalizers["session-handoff"] = &asyncTurnFinalizer{seq: 1, promptEpoch: a.asyncTurnEpochs["session-handoff"]}
	a.asyncTurnMu.Unlock()
	fake.releasePrompts()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("prompt did not settle")
	}
	a.asyncTurnMu.Lock()
	_, scheduled := a.asyncTurnFinalizers["session-handoff"]
	a.asyncTurnMu.Unlock()
	require.False(t, scheduled, "a terminal interruption cannot retain an idle-success finalizer")
}

// @covers AC-PLATFORM-INTERRUPTION-CONTINUATION-001.2
func TestCursorContinuationEvidenceToolOutcomes(t *testing.T) {
	cases := []struct {
		name   string
		frames []string
		safe   bool
		tools  uint16
	}{
		{name: "output only", safe: true},
		{name: "completed read", frames: []string{`{"sessionUpdate":"tool_call","toolCallId":"read-1","title":"Read","kind":"read","status":"in_progress","rawInput":{"path":"fixture.txt"}}`, `{"sessionUpdate":"tool_call_update","toolCallId":"read-1","status":"completed"}`}, safe: true, tools: 1},
		{name: "pending read", frames: []string{`{"sessionUpdate":"tool_call","toolCallId":"read-1","title":"Read","kind":"read","status":"in_progress"}`}},
		{name: "write", frames: []string{`{"sessionUpdate":"tool_call","toolCallId":"write-1","title":"Edit","kind":"edit","status":"completed"}`}, safe: true, tools: 1},
		{name: "execute", frames: []string{`{"sessionUpdate":"tool_call","toolCallId":"cmd-1","title":"Execute","kind":"execute","status":"completed"}`}, safe: true, tools: 1},
		{name: "missing kind", frames: []string{`{"sessionUpdate":"tool_call","toolCallId":"unknown-1","title":"Tool","status":"completed"}`}, safe: true, tools: 1},
		{name: "orphan update", frames: []string{`{"sessionUpdate":"tool_call_update","toolCallId":"unknown-1","status":"completed"}`}},
		{name: "failed read", frames: []string{`{"sessionUpdate":"tool_call","toolCallId":"read-1","title":"Read","kind":"read","status":"failed"}`}},
		{name: "MCP tool", frames: []string{`{"sessionUpdate":"tool_call","toolCallId":"mcp-1","title":"MCP Read","kind":"read","status":"completed","rawInput":{"providerIdentifier":"server","toolName":"read","args":{}}}`}, safe: true, tools: 1},
		{name: "background", frames: []string{`{"sessionUpdate":"tool_call","toolCallId":"read-1","title":"Read","kind":"read","status":"completed","_meta":{"isBackground":true}}`}},
		{name: "Cursor subagent title alone", frames: []string{`{"sessionUpdate":"tool_call","toolCallId":"task-1","title":"Task: Subagent task","kind":"other","status":"completed"}`}},
		{name: "Cursor subagent", frames: []string{`{"sessionUpdate":"tool_call","toolCallId":"task-1","title":"Task: Subagent task","kind":"other","status":"completed","rawInput":{"_toolName":"task","prompt":"inspect fixture"}}`}},
		{name: "Cursor background result", frames: []string{`{"sessionUpdate":"tool_call","toolCallId":"task-1","title":"Task: Subagent task","kind":"other","status":"in_progress"}`, `{"sessionUpdate":"tool_call_update","toolCallId":"task-1","status":"completed","rawOutput":{"durationMs":10,"isBackground":true}}`}},
		{name: "late Cursor subagent input", frames: []string{`{"sessionUpdate":"tool_call","toolCallId":"task-1","title":"Task: Subagent task","kind":"other","status":"in_progress"}`, `{"sessionUpdate":"tool_call_update","toolCallId":"task-1","status":"completed","rawInput":{"_toolName":"task"}}`}},
		{name: "background shell", frames: []string{`{"sessionUpdate":"tool_call","toolCallId":"cmd-1","title":"Execute","kind":"execute","status":"completed","rawInput":{"command":"fixture","wait":false}}`}},
		{name: "nested", frames: []string{`{"sessionUpdate":"tool_call","toolCallId":"read-1","title":"Read","kind":"read","status":"completed","_meta":{"parentToolCallId":"parent"}}`}},
		{name: "mixed completed and pending", frames: []string{`{"sessionUpdate":"tool_call","toolCallId":"read-1","title":"Read","kind":"read","status":"completed"}`, `{"sessionUpdate":"tool_call","toolCallId":"cmd-1","title":"Execute","kind":"execute","status":"in_progress"}`}},
		{name: "reused id", frames: []string{`{"sessionUpdate":"tool_call","toolCallId":"read-1","title":"Read","kind":"read","status":"completed"}`, `{"sessionUpdate":"tool_call","toolCallId":"read-1","title":"Read","kind":"read","status":"completed"}`}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a, turn := newCursorPromptTurn(t, 7)
			a.cfg.ProviderInterruptionContinuation = true
			a.capabilities.LoadSession = true
			a.sessionID = "session-1"
			for _, frame := range tc.frames {
				var n acpsdk.SessionNotification
				require.NoError(t, json.Unmarshal([]byte(`{"sessionId":"session-1","update":`+frame+`}`), &n))
				a.handleACPUpdate(n, 7)
			}
			snapshot := a.continuationSafetySnapshot(turn)
			require.Equal(t, tc.safe, snapshot.SafeFor(7))
			if tc.safe {
				require.Equal(t, tc.tools, snapshot.CompletedTools)
			}
		})
	}
}

func TestCursorContinuationEvidenceV1ReadOnlyWireSkew(t *testing.T) {
	cases := []struct {
		name   string
		frames []string
		safe   bool
		reads  uint16
	}{
		{name: "completed read", frames: []string{`{"sessionUpdate":"tool_call","toolCallId":"read-1","title":"Read","kind":"read","status":"in_progress","rawInput":{"path":"fixture.txt"}}`, `{"sessionUpdate":"tool_call_update","toolCallId":"read-1","status":"completed"}`}, safe: true, reads: 1},
		{name: "write fails under v1", frames: []string{`{"sessionUpdate":"tool_call","toolCallId":"write-1","title":"Edit","kind":"edit","status":"completed"}`}, safe: false},
		{name: "execute fails under v1", frames: []string{`{"sessionUpdate":"tool_call","toolCallId":"cmd-1","title":"Execute","kind":"execute","status":"completed"}`}, safe: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a, turn := newCursorPromptTurn(t, 7)
			a.dialect.continuationSupport = streams.ContinuationNativeSavedHistoryV1
			a.cfg.ProviderInterruptionContinuation = true
			a.capabilities.LoadSession = true
			a.sessionID = "session-1"
			for _, frame := range tc.frames {
				var n acpsdk.SessionNotification
				require.NoError(t, json.Unmarshal([]byte(`{"sessionId":"session-1","update":`+frame+`}`), &n))
				a.handleACPUpdate(n, 7)
			}
			snapshot := a.continuationSafetySnapshot(turn)
			require.Equal(t, tc.safe, snapshot.SafeFor(7))
			if tc.safe {
				require.Equal(t, tc.reads, snapshot.CompletedReads)
			}
		})
	}
}

// @covers AC-PLATFORM-INTERRUPTION-CONTINUATION-001.2
func TestCursorContinuationEvidenceBoundsAndFences(t *testing.T) {
	a, turn := newCursorPromptTurn(t, 7)
	a.cfg.ProviderInterruptionContinuation = true
	a.capabilities.LoadSession = true
	a.sessionID = "session-1"
	require.True(t, a.continuationSafetySnapshot(turn).SafeFor(7))
	var n acpsdk.SessionNotification
	require.NoError(t, json.Unmarshal([]byte(`{"sessionId":"session-1","update":{"sessionUpdate":"tool_call","toolCallId":"bad","title":"Write","kind":"edit","status":"completed"}}`), &n))
	a.handleACPUpdate(n, 6)
	require.True(t, a.continuationSafetySnapshot(turn).SafeFor(7), "stale generation cannot poison successor")
	a.isLoadingSession = true
	a.handleACPUpdate(n, 7)
	require.True(t, a.continuationSafetySnapshot(turn).SafeFor(7), "history replay cannot enter current safety ledger")
	a.isLoadingSession = false
	snapshot := a.continuationSafetySnapshot(turn)
	a.poisonContinuationSafety()
	require.False(t, a.continuationSafetySnapshot(turn).SafeFor(7), "permission/background uncertainty stays unsafe")
	require.True(t, snapshot.SafeFor(7), "published snapshot is immutable")
	a.cfg.ProviderInterruptionContinuation = false
	require.Nil(t, a.continuationSafetySnapshot(turn))
	a.cfg.ProviderInterruptionContinuation = true
	a.dialect = newACPDialect("other")
	require.Nil(t, a.continuationSafetySnapshot(turn))
}

func TestCursorContinuationEvidenceOverflow(t *testing.T) {
	a, turn := newCursorPromptTurn(t, 7)
	a.cfg.ProviderInterruptionContinuation = true
	a.capabilities.LoadSession = true
	a.sessionID = "session-1"
	for i := 0; i < 257; i++ {
		a.handleACPUpdate(makeNotification("session-1", acpsdk.SessionUpdate{ToolCall: &acpsdk.SessionUpdateToolCall{ToolCallId: acpsdk.ToolCallId(fmt.Sprintf("read-%d", i)), Title: "Read", Kind: acpsdk.ToolKindRead, Status: acpsdk.ToolCallStatusCompleted}}), 7)
		_ = drainEvents(a)
	}
	require.False(t, a.continuationSafetySnapshot(turn).SafeFor(7))
	require.LessOrEqual(t, len(turn.continuationTools), 256)
}

func TestCursorContinuationEvidencePermissionSessionFence(t *testing.T) {
	a, turn := newCursorPromptTurn(t, 7)
	a.cfg.ProviderInterruptionContinuation = true
	a.capabilities.LoadSession = true
	a.sessionID = "current-session"
	_, err := a.handlePermissionRequest(t.Context(), &PermissionRequest{SessionID: "predecessor-session", ToolCallID: "old-tool"})
	require.NoError(t, err)
	require.True(t, a.continuationSafetySnapshot(turn).SafeFor(7), "a different session cannot poison the current safety ledger")
}
