package acp

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	sdk "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/stretchr/testify/require"
)

func TestCodexCapacityContinuationEvidence(t *testing.T) {
	cases := []struct {
		name   string
		frames []string
		safe   bool
	}{
		{
			name: "completed read shell write and MCP tools",
			frames: []string{
				`{"sessionUpdate":"tool_call","toolCallId":"read-1","title":"Read","kind":"read","status":"completed"}`,
				`{"sessionUpdate":"tool_call","toolCallId":"shell-1","title":"Run","kind":"execute","status":"completed"}`,
				`{"sessionUpdate":"tool_call","toolCallId":"write-1","title":"Write","kind":"edit","status":"completed"}`,
				`{"sessionUpdate":"tool_call","toolCallId":"mcp-1","title":"MCP","kind":"execute","status":"completed","_meta":{"is_mcp_tool_call":true},"rawInput":{"server":"tools","tool":"lookup","arguments":{}}}`,
			},
			safe: true,
		},
		{name: "output only", safe: true},
		{
			name: "completed and pending",
			frames: []string{
				`{"sessionUpdate":"tool_call","toolCallId":"done-1","title":"Read","kind":"read","status":"completed"}`,
				`{"sessionUpdate":"tool_call","toolCallId":"pending-1","title":"Run","kind":"execute","status":"pending"}`,
			},
		},
		{name: "failed", frames: []string{`{"sessionUpdate":"tool_call","toolCallId":"failed-1","title":"Run","kind":"execute","status":"failed"}`}},
		{name: "unknown status", frames: []string{`{"sessionUpdate":"tool_call","toolCallId":"unknown-1","title":"Run","kind":"execute","status":"cancelled"}`}},
		{
			name: "conflicting status",
			frames: []string{
				`{"sessionUpdate":"tool_call","toolCallId":"conflict-1","title":"Run","kind":"execute","status":"completed"}`,
				`{"sessionUpdate":"tool_call_update","toolCallId":"conflict-1","status":"pending"}`,
			},
		},
		{name: "unmatched update", frames: []string{`{"sessionUpdate":"tool_call_update","toolCallId":"missing-1","status":"completed"}`}},
		{
			name: "duplicate tool id",
			frames: []string{
				`{"sessionUpdate":"tool_call","toolCallId":"same-1","title":"Read","kind":"read","status":"completed"}`,
				`{"sessionUpdate":"tool_call","toolCallId":"same-1","title":"Read","kind":"read","status":"completed"}`,
			},
		},
		{name: "missing tool id", frames: []string{`{"sessionUpdate":"tool_call","title":"Run","kind":"execute","status":"completed"}`}},
		{name: "unaccounted collaboration", frames: []string{`{"sessionUpdate":"tool_call","toolCallId":"spawn-1","title":"Spawn","kind":"execute","status":"completed","_meta":{"codex":{"collaboration":{"tool":"spawnAgent"}}}}`}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a, turn := newCapacityPromptTurn(t, 7)
			for _, frame := range tc.frames {
				var update sdk.SessionUpdate
				require.NoError(t, json.Unmarshal([]byte(frame), &update))
				a.handleACPUpdate(makeNotification("capacity-session", update), 7)
				_ = drainEvents(a)
			}
			snapshot := a.capacityContinuationSnapshot(turn, true)
			require.NotNil(t, snapshot)
			require.Equal(t, tc.safe, snapshot.SafeFor(7))
			if tc.name == "completed read shell write and MCP tools" {
				require.Equal(t, uint16(4), snapshot.CompletedTools)
			}
		})
	}
}

func TestCapacityContinuationPermissionBlocksSnapshot(t *testing.T) {
	a, turn := newCapacityPromptTurn(t, 8)
	a.handleACPUpdate(makeNotification("capacity-session", sdk.SessionUpdate{
		ToolCall: &sdk.SessionUpdateToolCall{
			ToolCallId: "permission-tool", Kind: sdk.ToolKindExecute, Status: sdk.ToolCallStatusPending,
		},
	}), 8)
	entered := make(chan struct{})
	release := make(chan struct{})
	a.permissionHandler = func(context.Context, *PermissionRequest) (*PermissionResponse, error) {
		close(entered)
		<-release
		return &PermissionResponse{OptionID: "allow"}, nil
	}
	permissionDone := make(chan error, 1)
	go func() {
		_, err := a.handlePermissionRequest(t.Context(), &PermissionRequest{
			SessionID: "capacity-session", ToolCallID: "permission-tool",
		})
		permissionDone <- err
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("permission handler did not start")
	}
	snapshot := a.capacityContinuationSnapshot(turn, true)
	require.NotNil(t, snapshot)
	require.True(t, snapshot.PermissionPending)
	require.False(t, snapshot.SafeFor(8))
	close(release)
	select {
	case err := <-permissionDone:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("permission handler did not finish")
	}
	require.False(t, a.capacityContinuationSnapshot(turn, true).PermissionPending)
	completed := sdk.ToolCallStatusCompleted
	a.handleACPUpdate(makeNotification("capacity-session", sdk.SessionUpdate{
		ToolCallUpdate: &sdk.SessionToolCallUpdate{ToolCallId: "permission-tool", Status: &completed},
	}), 8)
	require.True(t, a.capacityContinuationSnapshot(turn, true).SafeFor(8))
}

func TestCapacityContinuationEvidenceIgnoresStaleAndLoadHistory(t *testing.T) {
	a, turn := newCapacityPromptTurn(t, 7)
	completedCall := sdk.SessionUpdateToolCall{
		ToolCallId: "old-1", Kind: sdk.ToolKindRead, Status: sdk.ToolCallStatusCompleted,
	}
	a.handleACPUpdate(makeNotification("capacity-session", sdk.SessionUpdate{ToolCall: &completedCall}), 6)
	a.mu.Lock()
	a.isLoadingSession = true
	a.mu.Unlock()
	a.handleACPUpdate(makeNotification("capacity-session", sdk.SessionUpdate{ToolCall: &completedCall}), 7)
	a.mu.Lock()
	a.isLoadingSession = false
	a.mu.Unlock()
	snapshot := a.capacityContinuationSnapshot(turn, true)
	require.True(t, snapshot.SafeFor(7))
	require.Zero(t, snapshot.CompletedTools)
}

func TestCapacityContinuationEvidenceOverflowFailsClosed(t *testing.T) {
	a, turn := newCapacityPromptTurn(t, 7)
	turn.evidenceMu.Lock()
	for i := 0; i <= capacityContinuationToolLimit; i++ {
		call := sdk.SessionUpdateToolCall{
			ToolCallId: sdk.ToolCallId(fmt.Sprintf("tool-%d", i)),
			Kind:       sdk.ToolKindExecute, Status: sdk.ToolCallStatusCompleted,
		}
		turn.observeCapacityToolCall(&call)
	}
	turn.evidenceMu.Unlock()
	snapshot := a.capacityContinuationSnapshot(turn, true)
	require.False(t, snapshot.SafeFor(7))
	require.True(t, snapshot.UnknownOutcomes)
	require.Len(t, turn.capacityTools, capacityContinuationToolLimit)
}

func TestCodexCapacityContinuationFencesBackgroundWorkAcrossPrompts(t *testing.T) {
	a, _ := newCapacityPromptTurn(t, 7)
	spawn := a.convertToolCallUpdate("capacity-session", codexCollaborationToolCall("spawn-1", "thread-child", "running"))
	require.NotNil(t, spawn)

	// The first prompt settles while the Codex child remains live. The prompt-end
	// sweep intentionally preserves it so a later authoritative update can land.
	a.cancelPromptEndToolCalls("capacity-session")
	_, childStillTracked := a.activeToolCalls[spawn.ToolCallID]
	require.True(t, childStillTracked, "prompt-end cleanup must retain the live child identity")

	// Prompt B contains text and then fails at capacity. Its own tool ledger is
	// empty; the session-owned child remains the authority for unresolved work.
	_, secondTurn := a.registerPromptTurn(context.Background(), 8)
	a.handleACPUpdate(makeNotification("capacity-session", sdk.SessionUpdate{
		AgentMessageChunk: &sdk.SessionUpdateAgentMessageChunk{Content: sdk.TextBlock("Some progress before capacity.")},
	}), 8)
	_ = drainEvents(a)
	snapshot := a.capacityContinuationSnapshot(secondTurn, true)
	require.NotNil(t, snapshot)
	require.True(t, snapshot.UnaccountedBackground)
	require.False(t, snapshot.SafeFor(8))

	// Only a native terminal update releases the fence. A synthetic sweep or
	// the absence of an update during Prompt B is not completion evidence.
	terminal := a.convertToolCallResultUpdate("capacity-session", codexCollaborationResultUpdate("spawn-1", "thread-child", "completed"))
	require.NotNil(t, terminal)
	_, thirdTurn := a.registerPromptTurn(context.Background(), 9)
	require.True(t, a.capacityContinuationSnapshot(thirdTurn, true).SafeFor(9))
}

func TestCapacityContinuationFencesRetainedBackgroundShellEvidence(t *testing.T) {
	a, turn := newCapacityPromptTurn(t, 10)
	shell := streams.NewShellExec("sleep 10", "", "background shell", 0, false)
	shell.SetBackgroundWorkIdentity(streams.BackgroundWorkKindShell, "shell-work-1", true, false)
	a.activeToolCalls["background-shell-1"] = shell

	snapshot := a.capacityContinuationSnapshot(turn, true)
	require.True(t, snapshot.UnaccountedBackground)
	require.False(t, snapshot.SafeFor(10))
}

func TestCapacityContinuationFencesActiveMonitorWork(t *testing.T) {
	a, turn := newCapacityPromptTurn(t, 11)
	a.activeMonitors["capacity-session"] = map[string]string{"monitor-task": "monitor-call"}

	snapshot := a.capacityContinuationSnapshot(turn, true)
	require.True(t, snapshot.UnaccountedBackground)
	require.False(t, snapshot.SafeFor(11))
}

func newCapacityPromptTurn(t *testing.T, generation uint64) (*Adapter, *promptTurnState) {
	t.Helper()
	a, turn := newCursorPromptTurn(t, generation)
	a.agentID = codexAgentID
	a.normalizer = NewNormalizer(codexAgentID)
	a.dialect = newCodexACPDialect()
	a.capabilities.LoadSession = true
	a.sessionID = "capacity-session"
	return a, turn
}

func TestCapacityContinuationSupportIsNarrow(t *testing.T) {
	require.Equal(t, streams.CapacityContinuationCodexLiveSessionV1, newCodexACPDialect().capacityContinuationSupport)
	require.Equal(t, streams.CapacityContinuationMockLiveSessionV1, newMockACPDialect().capacityContinuationSupport)
	require.Empty(t, newACPDialect(cursorAgentID).capacityContinuationSupport)
	require.Empty(t, newACPDialect(claudeAgentID).capacityContinuationSupport)
}

func TestCapacityContinuationSnapshotRemoteRoundTrip(t *testing.T) {
	snapshot := &streams.CapacityContinuationSnapshot{
		Support: streams.CapacityContinuationCodexLiveSessionV1, PromptGeneration: 7, EvidenceComplete: true,
		CompletedTools: 2,
	}
	event := streams.AgentEvent{Type: streams.EventTypeError, PromptGeneration: 7, CapacityContinuation: snapshot}
	raw, err := json.Marshal(event)
	require.NoError(t, err)
	var decoded streams.AgentEvent
	require.NoError(t, json.Unmarshal(raw, &decoded))
	require.Equal(t, *snapshot, *decoded.CapacityContinuation)

	var omitted streams.AgentEvent
	require.NoError(t, json.Unmarshal([]byte(`{"type":"error","prompt_generation":7}`), &omitted))
	require.Nil(t, omitted.CapacityContinuation)
}
