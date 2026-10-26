package instance

import (
	"context"
	"errors"
	"net"
	"strconv"
	"testing"
	"time"

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
			next, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(candidatePort+1))
			if err != nil {
				_ = candidate.Close()
				continue
			}
			_ = next.Close()
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

	lease, listener, err := mgr.allocatePortAndListener("retry")
	if err != nil {
		t.Fatalf("allocatePortAndListener: %v", err)
	}
	t.Cleanup(func() {
		_ = listener.Close()
		mgr.portAlloc.Release(lease)
	})

	if lease.Port != base+1 {
		t.Fatalf("allocated port = %d, want retry on %d", lease.Port, base+1)
	}
}

func occupySinglePort(t *testing.T) (net.Listener, int) {
	t.Helper()
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen for occupied port: %v", err)
	}
	t.Cleanup(func() { _ = occupied.Close() })
	return occupied, occupied.Addr().(*net.TCPAddr).Port
}

func newSinglePortManager(t *testing.T, port int) *Manager {
	t.Helper()
	mgr := NewManager(&config.Config{Ports: config.PortConfig{Base: port, Max: port}}, newTestLogger(t))
	t.Cleanup(func() { _ = mgr.Shutdown(context.Background()) })
	mgr.portBusyDelay, mgr.portBusyAttempts = time.Millisecond, 5000
	return mgr
}

func TestAdmitCreateWaitsForSinglePortToBeReleased(t *testing.T) {
	occupied, port := occupySinglePort(t)
	mgr := newSinglePortManager(t, port)

	released := make(chan struct{})
	go func() {
		defer close(released)
		time.Sleep(20 * time.Millisecond)
		_ = occupied.Close()
	}()

	admission, err := mgr.admitCreateWithPortWait(context.Background(), &CreateRequest{ID: "waiter"})
	<-released
	if err != nil {
		t.Fatalf("admitCreateWithPortWait: %v", err)
	}
	if admission.bundle.lease.Port != port {
		t.Fatalf("lease port = %d, want %d", admission.bundle.lease.Port, port)
	}
	t.Cleanup(func() {
		_ = mgr.abandonPartialInstance(admission.bundle)
		mgr.activeCreations.Done()
	})
}

func TestAdmitCreateGivesUpAfterTheConfiguredRetries(t *testing.T) {
	_, port := occupySinglePort(t)
	mgr := newSinglePortManager(t, port)
	mgr.portBusyAttempts = 3

	_, err := mgr.admitCreateWithPortWait(context.Background(), &CreateRequest{ID: "never"})
	if !errors.Is(err, errPortsBusy) {
		t.Fatalf("err = %v, want errPortsBusy", err)
	}
}

func TestAdmitCreateWaitEndsWithTheCallerContext(t *testing.T) {
	_, port := occupySinglePort(t)
	mgr := newSinglePortManager(t, port)
	mgr.portBusyDelay = time.Hour
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := mgr.admitCreateWithPortWait(ctx, &CreateRequest{ID: "gone"})
	if err == nil || errors.Is(err, errPortsBusy) {
		t.Fatalf("err = %v, want a context error", err)
	}
}
