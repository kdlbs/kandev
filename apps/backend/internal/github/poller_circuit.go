package github

import (
	"sync"
	"time"

	"github.com/kandev/kandev/internal/common/authcircuit"
)

// pollerCircuits tracks a per-workspace in-memory auth/config circuit for
// the PR-monitor loop (Poller.checkPRWatches). It is in-memory only, never
// persisted — mirroring the existing repoErrorCache precedent (see
// errors.go): a backend restart clears it, costing at most one wasted probe
// per workspace on the next cycle rather than resuming a sustained
// hammering pattern. review/issue watch loops are not covered by this wave
// (see task-04 results).
type pollerCircuits struct {
	mu          sync.Mutex
	byWorkspace map[string]authcircuit.State
}

func newPollerCircuits() *pollerCircuits {
	return &pollerCircuits{byWorkspace: make(map[string]authcircuit.State)}
}

// open reports whether workspaceID's circuit currently forbids a poll.
func (c *pollerCircuits) open(workspaceID string, now time.Time) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.byWorkspace[workspaceID].Open(now)
}

// recordOutcome updates workspaceID's circuit after a poll attempt.
// FailureClassNone (nil error) clears any prior failure state.
func (c *pollerCircuits) recordOutcome(workspaceID string, class authcircuit.FailureClass, now time.Time) {
	if workspaceID == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	state := c.byWorkspace[workspaceID]
	if class == authcircuit.FailureClassNone {
		state.RecordSuccess()
	} else {
		state.RecordFailure(now, class, nil)
	}
	c.byWorkspace[workspaceID] = state
}

// resetIfFingerprintChanged resets workspaceID's circuit when its
// credential fingerprint (Service.WorkspaceConnectionFingerprint) has
// changed since last observed — a rotate/reconnect/revoke-then-reconfigure
// resumes polling on the very next cycle instead of waiting out the
// remaining backoff. Returns true when a reset happened.
func (c *pollerCircuits) resetIfFingerprintChanged(workspaceID, fingerprint string) bool {
	if workspaceID == "" {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	state := c.byWorkspace[workspaceID]
	changed := state.ResetIfFingerprintChanged(fingerprint)
	c.byWorkspace[workspaceID] = state
	return changed
}

// summary aggregates open-circuit counts by failure class, for bounded
// health/metrics reporting. Never returns workspace IDs.
func (c *pollerCircuits) summary(now time.Time) (openAuth, openConfig, openTransient int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, state := range c.byWorkspace {
		if !state.Open(now) {
			continue
		}
		switch state.FailureClass {
		case authcircuit.FailureClassAuth:
			openAuth++
		case authcircuit.FailureClassConfig:
			openConfig++
		default:
			openTransient++
		}
	}
	return openAuth, openConfig, openTransient
}
