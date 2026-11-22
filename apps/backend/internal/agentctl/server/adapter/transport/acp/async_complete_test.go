package acp

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	acp "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

// @covers AC-PLATFORM-PROMPT-COMPLETION-OWNERSHIP-001.12
func TestAsyncTurnCompleteWaitsForForegroundToolTerminal(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		setAsyncTurnCompleteIdleForTest(t, 10*time.Millisecond)
		a := newTestAdapter()
		defer func() { _ = a.Close() }()
		a.maybeScheduleAsyncTurnComplete(AgentEvent{Type: streams.EventTypeToolCall,
			SessionID: "session", ToolCallID: "foreground", ToolStatus: "in_progress"})
		a.maybeScheduleAsyncTurnComplete(AgentEvent{Type: streams.EventTypeMessageChunk,
			SessionID: "session", Text: "Tool is still working"})
		time.Sleep(50 * time.Millisecond)
		synctest.Wait()
		assertNoAdapterEvent(t, a, "while foreground tool remains active")
		a.maybeScheduleAsyncTurnComplete(AgentEvent{Type: streams.EventTypeToolUpdate,
			SessionID: "session", ToolCallID: "foreground", ToolStatus: toolStatusComplete})
		time.Sleep(50 * time.Millisecond)
		synctest.Wait()
		complete := readAdapterEvent(t, a, 100*time.Millisecond)
		if complete.Type != streams.EventTypeComplete {
			t.Fatalf("event = %q, want complete after foreground terminal", complete.Type)
		}
	})
}

func TestAsyncTurnCompleteWaitsForAllForegroundTools(t *testing.T) {
	for _, terminal := range []string{toolStatusComplete, toolStatusError, toolStatusCancelled} {
		t.Run(terminal, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				setAsyncTurnCompleteIdleForTest(t, 10*time.Millisecond)
				a := newTestAdapter()
				defer func() { _ = a.Close() }()
				for _, id := range []string{"one", "two"} {
					a.maybeScheduleAsyncTurnComplete(AgentEvent{Type: streams.EventTypeToolUpdate,
						SessionID: "session", ToolCallID: id, ToolStatus: toolStatusInProgress})
				}
				a.maybeScheduleAsyncTurnComplete(AgentEvent{Type: streams.EventTypeToolUpdate,
					SessionID: "session", ToolCallID: "one", ToolStatus: terminal})
				time.Sleep(50 * time.Millisecond)
				synctest.Wait()
				assertNoAdapterEvent(t, a, "while the second tool is active")
				a.maybeScheduleAsyncTurnComplete(AgentEvent{Type: streams.EventTypeToolUpdate,
					SessionID: "session", ToolCallID: "two", ToolStatus: terminal})
				time.Sleep(50 * time.Millisecond)
				synctest.Wait()
				if event := readAdapterEvent(t, a, 100*time.Millisecond); event.Type != streams.EventTypeComplete {
					t.Fatalf("event = %q, want complete", event.Type)
				}
				assertNoAdapterEvent(t, a, "after the single completion")
			})
		})
	}
}

func TestAsyncTurnCompleteBackgroundHandoffDoesNotBlockIdle(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		setAsyncTurnCompleteIdleForTest(t, 10*time.Millisecond)
		a := newTestAdapter()
		defer func() { _ = a.Close() }()
		event := AgentEvent{Type: streams.EventTypeToolCall, SessionID: "session",
			ToolCallID: "tool", ToolStatus: toolStatusInProgress}
		a.maybeScheduleAsyncTurnComplete(event)
		payload := streams.NewGeneric("other", nil)
		payload.SetBackgroundWorkIdentity(streams.BackgroundWorkKindShell, "work", true, false)
		event.Type, event.NormalizedPayload = streams.EventTypeToolUpdate, payload
		a.maybeScheduleAsyncTurnComplete(event)
		a.maybeScheduleAsyncTurnComplete(AgentEvent{Type: streams.EventTypeToolCall, SessionID: "session",
			ToolCallID: "nested", ParentToolCallID: "tool", ToolStatus: toolStatusInProgress})
		time.Sleep(50 * time.Millisecond)
		synctest.Wait()
		if got := readAdapterEvent(t, a, 100*time.Millisecond); got.Type != streams.EventTypeComplete {
			t.Fatalf("event = %q, want complete", got.Type)
		}
	})
}

func TestHandleACPUpdate_AsyncMonitorTextWithoutPromptEmitsIdleComplete(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		setAsyncTurnCompleteIdleForTest(t, 10*time.Millisecond)
		a := newTestAdapter()
		defer func() { _ = a.Close() }()

		a.handleACPUpdate(makeNotification("s-monitor", acp.SessionUpdate{
			AgentMessageChunk: &acp.SessionUpdateAgentMessageChunk{
				Content: acp.TextBlock("monitor finished without a prompt response"),
			},
		}), 0)

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
		_, turn := a.registerPromptTurn(context.Background(), 0)
		defer a.clearPromptTurn(turn)

		a.handleACPUpdate(makeNotification("s-prompt", acp.SessionUpdate{
			AgentMessageChunk: &acp.SessionUpdateAgentMessageChunk{
				Content: acp.TextBlock("normal prompt chunk"),
			},
		}), 0)

		first := readAdapterEvent(t, a, 100*time.Millisecond)
		if first.Type != streams.EventTypeMessageChunk {
			t.Fatalf("first event type = %q, want %q", first.Type, streams.EventTypeMessageChunk)
		}

		time.Sleep(50 * time.Millisecond)
		synctest.Wait()
		assertNoAdapterEvent(t, a, "while prompt active")
	})
}

func TestAsyncTurnComplete_TerminalToolUpdateDoesNotStartTurn(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		setAsyncTurnCompleteIdleForTest(t, 10*time.Millisecond)
		a := newTestAdapter()
		defer func() { _ = a.Close() }()

		a.maybeScheduleAsyncTurnComplete(AgentEvent{
			Type:       streams.EventTypeToolUpdate,
			SessionID:  "s-late-tool",
			ToolCallID: "tool-1",
			ToolStatus: toolStatusComplete,
		})

		time.Sleep(50 * time.Millisecond)
		synctest.Wait()
		assertNoAdapterEvent(t, a, "after a standalone terminal tool update")
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
		}), 0)

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
		}), 0)

		first := readAdapterEvent(t, a, 100*time.Millisecond)
		if first.Type != streams.EventTypeMessageChunk {
			t.Fatalf("first event type = %q, want %q", first.Type, streams.EventTypeMessageChunk)
		}

		a.beginPromptTurn("s-start")

		started := readAdapterEvent(t, a, 100*time.Millisecond)
		if started.Type != streams.EventTypeTurnStarted {
			t.Fatalf("event type = %q, want %q", started.Type, streams.EventTypeTurnStarted)
		}
		if started.SessionID != "s-start" {
			t.Errorf("turn_started SessionID = %q, want s-start", started.SessionID)
		}

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
		}), 0)
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
		}), 0)
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
	asyncTurnCompleteIdleMu.Lock()
	previous := asyncTurnCompleteIdle
	asyncTurnCompleteIdle = d
	asyncTurnCompleteIdleMu.Unlock()
	t.Cleanup(func() {
		asyncTurnCompleteIdleMu.Lock()
		asyncTurnCompleteIdle = previous
		asyncTurnCompleteIdleMu.Unlock()
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

func TestAsyncTurnCompletePreservesNestedToolLineage(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		setAsyncTurnCompleteIdleForTest(t, 10*time.Millisecond)
		a := newTestAdapter()
		defer func() { _ = a.Close() }()
		a.trackToolCallLineage("child", "parent")
		a.maybeScheduleAsyncTurnComplete(AgentEvent{Type: streams.EventTypeToolCall,
			SessionID: "session", ToolCallID: "child", ParentToolCallID: "parent", ToolStatus: toolStatusInProgress})
		title := "child still working"
		update := a.convertToolCallResultUpdate("session", &acp.SessionToolCallUpdate{ToolCallId: "child", Title: &title})
		if update.ParentToolCallID != "parent" {
			t.Errorf("child update lost remembered parent: %+v", update)
		}
		a.maybeScheduleAsyncTurnComplete(*update)
		time.Sleep(50 * time.Millisecond)
		synctest.Wait()
		if event := readAdapterEvent(t, a, 100*time.Millisecond); event.Type != streams.EventTypeComplete {
			t.Fatalf("event = %q, want native foreground idle", event.Type)
		}
	})
}

func TestAsyncRepliesWithoutProviderMessageIDsHaveDistinctStableScopes(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		setAsyncTurnCompleteIdleForTest(t, 10*time.Millisecond)
		a := newTestAdapter()
		defer func() { _ = a.Close() }()
		var previous string
		for reply := range 2 {
			var first string
			for range 2 {
				a.handleACPUpdate(makeNotification("session", acp.SessionUpdate{
					AgentMessageChunk: &acp.SessionUpdateAgentMessageChunk{Content: acp.TextBlock("native chunk")},
				}), 0)
				event := readAdapterEvent(t, a, 100*time.Millisecond)
				if event.ProtocolMessageID == "" {
					t.Fatal("native chunks need a journal-retained message identity")
				}
				if first != "" && first != event.ProtocolMessageID {
					t.Fatal("chunks of the same native reply changed identity")
				}
				first = event.ProtocolMessageID
			}
			if reply > 0 && first == previous {
				t.Fatal("separate native replies reused message identity")
			}
			previous = first
			time.Sleep(50 * time.Millisecond)
			synctest.Wait()
			if event := readAdapterEvent(t, a, 100*time.Millisecond); event.Type != streams.EventTypeComplete {
				t.Fatalf("event = %q, want complete", event.Type)
			}
		}
	})
}
