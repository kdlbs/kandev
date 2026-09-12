package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/kandev/kandev/internal/agentctl/server/process"
)

// The four handlers covered here are the long-lived upgrades that sit behind
// the same instanceAuth group as the agent and workspace streams but forward
// through their own loops. AC-EXECUTORS-CONTROL-OWNERSHIP-002.6 makes the
// credential issued by the highest-numbered rotation the only one that
// authenticates a stream, so each must be torn down when its credential is
// superseded -- otherwise a superseded backend keeps a live PTY, language
// server, or proxied socket into the workspace after being fenced out of
// every ordinary request.

// upstreamEchoWebSocket starts a server that upgrades and echoes every frame,
// standing in for whatever listens on a proxied port.
func upstreamEchoWebSocket(t *testing.T) *httptest.Server {
	t.Helper()
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		for {
			messageType, data, readErr := conn.ReadMessage()
			if readErr != nil {
				return
			}
			if writeErr := conn.WriteMessage(messageType, data); writeErr != nil {
				return
			}
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// startShellTerminalWithAuth starts a PTY-backed terminal through the routed
// server carrying a bearer credential, so the terminal exists as a specific
// holder before its stream is dialled.
func startShellTerminalWithAuth(t *testing.T, server *httptest.Server, token, terminalID string) {
	t.Helper()
	body, err := json.Marshal(shellTerminalStartRequest{TerminalID: terminalID, Cols: 80, Rows: 24})
	if err != nil {
		t.Fatalf("marshal start request: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, server.URL+"/api/v1/shell/terminal/start", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("build start request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("start terminal: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(resp.Body)
		t.Fatalf("start terminal: status = %d (body %s)", resp.StatusCode, payload)
	}
}

// dialWithAuth dials an arbitrary path on the agentctl test server carrying a
// bearer credential, so the connection is established as a specific holder.
func dialWithAuth(t *testing.T, server *httptest.Server, path, token string) *websocket.Conn {
	t.Helper()
	header := http.Header{}
	header.Set("Authorization", "Bearer "+token)
	conn, _, err := websocket.DefaultDialer.Dial(
		"ws"+strings.TrimPrefix(server.URL, "http")+path, header)
	if err != nil {
		t.Fatalf("dial %s: %v", path, err)
	}
	return conn
}

// assertEchoRoundTrip proves a proxied socket is genuinely carrying traffic
// before a rotation, so a post-rotation closure cannot be mistaken for a
// connection that never worked in the first place.
func assertEchoRoundTrip(t *testing.T, conn *websocket.Conn) {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	if err := conn.WriteMessage(websocket.TextMessage, []byte("ping")); err != nil {
		t.Fatalf("pre-rotation write: %v", err)
	}
	_, data, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("pre-rotation read: %v", err)
	}
	if string(data) != "ping" {
		t.Fatalf("pre-rotation echo = %q, want %q", data, "ping")
	}
}

// TestShellTerminalStreamTerminatesWhenCredentialRotates is the highest-value
// case: the terminal stream's read loop writes raw bytes into the PTY, so a
// superseded holder that keeps it open retains arbitrary command execution in
// the workspace long after every other route rejects its credential.
func TestShellTerminalStreamTerminatesWhenCredentialRotates(t *testing.T) {
	source := newCredentialState("initial-token")
	srv := newTestServer(t)
	srv.SetCredentialSource(source)

	httpServer := httptest.NewServer(srv.Router())
	defer httpServer.Close()

	t.Cleanup(func() {
		if err := srv.procMgr.ShellManager().StopAll(); err != nil {
			t.Errorf("stop all terminals: %v", err)
		}
	})
	const terminalID = "term-fencing"
	startShellTerminalWithAuth(t, httpServer, "initial-token", terminalID)

	conn := dialWithAuth(t, httpServer, "/api/v1/shell/terminal/"+terminalID+"/stream", "initial-token")
	defer func() { _ = conn.Close() }()

	// Drive the PTY once so the stream is proven live under the current
	// credential before rotating.
	const marker = "kandev-fencing-marker"
	if err := conn.WriteMessage(websocket.BinaryMessage, []byte("echo "+marker+"\r")); err != nil {
		t.Fatalf("pre-rotation terminal input: %v", err)
	}
	readShellTerminalUntil(t, conn, marker)

	if _, _, err := source.Rotate("initial-token"); err != nil {
		t.Fatalf("Rotate: %v", err)
	}

	assertConnectionClosedByServer(t, conn, 2*time.Second)
}

// TestShellTerminalStreamTerminatesWhenCredentialRotatesBetweenAcceptAndCapture
// pins the same accept/capture window the agent and workspace streams already
// cover: the invalidation channel must come from instanceAuth's own accept
// check, never a fresh Invalidated() call that could hand back the generation
// which replaced the one just authenticated.
func TestShellTerminalStreamTerminatesWhenCredentialRotatesBetweenAcceptAndCapture(t *testing.T) {
	source := newCredentialState("initial-token")
	srv := newTestServer(t)
	srv.SetCredentialSource(source)

	httpServer := httptest.NewServer(srv.Router())
	defer httpServer.Close()

	t.Cleanup(func() {
		if err := srv.procMgr.ShellManager().StopAll(); err != nil {
			t.Errorf("stop all terminals: %v", err)
		}
	})
	const terminalID = "term-fencing-gap"
	startShellTerminalWithAuth(t, httpServer, "initial-token", terminalID)

	afterInstanceAuthAccepted = func() {
		afterInstanceAuthAccepted = nil
		if _, _, err := source.Rotate("initial-token"); err != nil {
			t.Errorf("Rotate in accept/capture gap: %v", err)
		}
	}
	t.Cleanup(func() { afterInstanceAuthAccepted = nil })

	conn := dialWithAuth(t, httpServer, "/api/v1/shell/terminal/"+terminalID+"/stream", "initial-token")
	defer func() { _ = conn.Close() }()

	assertConnectionClosedByServer(t, conn, 2*time.Second)
}

// TestPortProxyTerminatesWhenCredentialRotates covers the proxied socket: the
// port proxy forwards an upgraded connection to an arbitrary local port, so a
// superseded holder keeping one open still reaches whatever listens there.
func TestPortProxyTerminatesWhenCredentialRotates(t *testing.T) {
	upstream := upstreamEchoWebSocket(t)
	port := portOf(t, upstream.URL)

	source := newCredentialState("initial-token")
	srv := newTestServer(t)
	srv.SetCredentialSource(source)

	httpServer := httptest.NewServer(srv.Router())
	defer httpServer.Close()

	conn := dialWithAuth(t, httpServer, "/api/v1/port-proxy/"+strconv.Itoa(port)+"/ws", "initial-token")
	defer func() { _ = conn.Close() }()

	assertEchoRoundTrip(t, conn)

	if _, _, err := source.Rotate("initial-token"); err != nil {
		t.Fatalf("Rotate: %v", err)
	}

	assertConnectionClosedByServer(t, conn, 2*time.Second)
}

// TestVscodeProxyTerminatesWhenCredentialRotates mirrors the port proxy for
// the code-server proxy, which forwards upgraded connections on the same
// terms.
func TestVscodeProxyTerminatesWhenCredentialRotates(t *testing.T) {
	upstream := upstreamEchoWebSocket(t)
	port := portOf(t, upstream.URL)

	source := newCredentialState("initial-token")
	srv := newTestServer(t)
	srv.SetCredentialSource(source)
	srv.procMgr.SetVscodeForTest(process.VscodeStatusRunning, port)

	httpServer := httptest.NewServer(srv.Router())
	defer httpServer.Close()

	conn := dialWithAuth(t, httpServer, "/api/v1/vscode/proxy/ws", "initial-token")
	defer func() { _ = conn.Close() }()

	assertEchoRoundTrip(t, conn)

	if _, _, err := source.Rotate("initial-token"); err != nil {
		t.Fatalf("Rotate: %v", err)
	}

	assertConnectionClosedByServer(t, conn, 2*time.Second)
}
