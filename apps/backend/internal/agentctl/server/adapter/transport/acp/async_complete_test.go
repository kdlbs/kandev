package acp

import (
	"context"
	"testing"
	"time"

	acp "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

func TestHandleACPUpdate_AsyncMonitorTextWithoutPromptEmitsIdleComplete(t *testing.T) {
	setAsyncTurnCompleteIdleForTest(t, 10*time.Millisecond)
	a := newTestAdapter()
	t.Cleanup(func() { _ = a.Close() })

	a.handleACPUpdate(makeNotification("s-monitor", acp.SessionUpdate{
		AgentMessageChunk: &acp.SessionUpdateAgentMessageChunk{
			Content: acp.TextBlock("monitor finished without a prompt response"),
		},
	}))

	first := readAdapterEvent(t, a, 100*time.Millisecond)
	if first.Type != streams.EventTypeMessageChunk {
		t.Fatalf("first event type = %q, want %q", first.Type, streams.EventTypeMessageChunk)
	}

	complete := readAdapterEvent(t, a, 250*time.Millisecond)
	if complete.Type != streams.EventTypeComplete {
		t.Fatalf("second event type = %q, want %q", complete.Type, streams.EventTypeComplete)
	}
	if complete.SessionID != "s-monitor" {
		t.Errorf("complete SessionID = %q, want s-monitor", complete.SessionID)
	}
	if complete.Data["synthetic_reason"] != "async_turn_idle" {
		t.Errorf("synthetic_reason = %v, want async_turn_idle", complete.Data["synthetic_reason"])
	}
}

func TestHandleACPUpdate_DoesNotEmitIdleCompleteWhilePromptActive(t *testing.T) {
	setAsyncTurnCompleteIdleForTest(t, 10*time.Millisecond)
	a := newTestAdapter()
	t.Cleanup(func() { _ = a.Close() })
	_, turn := a.registerPromptTurn(context.Background())
	defer a.clearPromptTurn(turn)

	a.handleACPUpdate(makeNotification("s-prompt", acp.SessionUpdate{
		AgentMessageChunk: &acp.SessionUpdateAgentMessageChunk{
			Content: acp.TextBlock("normal prompt chunk"),
		},
	}))

	first := readAdapterEvent(t, a, 100*time.Millisecond)
	if first.Type != streams.EventTypeMessageChunk {
		t.Fatalf("first event type = %q, want %q", first.Type, streams.EventTypeMessageChunk)
	}

	select {
	case ev := <-a.updatesCh:
		t.Fatalf("unexpected event while prompt active: %+v", ev)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestAsyncTurnComplete_CancelledByRealPromptCompletion(t *testing.T) {
	setAsyncTurnCompleteIdleForTest(t, 50*time.Millisecond)
	a := newTestAdapter()
	t.Cleanup(func() { _ = a.Close() })

	a.handleACPUpdate(makeNotification("s-cancel", acp.SessionUpdate{
		AgentMessageChunk: &acp.SessionUpdateAgentMessageChunk{
			Content: acp.TextBlock("async chunk"),
		},
	}))

	first := readAdapterEvent(t, a, 100*time.Millisecond)
	if first.Type != streams.EventTypeMessageChunk {
		t.Fatalf("first event type = %q, want %q", first.Type, streams.EventTypeMessageChunk)
	}

	a.cancelAsyncTurnComplete("s-cancel")

	select {
	case ev := <-a.updatesCh:
		t.Fatalf("unexpected event after cancel: %+v", ev)
	case <-time.After(150 * time.Millisecond):
	}
}

func TestAsyncTurnComplete_CancelledByPromptStart(t *testing.T) {
	setAsyncTurnCompleteIdleForTest(t, 50*time.Millisecond)
	a := newTestAdapter()
	t.Cleanup(func() { _ = a.Close() })

	a.handleACPUpdate(makeNotification("s-start", acp.SessionUpdate{
		AgentMessageChunk: &acp.SessionUpdateAgentMessageChunk{
			Content: acp.TextBlock("async chunk before prompt"),
		},
	}))

	first := readAdapterEvent(t, a, 100*time.Millisecond)
	if first.Type != streams.EventTypeMessageChunk {
		t.Fatalf("first event type = %q, want %q", first.Type, streams.EventTypeMessageChunk)
	}

	a.beginPromptTurn("s-start")

	select {
	case ev := <-a.updatesCh:
		t.Fatalf("unexpected event after prompt start: %+v", ev)
	case <-time.After(150 * time.Millisecond):
	}
}

func setAsyncTurnCompleteIdleForTest(t *testing.T, d time.Duration) {
	t.Helper()
	previous := asyncTurnCompleteIdle
	asyncTurnCompleteIdle = d
	t.Cleanup(func() {
		asyncTurnCompleteIdle = previous
	})
}

func readAdapterEvent(t *testing.T, a *Adapter, timeout time.Duration) AgentEvent {
	t.Helper()
	select {
	case ev := <-a.updatesCh:
		return ev
	case <-time.After(timeout):
		t.Fatalf("timed out waiting for adapter event after %s", timeout)
		return AgentEvent{}
	}
}
