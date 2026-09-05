package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// fakeCredentialSource is a minimal InstanceCredentialSource test double: it
// accepts exactly one token and exposes a channel the caller can close
// directly, without going through credentialState's rotation bookkeeping.
type fakeCredentialSource struct {
	accepted    string
	invalidated chan struct{}
}

func newFakeCredentialSource(accepted string) *fakeCredentialSource {
	return &fakeCredentialSource{accepted: accepted, invalidated: make(chan struct{})}
}

func (f *fakeCredentialSource) AcceptsFull(token string) bool { return token == f.accepted }
func (f *fakeCredentialSource) Invalidated() <-chan struct{}  { return f.invalidated }

// TestInstanceAuthAcceptsCurrentCredentialRejectsSuperseded pins
// AC-EXECUTORS-CONTROL-OWNERSHIP-002.6: when a credentialSource is wired,
// instanceAuth authenticates every per-instance request dynamically against
// it instead of the static token captured at construction, so a rotation
// takes effect on the very next request with no server restart.
func TestInstanceAuthAcceptsCurrentCredentialRejectsSuperseded(t *testing.T) {
	source := newFakeCredentialSource("current-token")
	s := &Server{credentialSource: source}
	r := gin.New()
	r.Use(s.instanceAuth("static-token-ignored-when-source-set"))
	r.GET("/api/v1/data", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"data": "secret"}) })

	for _, tc := range []struct {
		name  string
		token string
		want  int
	}{
		{name: "current credential accepted", token: "current-token", want: http.StatusOK},
		{name: "superseded credential rejected", token: "static-token-ignored-when-source-set", want: http.StatusUnauthorized},
		{name: "unknown token rejected", token: "some-other-token", want: http.StatusUnauthorized},
		{name: "missing token rejected", token: "", want: http.StatusUnauthorized},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/data", nil)
			if tc.token != "" {
				req.Header.Set("Authorization", "Bearer "+tc.token)
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != tc.want {
				t.Fatalf("status = %d, want %d", w.Code, tc.want)
			}
		})
	}

	// Rotation takes effect immediately on the next request: no caching of
	// AcceptsFull's answer anywhere in the middleware.
	source.accepted = "rotated-token"
	req := httptest.NewRequest(http.MethodGet, "/api/v1/data", nil)
	req.Header.Set("Authorization", "Bearer current-token")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("pre-rotation credential after rotation: status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
	req = httptest.NewRequest(http.MethodGet, "/api/v1/data", nil)
	req.Header.Set("Authorization", "Bearer rotated-token")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("post-rotation credential: status = %d, want %d", w.Code, http.StatusOK)
	}
}

// dialTestWSWithAuth connects a WebSocket client to the test server's
// /api/v1/agent/stream endpoint carrying an Authorization header, so a test
// can dial as a specific credential holder rather than relying on
// instanceAuth being disabled (dialTestWS's no-token path).
func dialTestWSWithAuth(t *testing.T, server *httptest.Server, token string) *websocket.Conn {
	t.Helper()
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/v1/agent/stream"
	header := http.Header{}
	header.Set("Authorization", "Bearer "+token)
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, header)
	if err != nil {
		t.Fatalf("failed to dial WebSocket: %v", err)
	}
	return conn
}

// TestAgentStreamTerminatesWhenCredentialRotates pins
// AC-EXECUTORS-CONTROL-OWNERSHIP-002.2 end to end: a live /agent/stream
// connection authenticated under the current credential must be terminated
// by the server the moment that credential is superseded, so a prior holder
// cannot keep consuming an instance's events. Unlike the credentialState
// unit tests (which only prove the Invalidated channel closes) and the
// instanceAuth test above (which only proves the *next* request is
// rejected), this dials a real, already-open stream and proves the server
// actively tears it down rather than merely refusing new connections.
func TestAgentStreamTerminatesWhenCredentialRotates(t *testing.T) {
	source := newCredentialState("initial-token")
	s := newTestServer(t)
	s.SetCredentialSource(source)
	httpServer := httptest.NewServer(s.router)
	defer httpServer.Close()

	conn := dialTestWSWithAuth(t, httpServer, "initial-token")
	defer func() { _ = conn.Close() }()

	// Prove the connection is alive under the current credential before
	// rotating, so a failure after rotation can't be blamed on a connection
	// that never worked.
	resp := sendWSRequest(t, conn, "agent.stderr", nil)
	if resp.Type != "response" {
		t.Fatalf("pre-rotation request: got type %q, want a response", resp.Type)
	}

	if _, _, err := source.Rotate("initial-token"); err != nil {
		t.Fatalf("Rotate: %v", err)
	}

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, _, err := conn.ReadMessage(); err == nil {
		t.Fatal("ReadMessage after credential rotation = nil error, want the server to have closed the connection")
	}
}
