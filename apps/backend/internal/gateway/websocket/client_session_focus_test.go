package websocket

import (
	"context"
	"encoding/json"
	"testing"

	ws "github.com/kandev/kandev/pkg/websocket"
)

// TestHandleSessionFocus_PushesSessionDataSnapshot verifies that session.focus
// pushes a fresh session-data snapshot to the focusing client, not just
// session.subscribe. This closes the "sidebar bulk-subscribe absorbs the
// task-page subscribe" gap — when the ref-counted client-side subscribe skips
// the frame, focus is the only signal the backend still receives on task open.
func TestHandleSessionFocus_PushesSessionDataSnapshot(t *testing.T) {
	h := newTestHub(t)

	const sessionID = "sess-focus-1"
	snapshot, err := ws.NewNotification("session.git.event", map[string]any{
		"session_id": sessionID,
		"type":       "status_update",
	})
	if err != nil {
		t.Fatalf("build snapshot: %v", err)
	}
	var provided bool
	h.SetSessionDataProvider(func(_ context.Context, sid string) ([]*ws.Message, error) {
		if sid != sessionID {
			t.Errorf("provider called with sid=%q, want %q", sid, sessionID)
		}
		provided = true
		return []*ws.Message{snapshot}, nil
	})

	c := newTestClient("c-focus")
	c.hub = h

	payload, _ := json.Marshal(SessionSubscribeRequest{SessionID: sessionID})
	msg := &ws.Message{ID: "req-1", Type: ws.MessageTypeRequest, Action: "session.focus", Payload: payload}

	c.handleSessionFocus(msg)

	if !provided {
		t.Fatal("session data provider was not invoked on focus")
	}

	// handleSessionFocus is synchronous and sendMessage writes to the buffered
	// send channel without blocking, so both frames are already queued by the
	// time we get here. A non-blocking select fails fast if the invariant
	// breaks in the future.
	var sawSnapshot bool
	for i := range 2 {
		select {
		case data := <-c.send:
			var m ws.Message
			if err := json.Unmarshal(data, &m); err != nil {
				t.Fatalf("decode frame: %v", err)
			}
			if m.Type == ws.MessageTypeNotification && m.Action == "session.git.event" {
				sawSnapshot = true
			}
		default:
			t.Fatalf("expected focus frame %d, none in buffer", i+1)
		}
	}
	if !sawSnapshot {
		t.Error("expected a session.git.event notification after focus, got none")
	}
}

// TestHandleSessionFocus_NoProviderDoesNotCrash guards the nil-provider path —
// the hub ships without a provider configured in some test setups.
func TestHandleSessionFocus_NoProviderDoesNotCrash(t *testing.T) {
	h := newTestHub(t)

	c := newTestClient("c-no-provider")
	c.hub = h

	payload, _ := json.Marshal(SessionSubscribeRequest{SessionID: "sess-x"})
	msg := &ws.Message{ID: "req-1", Type: ws.MessageTypeRequest, Action: "session.focus", Payload: payload}

	c.handleSessionFocus(msg)

	// Drain the ACK so it's clear exactly one frame was produced.
	select {
	case <-c.send:
	default:
		t.Fatal("expected ACK frame after focus")
	}
	select {
	case <-c.send:
		t.Error("unexpected extra frame when provider is nil")
	default:
	}
}
