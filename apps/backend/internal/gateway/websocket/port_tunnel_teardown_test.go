package websocket

import (
	"errors"
	"net/http"
	"testing"
)

// startTunnelRacingOnPort is startTunnelRacing for a caller that needs to know
// the tunnel port up front, so the port can be checked whichever way the race
// went.
func startTunnelRacingOnPort(
	t *testing.T,
	manager *TunnelManager,
	gate <-chan struct{},
	sessionID string,
	port, tunnelPort int,
) <-chan startOutcome {
	t.Helper()
	out := make(chan startOutcome, 1)
	go func() {
		outcome := startOutcome{}
		defer func() {
			outcome.recovered = recover()
			out <- outcome
		}()
		<-gate
		outcome.port, outcome.err = manager.StartTunnel(sessionID, port, tunnelPort)
	}()
	return out
}

// A start still binding when its session or the whole manager is torn down must
// be canceled. Its reservation is the cancellation token: once the teardown has
// dropped it, publish refuses to install the tunnel, so no listener survives a
// teardown that could not see it.
func TestTeardownCancelsInFlightStarts(t *testing.T) {
	tests := []struct {
		name     string
		teardown func(*TunnelManager)
	}{
		{name: "Shutdown", teardown: func(m *TunnelManager) { m.Shutdown() }},
		{name: "InvalidateSession", teardown: func(m *TunnelManager) { m.InvalidateSession("sess-teardown") }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager := newRacingTunnelManager(t)
			started, settle := reserveInFlightStart(t, manager, "sess-teardown:3000")

			tt.teardown(manager)

			manager.mu.Lock()
			_, stillReserved := manager.pending["sess-teardown:3000"]
			manager.mu.Unlock()
			if stillReserved {
				t.Fatalf("%s left the in-flight reservation in place", tt.name)
			}

			// The owning start now finishes its bind and must abandon it.
			if manager.publish("sess-teardown:3000", started, &tunnelEntry{port: 3000, tunnelPort: 41000}) {
				t.Fatalf("publish installed a tunnel after %s canceled its reservation", tt.name)
			}
			manager.mu.Lock()
			installed := len(manager.tunnels)
			manager.mu.Unlock()
			if installed != 0 {
				t.Fatalf("tunnels after a canceled start = %d, want 0", installed)
			}

			settle(0, errStartCanceled)
		})
	}
}

// A canceled reservation must not take a newer one down with it: after a
// teardown, the next start owns the key, and the canceled start's finishStart
// must leave that newcomer's reservation alone.
func TestFinishStartKeepsANewerReservation(t *testing.T) {
	manager := newRacingTunnelManager(t)
	canceled, settleCanceled := reserveInFlightStart(t, manager, "sess-newer:3000")
	manager.InvalidateSession("sess-newer")

	newer := &pendingTunnel{done: make(chan struct{})}
	manager.mu.Lock()
	manager.pending["sess-newer:3000"] = newer
	manager.mu.Unlock()
	t.Cleanup(func() { _, _ = manager.finishStart("sess-newer:3000", newer, 0, errors.New("test cleanup")) })

	settleCanceled(0, errStartCanceled)

	manager.mu.Lock()
	owner := manager.pending["sess-newer:3000"]
	manager.mu.Unlock()
	if owner != newer {
		t.Fatalf("the canceled start's finishStart dropped the newer reservation (owner = %p, want %p)", owner, newer)
	}
	if manager.publish("sess-newer:3000", canceled, &tunnelEntry{port: 3000}) {
		t.Fatal("publish installed a tunnel for the canceled start while a newer one owned the key")
	}
}

// End-to-end property, whatever the interleaving: a StartTunnel racing Shutdown
// either publishes a tunnel that Shutdown then closes, or is canceled and
// closes its own listener. Nothing may still be listening afterwards, and the
// manager must be left empty.
func TestStartTunnelRacingShutdownLeavesNoListener(t *testing.T) {
	log, _ := observedTerminalLogger(t)
	backend := newTunnelRaceBackend(t, log, "sess-shutdown-race")

	const attempts = 20
	published, canceled, refused, unbound := 0, 0, 0, 0
	for range attempts {
		manager := newBoundedTunnelManager(t, backend, log)
		wanted := reserveLoopbackPort(t)

		gate := make(chan struct{})
		start := startTunnelRacingOnPort(t, manager, gate, "sess-shutdown-race", 3000, wanted)
		shutdownDone := make(chan struct{})
		go func() {
			defer close(shutdownDone)
			<-gate
			manager.Shutdown()
		}()
		close(gate)

		outcome := awaitStart(t, "StartTunnel racing Shutdown", start)
		mustNotBlock(t, "Shutdown racing StartTunnel", func() { <-shutdownDone })

		switch {
		case outcome.err == nil:
			published++
			assertPortClosed(t, wanted) // Shutdown drained the published entry.
		case errors.Is(outcome.err, errStartCanceled):
			canceled++
			assertPortClosed(t, wanted) // The canceled start closed its own listener.
		case errors.Is(outcome.err, errManagerClosed):
			refused++
			assertPortClosed(t, wanted) // Refused before binding anything.
		default:
			// The reserved port was taken between reserve and bind; another
			// process owns it, so its state says nothing about this manager.
			unbound++
		}

		manager.mu.Lock()
		leftTunnels, leftPending := len(manager.tunnels), len(manager.pending)
		manager.mu.Unlock()
		if leftTunnels != 0 || leftPending != 0 {
			t.Fatalf("after the race: tunnels = %d, pending = %d, want 0 and 0", leftTunnels, leftPending)
		}
	}
	t.Logf("outcomes over %d attempts: published=%d canceled=%d refused=%d port-unavailable=%d",
		attempts, published, canceled, refused, unbound)
}

// A tunnel published before a teardown is still closed by it; this pins the
// ordering the property test above relies on.
func TestShutdownClosesATunnelPublishedBeforeIt(t *testing.T) {
	manager := newRacingTunnelManager(t, "sess-ordered")

	tunnelPort, err := manager.StartTunnel("sess-ordered", 3000, 0)
	if err != nil {
		t.Fatalf("StartTunnel: %v", err)
	}
	if status, _ := getThroughTunnel(t, newTunnelClient(t), tunnelPort, "/"); status != http.StatusOK {
		t.Fatalf("status before shutdown = %d, want %d", status, http.StatusOK)
	}

	manager.Shutdown()
	assertPortClosed(t, tunnelPort)

	// Shutdown is terminal. A start afterwards must be refused rather than bind
	// a listener that nothing is left to close.
	again, err := manager.StartTunnel("sess-ordered", 3000, 0)
	if !errors.Is(err, errManagerClosed) {
		t.Fatalf("StartTunnel after Shutdown error = %v, want %v", err, errManagerClosed)
	}
	if again != 0 {
		t.Fatalf("StartTunnel after Shutdown port = %d, want 0", again)
	}
}
