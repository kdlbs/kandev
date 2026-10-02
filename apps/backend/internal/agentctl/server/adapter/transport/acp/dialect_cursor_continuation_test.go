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
		return
	}
	t.Fatal("missing terminal error")
}

// @covers AC-PLATFORM-INTERRUPTION-CONTINUATION-001.2
func TestCursorContinuationEvidenceToolOutcomes(t *testing.T) {
	cases := []struct {
		name   string
		frames []string
		safe   bool
		reads  uint16
	}{
		{name: "output only", safe: true},
		{name: "completed read", frames: []string{`{"sessionUpdate":"tool_call","toolCallId":"read-1","title":"Read","kind":"read","status":"in_progress","rawInput":{"path":"fixture.txt"}}`, `{"sessionUpdate":"tool_call_update","toolCallId":"read-1","status":"completed"}`}, safe: true, reads: 1},
		{name: "pending read", frames: []string{`{"sessionUpdate":"tool_call","toolCallId":"read-1","title":"Read","kind":"read","status":"in_progress"}`}},
		{name: "write", frames: []string{`{"sessionUpdate":"tool_call","toolCallId":"write-1","title":"Read","kind":"edit","status":"completed"}`}},
		{name: "execute", frames: []string{`{"sessionUpdate":"tool_call","toolCallId":"cmd-1","title":"Read","kind":"execute","status":"completed"}`}},
		{name: "missing kind", frames: []string{`{"sessionUpdate":"tool_call","toolCallId":"unknown-1","title":"Read","status":"completed"}`}},
		{name: "orphan update", frames: []string{`{"sessionUpdate":"tool_call_update","toolCallId":"unknown-1","status":"completed"}`}},
		{name: "failed read", frames: []string{`{"sessionUpdate":"tool_call","toolCallId":"read-1","title":"Read","kind":"read","status":"failed"}`}},
		{name: "MCP disguised as read", frames: []string{`{"sessionUpdate":"tool_call","toolCallId":"read-1","title":"Read","kind":"read","status":"completed","rawInput":{"providerIdentifier":"server","toolName":"read","args":{}}}`}},
		{name: "background", frames: []string{`{"sessionUpdate":"tool_call","toolCallId":"read-1","title":"Read","kind":"read","status":"completed","_meta":{"isBackground":true}}`}},
		{name: "nested", frames: []string{`{"sessionUpdate":"tool_call","toolCallId":"read-1","title":"Read","kind":"read","status":"completed","_meta":{"parentToolCallId":"parent"}}`}},
		{name: "conflicting kind", frames: []string{`{"sessionUpdate":"tool_call","toolCallId":"read-1","title":"Read","kind":"read","status":"completed"}`, `{"sessionUpdate":"tool_call_update","toolCallId":"read-1","kind":"execute","status":"completed"}`}},
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
