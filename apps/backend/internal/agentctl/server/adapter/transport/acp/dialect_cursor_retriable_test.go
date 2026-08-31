package acp

import (
	"context"
	"testing"
	"time"

	acpsdk "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/acpcompat"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

const cursorRetriableStreamResetChunk = "Error: RetriableError: HTTP/2 stream closed with error code CANCEL (0x8)"

func cursorMessageNotification(sessionID, text string) acpsdk.SessionNotification {
	return makeNotification(sessionID, acpsdk.SessionUpdate{
		AgentMessageChunk: &acpsdk.SessionUpdateAgentMessageChunk{
			Content: acpsdk.TextBlock(text),
		},
	})
}

func newCursorPromptTurn(t *testing.T, generation uint64) (*Adapter, *promptTurnState) {
	t.Helper()
	a := newTestAdapter()
	a.agentID = acpcompat.CursorAgentID
	a.normalizer = NewNormalizer(acpcompat.CursorAgentID)
	a.dialect = newACPDialect(acpcompat.CursorAgentID)
	t.Cleanup(func() { _ = a.Close() })
	_, turn := a.registerPromptTurn(context.Background(), generation)
	t.Cleanup(func() { a.clearPromptTurn(turn) })
	return a, turn
}

func TestCursorRetriableStreamResetSuppressesExactCurrentChunk(t *testing.T) {
	a, _ := newCursorPromptTurn(t, 7)

	a.handleACPUpdate(cursorMessageNotification("session-1", cursorRetriableStreamResetChunk), 7)

	if events := drainEvents(a); len(events) != 0 {
		t.Fatalf("exact Cursor control chunk emitted %d events: %+v", len(events), events)
	}
}

func TestCursorRetriableStreamResetRejectsUnrelatedEvidence(t *testing.T) {
	tests := []struct {
		name       string
		agentID    string
		generation uint64
		text       string
	}{
		{
			name:       "prose before marker",
			generation: 7,
			text:       "The provider returned Error: RetriableError: HTTP/2 stream closed with error code CANCEL (0x8)",
		},
		{
			name:       "unrelated explanation before fingerprint",
			generation: 7,
			text:       "Error: RetriableError: explanation for CANCEL (0x8)",
		},
		{
			name:       "partial marker",
			generation: 7,
			text:       "Error: RetriableError:",
		},
		{
			name:       "missing transport fingerprint",
			generation: 7,
			text:       "Error: RetriableError: provider is busy",
		},
		{
			name:       "stale generation",
			generation: 6,
			text:       cursorRetriableStreamResetChunk,
		},
		{
			name:       "zero generation",
			generation: 0,
			text:       cursorRetriableStreamResetChunk,
		},
		{
			name:       "other adapter",
			agentID:    "claude-acp",
			generation: 7,
			text:       cursorRetriableStreamResetChunk,
		},
		{
			name:       "user message",
			generation: 7,
			text:       cursorRetriableStreamResetChunk,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			a, _ := newCursorPromptTurn(t, 7)
			if test.agentID != "" {
				a.agentID = test.agentID
			}

			notification := cursorMessageNotification("session-1", test.text)
			if test.name == "user message" {
				notification.Update.UserMessageChunk = &acpsdk.SessionUpdateUserMessageChunk{
					Content: acpsdk.TextBlock(test.text),
				}
				notification.Update.AgentMessageChunk = nil
			}
			a.handleACPUpdate(notification, test.generation)

			events := drainEvents(a)
			if len(events) != 1 || events[0].Type != streams.EventTypeMessageChunk || events[0].Text != test.text {
				t.Fatalf("events = %+v, want one ordinary message chunk", events)
			}
		})
	}
}

func TestCursorRetriableStreamResetClearsOnProgressAndRearmsOnLaterMarker(t *testing.T) {
	a, _ := newCursorPromptTurn(t, 7)

	a.handleACPUpdate(cursorMessageNotification("session-1", cursorRetriableStreamResetChunk), 7)
	a.handleACPUpdate(cursorMessageNotification("session-1", "Cursor resumed generation"), 7)
	a.handleACPUpdate(cursorMessageNotification("session-1", cursorRetriableStreamResetChunk), 7)

	events := drainEvents(a)
	if len(events) != 1 || events[0].Type != streams.EventTypeMessageChunk || events[0].Text != "Cursor resumed generation" {
		t.Fatalf("events = %+v, want only resumed provider progress", events)
	}
}

func TestCursorRetriableStreamResetNotClearedByToolUpdate(t *testing.T) {
	a, _ := newCursorPromptTurn(t, 7)
	a.handleACPUpdate(cursorMessageNotification("session-1", cursorRetriableStreamResetChunk), 7)

	completed := acpsdk.ToolCallStatus("completed")
	a.handleACPUpdate(makeNotification("session-1", acpsdk.SessionUpdate{
		ToolCallUpdate: &acpsdk.SessionToolCallUpdate{
			ToolCallId: "tool-1",
			Status:     &completed,
		},
	}), 7)

	if !a.currentPromptTurn().cursorRetriableFailure() {
		t.Fatal("tool update cleared pending Cursor retriable evidence")
	}
}

func TestSendPromptCursorRetriableStreamResetSettlesAfterNotificationBarrier(t *testing.T) {
	a, fake, conn := setupHandoffFakeAgent(t)
	a.agentID = acpcompat.CursorAgentID
	a.normalizer = NewNormalizer(acpcompat.CursorAgentID)
	a.dialect = newACPDialect(acpcompat.CursorAgentID)

	ctx := context.Background()
	if err := a.Initialize(ctx); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	if _, err := a.NewSession(ctx, nil); err != nil {
		t.Fatalf("new session: %v", err)
	}
	_ = drainEvents(a)

	done := make(chan error, 1)
	go func() { done <- a.Prompt(ctx, "try this", nil, 7) }()
	select {
	case <-fake.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("prompt did not reach the fake Cursor agent")
	}

	sendCapturedUpdate(t, conn, `{"sessionId":"session-handoff","update":{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"`+cursorRetriableStreamResetChunk+`"}}}`)
	fake.releasePrompts()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Prompt returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("prompt did not settle")
	}

	events := drainEvents(a)
	var providerErrors []*streams.ProviderError
	var errors, completes, messageChunks int
	for _, event := range events {
		switch event.Type {
		case streams.EventTypeError:
			errors++
			providerErrors = append(providerErrors, event.ProviderError)
		case streams.EventTypeComplete:
			completes++
		case streams.EventTypeMessageChunk:
			messageChunks++
		}
	}
	if errors != 1 || completes != 0 || messageChunks != 0 || len(providerErrors) != 1 {
		t.Fatalf("events = %+v, want one structured error, no raw chunk, and no complete", events)
	}
	providerError := providerErrors[0]
	if !providerError.Valid() || providerError.Source != streams.ProviderErrorSourceCursorACP ||
		providerError.ProviderID != acpcompat.CursorAgentID ||
		providerError.Message != cursorRetriableStreamResetChunk {
		t.Fatalf("provider error = %+v, want valid sanitized Cursor diagnostic", providerError)
	}
}

func TestSendPromptCursorProgressSupersedesRetriableMarker(t *testing.T) {
	a, fake, conn := setupHandoffFakeAgent(t)
	a.agentID = acpcompat.CursorAgentID
	a.normalizer = NewNormalizer(acpcompat.CursorAgentID)
	a.dialect = newACPDialect(acpcompat.CursorAgentID)

	ctx := context.Background()
	if err := a.Initialize(ctx); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	if _, err := a.NewSession(ctx, nil); err != nil {
		t.Fatalf("new session: %v", err)
	}
	_ = drainEvents(a)

	done := make(chan error, 1)
	go func() { done <- a.Prompt(ctx, "try this", nil, 7) }()
	select {
	case <-fake.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("prompt did not reach the fake Cursor agent")
	}

	sendCapturedUpdate(t, conn, `{"sessionId":"session-handoff","update":{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"`+cursorRetriableStreamResetChunk+`"}}}`)
	sendCapturedUpdate(t, conn, `{"sessionId":"session-handoff","update":{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"Cursor resumed generation"}}}`)
	fake.releasePrompts()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Prompt returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("prompt did not settle")
	}

	events := drainEvents(a)
	var errors, completes int
	for _, event := range events {
		switch event.Type {
		case streams.EventTypeError:
			errors++
		case streams.EventTypeComplete:
			completes++
		}
	}
	if errors != 0 || completes != 1 {
		t.Fatalf("events = %+v, want no error and one complete", events)
	}
}
