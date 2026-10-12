package instance

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

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

	lease, listener, err := mgr.allocatePortAndListener("abandoned")
	if err != nil {
		t.Fatalf("allocatePortAndListener: %v", err)
	}
	addr := listener.Addr().String()

	// A nil process manager stands in for abandonment before one exists; the
	// port and the listener still have to be given back.
	bundle := &provisionalInstance{id: "abandoned", lease: lease, listener: listener}
	if err := mgr.abandonPartialInstance(bundle); err != nil {
		t.Fatalf("abandonPartialInstance: %v", err)
	}

	// The listener is closed, so the address is bindable again.
	reopened, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatalf("port %d still bound after abandon: %v", lease.Port, err)
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

func TestAbandonRetainsLeaseOnCleanupFailure(t *testing.T) {
	mgr := NewManager(&config.Config{Ports: config.PortConfig{Base: 0, Max: 0}}, newTestLogger(t))
	lease, listener, err := mgr.allocatePortAndListener("abandoned")
	if err != nil {
		t.Fatalf("allocatePortAndListener: %v", err)
	}
	processErr := errors.New("tracker cleanup failed")
	procMgr := &fakeProcessManager{stopErr: processErr}
	bundle := &provisionalInstance{id: "abandoned", lease: lease, listener: listener, procMgr: procMgr}
	mgr.mu.Lock()
	mgr.provisional[bundle.id] = bundle
	mgr.mu.Unlock()

	err = mgr.cleanupProvisionalInstance(context.Background(), bundle)
	if !errors.Is(err, processErr) {
		t.Fatalf("cleanup error = %v, want %v", err, processErr)
	}
	if !bundle.listenerClosed {
		t.Fatal("listener was not closed before process teardown failed")
	}
	if !errors.Is(bundle.lastCleanupErr, processErr) {
		t.Fatalf("stored cleanup error = %v, want %v", bundle.lastCleanupErr, processErr)
	}
	if got := mgr.portAlloc.owners[lease.Owner]; got != lease {
		t.Fatalf("owner index = %+v, want retained lease %+v", got, lease)
	}
	if _, ok := mgr.provisional[bundle.id]; !ok {
		t.Fatal("failed provisional bundle was discarded")
	}

	procMgr.stopErr = nil
	if err := mgr.cleanupProvisionalInstance(context.Background(), bundle); err != nil {
		t.Fatalf("cleanup retry: %v", err)
	}
	if _, ok := mgr.provisional[bundle.id]; ok {
		t.Fatal("successful cleanup retained the provisional bundle")
	}
	if _, err := mgr.portAlloc.Allocate("successor"); err != nil {
		t.Fatalf("successful retry did not release lease: %v", err)
	}
}

func TestAbandonRetrySameOwnerDoesNotRebind(t *testing.T) {
	mgr := NewManager(&config.Config{
		Ports:    config.PortConfig{Base: 0, Max: 0},
		Defaults: config.InstanceDefaults{Protocol: agent.ProtocolACP},
	}, newTestLogger(t))
	lease, listener, err := mgr.allocatePortAndListener("same-owner")
	if err != nil {
		t.Fatalf("allocatePortAndListener: %v", err)
	}
	stopStarted := make(chan struct{})
	stopRelease := make(chan struct{})
	procMgr := &fakeProcessManager{stopErr: errors.New("retain for retry"), stopStarted: stopStarted, stopRelease: stopRelease}
	bundle := &provisionalInstance{id: "same-owner", lease: lease, listener: listener, procMgr: procMgr}
	mgr.mu.Lock()
	mgr.provisional[bundle.id] = bundle
	mgr.mu.Unlock()

	cleanupDone := make(chan error, 1)
	go func() { cleanupDone <- mgr.cleanupProvisionalInstance(context.Background(), bundle) }()
	select {
	case <-stopStarted:
	case <-time.After(time.Second):
		t.Fatal("cleanup did not reach process teardown")
	}

	createDone := make(chan error, 1)
	go func() {
		_, err := mgr.CreateInstance(context.Background(), &CreateRequest{ID: "same-owner", WorkspacePath: t.TempDir()})
		createDone <- err
	}()
	select {
	case err := <-createDone:
		if err == nil || !strings.Contains(err.Error(), "still cleaning up") {
			t.Fatalf("CreateInstance error = %v, want provisional cleanup guard", err)
		}
	case <-time.After(time.Second):
		t.Fatal("CreateInstance waited on manager lock held by cleanup")
	}

	close(stopRelease)
	if err := <-cleanupDone; err == nil {
		t.Fatal("cleanup error was lost")
	}
	if err := mgr.Shutdown(context.Background()); err == nil {
		t.Fatal("shutdown retry should report the persistent cleanup failure")
	}
}

func TestShutdownRetriesFailedAbandon(t *testing.T) {
	mgr := NewManager(&config.Config{Ports: config.PortConfig{Base: 0, Max: 0}}, newTestLogger(t))
	lease, listener, err := mgr.allocatePortAndListener("shutdown-retry")
	if err != nil {
		t.Fatalf("allocatePortAndListener: %v", err)
	}
	procMgr := &fakeProcessManager{stopErr: errors.New("first cleanup failure")}
	bundle := &provisionalInstance{id: "shutdown-retry", lease: lease, listener: listener, procMgr: procMgr}
	mgr.mu.Lock()
	mgr.provisional[bundle.id] = bundle
	mgr.mu.Unlock()
	if err := mgr.abandonPartialInstance(bundle); err == nil {
		t.Fatal("initial cleanup succeeded, want injected failure")
	}
	procMgr.stopErr = nil

	if err := mgr.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown retry: %v", err)
	}
	if !bundle.cleaned {
		t.Fatal("shutdown did not complete retained cleanup")
	}
	if bundle.lastCleanupErr != nil {
		t.Fatalf("stored cleanup error = %v after successful retry, want nil", bundle.lastCleanupErr)
	}
	if procMgr.stopCalls != 2 {
		t.Fatalf("StopForTeardown calls = %d, want initial attempt and one shutdown retry", procMgr.stopCalls)
	}
	if _, err := mgr.portAlloc.Allocate("after-shutdown"); err != nil {
		t.Fatalf("shutdown retry did not release lease: %v", err)
	}
}

func TestAbandonListenerCloseFailureRetainsLease(t *testing.T) {
	mgr := NewManager(&config.Config{Ports: config.PortConfig{Base: 41001, Max: 41001}}, newTestLogger(t))
	lease, err := mgr.portAlloc.Allocate("listener-close")
	if err != nil {
		t.Fatalf("Allocate: %v", err)
	}
	listener := &fakeNetListener{closeErr: errors.New("listener close failed")}
	bundle := &provisionalInstance{id: "listener-close", lease: lease, listener: listener}
	mgr.mu.Lock()
	mgr.provisional[bundle.id] = bundle
	mgr.mu.Unlock()

	if err := mgr.cleanupProvisionalInstance(context.Background(), bundle); err == nil {
		t.Fatal("cleanup succeeded despite listener close failure")
	}
	if got := mgr.portAlloc.owners[lease.Owner]; got != lease {
		t.Fatalf("owner index = %+v, want retained lease %+v", got, lease)
	}
	listener.closeErr = nil
	if err := mgr.cleanupProvisionalInstance(context.Background(), bundle); err != nil {
		t.Fatalf("cleanup retry: %v", err)
	}
	if listener.closeCalls != 2 {
		t.Fatalf("listener close calls = %d, want retry after the first error", listener.closeCalls)
	}
}

type fakeNetListener struct {
	closeErr   error
	closeCalls int
}

func (l *fakeNetListener) Accept() (net.Conn, error) { return nil, errors.New("not implemented") }
func (l *fakeNetListener) Close() error {
	l.closeCalls++
	return l.closeErr
}
func (l *fakeNetListener) Addr() net.Addr { return &net.TCPAddr{Port: 41001} }

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
	lease, listener, err := mgr.allocatePortAndListener("after-abandon")
	if err != nil {
		t.Fatalf("port was not released by the abandoned creation: %v", err)
	}
	_ = listener.Close()
	mgr.portAlloc.Release(lease)
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
