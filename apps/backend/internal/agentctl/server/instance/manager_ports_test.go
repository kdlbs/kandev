package instance

import (
	"context"
	"net"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/config"
)

func TestAllocatePortAndListenerRetriesAddressInUse(t *testing.T) {
	var occupied net.Listener
	var base int
	for attempt := 0; attempt < 10; attempt++ {
		candidate, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("listen for occupied port: %v", err)
		}
		candidatePort := candidate.Addr().(*net.TCPAddr).Port
		if candidatePort < 65535 {
			occupied = candidate
			base = candidatePort
			break
		}
		_ = candidate.Close()
	}
	if occupied == nil {
		t.Skip("could not reserve an occupied port with a valid consecutive retry range")
	}
	t.Cleanup(func() { _ = occupied.Close() })

	mgr := NewManager(&config.Config{
		Ports: config.PortConfig{Base: base, Max: base + 1},
	}, newTestLogger(t))
	t.Cleanup(func() { _ = mgr.Shutdown(context.Background()) })

	port, listener, err := mgr.allocatePortAndListener("retry")
	if err != nil {
		t.Fatalf("allocatePortAndListener: %v", err)
	}
	t.Cleanup(func() {
		_ = listener.Close()
		mgr.portAlloc.Release(port)
	})

	if port != base+1 {
		t.Fatalf("allocated port = %d, want retry on %d", port, base+1)
	}
}
