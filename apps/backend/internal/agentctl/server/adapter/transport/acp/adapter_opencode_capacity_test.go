package acp

import (
	"context"
	"encoding/json"
	"io"
	"sync/atomic"
	"testing"
	"time"

	sdk "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/stretchr/testify/require"
)

const openCodeCapacityTerminalMessage = "Internal error: Upstream request failed: [ttft_admission_refused] the cell is at capacity; retry shortly (after 21 s)"

const openCodeCapacitySessionID = "opencode-capacity-session"

type openCodeCapacityAgent struct {
	burstAgent
	conn           *sdk.AgentSideConnection
	promptRuns     atomic.Int32
	newRuns        atomic.Int32
	nativeRetries  atomic.Int32
	terminalFailed atomic.Bool
}

func (f *openCodeCapacityAgent) NewSession(context.Context, sdk.NewSessionRequest) (sdk.NewSessionResponse, error) {
	f.newRuns.Add(1)
	return sdk.NewSessionResponse{SessionId: openCodeCapacitySessionID}, nil
}

func (f *openCodeCapacityAgent) Prompt(ctx context.Context, _ sdk.PromptRequest) (sdk.PromptResponse, error) {
	if f.promptRuns.Add(1) == 1 {
		for _, frame := range []struct {
			raw        string
			nativeMeta bool
		}{
			{raw: `{"sessionId":"` + openCodeCapacitySessionID + `","update":{"sessionUpdate":"tool_call","toolCallId":"read-1","title":"Read","kind":"read","status":"completed"}}`},
			{raw: `{"sessionId":"` + openCodeCapacitySessionID + `","update":{"sessionUpdate":"session_info_update","_meta":{"opencode/retry":{"attempt":2,"error":{"message":"Upstream request failed: [ttft_admission_refused] the cell is at capacity; retry shortly (after 21 s)","status":429,"type":"provider.rate-limit"},"nextRetryAt":"2026-10-28T22:26:22.452Z"}}}}`, nativeMeta: true},
			{raw: `{"sessionId":"` + openCodeCapacitySessionID + `","update":{"sessionUpdate":"session_info_update","_meta":{"opencode/retry":null}}}`, nativeMeta: true},
		} {
			var notification sdk.SessionNotification
			if err := json.Unmarshal([]byte(frame.raw), &notification); err != nil {
				return sdk.PromptResponse{}, err
			}
			if frame.nativeMeta {
				f.nativeRetries.Add(1)
			}
			if err := f.conn.SessionUpdate(ctx, notification); err != nil {
				return sdk.PromptResponse{}, err
			}
		}
		f.terminalFailed.Store(true)
		return sdk.PromptResponse{}, &sdk.RequestError{Code: -32603, Message: openCodeCapacityTerminalMessage}
	}
	return sdk.PromptResponse{StopReason: sdk.StopReasonEndTurn}, nil
}

// @covers AC-PLATFORM-TURN-CONTINUITY-001.1, AC-PLATFORM-TURN-CONTINUITY-001.2, AC-PLATFORM-TURN-CONTINUITY-001.3
func TestOpenCodeCapacityPromptFailureRetainsRuntime(t *testing.T) {
	a := newTestAdapterForAgent(openCodeAgentID)
	clientToAgentR, clientToAgentW := io.Pipe()
	agentToClientR, agentToClientW := io.Pipe()
	fake := &openCodeCapacityAgent{}
	require.NoError(t, a.Connect(clientToAgentW, agentToClientR))
	fake.conn = sdk.NewAgentSideConnection(fake, agentToClientW, clientToAgentR)
	t.Cleanup(func() {
		_ = a.Close()
		_ = clientToAgentW.Close()
		_ = agentToClientW.Close()
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, a.Initialize(ctx))
	_, err := a.NewSession(ctx, nil)
	require.NoError(t, err)
	connection := a.acpConn

	require.NoError(t, a.Prompt(ctx, "first prompt", nil, 41), "a retained capacity failure must not surface as a prompt error")
	failure := waitForEventType(t, a, streams.EventTypeError)
	require.Equal(t, uint64(41), failure.PromptGeneration)
	require.Equal(t, streams.PromptFailureDispositionRetainRuntime, failure.PromptFailureDisposition)
	require.NotNil(t, failure.ProviderError)
	require.Contains(t, failure.ProviderError.Message, "ttft_admission_refused")
	require.NotNil(t, failure.CapacityContinuation)
	require.Equal(t, streams.CapacityContinuationOpenCodeLiveSessionV1, failure.CapacityContinuation.Support)
	require.Equal(t, uint16(1), failure.CapacityContinuation.CompletedTools)
	require.True(t, failure.CapacityContinuation.SafeFor(41))
	require.Equal(t, int32(2), fake.nativeRetries.Load(), "native retry metadata is advisory and must not consume a host dispatch")
	require.True(t, fake.terminalFailed.Load())

	require.NoError(t, a.Prompt(ctx, "follow-up prompt", nil, 42))
	require.Equal(t, uint64(42), waitForEventType(t, a, streams.EventTypeComplete).PromptGeneration)
	require.Same(t, connection, a.acpConn, "the continuation must reuse the initialized runtime")
	require.Equal(t, int32(2), fake.promptRuns.Load())
	require.Equal(t, int32(1), fake.newRuns.Load(), "no initialize, load, resume, or new may run during recovery")
	select {
	case <-connection.Done():
		t.Fatal("ACP connection closed after the capacity failure")
	default:
	}
}

// @covers AC-PLATFORM-TURN-CONTINUITY-003.1, AC-PLATFORM-TURN-CONTINUITY-003.6
func TestOpenCodeCapacityContinuationEvidence(t *testing.T) {
	const generation = uint64(7)
	cases := []struct {
		name          string
		frames        []string
		safe          bool
		unknown       bool
		background    bool
		completedTool uint16
	}{
		{
			name: "completed read execute edit and MCP tools",
			frames: []string{
				`{"sessionUpdate":"tool_call","toolCallId":"read-1","title":"Read","kind":"read","status":"completed"}`,
				`{"sessionUpdate":"tool_call","toolCallId":"shell-1","title":"Run","kind":"execute","status":"completed"}`,
				`{"sessionUpdate":"tool_call","toolCallId":"write-1","title":"Write","kind":"edit","status":"completed"}`,
				`{"sessionUpdate":"tool_call","toolCallId":"mcp-1","title":"MCP","kind":"execute","status":"completed","_meta":{"is_mcp_tool_call":true},"rawInput":{"server":"tools","tool":"lookup","arguments":{}}}`,
			},
			safe: true, completedTool: 4,
		},
		{name: "output only"},
		{
			name: "completed and pending",
			frames: []string{
				`{"sessionUpdate":"tool_call","toolCallId":"done-1","title":"Read","kind":"read","status":"completed"}`,
				`{"sessionUpdate":"tool_call","toolCallId":"pending-1","title":"Run","kind":"execute","status":"pending"}`,
			},
			completedTool: 1,
		},
		{
			name: "completed and failed",
			frames: []string{
				`{"sessionUpdate":"tool_call","toolCallId":"done-1","title":"Read","kind":"read","status":"completed"}`,
				`{"sessionUpdate":"tool_call","toolCallId":"failed-1","title":"Run","kind":"execute","status":"failed"}`,
			},
			completedTool: 1,
		},
		{
			name: "completed and unknown status",
			frames: []string{
				`{"sessionUpdate":"tool_call","toolCallId":"done-1","title":"Read","kind":"read","status":"completed"}`,
				`{"sessionUpdate":"tool_call","toolCallId":"unknown-1","title":"Run","kind":"execute","status":"cancelled"}`,
			},
			completedTool: 1,
			unknown:       true,
		},
		{
			name: "completed and duplicate tool id",
			frames: []string{
				`{"sessionUpdate":"tool_call","toolCallId":"same-1","title":"Read","kind":"read","status":"completed"}`,
				`{"sessionUpdate":"tool_call","toolCallId":"same-1","title":"Read","kind":"read","status":"completed"}`,
			},
			completedTool: 1,
			unknown:       true,
		},
		{
			name: "completed and unfinished OpenCode child session",
			frames: []string{
				`{"sessionUpdate":"tool_call","toolCallId":"done-1","title":"Read","kind":"read","status":"completed"}`,
				`{"sessionUpdate":"tool_call","toolCallId":"subagent-1","title":"task","kind":"other","status":"in_progress"}`,
			},
			background:    true,
			completedTool: 1,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a, turn := newOpenCodeCapacityPromptTurn(t, generation)
			for _, frame := range tc.frames {
				var update sdk.SessionUpdate
				require.NoError(t, json.Unmarshal([]byte(frame), &update))
				a.handleACPUpdate(makeNotification(openCodeCapacitySessionID, update), generation)
				_ = drainEvents(a)
			}
			snapshot := a.capacityContinuationSnapshot(turn, true)
			require.NotNil(t, snapshot)
			require.Equal(t, streams.CapacityContinuationOpenCodeLiveSessionV1, snapshot.Support)
			require.Equal(t, tc.safe, snapshot.SafeFor(generation))
			require.Equal(t, tc.unknown, snapshot.UnknownOutcomes)
			require.Equal(t, tc.background, snapshot.UnaccountedBackground)
			require.Equal(t, tc.completedTool, snapshot.CompletedTools)
			if !tc.safe && tc.completedTool > 0 {
				require.True(t, snapshot.PendingTools || snapshot.FailedTools || snapshot.UnknownOutcomes ||
					snapshot.PermissionPending || snapshot.UnaccountedBackground,
					"unsafe evidence must name a blocking condition")
			}
		})
	}
}

// Native retry metadata describes OpenCode's own active retry. The host must
// stay idle until the terminal prompt result settles.
//
// @covers AC-PLATFORM-TURN-CONTINUITY-003.1, AC-PLATFORM-TURN-CONTINUITY-003.9
func TestOpenCodeCapacityNativeRetryDoesNotDispatch(t *testing.T) {
	a, turn := newOpenCodeCapacityPromptTurn(t, 21)
	a.handleACPUpdate(makeNotification(openCodeCapacitySessionID, sdk.SessionUpdate{
		ToolCall: &sdk.SessionUpdateToolCall{
			ToolCallId: "read-1", Kind: sdk.ToolKindRead, Status: sdk.ToolCallStatusCompleted,
		},
	}), 21)
	_ = drainEvents(a)

	for _, raw := range []string{
		`{"sessionId":"` + openCodeCapacitySessionID + `","update":{"sessionUpdate":"session_info_update","_meta":{"opencode/retry":{"attempt":2,"error":{"message":"Upstream request failed: [ttft_admission_refused] the cell is at capacity; retry shortly (after 21 s)","status":429,"type":"provider.rate-limit"},"nextRetryAt":"2026-10-28T22:26:22.452Z"}}}}`,
		`{"sessionId":"` + openCodeCapacitySessionID + `","update":{"sessionUpdate":"session_info_update","_meta":{"opencode/retry":null}}}`,
	} {
		var notification sdk.SessionNotification
		require.NoError(t, json.Unmarshal([]byte(raw), &notification))
		a.handleACPUpdate(notification, 21)
		for _, event := range drainEvents(a) {
			require.NotEqual(t, streams.EventTypeError, event.Type, "native retry metadata is not a terminal failure")
			require.NotEqual(t, streams.EventTypeComplete, event.Type, "native retry metadata is not prompt completion")
		}
	}
	require.Equal(t, uint64(21), turn.promptGeneration)
	require.True(t, a.capacityContinuationSnapshot(turn, true).SafeFor(21),
		"advisory native retry metadata must not consume or poison completed evidence")
}

// @covers AC-PLATFORM-TURN-CONTINUITY-003.6, AC-PLATFORM-TURN-CONTINUITY-003.9
func TestOpenCodeCapacityRetentionIsCapacityOnly(t *testing.T) {
	tests := []struct {
		name       string
		message    string
		wantRetain bool
	}{
		{name: "terminal admission refusal", message: openCodeCapacityTerminalMessage, wantRetain: true},
		{name: "auth required", message: "Internal error: unauthorized"},
		{name: "generic internal error", message: "Internal error: runtime exploded"},
		{name: "rate limited", message: "AI_APICallError: Rate limit exceeded: free-models-per-min"},
		{name: "quota limited", message: "AI_APICallError: 5-hour usage limit reached"},
		{name: "unrelated refusal token", message: "Internal error: Upstream request failed: [budget_exhausted] the cell is at capacity"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a := newTestAdapterForAgent(openCodeAgentID)
			_, ok := a.retainableACPApplicationError(&sdk.RequestError{Code: -32603, Message: tc.message})
			require.Equal(t, tc.wantRetain, ok)
		})
	}
}

func newOpenCodeCapacityPromptTurn(t *testing.T, generation uint64) (*Adapter, *promptTurnState) {
	t.Helper()
	a, turn := newCursorPromptTurn(t, generation)
	a.agentID = openCodeAgentID
	a.normalizer = NewNormalizer(openCodeAgentID)
	a.dialect = newOpenCodeACPDialect()
	a.capabilities.LoadSession = true
	a.sessionID = openCodeCapacitySessionID
	return a, turn
}

// @covers AC-PLATFORM-TURN-CONTINUITY-003.1
func TestOpenCodeCapacityEvidenceRequiresDrainedNotifications(t *testing.T) {
	a, turn := newOpenCodeCapacityPromptTurn(t, 31)
	a.handleACPUpdate(makeNotification(openCodeCapacitySessionID, sdk.SessionUpdate{
		ToolCall: &sdk.SessionUpdateToolCall{
			ToolCallId: "read-1", Kind: sdk.ToolKindRead, Status: sdk.ToolCallStatusCompleted,
		},
	}), 31)
	_ = drainEvents(a)

	require.Nil(t, a.capacityContinuationSnapshot(turn, false),
		"queued notifications can still change the ledger, so evidence must be withheld")
	snapshot := a.capacityContinuationSnapshot(turn, true)
	require.NotNil(t, snapshot)
	require.True(t, snapshot.SafeFor(31))
}

// @covers AC-AGENTS-BACKGROUND-WORK-007
// @covers AC-PLATFORM-TURN-CONTINUITY-003.1
func TestOpenCodeCapacityDetachedLaunchBlocksContinuation(t *testing.T) {
	for _, inspection := range []bool{false, true} {
		name := "background inspection off"
		if inspection {
			name = "background inspection on"
		}
		t.Run(name, func(t *testing.T) {
			a, turn := newOpenCodeCapacityPromptTurn(t, 51)
			a.backgroundWorkEnabled = inspection
			a.normalizer.backgroundWorkEnabled = inspection
			a.handleACPUpdate(makeNotification(openCodeCapacitySessionID, sdk.SessionUpdate{
				ToolCall: &sdk.SessionUpdateToolCall{
					ToolCallId: "read-1", Kind: sdk.ToolKindRead, Status: sdk.ToolCallStatusCompleted,
				},
			}), 51)
			call, update := openCodeLaunchFrames("call-ci", "shell-ci", "gh pr checks 42 --watch")
			a.handleACPUpdate(makeNotification(openCodeCapacitySessionID, sdk.SessionUpdate{ToolCall: call}), 51)
			a.handleACPUpdate(makeNotification(openCodeCapacitySessionID, sdk.SessionUpdate{ToolCallUpdate: update}), 51)
			_ = drainEvents(a)

			a.mu.RLock()
			_, tracked := a.activeToolCalls["call-ci"]
			a.mu.RUnlock()
			require.Equal(t, inspection, tracked,
				"presentation retains the launch only when background inspection is enabled")

			snapshot := a.capacityContinuationSnapshot(turn, true)
			require.NotNil(t, snapshot)
			require.True(t, snapshot.UnaccountedBackground,
				"a completed launch tool is not completed workload evidence")
			require.False(t, snapshot.SafeFor(51))

			// The observed job exit is terminal workload evidence, not the launch
			// tool's completion.
			completed := sdk.ToolCallStatusCompleted
			a.handleACPUpdate(makeNotification(openCodeCapacitySessionID, sdk.SessionUpdate{
				ToolCallUpdate: &sdk.SessionToolCallUpdate{
					ToolCallId: "call-ci", Status: &completed,
					RawOutput: map[string]any{"output": "1 check passed", "exit_code": 0},
				},
			}), 51)
			_ = drainEvents(a)
			snapshot = a.capacityContinuationSnapshot(turn, true)
			require.False(t, snapshot.UnaccountedBackground)
			require.True(t, snapshot.SafeFor(51))
		})
	}
}

// @covers AC-AGENTS-BACKGROUND-WORK-007
// @covers AC-PLATFORM-TURN-CONTINUITY-003.1
func TestOpenCodeCapacityDetachedLaunchFailsClosed(t *testing.T) {
	tests := []struct {
		name   string
		frames func(t *testing.T, a *Adapter)
	}{
		{
			name: "launch result without workload metadata",
			frames: func(t *testing.T, a *Adapter) {
				call, _ := openCodeLaunchFrames("call-ci", "shell-ci", "sleep 30")
				a.handleACPUpdate(makeNotification(openCodeCapacitySessionID, sdk.SessionUpdate{ToolCall: call}), 7)
				completed := sdk.ToolCallStatusCompleted
				a.handleACPUpdate(makeNotification(openCodeCapacitySessionID, sdk.SessionUpdate{
					ToolCallUpdate: &sdk.SessionToolCallUpdate{
						ToolCallId: "call-ci", Status: &completed,
						RawOutput: map[string]any{"output": "Background process started"},
					},
				}), 7)
			},
		},
		{
			name: "terminal exit for a different shell",
			frames: func(t *testing.T, a *Adapter) {
				call, update := openCodeLaunchFrames("call-ci", "shell-ci", "sleep 30")
				a.handleACPUpdate(makeNotification(openCodeCapacitySessionID, sdk.SessionUpdate{ToolCall: call}), 7)
				a.handleACPUpdate(makeNotification(openCodeCapacitySessionID, sdk.SessionUpdate{ToolCallUpdate: update}), 7)
				completed := sdk.ToolCallStatusCompleted
				a.handleACPUpdate(makeNotification(openCodeCapacitySessionID, sdk.SessionUpdate{
					ToolCallUpdate: &sdk.SessionToolCallUpdate{
						ToolCallId: "call-ci", Status: &completed,
						RawOutput: map[string]any{
							"exit_code": 0,
							"metadata":  map[string]any{"shellID": "shell-other", "status": "completed"},
						},
					},
				}), 7)
			},
		},
		{
			name: "conflicting running identity",
			frames: func(t *testing.T, a *Adapter) {
				call, update := openCodeLaunchFrames("call-ci", "shell-ci", "sleep 30")
				a.handleACPUpdate(makeNotification(openCodeCapacitySessionID, sdk.SessionUpdate{ToolCall: call}), 7)
				a.handleACPUpdate(makeNotification(openCodeCapacitySessionID, sdk.SessionUpdate{ToolCallUpdate: update}), 7)
				a.handleACPUpdate(makeNotification(openCodeCapacitySessionID, sdk.SessionUpdate{
					ToolCallUpdate: &sdk.SessionToolCallUpdate{
						ToolCallId: "call-ci",
						RawOutput: map[string]any{"metadata": map[string]any{
							"shellID": "shell-other", "status": "running",
						}},
					},
				}), 7)
			},
		},
		{
			name: "terminal exit without an observed launch identity",
			frames: func(t *testing.T, a *Adapter) {
				a.handleACPUpdate(makeNotification(openCodeCapacitySessionID, sdk.SessionUpdate{
					ToolCall: &sdk.SessionUpdateToolCall{
						ToolCallId: "call-ci", Kind: sdk.ToolKindExecute, Status: sdk.ToolCallStatusCompleted,
						RawInput: map[string]any{"command": "sleep 30", "background": true},
					},
				}), 7)
				completed := sdk.ToolCallStatusCompleted
				a.handleACPUpdate(makeNotification(openCodeCapacitySessionID, sdk.SessionUpdate{
					ToolCallUpdate: &sdk.SessionToolCallUpdate{
						ToolCallId: "call-ci", Status: &completed,
						RawOutput: map[string]any{"output": "finished", "exit_code": 0},
					},
				}), 7)
			},
		},
		{
			name: "conflicting identity at terminal evidence",
			frames: func(t *testing.T, a *Adapter) {
				call, update := openCodeLaunchFrames("call-ci", "shell-ci", "sleep 30")
				a.handleACPUpdate(makeNotification(openCodeCapacitySessionID, sdk.SessionUpdate{ToolCall: call}), 7)
				a.handleACPUpdate(makeNotification(openCodeCapacitySessionID, sdk.SessionUpdate{ToolCallUpdate: update}), 7)
				completed := sdk.ToolCallStatusCompleted
				a.handleACPUpdate(makeNotification(openCodeCapacitySessionID, sdk.SessionUpdate{
					ToolCallUpdate: &sdk.SessionToolCallUpdate{
						ToolCallId: "call-ci", Status: &completed,
						RawOutput: map[string]any{
							"exit_code": 0,
							"metadata":  map[string]any{"shellID": "shell-a", "shellId": "shell-b", "status": "completed"},
						},
					},
				}), 7)
			},
		},
		{
			name: "conflicting identity without status",
			frames: func(t *testing.T, a *Adapter) {
				call, update := openCodeLaunchFrames("call-ci", "shell-ci", "sleep 30")
				a.handleACPUpdate(makeNotification(openCodeCapacitySessionID, sdk.SessionUpdate{ToolCall: call}), 7)
				a.handleACPUpdate(makeNotification(openCodeCapacitySessionID, sdk.SessionUpdate{ToolCallUpdate: update}), 7)
				completed := sdk.ToolCallStatusCompleted
				a.handleACPUpdate(makeNotification(openCodeCapacitySessionID, sdk.SessionUpdate{
					ToolCallUpdate: &sdk.SessionToolCallUpdate{
						ToolCallId: "call-ci", Status: &completed,
						RawOutput: map[string]any{
							"exit_code": 0,
							"metadata":  map[string]any{"shellID": "shell-a", "shellId": "shell-b"},
						},
					},
				}), 7)
			},
		},
		{
			name: "background flag on a non-execute call",
			frames: func(t *testing.T, a *Adapter) {
				a.handleACPUpdate(makeNotification(openCodeCapacitySessionID, sdk.SessionUpdate{
					ToolCall: &sdk.SessionUpdateToolCall{
						ToolCallId: "call-other", Kind: sdk.ToolKindOther,
						Status:   sdk.ToolCallStatusCompleted,
						RawInput: map[string]any{"background": true, "command": "sleep 30"},
					},
				}), 7)
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a, turn := newOpenCodeCapacityPromptTurn(t, 7)
			a.handleACPUpdate(makeNotification(openCodeCapacitySessionID, sdk.SessionUpdate{
				ToolCall: &sdk.SessionUpdateToolCall{
					ToolCallId: "read-1", Kind: sdk.ToolKindRead, Status: sdk.ToolCallStatusCompleted,
				},
			}), 7)
			tc.frames(t, a)
			_ = drainEvents(a)
			snapshot := a.capacityContinuationSnapshot(turn, true)
			require.NotNil(t, snapshot)
			require.False(t, snapshot.SafeFor(7))
			require.True(t, snapshot.UnaccountedBackground,
				"missing or contradictory workload evidence must fail closed")
		})
	}
}

// @covers AC-AGENTS-BACKGROUND-WORK-007
func TestOpenCodeCapacityOrdinaryForegroundExecuteStaysSafe(t *testing.T) {
	a, turn := newOpenCodeCapacityPromptTurn(t, 61)
	a.handleACPUpdate(makeNotification(openCodeCapacitySessionID, sdk.SessionUpdate{
		ToolCall: &sdk.SessionUpdateToolCall{
			ToolCallId: "shell-1", Kind: sdk.ToolKindExecute, Status: sdk.ToolCallStatusCompleted,
			RawInput: map[string]any{"command": "go test ./..."},
		},
	}), 61)
	_ = drainEvents(a)
	snapshot := a.capacityContinuationSnapshot(turn, true)
	require.False(t, snapshot.UnaccountedBackground)
	require.True(t, snapshot.SafeFor(61))
}
