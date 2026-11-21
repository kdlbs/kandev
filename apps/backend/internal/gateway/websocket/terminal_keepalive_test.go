package websocket

import (
	"testing"
	"time"

	gorillaws "github.com/gorilla/websocket"
	"github.com/kandev/kandev/internal/common/wskeepalive"
)

func TestTerminalKeepaliveIdle(t *testing.T) {
	tests := []struct {
		name  string
		start func(*testing.T, *TerminalHandler)
	}{
		{name: "local-shell", start: startUserShellKeepaliveBridge},
		{name: "local-agent", start: startAgentKeepaliveBridge},
		{name: "remote-bridge", start: startRemoteKeepaliveBridge},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			log, _ := observedTerminalLogger(t)
			handler, _ := newBridgeTestHandler(t, log)
			handler.keepalive = wskeepalive.Timing{WriteWait: time.Second, PongWait: time.Second, PingPeriod: 25 * time.Millisecond}
			tt.start(t, handler)
		})
	}
}

func startUserShellKeepaliveBridge(t *testing.T, handler *TerminalHandler) {
	t.Helper()
	runner := newBridgeTestRunner(t, handler.logger)
	info := startDeferredUserShell(t, runner, "keepalive-env", "keepalive-session", "keepalive-terminal")
	client, server := newTerminalWSPair(t)
	pings := observeTerminalPings(t, client)
	wsw := newWsWriter(server)
	join := spawnJoinable(t, "runUserShellBridge", func() { _ = client.Close() }, func() {
		handler.runUserShellBridge(server, "keepalive-session", "keepalive-env", "keepalive-terminal", info.ID, runner, wsw)
	})
	awaitTerminalPing(t, pings)
	_ = client.Close()
	joinWithin(t, join, "runUserShellBridge after ping")
}

func startAgentKeepaliveBridge(t *testing.T, handler *TerminalHandler) {
	t.Helper()
	runner := newBridgeTestRunner(t, handler.logger)
	client, server := newTerminalWSPair(t)
	pings := observeTerminalPings(t, client)
	wsw := newWsWriter(server)
	join := spawnJoinable(t, "runTerminalBridge", func() { _ = client.Close() }, func() {
		handler.runTerminalBridge(server, "keepalive-session", "proc-gone", runner, wsw)
	})
	awaitTerminalPing(t, pings)
	_ = client.Close()
	joinWithin(t, join, "runTerminalBridge after ping")
}

func startRemoteKeepaliveBridge(t *testing.T, handler *TerminalHandler) {
	t.Helper()
	client, server := newTerminalWSPair(t)
	upstream, agentctl := newTerminalWSPair(t)
	pings := observeTerminalPings(t, client)
	join := spawnJoinable(t, "bridgeBinaryWebSockets", func() { _ = client.Close() }, func() {
		handler.bridgeBinaryWebSockets(server, upstream, "keepalive-terminal")
	})
	awaitTerminalPing(t, pings)
	_ = client.Close()
	joinWithin(t, join, "bridgeBinaryWebSockets after ping")
	_ = agentctl.Close()
}

func observeTerminalPings(t *testing.T, conn *gorillaws.Conn) <-chan struct{} {
	t.Helper()
	pings := make(chan struct{}, 1)
	conn.SetPingHandler(func(string) error {
		select {
		case pings <- struct{}{}:
		default:
		}
		return conn.WriteControl(gorillaws.PongMessage, nil, time.Now().Add(time.Second))
	})
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()
	t.Cleanup(func() {
		_ = conn.Close()
		<-done
	})
	return pings
}

func awaitTerminalPing(t *testing.T, pings <-chan struct{}) {
	t.Helper()
	select {
	case <-pings:
	case <-time.After(wsTestTimeout):
		t.Fatal("browser did not receive a terminal keepalive ping")
	}
}

func shortTerminalKeepalive(pongWait time.Duration) wskeepalive.Timing {
	return wskeepalive.Timing{
		WriteWait:  time.Second,
		PongWait:   pongWait,
		PingPeriod: pongWait / 4,
	}
}
