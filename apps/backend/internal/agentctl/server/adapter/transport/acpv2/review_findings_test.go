package acpv2

import (
	"testing"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWireBackgroundChildBindsLateLaunchReference(t *testing.T) {
	fixture := backgroundFixture(t)
	fixture.update(fixtureSession, map[string]any{
		"sessionUpdate": "subagent_update", "sessionId": "child", "title": "Inspect files",
		"description": "Find the entry point", "state": map[string]any{"state": "running"},
	})
	firstChild := fixture.event(streams.EventTypeToolCall)
	fallbackCallID := entityID(fixtureSession, "subagent:child")
	require.Equal(t, fallbackCallID, firstChild.ToolCallID)

	fixture.update(fixtureSession, backgroundState(backgroundEntries("child"), true))
	initial := backgroundEventsBeforeMarker(fixture, "late-child-reference-initial")
	require.Len(t, initial, 1)
	require.Empty(t, initial[0].BackgroundWork.SourceCallID)

	fixture.update(fixtureSession, map[string]any{
		"sessionUpdate": "tool_call_update", "toolCallId": "child-launch", "title": "Spawn child",
		"status": "completed", "content": []any{map[string]any{"type": "session", "sessionId": "child"}},
	})
	launchCallID := entityID(fixtureSession, "child-launch")
	updated := backgroundEventsBeforeMarker(fixture, "late-child-reference-updated")
	assert.Len(t, updated, 1)
	if len(updated) == 1 {
		assert.Equal(t, launchCallID, updated[0].BackgroundWork.SourceCallID)
	}

	fixture.chunk("child", "child-message", "child output")
	childMessage := fixture.event(streams.EventTypeMessageChunk)
	assert.Equal(t, launchCallID, childMessage.ParentToolCallID)
}

func TestWireListedTerminalDoesNotAdoptLaterTurnOrigin(t *testing.T) {
	fixture := backgroundFixture(t)
	fixture.adapter.turn = &promptTurn{generation: 41, submissionID: "submit-a"}
	fixture.update(fixtureSession, backgroundState(backgroundEntries("terminal"), true))
	listed := backgroundEventsBeforeMarker(fixture, "listed-terminal-before-frame")
	require.Len(t, listed, 1)

	fixture.adapter.turn = &promptTurn{generation: 42, submissionID: "submit-b"}
	fixture.update(fixtureSession, map[string]any{
		"sessionUpdate": "terminal_update", "terminalId": "terminal", "command": "go test ./...",
	})
	observed := backgroundEventsBeforeMarker(fixture, "terminal-frame-during-later-turn")
	require.Len(t, observed, 1)
	assert.Zero(t, observed[0].PromptGeneration)
	assert.Empty(t, observed[0].DeliverySubmissionID)
}

func TestWireListedChildDoesNotAdoptLaterTurnOrigin(t *testing.T) {
	fixture := backgroundFixture(t)
	fixture.adapter.turn = &promptTurn{generation: 41, submissionID: "submit-a"}
	fixture.update(fixtureSession, backgroundState(backgroundEntries("child"), true))
	listed := backgroundEventsBeforeMarker(fixture, "listed-child-before-update")
	require.Len(t, listed, 1)

	fixture.adapter.turn = &promptTurn{generation: 42, submissionID: "submit-b"}
	fixture.update(fixtureSession, map[string]any{
		"sessionUpdate": "subagent_update", "sessionId": "child", "title": "Inspect files",
		"description": "Find the entry point", "state": map[string]any{"state": "running"},
	})
	observed := backgroundEventsBeforeMarker(fixture, "child-update-during-later-turn")
	require.Len(t, observed, 1)
	assert.Zero(t, observed[0].PromptGeneration)
	assert.Empty(t, observed[0].DeliverySubmissionID)
}

func TestMergeBackgroundWorkPreservesRetainedOriginWithoutReplacingReplay(t *testing.T) {
	key := backgroundWorkKey{session: fixtureSession, kind: streams.WorkloadKindShell, id: "terminal"}
	retainedOrigin := promptIdentity{generation: 41, submissionID: "submit-a"}
	replayedObservation := streams.WorkloadRunObservation{
		SessionID: string(fixtureSession), WorkID: "replayed-work", State: streams.RunStateUnknown, Title: "replayed title",
	}
	projection := projection{backgroundWork: map[backgroundWorkKey]backgroundWorkRecord{
		key: {listed: true, observation: replayedObservation},
	}}
	projection.mergeBackgroundWork(map[backgroundWorkKey]backgroundWorkRecord{
		key: {listed: true, observation: streams.WorkloadRunObservation{Title: "retained title"}, origin: retainedOrigin},
	})

	merged := projection.backgroundWork[key]
	assert.Equal(t, replayedObservation, merged.observation)
	assert.Equal(t, retainedOrigin, merged.origin)

	newerOrigin := promptIdentity{generation: 43, submissionID: "submit-c"}
	projection.backgroundWork[key] = backgroundWorkRecord{observation: replayedObservation, origin: newerOrigin}
	projection.mergeBackgroundWork(map[backgroundWorkKey]backgroundWorkRecord{key: {origin: retainedOrigin}})
	assert.Equal(t, newerOrigin, projection.backgroundWork[key].origin)
}
