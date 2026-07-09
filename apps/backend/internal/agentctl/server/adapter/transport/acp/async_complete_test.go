package acp

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	acp "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

func TestHandleACPUpdate_AsyncMonitorTextWithoutPromptEmitsIdleComplete(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		setAsyncTurnCompleteIdleForTest(t, 10*time.Millisecond)
		a := newTestAdapter()
		defer func() { _ = a.Close() }()

		a.handleACPUpdate(makeNotification("s-monitor", acp.SessionUpdate{
			AgentMessageChunk: &acp.SessionUpdateAgentMessageChunk{
				Content: acp.TextBlock("monitor finished without a prompt response"),
			},
		}))

		first := readAdapterEvent(t, a, 100*time.Millisecond)
		if first.Type != streams.EventTypeMessageChunk {
			t.Fatalf("first event type = %q, want %q", first.Type, streams.EventTypeMessageChunk)
		}

		time.Sleep(50 * time.Millisecond)
		synctest.Wait()

		complete := readAdapterEvent(t, a, 100*time.Millisecond)
		if complete.Type != streams.EventTypeComplete {
			t.Fatalf("second event type = %q, want %q", complete.Type, streams.EventTypeComplete)
		}
		if complete.SessionID != "s-monitor" {
			t.Errorf("complete SessionID = %q, want s-monitor", complete.SessionID)
		}
		if complete.Data["synthetic_reason"] != "async_turn_idle" {
			t.Errorf("synthetic_reason = %v, want async_turn_idle", complete.Data["synthetic_reason"])
		}
	})
}

func TestHandleACPUpdate_DoesNotEmitIdleCompleteWhilePromptActive(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		setAsyncTurnCompleteIdleForTest(t, 10*time.Millisecond)
		a := newTestAdapter()
		defer func() { _ = a.Close() }()
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

		time.Sleep(50 * time.Millisecond)
		synctest.Wait()
		assertNoAdapterEvent(t, a, "while prompt active")
	})
}

func TestAsyncTurnComplete_CancelledByRealPromptCompletion(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		setAsyncTurnCompleteIdleForTest(t, 50*time.Millisecond)
		a := newTestAdapter()
		defer func() { _ = a.Close() }()

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

		time.Sleep(150 * time.Millisecond)
		synctest.Wait()
		assertNoAdapterEvent(t, a, "after cancel")
	})
}

func TestAsyncTurnComplete_CancelledByPromptStart(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		setAsyncTurnCompleteIdleForTest(t, 50*time.Millisecond)
		a := newTestAdapter()
		defer func() { _ = a.Close() }()

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

		time.Sleep(150 * time.Millisecond)
		synctest.Wait()
		assertNoAdapterEvent(t, a, "after prompt start")
	})
}

func TestAsyncTurnComplete_CancelledByNewSession(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		setAsyncTurnCompleteIdleForTest(t, 50*time.Millisecond)
		a, _ := setupConcurrencyFakeAgent(t)
		if err := a.Initialize(context.Background()); err != nil {
			t.Fatalf("Initialize: %v", err)
		}
		_ = drainEvents(a)

		a.handleACPUpdate(makeNotification("s-old", acp.SessionUpdate{
			AgentMessageChunk: &acp.SessionUpdateAgentMessageChunk{
				Content: acp.TextBlock("old session async chunk"),
			},
		}))
		first := readAdapterEvent(t, a, 100*time.Millisecond)
		if first.Type != streams.EventTypeMessageChunk {
			t.Fatalf("first event type = %q, want %q", first.Type, streams.EventTypeMessageChunk)
		}

		if _, err := a.NewSession(context.Background(), nil); err != nil {
			t.Fatalf("NewSession: %v", err)
		}
		_ = drainEvents(a)

		time.Sleep(150 * time.Millisecond)
		synctest.Wait()
		assertNoAdapterEvent(t, a, "after NewSession")
	})
}

func TestAsyncTurnComplete_CancelledByLoadSession(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		setAsyncTurnCompleteIdleForTest(t, 50*time.Millisecond)
		a, _ := setupConcurrencyFakeAgent(t)
		if err := a.Initialize(context.Background()); err != nil {
			t.Fatalf("Initialize: %v", err)
		}
		_ = drainEvents(a)
		a.capabilities.LoadSession = true

		a.handleACPUpdate(makeNotification("s-old", acp.SessionUpdate{
			AgentMessageChunk: &acp.SessionUpdateAgentMessageChunk{
				Content: acp.TextBlock("old session async chunk"),
			},
		}))
		first := readAdapterEvent(t, a, 100*time.Millisecond)
		if first.Type != streams.EventTypeMessageChunk {
			t.Fatalf("first event type = %q, want %q", first.Type, streams.EventTypeMessageChunk)
		}

		if err := a.LoadSession(context.Background(), "s-new", nil); err != nil {
			t.Fatalf("LoadSession: %v", err)
		}
		_ = drainEvents(a)

		time.Sleep(150 * time.Millisecond)
		synctest.Wait()
		assertNoAdapterEvent(t, a, "after LoadSession")
	})
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

func assertNoAdapterEvent(t *testing.T, a *Adapter, context string) {
	t.Helper()
	select {
	case ev := <-a.updatesCh:
		t.Fatalf("unexpected event %s: %+v", context, ev)
	default:
	}
}
