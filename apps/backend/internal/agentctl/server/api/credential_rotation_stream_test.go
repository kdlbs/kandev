package api

import (
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/kandev/kandev/internal/agentctl/types"
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

func (f *fakeCredentialSource) AcceptsFullWithInvalidation(token string) (bool, <-chan struct{}) {
	if token != f.accepted {
		return false, nil
	}
	return true, f.invalidated
}

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

// assertConnectionClosedByServer reads from conn and requires the read to
// fail with something other than a deadline timeout. A bare "err == nil
// means still open" check cannot tell a deliberate server-side close apart
// from an untouched connection that simply outlived `within`: both produce
// a non-nil error from ReadMessage, but only the timeout case means the
// fencing mechanism never fired. Confirmed empirically: an identical dial
// with no rotation at all still errors with "i/o timeout" once `within`
// elapses, which a bare nil check would have accepted as proof of closure.
func assertConnectionClosedByServer(t *testing.T, conn *websocket.Conn, within time.Duration) {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(within))
	_, _, err := conn.ReadMessage()
	if err == nil {
		t.Fatal("ReadMessage after credential rotation = nil error, want the server to have closed the connection")
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		t.Fatalf("ReadMessage after credential rotation timed out waiting for the server to close (%v), want the server to close immediately", err)
	}
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

	assertConnectionClosedByServer(t, conn, 2*time.Second)
}

// TestAgentStreamTerminatesWhenCredentialRotatesBetweenAcceptAndCapture pins
// Review round 5 finding 1: instanceAuth's accept check and a stream
// handler's own invalidation-channel capture used to be two independent lock
// acquisitions, so a rotation landing in the gap between them could
// authenticate a request against the generation it just superseded while
// handing the handler a channel for the generation that replaced it -- a
// channel that never closes for the rotation that actually invalidated the
// accepted credential. afterInstanceAuthAccepted deterministically lands a
// rotation in exactly that gap; the fix (atomic AcceptsFullWithInvalidation,
// captured by the middleware and read by the handler from gin context) must
// still close the stream.
func TestAgentStreamTerminatesWhenCredentialRotatesBetweenAcceptAndCapture(t *testing.T) {
	source := newCredentialState("initial-token")
	s := newTestServer(t)
	s.SetCredentialSource(source)
	httpServer := httptest.NewServer(s.router)
	defer httpServer.Close()

	afterInstanceAuthAccepted = func() {
		afterInstanceAuthAccepted = nil
		if _, _, err := source.Rotate("initial-token"); err != nil {
			t.Errorf("Rotate in accept/capture gap: %v", err)
		}
	}
	t.Cleanup(func() { afterInstanceAuthAccepted = nil })

	conn := dialTestWSWithAuth(t, httpServer, "initial-token")
	defer func() { _ = conn.Close() }()

	assertConnectionClosedByServer(t, conn, 2*time.Second)
}

// TestWorkspaceStreamTerminatesWhenCredentialRotatesBetweenAcceptAndCapture
// mirrors TestAgentStreamTerminatesWhenCredentialRotatesBetweenAcceptAndCapture
// for the workspace stream's own credentialInvalidatedFromContext read.
func TestWorkspaceStreamTerminatesWhenCredentialRotatesBetweenAcceptAndCapture(t *testing.T) {
	source := newCredentialState("initial-token")
	s := newTestServer(t)
	s.SetCredentialSource(source)
	httpServer := httptest.NewServer(s.router)
	defer httpServer.Close()

	afterInstanceAuthAccepted = func() {
		afterInstanceAuthAccepted = nil
		if _, _, err := source.Rotate("initial-token"); err != nil {
			t.Errorf("Rotate in accept/capture gap: %v", err)
		}
	}
	t.Cleanup(func() { afterInstanceAuthAccepted = nil })

	conn := dialTestWorkspaceStreamWithAuth(t, httpServer, "initial-token")
	defer func() { _ = conn.Close() }()

	// The handler unconditionally sends a "connected" message as its first
	// frame before the forwarding loop (and its invalidation check) ever
	// runs; consume it before asserting closure, exactly like
	// TestWorkspaceStreamTerminatesWhenCredentialRotates does.
	var connected types.WorkspaceStreamMessage
	if err := conn.ReadJSON(&connected); err != nil {
		t.Fatalf("reading connected message: %v", err)
	}

	assertConnectionClosedByServer(t, conn, 2*time.Second)
}

// dialTestWorkspaceStreamWithAuth connects a WebSocket client to the test
// server's /api/v1/workspace/stream endpoint carrying an Authorization
// header, mirroring dialTestWSWithAuth for the agent stream.
func dialTestWorkspaceStreamWithAuth(t *testing.T, server *httptest.Server, token string) *websocket.Conn {
	t.Helper()
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/v1/workspace/stream"
	header := http.Header{}
	header.Set("Authorization", "Bearer "+token)
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, header)
	if err != nil {
		t.Fatalf("failed to dial WebSocket: %v", err)
	}
	return conn
}

// TestWorkspaceStreamTerminatesWhenCredentialRotates mirrors
// TestAgentStreamTerminatesWhenCredentialRotates for the workspace stream's
// own select-on-Invalidated arm (workspace.go forwardWorkspaceStream,
// AC-EXECUTORS-CONTROL-OWNERSHIP-002.2). Both streams sit behind the same
// instanceAuth middleware group (server.go) but are forwarded by two
// separate goroutines with two separate select statements, so proving one
// terminates on rotation says nothing about the other.
func TestWorkspaceStreamTerminatesWhenCredentialRotates(t *testing.T) {
	source := newCredentialState("initial-token")
	s := newTestServer(t)
	s.SetCredentialSource(source)
	httpServer := httptest.NewServer(s.router)
	defer httpServer.Close()

	conn := dialTestWorkspaceStreamWithAuth(t, httpServer, "initial-token")
	defer func() { _ = conn.Close() }()

	// The handler sends a "connected" message immediately on upgrade, then
	// answers a client ping with a pong -- together they prove the
	// connection and its forwarding goroutine are live under the current
	// credential before rotating, so a failure after rotation can't be
	// blamed on a connection that never worked.
	var connected types.WorkspaceStreamMessage
	if err := conn.ReadJSON(&connected); err != nil {
		t.Fatalf("reading connected message: %v", err)
	}
	if connected.Type != types.WorkspaceMessageTypeConnected {
		t.Fatalf("first message type = %q, want %q", connected.Type, types.WorkspaceMessageTypeConnected)
	}

	if err := conn.WriteJSON(types.NewWorkspacePing()); err != nil {
		t.Fatalf("writing ping: %v", err)
	}
	var pong types.WorkspaceStreamMessage
	if err := conn.ReadJSON(&pong); err != nil {
		t.Fatalf("pre-rotation ping/pong: %v", err)
	}
	if pong.Type != types.WorkspaceMessageTypePong {
		t.Fatalf("pre-rotation response type = %q, want %q", pong.Type, types.WorkspaceMessageTypePong)
	}

	if _, _, err := source.Rotate("initial-token"); err != nil {
		t.Fatalf("Rotate: %v", err)
	}

	assertConnectionClosedByServer(t, conn, 2*time.Second)
}
