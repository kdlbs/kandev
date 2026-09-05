package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// @covers AC-AGENTS-MCP-BRIDGE-RELIABILITY-001.3
func TestStreamUpdatesReturnsCorrelatedErrorWhenMCPHandlerIsUnavailable(t *testing.T) {
	type serverResult struct {
		message ws.Message
		err     error
	}
	resultCh := make(chan serverResult, 1)
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			resultCh <- serverResult{err: err}
			return
		}
		defer func() { _ = conn.Close() }()
		request, _ := ws.NewRequest("req-unavailable", "mcp.tools.call", nil)
		data, _ := json.Marshal(request)
		if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
			resultCh <- serverResult{err: err}
			return
		}
		_ = conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		_, data, err = conn.ReadMessage()
		if err != nil {
			resultCh <- serverResult{err: err}
			return
		}
		var response ws.Message
		err = json.Unmarshal(data, &response)
		resultCh <- serverResult{message: response, err: err}
	}))
	t.Cleanup(server.Close)

	client := &Client{
		baseURL:         server.URL,
		httpClient:      &http.Client{Timeout: time.Second},
		logger:          newTestLogger(),
		pendingRequests: make(map[string]chan *ws.Message),
	}
	t.Cleanup(client.Close)
	if err := client.StreamUpdates(context.Background(), func(AgentEvent) {}, nil, nil); err != nil {
		t.Fatalf("StreamUpdates: %v", err)
	}

	select {
	case result := <-resultCh:
		if result.err != nil {
			t.Fatalf("read response: %v", result.err)
		}
		if result.message.Type != ws.MessageTypeError || result.message.ID != "req-unavailable" {
			t.Fatalf("response = %+v, want correlated error", result.message)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for unavailable-handler response")
	}
}
