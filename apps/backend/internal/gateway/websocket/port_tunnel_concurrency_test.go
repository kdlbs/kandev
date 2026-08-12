package websocket

import (
	"net/http"
	"testing"
	"time"
)

// startOutcome is one concurrent StartTunnel result. A panic in a racing
// goroutine is recovered and reported here so it fails the test that caused it
// instead of taking the whole test binary down with it.
type startOutcome struct {
	port      int
	err       error
	recovered any
}

// startTunnelRacing runs StartTunnel in its own goroutine, releasing gate first
// so every caller enters the reservation window together.
func startTunnelRacing(
	t *testing.T,
	manager *TunnelManager,
	gate <-chan struct{},
	sessionID string,
	port int,
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
		outcome.port, outcome.err = manager.StartTunnel(sessionID, port, 0)
	}()
	return out
}

// awaitStart collects one racing StartTunnel result under a bound. A leaked
// m.mu blocks the caller forever, so an unbounded receive here would hang the
// suite instead of reporting the defect.
func awaitStart(t *testing.T, what string, out <-chan startOutcome) startOutcome {
	t.Helper()
	timer := time.NewTimer(wsTestTimeout)
	defer timer.Stop()
	select {
	case outcome := <-out:
		if outcome.recovered != nil {
			t.Errorf("%s panicked: %v", what, outcome.recovered)
		}
		return outcome
	case <-timer.C:
		t.Fatalf("%s did not return within %s: TunnelManager.mu is held by a goroutine that never released it", what, wsTestTimeout)
		return startOutcome{}
	}
}

// mustNotBlock fails if fn has not returned within wsTestTimeout. Every
// TunnelManager method takes m.mu, so a lock leaked by a panicking StartTunnel
// shows up here as a hang rather than as an error.
func mustNotBlock(t *testing.T, what string, fn func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		fn()
	}()
	timer := time.NewTimer(wsTestTimeout)
	defer timer.Stop()
	select {
	case <-done:
	case <-timer.C:
		t.Fatalf("%s blocked for %s: TunnelManager.mu was never released", what, wsTestTimeout)
	}
}

// newRacingTunnelManager returns a manager whose cleanup cannot hang the suite:
// Shutdown takes m.mu, so a leaked lock would block teardown forever.
func newRacingTunnelManager(t *testing.T, sessionIDs ...string) *TunnelManager {
	t.Helper()
	log, _ := observedTerminalLogger(t)
	upstream := newFakeAgentctl(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	lifecycleMgr := newProxyTestManager(t, log)
	for _, sessionID := range sessionIDs {
		addExecutionForURL(t, lifecycleMgr, sessionID, upstream.server.URL, "", log)
	}
	manager := NewTunnelManager(lifecycleMgr, log)
	t.Cleanup(func() { mustNotBlock(t, "Shutdown", manager.Shutdown) })
	return manager
}

// Two clicks on "expose port" arrive as two ports.tunnel.start actions, and
// client.go dispatches each in its own goroutine. Both callers must end up
// sharing a single tunnel instead of observing the reservation the first one
// left behind mid-bind.
func TestStartTunnelIsSafeUnderConcurrentDuplicateStarts(t *testing.T) {
	manager := newRacingTunnelManager(t, "sess-race")

	gate := make(chan struct{})
	first := startTunnelRacing(t, manager, gate, "sess-race", 3000)
	second := startTunnelRacing(t, manager, gate, "sess-race", 3000)
	close(gate)

	firstOutcome := awaitStart(t, "first StartTunnel", first)
	secondOutcome := awaitStart(t, "second StartTunnel", second)

	for _, outcome := range []struct {
		what string
		got  startOutcome
	}{{"first", firstOutcome}, {"second", secondOutcome}} {
		if outcome.got.err != nil {
			t.Fatalf("%s StartTunnel() error = %v, want nil", outcome.what, outcome.got.err)
		}
		if outcome.got.port <= 0 {
			t.Fatalf("%s StartTunnel() port = %d, want an OS-assigned port", outcome.what, outcome.got.port)
		}
	}
	if firstOutcome.port != secondOutcome.port {
		t.Fatalf("concurrent starts returned different ports (%d and %d), want one shared tunnel",
			firstOutcome.port, secondOutcome.port)
	}

	listed := tunnelList(t, manager, "sess-race")
	if len(listed) != 1 {
		t.Fatalf("tunnels = %v, want exactly 1 (a duplicate start must not bind a second listener)", listed)
	}
	if listed[0].TunnelPort != firstOutcome.port {
		t.Fatalf("listed tunnel port = %d, want the started %d", listed[0].TunnelPort, firstOutcome.port)
	}
	if status, _ := getThroughTunnel(t, newTunnelClient(t), firstOutcome.port, "/"); status != http.StatusOK {
		t.Fatalf("shared tunnel status = %d, want %d", status, http.StatusOK)
	}
}

// The serious half of the defect: StartTunnel dereferenced the reservation
// while holding m.mu behind a non-deferred Unlock, so a panic there stranded
// the lock and every later start/stop/list/invalidate/shutdown blocked for the
// process lifetime, graceful backend shutdown included.
func TestTunnelManagerStaysUsableAfterConcurrentDuplicateStarts(t *testing.T) {
	manager := newRacingTunnelManager(t, "sess-usable")

	gate := make(chan struct{})
	first := startTunnelRacing(t, manager, gate, "sess-usable", 3000)
	second := startTunnelRacing(t, manager, gate, "sess-usable", 3000)
	close(gate)
	tunnelPort := awaitStart(t, "first StartTunnel", first).port
	awaitStart(t, "second StartTunnel", second)

	mustNotBlock(t, "ListTunnels after a duplicate start", func() {
		_ = manager.ListTunnels("sess-usable")
	})
	mustNotBlock(t, "StartTunnel for another port after a duplicate start", func() {
		if _, err := manager.StartTunnel("sess-usable", 4000, 0); err != nil {
			t.Errorf("StartTunnel(4000) error = %v, want nil", err)
		}
	})
	mustNotBlock(t, "InvalidateSession after a duplicate start", func() {
		manager.InvalidateSession("sess-nobody")
	})
	mustNotBlock(t, "StopTunnel after a duplicate start", func() {
		if err := manager.StopTunnel("sess-usable", 3000); err != nil {
			t.Errorf("StopTunnel() error = %v, want nil", err)
		}
	})
	assertPortClosed(t, tunnelPort)

	mustNotBlock(t, "Shutdown after a duplicate start", manager.Shutdown)
}

// Every reservation must be released even when many callers pile onto the same
// key and the start fails, or the session:port pair stays permanently unusable.
func TestConcurrentFailingStartsReleaseTheReservation(t *testing.T) {
	manager := newRacingTunnelManager(t)

	const callers = 8
	gate := make(chan struct{})
	outs := make([]<-chan startOutcome, 0, callers)
	for range callers {
		outs = append(outs, startTunnelRacing(t, manager, gate, "sess-missing", 3000))
	}
	close(gate)

	for i, out := range outs {
		outcome := awaitStart(t, "failing StartTunnel", out)
		if outcome.err == nil {
			t.Fatalf("caller %d: StartTunnel() error = nil, want a failure", i)
		}
		if outcome.port != 0 {
			t.Fatalf("caller %d: StartTunnel() port = %d, want 0 on failure", i, outcome.port)
		}
	}

	// The key is usable again: nothing was left reserved by the failed round.
	log, _ := observedTerminalLogger(t)
	upstream := newFakeAgentctl(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	addExecutionForURL(t, manager.lifecycleMgr, "sess-missing", upstream.server.URL, "", log)
	mustNotBlock(t, "StartTunnel after concurrent failures", func() {
		if _, err := manager.StartTunnel("sess-missing", 3000, 0); err != nil {
			t.Errorf("StartTunnel after the failed round: %v", err)
		}
	})
}
