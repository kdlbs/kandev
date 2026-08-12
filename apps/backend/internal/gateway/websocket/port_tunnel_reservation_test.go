package websocket

import (
	"errors"
	"testing"
	"time"
)

// reserveInFlightStart seeds the reservation StartTunnel takes while it
// resolves and binds, and returns the settle function the owning start would
// call. Binding is a real syscall of unbounded duration, so this window is wide
// enough in production for every other method to run inside it.
func reserveInFlightStart(t *testing.T, manager *TunnelManager, cacheKey string) (*pendingTunnel, func(port int, err error)) {
	t.Helper()
	started := &pendingTunnel{done: make(chan struct{})}
	manager.mu.Lock()
	manager.pending[cacheKey] = started
	manager.mu.Unlock()

	var settled bool
	settle := func(port int, err error) {
		if settled {
			return
		}
		settled = true
		_, _ = manager.finishStart(cacheKey, started, port, err)
	}
	// Registered immediately: a t.Fatal below must not strand callers that are
	// already blocked on this reservation.
	t.Cleanup(func() { settle(0, errors.New("test cleanup")) })
	return started, settle
}

// A start that has not finished binding is invisible to every other method:
// none of them can observe a half-built tunnel, and none of them blocks on the
// in-flight bind.
func TestInFlightStartIsInvisibleToTheOtherReaders(t *testing.T) {
	manager := newRacingTunnelManager(t)
	_, settle := reserveInFlightStart(t, manager, "sess-inflight:3000")

	mustNotBlock(t, "ListTunnels during an in-flight start", func() {
		if listed := tunnelList(t, manager, "sess-inflight"); len(listed) != 0 {
			t.Errorf("ListTunnels() = %v during an in-flight start, want none", listed)
		}
	})
	mustNotBlock(t, "StopTunnel during an in-flight start", func() {
		if err := manager.StopTunnel("sess-inflight", 3000); err == nil {
			t.Error("StopTunnel() error = nil during an in-flight start, want a not-found error")
		}
	})
	mustNotBlock(t, "InvalidateSession for another session during an in-flight start", func() {
		manager.InvalidateSession("sess-unrelated")
	})
	// A start for a different session:port is not blocked by this reservation.
	mustNotBlock(t, "StartTunnel for another key during an in-flight start", func() {
		if _, err := manager.StartTunnel("sess-other", 3000, 0); err == nil {
			t.Error("StartTunnel(sess-other) error = nil, want the missing-session failure")
		}
	})
	// Last, because Shutdown is terminal.
	mustNotBlock(t, "Shutdown during an in-flight start", manager.Shutdown)

	settle(41234, nil)
}

// A caller that loses the reservation race waits for the in-flight bind and
// shares its result, rather than binding a second listener or reading a
// half-built entry.
func TestDuplicateStartWaitsForTheInFlightResult(t *testing.T) {
	tests := []struct {
		name      string
		port      int
		err       error
		wantPort  int
		wantError string
	}{
		{name: "in-flight start succeeds", port: 41234, wantPort: 41234},
		{name: "in-flight start fails", err: errors.New("bind exploded"), wantError: "bind exploded"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager := newRacingTunnelManager(t)
			_, settle := reserveInFlightStart(t, manager, "sess-wait:3000")

			gate := make(chan struct{})
			close(gate)
			waiter := startTunnelRacing(t, manager, gate, "sess-wait", 3000)

			// The waiter must be parked without holding m.mu, so an unrelated
			// reader still runs while it waits.
			mustNotBlock(t, "ListTunnels while a duplicate start waits", func() {
				_ = manager.ListTunnels("sess-wait")
			})
			// ListTunnels above is the m.mu liveness proof. This select is a
			// secondary sanity-check that the waiter has not returned early;
			// 50ms is ample for an in-process channel wait to park.
			select {
			case outcome := <-waiter:
				t.Fatalf("duplicate StartTunnel returned %+v before the in-flight start settled", outcome)
			case <-time.After(50 * time.Millisecond):
			}

			settle(tt.port, tt.err)

			outcome := awaitStart(t, "duplicate StartTunnel", waiter)
			if outcome.port != tt.wantPort {
				t.Fatalf("duplicate StartTunnel() port = %d, want the in-flight %d", outcome.port, tt.wantPort)
			}
			if tt.wantError == "" {
				if outcome.err != nil {
					t.Fatalf("duplicate StartTunnel() error = %v, want nil", outcome.err)
				}
				return
			}
			if outcome.err == nil || outcome.err.Error() != tt.wantError {
				t.Fatalf("duplicate StartTunnel() error = %v, want %q", outcome.err, tt.wantError)
			}
		})
	}
}

// The reservation is released even when the start fails, so the same
// session:port can be started again.
func TestFinishStartReleasesTheReservation(t *testing.T) {
	manager := newRacingTunnelManager(t)
	_, settle := reserveInFlightStart(t, manager, "sess-release:3000")
	settle(0, errors.New("bind exploded"))

	manager.mu.Lock()
	_, stillReserved := manager.pending["sess-release:3000"]
	manager.mu.Unlock()
	if stillReserved {
		t.Fatal("finishStart left the reservation in place")
	}

	// A later start owns the key again instead of waiting on a dead reservation.
	mustNotBlock(t, "StartTunnel after a released reservation", func() {
		if _, err := manager.StartTunnel("sess-release", 3000, 0); err == nil {
			t.Error("StartTunnel() error = nil, want the missing-session failure")
		}
	})
}
