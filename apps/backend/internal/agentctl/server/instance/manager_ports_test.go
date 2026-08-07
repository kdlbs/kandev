package instance

import (
	"context"
	"net"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/config"
)

func TestAllocatePortAndListenerRetriesAddressInUse(t *testing.T) {
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen for occupied port: %v", err)
	}
	t.Cleanup(func() { _ = occupied.Close() })

	base := occupied.Addr().(*net.TCPAddr).Port
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
