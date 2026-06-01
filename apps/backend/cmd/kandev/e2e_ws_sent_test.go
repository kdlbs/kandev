package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/synctest"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/common/logger"
	gateways "github.com/kandev/kandev/internal/gateway/websocket"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func newTestE2EHub(t *testing.T) (*gateways.Hub, context.CancelFunc) {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	hub := gateways.NewHub(nil, log)
	ctx, cancel := context.WithCancel(context.Background())
	go hub.Run(ctx)
	// The caller owns `cancel`: tests that drive registration run inside a
	// synctest bubble and `defer cancel()` so the Run goroutine exits before
	// the bubble closes (synctest requires all bubbled goroutines to finish).
	return hub, cancel
}

// registerAndWait registers a client and blocks until the hub's Run goroutine
// has committed it to the client map, i.e. until the ws-sent endpoint can
// resolve the connection. hub.Register() pushes onto an unbuffered channel and
// returns before Run finishes the (locked) map write. Rather than poll, we use
// synctest.Wait() to drain the Run goroutine until it is durably blocked again
// — at which point the registration has landed, deterministically and with no
// real-time waiting. MUST be called from inside a synctest.Test bubble.
func registerAndWait(t *testing.T, hub *gateways.Hub, client *gateways.Client) {
	t.Helper()
	hub.Register(client)
	synctest.Wait()
	if _, _, ok := hub.GetSentEventsFor(client.ID, 0); !ok {
		t.Fatalf("client %q was not registered after synctest.Wait()", client.ID)
	}
}

func mustNotif(t *testing.T, action string) *ws.Message {
	t.Helper()
	m, err := ws.NewNotification(action, map[string]string{"k": "v"})
	if err != nil {
		t.Fatalf("build notif: %v", err)
	}
	return m
}

// setupE2EWsSentRouter builds a gin engine with the ws-sent route mounted.
// Skips the test if KANDEV_MOCK_AGENT isn't set in the gate-on case.
func setupE2EWsSentRouter(t *testing.T, hub *gateways.Hub) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	api := r.Group("/api/v1/e2e")
	api.GET("/ws-sent", handleE2EWsSent(hub))
	return r
}

// TestE2EWsSent_UnknownConnection400_or_404 covers the input-validation paths:
// missing connection_id → 400, unknown connection_id → 404.
func TestE2EWsSent_UnknownConnection(t *testing.T) {
	// No client registration here, so no synctest bubble is needed — the
	// validation paths never touch the Run goroutine's register channel.
	hub, cancel := newTestE2EHub(t)
	defer cancel()
	r := setupE2EWsSentRouter(t, hub)

	cases := []struct {
		name     string
		query    string
		wantCode int
	}{
		{name: "missing connection_id", query: "", wantCode: http.StatusBadRequest},
		{name: "unknown connection_id", query: "?connection_id=ghost", wantCode: http.StatusNotFound},
		{name: "bad since_seq", query: "?connection_id=ghost&since_seq=nope", wantCode: http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/e2e/ws-sent"+tc.query, nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != tc.wantCode {
				t.Errorf("status=%d, want %d, body=%s", w.Code, tc.wantCode, w.Body.String())
			}
		})
	}
}

// TestE2EWsSent_ReturnsRingBufferForKnownClient drives a real client through
// the public Hub.Register channel and verifies the endpoint exposes that
// client's ring buffer. This is the happy path the FE accountant relies on.
func TestE2EWsSent_ReturnsRingBufferForKnownClient(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		hub, cancel := newTestE2EHub(t)
		defer cancel()
		r := setupE2EWsSentRouter(t, hub)

		// Register a client via the hub's public seam — the only stable way to
		// land a *Client in hub.clients from outside the gateway package.
		log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
		client := gateways.NewClient("conn-e2e", nil, hub, log)
		registerAndWait(t, hub, client)

		// SubscribeToUser and BroadcastToUser are synchronous (they stamp + write to
		// the client's ring buffer before returning), so no post-broadcast wait is
		// needed once registration has landed.
		hub.SubscribeToUser(client, "u1")
		for i := 1; i <= 3; i++ {
			hub.BroadcastToUser("u1", mustNotif(t, "evt"))
		}

		// subtests are flattened into scoped blocks: synctest bubbles disallow
		// t.Run, and the ring-buffer reads must happen before defer cancel()
		// tears down the client registration on hub shutdown.

		// returns all when since_seq omitted
		{
			req := httptest.NewRequest(http.MethodGet, "/api/v1/e2e/ws-sent?connection_id=conn-e2e", nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != http.StatusOK {
				t.Fatalf("status=%d, body=%s", w.Code, w.Body.String())
			}
			var resp e2eWsSentResponse
			if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if resp.ConnectionID != "conn-e2e" {
				t.Errorf("connection_id=%q, want conn-e2e", resp.ConnectionID)
			}
			if resp.MaxSeq != 3 {
				t.Errorf("max_seq=%d, want 3", resp.MaxSeq)
			}
			if len(resp.Events) != 3 {
				t.Fatalf("len(events)=%d, want 3 (body=%s)", len(resp.Events), w.Body.String())
			}
			for i, e := range resp.Events {
				if e.Seq != int64(i+1) {
					t.Errorf("events[%d].Seq=%d, want %d", i, e.Seq, i+1)
				}
				if e.Action != "evt" {
					t.Errorf("events[%d].Action=%q, want evt", i, e.Action)
				}
			}
		}

		// filters by since_seq
		{
			req := httptest.NewRequest(http.MethodGet, "/api/v1/e2e/ws-sent?connection_id=conn-e2e&since_seq=2", nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != http.StatusOK {
				t.Fatalf("status=%d", w.Code)
			}
			var resp e2eWsSentResponse
			if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if len(resp.Events) != 1 || resp.Events[0].Seq != 3 {
				t.Errorf("events=%+v, want one entry seq=3", resp.Events)
			}
			if resp.MaxSeq != 3 {
				t.Errorf("max_seq=%d, want 3", resp.MaxSeq)
			}
		}
	})
}

// TestE2EWsSent_SessionFilter_ReturnsOnlyMatchingEvents covers the
// Workstream 1 `session_id` query param: the endpoint should narrow the ring
// buffer to events whose backend-stamped SessionID matches the filter, sorted
// by SessionSeq ascending. `max_seq` carries the max SessionSeq for that
// (connection, session) pair, NOT the per-connection max.
func TestE2EWsSent_SessionFilter_ReturnsOnlyMatchingEvents(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		hub, cancel := newTestE2EHub(t)
		defer cancel()
		r := setupE2EWsSentRouter(t, hub)

		log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
		client := gateways.NewClient("conn-sess", nil, hub, log)
		registerAndWait(t, hub, client)

		// SubscribeToSession and BroadcastToSession are synchronous, so the ring
		// buffer is fully populated once these calls return.
		// Subscribe to two sessions and broadcast a mixed stream:
		// session A: 2 events
		// session B: 1 event
		// session A: 1 event
		hub.SubscribeToSession(client, "sa")
		hub.SubscribeToSession(client, "sb")
		hub.BroadcastToSession("sa", mustNotif(t, "a.1"))
		hub.BroadcastToSession("sa", mustNotif(t, "a.2"))
		hub.BroadcastToSession("sb", mustNotif(t, "b.1"))
		hub.BroadcastToSession("sa", mustNotif(t, "a.3"))

		// subtests flattened into scoped blocks (synctest bans t.Run; reads must
		// run before defer cancel() tears down the registration).

		// session A returns only a.* sorted by session_seq
		{
			req := httptest.NewRequest(
				http.MethodGet,
				"/api/v1/e2e/ws-sent?connection_id=conn-sess&session_id=sa",
				nil,
			)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != http.StatusOK {
				t.Fatalf("status=%d, body=%s", w.Code, w.Body.String())
			}
			var resp e2eWsSentResponse
			if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if len(resp.Events) != 3 {
				t.Fatalf("len(events)=%d, want 3 (body=%s)", len(resp.Events), w.Body.String())
			}
			for i, e := range resp.Events {
				if e.SessionID != "sa" {
					t.Errorf("events[%d].SessionID=%q, want sa", i, e.SessionID)
				}
				wantSessionSeq := int64(i + 1)
				if e.SessionSeq != wantSessionSeq {
					t.Errorf("events[%d].SessionSeq=%d, want %d", i, e.SessionSeq, wantSessionSeq)
				}
			}
			// MaxSeq carries the max SessionSeq for session A (3), not the
			// per-connection max (which would be 4 — a.1, a.2, b.1, a.3).
			if resp.MaxSeq != 3 {
				t.Errorf("max_seq=%d, want 3 (max SessionSeq for sa)", resp.MaxSeq)
			}
		}

		// session B returns only b.* with monotonic session_seq
		{
			req := httptest.NewRequest(
				http.MethodGet,
				"/api/v1/e2e/ws-sent?connection_id=conn-sess&session_id=sb",
				nil,
			)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != http.StatusOK {
				t.Fatalf("status=%d", w.Code)
			}
			var resp e2eWsSentResponse
			if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if len(resp.Events) != 1 {
				t.Fatalf("len(events)=%d, want 1", len(resp.Events))
			}
			if resp.Events[0].SessionSeq != 1 {
				t.Errorf("events[0].SessionSeq=%d, want 1", resp.Events[0].SessionSeq)
			}
			if resp.MaxSeq != 1 {
				t.Errorf("max_seq=%d, want 1", resp.MaxSeq)
			}
		}

		// without session_id, returns the full per-connection log
		{
			req := httptest.NewRequest(
				http.MethodGet,
				"/api/v1/e2e/ws-sent?connection_id=conn-sess",
				nil,
			)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != http.StatusOK {
				t.Fatalf("status=%d", w.Code)
			}
			var resp e2eWsSentResponse
			if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
				t.Fatalf("decode: %v", err)
			}
			// 4 broadcasts × 1 subscribing client = 4 stamped frames.
			if len(resp.Events) != 4 {
				t.Errorf("len(events)=%d, want 4 (per-connection backward-compat)", len(resp.Events))
			}
			if resp.MaxSeq != 4 {
				t.Errorf("max_seq=%d, want 4 (per-connection max)", resp.MaxSeq)
			}
		}
	})
}
