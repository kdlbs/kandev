package websocket

import (
	"encoding/json"
	"testing"

	ws "github.com/kandev/kandev/pkg/websocket"
)

// nextClientFrame pops one queued frame from the client's send channel.
func nextClientFrame(t *testing.T, c *Client) ws.Message {
	t.Helper()
	select {
	case data := <-c.send:
		var msg ws.Message
		if err := json.Unmarshal(data, &msg); err != nil {
			t.Fatalf("decode client frame: %v", err)
		}
		return msg
	default:
		t.Fatal("expected a frame in the client send queue")
		return ws.Message{}
	}
}

func assertNoMoreFrames(t *testing.T, c *Client) {
	t.Helper()
	select {
	case data := <-c.send:
		t.Fatalf("unexpected extra frame: %s", data)
	default:
	}
}

// errorCode extracts the error code from an error-typed frame.
func errorCode(t *testing.T, msg ws.Message) string {
	t.Helper()
	var payload ws.ErrorPayload
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		t.Fatalf("decode error payload %s: %v", msg.Payload, err)
	}
	return payload.Code
}

func requestMessage(t *testing.T, action string, payload any) *ws.Message {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return &ws.Message{ID: "req-1", Type: ws.MessageTypeRequest, Action: action, Payload: raw}
}

func TestHandleUnsubscribeRemovesTaskSubscription(t *testing.T) {
	h := newTestHub(t)
	c := newTestClient("c-unsub")
	c.hub = h

	h.SubscribeToTask(c, "task-1")
	if !c.subscriptions["task-1"] {
		t.Fatal("precondition failed: client is not subscribed to task-1")
	}

	c.handleUnsubscribe(requestMessage(t, ws.ActionTaskUnsubscribe, SubscribeRequest{TaskID: "task-1"}))

	if c.subscriptions["task-1"] {
		t.Fatal("client is still subscribed to task-1")
	}
	h.mu.RLock()
	_, stillTracked := h.taskSubscribers["task-1"]
	h.mu.RUnlock()
	if stillTracked {
		t.Fatal("hub still tracks a subscriber set for task-1")
	}

	resp := nextClientFrame(t, c)
	if resp.Type != ws.MessageTypeResponse || resp.Action != ws.ActionTaskUnsubscribe {
		t.Fatalf("response = %+v, want a task.unsubscribe response", resp)
	}
	assertNoMoreFrames(t, c)
}

func TestHandleSessionUnsubscribeRemovesSessionSubscription(t *testing.T) {
	h := newTestHub(t)
	c := newTestClient("c-sess-unsub")
	c.hub = h

	h.SubscribeToSession(c, "sess-1")
	if !c.sessionSubscriptions["sess-1"] {
		t.Fatal("precondition failed: client is not subscribed to sess-1")
	}

	c.handleSessionUnsubscribe(
		requestMessage(t, ws.ActionSessionUnsubscribe, SessionSubscribeRequest{SessionID: "sess-1"}),
	)

	if c.sessionSubscriptions["sess-1"] {
		t.Fatal("client is still subscribed to sess-1")
	}
	if resp := nextClientFrame(t, c); resp.Type != ws.MessageTypeResponse {
		t.Fatalf("response = %+v, want a success response", resp)
	}
}

func TestHandleSessionUnfocusReleasesFocusButKeepsSubscription(t *testing.T) {
	h := newTestHub(t)
	c := newTestClient("c-unfocus")
	c.hub = h

	h.SubscribeToSession(c, "sess-1")
	h.FocusSession(c, "sess-1")
	if !c.sessionFocus["sess-1"] {
		t.Fatal("precondition failed: session is not focused")
	}

	c.handleSessionUnfocus(
		requestMessage(t, ws.ActionSessionUnfocus, SessionSubscribeRequest{SessionID: "sess-1"}),
	)

	if c.sessionFocus["sess-1"] {
		t.Fatal("focus was not released")
	}
	if !c.sessionSubscriptions["sess-1"] {
		t.Fatal("unfocus must not drop the subscription")
	}
	if resp := nextClientFrame(t, c); resp.Action != ws.ActionSessionUnfocus {
		t.Fatalf("response action = %q, want %q", resp.Action, ws.ActionSessionUnfocus)
	}
}

func TestHandleUserUnsubscribeIsScopedToOwnTopic(t *testing.T) {
	h := newTestHub(t)
	c := newTestClient("c-user-unsub")
	c.hub = h

	own := c.ownUserTopic()
	h.SubscribeToUser(c, own)
	if !c.userSubscriptions[own] {
		t.Fatal("precondition failed: client is not subscribed to its own user topic")
	}

	c.handleUserUnsubscribe(
		requestMessage(t, ws.ActionUserUnsubscribe, UserSubscribeRequest{UserID: "someone-else"}),
	)
	forbidden := nextClientFrame(t, c)
	if forbidden.Type != ws.MessageTypeError {
		t.Fatalf("response = %+v, want an error for a foreign user topic", forbidden)
	}
	if !c.userSubscriptions[own] {
		t.Fatal("a rejected unsubscribe must not drop the client's own subscription")
	}

	c.handleUserUnsubscribe(requestMessage(t, ws.ActionUserUnsubscribe, UserSubscribeRequest{}))
	if c.userSubscriptions[own] {
		t.Fatal("an empty user_id must unsubscribe the client's own topic")
	}
	if resp := nextClientFrame(t, c); resp.Type != ws.MessageTypeResponse {
		t.Fatalf("response = %+v, want a success response", resp)
	}
}

func TestHandleRunUnsubscribeRemovesRunSubscription(t *testing.T) {
	h := newTestHub(t)
	c := newTestClient("c-run-unsub")
	c.hub = h

	h.SubscribeToRun(c, "run-1")
	if !c.runSubscriptions["run-1"] {
		t.Fatal("precondition failed: client is not subscribed to run-1")
	}

	c.handleRunUnsubscribe(requestMessage(t, ws.ActionRunUnsubscribe, RunSubscribeRequest{RunID: "run-1"}))

	if c.runSubscriptions["run-1"] {
		t.Fatal("client is still subscribed to run-1")
	}
	h.mu.RLock()
	_, stillTracked := h.runSubscribers["run-1"]
	h.mu.RUnlock()
	if stillTracked {
		t.Fatal("hub still tracks a subscriber set for run-1")
	}
	if resp := nextClientFrame(t, c); resp.Type != ws.MessageTypeResponse {
		t.Fatalf("response = %+v, want a success response", resp)
	}
}

// Every subscription handler must reject a malformed envelope and a missing
// identifier before touching hub state.
func TestSubscriptionHandlersRejectBadPayloads(t *testing.T) {
	handlers := map[string]struct {
		action string
		invoke func(*Client, *ws.Message)
		empty  any
	}{
		"task.subscribe": {
			action: ws.ActionTaskSubscribe,
			invoke: (*Client).handleSubscribe,
			empty:  SubscribeRequest{},
		},
		"task.unsubscribe": {
			action: ws.ActionTaskUnsubscribe,
			invoke: (*Client).handleUnsubscribe,
			empty:  SubscribeRequest{},
		},
		"session.subscribe": {
			action: ws.ActionSessionSubscribe,
			invoke: (*Client).handleSessionSubscribe,
			empty:  SessionSubscribeRequest{},
		},
		"session.unsubscribe": {
			action: ws.ActionSessionUnsubscribe,
			invoke: (*Client).handleSessionUnsubscribe,
			empty:  SessionSubscribeRequest{},
		},
		"session.unfocus": {
			action: ws.ActionSessionUnfocus,
			invoke: (*Client).handleSessionUnfocus,
			empty:  SessionSubscribeRequest{},
		},
		"run.subscribe": {
			action: ws.ActionRunSubscribe,
			invoke: (*Client).handleRunSubscribe,
			empty:  RunSubscribeRequest{},
		},
		"run.unsubscribe": {
			action: ws.ActionRunUnsubscribe,
			invoke: (*Client).handleRunUnsubscribe,
			empty:  RunSubscribeRequest{},
		},
	}

	for name, tc := range handlers {
		t.Run(name, func(t *testing.T) {
			h := newTestHub(t)
			c := newTestClient("c-" + name)
			c.hub = h

			malformed := &ws.Message{
				ID:      "req-1",
				Type:    ws.MessageTypeRequest,
				Action:  tc.action,
				Payload: json.RawMessage(`"not-an-object"`),
			}
			tc.invoke(c, malformed)
			if resp := nextClientFrame(t, c); resp.Type != ws.MessageTypeError {
				t.Fatalf("malformed payload response = %+v, want an error", resp)
			}

			tc.invoke(c, requestMessage(t, tc.action, tc.empty))
			resp := nextClientFrame(t, c)
			if resp.Type != ws.MessageTypeError {
				t.Fatalf("missing-identifier response = %+v, want an error", resp)
			}
			if code := errorCode(t, resp); code != ws.ErrorCodeValidation {
				t.Fatalf("missing-identifier error code = %q, want %q", code, ws.ErrorCodeValidation)
			}
		})
	}
}
