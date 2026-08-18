package websocket

import (
	"errors"
	"io"
	"testing"
	"time"

	gorillaws "github.com/gorilla/websocket"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
)

// newBridgeTestHandler builds a TerminalHandler backed by a real (dependency-free)
// lifecycle manager, which is all the pump code needs.
func newBridgeTestHandler(t *testing.T, log *logger.Logger) (*TerminalHandler, *lifecycle.Manager) {
	t.Helper()
	manager := lifecycle.NewManager(
		nil,
		bus.NewMemoryEventBus(log),
		nil,
		nil,
		nil,
		nil,
		lifecycle.ExecutorFallbackDeny,
		t.TempDir(),
		log,
	)
	return NewTerminalHandler(manager, nil, nil, log), manager
}

func newBridgeTestRunner(t *testing.T, log *logger.Logger) *process.InteractiveRunner {
	t.Helper()
	return process.NewInteractiveRunner(nil, log, 2*1024*1024)
}

func TestWsWriterSendsBinaryFramesAndRefusesWritesAfterClose(t *testing.T) {
	browser, server := newTerminalWSPair(t)
	wsw := newWsWriter(server)
	t.Cleanup(func() { _ = wsw.Close() })

	payload := []byte("\x1b[32mready\x1b[0m$ ")
	n, err := wsw.Write(payload)
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if n != len(payload) {
		t.Fatalf("Write() n = %d, want %d", n, len(payload))
	}
	if got := readBinary(t, browser); string(got) != string(payload) {
		t.Fatalf("browser received %q, want %q", got, payload)
	}

	if err := wsw.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	n, err = wsw.Write([]byte("after close"))
	if !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("Write() after Close error = %v, want io.ErrClosedPipe", err)
	}
	if n != 0 {
		t.Fatalf("Write() after Close n = %d, want 0", n)
	}

	// Nothing may reach the peer once the writer is closed.
	if err := browser.SetReadDeadline(time.Now().Add(150 * time.Millisecond)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}
	if _, data, err := browser.ReadMessage(); err == nil {
		t.Fatalf("browser received %q after wsWriter.Close(), want no frame", data)
	}
}

func TestWsWriterReportsUnderlyingConnectionFailure(t *testing.T) {
	_, server := newTerminalWSPair(t)
	wsw := newWsWriter(server)
	t.Cleanup(func() { _ = wsw.Close() })

	if err := server.Close(); err != nil {
		t.Fatalf("close server conn: %v", err)
	}
	n, err := wsw.Write([]byte("orphaned"))
	if err == nil {
		t.Fatal("Write() on a closed connection returned nil error")
	}
	if n != 0 {
		t.Fatalf("Write() n = %d, want 0 on failure", n)
	}
}

func TestBridgeBinaryWebSocketsForwardsBothDirections(t *testing.T) {
	log, _ := observedTerminalLogger(t)
	handler, _ := newBridgeTestHandler(t, log)

	browser, bridgeClientSide := newTerminalWSPair(t)
	bridgeUpstreamSide, agentctl := newTerminalWSPair(t)

	done := spawnJoinable(t, "bridgeBinaryWebSockets", func() { _ = browser.Close() }, func() {
		handler.bridgeBinaryWebSockets(bridgeClientSide, bridgeUpstreamSide, "term-1")
	})

	// client → agentctl (keystrokes)
	if err := browser.WriteMessage(gorillaws.BinaryMessage, []byte("echo hi\r")); err != nil {
		t.Fatalf("browser write: %v", err)
	}
	if got := readBinary(t, agentctl); string(got) != "echo hi\r" {
		t.Fatalf("agentctl received %q, want %q", got, "echo hi\r")
	}

	// agentctl → client (shell output)
	if err := agentctl.WriteMessage(gorillaws.BinaryMessage, []byte("hi\r\n$ ")); err != nil {
		t.Fatalf("agentctl write: %v", err)
	}
	if got := readBinary(t, browser); string(got) != "hi\r\n$ " {
		t.Fatalf("browser received %q, want %q", got, "hi\r\n$ ")
	}

	// A client disconnect must tear the upstream connection down too, otherwise
	// the agentctl→client goroutine blocks forever and the bridge never returns.
	if err := browser.Close(); err != nil {
		t.Fatalf("close browser conn: %v", err)
	}
	joinWithin(t, done, "bridgeBinaryWebSockets")

	if err := agentctl.SetReadDeadline(time.Now().Add(wsTestTimeout)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}
	if _, _, err := agentctl.ReadMessage(); err == nil {
		t.Fatal("agentctl connection still readable after the client disconnected")
	}
}

func TestBridgeBinaryWebSocketsReturnsWhenUpstreamDisconnects(t *testing.T) {
	log, _ := observedTerminalLogger(t)
	handler, _ := newBridgeTestHandler(t, log)

	browser, bridgeClientSide := newTerminalWSPair(t)
	bridgeUpstreamSide, agentctl := newTerminalWSPair(t)

	done := spawnJoinable(t, "bridgeBinaryWebSockets", func() { _ = agentctl.Close() }, func() {
		handler.bridgeBinaryWebSockets(bridgeClientSide, bridgeUpstreamSide, "term-2")
	})

	if err := agentctl.Close(); err != nil {
		t.Fatalf("close agentctl conn: %v", err)
	}
	joinWithin(t, done, "bridgeBinaryWebSockets")
	_ = browser.Close()
}

func TestRunTerminalBridgeReleasesSessionOutputOnDisconnect(t *testing.T) {
	log, recorded := observedTerminalLogger(t)
	handler, _ := newBridgeTestHandler(t, log)
	runner := newBridgeTestRunner(t, log)

	browser, server := newTerminalWSPair(t)
	wsw := newWsWriter(server)

	const sessionID = "session-bridge"
	runner.TrackSessionWebSocket(sessionID, wsw)
	if !runner.HasActiveWebSocketBySession(sessionID) {
		t.Fatal("session WebSocket was not tracked before the bridge started")
	}

	// "proc-gone" is deliberately unknown to the runner: PTY setup fails, so the
	// bridge exercises its drop-input path without spawning a real PTY.
	done := spawnJoinable(t, "runTerminalBridge", func() { _ = browser.Close() }, func() {
		handler.runTerminalBridge(server, sessionID, "proc-gone", runner, wsw)
	})

	if err := browser.WriteMessage(gorillaws.BinaryMessage, nil); err != nil {
		t.Fatalf("write empty frame: %v", err)
	}
	if err := browser.WriteMessage(gorillaws.BinaryMessage, []byte("ls\n")); err != nil {
		t.Fatalf("write input frame: %v", err)
	}
	if err := browser.Close(); err != nil {
		t.Fatalf("close browser conn: %v", err)
	}
	joinWithin(t, done, "runTerminalBridge")

	if runner.HasActiveWebSocketBySession(sessionID) {
		t.Fatal("session direct output was not cleared when the terminal disconnected")
	}
	if _, err := wsw.Write([]byte("late")); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("wsWriter usable after bridge exit: err = %v, want io.ErrClosedPipe", err)
	}
	if got := countLogged(recorded, "PTY not ready, dropping input"); got != 1 {
		t.Fatalf("dropped-input records = %d, want 1 (the empty frame must be skipped)", got)
	}
}

func TestRunTerminalBridgeResizeFallsBackWhenProcessAndSessionAreGone(t *testing.T) {
	log, recorded := observedTerminalLogger(t)
	handler, _ := newBridgeTestHandler(t, log)
	runner := newBridgeTestRunner(t, log)

	browser, server := newTerminalWSPair(t)
	wsw := newWsWriter(server)

	done := spawnJoinable(t, "runTerminalBridge", func() { _ = browser.Close() }, func() {
		handler.runTerminalBridge(server, "session-resize", "proc-gone", runner, wsw)
	})

	resize := append([]byte{resizeCommandByte}, []byte(`{"cols":120,"rows":40}`)...)
	if err := browser.WriteMessage(gorillaws.BinaryMessage, resize); err != nil {
		t.Fatalf("write resize frame: %v", err)
	}
	if err := browser.Close(); err != nil {
		t.Fatalf("close browser conn: %v", err)
	}
	joinWithin(t, done, "runTerminalBridge")

	if got := countLogged(recorded, "failed to resize PTY (both process and session)"); got != 1 {
		t.Fatalf("resize-fallback records = %d, want 1", got)
	}
}

func TestRunUserShellBridgeClearsShellDirectOutputOnDisconnect(t *testing.T) {
	log, recorded := observedTerminalLogger(t)
	handler, _ := newBridgeTestHandler(t, log)
	runner := newBridgeTestRunner(t, log)

	const (
		scopeID    = "env-shell"
		terminalID = "term-shell"
		sessionID  = "session-shell"
	)
	info := startDeferredUserShell(t, runner, scopeID, sessionID, terminalID)

	browser, server := newTerminalWSPair(t)
	wsw := newWsWriter(server)
	if err := runner.SetDirectOutput(info.ID, wsw); err != nil {
		t.Fatalf("SetDirectOutput: %v", err)
	}
	if !runner.HasActiveWebSocket(info.ID) {
		t.Fatal("direct output was not registered on the user shell process")
	}

	done := spawnJoinable(t, "runUserShellBridge", func() { _ = browser.Close() }, func() {
		handler.runUserShellBridge(server, sessionID, scopeID, terminalID, info.ID, runner, wsw)
	})

	if err := browser.WriteMessage(gorillaws.BinaryMessage, nil); err != nil {
		t.Fatalf("write empty frame: %v", err)
	}
	if err := browser.WriteMessage(gorillaws.BinaryMessage, []byte("pwd\n")); err != nil {
		t.Fatalf("write input frame: %v", err)
	}
	if err := browser.Close(); err != nil {
		t.Fatalf("close browser conn: %v", err)
	}
	joinWithin(t, done, "runUserShellBridge")

	if runner.HasActiveWebSocket(info.ID) {
		t.Fatal("user shell direct output was not cleared when the terminal disconnected")
	}
	if _, err := wsw.Write([]byte("late")); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("wsWriter usable after bridge exit: err = %v, want io.ErrClosedPipe", err)
	}
	if got := countLogged(recorded, "PTY not ready, dropping input"); got != 1 {
		t.Fatalf("dropped-input records = %d, want 1 (the empty frame must be skipped)", got)
	}
}

func TestRunUserShellBridgeReportsUnresizableShell(t *testing.T) {
	log, recorded := observedTerminalLogger(t)
	handler, _ := newBridgeTestHandler(t, log)
	runner := newBridgeTestRunner(t, log)

	browser, server := newTerminalWSPair(t)
	wsw := newWsWriter(server)

	done := spawnJoinable(t, "runUserShellBridge", func() { _ = browser.Close() }, func() {
		handler.runUserShellBridge(server, "session-x", "env-x", "term-unknown", "proc-gone", runner, wsw)
	})

	resize := append([]byte{resizeCommandByte}, []byte(`{"cols":100,"rows":30}`)...)
	if err := browser.WriteMessage(gorillaws.BinaryMessage, resize); err != nil {
		t.Fatalf("write resize frame: %v", err)
	}
	if err := browser.Close(); err != nil {
		t.Fatalf("close browser conn: %v", err)
	}
	joinWithin(t, done, "runUserShellBridge")

	if got := countLogged(recorded, "failed to resize user shell PTY"); got != 1 {
		t.Fatalf("user-shell resize failure records = %d, want 1", got)
	}
}
