package websocket

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

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
			t.Fatalf("decode refresh response: %v", err)
		}
		if response.Action != ws.ActionSessionGitRefresh {
			t.Fatalf("unexpected refresh response: %+v", response)
		}
		var payload struct {
			Success   bool         `json:"success"`
			SessionID string       `json:"session_id"`
			Mode      string       `json:"mode"`
			Snapshots []ws.Message `json:"snapshots"`
		}
		if err := json.Unmarshal(response.Payload, &payload); err != nil {
			t.Fatalf("decode correlated refresh payload: %v", err)
		}
		if !payload.Success || payload.SessionID != sessionID || payload.Mode != "fresh" || len(payload.Snapshots) != 1 || payload.Snapshots[0].Action != ws.ActionSessionGitEvent {
			t.Fatalf("correlated refresh payload = %+v, want the single Git snapshot", payload)
		}
	default:
		t.Fatal("expected correlated refresh response")
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

func TestHandleSessionGitRefreshForwardsRecoveryMode(t *testing.T) {
	h := newTestHub(t)
	modeSeen := make(chan string, 1)
	h.SetSessionGitRefreshProvider(func(_ context.Context, sessionID, mode string) (SessionGitRefreshResult, error) {
		modeSeen <- mode
		return SessionGitRefreshResult{SessionID: sessionID, Mode: mode, StatusState: "unavailable", Snapshots: []*ws.Message{}}, nil
	})
	c := newTestClient("c-git-recover")
	c.hub = h
	c.controlSend = make(chan []byte, 4)
	payload, _ := json.Marshal(SessionSubscribeRequest{SessionID: "sess-git-recover", Mode: "recover"})
	c.handleSessionGitRefresh(&ws.Message{ID: "req-recover", Action: ws.ActionSessionGitRefresh, Payload: payload})
	select {
	case mode := <-modeSeen:
		if mode != "recover" {
			t.Fatalf("mode = %q, want recover", mode)
		}
	case <-time.After(time.Second):
		t.Fatal("refresh provider was not called")
	}
}

func TestSessionGitRefreshIsCanceledWithConnection(t *testing.T) {
	h := newTestHub(t)
	started := make(chan struct{})
	canceled := make(chan struct{})
	h.SetSessionGitRefreshProvider(func(ctx context.Context, sessionID, mode string) (SessionGitRefreshResult, error) {
		close(started)
		<-ctx.Done()
		close(canceled)
		return SessionGitRefreshResult{SessionID: sessionID, Mode: mode}, nil
	})
	c := newTestClient("c-git-cancel")
	c.hub = h
	c.controlSend = make(chan []byte, 4)
	payload, _ := json.Marshal(SessionSubscribeRequest{SessionID: "sess-git-cancel"})
	go c.handleSessionGitRefresh(&ws.Message{ID: "req-cancel", Action: ws.ActionSessionGitRefresh, Payload: payload})
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("refresh provider did not start")
	}
	c.cancelSessionGitRefreshes()
	select {
	case <-canceled:
	case <-time.After(time.Second):
		t.Fatal("connection teardown did not cancel refresh work")
	}
	if len(c.controlSend) != 0 {
		t.Fatal("canceled refresh sent a response")
	}
}

func TestSessionGitRefreshAdmissionIsBoundedPerConnection(t *testing.T) {
	h := newTestHub(t)
	started := make(chan struct{}, maxConcurrentSessionGitRefreshes+1)
	h.SetSessionGitRefreshProvider(func(ctx context.Context, sessionID, mode string) (SessionGitRefreshResult, error) {
		started <- struct{}{}
		<-ctx.Done()
		return SessionGitRefreshResult{SessionID: sessionID, Mode: mode}, nil
	})
	c := newTestClient("c-git-refresh-cap")
	c.hub = h
	c.controlSend = make(chan []byte, maxConcurrentSessionGitRefreshes+2)
	payload, _ := json.Marshal(SessionSubscribeRequest{SessionID: "sess-git-cap"})
	var workers sync.WaitGroup
	for i := 0; i < maxConcurrentSessionGitRefreshes; i++ {
		workers.Add(1)
		go func(id int) {
			defer workers.Done()
			c.handleSessionGitRefresh(&ws.Message{ID: fmt.Sprintf("req-cap-%d", id), Action: ws.ActionSessionGitRefresh, Payload: payload})
		}(i)
	}
	for i := 0; i < maxConcurrentSessionGitRefreshes; i++ {
		select {
		case <-started:
		case <-time.After(time.Second):
			c.cancelSessionGitRefreshes()
			workers.Wait()
			t.Fatalf("only %d refresh providers started", i)
		}
	}

	c.gitRefreshMu.Lock()
	active := len(c.gitRefreshCancels)
	c.gitRefreshMu.Unlock()
	if active != maxConcurrentSessionGitRefreshes {
		t.Fatalf("active refreshes = %d, want cap %d", active, maxConcurrentSessionGitRefreshes)
	}
	c.handleSessionGitRefresh(&ws.Message{ID: "req-over-cap", Action: ws.ActionSessionGitRefresh, Payload: payload})
	select {
	case data := <-c.controlSend:
		var response ws.Message
		if err := json.Unmarshal(data, &response); err != nil {
			t.Fatalf("decode capacity response: %v", err)
		}
		if response.Type != ws.MessageTypeError || response.ID != "req-over-cap" {
			t.Fatalf("capacity response = %+v, want correlated error", response)
		}
		var body ws.ErrorPayload
		if err := json.Unmarshal(response.Payload, &body); err != nil {
			t.Fatalf("decode capacity error body: %v", err)
		}
		if body.Code != ws.ErrorCodeUnavailable {
			t.Fatalf("capacity error code = %q, want %q", body.Code, ws.ErrorCodeUnavailable)
		}
	case <-time.After(time.Second):
		t.Fatal("refresh over the cap did not receive a correlated capacity response")
	}
	select {
	case <-started:
		t.Fatal("refresh over the cap started another provider call")
	default:
	}

	c.cancelSessionGitRefreshes()
	workers.Wait()
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
