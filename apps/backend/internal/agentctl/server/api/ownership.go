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

// IsShuttingDown reports whether the one-way shutdown latch has fired.
func (o *ownershipState) IsShuttingDown() bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.shuttingDown
}

// handleOwnershipClaim establishes or renews ownership. Unlike /identity,
// this is a normal authenticated endpoint: it sits above the identity and
// capability negotiation in design 01's gate ordering.
func (m *ControlServer) handleOwnershipClaim(c *gin.Context) {
	if !m.ownership.Renew() {
		c.JSON(http.StatusConflict, gin.H{errKey: "control server is shutting down"})
		return
	}
	c.JSON(http.StatusOK, gin.H{lspStatusKey: "claimed"})
}
