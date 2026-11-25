package acpv2

import (
	"testing"
	"time"

	wire "github.com/coder/acp-go-sdk/experimental/v2"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/stretchr/testify/require"
)

func TestWireCancelRemainsResponsiveDuringUpdateBackpressure(t *testing.T) {
	fixture := newWireFixture(t)
	fixture.initialize()
	fixture.newSession(fixtureSession)
	done, request := fixture.prompt()
	fixture.reply(request, map[string]any{"messageId": "accepted"})
	fixture.state("running", "")
	updates := fixture.adapter.updates
	for range cap(updates) {
		updates <- streams.AgentEvent{Type: streams.EventTypeReasoning}
	}
	fixture.chunk(fixtureSession, "answer", "first delta")
	require.Eventually(t, func() bool {
		if !fixture.adapter.mu.TryLock() {
			return false
		}
		defer fixture.adapter.mu.Unlock()
		key := makeMessageKey(fixtureSession, "answer", wire.UpdateAgentMessageChunk)
		return textContent(fixture.adapter.state.messages[key]) == "first delta"
	}, time.Second, 5*time.Millisecond, "a projected delta must release the state lock before waiting for delivery")

	stopped := asyncCall(func() error { return fixture.adapter.Cancel(fixture.ctx) })
	notification := fixture.read("session/cancel")
	require.Equal(t, fixtureSession, notification.Params["sessionId"])
	require.NoError(t, fixture.wait(stopped))
	fixture.chunk(fixtureSession, "answer", "last delta")
	fixture.state("idle", "cancelled")
	for range cap(updates) {
		fixture.event(streams.EventTypeReasoning)
	}
	require.Equal(t, "first delta", fixture.event(streams.EventTypeMessageChunk).Text)
	require.Equal(t, "last delta", fixture.event(streams.EventTypeMessageChunk).Text)
	require.Equal(t, "cancelled", fixture.event(streams.EventTypeComplete).Data["stop_reason"])
	require.NoError(t, fixture.wait(done))
}

func TestWireIdleWithoutStopReasonRetainsRuntimeWithUnknownOutcome(t *testing.T) {
	fixture := newWireFixture(t)
	fixture.initialize()
	fixture.newSession(fixtureSession)
	done, request := fixture.prompt()
	fixture.reply(request, map[string]any{"messageId": "accepted"})
	fixture.state("running", "")
	fixture.chunk(fixtureSession, "answer", "partial answer")
	fixture.state("idle", "")
	require.Equal(t, "partial answer", fixture.event(streams.EventTypeMessageChunk).Text)
	terminal := fixture.event(streams.EventTypeError)
	require.Equal(t, "unknown", terminal.Data["stop_reason"])
	require.Equal(t, streams.PromptFailureDispositionRetainRuntime, terminal.PromptFailureDisposition)
	var failure *streams.ReportedPromptFailure
	require.ErrorAs(t, fixture.wait(done), &failure)

	done, request = fixture.prompt()
	fixture.reply(request, map[string]any{"messageId": "successor"})
	fixture.state("running", "")
	fixture.state("idle", "end_turn")
	require.Equal(t, "successor", fixture.event(streams.EventTypeComplete).OperationID)
	require.NoError(t, fixture.wait(done))
}
