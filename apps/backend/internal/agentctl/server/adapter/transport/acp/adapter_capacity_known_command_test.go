package acp

import (
	"encoding/json"
	"testing"

	sdk "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/stretchr/testify/require"
)

// @covers AC-PLATFORM-TURN-CONTINUITY-003.10
func TestCodexCapacityKnownCommandFailure(t *testing.T) {
	cases := []struct {
		name      string
		frames    []string
		safe      bool
		known     uint16
		completed uint16
		pending   bool
	}{
		{
			name: "failed status with matching terminal exit",
			frames: []string{
				`{"sessionUpdate":"tool_call","toolCallId":"read-1","title":"Read","kind":"read","status":"completed"}`,
				`{"sessionUpdate":"tool_call","toolCallId":"exec-1","title":"Run tests","kind":"execute","status":"in_progress","_meta":{"terminal_info":{"cwd":"/workspace","terminal_id":"exec-1"}}}`,
				`{"sessionUpdate":"tool_call_update","toolCallId":"exec-1","status":"failed","_meta":{"terminal_exit":{"exit_code":1,"signal":null,"terminal_id":"exec-1"},"terminal_output":{"data":"failure","terminal_id":"exec-1"}}}`,
			},
			safe: true, known: 1, completed: 1,
		},
		{
			name: "completed status with nonzero terminal exit",
			frames: []string{
				`{"sessionUpdate":"tool_call","toolCallId":"exec-2","title":"Run tests","kind":"execute","status":"in_progress"}`,
				`{"sessionUpdate":"tool_call_update","toolCallId":"exec-2","status":"completed","_meta":{"terminal_exit":{"exit_code":2,"signal":null,"terminal_id":"exec-2"},"terminal_output":{"data":"failure","terminal_id":"exec-2"}}}`,
			},
			safe: true, known: 1,
		},
		{
			name: "signal termination blocks",
			frames: []string{
				`{"sessionUpdate":"tool_call","toolCallId":"exec-3","title":"Run","kind":"execute","status":"in_progress"}`,
				`{"sessionUpdate":"tool_call_update","toolCallId":"exec-3","status":"failed","_meta":{"terminal_exit":{"exit_code":143,"signal":"SIGTERM","terminal_id":"exec-3"}}}`,
			},
		},
		{
			name: "missing terminal identity blocks",
			frames: []string{
				`{"sessionUpdate":"tool_call","toolCallId":"exec-4","title":"Run","kind":"execute","status":"in_progress"}`,
				`{"sessionUpdate":"tool_call_update","toolCallId":"exec-4","status":"failed","_meta":{"terminal_exit":{"exit_code":1,"signal":null}}}`,
			},
		},
		{
			name: "mismatched terminal identity blocks",
			frames: []string{
				`{"sessionUpdate":"tool_call","toolCallId":"exec-5","title":"Run","kind":"execute","status":"in_progress"}`,
				`{"sessionUpdate":"tool_call_update","toolCallId":"exec-5","status":"failed","_meta":{"terminal_exit":{"exit_code":1,"signal":null,"terminal_id":"other-terminal"}}}`,
			},
		},
		{
			name: "fractional exit blocks",
			frames: []string{
				`{"sessionUpdate":"tool_call","toolCallId":"exec-6","title":"Run","kind":"execute","status":"in_progress"}`,
				`{"sessionUpdate":"tool_call_update","toolCallId":"exec-6","status":"failed","_meta":{"terminal_exit":{"exit_code":1.5,"signal":null,"terminal_id":"exec-6"}}}`,
			},
		},
		{
			name: "cancelled flag blocks",
			frames: []string{
				`{"sessionUpdate":"tool_call","toolCallId":"exec-7","title":"Run","kind":"execute","status":"in_progress"}`,
				`{"sessionUpdate":"tool_call_update","toolCallId":"exec-7","status":"failed","_meta":{"terminal_exit":{"exit_code":1,"signal":null,"cancelled":true,"terminal_id":"exec-7"}}}`,
			},
		},
		{
			name: "raw exit before conflicting terminal exit blocks",
			frames: []string{
				`{"sessionUpdate":"tool_call","toolCallId":"exec-9","title":"Run","kind":"execute","status":"in_progress"}`,
				`{"sessionUpdate":"tool_call_update","toolCallId":"exec-9","status":"completed","rawOutput":{"exit_code":0}}`,
				`{"sessionUpdate":"tool_call_update","toolCallId":"exec-9","status":"completed","_meta":{"terminal_exit":{"exit_code":1,"signal":null,"terminal_id":"exec-9"}}}`,
			},
		},
		{
			name: "terminal exit before conflicting raw exit blocks",
			frames: []string{
				`{"sessionUpdate":"tool_call","toolCallId":"exec-10","title":"Run","kind":"execute","status":"in_progress"}`,
				`{"sessionUpdate":"tool_call_update","toolCallId":"exec-10","status":"completed","_meta":{"terminal_exit":{"exit_code":1,"signal":null,"terminal_id":"exec-10"}}}`,
				`{"sessionUpdate":"tool_call_update","toolCallId":"exec-10","status":"completed","rawOutput":{"exit_code":0}}`,
			},
		},
		{
			name: "same-frame raw and terminal conflict blocks",
			frames: []string{
				`{"sessionUpdate":"tool_call","toolCallId":"exec-11","title":"Run","kind":"execute","status":"in_progress"}`,
				`{"sessionUpdate":"tool_call_update","toolCallId":"exec-11","status":"completed","rawOutput":{"exit_code":0},"_meta":{"terminal_exit":{"exit_code":1,"signal":null,"terminal_id":"exec-11"}}}`,
			},
		},
		{
			name: "agreeing split-frame raw and terminal evidence continues",
			frames: []string{
				`{"sessionUpdate":"tool_call","toolCallId":"exec-12","title":"Run","kind":"execute","status":"in_progress"}`,
				`{"sessionUpdate":"tool_call_update","toolCallId":"exec-12","status":"completed","rawOutput":{"exit_code":1}}`,
				`{"sessionUpdate":"tool_call_update","toolCallId":"exec-12","status":"completed","_meta":{"terminal_exit":{"exit_code":1,"signal":null,"terminal_id":"exec-12"}}}`,
			},
			safe: true, known: 1,
		},
		{
			name: "malformed raw exit before terminal blocks",
			frames: []string{
				`{"sessionUpdate":"tool_call","toolCallId":"exec-13","title":"Run","kind":"execute","status":"in_progress"}`,
				`{"sessionUpdate":"tool_call_update","toolCallId":"exec-13","status":"completed","rawOutput":{"exit_code":1.5}}`,
				`{"sessionUpdate":"tool_call_update","toolCallId":"exec-13","status":"completed","_meta":{"terminal_exit":{"exit_code":1,"signal":null,"terminal_id":"exec-13"}}}`,
			},
		},
		{
			name: "malformed raw exit in same frame blocks",
			frames: []string{
				`{"sessionUpdate":"tool_call","toolCallId":"exec-14","title":"Run","kind":"execute","status":"in_progress"}`,
				`{"sessionUpdate":"tool_call_update","toolCallId":"exec-14","status":"completed","rawOutput":{"exit_code":"not-a-number"},"_meta":{"terminal_exit":{"exit_code":1,"signal":null,"terminal_id":"exec-14"}}}`,
			},
		},
		{
			name: "malformed formatted return code blocks",
			frames: []string{
				`{"sessionUpdate":"tool_call","toolCallId":"exec-15","title":"Run","kind":"execute","status":"in_progress"}`,
				`{"sessionUpdate":"tool_call_update","toolCallId":"exec-15","status":"completed","rawOutput":{"formatted_output":"<return-code>nope</return-code>"},"_meta":{"terminal_exit":{"exit_code":1,"signal":null,"terminal_id":"exec-15"}}}`,
			},
		},
		{
			name: "string cancellation marker blocks",
			frames: []string{
				`{"sessionUpdate":"tool_call","toolCallId":"exec-16","title":"Run","kind":"execute","status":"in_progress"}`,
				`{"sessionUpdate":"tool_call_update","toolCallId":"exec-16","status":"failed","_meta":{"terminal_exit":{"exit_code":1,"signal":null,"cancelled":"true","terminal_id":"exec-16"}}}`,
			},
		},
		{
			name: "conflicting terminal exit aliases block",
			frames: []string{
				`{"sessionUpdate":"tool_call","toolCallId":"exec-17","title":"Run","kind":"execute","status":"in_progress"}`,
				`{"sessionUpdate":"tool_call_update","toolCallId":"exec-17","status":"failed","_meta":{"terminal_exit":{"exit_code":1,"code":2,"signal":null,"terminal_id":"exec-17"}}}`,
			},
		},
		{
			name: "qualifying command with pending non-command tool still blocks",
			frames: []string{
				`{"sessionUpdate":"tool_call","toolCallId":"exec-18","title":"Run","kind":"execute","status":"in_progress"}`,
				`{"sessionUpdate":"tool_call_update","toolCallId":"exec-18","status":"failed","_meta":{"terminal_exit":{"exit_code":1,"signal":null,"terminal_id":"exec-18"}}}`,
				`{"sessionUpdate":"tool_call","toolCallId":"pending-read","title":"Read","kind":"read","status":"in_progress"}`,
			},
			known: 1, pending: true,
		},
		{
			name: "conflicting terminal exits block",
			frames: []string{
				`{"sessionUpdate":"tool_call","toolCallId":"exec-8","title":"Run","kind":"execute","status":"in_progress"}`,
				`{"sessionUpdate":"tool_call_update","toolCallId":"exec-8","status":"completed","_meta":{"terminal_exit":{"exit_code":1,"signal":null,"terminal_id":"exec-8"}}}`,
				`{"sessionUpdate":"tool_call_update","toolCallId":"exec-8","status":"completed","_meta":{"terminal_exit":{"exit_code":0,"signal":null,"terminal_id":"exec-8"}}}`,
			},
		},
		{
			name: "failed non-command tool blocks",
			frames: []string{
				`{"sessionUpdate":"tool_call","toolCallId":"read-failed","title":"Read","kind":"read","status":"failed"}`,
			},
		},
		{
			name: "mcp execute with terminal metadata blocks",
			frames: []string{
				`{"sessionUpdate":"tool_call","toolCallId":"mcp-1","title":"MCP","kind":"execute","status":"completed","_meta":{"is_mcp_tool_call":true},"rawInput":{"server":"tools","tool":"lookup","arguments":{}}}`,
				`{"sessionUpdate":"tool_call_update","toolCallId":"mcp-1","status":"completed","_meta":{"terminal_exit":{"exit_code":1,"signal":null,"terminal_id":"mcp-1"}}}`,
			},
		},
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
			require.Equal(t, tc.known, snapshot.KnownFailedShellTools)
			require.Equal(t, tc.completed, snapshot.CompletedTools)
			require.Equal(t, tc.pending, snapshot.PendingTools)
			if !tc.safe && tc.known == 0 {
				require.True(t, snapshot.FailedTools || snapshot.UnknownOutcomes,
					"blocked command evidence must keep the recovery fence")
			}
		})
	}
}

// @covers AC-PLATFORM-TURN-CONTINUITY-003.6
func TestCapacityContinuationSnapshotV2RemoteRoundTrip(t *testing.T) {
	snapshot := &streams.CapacityContinuationSnapshot{
		Support: streams.CapacityContinuationCodexLiveSessionV2, PromptGeneration: 7,
		EvidenceComplete: true, KnownFailedShellTools: 2,
	}
	event := streams.AgentEvent{Type: streams.EventTypeError, PromptGeneration: 7, CapacityContinuation: snapshot}
	raw, err := json.Marshal(event)
	require.NoError(t, err)
	var decoded streams.AgentEvent
	require.NoError(t, json.Unmarshal(raw, &decoded))
	require.Equal(t, *snapshot, *decoded.CapacityContinuation)
	require.True(t, decoded.CapacityContinuation.SafeFor(7))
}
