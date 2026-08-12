package websocket

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	gorillaws "github.com/gorilla/websocket"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"github.com/kandev/kandev/internal/common/logger"
)

// wsTestTimeout bounds every blocking read/join in the terminal WebSocket
// tests. All of them join on an observable side effect, so the timeout only
// fires when the production code genuinely failed to act.
const wsTestTimeout = 3 * time.Second

var testWSUpgrader = gorillaws.Upgrader{
	CheckOrigin: func(*http.Request) bool { return true },
}

// newTerminalWSPair returns a connected pair of gorilla WebSocket connections:
// the first is the dialing (browser) side, the second the accepting (server)
// side. Both are closed on cleanup, which is what lets the goleak TestMain in
// this package stay green even when a test fails early.
func newTerminalWSPair(t *testing.T) (dialed, accepted *gorillaws.Conn) {
	t.Helper()

	accepts := make(chan *gorillaws.Conn, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := testWSUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		accepts <- conn
	}))
	t.Cleanup(srv.Close)

	conn, resp, err := gorillaws.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http"), nil)
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	select {
	case server := <-accepts:
		t.Cleanup(func() { _ = server.Close() })
		return conn, server
	case <-time.After(wsTestTimeout):
		t.Fatal("timed out waiting for the server side of the websocket pair")
		return nil, nil
	}
}

// observedTerminalLogger returns a logger whose records are captured in memory.
// Several terminal-pump branches (dropped input, resize fallbacks) have no
// other in-process observable, so the emitted record is the assertion target.
func observedTerminalLogger(t *testing.T) (*logger.Logger, *observer.ObservedLogs) {
	t.Helper()
	core, recorded := observer.New(zap.DebugLevel)
	log, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("create observed logger: %v", err)
	}
	return log, recorded
}

// readBinary reads one message with a bounded deadline and asserts it is a
// binary frame.
func readBinary(t *testing.T, conn *gorillaws.Conn) []byte {
	t.Helper()
	if err := conn.SetReadDeadline(time.Now().Add(wsTestTimeout)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}
	msgType, data, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read message: %v", err)
	}
	if msgType != gorillaws.BinaryMessage {
		t.Fatalf("message type = %d, want %d (binary)", msgType, gorillaws.BinaryMessage)
	}
	return data
}

// joinWithin waits for done to close, failing the test when the production
// goroutine under test did not terminate.
func joinWithin(t *testing.T, done <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(wsTestTimeout):
		t.Fatalf("%s did not return within %s", what, wsTestTimeout)
	}
}

// countLogged returns how many captured records carry the given message.
func countLogged(recorded *observer.ObservedLogs, message string) int {
	return len(recorded.FilterMessage(message).All())
}
