package acpv2

import (
	"testing"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/stretchr/testify/require"
)

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
