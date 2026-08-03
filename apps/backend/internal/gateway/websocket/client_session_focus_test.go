package websocket

import (
	"context"
	"encoding/json"
	"testing"

	ws "github.com/kandev/kandev/pkg/websocket"
)

// TestHandleSessionFocus_IsAckOnly verifies that session.focus only changes
// polling interest and acknowledges the request. Detail surfaces fetch their
// snapshot explicitly; replaying the full session stream on every focus event
// made task switching compete with control responses on the same connection.
func TestHandleSessionFocus_IsAckOnly(t *testing.T) {
	h := newTestHub(t)

	const sessionID = "sess-focus-1"
	var provided bool
	h.SetSessionDataProvider(func(_ context.Context, sid string) ([]*ws.Message, error) {
		if sid != sessionID {
			t.Errorf("provider called with sid=%q, want %q", sid, sessionID)
		}
		provided = true
		return nil, nil
	})

	c := newTestClient("c-focus")
	c.hub = h

	payload, _ := json.Marshal(SessionSubscribeRequest{SessionID: sessionID})
	msg := &ws.Message{ID: "req-1", Type: ws.MessageTypeRequest, Action: "session.focus", Payload: payload}

	c.handleSessionFocus(msg)

	if provided {
		t.Fatal("session data provider should not be invoked on focus")
	}

	// The ACK is a control frame, not a replayed session notification.
	select {
	case data := <-c.send:
		var response ws.Message
		if err := json.Unmarshal(data, &response); err != nil {
			t.Fatalf("decode ACK: %v", err)
		}
		if response.Type != ws.MessageTypeResponse || response.Action != "session.focus" {
			t.Fatalf("unexpected focus response: %+v", response)
		}
	default:
		t.Fatal("expected focus ACK in control queue")
	}
	select {
	case <-c.send:
		t.Fatal("unexpected session-data replay after focus")
	default:
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

func TestHandleSessionDataRefresh_ReplaysExplicitSnapshot(t *testing.T) {
	h := newTestHub(t)
	const sessionID = "sess-refresh-1"
	provided := false
	h.SetSessionDataProvider(func(_ context.Context, sid string) ([]*ws.Message, error) {
		if sid != sessionID {
			t.Errorf("provider called with sid=%q, want %q", sid, sessionID)
		}
		provided = true
		return []*ws.Message{{
			Type:    ws.MessageTypeNotification,
			Action:  "session.git.event",
			Payload: json.RawMessage(`{"session_id":"sess-refresh-1"}`),
		}}, nil
	})

	c := newTestClient("c-refresh")
	c.hub = h
	c.controlSend = make(chan []byte, 16)

	payload, _ := json.Marshal(SessionSubscribeRequest{SessionID: sessionID})
	msg := &ws.Message{ID: "req-refresh", Type: ws.MessageTypeRequest, Action: ws.ActionSessionDataRefresh, Payload: payload}
	c.handleSessionDataRefresh(msg)

	if !provided {
		t.Fatal("session data provider should be invoked by explicit refresh")
	}
	select {
	case data := <-c.controlSend:
		var response ws.Message
		if err := json.Unmarshal(data, &response); err != nil {
			t.Fatalf("decode refresh ACK: %v", err)
		}
		if response.Type != ws.MessageTypeResponse || response.Action != ws.ActionSessionDataRefresh {
			t.Fatalf("unexpected refresh response: %+v", response)
		}
	default:
		t.Fatal("expected refresh ACK in control queue")
	}
	select {
	case data := <-c.send:
		var notification ws.Message
		if err := json.Unmarshal(data, &notification); err != nil {
			t.Fatalf("decode refreshed session data: %v", err)
		}
		if notification.Action != "session.git.event" {
			t.Fatalf("unexpected refreshed data: %+v", notification)
		}
	default:
		t.Fatal("expected explicit session-data notification")
	}
}

func TestHandleSessionGitRefresh_SendsOnlyGitData(t *testing.T) {
	h := newTestHub(t)
	const sessionID = "sess-git-refresh-1"
	h.SetSessionGitDataProvider(func(_ context.Context, sid string) ([]*ws.Message, error) {
		if sid != sessionID {
			t.Errorf("provider called with sid=%q, want %q", sid, sessionID)
		}
		return []*ws.Message{
			{Type: ws.MessageTypeNotification, Action: ws.ActionSessionStateChanged},
			{Type: ws.MessageTypeNotification, Action: ws.ActionSessionGitEvent},
		}, nil
	})

	c := newTestClient("c-git-refresh")
	c.hub = h
	c.controlSend = make(chan []byte, 16)
	payload, _ := json.Marshal(SessionSubscribeRequest{SessionID: sessionID})
	c.handleSessionGitRefresh(&ws.Message{
		ID:      "req-git-refresh",
		Type:    ws.MessageTypeRequest,
		Action:  ws.ActionSessionGitRefresh,
		Payload: payload,
	})

	select {
	case data := <-c.controlSend:
		var response ws.Message
		if err := json.Unmarshal(data, &response); err != nil {
			t.Fatalf("decode refresh ACK: %v", err)
		}
		if response.Action != ws.ActionSessionGitRefresh {
			t.Fatalf("unexpected refresh response: %+v", response)
		}
	default:
		t.Fatal("expected refresh ACK")
	}
	select {
	case data := <-c.send:
		var notification ws.Message
		if err := json.Unmarshal(data, &notification); err != nil {
			t.Fatalf("decode git notification: %v", err)
		}
		if notification.Action != ws.ActionSessionGitEvent {
			t.Fatalf("unexpected refresh data: %+v", notification)
		}
	default:
		t.Fatal("expected git notification")
	}
	select {
	case data := <-c.send:
		t.Fatalf("unexpected non-git refresh data: %s", data)
	default:
	}
}

func TestHandleSessionSubscribe_DuplicateDoesNotReplaySnapshot(t *testing.T) {
	h := newTestHub(t)
	const sessionID = "sess-subscribe-1"
	providerCalls := 0
	h.SetSessionDataProvider(func(_ context.Context, sid string) ([]*ws.Message, error) {
		if sid != sessionID {
			t.Errorf("provider called with sid=%q, want %q", sid, sessionID)
		}
		providerCalls++
		return []*ws.Message{{
			Type:    ws.MessageTypeNotification,
			Action:  "session.git.event",
			Payload: json.RawMessage(`{"session_id":"sess-subscribe-1"}`),
		}}, nil
	})

	c := newTestClient("c-subscribe")
	c.hub = h
	c.controlSend = make(chan []byte, 16)
	payload, _ := json.Marshal(SessionSubscribeRequest{SessionID: sessionID})

	for index := 1; index <= 2; index++ {
		c.handleSessionSubscribe(&ws.Message{
			ID:      "req-subscribe-" + string(rune('0'+index)),
			Type:    ws.MessageTypeRequest,
			Action:  ws.ActionSessionSubscribe,
			Payload: payload,
		})
	}
	if providerCalls != 1 {
		t.Fatalf("session data provider calls = %d, want 1", providerCalls)
	}
	for index := 0; index < 2; index++ {
		select {
		case <-c.controlSend:
		default:
			t.Fatal("expected subscribe acknowledgement")
		}
	}
	select {
	case <-c.send:
	default:
		t.Fatal("expected one initial session snapshot")
	}
	select {
	case <-c.send:
		t.Fatal("duplicate subscribe replayed session snapshot")
	default:
	}
}
