package websocket

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	gorillaws "github.com/gorilla/websocket"
)

// startWSGateway serves the real /ws route backed by a running hub and returns
// the gateway plus the ws:// URL to dial.
func startWSGateway(t *testing.T) (*Gateway, string) {
	t.Helper()

	g := NewGateway(newTestProxyLogger())

	ctx, cancel := context.WithCancel(context.Background())
	hubDone := make(chan struct{})
	go func() {
		defer close(hubDone)
		g.Hub.Run(ctx)
	}()

	gin.SetMode(gin.TestMode)
	router := gin.New()
	g.SetupRoutes(router)
	srv := httptest.NewServer(router)

	t.Cleanup(func() {
		srv.Close()
		cancel()
		<-hubDone
	})

	return g, "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws"
}

func dialWS(t *testing.T, wsURL, origin string) (*gorillaws.Conn, *http.Response, error) {
	t.Helper()

	header := http.Header{}
	if origin != "" {
		header.Set("Origin", origin)
	}
	conn, resp, err := gorillaws.DefaultDialer.Dial(wsURL, header)
	if resp != nil && resp.Body != nil {
		t.Cleanup(func() { _ = resp.Body.Close() })
	}
	return conn, resp, err
}

// waitForClientCount blocks until the hub observes the expected client count.
// Successful WebSocket dials can return before the handler goroutine has
// registered the client, so tests must synchronize on the hub before cleanup.
func waitForClientCount(t *testing.T, g *Gateway, want int) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for g.Hub.GetClientCount() != want {
		if time.Now().After(deadline) {
			t.Fatalf("hub has %d client(s), want %d", g.Hub.GetClientCount(), want)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// TestHandleConnection_RejectsForeignOrigin is the CSWSH regression test: a
// browser page on an attacker-controlled origin must not be able to complete
// a WebSocket upgrade against the gateway.
func TestHandleConnection_RejectsForeignOrigin(t *testing.T) {
	_, wsURL := startWSGateway(t)

	conn, resp, err := dialWS(t, wsURL, "https://attacker.example")
	if err == nil {
		_ = conn.Close()
		t.Fatal("expected upgrade with foreign Origin to fail")
	}
	if resp == nil {
		t.Fatalf("expected HTTP response on rejected upgrade, got err=%v", err)
	}
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("rejected upgrade status = %d, want %d", resp.StatusCode, http.StatusForbidden)
	}
}

// TestHandleConnection_AllowsTrustedOrigins verifies legitimate clients still
// connect: same-origin browsers, loopback dev servers on another port, and
// non-browser clients that send no Origin header.
func TestHandleConnection_AllowsTrustedOrigins(t *testing.T) {
	g, wsURL := startWSGateway(t)

	sameOrigin := "http" + strings.TrimSuffix(strings.TrimPrefix(wsURL, "ws"), "/ws")

	for name, origin := range map[string]string{
		"no origin":              "",
		"same origin":            sameOrigin,
		"loopback cross port":    "http://localhost:5173",
		"loopback 127 with port": "http://127.0.0.1:12345",
	} {
		t.Run(name, func(t *testing.T) {
			conn, resp, err := dialWS(t, wsURL, origin)
			if err != nil {
				status := 0
				if resp != nil {
					status = resp.StatusCode
				}
				t.Fatalf("upgrade with origin %q failed (status %d): %v", origin, status, err)
			}
			waitForClientCount(t, g, 1)
			_ = conn.Close()
			waitForClientCount(t, g, 0)
		})
	}
}
