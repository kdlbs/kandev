package api

import (
	"net/http/httptest"
	"testing"
	"time"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/instance"
	"github.com/kandev/kandev/internal/common/logger"
)

// TestOwnershipStateRenewUpdatesLastRenewal pins that Renew advances the
// tracked instant, which the unowned-period reaper (Layer 5.5) will measure
// elapsed time against.
func TestOwnershipStateRenewUpdatesLastRenewal(t *testing.T) {
	o := newOwnershipState()
	time.Sleep(2 * time.Millisecond)
	before := o.UnownedFor()

	if !o.Renew() {
		t.Fatal("Renew() = false, want true")
	}
	after := o.UnownedFor()

	if after >= before {
		t.Fatalf("UnownedFor after Renew (%v) >= before (%v), want renewal to reset the elapsed duration", after, before)
	}
}

// TestOwnershipStateNewStartsFromProcessLaunch pins design 01's "A server
// spawned by a backend that then dies before completing the handshake has
// no successful renewal at all, so its period runs from process start and it
// reaps itself without special handling" -- the very first UnownedFor call,
// before any Renew, must already report a small nonzero-or-zero duration
// measured from construction, not an unset/zero-time sentinel that would
// read as "just now" forever or as an enormous elapsed duration.
func TestOwnershipStateNewStartsFromProcessLaunch(t *testing.T) {
	o := newOwnershipState()
	time.Sleep(2 * time.Millisecond)

	elapsed := o.UnownedFor()
	if elapsed <= 0 {
		t.Fatalf("UnownedFor() = %v, want a small positive duration measured from construction", elapsed)
	}
	if elapsed > time.Second {
		t.Fatalf("UnownedFor() = %v, want a small duration (construction just happened), not a large one implying a zero-time baseline", elapsed)
	}
}

// TestOwnershipStateBeginShutdownIsOneWay pins the one-way door: once a
// shutdown has begun, Renew must refuse (never resurrect a decided
// shutdown), and BeginShutdown itself is idempotent (a second call reports
// it was already begun rather than restarting the decision).
func TestOwnershipStateBeginShutdownIsOneWay(t *testing.T) {
	o := newOwnershipState()

	if !o.BeginShutdown() {
		t.Fatal("first BeginShutdown() = false, want true")
	}
	if o.BeginShutdown() {
		t.Fatal("second BeginShutdown() = true, want false (idempotent one-way latch)")
	}
	if !o.IsShuttingDown() {
		t.Fatal("IsShuttingDown() = false after BeginShutdown, want true")
	}
	if o.Renew() {
		t.Fatal("Renew() = true after shutdown began, want false (a decided shutdown must never be reversed)")
	}
}

// TestOwnershipStateTryBeginShutdownIfUnownedForRefusesAfterRecentRenewal
// pins the atomic check-and-latch's core correctness: a renewal that landed
// before the call must be observed by the SAME call that would otherwise
// latch shutdown, closing the TOCTOU window a separate UnownedFor()-then-
// BeginShutdown() sequence leaves open (Review round 1, finding 5).
func TestOwnershipStateTryBeginShutdownIfUnownedForRefusesAfterRecentRenewal(t *testing.T) {
	o := newOwnershipState()
	if !o.Renew() {
		t.Fatal("Renew() = false, want true")
	}

	if o.TryBeginShutdownIfUnownedFor(time.Hour) {
		t.Fatal("TryBeginShutdownIfUnownedFor() = true immediately after a renewal, want false")
	}
	if o.IsShuttingDown() {
		t.Fatal("IsShuttingDown() = true after a refused attempt, want false")
	}
}

// TestOwnershipStateTryBeginShutdownIfUnownedForLatchesOnceElapsed pins the
// success path: once the period has genuinely elapsed, the call both
// reports true and latches the one-way shutdown door.
func TestOwnershipStateTryBeginShutdownIfUnownedForLatchesOnceElapsed(t *testing.T) {
	o := newOwnershipState()
	o.mu.Lock()
	o.lastRenewal = time.Now().Add(-time.Hour)
	o.mu.Unlock()

	if !o.TryBeginShutdownIfUnownedFor(time.Minute) {
		t.Fatal("TryBeginShutdownIfUnownedFor() = false once the period elapsed, want true")
	}
	if !o.IsShuttingDown() {
		t.Fatal("IsShuttingDown() = false after a successful latch, want true")
	}
}

// TestOwnershipStateTryBeginShutdownIfUnownedForIsOneWay pins idempotency:
// a second call after the door is already latched must not report success
// again, matching BeginShutdown's existing one-way-door contract.
func TestOwnershipStateTryBeginShutdownIfUnownedForIsOneWay(t *testing.T) {
	o := newOwnershipState()
	o.mu.Lock()
	o.lastRenewal = time.Now().Add(-time.Hour)
	o.mu.Unlock()

	if !o.TryBeginShutdownIfUnownedFor(time.Minute) {
		t.Fatal("first TryBeginShutdownIfUnownedFor() = false, want true")
	}
	if o.TryBeginShutdownIfUnownedFor(time.Minute) {
		t.Fatal("second TryBeginShutdownIfUnownedFor() = true, want false (one-way latch)")
	}
}

// TestHandleOwnershipClaimRenewsOwnership pins that a successful claim
// renews the tracked ownership instant. The claim operation carries no
// instance identity -- a server with zero instances is still owned.
func TestHandleOwnershipClaimRenewsOwnership(t *testing.T) {
	cfg := &config.Config{AuthToken: "test-token"}
	log := logger.Default()
	cs := NewControlServer(cfg, &instance.Manager{}, log)
	server := httptest.NewServer(cs.Router())
	defer server.Close()
	host, port := parseHostPort(t, server.URL)
	client := agentctl.NewControlClient(host, port, log, agentctl.WithControlAuthToken("test-token"))

	backdateOwnership(cs, time.Hour)

	if err := client.ClaimOwnership(t.Context()); err != nil {
		t.Fatalf("ClaimOwnership: %v", err)
	}

	if elapsed := cs.ownership.UnownedFor(); elapsed >= time.Minute {
		t.Fatalf("UnownedFor after claim = %v, want well under the backdated hour (claim should have renewed)", elapsed)
	}
}

// backdateOwnership sets a ControlServer's tracked last-renewal instant into
// the past, so a test can assert that a subsequent operation renews it
// without racing HTTP round-trip latency against a tiny real elapsed
// duration.
func backdateOwnership(cs *ControlServer, age time.Duration) {
	cs.ownership.mu.Lock()
	defer cs.ownership.mu.Unlock()
	cs.ownership.lastRenewal = time.Now().Add(-age)
}

// TestHandleOwnershipClaimRequiresAuth pins that the claim operation sits
// ABOVE identity/capability negotiation in design 01's gate ordering: unlike
// /identity, it is a normal authenticated endpoint.
func TestHandleOwnershipClaimRequiresAuth(t *testing.T) {
	cfg := &config.Config{AuthToken: "test-token"}
	log := logger.Default()
	cs := NewControlServer(cfg, &instance.Manager{}, log)
	server := httptest.NewServer(cs.Router())
	defer server.Close()
	host, port := parseHostPort(t, server.URL)
	client := agentctl.NewControlClient(host, port, log) // no auth token

	if err := client.ClaimOwnership(t.Context()); err == nil {
		t.Fatal("ClaimOwnership without auth = nil error, want a rejection")
	}
}

// TestHandleOwnershipClaimRefusedAfterShutdownBegun pins the one-way-door
// refusal at the HTTP layer: a claim arriving after an unowned shutdown has
// begun must be refused with a shutting-down outcome rather than reviving
// ownership underneath a teardown in progress (design 02's failure table).
func TestHandleOwnershipClaimRefusedAfterShutdownBegun(t *testing.T) {
	cfg := &config.Config{AuthToken: "test-token"}
	log := logger.Default()
	cs := NewControlServer(cfg, &instance.Manager{}, log)
	cs.ownership.BeginShutdown()
	server := httptest.NewServer(cs.Router())
	defer server.Close()
	host, port := parseHostPort(t, server.URL)
	client := agentctl.NewControlClient(host, port, log, agentctl.WithControlAuthToken("test-token"))

	if err := client.ClaimOwnership(t.Context()); err == nil {
		t.Fatal("ClaimOwnership after shutdown began = nil error, want a rejection")
	}
}

// TestHandleHandshakeRenewsOwnership pins design 01's "two other operations
// renew it: the bootstrap handshake and a successful credential rotation" --
// a fresh server's first renewal comes from the handshake that mints its
// credential, giving the unowned-period timer a definite start without
// requiring a separate claim call immediately after.
func TestHandleHandshakeRenewsOwnership(t *testing.T) {
	cfg := &config.Config{
		AuthToken:      "test-generated-token",
		BootstrapNonce: "test-nonce",
	}
	log := logger.Default()
	cs := NewControlServer(cfg, &instance.Manager{}, log)
	server := httptest.NewServer(cs.Router())
	defer server.Close()
	host, port := parseHostPort(t, server.URL)
	client := agentctl.NewControlClient(host, port, log)

	backdateOwnership(cs, time.Hour)

	if _, err := client.Handshake(t.Context(), "test-nonce"); err != nil {
		t.Fatalf("Handshake: %v", err)
	}

	if elapsed := cs.ownership.UnownedFor(); elapsed >= time.Minute {
		t.Fatalf("UnownedFor after handshake = %v, want well under the backdated hour (handshake should have renewed)", elapsed)
	}
}

// TestNonRenewingControlOperationsDoNotRenewOwnership pins the negative
// clause of AC-EXECUTORS-CONTROL-OWNERSHIP-003.8: beyond the claim, the
// bootstrap handshake and a successful credential rotation, "no operation
// shall renew ownership: neither an instance operation, nor an open stream,
// nor enumeration". Only the three renewing operations are pinned
// positively elsewhere, which leaves the clause that actually carries the
// guarantee unguarded -- a renewal added to the shared credential
// middleware, or to any instance route, would make a backend that is busy
// but has stopped renewing look present forever and defeat the unowned
// shutdown entirely. Every case drives real HTTP through the router, so the
// middleware is on the path under test rather than bypassed.
//
// The clause's "open stream" arm is structural rather than testable here:
// the streaming routes live on the per-instance Server (server.go), which
// holds no ownership state at all, so no stream can reach this clock.
func TestNonRenewingControlOperationsDoNotRenewOwnership(t *testing.T) {
	const backdate = time.Hour

	cfg := &config.Config{AuthToken: "test-token"}
	log := logger.Default()
	cs := NewControlServer(cfg, &instance.Manager{}, log)
	server := httptest.NewServer(cs.Router())
	defer server.Close()
	host, port := parseHostPort(t, server.URL)
	client := agentctl.NewControlClient(host, port, log, agentctl.WithControlAuthToken("test-token"))

	// Each call asserts its own outcome before ownership is examined. A
	// request that never reached the handler would also leave ownership
	// unrenewed, so without these the test would pass vacuously against a
	// server that was not listening at all.
	cases := []struct {
		name string
		call func(t *testing.T)
	}{
		{"enumeration", func(t *testing.T) {
			if _, err := client.ListInstances(t.Context()); err != nil {
				t.Fatalf("ListInstances: %v", err)
			}
		}},
		{"instance operation", func(t *testing.T) {
			_, err := client.GetInstance(t.Context(), "no-such-instance")
			// The not-found wording is the handler's 404, distinct from the
			// "failed to get instance" a transport error produces, so this
			// asserts the request was served rather than merely refused.
			if err == nil || err.Error() != `instance "no-such-instance" not found` {
				t.Fatalf("GetInstance error = %v, want the handler's not-found response", err)
			}
		}},
		{"ownership details read", func(t *testing.T) {
			if _, err := client.GetServerDetails(t.Context()); err != nil {
				t.Fatalf("GetServerDetails: %v", err)
			}
		}},
		{"health", func(t *testing.T) {
			if err := client.Health(t.Context()); err != nil {
				t.Fatalf("Health: %v", err)
			}
		}},
		{"identity", func(t *testing.T) {
			if _, err := client.GetIdentity(t.Context()); err != nil {
				t.Fatalf("GetIdentity: %v", err)
			}
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			backdateOwnership(cs, backdate)
			tc.call(t)
			if elapsed := cs.ownership.UnownedFor(); elapsed < backdate {
				t.Fatalf("UnownedFor() after %s = %v, want >= %v (this operation must not renew ownership)", tc.name, elapsed, backdate)
			}
		})
	}
}
