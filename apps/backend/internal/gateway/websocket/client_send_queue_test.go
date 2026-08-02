package websocket

import (
	"encoding/json"
	"testing"

	ws "github.com/kandev/kandev/pkg/websocket"
)

func TestClient_ControlResponseIsNotBlockedByStreamBacklog(t *testing.T) {
	c := newTestClient("control-priority")
	c.controlSend = make(chan []byte, controlSendBufferSize)

	for index := 0; index < cap(c.send); index++ {
		if !c.sendBytes([]byte("stream")) {
			t.Fatalf("stream frame %d was not queued", index)
		}
	}

	response, err := ws.NewResponse("request-1", "message.add", map[string]any{"accepted": true})
	if err != nil {
		t.Fatalf("build response: %v", err)
	}
	c.sendMessage(response)

	select {
	case data := <-c.controlSend:
		var got ws.Message
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatalf("decode control frame: %v", err)
		}
		if got.ID != "request-1" || got.Type != ws.MessageTypeResponse {
			t.Fatalf("unexpected control frame: %+v", got)
		}
	default:
		t.Fatal("control response was not queued while stream queue was full")
	}
}

func TestClient_ControlOverflowClosesConnection(t *testing.T) {
	c := newTestClient("control-overflow")
	c.controlSend = make(chan []byte, 1)

	first, err := ws.NewResponse("request-1", "message.add", map[string]any{"accepted": true})
	if err != nil {
		t.Fatalf("build first response: %v", err)
	}
	second, err := ws.NewResponse("request-2", "message.add", map[string]any{"accepted": true})
	if err != nil {
		t.Fatalf("build second response: %v", err)
	}
	c.sendMessage(first)
	c.sendMessage(second)

	if !c.closed {
		t.Fatal("control overflow should close the client")
	}
	if _, ok := <-c.send; ok {
		t.Fatal("stream queue should be closed after control overflow")
	}
	if _, ok := <-c.controlSend; ok {
		// The first response is still drainable, but the queue must close after
		// its buffered frame is consumed.
		if _, stillOpen := <-c.controlSend; stillOpen {
			t.Fatal("control queue remained open after overflow")
		}
	}
}
