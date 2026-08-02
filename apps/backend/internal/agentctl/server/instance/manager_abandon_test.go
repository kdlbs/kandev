package instance

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/pkg/agent"
)

// TestCreateInstanceAbandonsWhenCallerGaveUp covers the leak that made instance
// creation collapse under load. Creation is serialised on m.mu and the control
// client gives up after 30s, so a queue fills with requests nobody awaits; each
// one used to build a full instance — port, HTTP server, workspace trackers —
// that no caller could ever stop, and the polling those trackers did made the
// next creation slower still.
func TestCreateInstanceAbandonsWhenCallerGaveUp(t *testing.T) {
	log := newTestLogger(t)
	mgr := NewManager(&config.Config{
		Ports:    config.PortConfig{Base: 0, Max: 0},
		Defaults: config.InstanceDefaults{Protocol: agent.ProtocolACP},
	}, log)
	t.Cleanup(func() { _ = mgr.Shutdown(context.Background()) })

	// The caller timed out while this request sat in the queue.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	resp, err := mgr.CreateInstance(ctx, &CreateRequest{WorkspacePath: t.TempDir()})
	if err == nil {
		t.Fatalf("expected an error for a caller that had gone, got instance %+v", resp)
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("error = %v, want it to wrap context.Canceled", err)
	}
	if resp != nil {
		t.Errorf("returned a response %+v alongside the error; the caller would treat the instance as live", resp)
	}

	mgr.mu.RLock()
	live := len(mgr.instances)
	mgr.mu.RUnlock()
	if live != 0 {
		t.Errorf("registered %d instances for a caller that had gone, want 0", live)
	}
}

// TestAbandonPartialInstanceReleasesPort pins the unwind path taken when the
// caller disappears mid-creation: the port must go back to the allocator and
// the listener must close, or the pool drains one entry per abandoned request.
func TestAbandonPartialInstanceReleasesPort(t *testing.T) {
	log := newTestLogger(t)
	mgr := NewManager(&config.Config{
		Ports:    config.PortConfig{Base: 0, Max: 0},
		Defaults: config.InstanceDefaults{Protocol: agent.ProtocolACP},
	}, log)
	t.Cleanup(func() { _ = mgr.Shutdown(context.Background()) })

	port, listener, err := mgr.allocatePortAndListener("abandoned")
	if err != nil {
		t.Fatalf("allocatePortAndListener: %v", err)
	}
	addr := listener.Addr().String()

	// A nil process manager stands in for abandonment before one exists; the
	// port and the listener still have to be given back.
	mgr.abandonPartialInstance("abandoned", port, listener, nil)

	// The listener is closed, so the address is bindable again.
	reopened, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatalf("port %d still bound after abandon: %v", port, err)
	}
	_ = reopened.Close()

	// And the allocator handed the port back rather than holding it as in use.
	next, nextListener, err := mgr.allocatePortAndListener("next")
	if err != nil {
		t.Fatalf("allocatePortAndListener after abandon: %v", err)
	}
	_ = nextListener.Close()
	mgr.portAlloc.Release(next)
}

// TestCreateInstanceAbandonsAfterTrackerStartup drives the second context check
// deterministically. The caller's context is live when CreateInstance takes the
// lock and is cancelled exactly once the trackers have started, via the
// afterTrackerStart seam — so the abandonment branch is guaranteed to run
// rather than merely likely to.
func TestCreateInstanceAbandonsAfterTrackerStartup(t *testing.T) {
	log := newTestLogger(t)
	mgr := NewManager(&config.Config{
		Ports:    config.PortConfig{Base: 0, Max: 0},
		Defaults: config.InstanceDefaults{Protocol: agent.ProtocolACP},
	}, log)

	ctx, cancel := context.WithCancel(context.Background())
	mgr.afterTrackerStart = cancel

	resp, err := mgr.CreateInstance(ctx, &CreateRequest{WorkspacePath: t.TempDir()})
	if err == nil {
		t.Fatalf("expected an error once the caller was cancelled mid-creation, got %+v", resp)
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("error = %v, want it to wrap context.Canceled", err)
	}
	if !strings.Contains(err.Error(), "during startup") {
		t.Errorf("error = %v, want the post-startup branch rather than the queued one", err)
	}
	if resp != nil {
		t.Errorf("returned response %+v alongside the error", resp)
	}

	mgr.mu.RLock()
	live := len(mgr.instances)
	mgr.mu.RUnlock()
	if live != 0 {
		t.Errorf("registered %d instances after abandoning creation, want 0", live)
	}

	// Shutdown must wait for the teardown goroutine rather than racing it.
	if err := mgr.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	// With teardown drained, the port is back in the pool and bindable.
	port, listener, err := mgr.allocatePortAndListener("after-abandon")
	if err != nil {
		t.Fatalf("port was not released by the abandoned creation: %v", err)
	}
	_ = listener.Close()
	mgr.portAlloc.Release(port)
}

// TestCreateInstanceRefusedAfterShutdown closes the ordering window Shutdown
// would otherwise leave: it could observe abandonWG at zero and a creation
// already past its own checks could register a teardown behind the Wait.
func TestCreateInstanceRefusedAfterShutdown(t *testing.T) {
	log := newTestLogger(t)
	mgr := NewManager(&config.Config{
		Ports:    config.PortConfig{Base: 0, Max: 0},
		Defaults: config.InstanceDefaults{Protocol: agent.ProtocolACP},
	}, log)

	if err := mgr.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	resp, err := mgr.CreateInstance(context.Background(), &CreateRequest{WorkspacePath: t.TempDir()})
	if !errors.Is(err, ErrManagerShuttingDown) {
		t.Fatalf("error = %v, want ErrManagerShuttingDown", err)
	}
	if resp != nil {
		t.Errorf("returned response %+v after shutdown", resp)
	}

	mgr.mu.RLock()
	live := len(mgr.instances)
	mgr.mu.RUnlock()
	if live != 0 {
		t.Errorf("registered %d instances after shutdown, want 0", live)
	}
}
