package websocket

import (
	"errors"
	"io"
	"sync/atomic"
	"testing"
	"time"

	gorillaws "github.com/gorilla/websocket"
	"github.com/kandev/kandev/internal/common/wskeepalive"
)

func TestTerminalKeepaliveUnresponsivePeer(t *testing.T) {
	log, _ := observedTerminalLogger(t)
	handler, _ := newBridgeTestHandler(t, log)
	handler.keepalive = shortTerminalKeepalive(160 * time.Millisecond)
	silentBrowser, silentBridge := newTerminalWSPair(t)
	silentUpstream, silentAgentctl := newTerminalWSPair(t)
	responsiveBrowser, responsiveBridge := newTerminalWSPair(t)
	responsiveUpstream, responsiveAgentctl := newTerminalWSPair(t)
	silentFrames := observeTerminalFrames(t, silentBrowser, false)
	responsiveFrames := observeTerminalFrames(t, responsiveBrowser, true)
	silentDone := spawnJoinable(t, "silent remote bridge", func() { _ = silentBrowser.Close() }, func() {
		handler.bridgeBinaryWebSockets(silentBridge, silentUpstream, "silent-terminal")
	})
	responsiveDone := spawnJoinable(t, "responsive remote bridge", func() { _ = responsiveBrowser.Close() }, func() {
		handler.bridgeBinaryWebSockets(responsiveBridge, responsiveUpstream, "responsive-terminal")
	})

	var outputWrites atomic.Int32
	stopOutput := make(chan struct{})
	outputDone := make(chan struct{})
	go func() {
		defer close(outputDone)
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stopOutput:
				return
			case <-ticker.C:
				if err := silentAgentctl.WriteMessage(gorillaws.BinaryMessage, []byte("ongoing output")); err != nil {
					return
				}
				outputWrites.Add(1)
			}
		}
	}()
	waitTerminalPing(t, silentFrames.pings)
	waitTerminalPing(t, responsiveFrames.pings)
	select {
	case err := <-silentFrames.closed:
		if err == nil {
			t.Fatal("unresponsive browser closed without a read error")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("unresponsive browser remained connected despite missing pongs")
	}
	close(stopOutput)
	<-outputDone
	if got := outputWrites.Load(); got < 3 {
		t.Fatalf("ongoing output writes before liveness close = %d, want at least 3", got)
	}
	joinWithin(t, silentDone, "silent remote bridge")

	output := []byte("responsive-peer-still-live")
	if err := responsiveAgentctl.WriteMessage(gorillaws.BinaryMessage, output); err != nil {
		t.Fatalf("write to responsive peer: %v", err)
	}
	if got := waitTerminalFrame(t, responsiveFrames.frames); string(got.data) != string(output) {
		t.Fatalf("responsive peer output = %q, want %q", got.data, output)
	}
	input := []byte("responsive-input")
	if err := responsiveBrowser.WriteMessage(gorillaws.BinaryMessage, input); err != nil {
		t.Fatalf("write from responsive peer: %v", err)
	}
	if got := readBinary(t, responsiveAgentctl); string(got) != string(input) {
		t.Fatalf("responsive peer input = %q, want %q", got, input)
	}
	_ = responsiveBrowser.Close()
	joinWithin(t, responsiveDone, "responsive remote bridge")
}

func TestTerminalKeepaliveConcurrentOutput(t *testing.T) {
	log, _ := observedTerminalLogger(t)
	handler, _ := newBridgeTestHandler(t, log)
	handler.keepalive = shortTerminalKeepalive(240 * time.Millisecond)
	runner := newBridgeTestRunner(t, log)
	browser, server := newTerminalWSPair(t)
	frames := observeTerminalFrames(t, browser, true)
	wsw := newWsWriter(server)
	bridgeDone := spawnJoinable(t, "terminal bridge with output", func() { _ = browser.Close() }, func() {
		handler.runTerminalBridge(server, "concurrent-output", "proc-gone", runner, wsw)
	})
	writerDone := make(chan error, 1)
	go func() {
		for range 100 {
			if _, err := wsw.Write([]byte("pty-output")); err != nil {
				writerDone <- err
				return
			}
			time.Sleep(5 * time.Millisecond)
		}
		writerDone <- nil
	}()
	for range 3 {
		waitTerminalPing(t, frames.pings)
	}
	if err := <-writerDone; err != nil {
		t.Fatalf("concurrent PTY output write: %v", err)
	}
	if got := len(frames.frames); got < 10 {
		t.Fatalf("received output frames = %d, want at least 10", got)
	}
	_ = browser.Close()
	joinWithin(t, bridgeDone, "terminal bridge with output")
}

func TestRunTerminalBridgeKeepaliveSetupFailureCleansUp(t *testing.T) {
	log, _ := observedTerminalLogger(t)
	handler, _ := newBridgeTestHandler(t, log)
	handler.keepalive = wskeepalive.Timing{}
	runner := newBridgeTestRunner(t, log)
	info := startDeferredPassthrough(t, runner, "setup-failure-session")
	browser, server := newTerminalWSPair(t)
	wsw := newWsWriter(server)
	const sessionID = "setup-failure-session"
	runner.TrackSessionWebSocket(sessionID, wsw)
	if !runner.HasActiveWebSocketBySession(sessionID) {
		t.Fatal("session output was not tracked before setup failure")
	}
	if err := server.Close(); err != nil {
		t.Fatalf("close server connection: %v", err)
	}

	handler.runTerminalBridge(server, sessionID, info.ID, runner, wsw)

	if runner.HasActiveWebSocketBySession(sessionID) {
		t.Fatal("session output remained tracked after keepalive setup failure")
	}
	if _, err := wsw.Write([]byte("late")); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("wsWriter error = %v, want io.ErrClosedPipe", err)
	}
	if !runner.IsProcessReadyOrPending(info.ID) {
		t.Fatal("keepalive setup failure stopped the retained passthrough process")
	}
	_ = browser.Close()
}

func TestRunUserShellBridgeKeepaliveSetupFailureCleansUp(t *testing.T) {
	log, _ := observedTerminalLogger(t)
	handler, _ := newBridgeTestHandler(t, log)
	handler.keepalive = wskeepalive.Timing{}
	runner := newBridgeTestRunner(t, log)
	const (
		scopeID    = "setup-failure-env"
		terminalID = "setup-failure-terminal"
		sessionID  = "setup-failure-user-shell"
	)
	info := startDeferredUserShell(t, runner, scopeID, sessionID, terminalID)
	browser, server := newTerminalWSPair(t)
	wsw := newWsWriter(server)
	if err := runner.SetDirectOutput(info.ID, wsw); err != nil {
		t.Fatalf("SetDirectOutput: %v", err)
	}
	if !runner.HasActiveWebSocket(info.ID) {
		t.Fatal("user shell output was not tracked before setup failure")
	}
	if err := server.Close(); err != nil {
		t.Fatalf("close server connection: %v", err)
	}

	handler.runUserShellBridge(server, sessionID, scopeID, terminalID, info.ID, runner, wsw)

	if runner.HasActiveWebSocket(info.ID) {
		t.Fatal("user shell output remained tracked after keepalive setup failure")
	}
	if _, err := wsw.Write([]byte("late")); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("wsWriter error = %v, want io.ErrClosedPipe", err)
	}
	if !runner.IsProcessReadyOrPending(info.ID) {
		t.Fatal("keepalive setup failure stopped the retained user shell")
	}
	_ = browser.Close()
}
