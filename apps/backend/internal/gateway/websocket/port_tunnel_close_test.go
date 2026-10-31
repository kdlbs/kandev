package websocket

import (
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Model a TCP descriptor whose first closer is still releasing active I/O.
// Subsequent raw closes return ErrClosed without waiting for that release.
type delayedTunnelCloseListener struct {
	net.Listener
	started atomic.Bool
	entered chan struct{}
	release <-chan struct{}
}

func (ln *delayedTunnelCloseListener) Close() error {
	if !ln.started.CompareAndSwap(false, true) {
		return net.ErrClosed
	}
	close(ln.entered)
	<-ln.release
	return ln.Listener.Close()
}

func TestTunnelTeardownWaitsForListenerClosure(t *testing.T) {
	tests := []struct {
		name string
		stop func(*TunnelManager) error
	}{
		{"stop", func(m *TunnelManager) error { return m.StopTunnel("session-close", 3000) }},
		{"invalidate", func(m *TunnelManager) error { m.InvalidateSession("session-close"); return nil }},
		{"shutdown", func(m *TunnelManager) error { m.Shutdown(); return nil }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = raw.Close() })
			release := make(chan struct{})
			var releaseOnce sync.Once
			unblock := func() { releaseOnce.Do(func() { close(release) }) }
			delayed := &delayedTunnelCloseListener{Listener: raw, entered: make(chan struct{}), release: release}
			ln := newTunnelListener(delayed)
			log, _ := observedTerminalLogger(t)
			manager := NewTunnelManager(nil, log)
			t.Cleanup(manager.Shutdown)
			t.Cleanup(unblock)
			manager.tunnels["session-close:3000"] = &tunnelEntry{
				port: 3000, tunnelPort: raw.Addr().(*net.TCPAddr).Port, ln: ln,
				cancel: func() {
					// Cancellation wakes the HTTP server closer before teardown
					// closes the same shared listener itself.
					go func() { _ = ln.Close() }()
					<-delayed.entered
				},
			}
			done := make(chan error, 1)
			go func() { done <- tt.stop(manager) }()
			select {
			case <-delayed.entered:
			case <-time.After(wsTestTimeout):
				t.Fatal("teardown did not start closing the listener")
			}
			select {
			case err := <-done:
				t.Fatalf("teardown returned before the listener finished closing: %v", err)
			case <-time.After(100 * time.Millisecond):
			}
			unblock()
			select {
			case err := <-done:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(wsTestTimeout):
				t.Fatal("teardown did not finish after the listener closed")
			}
			assertPortClosed(t, raw.Addr().(*net.TCPAddr).Port)
		})
	}
}
