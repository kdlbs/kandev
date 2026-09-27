package api

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// TestAgentStreamLiveness pins system design part 2 "Stream liveness": a
// peer that keeps answering pings stays connected past what would have been
// the read deadline without a single answered ping, and a peer that stops
// answering pings entirely has its stream closed within the read deadline
// (acceptance #1: "A stream whose peer stops answering pings ends within
// 45s"). SetStreamLivenessTimings swaps in millisecond-scale timings so this
// runs without a real 15s/45s wait.
func TestAgentStreamLiveness(t *testing.T) {
	const (
		pingInterval  = 20 * time.Millisecond
		readDeadline  = 80 * time.Millisecond
		writeDeadline = time.Second
	)

	t.Run("answered pings keep the stream alive past one read deadline", func(t *testing.T) {
		s := newTestServer(t)
		s.SetStreamLivenessTimings(pingInterval, readDeadline, writeDeadline)
		server := httptest.NewServer(s.router)
		defer server.Close()

		wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/v1/agent/stream"
		conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
		if err != nil {
			t.Fatalf("failed to dial WebSocket: %v", err)
		}
		defer func() { _ = conn.Close() }()

		// gorilla/websocket only answers a Ping with a Pong from within a
		// goroutine actively reading, so this loop is what keeps the
		// connection's server-side read deadline extended.
		readErr := make(chan error, 1)
		go func() {
			for {
				if _, _, err := conn.ReadMessage(); err != nil {
					readErr <- err
					return
				}
			}
		}()

		select {
		case err := <-readErr:
			t.Fatalf("connection closed while answering pings (err=%v), want it to survive past one read deadline", err)
		case <-time.After(3 * readDeadline):
		}
	})

	t.Run("a peer that stops answering pings is closed within the read deadline", func(t *testing.T) {
		s := newTestServer(t)
		s.SetStreamLivenessTimings(pingInterval, readDeadline, writeDeadline)
		server := httptest.NewServer(s.router)
		defer server.Close()

		wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/v1/agent/stream"
		conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
		if err != nil {
			t.Fatalf("failed to dial WebSocket: %v", err)
		}
		defer func() { _ = conn.Close() }()

		// Never read: no control frame the server sends is ever answered, so
		// no Pong ever extends the server's read deadline.
		if !waitForAttachmentCurrent(t, s, "") {
			t.Fatal("stream never became current")
		}

		deadline := time.Now().Add(3 * readDeadline)
		for time.Now().Before(deadline) {
			if !s.procMgr.AttachmentStatus().Current {
				return
			}
			time.Sleep(5 * time.Millisecond)
		}
		t.Fatal("stream was never ended after its peer stopped answering pings")
	})
}
