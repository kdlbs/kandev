package acp

import (
	"fmt"
	"io"
	"testing"
	"time"

	sdk "github.com/coder/acp-go-sdk"
	acpclient "github.com/kandev/kandev/internal/agentctl/server/acp"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/stretchr/testify/require"
)

func TestCursorBackgroundSubagentCapability(t *testing.T) {
	meta := handshakeMeta(t, cursorAgentID)
	if _, ok := meta["subagents"]; !ok {
		t.Fatal("Cursor child lifecycle missing from interactive handshake")
	}
}

// @covers AC-AGENTS-SUBAGENT-LIFECYCLE-005.2
// @covers AC-AGENTS-SUBAGENT-LIFECYCLE-005.3
func TestCursorBackgroundSubagentLifecycle(t *testing.T) {
	const spawned = `{"sessionUpdate":"subagent_spawned","subagentSessionId":"child","task":"Run checks","_meta":{"cursor":{"toolCallId":"launch"}}}`
	const launched = `{"sessionUpdate":"tool_call_update","toolCallId":"launch","status":"completed","rawOutput":{"isBackground":true}}`
	const finished = `{"sessionUpdate":"subagent_state_update","subagentSessionId":"child","state":"completed","_meta":{"cursor":{"toolCallId":"launch"}}}`
	for _, order := range []struct{ name, first, second string }{{"spawn first", spawned, launched}, {"tool first", launched, spawned}} {
		t.Run(order.name, func(t *testing.T) {
			a := newCursorShellWatchTestAdapter(t, true, "parent", 1)
			input, peer := io.Pipe()
			defer func() { _ = input.Close() }()
			defer func() { _ = peer.Close() }()
			client := cursorSessionClient{Client: acpclient.NewClient(acpclient.WithUpdateHandler(a.enqueueACPUpdate)), adapter: a}
			sdk.NewClientSideConnection(client, io.Discard, input)
			send := func(session, update string) {
				t.Helper()
				_, err := fmt.Fprintf(peer, `{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":%q,"update":%s}}`+"\n", session, update)
				if err != nil {
					t.Fatal(err)
				}
			}
			const message = `{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"report"}}`
			send("parent", order.first)
			send("parent", order.second)
			send("child", message)
			send("parent", finished)
			send("parent", finished)
			send("parent", spawned)
			send("parent", message)
			expectCursorChildEvents(t, a)
		})
	}
}
func expectCursorChildEvents(t *testing.T, a *Adapter) {
	t.Helper()
	var observations []*streams.WorkloadRunObservation
	timeout := time.After(5 * time.Second)
	for {
		select {
		case event := <-a.Updates():
			if event.BackgroundWork != nil {
				observations = append(observations, event.BackgroundWork)
			}
			if event.Type != streams.EventTypeMessageChunk {
				continue
			}
			require.Equal(t, "parent", event.SessionID)
			require.Len(t, observations, 2)
			first, last := observations[0], observations[1]
			require.Equal(t, streams.WorkloadKindSubagent, first.Kind)
			require.Equal(t, "Run checks", first.Title)
			require.Equal(t, streams.RunStateRunning, first.State)
			require.Equal(t, streams.RunStateCompleted, last.State)
			require.Equal(t, first.WorkID, last.WorkID)
			require.Equal(t, "launch", last.SourceCallID)
			return
		case <-timeout:
			t.Fatal("parent report missing")
		}
	}
}

// @covers AC-AGENTS-SUBAGENT-LIFECYCLE-005.4
func TestCursorBackgroundSubagentRequiresOwnedSpawnAndLaunch(t *testing.T) {
	a := newCursorShellWatchTestAdapter(t, true, "parent", 1)
	spawn := cursorChildFrame{SessionID: "parent"}
	spawn.Update.SessionUpdate = "subagent_spawned"
	spawn.Update.SubagentSessionID = "child"
	spawn.Update.Meta.Cursor.ToolCallID = "call"
	launch := cursorChildFrame{SessionID: "parent", background: true}
	launch.Update.Meta.Cursor.ToolCallID = "call"
	a.observeCursorChild(launch, 1)
	foreign := spawn
	foreign.SessionID = "other"
	a.observeCursorChild(foreign, 1)
	require.Empty(t, backgroundObservations(drainEvents(a)))
	a.observeCursorChild(spawn, 1)
	runs := backgroundObservations(drainEvents(a))
	require.Len(t, runs, 1)
	a.mu.Lock()
	retired := a.advanceNativeGenerationLocked("new-parent")
	a.sessionID = "new-parent"
	a.mu.Unlock()
	require.Len(t, retired, 1)
	require.Equal(t, runs[0].WorkID, retired[0].BackgroundWork.WorkID)
	require.Equal(t, streams.RunStateUnknown, retired[0].BackgroundWork.State)
	a.observeCursorChild(spawn, 1)
	require.Empty(t, a.cursorChildren)
}

func TestCursorReusedLaunchWithOverlappingChildren(t *testing.T) {
	a := newCursorShellWatchTestAdapter(t, true, "parent", 1)
	spawn := cursorChildFrame{SessionID: "parent"}
	spawn.Update.SessionUpdate = cursorChildSpawned
	spawn.Update.Meta.Cursor.ToolCallID = "launch"
	launch := cursorChildFrame{SessionID: "parent", background: true}
	launch.Update.Meta.Cursor.ToolCallID = "launch"
	for i, id := range []string{"first", "second"} {
		turn := uint64(i + 1)
		spawn.Update.SubagentSessionID = id
		a.observeCursorChild(spawn, turn)
		a.observeCursorChild(launch, turn)
		a.sweepCursorTaskMetaOnPromptEnd("parent", turn)
	}
	running := backgroundObservations(drainEvents(a))
	require.Len(t, running, 2)
	finish := spawn
	finish.Update.SessionUpdate = cursorChildStateUpdate
	finish.Update.State = "completed"
	for i, id := range []string{"first", "second"} {
		finish.Update.SubagentSessionID = id
		a.observeCursorChild(finish, 3)
		ended := backgroundObservations(drainEvents(a))
		require.Len(t, ended, 1)
		require.Equal(t, running[i].WorkID, ended[0].WorkID)
	}
	a.sweepCursorTaskMetaOnPromptEnd("parent", 3)
	require.Empty(t, a.cursorChildren)
	// A duplicate from the old turn must not claim the next launch binding.
	a.observeCursorChild(finish, 4)
	a.observeCursorChild(spawn, 4)
	a.observeCursorChild(launch, 4)
	require.Empty(t, backgroundObservations(drainEvents(a)))
	spawn.Update.SubagentSessionID = "third"
	a.observeCursorChild(spawn, 4)
	third := backgroundObservations(drainEvents(a))
	require.Len(t, third, 1)
	require.Equal(t, streams.RunStateRunning, third[0].State)

}

func TestCursorChildTerminalState(t *testing.T) {
	for _, test := range []struct {
		wire  string
		state streams.RunState
	}{
		{"completed", streams.RunStateCompleted}, {"failed", streams.RunStateFailed},
		{"cancelled", streams.RunStateInterrupted}, {"disconnected", streams.RunStateUnknown},
	} {
		child := cursorChild{spawned: true, background: true}
		frame := cursorChildFrame{}
		frame.Update.SessionUpdate = cursorChildStateUpdate
		frame.Update.State = test.wire
		require.True(t, child.apply(frame))
		require.Equal(t, test.state, child.observedState())
	}
}
