package api

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// ownershipState tracks how long it has been since a backend last proved it
// owns this control server (design 01, "Unowned shutdown"). The claim is a
// single explicit operation that both establishes and renews it; the
// bootstrap handshake and a successful credential rotation also renew it.
// It carries no instance identity: a server with zero instances is still
// owned.
type ownershipState struct {
	mu           sync.Mutex
	lastRenewal  time.Time
	shuttingDown bool
}

// newOwnershipState starts the clock at construction (process start), so a
// server whose owning backend dies before ever renewing reaps itself from
// its own launch time without any special-cased "never renewed" baseline.
func newOwnershipState() *ownershipState {
	return &ownershipState{lastRenewal: time.Now()}
}

// Renew records a successful claim, handshake, or rotation. Returns false
// without renewing when a shutdown has already begun (design 02's one-way
// door): a claim arriving after that point must be refused, not treated as
// reviving the ownership the shutdown decided to end.
func (o *ownershipState) Renew() bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.shuttingDown {
		return false
	}
	o.lastRenewal = time.Now()
	return true
}

// UnownedFor returns how long it has been since the last successful
// renewal, judged on this process's own clock per design 01 ("never on a
// wall-clock value either side supplies").
func (o *ownershipState) UnownedFor() time.Duration {
	o.mu.Lock()
	defer o.mu.Unlock()
	return time.Since(o.lastRenewal)
}

// BeginShutdown latches the one-way unowned-shutdown decision. Returns false
// if a shutdown had already begun, so a caller can distinguish "this call
// began it" from "it was already in progress" without a second check.
func (o *ownershipState) BeginShutdown() bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.shuttingDown {
		return false
	}
	o.shuttingDown = true
	return true
}

// TryBeginShutdownIfUnownedFor atomically checks whether at least period has
// elapsed since the last renewal and, if so, latches the one-way shutdown
// door in the same critical section (AC-EXECUTORS-CONTROL-OWNERSHIP-003.9).
// Performing the elapsed check and the latch under one lock acquisition
// closes the window a separate UnownedFor()-then-BeginShutdown() sequence
// leaves open: a claim's Renew() landing between those two calls would
// succeed (shuttingDown still false) while the reaper's latch still fires
// right after, killing every instance out from under a backend that was just
// told its claim succeeded. Returns true only when this call itself both
// observed the elapsed period and performed the latch.
func (o *ownershipState) TryBeginShutdownIfUnownedFor(period time.Duration) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.shuttingDown {
		return false
	}
	if time.Since(o.lastRenewal) < period {
		return false
	}
	o.shuttingDown = true
	return true
}

// IsShuttingDown reports whether the one-way shutdown latch has fired.
func (o *ownershipState) IsShuttingDown() bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.shuttingDown
}

// shuttingDownMessage is the shared one-way-door refusal message for every
// ownership-lifecycle operation (claim, rotate, confirm) once
// AC-EXECUTORS-CONTROL-OWNERSHIP-003.9's shutdown latch has fired.
const shuttingDownMessage = "control server is shutting down"

// handleOwnershipClaim establishes or renews ownership. Unlike /identity,
// this is a normal authenticated endpoint: it sits above the identity and
// capability negotiation in design 01's gate ordering.
func (m *ControlServer) handleOwnershipClaim(c *gin.Context) {
	if !m.ownership.Renew() {
		c.JSON(http.StatusConflict, gin.H{errKey: shuttingDownMessage})
		return
	}
	c.JSON(http.StatusOK, gin.H{lspStatusKey: "claimed"})
}

// handleOwnershipShutdown is the ownership-shutdown operation of
// AC-EXECUTORS-CONTROL-OWNERSHIP-002.9: it sits below capability negotiation
// (registered as an adoption-only path, so the superseded credential
// authenticates it too, per AC-002.7) and requires no prior adoption or
// rotation. It only destroys -- it cannot drive an instance or outlive the
// call -- which is why the wider credential set is safe to admit here.
//
// The actual "stop every instance and exit" work happens in the run loop
// (cmd/agentctl/main.go), which already owns that sequence for signal- and
// parent-death-triggered shutdown; this handler only latches the one-way
// shutdown door and signals the loop to run it, mirroring the existing
// parent-liveness trigger rather than duplicating teardown here.
func (m *ControlServer) handleOwnershipShutdown(c *gin.Context) {
	m.ownership.BeginShutdown()
	m.requestShutdown()
	c.JSON(http.StatusOK, gin.H{lspStatusKey: "shutting_down"})
}
