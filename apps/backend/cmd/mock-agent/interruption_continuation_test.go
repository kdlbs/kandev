package main

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	acp "github.com/coder/acp-go-sdk"
	"github.com/stretchr/testify/require"
)

const mockContinuationPrompt = "continue"

func TestMockInterruptionContinuationRequiresFixturePrefix(t *testing.T) {
	a := newTransportLostTestAgent()
	a.conn = newCapturingUpdater()
	for _, prompt := range []string{mockContinuationReadScenario, mockContinuationOutputScenario, mockContinuationWriteScenario, mockContinuationUnknownScenario, "continue"} {
		_, _, handled := a.handleMockInterruptionContinuation(t.Context(), "ordinary", prompt)
		require.False(t, handled, "ordinary prompts cannot enter an interruption fixture")
	}
}

func TestMockInterruptionContinuationCancelledCompletion(t *testing.T) {
	sid := acp.SessionId(t.Name())
	_ = os.Remove(mockContinuationPath(sid))
	a := newTransportLostTestAgent()
	a.conn = newCapturingUpdater()
	t.Cleanup(func() { _ = os.Remove(mockContinuationPath(sid)) })
	_, _, _ = a.handleMockInterruptionContinuation(t.Context(), sid, "/continuation-"+mockContinuationReadScenario)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	response, err, handled := a.handleMockInterruptionContinuation(ctx, sid, mockContinuationPrompt)
	require.True(t, handled)
	require.NoError(t, err)
	require.Equal(t, acp.StopReasonCancelled, response.StopReason)
}

func TestMockInterruptionContinuationUnknownOutcome(t *testing.T) {
	sid := acp.SessionId(t.Name())
	_ = os.Remove(mockContinuationPath(sid))
	t.Cleanup(func() { _ = os.Remove(mockContinuationPath(sid)) })
	a := newTransportLostTestAgent()
	updates := newCapturingUpdater()
	a.conn = updates
	_, err, handled := a.handleMockInterruptionContinuation(t.Context(), sid, "/continuation-unknown")
	require.True(t, handled)
	require.Error(t, err)
	started := make(map[acp.ToolCallId]bool)
	unknownCompletion := false
	for _, note := range updates.notes {
		if call := note.Update.ToolCall; call != nil {
			started[call.ToolCallId] = true
		}
		if update := note.Update.ToolCallUpdate; update != nil && update.Status != nil && *update.Status == acp.ToolCallStatusCompleted {
			unknownCompletion = unknownCompletion || !started[update.ToolCallId]
		}
	}
	require.True(t, unknownCompletion, "unknown fixture must contain an uncorrelated outcome, not a completed foreground tool")
}

func TestMockInterruptionContinuationShellOutcome(t *testing.T) {
	sid := acp.SessionId(t.Name())
	_ = os.Remove(mockContinuationPath(sid))
	t.Cleanup(func() { _ = os.Remove(mockContinuationPath(sid)) })
	a := newTransportLostTestAgent()
	updates := newCapturingUpdater()
	a.conn = updates
	_, err, handled := a.handleMockInterruptionContinuation(t.Context(), sid, "/continuation-shell")
	require.True(t, handled)
	require.Error(t, err)
	completedShell := false
	shellIDs := make(map[acp.ToolCallId]bool)
	for _, note := range updates.notes {
		if call := note.Update.ToolCall; call != nil && call.Kind == acp.ToolKindExecute {
			shellIDs[call.ToolCallId] = true
			completedShell = call.Status == acp.ToolCallStatusCompleted
		}
		if update := note.Update.ToolCallUpdate; update != nil && update.Status != nil && *update.Status == acp.ToolCallStatusCompleted {
			completedShell = completedShell || shellIDs[update.ToolCallId]
		}
	}
	require.True(t, completedShell)
}

func TestMockInterruptionContinuationRestoresSameConversation(t *testing.T) {
	for _, scenario := range []string{mockContinuationOutputScenario, mockContinuationReadScenario, mockContinuationWriteScenario, mockContinuationPendingScenario, mockContinuationUnknownScenario} {
		t.Run(scenario, func(t *testing.T) {
			sid := acp.SessionId(t.Name())
			_ = os.Remove(mockContinuationPath(sid))
			a := newTransportLostTestAgent()
			a.conn = newCapturingUpdater()
			_, err := a.LoadSession(t.Context(), acp.LoadSessionRequest{SessionId: sid})
			require.NoError(t, err)
			t.Cleanup(func() { _, _ = a.CloseSession(t.Context(), acp.CloseSessionRequest{SessionId: sid}) })
			_, err = a.Prompt(t.Context(), acp.PromptRequest{SessionId: sid, Prompt: []acp.ContentBlock{acp.TextBlock("/continuation-" + scenario)}})
			var request *acp.RequestError
			require.ErrorAs(t, err, &request)
			require.Contains(t, request.Message, "peer disconnected")
			require.Contains(t, a.conn.(*capturingUpdater).textMessages(), "Mock interruption: partial history preserved.\n")
			restored := newTransportLostTestAgent()
			updater := newCapturingUpdater()
			restored.conn = updater
			_, err = restored.LoadSession(t.Context(), acp.LoadSessionRequest{SessionId: sid})
			require.NoError(t, err)
			_, err = restored.Prompt(t.Context(), acp.PromptRequest{SessionId: sid, Prompt: []acp.ContentBlock{acp.TextBlock(mockContinuationPrompt)}})
			require.NoError(t, err)
			require.True(t, strings.Contains(strings.Join(updater.textMessages(), ""), "Mock continuation complete: original=1 continuation=1 native="+string(sid)))
		})
	}
}

func TestMockInterruptionContinuationAcceptedCancelAndClose(t *testing.T) {
	const sid acp.SessionId = "continuation-cancel-owned-fixture"
	_ = os.Remove(mockContinuationPath(sid))
	a := newTransportLostTestAgent()
	a.conn = newCapturingUpdater()
	t.Cleanup(func() { _, _ = a.CloseSession(context.Background(), acp.CloseSessionRequest{SessionId: sid}) })
	_, _, handled := a.handleMockInterruptionContinuation(t.Context(), sid, "/continuation-"+mockContinuationReadHoldScenario)
	require.True(t, handled)
	updater := newCapturingUpdater()
	a.conn = updater
	done := make(chan acp.PromptResponse, 1)
	go func() {
		response, err := a.Prompt(t.Context(), acp.PromptRequest{SessionId: sid, Prompt: []acp.ContentBlock{acp.TextBlock(mockContinuationPrompt)}})
		if err == nil {
			done <- response
		}
	}()
	select {
	case <-updater.textSeen:
	case <-time.After(time.Second):
		t.Fatal("continuation not accepted")
	}
	require.Contains(t, strings.Join(updater.textMessages(), ""), "Mock continuation accepted: original=1 continuation=1")
	require.NoError(t, a.Cancel(t.Context(), acp.CancelNotification{SessionId: sid}))
	select {
	case response := <-done:
		require.Equal(t, acp.StopReasonCancelled, response.StopReason)
	case <-time.After(time.Second):
		t.Fatal("accepted continuation did not stop")
	}
	_, err := a.CloseSession(t.Context(), acp.CloseSessionRequest{SessionId: sid})
	require.NoError(t, err)
	_, err = os.Stat(mockContinuationPath(sid))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestMockInterruptionContinuationRestoreOutcomes(t *testing.T) {
	for _, scenario := range []string{mockContinuationReadRestoreTransientScenario, mockContinuationReadRestoreHardScenario, mockContinuationReadAmbiguousScenario} {
		t.Run(scenario, func(t *testing.T) {
			sid := acp.SessionId(t.Name())
			_ = os.Remove(mockContinuationPath(sid))
			a := newTransportLostTestAgent()
			a.conn = newCapturingUpdater()
			t.Cleanup(func() { _, _ = a.CloseSession(context.Background(), acp.CloseSessionRequest{SessionId: sid}) })
			_, initialErr := a.Prompt(t.Context(), acp.PromptRequest{SessionId: sid, Prompt: []acp.ContentBlock{acp.TextBlock("/continuation-" + scenario)}})
			require.Error(t, initialErr, "the fixture must first produce an attested read interruption")
			restored := newTransportLostTestAgent()
			restored.conn = newCapturingUpdater()
			_, err := restored.LoadSession(t.Context(), acp.LoadSessionRequest{SessionId: sid})
			if scenario == mockContinuationReadRestoreHardScenario {
				require.ErrorContains(t, err, "saved conversation unavailable")
				return
			}
			if scenario == mockContinuationReadRestoreTransientScenario {
				require.ErrorContains(t, err, "network is unreachable")
				_, err = restored.LoadSession(t.Context(), acp.LoadSessionRequest{SessionId: sid})
			}
			require.NoError(t, err)
			_, err = restored.Prompt(t.Context(), acp.PromptRequest{SessionId: sid, Prompt: []acp.ContentBlock{acp.TextBlock(mockContinuationPrompt)}})
			if scenario == mockContinuationReadAmbiguousScenario {
				require.ErrorContains(t, err, "acceptance uncertain")
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestMockInterruptionContinuationConsumesEpisode(t *testing.T) {
	for _, scenario := range []string{mockContinuationReadScenario, mockContinuationReadHoldScenario} {
		t.Run(scenario, func(t *testing.T) {
			sid := acp.SessionId(t.Name())
			_ = os.Remove(mockContinuationPath(sid))
			t.Cleanup(func() { _ = os.Remove(mockContinuationPath(sid)) })
			a := newTransportLostTestAgent()
			a.conn = newCapturingUpdater()
			_, _, handled := a.handleMockInterruptionContinuation(t.Context(), sid, "/continuation-"+scenario)
			require.True(t, handled)
			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			_, _, handled = a.handleMockInterruptionContinuation(ctx, sid, "continue")
			require.True(t, handled)
			_, _, handled = a.handleMockInterruptionContinuation(ctx, sid, "continue")
			require.False(t, handled, "a later human continue must use the ordinary prompt path")
		})
	}
}
