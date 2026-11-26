package acpv2

import (
	"encoding/base64"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/stretchr/testify/require"
)

func (f *wireFixture) nextEvent() streams.AgentEvent {
	f.t.Helper()
	select {
	case event := <-f.adapter.Updates():
		return event
	case <-f.ctx.Done():
		f.t.Fatalf("waiting for event: %v", f.ctx.Err())
		return streams.AgentEvent{}
	}
}

func backgroundFixture(t *testing.T) *wireFixture {
	t.Helper()
	fixture := newWireFixture(t)
	fixture.adapter.cfg.AgentBackgroundWorkEnabled = true
	fixture.initialize()
	fixture.newSession(fixtureSession)
	return fixture
}

func addBackgroundTerminal(fixture *wireFixture, terminalID string) string {
	fixture.t.Helper()
	fixture.update(fixtureSession, map[string]any{
		"sessionUpdate": "terminal_update", "terminalId": terminalID,
		"command": "go test ./...", "cwd": "/workspace", "output": map[string]any{"data": encodeTerminalOutput("initial output")},
	})
	fixture.event(streams.EventTypeToolCall)
	fixture.update(fixtureSession, map[string]any{
		"sessionUpdate": "tool_call_update", "toolCallId": "shell-launch", "title": "Run tests",
		"status": "completed", "content": []any{map[string]any{"type": "terminal", "terminalId": terminalID}},
	})
	return fixture.event(streams.EventTypeToolCall).ToolCallID
}

func encodeTerminalOutput(output string) string {
	return base64.StdEncoding.EncodeToString([]byte(output))
}

func addTerminalOutputChunk(fixture *wireFixture, terminalID, output string) {
	fixture.t.Helper()
	fixture.update(fixtureSession, map[string]any{
		"sessionUpdate": "terminal_output_chunk", "terminalId": terminalID,
		"data": encodeTerminalOutput(output),
	})
	fixture.event(streams.EventTypeToolUpdate)
}

func addBackgroundChild(fixture *wireFixture, childID, callID string) string {
	fixture.t.Helper()
	fixture.update(fixtureSession, map[string]any{
		"sessionUpdate": "tool_call_update", "toolCallId": callID, "title": "Spawn child",
		"status": "completed", "content": []any{map[string]any{"type": "session", "sessionId": childID}},
	})
	call := fixture.event(streams.EventTypeToolCall)
	fixture.update(fixtureSession, map[string]any{
		"sessionUpdate": "subagent_update", "sessionId": childID, "title": "Inspect files",
		"description": "Find the entry point", "state": map[string]any{"state": "running"},
	})
	child := fixture.event(streams.EventTypeToolUpdate)
	require.Equal(fixture.t, call.ToolCallID, child.ToolCallID)
	return child.ToolCallID
}

func backgroundState(background any, include bool) map[string]any {
	state := map[string]any{"sessionUpdate": "state_update", "state": "idle"}
	if include {
		state["background"] = background
	}
	return state
}

func backgroundEventsBeforeMarker(fixture *wireFixture, markerID string) []streams.AgentEvent {
	fixture.t.Helper()
	fixture.chunk(fixtureSession, markerID, markerID)
	var events []streams.AgentEvent
	for {
		event := fixture.nextEvent()
		if event.Type == streams.EventTypeMessageChunk && event.ProtocolMessageID == entityID(fixtureSession, markerID) {
			return events
		}
		if event.Type == streams.EventTypeBackgroundWorkUpdated {
			events = append(events, event)
		}
	}
}

func backgroundEntries(ids ...string) []any {
	entries := make([]any, 0, len(ids))
	for _, id := range ids {
		if id == "child" || id == "child-a" || id == "child-b" {
			entries = append(entries, map[string]any{"type": "subagent", "sessionId": id})
		} else {
			entries = append(entries, map[string]any{"type": "terminal", "terminalId": id})
		}
	}
	return entries
}

func TestWireProjectsBackgroundSnapshot(t *testing.T) {
	fixture := backgroundFixture(t)
	terminalCall := addBackgroundTerminal(fixture, "terminal")
	childCall := addBackgroundChild(fixture, "child", "child-launch")
	addTerminalOutputChunk(fixture, "terminal", " first")
	addTerminalOutputChunk(fixture, "terminal", " second")
	fixture.update(fixtureSession, backgroundState(backgroundEntries("terminal", "child"), true))
	events := backgroundEventsBeforeMarker(fixture, "snapshot-marker")
	require.Len(t, events, 2)
	byKind := make(map[streams.WorkloadKind]streams.WorkloadRunObservation, len(events))
	for _, event := range events {
		require.NotNil(t, event.BackgroundWork)
		byKind[event.BackgroundWork.Kind] = *event.BackgroundWork
		require.Equal(t, streams.RunStateRunning, event.BackgroundWork.State)
		require.NotEmpty(t, event.BackgroundWork.WorkID)
	}
	shell := byKind[streams.WorkloadKindShell]
	require.Equal(t, "go test ./...", shell.Title)
	require.Equal(t, terminalCall, shell.SourceCallID)
	require.Equal(t, "initial output first second", shell.Output)
	require.Equal(t, "snapshot", shell.Capabilities.Output)
	require.Empty(t, shell.Capabilities.Actions)
	child := byKind[streams.WorkloadKindSubagent]
	require.Equal(t, "Inspect files", child.Title)
	require.Equal(t, childCall, child.SourceCallID)
}

func TestWireBackgroundSnapshotsAreIdempotentAndReconcileChildren(t *testing.T) {
	fixture := backgroundFixture(t)
	childA := addBackgroundChild(fixture, "child-a", "launch-a")
	addBackgroundChild(fixture, "child-b", "launch-b")
	fixture.update(fixtureSession, backgroundState(backgroundEntries("child-a", "child-b"), true))
	initial := backgroundEventsBeforeMarker(fixture, "initial-marker")
	require.Len(t, initial, 2)
	var childAWorkID string
	for _, event := range initial {
		if event.BackgroundWork.SourceCallID == childA {
			childAWorkID = event.BackgroundWork.WorkID
		}
	}
	require.NotEmpty(t, childAWorkID)
	fixture.update(fixtureSession, backgroundState(backgroundEntries("child-a", "child-b"), true))
	require.Empty(t, backgroundEventsBeforeMarker(fixture, "repeat-marker-after-snapshot"))
	fixture.update(fixtureSession, backgroundState(backgroundEntries("child-b"), true))
	removed := backgroundEventsBeforeMarker(fixture, "removed-marker")
	require.Len(t, removed, 1)
	require.Equal(t, streams.WorkloadKindSubagent, removed[0].BackgroundWork.Kind)
	require.Equal(t, childA, removed[0].BackgroundWork.SourceCallID)
	require.Equal(t, streams.RunStateUnknown, removed[0].BackgroundWork.State)
	fixture.update(fixtureSession, backgroundState(backgroundEntries("child-a", "child-b"), true))
	returned := backgroundEventsBeforeMarker(fixture, "returned-marker")
	require.Len(t, returned, 1)
	require.Equal(t, childAWorkID, returned[0].BackgroundWork.WorkID)
	require.Equal(t, streams.RunStateRunning, returned[0].BackgroundWork.State)
}

func TestWireBackgroundUnknownAndEmptySnapshots(t *testing.T) {
	for _, test := range []struct {
		name    string
		value   any
		include bool
	}{
		{name: "omitted"}, {name: "null", include: true}, {name: "empty", value: []any{}, include: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := backgroundFixture(t)
			addBackgroundTerminal(fixture, "terminal")
			addBackgroundChild(fixture, "child", "child-launch")
			fixture.update(fixtureSession, backgroundState(backgroundEntries("terminal", "child"), true))
			require.Len(t, backgroundEventsBeforeMarker(fixture, "running-marker"), 2)
			fixture.update(fixtureSession, backgroundState(test.value, test.include))
			unknown := backgroundEventsBeforeMarker(fixture, "unknown-marker")
			require.Len(t, unknown, 2)
			for _, event := range unknown {
				require.Equal(t, streams.RunStateUnknown, event.BackgroundWork.State)
			}
			fixture.update(fixtureSession, backgroundState(test.value, test.include))
			require.Empty(t, backgroundEventsBeforeMarker(fixture, "unknown-repeat-marker"))
		})
	}
}

func TestWireSubagentBackgroundStateIsNormalized(t *testing.T) {
	fixture := backgroundFixture(t)
	addBackgroundChild(fixture, "child-a", "launch-a")
	addBackgroundChild(fixture, "child-b", "launch-b")
	fixture.update(fixtureSession, backgroundState(backgroundEntries("child-a", "child-b"), true))
	require.Len(t, backgroundEventsBeforeMarker(fixture, "children-running-marker"), 2)
	for _, test := range []struct {
		child string
		state string
		want  streams.RunState
	}{
		{child: "child-a", state: "requires_action", want: streams.RunStateWaiting},
		{child: "child-b", state: "unknown", want: streams.RunStateUnknown},
	} {
		fixture.update(fixtureSession, map[string]any{
			"sessionUpdate": "subagent_update", "sessionId": test.child,
			"state": map[string]any{"state": test.state},
		})
		fixture.event(streams.EventTypeToolUpdate)
		observation := fixture.event(streams.EventTypeBackgroundWorkUpdated).BackgroundWork
		require.Equal(t, test.want, observation.State)
	}
	fixture.update(fixtureSession, map[string]any{
		"sessionUpdate": "subagent_update", "sessionId": "child-a",
		"state": map[string]any{
			"state":      "idle",
			"background": []any{map[string]any{"type": "terminal", "terminalId": "child-terminal"}},
		},
	})
	fixture.event(streams.EventTypeToolUpdate)
	background := fixture.event(streams.EventTypeBackgroundWorkUpdated).BackgroundWork
	require.Equal(t, streams.RunStateRunning, background.State)
}

func TestWireTerminalExitUpdatesBackgroundWorkAfterDelisting(t *testing.T) {
	fixture := backgroundFixture(t)
	callID := addBackgroundTerminal(fixture, "terminal")
	fixture.update(fixtureSession, backgroundState(backgroundEntries("terminal"), true))
	initial := backgroundEventsBeforeMarker(fixture, "terminal-running-marker")
	require.Len(t, initial, 1)
	workID := initial[0].BackgroundWork.WorkID
	fixture.update(fixtureSession, backgroundState([]any{}, true))
	delisted := backgroundEventsBeforeMarker(fixture, "terminal-delisted-marker")
	require.Len(t, delisted, 1)
	require.Equal(t, streams.RunStateUnknown, delisted[0].BackgroundWork.State)
	fixture.update(fixtureSession, map[string]any{
		"sessionUpdate": "terminal_update", "terminalId": "terminal",
		"output": map[string]any{"data": encodeTerminalOutput("final output")}, "exitStatus": map[string]any{"exitCode": 17},
	})
	fixture.event(streams.EventTypeToolUpdate)
	completed := backgroundEventsBeforeMarker(fixture, "terminal-exit-marker")
	require.Len(t, completed, 1)
	require.Equal(t, workID, completed[0].BackgroundWork.WorkID)
	require.Equal(t, callID, completed[0].BackgroundWork.SourceCallID)
	require.Equal(t, streams.RunStateFailed, completed[0].BackgroundWork.State)
	require.Equal(t, "final output", completed[0].BackgroundWork.Output)
	require.NotNil(t, completed[0].BackgroundWork.ExitCode)
	require.Equal(t, 17, *completed[0].BackgroundWork.ExitCode)
}

func TestWireBackgroundWorkKeepsLaunchGenerationAcrossLaterObservations(t *testing.T) {
	fixture := backgroundFixture(t)
	fixture.adapter.turn = &promptTurn{generation: 41, submissionID: "submit-a"}
	addBackgroundTerminal(fixture, "terminal")
	addBackgroundChild(fixture, "child", "child-launch")
	fixture.adapter.turn = &promptTurn{generation: 42, submissionID: "submit-b"}

	fixture.update(fixtureSession, backgroundState(backgroundEntries("terminal", "child"), true))
	observed := backgroundEventsBeforeMarker(fixture, "late-list-marker")
	require.Len(t, observed, 2)
	for _, event := range observed {
		require.Equal(t, uint64(41), event.PromptGeneration)
		require.Equal(t, "submit-a", event.DeliverySubmissionID)
	}

	fixture.update(fixtureSession, map[string]any{
		"sessionUpdate": "terminal_update", "terminalId": "terminal",
		"output":     map[string]any{"data": encodeTerminalOutput("finished")},
		"exitStatus": map[string]any{"exitCode": 0},
	})
	fixture.event(streams.EventTypeToolUpdate)
	exited := backgroundEventsBeforeMarker(fixture, "late-exit-marker")
	require.Len(t, exited, 1)
	require.Equal(t, uint64(41), exited[0].PromptGeneration)
	require.Equal(t, "submit-a", exited[0].DeliverySubmissionID)
}

func TestWireBackgroundWorkLeavesLaunchGenerationUnsetWhenUnavailable(t *testing.T) {
	fixture := backgroundFixture(t)
	fixture.update(fixtureSession, backgroundState(backgroundEntries("terminal"), true))
	events := backgroundEventsBeforeMarker(fixture, "unattributed-marker")
	require.Len(t, events, 1)
	require.Zero(t, events[0].PromptGeneration)
	require.Empty(t, events[0].DeliverySubmissionID)
}

func TestWireBackgroundSnapshotRespectsFeatureToggle(t *testing.T) {
	fixture := newWireFixture(t)
	fixture.initialize()
	fixture.newSession(fixtureSession)
	addBackgroundTerminal(fixture, "terminal")
	addBackgroundChild(fixture, "child", "child-launch")
	fixture.update(fixtureSession, backgroundState(backgroundEntries("terminal", "child"), true))
	require.Empty(t, backgroundEventsBeforeMarker(fixture, "disabled-marker"))
}

func TestWireTerminalReferenceBeforeFrameKeepsInitialToolCallWhenDisabled(t *testing.T) {
	fixture := newWireFixture(t)
	fixture.initialize()
	fixture.newSession(fixtureSession)
	fixture.update(fixtureSession, map[string]any{
		"sessionUpdate": "tool_call_update", "toolCallId": "shell-launch", "title": "Run tests",
		"status": "completed", "content": []any{map[string]any{"type": "terminal", "terminalId": "terminal"}},
	})
	fixture.event(streams.EventTypeToolCall)
	fixture.update(fixtureSession, map[string]any{
		"sessionUpdate": "terminal_update", "terminalId": "terminal", "command": "go test ./...",
	})
	terminal := fixture.event(streams.EventTypeToolCall)
	require.Equal(t, entityID(fixtureSession, "terminal:terminal"), terminal.ToolCallID)
}

func TestWireMessageSnapshotsAndChildIDs(t *testing.T) {
	fixture := newWireFixture(t)
	fixture.initialize()
	fixture.newSession(fixtureSession)
	fixture.chunk(fixtureSession, "same-id", "streamed")
	first := fixture.event(streams.EventTypeMessageChunk)
	fixture.update(fixtureSession, map[string]any{"sessionUpdate": "agent_message", "messageId": "same-id"})
	fixture.update(fixtureSession, map[string]any{"sessionUpdate": "agent_message", "messageId": "same-id", "content": []any{
		map[string]any{"type": "text", "text": "streamed suffix"},
	}})
	suffix := fixture.event(streams.EventTypeMessageChunk)
	require.Equal(t, " suffix", suffix.Text)
	require.False(t, suffix.ReplaceMessageContent)
	fixture.update(fixtureSession, map[string]any{"sessionUpdate": "agent_message", "messageId": "same-id", "content": nil})
	cleared := fixture.event(streams.EventTypeMessageChunk)
	require.Equal(t, first.ProtocolMessageID, cleared.ProtocolMessageID)
	require.True(t, cleared.ReplaceMessageContent)
	require.Empty(t, cleared.Text)
	fixture.update(fixtureSession, map[string]any{"sessionUpdate": "subagent_update", "sessionId": "child", "title": "Child"})
	child := fixture.event(streams.EventTypeToolCall)
	fixture.chunk("child", "same-id", "child answer")
	childAnswer := fixture.event(streams.EventTypeMessageChunk)
	require.Equal(t, child.ToolCallID, childAnswer.ParentToolCallID)
	require.NotEqual(t, first.ProtocolMessageID, childAnswer.ProtocolMessageID)
}

func TestWireToolPatchesPreserveMissingFieldsAndClearNull(t *testing.T) {
	fixture := newWireFixture(t)
	fixture.initialize()
	fixture.newSession(fixtureSession)
	fixture.update(fixtureSession, map[string]any{
		"sessionUpdate": "tool_call_update", "toolCallId": "tool", "title": "Execute", "status": "in_progress",
	})
	start := fixture.event(streams.EventTypeToolCall)
	fixture.update(fixtureSession, map[string]any{
		"sessionUpdate": "tool_call_content_chunk", "toolCallId": "tool", "content": map[string]any{
			"type": "content", "content": map[string]any{"type": "text", "text": "result"},
		},
	})
	update := fixture.event(streams.EventTypeToolUpdate)
	require.Equal(t, start.ToolCallID, update.ToolCallID)
	require.Equal(t, "Execute", update.ToolTitle)
	require.Equal(t, "running", update.ToolStatus)
	require.Equal(t, "result", update.ToolCallContents[0].Content.Text)
	fixture.update(fixtureSession, map[string]any{
		"sessionUpdate": "tool_call_update", "toolCallId": "tool", "title": nil, "content": nil, "status": "completed",
	})
	end := fixture.event(streams.EventTypeToolUpdate)
	require.Empty(t, end.ToolTitle)
	require.Empty(t, end.ToolCallContents)
	require.Equal(t, "completed", end.ToolStatus)
}

func TestWireToolLocationsSupplyFilePaths(t *testing.T) {
	fixture := newWireFixture(t)
	fixture.initialize()
	fixture.newSession(fixtureSession)
	const location = "/workspace/example.go"
	fixture.update(fixtureSession, map[string]any{
		"sessionUpdate": "tool_call_update", "toolCallId": "edit-file",
		"name": "edit", "kind": "edit",
		"rawInput":  map[string]any{"old_str_1": "before", "new_str_1": "after"},
		"locations": []any{map[string]any{"path": location, "line": 4}},
	})
	start := fixture.event(streams.EventTypeToolCall)
	require.Equal(t, location, start.NormalizedPayload.ModifyFile().FilePath)

	fixture.update(fixtureSession, map[string]any{
		"sessionUpdate": "tool_call_update", "toolCallId": "edit-file", "status": toolStatusCompleted,
	})
	completed := fixture.event(streams.EventTypeToolUpdate)
	require.Equal(t, location, completed.NormalizedPayload.ModifyFile().FilePath)

	fixture.update(fixtureSession, map[string]any{
		"sessionUpdate": "tool_call_update", "toolCallId": "edit-file", "locations": nil,
	})
	cleared := fixture.event(streams.EventTypeToolUpdate)
	require.Empty(t, cleared.NormalizedPayload.ModifyFile().FilePath)
}

func TestWireSubagentPatchesPreserveMissingFieldsAndClearNull(t *testing.T) {
	fixture := newWireFixture(t)
	fixture.initialize()
	fixture.newSession(fixtureSession)
	fixture.update(fixtureSession, map[string]any{
		"sessionUpdate": "subagent_update", "sessionId": "child",
		"title": "Inspect files", "description": "Find the entry point",
	})
	start := fixture.event(streams.EventTypeToolCall)
	fixture.update(fixtureSession, map[string]any{
		"sessionUpdate": "subagent_update", "sessionId": "child",
		"state": map[string]any{"state": "idle"},
	})
	completed := fixture.event(streams.EventTypeToolUpdate)
	require.Equal(t, start.ToolCallID, completed.ToolCallID)
	require.Equal(t, "Inspect files", completed.ToolTitle)
	require.Equal(t, "Find the entry point", completed.NormalizedPayload.SubagentTask().Prompt)
	require.Equal(t, toolStatusCompleted, completed.ToolStatus)

	fixture.update(fixtureSession, map[string]any{
		"sessionUpdate": "subagent_update", "sessionId": "child", "description": nil,
	})
	cleared := fixture.event(streams.EventTypeToolUpdate)
	require.Equal(t, completed.ToolTitle, cleared.ToolTitle)
	require.Empty(t, cleared.NormalizedPayload.SubagentTask().Prompt)
	require.Equal(t, toolStatusCompleted, cleared.ToolStatus)
}
